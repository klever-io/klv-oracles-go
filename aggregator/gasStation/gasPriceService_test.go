package gas_test

import (
	"context"
	"testing"

	gas "github.com/klever-io/klv-oracles-go/aggregator/gasStation"
	"github.com/klever-io/klv-oracles-go/aggregator/mock"
	"github.com/multiversx/mx-chain-core-go/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGasPriceService_NewGasPriceService(t *testing.T) {
	t.Parallel()

	t.Run("nil gasPriceFetcher should error", func(t *testing.T) {
		t.Parallel()

		gps, err := gas.NewGasPriceService(gas.ArgsGasPriceService{})

		assert.Nil(t, gps)
		assert.Equal(t, gas.ErrNilGasPriceFetcher, err)
	})

	t.Run("valid setup should work", func(t *testing.T) {
		t.Parallel()

		gasPriceFetcher := &mock.PriceFetcherStub{}
		gps, err := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: gasPriceFetcher,
		})

		assert.False(t, check.IfNil(gps))
		assert.Nil(t, err)
	})
}

func TestGasPriceService_ConvertGasPrices(t *testing.T) {
	t.Parallel()

	t.Run("should fail, missing BTC/USD pair", func(t *testing.T) {
		t.Parallel()

		fetcher := &mock.PriceFetcherStub{}

		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		fetchedPrices := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 2000.0, Timestamp: 123}, // ETH/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},   // GWEI/USD Price (will be converted)
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},   // GWEI/BTC Price (will be converted)
		}

		result, err := gps.ConvertGasPrices(context.Background(), fetchedPrices)
		assert.Nil(t, result)
		assert.NotNil(t, err)
		assert.ErrorIs(t, err, gas.ErrMissingPairs)
	})

	t.Run("zero ETH Price should error", func(t *testing.T) {
		t.Parallel()

		fetcher := &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 30.0, nil
			},
		}
		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		fetchedPrices := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 0.0, Timestamp: 123},     // ETH/USD Price is zero
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},    // GWEI/USD Price
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},    // GWEI/BTC Price
		}

		result, err := gps.ConvertGasPrices(context.Background(), fetchedPrices)

		assert.NotNil(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, gas.ErrEthUsdPriceZero)
	})

	t.Run("fetcher error should fail", func(t *testing.T) {
		t.Parallel()

		fetcher := &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 0, assert.AnError
			},
		}
		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		fetchedPrices := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 2000.0, Timestamp: 123},  // ETH/USD Price
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},    // GWEI/USD Price
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},    // GWEI/BTC Price
		}

		result, err := gps.ConvertGasPrices(context.Background(), fetchedPrices)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("no conversion needed, no gas pair found", func(t *testing.T) {
		t.Parallel()

		fetcherCalled := false
		fetcher := &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				fetcherCalled = true
				return 30.0, nil
			},
		}
		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		fetchedPrices := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 2000.0, Timestamp: 123},  // ETH/USD Price
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
		}

		result, err := gps.ConvertGasPrices(context.Background(), fetchedPrices)
		assert.Nil(t, err)
		assert.False(t, fetcherCalled, "fetcher should not be called when no gas pair is found")

		require.Equal(t, 2, len(result))
		// should return the same prices as no conversion is needed
		assert.Equal(t, fetchedPrices[0].Price, result[0].Price)
		assert.Equal(t, fetchedPrices[1].Price, result[1].Price)
	})

	t.Run("conversion successful", func(t *testing.T) {
		t.Parallel()

		fetcher := &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 30.0, nil
			},
		}
		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		fetchedPrices := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 2000.0, Timestamp: 123},  // ETH/USD Price
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},    // GWEI/USD Price (will be converted)
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},    // GWEI/BTC Price (will be converted)
		}

		result, err := gps.ConvertGasPrices(context.Background(), fetchedPrices)
		require.Nil(t, err)
		require.Equal(t, 4, len(result))
		// GWEI/USD calculated value = 30 * 1e-9 * 2000 = 0.00006
		assert.InDelta(t, 0.00006, result[2].Price, 0.000001)
		// GWEI/BTC calculated value = 30 * 1e-9 * 2000 / 40000 = 0.0000000015
		assert.InDelta(t, 0.0000000015, result[3].Price, 0.0000000001)
	})
}

func TestGasPriceService_VerifyRequiredPairs(t *testing.T) {
	t.Parallel()

	t.Run("should fail, no gas pair found", func(t *testing.T) {
		t.Parallel()

		pairs := []gas.ArgsPairInfo{
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
		}

		fetcher := &mock.PriceFetcherStub{}

		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		err := gps.VerifyRequiredPairs(pairs)
		assert.ErrorIs(t, err, gas.ErrNoGasPairs)
	})

	t.Run("should fail, missing BTC/USD pair", func(t *testing.T) {
		t.Parallel()

		pairs := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 2000.0, Timestamp: 123}, // ETH/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},   // GWEI/USD Price (will be converted)
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},   // GWEI/BTC Price (will be converted)
		}

		fetcher := &mock.PriceFetcherStub{}

		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		err := gps.VerifyRequiredPairs(pairs)

		assert.NotNil(t, err)
		assert.ErrorIs(t, err, gas.ErrMissingPairs)
	})

	t.Run("should fail, missing ETH/USD pair", func(t *testing.T) {
		t.Parallel()

		pairs := []gas.ArgsPairInfo{
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},    // GWEI/USD Price (will be converted)
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},    // GWEI/BTC Price (will be converted)
		}

		fetcher := &mock.PriceFetcherStub{}

		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		err := gps.VerifyRequiredPairs(pairs)

		assert.NotNil(t, err)
		assert.ErrorIs(t, err, gas.ErrMissingPairs)
	})

	t.Run("should pass, all required pairs present", func(t *testing.T) {
		t.Parallel()

		pairs := []gas.ArgsPairInfo{
			{Base: "ETH", Quote: "USD", Price: 2000.0, Timestamp: 123},  // ETH/USD Price
			{Base: "BTC", Quote: "USD", Price: 40000.0, Timestamp: 123}, // BTC/USD Price
			{Base: "GWEI", Quote: "USD", Price: 0.0, Timestamp: 123},    // GWEI/USD Price (will be converted)
			{Base: "GWEI", Quote: "BTC", Price: 0.0, Timestamp: 123},    // GWEI/BTC Price (will be converted)
		}

		fetcher := &mock.PriceFetcherStub{}

		gps, _ := gas.NewGasPriceService(gas.ArgsGasPriceService{
			GasPriceFetcher: fetcher,
		})

		err := gps.VerifyRequiredPairs(pairs)

		assert.Nil(t, err)
	})
}
