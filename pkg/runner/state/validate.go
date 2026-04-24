package state

import (
	"fmt"
	"strings"
)

func ValidateResourceID(id string) error {
	if id == "" {
		return fmt.Errorf("resource_id is required")
	}
	if strings.ContainsAny(id, " \t\n\r") {
		return fmt.Errorf("resource_id must not contain whitespace")
	}
	return nil
}
