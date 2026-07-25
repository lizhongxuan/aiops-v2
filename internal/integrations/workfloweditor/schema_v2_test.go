package workfloweditor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowOperationSchemaV2IsStrictDiscriminatedUnion(t *testing.T) {
	root := decodeSchemaObject(t, WorkflowOperationSchemaV2())
	oneOf, ok := root["oneOf"].([]any)
	if !ok || len(oneOf) != 9 {
		t.Fatalf("operation oneOf = %#v, want 9 variants", root["oneOf"])
	}
	want := map[string]bool{
		"add_node": true, "update_node": true, "delete_node": true,
		"add_edge": true, "update_edge": true, "delete_edge": true,
		"update_workflow": true, "update_layout": true, "attach_manual_binding": true,
	}
	for _, rawVariant := range oneOf {
		variant, ok := rawVariant.(map[string]any)
		if !ok {
			t.Fatalf("variant = %#v, want object", rawVariant)
		}
		if additional, exists := variant["additionalProperties"]; !exists || additional != false {
			t.Fatalf("variant additionalProperties = %#v, want false", additional)
		}
		properties := variant["properties"].(map[string]any)
		op := properties["op"].(map[string]any)
		enum := op["enum"].([]any)
		if len(enum) != 1 || !want[enum[0].(string)] {
			t.Fatalf("unexpected operation enum %#v", enum)
		}
		delete(want, enum[0].(string))
		if enum[0] == "add_node" {
			assertRequiredFields(t, properties["node"], "type", "position")
		}
		if enum[0] == "add_edge" {
			assertRequiredFields(t, properties["edge"], "source", "target")
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing operation variants: %#v", want)
	}
}

func TestWorkflowDomainErrorSchemaFreezesTypedCodes(t *testing.T) {
	schema := decodeSchemaObject(t, WorkflowContractSchemasV2()["DomainError"])
	properties := schema["properties"].(map[string]any)
	codes := properties["errorCode"].(map[string]any)["enum"].([]any)
	if len(codes) != 18 {
		t.Fatalf("errorCode enum = %d values, want 18", len(codes))
	}
}

func TestWorkflowToolContractsV2ExposeSixStrictSchemas(t *testing.T) {
	contracts := WorkflowToolContractsV2()
	if len(contracts) != 6 {
		t.Fatalf("tool contracts = %d, want 6", len(contracts))
	}
	for name, contract := range contracts {
		input := decodeSchemaObject(t, contract.InputSchema)
		if input["additionalProperties"] != false {
			t.Fatalf("%s input additionalProperties = %#v, want false", name, input["additionalProperties"])
		}
		output := decodeSchemaObject(t, contract.OutputSchema)
		if oneOf, ok := output["oneOf"].([]any); !ok || len(oneOf) != 2 {
			t.Fatalf("%s output must distinguish success and typed error: %#v", name, output)
		}
	}
	for _, name := range []string{"workflow.get_context", "workflow.get_action_catalog"} {
		properties := decodeSchemaObject(t, contracts[name].InputSchema)["properties"].(map[string]any)
		if _, exposed := properties["workflowKey"]; exposed {
			t.Fatalf("%s exposes server-owned workflow identity to the model", name)
		}
	}
	contextProperties := decodeSchemaObject(t, contracts["workflow.get_context"].InputSchema)["properties"].(map[string]any)
	if got := contextProperties["view"].(map[string]any)["enum"].([]any); len(got) != 4 {
		t.Fatalf("workflow.get_context view enum = %#v, want four bounded views", got)
	}
	preview := decodeSchemaObject(t, contracts["workflow.preview_patch"].InputSchema)
	properties := preview["properties"].(map[string]any)
	operations := properties["operations"].(map[string]any)
	items := operations["items"].(map[string]any)
	if variants, ok := items["oneOf"].([]any); !ok || len(variants) != 9 {
		t.Fatalf("preview operations schema = %#v, want 9 variants", items)
	}
}

func TestWorkflowPlanIRSchemaIsStrictTypedUnion(t *testing.T) {
	root := decodeSchemaObject(t, WorkflowPlanIRSchemaV1())
	if root["additionalProperties"] != false {
		t.Fatalf("plan additionalProperties = %#v", root["additionalProperties"])
	}
	items := root["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	variants, ok := items["oneOf"].([]any)
	if !ok || len(variants) != 8 {
		t.Fatalf("plan item union = %#v", items)
	}
	want := map[string]string{"action": "action", "branch": "branch", "parallel": "parallel", "join": "join", "loop": "loop", "approval": "approval", "subflow": "subflow", "aggregation": "aggregation"}
	for _, raw := range variants {
		variant := raw.(map[string]any)
		properties := variant["properties"].(map[string]any)
		kind := properties["kind"].(map[string]any)["enum"].([]any)[0].(string)
		if _, ok := properties[want[kind]]; !ok || variant["additionalProperties"] != false {
			t.Fatalf("invalid %s variant: %#v", kind, variant)
		}
	}
	compile := decodeSchemaObject(t, WorkflowToolContractsV2()["workflow.compile_plan"].InputSchema)
	if compile["additionalProperties"] != false || compile["properties"].(map[string]any)["plan"] == nil {
		t.Fatalf("compile input schema = %#v", compile)
	}
}

func TestWorkflowContractSchemaV2Golden(t *testing.T) {
	contractJSON, err := json.Marshal(WorkflowContractSchemasV2())
	if err != nil {
		t.Fatal(err)
	}
	toolJSON, err := json.Marshal(WorkflowToolContractsV2())
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(append(contractJSON, toolJSON...))
	got := hex.EncodeToString(digest[:])
	const want = "bad40efc513cf6bcd77b7220b6fad9af97ae976b8b32215520582fe46f669195"
	if got != want {
		t.Fatalf("workflow v2 schema golden digest = %s, want %s", got, want)
	}
	for _, forbidden := range []string{`"password"`, `"apiKey"`, `"token"`, `"status":"ok"`} {
		if strings.Contains(string(contractJSON), forbidden) || strings.Contains(string(toolJSON), forbidden) {
			t.Fatalf("generated schema contains forbidden legacy/credential field %s", forbidden)
		}
	}
}

func decodeSchemaObject(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("invalid schema JSON: %v\n%s", err, raw)
	}
	return object
}

func assertRequiredFields(t *testing.T, raw any, fields ...string) {
	t.Helper()
	object := raw.(map[string]any)
	required := object["required"].([]any)
	for _, field := range fields {
		found := false
		for _, value := range required {
			found = found || value == field
		}
		if !found {
			t.Fatalf("required = %#v, missing %s", required, field)
		}
	}
}
