package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"runner/workflow"
)

func TestActionCatalogDefaultSpecsAreDeterministicAndValid(t *testing.T) {
	catalog := NewActionCatalog()

	items := catalog.List(context.Background(), ActionCatalogFilter{})
	if len(items) != 8 {
		t.Fatalf("expected 8 default specs, got %d", len(items))
	}
	for i := 1; i < len(items); i++ {
		prev := items[i-1].Category + "/" + items[i-1].Action
		curr := items[i].Category + "/" + items[i].Action
		if prev > curr {
			t.Fatalf("catalog is not sorted: %s before %s", prev, curr)
		}
	}

	for _, action := range []string{"cmd.run", "shell.run"} {
		if _, ok := catalog.Get(context.Background(), action); ok {
			t.Fatalf("%s should not be in the default catalog", action)
		}
	}
	for _, action := range []string{"script.shell", "script.python", "wait.event"} {
		if _, ok := catalog.Get(context.Background(), action); !ok {
			t.Fatalf("%s should be present", action)
		}
	}
	for _, item := range items {
		if item.Risk == "" {
			t.Fatalf("action %s missing risk metadata", item.Action)
		}
		if len(item.ArgsSchema) == 0 || !json.Valid(item.ArgsSchema) {
			t.Fatalf("action %s missing valid args schema", item.Action)
		}
		if len(item.Defaults) == 0 {
			t.Fatalf("action %s missing defaults", item.Action)
		}
		if len(item.Outputs) == 0 {
			t.Fatalf("action %s missing outputs", item.Action)
		}
		if len(item.Examples) == 0 {
			t.Fatalf("action %s missing examples", item.Action)
		}
	}
}

func TestActionCatalogStructuredIOSchemasForCoreActions(t *testing.T) {
	catalog := NewActionCatalog()

	for _, action := range []string{"script.shell", "script.python", "wait.event", "notify.send"} {
		t.Run(action, func(t *testing.T) {
			spec, ok := catalog.Get(context.Background(), action)
			if !ok {
				t.Fatalf("%s should be present", action)
			}
			if len(spec.ArgsSchema) == 0 || !json.Valid(spec.ArgsSchema) {
				t.Fatalf("%s should keep valid args_schema for compatibility: %s", action, string(spec.ArgsSchema))
			}
			if len(spec.InputsSchema) == 0 || !json.Valid(spec.InputsSchema) {
				t.Fatalf("%s missing valid inputs_schema: %s", action, string(spec.InputsSchema))
			}
			if len(spec.OutputsSchema) == 0 || !json.Valid(spec.OutputsSchema) {
				t.Fatalf("%s missing valid outputs_schema: %s", action, string(spec.OutputsSchema))
			}
			if len(spec.InputExamples) == 0 {
				t.Fatalf("%s missing input_examples", action)
			}
			if len(spec.OutputExamples) == 0 {
				t.Fatalf("%s missing output_examples", action)
			}
			if spec.Label == "" {
				t.Fatalf("%s missing label", action)
			}
			if len(spec.Capabilities) == 0 {
				t.Fatalf("%s missing capabilities", action)
			}
			if len(spec.DefaultPorts.Inputs) == 0 {
				t.Fatalf("%s missing default input ports", action)
			}
			if len(spec.DefaultPorts.Outputs) == 0 {
				t.Fatalf("%s missing default output ports", action)
			}
			inputs := decodeSchema(t, spec.InputsSchema)
			outputs := decodeSchema(t, spec.OutputsSchema)
			if inputs["type"] != "object" {
				t.Fatalf("%s inputs_schema type = %v, want object", action, inputs["type"])
			}
			if outputs["type"] != "object" {
				t.Fatalf("%s outputs_schema type = %v, want object", action, outputs["type"])
			}
			outputProps := schemaProperties(t, outputs)
			if len(outputProps) == 0 {
				t.Fatalf("%s outputs_schema has no properties: %+v", action, outputs)
			}
			if action == "notify.send" {
				if _, ok := outputProps["delivered"]; !ok {
					t.Fatalf("%s outputs_schema missing delivered property: %+v", action, outputs)
				}
			} else if action == "wait.event" {
				if _, ok := outputProps["event"]; !ok {
					t.Fatalf("%s outputs_schema missing event property: %+v", action, outputs)
				}
			} else if _, ok := outputProps["stdout"]; !ok {
				t.Fatalf("%s outputs_schema missing stdout property: %+v", action, outputs)
			}
		})
	}

	shell, _ := catalog.Get(context.Background(), "script.shell")
	if got, _ := shell.Defaults["script"].(string); !strings.HasPrefix(got, "set -euo pipefail\n") {
		t.Fatalf("script.shell should default to an inline strict shell script, got %+v", shell.Defaults)
	}
	if _, ok := shell.Defaults["script_ref"]; ok {
		t.Fatalf("script.shell defaults should prefer inline script, got %+v", shell.Defaults)
	}

	approval, _ := catalog.Get(context.Background(), "manual.approval")
	if got := portIDs(approval.DefaultPorts.Outputs); len(got) != 2 || got[0] != "approved" || got[1] != "rejected" {
		t.Fatalf("manual.approval default output ports = %+v", got)
	}
	aggregator, ok := catalog.Get(context.Background(), "variable.aggregate")
	if !ok {
		t.Fatal("variable.aggregate should be present")
	}
	if aggregator.NodeType != "variable_aggregator" {
		t.Fatalf("variable.aggregate node type = %q, want variable_aggregator", aggregator.NodeType)
	}
	if got := portIDs(aggregator.DefaultPorts.Outputs); len(got) != 1 || got[0] != "next" {
		t.Fatalf("variable.aggregate default output ports = %+v", got)
	}
	if _, ok := schemaProperties(t, decodeSchema(t, aggregator.OutputsSchema))["value"]; !ok {
		t.Fatalf("variable.aggregate outputs schema missing value: %s", string(aggregator.OutputsSchema))
	}
}

func TestActionCatalogReturnsDefensiveCopies(t *testing.T) {
	catalog := NewActionCatalog()
	spec, ok := catalog.Get(context.Background(), "script.shell")
	if !ok {
		t.Fatal("script.shell should be present")
	}
	spec.ArgsSchema[0] = 'x'
	spec.Defaults["script"] = "mutated"
	spec.Examples[0].Args["script"] = "mutated"
	spec.InputsSchema[0] = 'x'
	spec.OutputsSchema[0] = 'x'
	spec.InputExamples[0].Values["script"] = "mutated"
	spec.OutputExamples[0].Values["stdout"] = "mutated"
	spec.DefaultPorts.Outputs[0].ID = "mutated"
	spec.Capabilities[0] = "mutated"

	fresh, ok := catalog.Get(context.Background(), "script.shell")
	if !ok {
		t.Fatal("script.shell should still be present")
	}
	if !json.Valid(fresh.ArgsSchema) {
		t.Fatalf("catalog returned mutable args schema: %s", string(fresh.ArgsSchema))
	}
	if !json.Valid(fresh.InputsSchema) {
		t.Fatalf("catalog returned mutable inputs schema: %s", string(fresh.InputsSchema))
	}
	if !json.Valid(fresh.OutputsSchema) {
		t.Fatalf("catalog returned mutable outputs schema: %s", string(fresh.OutputsSchema))
	}
	if fresh.Defaults["script"] == "mutated" {
		t.Fatalf("catalog returned mutable defaults: %+v", fresh.Defaults)
	}
	if fresh.Examples[0].Args["script"] == "mutated" {
		t.Fatalf("catalog returned mutable examples: %+v", fresh.Examples)
	}
	if fresh.InputExamples[0].Values["script"] == "mutated" {
		t.Fatalf("catalog returned mutable input examples: %+v", fresh.InputExamples)
	}
	if fresh.OutputExamples[0].Values["stdout"] == "mutated" {
		t.Fatalf("catalog returned mutable output examples: %+v", fresh.OutputExamples)
	}
	if fresh.DefaultPorts.Outputs[0].ID == "mutated" {
		t.Fatalf("catalog returned mutable default ports: %+v", fresh.DefaultPorts)
	}
	if fresh.Capabilities[0] == "mutated" {
		t.Fatalf("catalog returned mutable capabilities: %+v", fresh.Capabilities)
	}
}

func TestActionCatalogValidateStep(t *testing.T) {
	catalog := NewActionCatalog()

	missingRequired := catalog.ValidateStep(workflow.Step{
		Name:   "missing",
		Action: "script.shell",
	})
	if len(missingRequired) != 1 || missingRequired[0].Field != "args.script" {
		t.Fatalf("expected missing args.script issue, got %+v", missingRequired)
	}

	scriptWithRef := catalog.ValidateStep(workflow.Step{
		Name:   "stored",
		Action: "script.shell",
		Args: map[string]any{
			"script_ref": "restore.sh",
		},
	})
	if len(scriptWithRef) != 0 {
		t.Fatalf("script_ref should be accepted for stored scripts: %+v", scriptWithRef)
	}

	scriptConflict := catalog.ValidateStep(workflow.Step{
		Name:   "stored",
		Action: "script.shell",
		Args: map[string]any{
			"script_ref": "restore.sh",
			"script":     "echo restore",
		},
	})
	if len(scriptConflict) != 1 || scriptConflict[0].Field != "args.script_ref" {
		t.Fatalf("expected script/script_ref conflict, got %+v", scriptConflict)
	}

	unknown := catalog.ValidateStep(workflow.Step{
		Name:   "bad",
		Action: "bad.action",
	})
	if len(unknown) != 1 || unknown[0].Field != "action" {
		t.Fatalf("expected unknown action issue, got %+v", unknown)
	}
}

func TestActionCatalogRegisterCustomSpec(t *testing.T) {
	catalog := NewActionCatalog(ActionSpec{
		Action:      "custom.echo",
		Title:       "Echo",
		Category:    "custom",
		Description: "custom action",
		ArgsSchema:  json.RawMessage(`{"type":"object"}`),
	})

	spec, ok := catalog.Get(context.Background(), "custom.echo")
	if !ok {
		t.Fatal("custom action should be registered")
	}
	if spec.NodeType != "action" {
		t.Fatalf("default node type mismatch: %s", spec.NodeType)
	}

	if err := catalog.Register(ActionSpec{
		Action:     "broken",
		Title:      "Broken",
		Category:   "custom",
		ArgsSchema: json.RawMessage(`{`),
	}); err == nil {
		t.Fatal("invalid json schema should be rejected")
	}
}

func decodeSchema(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode schema %s: %v", string(raw), err)
	}
	return out
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %+v", schema)
	}
	return props
}

func schemaRequired(schema map[string]any) map[string]bool {
	out := map[string]bool{}
	raw, _ := schema["required"].([]any)
	for _, item := range raw {
		if key, ok := item.(string); ok {
			out[key] = true
		}
	}
	return out
}

func portIDs(ports []ActionPortSpec) []string {
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		out = append(out, port.ID)
	}
	return out
}
