package mock

import (
	"context"

	gas "github.com/klever-io/klv-oracles-go/aggregator/gasStation"
)

// GasPriceServiceStub -
type GasPriceServiceStub struct {
	ConvertGasPricesCalled    func(ctx context.Context, pairs []gas.ArgsPairInfo) ([]gas.ArgsPairInfo, error)
	VerifyRequiredPairsCalled func(pairs []gas.ArgsPairInfo) error
}

// ConvertGasPrices -
func (stub *GasPriceServiceStub) ConvertGasPrices(ctx context.Context, pairsInfo []gas.ArgsPairInfo) ([]gas.ArgsPairInfo, error) {
	if stub.ConvertGasPricesCalled != nil {
		return stub.ConvertGasPricesCalled(ctx, pairsInfo)
	}

	return pairsInfo, nil
}

// VerifyRequiredPairs -
func (stub *GasPriceServiceStub) VerifyRequiredPairs(pairsInfo []gas.ArgsPairInfo) error {
	if stub.VerifyRequiredPairsCalled != nil {
		return stub.VerifyRequiredPairsCalled(pairsInfo)
	}

	return nil
}

// IsInterfaceNil -
func (stub *GasPriceServiceStub) IsInterfaceNil() bool {
	return stub == nil
}
