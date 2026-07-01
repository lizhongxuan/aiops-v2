package promptinput

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func (r ProviderRole) IsValid() bool {
	switch r {
	case ProviderRoleSystem, ProviderRoleDeveloper, ProviderRoleUser, ProviderRoleAssistant, ProviderRoleTool:
		return true
	default:
		return false
	}
}

func (i ModelInputItem) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if !i.ProviderRole.IsValid() {
		return fmt.Errorf("provider role %q is invalid", i.ProviderRole)
	}
	if i.ProviderRole == ProviderRoleTool {
		if strings.TrimSpace(i.ToolCallID) == "" && strings.TrimSpace(i.ToolResultToolCallID()) == "" {
			return fmt.Errorf("tool result requires tool call id")
		}
	}
	for idx, call := range i.ToolCalls {
		if strings.TrimSpace(call.ID) == "" {
			return fmt.Errorf("tool call[%d] id is required", idx)
		}
		if strings.TrimSpace(call.Name) == "" {
			return fmt.Errorf("tool call[%d] name is required", idx)
		}
		if len(call.Arguments) > 0 && !json.Valid(call.Arguments) {
			return fmt.Errorf("tool call[%d] arguments must be valid json", idx)
		}
	}
	for idx, part := range i.ContentParts {
		if strings.TrimSpace(part.Type) == "" {
			return fmt.Errorf("content part[%d] type is required", idx)
		}
		if part.Type != "text" {
			return fmt.Errorf("content part[%d] type %q is unsupported", idx, part.Type)
		}
	}
	return nil
}

func (i ModelInputItem) StableHash() string {
	data, _ := json.Marshal(i)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func StableModelInputHash(items []ModelInputItem) string {
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
