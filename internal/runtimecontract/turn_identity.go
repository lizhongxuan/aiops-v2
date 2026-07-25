package runtimecontract

import (
	"fmt"
	"strings"
)

// TurnIdentity is authenticated admission identity frozen before a turn
// enters RuntimeKernel. It is never populated from model arguments or message
// metadata.
type TurnIdentity struct {
	TenantID  string `json:"tenantId"`
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
}

func (identity TurnIdentity) Validate() error {
	values := []string{identity.TenantID, identity.UserID, identity.SessionID}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid trusted turn identity")
		}
	}
	return nil
}
