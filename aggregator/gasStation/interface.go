package gas

import "context"

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
