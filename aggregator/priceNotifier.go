package aggregator

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	gas "github.com/klever-io/klv-oracles-go/aggregator/gasStation"
	"github.com/multiversx/mx-chain-core-go/core/check"
)

const epsilon = 0.0001
const minAutoSendInterval = time.Second
const gweiTicker = "GWEI"

// ArgsPriceNotifier is the argument DTO for the price notifier
type ArgsPriceNotifier struct {
	Pairs            []*ArgsPair
	Aggregator       PriceAggregator
	GasPriceService  GasPriceService
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
	gasPriceService    GasPriceService
	pairs              []*pair
	lastNotifiedPrices []float64
	notifee            PriceNotifee
	autoSendInterval   time.Duration
	lastTimeAutoSent   time.Time
	timeSinceHandler   func(t time.Time) time.Duration
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
		gasPriceService:    args.GasPriceService,
		pairs:              pairs,
		lastNotifiedPrices: make([]float64, len(args.Pairs)),
		notifee:            args.Notifee,
		autoSendInterval:   args.AutoSendInterval,
		lastTimeAutoSent:   time.Now(),
		timeSinceHandler:   time.Since,
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
	if check.IfNil(args.GasPriceService) {
		return ErrNilGasPriceService
	}

	return nil
}

// Execute will trigger the price fetching and notification if the new price exceeded provided percentage change
func (pn *priceNotifier) Execute(ctx context.Context) error {
	fetchedPrices, err := pn.getAllPrices(ctx)
	if err != nil {
		return err
	}

	fetchedPrices, err = pn.denominateGasPrice(ctx, fetchedPrices)
	if err != nil {
		return err
	}

	notifyArgsSlice := pn.computeNotifyArgsSlice(fetchedPrices)

	return pn.notify(ctx, notifyArgsSlice)
}

func (pn *priceNotifier) getAllPrices(ctx context.Context) ([]priceInfo, error) {
	fetchedPrices := make([]priceInfo, len(pn.pairs))
	for idx, pair := range pn.pairs {
		var price float64
		var err error
		// If the pair is a gas price ticker, we need to fetch the gas price
		if pair.base != gweiTicker {
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

func (pn *priceNotifier) denominateGasPrice(ctx context.Context, fetchedPrices []priceInfo) ([]priceInfo, error) {
	args := make([]gas.ArgsPairInfo, 0, len(pn.pairs))
	for idx, pair := range pn.pairs {
		args = append(args, gas.ArgsPairInfo{
			Base:      pair.base,
			Quote:     pair.quote,
			Price:     fetchedPrices[idx].price,
			Timestamp: fetchedPrices[idx].timestamp,
		})
	}

	gasPricesInfo, err := pn.gasPriceService.ConvertGasPrices(ctx, args)
	if err != nil {
		return nil, err
	}

	result := make([]priceInfo, len(fetchedPrices))
	copy(result, fetchedPrices)

	for _, gasPrice := range gasPricesInfo {
		for idx, pair := range pn.pairs {
			if pair.base != gasPrice.Base || pair.quote != gasPrice.Quote {
				continue
			}

			result[idx].price = gasPrice.Price
			result[idx].timestamp = gasPrice.Timestamp
		}
	}

	return result, nil
}

// IsInterfaceNil returns true if there is no value under the interface
func (pn *priceNotifier) IsInterfaceNil() bool {
	return pn == nil
}
