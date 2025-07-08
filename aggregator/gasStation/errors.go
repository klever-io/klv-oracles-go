package gas

import "errors"

var (
	// ErrNilGasPriceFetcher signals that a nil gas price fetcher was provided
	ErrNilGasPriceFetcher = errors.New("nil gas price fetcher")
	// ErrMismatchFetchedPricesLen signals that there is a mismatch between the pairs and fetched prices length
	ErrMismatchFetchedPricesLen = errors.New("mismatch between pairs and fetched prices length")
	// ErrEthUsdPriceZero signals that the ETH/USD price is zero, making gas price calculation impossible
	ErrEthUsdPriceZero = errors.New("ETH/USD price is zero, gas price calculation not possible")
	// ErrMissingPairs signals that the pairs required for gas price conversion are missing
	ErrMissingPairs = errors.New("missing pairs for gas price conversion")
	// ErrNoGweiPairs signals that no Gas pairs were found for gas price calculation
	ErrNoGasPairs = errors.New("no Gas pairs found for gas price calculation")
)
