package toolfailure

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateArgumentsAgainstInputSchema(t *testing.T) {
	cases := []struct {
		name      string
		schema    json.RawMessage
		arguments json.RawMessage
		wantErr   string
	}{
		{
			name:      "empty schema allows arguments",
			schema:    nil,
			arguments: json.RawMessage(`{"namespace":"prod"}`),
		},
		{
			name: "missing required object field",
			schema: json.RawMessage(`{
				"type":"object",
				"required":["namespace"],
				"properties":{"namespace":{"type":"string"}}
			}`),
			arguments: json.RawMessage(`{"service":"api"}`),
			wantErr:   "namespace",
		},
		{
			name: "malformed JSON arguments",
			schema: json.RawMessage(`{
				"type":"object",
				"required":["namespace"],
				"properties":{"namespace":{"type":"string"}}
			}`),
			arguments: json.RawMessage(`{`),
			wantErr:   "valid JSON",
		},
		{
			name: "simple property type mismatch",
			schema: json.RawMessage(`{
				"type":"object",
				"properties":{"limit":{"type":"integer"}}
			}`),
			arguments: json.RawMessage(`{"limit":"10"}`),
			wantErr:   "limit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateArguments(tc.schema, tc.arguments)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateArguments returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateArguments returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateArguments error = %q, want containing %q", err.Error(), tc.wantErr)
			}
		})
	}
}
