package notifees

import "errors"

var (
	errNilProxy                  = errors.New("nil proxy")
	errNilTxNonceHandler         = errors.New("nil tx nonce handler")
	errNilContractAddressHandler = errors.New("nil contract address handler")
	errNilWallet                 = errors.New("nil wallet")
)
