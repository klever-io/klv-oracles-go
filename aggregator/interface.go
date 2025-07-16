package aggregator

import (
	"context"

	gas "github.com/klever-io/klv-oracles-go/aggregator/gasStation"
)

// ResponseGetter is the component able to execute a get operation on the provided URL
type ResponseGetter interface {
	Get(ctx context.Context, url string, response interface{}) error
}

// GraphqlGetter is the graphql component able to execute a get operation on the provided URL
type GraphqlGetter interface {
	Query(ctx context.Context, url string, query string, variables string) ([]byte, error)
}

// basePriceFetcher defines the behavior of a component able to query the price
type basePriceFetcher interface {
	Name() string
	FetchPrice(ctx context.Context, base string, quote string) (float64, error)
	IsInterfaceNil() bool
}

// PriceAggregator defines the behavior of a component able to query the median price of a provided pair
// from all the fetchers that has the pair
type PriceAggregator interface {
	basePriceFetcher
}

// PriceFetcher defines the behavior of a component able to query the price for the provided pairs
type PriceFetcher interface {
	basePriceFetcher
	AddPair(base, quote string)
}

// ArgsPriceChanged is the argument used when notifying the notifee instance
type ArgsPriceChanged struct {
	Base             string
	Quote            string
	DenominatedPrice uint64
	Decimals         uint64
	Timestamp        int64
}

// PriceNotifee defines the behavior of a component able to be notified over a price change
type PriceNotifee interface {
	PriceChanged(ctx context.Context, priceChanges []*ArgsPriceChanged) error
	IsInterfaceNil() bool
}

// GasPriceService handles all gas price related conversions and operations
type GasPriceService interface {
	// ConvertGasPrices converts gas prices in GWEI to various denominations
	ConvertGasPrices(ctx context.Context, pairs []gas.ArgsPairInfo) ([]gas.ArgsPairInfo, error)
	// VerifyRequiredPairs checks if all required pairs for gas price calculation are available
	VerifyRequiredPairs(pairs []gas.ArgsPairInfo) error
	// IsInterfaceNil returns true if there is no value under the interface
	IsInterfaceNil() bool
}
