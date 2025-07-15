package notifees

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/klever-io/klever-go/data/transaction"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/address"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/builders"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/proxy/models"
	"github.com/klever-io/klv-bridge-eth-go/testsCommon"
	"github.com/klever-io/klv-bridge-eth-go/testsCommon/interactors"
	"github.com/klever-io/klv-oracles-go/aggregator"
	"github.com/klever-io/klv-oracles-go/tools/wallet"
	"github.com/multiversx/mx-chain-core-go/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var walletSk = "8734062c1158f26a3ca8a4a0da87b527a7c168653f7f4c77045e5cf571497d9d"

const chainID = "test"

func createMockArgsKCNotifee() ArgsKCNotifee {
	contractAddress, _ := address.NewAddressFromBytes(bytes.Repeat([]byte{1}, 32))

	notifeesWallet, _ := wallet.NewWalletFroHex(walletSk)

	return ArgsKCNotifee{
		Proxy:           &interactors.ProxyStub{},
		TxNonceHandler:  &testsCommon.TxNonceHandlerV2Stub{},
		Wallet:          notifeesWallet,
		ContractAddress: contractAddress,
		BaseGasLimit:    1,
		GasLimitForEach: 1,
	}
}

func createMockArgsKCNotifeeWithSomeRealComponents() ArgsKCNotifee {
	proxy := &interactors.ProxyStub{
		GetNetworkConfigCalled: func(ctx context.Context) (*models.NetworkConfig, error) {
			return &models.NetworkConfig{
				ChainID: chainID,
			}, nil
		},
	}

	contractAddress, _ := address.NewAddressFromBytes(bytes.Repeat([]byte{1}, 32))

	notifeesWallet, _ := wallet.NewWalletFroHex(walletSk)
	return ArgsKCNotifee{
		Proxy:           proxy,
		TxNonceHandler:  &testsCommon.TxNonceHandlerV2Stub{},
		ContractAddress: contractAddress,
		Wallet:          notifeesWallet,
		BaseGasLimit:    2000,
		GasLimitForEach: 30,
	}
}

func createMockPriceChanges() []*aggregator.ArgsPriceChanged {
	return []*aggregator.ArgsPriceChanged{
		{
			Base:             "USD",
			Quote:            "ETH",
			DenominatedPrice: 380000,
			Decimals:         2,
			Timestamp:        200,
		},
		{
			Base:             "USD",
			Quote:            "BTC",
			DenominatedPrice: 47000000000,
			Decimals:         6,
			Timestamp:        300,
		},
	}
}

func TestNewKCNotifee(t *testing.T) {
	t.Parallel()

	t.Run("nil proxy should error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsKCNotifee()
		args.Proxy = nil
		en, err := NewKCNotifee(args)

		assert.True(t, check.IfNil(en))
		assert.Equal(t, errNilProxy, err)
	})
	t.Run("nil tx nonce handler should error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsKCNotifee()
		args.TxNonceHandler = nil
		en, err := NewKCNotifee(args)

		assert.True(t, check.IfNil(en))
		assert.Equal(t, errNilTxNonceHandler, err)
	})
	t.Run("nil contract address should error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsKCNotifee()
		args.ContractAddress = nil
		en, err := NewKCNotifee(args)

		assert.True(t, check.IfNil(en))
		assert.Equal(t, errNilContractAddressHandler, err)
	})
	t.Run("nil wallet should error", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsKCNotifee()
		args.Wallet = nil
		en, err := NewKCNotifee(args)

		assert.True(t, check.IfNil(en))
		assert.Equal(t, errNilWallet, err)
	})
	t.Run("should work", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsKCNotifee()
		en, err := NewKCNotifee(args)

		assert.False(t, check.IfNil(en))
		assert.Nil(t, err)
	})
}

func TestKCNotifee_PriceChanged(t *testing.T) {
	t.Parallel()

	t.Run("get nonce errors", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("expected error")
		args := createMockArgsKCNotifeeWithSomeRealComponents()
		args.TxNonceHandler = &testsCommon.TxNonceHandlerV2Stub{
			ApplyNonceAndGasPriceCalled: func(ctx context.Context, address address.Address, tx *transaction.Transaction) error {
				return expectedErr
			},
			SendTransactionCalled: func(ctx context.Context, tx *transaction.Transaction) (string, error) {
				assert.Fail(t, "should have not called SendTransaction")
				return "", nil
			},
		}

		en, err := NewKCNotifee(args)
		require.Nil(t, err)

		priceChanges := createMockPriceChanges()
		err = en.PriceChanged(context.Background(), priceChanges)
		assert.Equal(t, expectedErr, err)
	})
	t.Run("invalid price arguments", func(t *testing.T) {
		t.Parallel()

		args := createMockArgsKCNotifeeWithSomeRealComponents()
		args.TxNonceHandler = &testsCommon.TxNonceHandlerV2Stub{
			ApplyNonceAndGasPriceCalled: func(ctx context.Context, address address.Address, tx *transaction.Transaction) error {
				tx.RawData.Nonce = 43
				return nil
			},
			SendTransactionCalled: func(ctx context.Context, tx *transaction.Transaction) (string, error) {
				assert.Fail(t, "should have not called SendTransaction")
				return "", nil
			},
		}

		en, err := NewKCNotifee(args)
		require.Nil(t, err)

		priceChanges := createMockPriceChanges()
		priceChanges[0].Base = ""
		err = en.PriceChanged(context.Background(), priceChanges)
		assert.True(t, errors.Is(err, builders.ErrInvalidValue))
	})
	t.Run("get network config errors", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("expected error")
		args := createMockArgsKCNotifeeWithSomeRealComponents()
		args.Proxy = &interactors.ProxyStub{
			GetNetworkConfigCalled: func(ctx context.Context) (*models.NetworkConfig, error) {
				return nil, expectedErr
			},
		}
		args.TxNonceHandler = &testsCommon.TxNonceHandlerV2Stub{
			ApplyNonceAndGasPriceCalled: func(ctx context.Context, address address.Address, tx *transaction.Transaction) error {
				tx.RawData.Nonce = 43
				return nil
			},
			SendTransactionCalled: func(ctx context.Context, tx *transaction.Transaction) (string, error) {
				assert.Fail(t, "should have not called SendTransaction")
				return "", nil
			},
		}

		en, err := NewKCNotifee(args)
		require.Nil(t, err)

		priceChanges := createMockPriceChanges()
		err = en.PriceChanged(context.Background(), priceChanges)
		assert.Equal(t, expectedErr, err)
	})
	t.Run("send transaction errors", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("expected error")
		args := createMockArgsKCNotifeeWithSomeRealComponents()
		args.TxNonceHandler = &testsCommon.TxNonceHandlerV2Stub{
			ApplyNonceAndGasPriceCalled: func(ctx context.Context, address address.Address, tx *transaction.Transaction) error {
				tx.RawData.Nonce = 43
				return nil
			},
			SendTransactionCalled: func(ctx context.Context, tx *transaction.Transaction) (string, error) {
				return "", expectedErr
			},
		}

		en, err := NewKCNotifee(args)
		require.Nil(t, err)

		priceChanges := createMockPriceChanges()
		err = en.PriceChanged(context.Background(), priceChanges)
		assert.Equal(t, expectedErr, err)
	})
	t.Run("should work", func(t *testing.T) {
		t.Parallel()

		priceChanges := createMockPriceChanges()
		sentWasCalled := false
		args := createMockArgsKCNotifeeWithSomeRealComponents()
		args.TxNonceHandler = &testsCommon.TxNonceHandlerV2Stub{
			ApplyNonceAndGasPriceCalled: func(ctx context.Context, address address.Address, tx *transaction.Transaction) error {
				tx.RawData.Nonce = 43
				return nil
			},
			SendTransactionCalled: func(ctx context.Context, tx *transaction.Transaction) (string, error) {
				txDataStrings := []string{
					function,
					hex.EncodeToString([]byte(priceChanges[0].Base)),
					hex.EncodeToString([]byte(priceChanges[0].Quote)),
					hex.EncodeToString(big.NewInt(priceChanges[0].Timestamp).Bytes()),
					hex.EncodeToString(big.NewInt(int64(priceChanges[0].DenominatedPrice)).Bytes()),
					hex.EncodeToString(big.NewInt(int64(priceChanges[0].Decimals)).Bytes()),
					hex.EncodeToString([]byte(priceChanges[1].Base)),
					hex.EncodeToString([]byte(priceChanges[1].Quote)),
					hex.EncodeToString(big.NewInt(priceChanges[1].Timestamp).Bytes()),
					hex.EncodeToString(big.NewInt(int64(priceChanges[1].DenominatedPrice)).Bytes()),
					hex.EncodeToString(big.NewInt(int64(priceChanges[1].Decimals)).Bytes()),
				}
				txData := []byte(strings.Join(txDataStrings, "@"))

				rawData := tx.GetRawData()
				require.NotNil(t, rawData)
				assert.Equal(t, uint64(43), rawData.GetNonce())

				require.Len(t, tx.GetContracts(), 1)
				sc := tx.GetContracts()[0]
				tc := &transaction.SmartContract{}
				// Get the message name which is compatible with the golang transaction.protoType registry
				err := anypb.UnmarshalTo(sc.Parameter, tc, protobuf.UnmarshalOptions{})
				require.Nil(t, err)

				scAddr, err := address.NewAddressFromBytes(tc.Address)
				require.Nil(t, err)
				assert.Equal(t, "klv1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqsy6zanq", scAddr.Bech32())

				recvAddr, err := address.NewAddressFromBytes(rawData.GetSender())
				require.Nil(t, err)
				assert.Equal(t, "klv1usdnywjhrlv4tcyu6stxpl6yvhplg35nepljlt4y5r7yppe8er4qujlazy", recvAddr.Bech32())

				require.Len(t, tx.GetRawData().GetData(), 1)
				assert.Equal(t, txData, rawData.GetData()[0])
				assert.Equal(t, []byte(chainID), rawData.GetChainID())
				assert.Equal(t, uint32(1), rawData.GetVersion())

				sentWasCalled = true

				return "hash", nil
			},
		}

		en, err := NewKCNotifee(args)
		require.Nil(t, err)

		err = en.PriceChanged(context.Background(), priceChanges)
		assert.Nil(t, err)
		assert.True(t, sentWasCalled)
	})
}
