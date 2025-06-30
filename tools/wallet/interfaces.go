package wallet

import "github.com/klever-io/klv-bridge-eth-go/clients/klever/blockchain/address"

type Wallet interface {
	PrivateKey() []byte
	PublicKey() []byte
	Address() (address.Address, error)
	Sign(msg []byte) ([]byte, error)
	SignHex(msg string) ([]byte, error)
}
