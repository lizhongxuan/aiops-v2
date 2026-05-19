package opsmanual

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func (f *OperationFrame) UnmarshalJSON(data []byte) error {
	type operationFrameAlias OperationFrame
	var aux struct {
		operationFrameAlias
		RequiredParams json.RawMessage `json:"required_params"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*f = OperationFrame(aux.operationFrameAlias)
	if len(aux.RequiredParams) == 0 || bytes.Equal(bytes.TrimSpace(aux.RequiredParams), []byte("null")) {
		return nil
	}
	params, err := decodeOperationFrameRequiredParams(aux.RequiredParams)
	if err != nil {
		return err
	}
	f.RequiredParams = params
	return nil
}

func decodeOperationFrameRequiredParams(raw json.RawMessage) (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err == nil {
		return object, nil
	}

	var values []any
	if err := json.Unmarshal(raw, &values); err == nil {
		params := map[string]any{}
		for _, value := range values {
			name := requiredParamName(value)
			if name == "" {
				continue
			}
			params[name] = ""
		}
		return params, nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil, nil
		}
		return map[string]any{single: ""}, nil
	}

	return nil, fmt.Errorf("invalid operation_frame.required_params")
}

func requiredParamName(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"id", "name", "key"} {
			raw, ok := typed[key]
			if !ok {
				continue
			}
			if name := strings.TrimSpace(fmt.Sprint(raw)); name != "" {
				return name
			}
		}
	}
	return ""
}
