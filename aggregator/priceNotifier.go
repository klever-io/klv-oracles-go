package aggregator

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/multiversx/mx-chain-core-go/core/check"
)

const epsilon = 0.0001
const minAutoSendInterval = time.Second
const gweiTicker = "GWEI"
const ethTicker = "ETH"
const quoteUSD = "USD"
const weiNeg = 1e-9 // Gwei to ETH conversion factor

// ArgsPriceNotifier is the argument DTO for the price notifier
type ArgsPriceNotifier struct {
	Pairs            []*ArgsPair
	Aggregator       PriceAggregator
	GasPriceFetcher  PriceFetcher
	Notifee          PriceNotifee
	AutoSendInterval time.Duration
}

type priceInfo struct {
	price     float64
	timestamp int64
}

type notifyArgs struct {
	*pair
	newPrice          priceInfo
	lastNotifiedPrice float64
	index             int
}

type priceNotifier struct {
	mut                sync.Mutex
	priceAggregator    PriceAggregator
	gasPriceFetcher    PriceFetcher
	pairs              []*pair
	lastNotifiedPrices []float64
	notifee            PriceNotifee
	autoSendInterval   time.Duration
	lastTimeAutoSent   time.Time
	timeSinceHandler   func(t time.Time) time.Duration
	// Cache for token pairs needed for gas price calculation
	gasPricePairs map[string]int
	hasEthUsdPair bool
}

// NewPriceNotifier will create a new priceNotifier instance
func NewPriceNotifier(args ArgsPriceNotifier) (*priceNotifier, error) {
	err := checkArgsPriceNotifier(args)
	if err != nil {
		return nil, err
	}

	pairs := make([]*pair, 0)
	for idx, argsPair := range args.Pairs {
		if argsPair == nil {
			return nil, fmt.Errorf("%w, index %d", ErrNilArgsPair, idx)
		}
		pair, err := newPair(argsPair)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}

	priceNotifier := &priceNotifier{
		priceAggregator:    args.Aggregator,
		gasPriceFetcher:    args.GasPriceFetcher,
		pairs:              pairs,
		lastNotifiedPrices: make([]float64, len(args.Pairs)),
		notifee:            args.Notifee,
		autoSendInterval:   args.AutoSendInterval,
		lastTimeAutoSent:   time.Now(),
		timeSinceHandler:   time.Since,
		gasPricePairs:      make(map[string]int),
	}

	// Verify and initialize the gas price pairs cache
	err = priceNotifier.verifyGasPricePairs()
	if err != nil {
		return nil, err
	}

	return priceNotifier, nil
}

func checkArgsPriceNotifier(args ArgsPriceNotifier) error {
	if len(args.Pairs) < 1 {
		return ErrEmptyArgsPairsSlice
	}

	if args.AutoSendInterval < minAutoSendInterval {
		return fmt.Errorf("%w, minimum %v, got %v", ErrInvalidAutoSendInterval, minAutoSendInterval, args.AutoSendInterval)
	}
	if check.IfNil(args.Notifee) {
		return ErrNilPriceNotifee
	}
	if check.IfNil(args.Aggregator) {
		return ErrNilPriceAggregator
	}

	return nil
}

// Execute will trigger the price fetching and notification if the new price exceeded provided percentage change
func (pn *priceNotifier) Execute(ctx context.Context) error {
	fetchedPrices, err := pn.getAllPrices(ctx)
	if err != nil {
		return err
	}

	fetchedPrices, err = pn.convertGasPricesToDenomination(fetchedPrices)
	if err != nil {
		return err
	}

	notifyArgsSlice := pn.computeNotifyArgsSlice(fetchedPrices)

	return pn.notify(ctx, notifyArgsSlice)
}

func (pn *priceNotifier) convertGasPricesToDenomination(fetchedPrices []priceInfo) ([]priceInfo, error) {
	pn.mut.Lock()
	defer pn.mut.Unlock()

	if !pn.hasEthUsdPair {
		return nil, fmt.Errorf("ETH/USD pair not found, gas price calculation not possible")
	}

	// Find the ETH/USD price from the cached index
	ethPrice := 0.0
	for idx, pair := range pn.pairs {
		if pair.base == ethTicker && pair.quote == quoteUSD {
			ethPrice = fetchedPrices[idx].price
			break
		}
	}

	if ethPrice == 0.0 {
		return nil, fmt.Errorf("ETH/USD price is zero, gas price calculation not possible")
	}

	// Find token/USD prices for all tokens that need gas price denominated
	targetTokensIndex := make(map[string]int)
	for idx, pair := range pn.pairs {
		if pair.quote == quoteUSD && pair.base != ethTicker && pair.base != gweiTicker {
			targetTokensIndex[pair.base] = idx
		}
	}

	// Process each GWEI pair using the cached indices from gasPricePairs
	for quote, idx := range pn.gasPricePairs {
		// Gas price received from the gas station, in GWEI format
		gasPrice := fetchedPrices[idx].price

		gweiAsEth := gasPrice * weiNeg
		nominalValue := ethPrice * gweiAsEth

		// For GWEI/USD just use the nominal value directly
		if quote == quoteUSD {
			fetchedPrices[idx].price = nominalValue
			continue
		}

		// For other tokens, divide by their USD price
		targetIndex, ok := targetTokensIndex[quote]
		if !ok {
			return nil, fmt.Errorf("missing required %s/%s pair for gas price calculation", quote, quoteUSD)
		}

		fetchedPrices[idx].price = nominalValue / fetchedPrices[targetIndex].price
	}

	return fetchedPrices, nil
}

func (pn *priceNotifier) getAllPrices(ctx context.Context) ([]priceInfo, error) {
	fetchedPrices := make([]priceInfo, len(pn.pairs))
	for idx, pair := range pn.pairs {
		var price float64
		var err error
		// If the pair is a gas price ticker, we need to fetch the gas price
		if pair.base == gweiTicker {
			price, err = pn.gasPriceFetcher.FetchPrice(ctx, pair.base, pair.quote)
		} else {
			price, err = pn.priceAggregator.FetchPrice(ctx, pair.base, pair.quote)
		}

		if err != nil {
			return nil, fmt.Errorf("%w while querying the pair %s-%s", err, pair.base, pair.quote)
		}

		fetchedPrice := priceInfo{
			price:     trim(price, pair.trimPrecision),
			timestamp: time.Now().Unix(),
		}
		fetchedPrices[idx] = fetchedPrice
	}

	return fetchedPrices, nil
}

func (pn *priceNotifier) computeNotifyArgsSlice(fetchedPrices []priceInfo) []*notifyArgs {
	pn.mut.Lock()
	defer pn.mut.Unlock()

	shouldNotifyAll := pn.timeSinceHandler(pn.lastTimeAutoSent) > pn.autoSendInterval

	result := make([]*notifyArgs, 0, len(pn.pairs))
	for idx, pair := range pn.pairs {
		notifyArgsValue := &notifyArgs{
			pair:              pair,
			newPrice:          fetchedPrices[idx],
			lastNotifiedPrice: pn.lastNotifiedPrices[idx],
			index:             idx,
		}

		if shouldNotifyAll || shouldNotify(notifyArgsValue) {
			result = append(result, notifyArgsValue)
		}
	}

	if shouldNotifyAll {
		pn.lastTimeAutoSent = time.Now()
	}

	return result
}

func shouldNotify(notifyArgsValue *notifyArgs) bool {
	percentValue := float64(notifyArgsValue.percentDifferenceToNotify) / 100
	shouldBypassPercentCheck := notifyArgsValue.lastNotifiedPrice < epsilon || percentValue < epsilon
	if shouldBypassPercentCheck {
		return true
	}

	absoluteChange := math.Abs(notifyArgsValue.lastNotifiedPrice - notifyArgsValue.newPrice.price)
	percentageChange := absoluteChange * 100 / notifyArgsValue.lastNotifiedPrice

	return percentageChange >= float64(notifyArgsValue.percentDifferenceToNotify)
}

func (pn *priceNotifier) notify(ctx context.Context, notifyArgsSlice []*notifyArgs) error {
	if len(notifyArgsSlice) == 0 {
		return nil
	}

	args := make([]*ArgsPriceChanged, 0, len(notifyArgsSlice))
	for _, notify := range notifyArgsSlice {
		priceTrimmed := trim(notify.newPrice.price, notify.trimPrecision)
		denominatedPrice := uint64(priceTrimmed * float64(notify.denominationFactor))

		argPriceChanged := &ArgsPriceChanged{
			Base:             notify.base,
			Quote:            notify.quote,
			DenominatedPrice: denominatedPrice,
			Decimals:         notify.decimals,
			Timestamp:        notify.newPrice.timestamp,
		}

		args = append(args, argPriceChanged)

		pn.mut.Lock()
		pn.lastNotifiedPrices[notify.index] = priceTrimmed
		pn.mut.Unlock()
	}

	return pn.notifee.PriceChanged(ctx, args)
}

// verifyGasPricePairs checks if we have all the necessary token pairs for gas price calculation
// and initializes the gas price pairs cache
func (pn *priceNotifier) verifyGasPricePairs() error {
	// Check if ETH/USD pair exists
	hasEthUsd := false
	// Create maps for token/USD pairs that are used for gas price calculation in different tokens
	gweiPairsMap := make(map[string]bool)
	tokenUsdPairsMap := make(map[string]bool)

	for idx, pair := range pn.pairs {
		// Check for ETH/USD pair which is required for all gas price calculations
		if pair.base == ethTicker && pair.quote == quoteUSD {
			hasEthUsd = true
			pn.hasEthUsdPair = true
		}

		// Store indices of GWEI pairs
		if pair.base == gweiTicker {
			gweiPairsMap[pair.quote] = true
			pn.gasPricePairs[pair.quote] = idx
		}

		// Store USD quoted pairs for target tokens
		if pair.quote == quoteUSD && pair.base != ethTicker && pair.base != gweiTicker {
			tokenUsdPairsMap[pair.base] = true
		}
	}

	// Return error if ETH/USD pair is missing
	if !hasEthUsd {
		return fmt.Errorf("ETH/USD pair not found, required for gas price calculation")
	}

	// Check for each GWEI/Token pair if we have corresponding Token/USD pairs
	missingTokenUsdPairs := make([]string, 0)
	for quote := range gweiPairsMap {
		if quote != quoteUSD && !tokenUsdPairsMap[quote] {
			missingPair := fmt.Sprintf("%s/%s", quote, quoteUSD)
			log.Error("missing required token/USD pair for gas price calculation",
				"token", quote,
				"required_pair", missingPair)
			missingTokenUsdPairs = append(missingTokenUsdPairs, missingPair)
		}
	}

	if len(missingTokenUsdPairs) > 0 {
		return fmt.Errorf("missing required token/USD pairs for gas price calculation: %v", missingTokenUsdPairs)
	}

	return nil
}

// IsInterfaceNil returns true if there is no value under the interface
func (pn *priceNotifier) IsInterfaceNil() bool {
	return pn == nil
}
