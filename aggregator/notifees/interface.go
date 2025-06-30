package notifees

import (
	"context"

	"github.com/klever-io/klever-go/data/transaction"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/address"
	"github.com/klever-io/klv-bridge-eth-go/clients/klever/proxy/models"
)

// Proxy holds the primitive functions that the multiversx proxy engine supports & implements
// dependency inversion: blockchain package is considered inner business logic, this package is considered "plugin"
type Proxy interface {
	GetNetworkConfig(ctx context.Context) (*models.NetworkConfig, error)
	GetAccount(ctx context.Context, address address.Address) (*models.Account, error)
	SendTransaction(ctx context.Context, tx *transaction.Transaction) (string, error)
	SendTransactions(ctx context.Context, txs []*transaction.Transaction) ([]string, error)
	IsInterfaceNil() bool
}

// TransactionNonceHandler defines the component able to apply nonce for a given FrontendTransaction
type TransactionNonceHandler interface {
	ApplyNonceAndGasPrice(ctx context.Context, address address.Address, tx *transaction.Transaction) error
	SendTransaction(ctx context.Context, tx *transaction.Transaction) (string, error)
	IsInterfaceNil() bool
}
