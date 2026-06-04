package toolfailure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ValidateArguments performs conservative pre-dispatch checks for the subset of
// JSON Schema used by tool input definitions. It intentionally avoids enforcing
// additionalProperties so runtime-added context fields do not break legacy tools.
func ValidateArguments(schema json.RawMessage, arguments json.RawMessage) error {
	if len(bytes.TrimSpace(schema)) == 0 {
		return nil
	}
	var spec inputSchema
	if err := decodeJSON(schema, &spec); err != nil {
		return fmt.Errorf("input schema is not valid JSON: %w", err)
	}
	if spec.isEmpty() {
		return nil
	}

	args := bytes.TrimSpace(arguments)
	if len(args) == 0 {
		args = []byte(`{}`)
	}
	var value any
	if err := decodeJSON(args, &value); err != nil {
		return fmt.Errorf("arguments must be valid JSON: %w", err)
	}

	if spec.hasObjectShape() {
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("arguments must be an object")
		}
		for _, required := range spec.Required {
			field := strings.TrimSpace(required)
			if field == "" {
				continue
			}
			if _, ok := obj[field]; !ok {
				return fmt.Errorf("%s is required", field)
			}
		}
		for name, propertySchema := range spec.Properties {
			if propertySchema == nil {
				continue
			}
			fieldValue, ok := obj[name]
			if !ok {
				continue
			}
			if err := validateSimpleType(name, propertySchema.Type, fieldValue); err != nil {
				return err
			}
		}
		return nil
	}

	return validateSimpleType("arguments", spec.Type, value)
}

type inputSchema struct {
	Type       any                        `json:"type"`
	Required   []string                   `json:"required"`
	Properties map[string]*propertySchema `json:"properties"`
}

func (s inputSchema) isEmpty() bool {
	return s.Type == nil && len(s.Required) == 0 && len(s.Properties) == 0
}

func (s inputSchema) hasObjectShape() bool {
	return schemaTypeAllows(s.Type, "object") || len(s.Required) > 0 || len(s.Properties) > 0
}

type propertySchema struct {
	Type any `json:"type"`
}

func decodeJSON(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(v)
}

func validateSimpleType(name string, schemaType any, value any) error {
	if schemaType == nil || schemaTypeAllows(schemaType, "null") && value == nil {
		return nil
	}
	switch {
	case schemaTypeAllows(schemaType, "string"):
		if _, ok := value.(string); ok {
			return nil
		}
	case schemaTypeAllows(schemaType, "boolean"):
		if _, ok := value.(bool); ok {
			return nil
		}
	case schemaTypeAllows(schemaType, "number"):
		if isJSONNumber(value) {
			return nil
		}
	case schemaTypeAllows(schemaType, "integer"):
		if isJSONInteger(value) {
			return nil
		}
	case schemaTypeAllows(schemaType, "array"):
		if _, ok := value.([]any); ok {
			return nil
		}
	case schemaTypeAllows(schemaType, "object"):
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s must be %s", name, describeSchemaType(schemaType))
}

func schemaTypeAllows(schemaType any, target string) bool {
	switch typed := schemaType.(type) {
	case string:
		return typed == target
	case []any:
		for _, item := range typed {
			if item == target {
				return true
			}
		}
	}
	return false
}

func isJSONNumber(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := number.Float64()
	return err == nil
}

func isJSONInteger(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := strconv.ParseInt(number.String(), 10, 64)
	return err == nil
}

func describeSchemaType(schemaType any) string {
	switch typed := schemaType.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "null" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " or ")
		}
	}
	return "the declared schema type"
}
