package notifees

import (
	"context"

	"github.com/klever-io/klever-go/crypto/hashing"
	factoryHasher "github.com/klever-io/klever-go/crypto/hashing/factory"
	"github.com/klever-io/klever-go/data/transaction"
	"github.com/klever-io/klever-go/tools/marshal"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/address"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/builders"
	"github.com/klever-io/klv-oracles-go/aggregator"
	"github.com/klever-io/klv-oracles-go/tools/wallet"
	"github.com/multiversx/mx-chain-core-go/core/check"
	logger "github.com/multiversx/mx-chain-logger-go"
)

const function = "submitBatch"
const minGasLimit = uint64(1)

var log = logger.GetOrCreate("klv-oracle-go/aggregator/notifees")

// ArgsKCNotifee is the argument DTO for the NewKCNotifee function
type ArgsKCNotifee struct {
	Proxy           Proxy
	TxNonceHandler  TransactionNonceHandler
	ContractAddress address.Address
	Wallet          wallet.Wallet
	BaseGasLimit    uint64
	GasLimitForEach uint64
}

type kcNotifee struct {
	proxy           Proxy
	txNonceHandler  TransactionNonceHandler
	contractAddress address.Address
	wallet          wallet.Wallet
	hasher          hashing.Hasher
	marshalizer     marshal.Marshalizer
	baseGasLimit    uint64
	gasLimitForEach uint64
}

// NewKCNotifee will create a new instance of kcNotifee
func NewKCNotifee(args ArgsKCNotifee) (*kcNotifee, error) {
	err := checkArgsKCNotifee(args)
	if err != nil {
		return nil, err
	}

	hasher, err := factoryHasher.NewHasher("blake2b")
	if err != nil {
		return nil, err
	}

	notifee := &kcNotifee{
		proxy:           args.Proxy,
		txNonceHandler:  args.TxNonceHandler,
		contractAddress: args.ContractAddress,
		wallet:          args.Wallet,
		hasher:          hasher,
		marshalizer:     marshal.NewProtoMarshalizer(),
		baseGasLimit:    args.BaseGasLimit,
		gasLimitForEach: args.GasLimitForEach,
	}

	return notifee, nil
}

func checkArgsKCNotifee(args ArgsKCNotifee) error {
	if check.IfNil(args.Proxy) {
		return errNilProxy
	}
	if check.IfNil(args.TxNonceHandler) {
		return errNilTxNonceHandler
	}
	if check.IfNil(args.ContractAddress) {
		return errNilContractAddressHandler
	}
	if args.BaseGasLimit < minGasLimit {
		return errInvalidBaseGasLimit
	}
	if args.GasLimitForEach < minGasLimit {
		return errInvalidGasLimitForEach
	}

	return nil
}

// PriceChanged is the function that gets called by a price notifier. This function will assemble a Klever Blockchain
// transaction, having the transaction's data field containing all the price changes information
func (en *kcNotifee) PriceChanged(ctx context.Context, priceChanges []*aggregator.ArgsPriceChanged) error {
	txData, err := en.prepareTxData(priceChanges)
	if err != nil {
		return err
	}

	networkConfig, err := en.proxy.GetNetworkConfig(ctx)
	if err != nil {
		return err
	}

	// building transaction to be signed, and send using proxy interface, but noncehandler as intermediare to help with nonce logic
	tx := transaction.NewBaseTransaction(en.wallet.PublicKey(), 0, [][]byte{txData}, 0, 0)
	tx.SetChainID([]byte(networkConfig.ChainID))

	contractRequest := &transaction.SmartContract{
		Type:    transaction.SmartContract_SCInvoke,
		Address: en.contractAddress.Bytes(),
	}

	tx.PushContract(transaction.TXContract_SmartContractType, contractRequest)

	notifeeAddress, err := address.NewAddressFromBytes(en.wallet.PublicKey())
	if err != nil {
		return err
	}

	err = en.txNonceHandler.ApplyNonceAndGasPrice(ctx, notifeeAddress, tx)
	if err != nil {
		return err
	}

	hash, err := en.calculateHash(tx.GetRawData())
	if err != nil {
		return err
	}

	signature, err := en.wallet.Sign(hash)
	if err != nil {
		return err
	}

	tx.AddSignature(signature)

	txHash, err := en.txNonceHandler.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}

	log.Debug("sent transaction", "hash", txHash)

	return nil
}

// calculateHash marshalizes the interface and calculates its hash
func (en *kcNotifee) calculateHash(
	object interface{},
) ([]byte, error) {
	mrsData, err := en.marshalizer.Marshal(object)
	if err != nil {
		return nil, err
	}

	hash := en.hasher.Compute(string(mrsData))
	return hash, nil
}

func (en *kcNotifee) prepareTxData(priceChanges []*aggregator.ArgsPriceChanged) ([]byte, error) {
	txDataBuilder := builders.NewTxDataBuilder()
	txDataBuilder.Function(function)

	for _, priceChange := range priceChanges {
		txDataBuilder.ArgBytes([]byte(priceChange.Base)).
			ArgBytes([]byte(priceChange.Quote)).
			ArgInt64(priceChange.Timestamp).
			ArgInt64(int64(priceChange.DenominatedPrice)).
			ArgInt64(int64(priceChange.Decimals))
	}

	return txDataBuilder.ToDataBytes()
}

// IsInterfaceNil returns true if there is no value under the interface
func (en *kcNotifee) IsInterfaceNil() bool {
	return en == nil
}
