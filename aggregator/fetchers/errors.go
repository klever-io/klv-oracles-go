package fetchers

import "errors"

var (
	errInvalidResponseData     = errors.New("invalid response data")
	errInvalidFetcherName      = errors.New("invalid fetcher name")
	errNilResponseGetter       = errors.New("nil response getter")
	errInvalidPair             = errors.New("invalid pair")
	errInvalidGasPriceSelector = errors.New("invalid gas price selector")
)
