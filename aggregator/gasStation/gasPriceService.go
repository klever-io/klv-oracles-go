package gas

import (
	"context"
	"fmt"

	"github.com/multiversx/mx-chain-core-go/core/check"
)

const gweiTicker = "GWEI"
const ethTicker = "ETH"
const quoteUSD = "USD"
const weiNeg = 1e-9 // Gwei to ETH conversion factor

type ArgsPairInfo struct {
	Base      string  // Base currency ticker
	Quote     string  // Quote currency ticker
	Price     float64 // Price of the pair
	Timestamp int64   // Timestamp of the price
}

// ArgsGasPriceService is the DTO used to create a new GasPriceService
type ArgsGasPriceService struct {
	GasPriceFetcher PriceFetcher // Fetcher for gas prices
}

// TODO: add gas ticker, Base chain currency ticker
type gasPriceService struct {
	gasPriceFetcher PriceFetcher
}

// NewGasPriceService creates a new instance of the gas price service
func NewGasPriceService(args ArgsGasPriceService) (*gasPriceService, error) {
	if err := checkArgsGasPriceService(args); err != nil {
		return nil, err
	}

	return &gasPriceService{
		gasPriceFetcher: args.GasPriceFetcher,
	}, nil
}

func checkArgsGasPriceService(args ArgsGasPriceService) error {
	if check.IfNil(args.GasPriceFetcher) {
		return ErrNilGasPriceFetcher
	}

	return nil
}

// ConvertGasPrices converts gas prices in GWEI to various denominations
func (gps *gasPriceService) ConvertGasPrices(ctx context.Context, pairs []ArgsPairInfo) ([]ArgsPairInfo, error) {
	err := gps.VerifyRequiredPairs(pairs)
	if err == ErrNoGasPairs {
		// If no gas pairs are found, return the pairs as is
		return pairs, nil
	}

	if err != nil {
		return nil, err
	}

	// Find the ETH/USD price from the cached index
	ethPrice := 0.0
	for _, pair := range pairs {
		if pair.Base == ethTicker && pair.Quote == quoteUSD {
			ethPrice = pair.Price
			break
		}
	}

	if ethPrice == 0.0 {
		return nil, ErrEthUsdPriceZero
	}

	// Fetch the actual GWEI value using the gas price fetcher
	gasPrice, err := gps.gasPriceFetcher.FetchPrice(ctx, gweiTicker, quoteUSD)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch gas price in GWEI: %w", err)
	}

	// Find token/USD prices for all tokens that need gas price denominated
	targetTokensIndex := make(map[string]int)
	for idx, pair := range pairs {
		if pair.Quote == quoteUSD && pair.Base != ethTicker && pair.Base != gweiTicker {
			targetTokensIndex[pair.Base] = idx
		}
	}

	// Create a copy of fetchedPrices to avoid modifying the original
	result := make([]ArgsPairInfo, len(pairs))
	copy(result, pairs)

	// Process each GWEI pair using the cached indices from gasPricePairs
	for idx, pair := range result {
		if pair.Base != gweiTicker {
			continue
		}

		gweiAsEth := gasPrice * weiNeg
		nominalValue := ethPrice * gweiAsEth

		// For GWEI/USD just use the nominal value directly
		if pair.Quote == quoteUSD {
			result[idx].Price = nominalValue
			continue
		}

		// For other tokens, divide by their USD price
		targetIndex, ok := targetTokensIndex[pair.Quote]
		if !ok {
			return nil, fmt.Errorf("%w: %s/%s pair for gas price calculation", ErrMissingPairs, pair.Quote, quoteUSD)
		}

		result[idx].Price = nominalValue / result[targetIndex].Price
	}

	return result, nil
}

// VerifyRequiredPairs checks if all required pairs for gas price calculation are available
func (gps *gasPriceService) VerifyRequiredPairs(pairs []ArgsPairInfo) error {
	// Check if ETH/USD pair exists
	hasEthUsd := false
	// Create maps for token/USD pairs that are used for gas price calculation in different tokens
	gweiPairsMap := make(map[string]bool)
	tokenUsdPairsMap := make(map[string]bool)

	for _, pair := range pairs {
		// Check for ETH/USD pair which is required for all gas price calculations
		if pair.Base == ethTicker && pair.Quote == quoteUSD {
			hasEthUsd = true
		}

		// Store indices of GWEI pairs
		if pair.Base == gweiTicker {
			gweiPairsMap[pair.Quote] = true
		}

		if pair.Quote != quoteUSD {
			continue
		}
		// Store USD quoted pairs for target tokens
		tokenUsdPairsMap[pair.Base] = true
	}

	if len(gweiPairsMap) == 0 {
		return ErrNoGasPairs
	}

	// Return error if ETH/USD pair is missing
	if !hasEthUsd {
		return fmt.Errorf("%w: %s/%s pair for gas price calculation", ErrMissingPairs, ethTicker, quoteUSD)
	}

	// Check for each GWEI/Token pair if we have corresponding Token/USD pairs
	missingTokenUsdPairs := make([]string, 0)
	for Quote := range gweiPairsMap {
		if Quote == quoteUSD || tokenUsdPairsMap[Quote] {
			continue
		}

		missingTokenUsdPairs = append(missingTokenUsdPairs, fmt.Sprintf("%s/%s", Quote, quoteUSD))
	}

	if len(missingTokenUsdPairs) > 0 {
		return fmt.Errorf("%w: token/USD pairs for gas price calculation: %v", ErrMissingPairs, missingTokenUsdPairs)
	}

	return nil
}

// IsInterfaceNil returns true if there is no value under the interface
func (gps *gasPriceService) IsInterfaceNil() bool {
	return gps == nil
}
