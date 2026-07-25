package workfloweditor

import (
	"encoding/json"

	"github.com/eino-contrib/jsonschema"
	contract "runner/workflow/edit"
)

type ToolContractV2 struct {
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
}

var operationVariantsV2 = []any{
	&contract.AddNodeOperation{},
	&contract.UpdateNodeOperation{},
	&contract.DeleteNodeOperation{},
	&contract.AddEdgeOperation{},
	&contract.UpdateEdgeOperation{},
	&contract.DeleteEdgeOperation{},
	&contract.UpdateWorkflowOperation{},
	&contract.UpdateLayoutOperation{},
	&contract.AttachManualBindingOperation{},
}

func WorkflowOperationSchemaV2() json.RawMessage {
	oneOf := make([]any, 0, len(operationVariantsV2))
	for index, variant := range operationVariantsV2 {
		schema := reflectedSchemaObject(variant)
		properties := schema["properties"].(map[string]any)
		if index == 0 {
			requireNested(properties, "node", "type", "position")
		}
		if index == 3 {
			requireNested(properties, "edge", "source", "target")
		}
		oneOf = append(oneOf, schema)
	}
	return mustJSON(map[string]any{
		"$schema":       jsonschema.Version,
		"oneOf":         oneOf,
		"discriminator": map[string]any{"propertyName": "op"},
	})
}

func requireNested(properties map[string]any, property string, required ...string) {
	nested := properties[property].(map[string]any)
	nested["required"] = required
}

func WorkflowContractSchemasV2() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"WorkflowKey":               reflectSchema(contract.WorkflowKey{}),
		"Revision":                  reflectSchema(contract.Revision{}),
		"ValidationBasis":           reflectSchema(contract.ValidationBasis{}),
		"WorkflowEditSessionV2":     reflectSchema(contract.WorkflowEditSessionV2{}),
		"WorkflowOperation":         WorkflowOperationSchemaV2(),
		"WorkflowChangeSet":         patchOperations(reflectSchema(contract.WorkflowChangeSet{})),
		"WorkflowCandidate":         reflectSchema(contract.WorkflowCandidate{}),
		"WorkflowEditGrant":         reflectSchema(contract.WorkflowEditGrant{}),
		"WorkflowApplyApproval":     reflectSchema(contract.WorkflowApplyApproval{}),
		"WorkflowMutationReceipt":   reflectSchema(contract.WorkflowMutationReceipt{}),
		"WorkflowDiagnostic":        reflectSchema(contract.WorkflowDiagnostic{}),
		"WorkflowConflict":          reflectSchema(contract.WorkflowConflict{}),
		"LifecycleGate":             reflectSchema(contract.LifecycleGate{}),
		"WorkflowDomainEvent":       reflectSchema(contract.WorkflowDomainEvent{}),
		"DomainError":               reflectSchema(contract.DomainError{}),
		"WorkflowPlanIR":            WorkflowPlanIRSchemaV1(),
		"WorkflowPlanDiagnostic":    reflectSchema(contract.WorkflowPlanDiagnostic{}),
		"WorkflowPlanCompileResult": workflowCompileResultSchemaV1(),
	}
}

func WorkflowToolContractsV2() map[string]ToolContractV2 {
	return map[string]ToolContractV2{
		"workflow.get_context": {
			InputSchema:  reflectSchema(workflowGetContextInputV2{}),
			OutputSchema: toolResultSchema(contract.WorkflowContext{}),
		},
		"workflow.get_action_catalog": {
			InputSchema:  reflectSchema(workflowActionCatalogInputV2{}),
			OutputSchema: toolResultSchema(contract.ActionCatalogPage{}),
		},
		"workflow.preview_patch": {
			InputSchema:  patchOperations(reflectSchema(contract.PreviewPatchRequest{})),
			OutputSchema: toolResultSchema(contract.ChangeSetPreview{}),
		},
		"workflow.apply_patch": {
			InputSchema:  reflectSchema(contract.ApplyPatchRequest{}),
			OutputSchema: toolResultSchema(contract.WorkflowMutationReceipt{}),
		},
		"workflow.validate": {
			InputSchema:  reflectSchema(contract.ValidateRequest{}),
			OutputSchema: toolResultSchema(contract.ValidationResult{}),
		},
		"workflow.compile_plan": {
			InputSchema:  workflowCompilePlanInputSchemaV1(),
			OutputSchema: toolResultSchema(WorkflowCompilePlanResultV2{}),
		},
	}
}

func toolResultSchema(success any) json.RawMessage {
	var successResult any
	switch success.(type) {
	case contract.WorkflowContext:
		successResult = contract.ToolResult[contract.WorkflowContext]{}
	case contract.ActionCatalogPage:
		successResult = contract.ToolResult[contract.ActionCatalogPage]{}
	case contract.ChangeSetPreview:
		successResult = contract.ToolResult[contract.ChangeSetPreview]{}
	case contract.WorkflowMutationReceipt:
		successResult = contract.ToolResult[contract.WorkflowMutationReceipt]{}
	case WorkflowCompilePlanResultV2:
		successResult = contract.ToolResult[WorkflowCompilePlanResultV2]{}
	default:
		successResult = contract.ToolResult[contract.ValidationResult]{}
	}
	schema := map[string]any{
		"$schema": jsonschema.Version,
		"oneOf": []any{
			reflectedSchemaObject(successResult),
			reflectedSchemaObject(contract.ToolResult[contract.DomainError]{}),
		},
	}
	if _, ok := success.(WorkflowCompilePlanResultV2); ok {
		patchCompileResultOperations(schema["oneOf"].([]any)[0].(map[string]any))
	}
	if _, ok := success.(contract.ChangeSetPreview); ok {
		patchNestedOperations(schema["oneOf"].([]any)[0].(map[string]any), "structuredContent", "changeSet", "operations")
	}
	normalizeRawMessageSchemas(schema)
	return mustJSON(schema)
}

func WorkflowPlanIRSchemaV1() json.RawMessage {
	root := reflectedSchemaObject(contract.WorkflowPlanIR{})
	items := root["properties"].(map[string]any)["items"].(map[string]any)
	base := items["items"].(map[string]any)
	payloads := map[contract.WorkflowPlanItemKind]string{contract.PlanItemAction: "action", contract.PlanItemBranch: "branch", contract.PlanItemParallel: "parallel", contract.PlanItemJoin: "join", contract.PlanItemLoop: "loop", contract.PlanItemApproval: "approval", contract.PlanItemSubflow: "subflow", contract.PlanItemAggregation: "aggregation"}
	kinds := []contract.WorkflowPlanItemKind{contract.PlanItemAction, contract.PlanItemBranch, contract.PlanItemParallel, contract.PlanItemJoin, contract.PlanItemLoop, contract.PlanItemApproval, contract.PlanItemSubflow, contract.PlanItemAggregation}
	variants := make([]any, 0, len(kinds))
	for _, kind := range kinds {
		variant := cloneSchemaObject(base)
		properties := variant["properties"].(map[string]any)
		properties["kind"].(map[string]any)["enum"] = []any{string(kind)}
		for other, field := range payloads {
			if other != kind {
				delete(properties, field)
			}
		}
		variant["required"] = appendSchemaRequired(variant["required"], payloads[kind])
		variants = append(variants, variant)
	}
	items["items"] = map[string]any{"oneOf": variants, "discriminator": map[string]any{"propertyName": "kind"}}
	normalizeRawMessageSchemas(root)
	return mustJSON(root)
}

func workflowCompilePlanInputSchemaV1() json.RawMessage {
	root := reflectedSchemaObject(WorkflowCompilePlanRequestV2{})
	var plan any
	_ = json.Unmarshal(WorkflowPlanIRSchemaV1(), &plan)
	root["properties"].(map[string]any)["plan"] = plan
	return mustJSON(root)
}

func workflowCompileResultSchemaV1() json.RawMessage {
	root := reflectedSchemaObject(WorkflowCompilePlanResultV2{})
	patchCompileResultOperations(root)
	normalizeRawMessageSchemas(root)
	return mustJSON(root)
}

func patchCompileResultOperations(root map[string]any) {
	properties := root["properties"].(map[string]any)
	if structured, ok := properties["structuredContent"].(map[string]any); ok {
		properties = structured["properties"].(map[string]any)
	}
	operations := properties["operations"].(map[string]any)
	var union any
	_ = json.Unmarshal(WorkflowOperationSchemaV2(), &union)
	operations["items"] = union
}

func patchNestedOperations(root map[string]any, path ...string) {
	current := root
	for _, name := range path {
		current = current["properties"].(map[string]any)[name].(map[string]any)
	}
	var union any
	_ = json.Unmarshal(WorkflowOperationSchemaV2(), &union)
	current["items"] = union
}

func cloneSchemaObject(source map[string]any) map[string]any {
	raw := mustJSON(source)
	var clone map[string]any
	_ = json.Unmarshal(raw, &clone)
	return clone
}

func appendSchemaRequired(raw any, field string) []any {
	required, _ := raw.([]any)
	for _, value := range required {
		if value == field {
			return required
		}
	}
	return append(required, field)
}

// The reflector uses the valid JSON Schema boolean `true` for RawMessage.
// The shared runtime output validator consumes object schemas, so express the
// same unrestricted value as an empty schema object at those property leaves.
func normalizeRawMessageSchemas(schema map[string]any) {
	if properties, ok := schema["properties"].(map[string]any); ok {
		for name, raw := range properties {
			if allowed, boolean := raw.(bool); boolean && allowed {
				properties[name] = map[string]any{}
				continue
			}
			if child, ok := raw.(map[string]any); ok {
				normalizeRawMessageSchemas(child)
			}
		}
	}
	if allowed, boolean := schema["items"].(bool); boolean && allowed {
		schema["items"] = map[string]any{}
	} else if items, ok := schema["items"].(map[string]any); ok {
		normalizeRawMessageSchemas(items)
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		if variants, ok := schema[keyword].([]any); ok {
			for _, raw := range variants {
				if child, ok := raw.(map[string]any); ok {
					normalizeRawMessageSchemas(child)
				}
			}
		}
	}
}

func reflectSchema(value any) json.RawMessage { return mustJSON(reflectedSchemaObject(value)) }

func reflectedSchemaObject(value any) map[string]any {
	reflector := jsonschema.Reflector{Anonymous: true, DoNotReference: true, ExpandedStruct: true}
	raw := mustJSON(reflector.Reflect(value))
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		panic(err)
	}
	return object
}

func patchOperations(schema json.RawMessage) json.RawMessage {
	var object map[string]any
	if err := json.Unmarshal(schema, &object); err != nil {
		panic(err)
	}
	properties, _ := object["properties"].(map[string]any)
	operations, _ := properties["operations"].(map[string]any)
	var union any
	if err := json.Unmarshal(WorkflowOperationSchemaV2(), &union); err != nil {
		panic(err)
	}
	operations["items"] = union
	return mustJSON(object)
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
