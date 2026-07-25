package runtimekernel

import "aiops-v2/internal/runtimecontract"

func cloneTrustedTurnIdentity(identity *runtimecontract.TurnIdentity) *runtimecontract.TurnIdentity {
	if identity == nil {
		return nil
	}
	cloned := *identity
	return &cloned
}
