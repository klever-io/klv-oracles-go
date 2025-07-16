package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/address"
	nonceHandler "github.com/klever-io/klv-bridge-eth-go/clients/klever/interactors/nonceHandlerV2"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/proxy"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/proxy/models"
	"github.com/klever-io/klv-oracles-go/aggregator"
	"github.com/klever-io/klv-oracles-go/aggregator/api/gin"
	"github.com/klever-io/klv-oracles-go/aggregator/fetchers"
	gas "github.com/klever-io/klv-oracles-go/aggregator/gasStation"
	"github.com/klever-io/klv-oracles-go/aggregator/notifees"
	"github.com/klever-io/klv-oracles-go/config"
	"github.com/klever-io/klv-oracles-go/tools/wallet"
	chainCore "github.com/multiversx/mx-chain-core-go/core"
	"github.com/multiversx/mx-chain-core-go/core/check"
	chainFactory "github.com/multiversx/mx-chain-go/cmd/node/factory"
	chainCommon "github.com/multiversx/mx-chain-go/common"
	logger "github.com/multiversx/mx-chain-logger-go"
	"github.com/multiversx/mx-chain-logger-go/file"
	"github.com/multiversx/mx-sdk-go/core/polling"
	"github.com/urfave/cli"
)

const (
	defaultLogsPath = "logs"
	logFilePrefix   = "klv-oracle"
)

var log = logger.GetOrCreate("priceFeeder/main")

// appVersion should be populated at build time using ldflags
// Usage examples:
// linux/mac:
//
//	go build -i -v -ldflags="-X main.appVersion=$(git describe --tags --long --dirty)"
//
// windows:
//
//	for /f %i in ('git describe --tags --long --dirty') do set VERS=%i
//	go build -i -v -ldflags="-X main.appVersion=%VERS%"
var appVersion = chainCommon.UnVersionedAppString

func main() {
	app := cli.NewApp()
	app.Name = "Relay CLI app"
	app.Usage = "Price feeder will fetch the price of a defined pair from a bunch of exchanges, and will" +
		" write to the contract if the price changed"
	app.Flags = getFlags()
	machineID := chainCore.GetAnonymizedMachineID(app.Name)
	app.Version = fmt.Sprintf("%s/%s/%s-%s/%s", appVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, machineID)
	app.Authors = []cli.Author{
		{
			Name:  "The Klever Blockchain Team",
			Email: "contact@klever.io",
		},
	}

	app.Action = func(c *cli.Context) error {
		return startOracle(c, app.Version)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func startOracle(ctx *cli.Context, version string) error {
	flagsConfig := getFlagsConfig(ctx)

	fileLogging, errLogger := attachFileLogger(log, flagsConfig)
	if errLogger != nil {
		return errLogger
	}

	log.Info("starting oracle node", "version", version, "pid", os.Getpid())

	err := logger.SetLogLevel(flagsConfig.LogLevel)
	if err != nil {
		return err
	}

	cfg, err := loadConfig(flagsConfig.ConfigurationFile)
	if err != nil {
		return err
	}

	if !check.IfNil(fileLogging) {
		logsCfg := cfg.GeneralConfig.Logs
		timeLogLifeSpan := time.Second * time.Duration(logsCfg.LogFileLifeSpanInSec)
		sizeLogLifeSpanInMB := uint64(logsCfg.LogFileLifeSpanInMB)
		err = fileLogging.ChangeFileLifeSpan(timeLogLifeSpan, sizeLogLifeSpanInMB)
		if err != nil {
			return err
		}
	}

	for key, val := range cfg.XExchangeTokenIDsMappings {
		log.Info("read xExchange token IDs mapping", "key", key, "quote", val.Quote, "base", val.Base)
	}

	if len(cfg.GeneralConfig.NetworkAddress) == 0 {
		return fmt.Errorf("empty NetworkAddress in config file")
	}

	argsProxy := proxy.ArgsProxy{
		ProxyURL:            cfg.GeneralConfig.NetworkAddress,
		SameScState:         false,
		ShouldBeSynced:      false,
		FinalityCheck:       cfg.GeneralConfig.ProxyFinalityCheck,
		AllowedDeltaToFinal: cfg.GeneralConfig.ProxyMaxNoncesDelta,
		CacheExpirationTime: time.Second * time.Duration(cfg.GeneralConfig.ProxyCacherExpirationSeconds),
		EntityType:          models.RestAPIEntityType(cfg.GeneralConfig.ProxyRestAPIEntityType),
	}
	proxy, err := proxy.NewProxy(argsProxy)
	if err != nil {
		return err
	}

	args := nonceHandler.ArgsNonceTransactionsHandlerV2{
		Proxy:            proxy,
		IntervalToResend: time.Second * time.Duration(cfg.GeneralConfig.IntervalToResendTxsInSeconds),
	}
	txNonceHandler, err := nonceHandler.NewNonceTransactionHandlerV2(args)
	if err != nil {
		return err
	}

	aggregatorAddress, err := address.NewAddress(cfg.GeneralConfig.AggregatorContractAddress)
	if err != nil {
		return err
	}

	oracleWallet, err := wallet.NewWalletFromPEM(cfg.GeneralConfig.PrivateKeyFile)
	if err != nil {
		return err
	}

	httpResponseGetter, err := aggregator.NewHttpResponseGetter()
	if err != nil {
		return err
	}

	priceFetchers, err := createPriceFetchers(httpResponseGetter)
	if err != nil {
		return err
	}

	argsPriceAggregator := aggregator.ArgsPriceAggregator{
		PriceFetchers: priceFetchers,
		MinResultsNum: cfg.GeneralConfig.MinResultsNum,
	}
	priceAggregator, err := aggregator.NewPriceAggregator(argsPriceAggregator)
	if err != nil {
		return err
	}

	argsNotifee := notifees.ArgsKCNotifee{
		Proxy:           proxy,
		TxNonceHandler:  txNonceHandler,
		ContractAddress: aggregatorAddress,
		Wallet:          oracleWallet,
		BaseGasLimit:    cfg.GeneralConfig.BaseGasLimit,
		GasLimitForEach: cfg.GeneralConfig.GasLimitForEach,
	}
	kcNotifee, err := notifees.NewKCNotifee(argsNotifee)
	if err != nil {
		return err
	}

	gasStationFetcherArgs := fetchers.ArgsPriceFetcher{
		FetcherName:    fetchers.EVMGasPriceStation,
		ResponseGetter: httpResponseGetter,
		EVMGasConfig: fetchers.EVMGasPriceFetcherConfig{
			ApiURL:   cfg.GeneralConfig.GasStationAPI,
			Selector: "SafeGasPrice",
		},
	}

	gasPriceFetcher, err := fetchers.NewPriceFetcher(gasStationFetcherArgs)
	if err != nil {
		return err
	}

	gasServiceArgs := gas.ArgsGasPriceService{
		GasPriceFetcher: gasPriceFetcher,
	}

	gasService, err := gas.NewGasPriceService(gasServiceArgs)
	if err != nil {
		return err
	}

	argsPriceNotifier := aggregator.ArgsPriceNotifier{
		Pairs:            []*aggregator.ArgsPair{},
		Aggregator:       priceAggregator,
		GasPriceService:  gasService,
		Notifee:          kcNotifee,
		AutoSendInterval: time.Second * time.Duration(cfg.GeneralConfig.AutoSendIntervalInSeconds),
	}
	for _, pair := range cfg.Pairs {
		argsPair := aggregator.ArgsPair{
			Base:                      pair.Base,
			Quote:                     pair.Quote,
			PercentDifferenceToNotify: pair.PercentDifferenceToNotify,
			Decimals:                  pair.Decimals,
			Exchanges:                 getMapFromSlice(pair.Exchanges),
		}
		addPairToFetchers(argsPair, priceFetchers)
		argsPriceNotifier.Pairs = append(argsPriceNotifier.Pairs, &argsPair)
	}

	for _, pair := range cfg.GasStationPair {
		gasArgsPair := aggregator.ArgsPair{
			Base:                      "GWEI",
			Quote:                     pair.Quote,
			PercentDifferenceToNotify: pair.PercentDifferenceToNotify,
			Decimals:                  pair.Decimals,
			Exchanges:                 getMapFromSlice(pair.Exchanges),
		}

		gasPriceFetcher.AddPair(gasArgsPair.Base, gasArgsPair.Quote)
		argsPriceNotifier.Pairs = append(argsPriceNotifier.Pairs, &gasArgsPair)
	}

	priceNotifier, err := aggregator.NewPriceNotifier(argsPriceNotifier)
	if err != nil {
		return err
	}

	argsPollingHandler := polling.ArgsPollingHandler{
		Log:              log,
		Name:             "price notifier polling handler",
		PollingInterval:  time.Second * time.Duration(cfg.GeneralConfig.PollIntervalInSeconds),
		PollingWhenError: time.Second * time.Duration(cfg.GeneralConfig.PollIntervalInSeconds),
		Executor:         priceNotifier,
	}

	pollingHandler, err := polling.NewPollingHandler(argsPollingHandler)
	if err != nil {
		return err
	}

	httpServerWrapper, err := gin.NewWebServerHandler(flagsConfig.RestApiInterface)
	if err != nil {
		return err
	}

	err = httpServerWrapper.StartHttpServer()
	if err != nil {
		return err
	}

	log.Info("Starting Klever Blockchain Notifee")

	err = pollingHandler.StartProcessingLoop()
	if err != nil {
		return err
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	<-sigs

	log.Info("application closing, closing polling handler...")

	err = pollingHandler.Close()
	return err
}

func loadConfig(filepath string) (config.PriceNotifierConfig, error) {
	cfg := config.PriceNotifierConfig{}
	err := chainCore.LoadTomlFile(&cfg, filepath)
	if err != nil {
		return config.PriceNotifierConfig{}, err
	}

	return cfg, nil
}

func createPriceFetchers(httpReponseGetter aggregator.ResponseGetter) ([]aggregator.PriceFetcher, error) {
	exchanges := fetchers.ImplementedFetchers
	priceFetchers := make([]aggregator.PriceFetcher, 0, len(exchanges))

	for exchangeName := range exchanges {
		args := fetchers.ArgsPriceFetcher{
			FetcherName:    exchangeName,
			ResponseGetter: httpReponseGetter,
		}

		priceFetcher, err := fetchers.NewPriceFetcher(args)
		if err != nil {
			return nil, err
		}

		priceFetchers = append(priceFetchers, priceFetcher)
	}

	return priceFetchers, nil
}

func addPairToFetchers(argsPair aggregator.ArgsPair, priceFetchers []aggregator.PriceFetcher) {
	for _, fetcher := range priceFetchers {
		_, ok := argsPair.Exchanges[fetcher.Name()]
		if ok {
			fetcher.AddPair(argsPair.Base, argsPair.Quote)
		}
	}
}

func getMapFromSlice(exchangesSlice []string) map[string]struct{} {
	exchangesMap := make(map[string]struct{})
	for _, exchange := range exchangesSlice {
		exchangesMap[exchange] = struct{}{}
	}
	return exchangesMap
}

// TODO: EN-12835 extract this into core
func attachFileLogger(log logger.Logger, flagsConfig config.ContextFlagsConfig) (chainFactory.FileLoggingHandler, error) {
	var fileLogging chainFactory.FileLoggingHandler
	var err error
	if flagsConfig.SaveLogFile {
		args := file.ArgsFileLogging{
			WorkingDir:      flagsConfig.WorkingDir,
			DefaultLogsPath: defaultLogsPath,
			LogFilePrefix:   logFilePrefix,
		}
		fileLogging, err = file.NewFileLogging(args)
		if err != nil {
			return nil, fmt.Errorf("%w creating a log file", err)
		}
	}

	err = logger.SetDisplayByteSlice(logger.ToHex)
	log.LogIfError(err)
	logger.ToggleLoggerName(flagsConfig.EnableLogName)
	logLevelFlagValue := flagsConfig.LogLevel
	err = logger.SetLogLevel(logLevelFlagValue)
	if err != nil {
		return nil, err
	}

	if flagsConfig.DisableAnsiColor {
		err = logger.RemoveLogObserver(os.Stdout)
		if err != nil {
			return nil, err
		}

		err = logger.AddLogObserver(os.Stdout, &logger.PlainFormatter{})
		if err != nil {
			return nil, err
		}
	}
	log.Trace("logger updated", "level", logLevelFlagValue, "disable ANSI color", flagsConfig.DisableAnsiColor)

	return fileLogging, nil
}
