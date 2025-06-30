package notifees

import "errors"

var (
	errNilProxy                  = errors.New("nil proxy")
	errNilTxNonceHandler         = errors.New("nil tx nonce handler")
	errNilContractAddressHandler = errors.New("nil contract address handler")
	errInvalidBaseGasLimit       = errors.New("invalid base gas limit")
	errInvalidGasLimitForEach    = errors.New("invalid gas limit for each price change")
	errNilWallet                 = errors.New("nil wallet")
)
