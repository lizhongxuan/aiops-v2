package tooling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ValidateToolResult validates the canonical structured result at the shared
// dispatcher boundary. Legacy tools may satisfy an object/array schema with
// JSON Content and a string schema with their plain Content value.
func ValidateToolResult(schema json.RawMessage, result ToolResult) error {
	if len(bytes.TrimSpace(schema)) == 0 || result.ExecutionFailed() {
		return nil
	}
	var spec resultSchema
	if err := decodeResultJSON(schema, &spec); err != nil {
		return fmt.Errorf("output schema is not valid JSON: %w", err)
	}
	value, err := resultValue(spec, result)
	if err != nil {
		return err
	}
	return validateResultValue("result", spec, value)
}

type resultSchema struct {
	Type                 any                     `json:"type"`
	Required             []string                `json:"required"`
	Properties           map[string]resultSchema `json:"properties"`
	AdditionalProperties any                     `json:"additionalProperties"`
	Items                *resultSchema           `json:"items"`
	OneOf                []resultSchema          `json:"oneOf"`
	AnyOf                []resultSchema          `json:"anyOf"`
	Enum                 []any                   `json:"enum"`
	Const                any                     `json:"const"`
}

func resultValue(spec resultSchema, result ToolResult) (any, error) {
	if len(bytes.TrimSpace(result.StructuredContent)) > 0 {
		var value any
		if err := decodeResultJSON(result.StructuredContent, &value); err != nil {
			return nil, fmt.Errorf("structuredContent must be valid JSON: %w", err)
		}
		return value, nil
	}
	if schemaAllowsType(spec.Type, "string") && !schemaAllowsType(spec.Type, "object") && !schemaAllowsType(spec.Type, "array") {
		return result.Content, nil
	}
	var value any
	if err := decodeResultJSON([]byte(result.Content), &value); err != nil {
		return nil, fmt.Errorf("tool Content must contain JSON matching output schema: %w", err)
	}
	return value, nil
}

func validateResultValue(path string, spec resultSchema, value any) error {
	if len(spec.OneOf) > 0 {
		matches := 0
		for _, candidate := range spec.OneOf {
			if validateResultValue(path, candidate, value) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s must match exactly one output schema", path)
		}
	}
	if len(spec.AnyOf) > 0 {
		matched := false
		for _, candidate := range spec.AnyOf {
			if validateResultValue(path, candidate, value) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must match an allowed output schema", path)
		}
	}
	if spec.Const != nil && !reflect.DeepEqual(spec.Const, value) {
		return fmt.Errorf("%s must equal the declared const", path)
	}
	if len(spec.Enum) > 0 {
		matched := false
		for _, allowed := range spec.Enum {
			if reflect.DeepEqual(allowed, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s is not an allowed value", path)
		}
	}
	if err := validateResultType(path, spec.Type, value); err != nil {
		return err
	}
	if object, ok := value.(map[string]any); ok {
		for _, name := range spec.Required {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for name, child := range object {
			property, declared := spec.Properties[name]
			if !declared {
				if allow, ok := spec.AdditionalProperties.(bool); ok && !allow {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
				continue
			}
			if err := validateResultValue(path+"."+name, property, child); err != nil {
				return err
			}
		}
	}
	if items, ok := value.([]any); ok && spec.Items != nil {
		for index, item := range items {
			if err := validateResultValue(path+"["+strconv.Itoa(index)+"]", *spec.Items, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateResultType(path string, schemaType any, value any) error {
	if schemaType == nil || value == nil && schemaAllowsType(schemaType, "null") {
		return nil
	}
	valid := false
	for _, kind := range schemaTypes(schemaType) {
		switch kind {
		case "null":
			valid = valid || value == nil
		case "string":
			_, ok := value.(string)
			valid = valid || ok
		case "boolean":
			_, ok := value.(bool)
			valid = valid || ok
		case "object":
			_, ok := value.(map[string]any)
			valid = valid || ok
		case "array":
			_, ok := value.([]any)
			valid = valid || ok
		case "number":
			_, ok := value.(json.Number)
			valid = valid || ok
		case "integer":
			if number, ok := value.(json.Number); ok {
				_, err := strconv.ParseInt(number.String(), 10, 64)
				valid = valid || err == nil
			}
		}
	}
	if valid || len(schemaTypes(schemaType)) == 0 {
		return nil
	}
	return fmt.Errorf("%s must be %s", path, strings.Join(schemaTypes(schemaType), " or "))
}

func schemaAllowsType(schemaType any, want string) bool {
	for _, value := range schemaTypes(schemaType) {
		if value == want {
			return true
		}
	}
	return false
}

func schemaTypes(schemaType any) []string {
	switch typed := schemaType.(type) {
	case string:
		return []string{typed}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
		return values
	default:
		return nil
	}
}

func decodeResultJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return nil
}
