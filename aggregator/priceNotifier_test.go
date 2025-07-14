package aggregator_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/klever-io/klv-oracles-go/aggregator"
	gas "github.com/klever-io/klv-oracles-go/aggregator/gasStation"
	"github.com/klever-io/klv-oracles-go/aggregator/mock"
	"github.com/multiversx/mx-chain-core-go/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createMockArgsPriceNotifier() aggregator.ArgsPriceNotifier {
	return aggregator.ArgsPriceNotifier{
		Pairs: []*aggregator.ArgsPair{
			{
				Base:                      "BASE",
				Quote:                     "QUOTE",
				PercentDifferenceToNotify: 1,
				Decimals:                  2,
				Exchanges:                 map[string]struct{}{"Binance": {}},
			},
		},
		Aggregator:       &mock.PriceFetcherStub{},
		Notifee:          &mock.PriceNotifeeStub{},
		GasPriceService:  &mock.GasPriceServiceStub{},
		AutoSendInterval: time.Minute,
	}
}

func TestNewPriceNotifier(t *testing.T) {
	t.Parallel()

	t.Run("empty pair arguments slice should error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Pairs = nil

		pn, err := aggregator.NewPriceNotifier(args)
		assert.True(t, check.IfNil(pn))
		assert.Equal(t, aggregator.ErrEmptyArgsPairsSlice, err)
	})
	t.Run("nil pair argument should error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Pairs = append(args.Pairs, nil)

		pn, err := aggregator.NewPriceNotifier(args)
		assert.True(t, check.IfNil(pn))
		assert.True(t, errors.Is(err, aggregator.ErrNilArgsPair))
		assert.True(t, strings.Contains(err.Error(), "index 1"))
	})
	t.Run("invalid auto send interval", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.AutoSendInterval = time.Second - time.Nanosecond

		pn, err := aggregator.NewPriceNotifier(args)
		assert.True(t, check.IfNil(pn))
		assert.True(t, errors.Is(err, aggregator.ErrInvalidAutoSendInterval))
	})
	t.Run("nil notifee", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Notifee = nil

		pn, err := aggregator.NewPriceNotifier(args)
		assert.True(t, check.IfNil(pn))
		assert.Equal(t, aggregator.ErrNilPriceNotifee, err)
	})
	t.Run("nil aggregator", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Aggregator = nil

		pn, err := aggregator.NewPriceNotifier(args)
		assert.True(t, check.IfNil(pn))
		assert.Equal(t, aggregator.ErrNilPriceAggregator, err)
	})
	t.Run("nil gas service", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.GasPriceService = nil

		pn, err := aggregator.NewPriceNotifier(args)
		assert.True(t, check.IfNil(pn))
		assert.Equal(t, aggregator.ErrNilGasPriceService, err)
	})
	t.Run("should work", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()

		pn, err := aggregator.NewPriceNotifier(args)
		assert.False(t, check.IfNil(pn))
		assert.Nil(t, err)
	})
	t.Run("should work with 0 percentage to notify", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Pairs[0].PercentDifferenceToNotify = 0

		pn, err := aggregator.NewPriceNotifier(args)
		assert.False(t, check.IfNil(pn))
		assert.Nil(t, err)
	})
}

func TestPriceNotifier_Execute(t *testing.T) {
	t.Parallel()

	t.Run("price fetch errors should error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("expected error")
		args := createMockArgsPriceNotifier()
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 0, expectedErr
			},
		}
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				assert.Fail(t, "should have not called notifee.PriceChanged")
				return nil
			},
		}

		pn, _ := aggregator.NewPriceNotifier(args)
		err := pn.Execute(context.Background())
		assert.True(t, errors.Is(err, expectedErr))
	})
	t.Run("first time should notify", func(t *testing.T) {
		t.Parallel()

		var startTimestamp, endTimestamp, receivedTimestamp int64
		args := createMockArgsPriceNotifier()
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 1.987654321, nil
			},
		}
		wasCalled := false
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Equal(t, 1, len(args))
				for _, arg := range args {
					assert.Equal(t, arg.Base, "BASE")
					assert.Equal(t, arg.Quote, "QUOTE")
					assert.Equal(t, uint64(199), arg.DenominatedPrice)
					assert.Equal(t, uint64(2), arg.Decimals)
					receivedTimestamp = arg.Timestamp
				}
				wasCalled = true

				return nil
			},
		}

		pn, _ := aggregator.NewPriceNotifier(args)
		startTimestamp = time.Now().Unix()
		err := pn.Execute(context.Background())
		endTimestamp = time.Now().Unix()
		assert.Nil(t, err)
		assert.True(t, wasCalled)
		assert.True(t, startTimestamp <= receivedTimestamp)
		assert.True(t, endTimestamp >= receivedTimestamp)
	})
	t.Run("double call should notify once", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 1.987654321, nil
			},
		}
		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Equal(t, 1, len(args))
				for _, arg := range args {
					assert.Equal(t, arg.Base, "BASE")
					assert.Equal(t, arg.Quote, "QUOTE")
					assert.Equal(t, uint64(199), arg.DenominatedPrice)
					assert.Equal(t, uint64(2), arg.Decimals)
				}
				numCalled++

				return nil
			},
		}

		pn, _ := aggregator.NewPriceNotifier(args)
		err := pn.Execute(context.Background())
		assert.Nil(t, err)

		err = pn.Execute(context.Background())
		assert.Nil(t, err)

		assert.Equal(t, 1, numCalled)
	})
	t.Run("double call should notify twice if the percentage value is 0", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Pairs[0].PercentDifferenceToNotify = 0
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 1.987654321, nil
			},
		}
		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Equal(t, 1, len(args))
				for _, arg := range args {
					assert.Equal(t, arg.Base, "BASE")
					assert.Equal(t, arg.Quote, "QUOTE")
					assert.Equal(t, uint64(199), arg.DenominatedPrice)
					assert.Equal(t, uint64(2), arg.Decimals)
				}
				numCalled++

				return nil
			},
		}

		pn, err := aggregator.NewPriceNotifier(args)
		require.Nil(t, err)

		err = pn.Execute(context.Background())
		assert.Nil(t, err)

		err = pn.Execute(context.Background())
		assert.Nil(t, err)

		assert.Equal(t, 2, numCalled)
	})
	t.Run("no price changes should not notify", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		args.Pairs[0].PercentDifferenceToNotify = 1
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 1.987654321, nil
			},
		}
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Fail(t, "should have not called pricesChanged")

				return nil
			},
		}

		pn, err := aggregator.NewPriceNotifier(args)
		require.Nil(t, err)
		pn.SetLastNotifiedPrices([]float64{1.987654321})

		err = pn.Execute(context.Background())
		assert.Nil(t, err)
	})
	t.Run("no price changes but auto send duration exceeded", func(t *testing.T) {
		t.Parallel()

		startTime := time.Now()

		args := createMockArgsPriceNotifier()
		args.Pairs[0].PercentDifferenceToNotify = 1
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return 1.987654321, nil
			},
		}
		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				numCalled++
				return nil
			},
		}

		time.Sleep(time.Second)
		pn, err := aggregator.NewPriceNotifier(args)
		require.Nil(t, err)
		pn.SetLastNotifiedPrices([]float64{1.987654321})

		lastTimeAutoSent := pn.LastTimeAutoSent()
		assert.True(t, lastTimeAutoSent.Sub(startTime) > 0)

		pn.SetTimeSinceHandler(func(providedTime time.Time) time.Duration {
			assert.Equal(t, pn.LastTimeAutoSent(), providedTime)

			return time.Second * time.Duration(10000)
		})

		time.Sleep(time.Second)

		err = pn.Execute(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, 1, numCalled)
		assert.True(t, pn.LastTimeAutoSent().Sub(lastTimeAutoSent) > 0)
	})
	t.Run("price changed over the limit should notify twice", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		price := 1.987654321
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				price = price * 1.012 // due to rounding errors, we need this slightly higher increase

				fmt.Printf("new price: %v\n", price)

				return price, nil
			},
		}
		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Equal(t, 1, len(args))
				for _, arg := range args {
					assert.Equal(t, arg.Base, "BASE")
					assert.Equal(t, arg.Quote, "QUOTE")
				}
				numCalled++

				return nil
			},
		}

		pn, _ := aggregator.NewPriceNotifier(args)
		err := pn.Execute(context.Background())
		assert.Nil(t, err)

		err = pn.Execute(context.Background())
		assert.Nil(t, err)

		assert.Equal(t, 2, numCalled)
	})

	t.Run("gas token pair should denominated gwei price", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		price := 1.987654321
		conversionFactor := 0.000000001 // Assuming 1 GWEI = 0.000000001 ETH
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return price, nil
			},
		}
		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Equal(t, 2, len(args))
				assert.Equal(t, "GWEI", args[1].Base)
				assert.Equal(t, "QUOTE", args[1].Quote)
				assert.Equal(t, uint64(1988), args[1].DenominatedPrice)
				numCalled++

				return nil
			},
		}

		args.GasPriceService = &mock.GasPriceServiceStub{
			ConvertGasPricesCalled: func(ctx context.Context, pairs []gas.ArgsPairInfo) ([]gas.ArgsPairInfo, error) {
				// Simulate the conversion of GWEI to QUOTE token
				for i, pair := range pairs {
					if pair.Base == "GWEI" && pair.Quote == "QUOTE" {
						pairs[i].Price = conversionFactor * price // Assuming 1 GWEI = 0.000000001 ETH
						pairs[i].Timestamp = time.Now().Unix()
					}
				}
				return pairs, nil
			},
		}

		args.Pairs = append(args.Pairs, &aggregator.ArgsPair{
			Base:                      "GWEI",
			Quote:                     "QUOTE",
			PercentDifferenceToNotify: 1,
			Decimals:                  12,
			Exchanges:                 map[string]struct{}{"Binance": {}},
		})

		pn, _ := aggregator.NewPriceNotifier(args)
		err := pn.Execute(context.Background())
		assert.Nil(t, err)

		assert.Equal(t, 1, numCalled)
	})

	t.Run("multiple gas token pair should denominated gwei price for each", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		price := 1.987654321
		conversionFactor := 1e-9 // Assuming 1 GWEI = 0.000000001 ETH
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return price, nil
			},
		}
		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				require.Equal(t, 3, len(args))
				assert.Equal(t, "GWEI", args[1].Base)
				assert.Equal(t, "QUOTE2", args[1].Quote)
				assert.Equal(t, uint64(3975), args[1].DenominatedPrice)

				assert.Equal(t, "GWEI", args[2].Base)
				assert.Equal(t, "QUOTE", args[2].Quote)
				assert.Equal(t, uint64(1988), args[2].DenominatedPrice)
				numCalled++

				return nil
			},
		}

		args.GasPriceService = &mock.GasPriceServiceStub{
			ConvertGasPricesCalled: func(ctx context.Context, pairs []gas.ArgsPairInfo) ([]gas.ArgsPairInfo, error) {
				// Simulate the conversion of GWEI to QUOTE token
				for i, pair := range pairs {
					if pair.Base == "GWEI" && pair.Quote == "QUOTE" {
						pairs[i].Price = conversionFactor * price
						pairs[i].Timestamp = time.Now().Unix()
					}

					if pair.Base == "GWEI" && pair.Quote == "QUOTE2" {
						pairs[i].Price = conversionFactor * price * 2
						pairs[i].Timestamp = time.Now().Unix()
					}
				}
				return pairs, nil
			},
		}

		args.Pairs = append(args.Pairs, &aggregator.ArgsPair{
			Base:                      "GWEI",
			Quote:                     "QUOTE2",
			PercentDifferenceToNotify: 1,
			Decimals:                  12,
			Exchanges:                 map[string]struct{}{"Binance": {}},
		})

		args.Pairs = append(args.Pairs, &aggregator.ArgsPair{
			Base:                      "GWEI",
			Quote:                     "QUOTE",
			PercentDifferenceToNotify: 1,
			Decimals:                  12,
			Exchanges:                 map[string]struct{}{"Binance": {}},
		})

		pn, _ := aggregator.NewPriceNotifier(args)
		err := pn.Execute(context.Background())
		assert.Nil(t, err)

		assert.Equal(t, 1, numCalled)
	})

	t.Run("should fail, gas service returns error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsPriceNotifier()
		price := 1.987654321
		args.Aggregator = &mock.PriceFetcherStub{
			FetchPriceCalled: func(ctx context.Context, base string, quote string) (float64, error) {
				return price, nil
			},
		}

		numCalled := 0
		args.Notifee = &mock.PriceNotifeeStub{
			PriceChangedCalled: func(ctx context.Context, args []*aggregator.ArgsPriceChanged) error {
				numCalled++

				return nil
			},
		}

		args.GasPriceService = &mock.GasPriceServiceStub{
			ConvertGasPricesCalled: func(ctx context.Context, pairs []gas.ArgsPairInfo) ([]gas.ArgsPairInfo, error) {
				return nil, assert.AnError
			},
		}

		args.Pairs = append(args.Pairs, &aggregator.ArgsPair{
			Base:                      "GWEI",
			Quote:                     "QUOTE",
			PercentDifferenceToNotify: 1,
			Decimals:                  12,
			Exchanges:                 map[string]struct{}{"Binance": {}},
		})

		pn, _ := aggregator.NewPriceNotifier(args)
		err := pn.Execute(context.Background())
		assert.ErrorIs(t, err, assert.AnError)

		// should not notify if gas price service fails
		assert.Equal(t, 0, numCalled)
	})
}
