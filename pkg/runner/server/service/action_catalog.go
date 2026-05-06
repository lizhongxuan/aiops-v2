package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"runner/workflow"
)

type ActionCatalog struct {
	mu    sync.RWMutex
	specs map[string]ActionSpec
}

type ActionCatalogFilter struct {
	Category     string
	Experimental *bool
}

type ActionSpec struct {
	Action         string             `json:"action"`
	Label          string             `json:"label,omitempty"`
	Title          string             `json:"title"`
	Category       string             `json:"category"`
	Description    string             `json:"description,omitempty"`
	Risk           string             `json:"risk,omitempty"`
	NodeType       string             `json:"node_type,omitempty"`
	Capabilities   []string           `json:"capabilities,omitempty"`
	ArgsSchema     json.RawMessage    `json:"args_schema,omitempty"`
	InputSchema    json.RawMessage    `json:"input_schema,omitempty"`
	OutputSchema   json.RawMessage    `json:"output_schema,omitempty"`
	InputsSchema   json.RawMessage    `json:"inputs_schema,omitempty"`
	OutputsSchema  json.RawMessage    `json:"outputs_schema,omitempty"`
	DefaultPorts   ActionDefaultPorts `json:"default_ports,omitempty"`
	Defaults       map[string]any     `json:"defaults,omitempty"`
	RequiredArgs   []string           `json:"required_args,omitempty"`
	Outputs        []OutputSpec       `json:"outputs,omitempty"`
	Examples       []ActionExample    `json:"examples,omitempty"`
	InputExamples  []ActionIOExample  `json:"input_examples,omitempty"`
	OutputExamples []ActionIOExample  `json:"output_examples,omitempty"`
	Experimental   bool               `json:"experimental,omitempty"`
	Deprecated     bool               `json:"deprecated,omitempty"`
}

type ActionDefaultPorts struct {
	Inputs  []ActionPortSpec `json:"inputs,omitempty"`
	Outputs []ActionPortSpec `json:"outputs,omitempty"`
}

type ActionPortSpec struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

type OutputSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

type ActionExample struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
}

type ActionIOExample struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Values      map[string]any `json:"values,omitempty"`
}

type ActionValidationIssue struct {
	Type    string `json:"type"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func NewActionCatalog(specs ...ActionSpec) *ActionCatalog {
	c := &ActionCatalog{specs: map[string]ActionSpec{}}
	for _, spec := range DefaultActionSpecs() {
		_ = c.Register(spec)
	}
	for _, spec := range specs {
		_ = c.Register(spec)
	}
	return c
}

func DefaultActionSpecs() []ActionSpec {
	return []ActionSpec{
		{
			Action:       "cmd.run",
			Title:        "Command",
			Category:     "command",
			Description:  "Run a shell command through /bin/sh -c on each target.",
			Risk:         "medium",
			NodeType:     "action",
			RequiredArgs: []string{"cmd"},
			Defaults:     map[string]any{"cmd": "echo hello"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"cmd": map[string]any{
					"type":        "string",
					"title":       "Command",
					"description": "Command passed to /bin/sh -c.",
					"minLength":   1,
				},
				"dir": envStringSchema("Working directory"),
				"env": envObjectSchema(),
			}, []string{"cmd"}),
			Outputs: commandOutputs(),
			Examples: []ActionExample{{
				Title: "Check disk",
				Args:  map[string]any{"cmd": "df -h"},
			}},
		},
		{
			Action:       "shell.run",
			Title:        "Shell Script",
			Category:     "script",
			Description:  "Run inline shell script content through /bin/sh.",
			Risk:         "high",
			NodeType:     "action",
			RequiredArgs: []string{"script"},
			Defaults:     map[string]any{"script": "set -e\necho ok"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"script": map[string]any{
					"type":        "string",
					"title":       "Script",
					"description": "Shell script content.",
					"minLength":   1,
				},
				"dir":         envStringSchema("Working directory"),
				"env":         envObjectSchema(),
				"export_vars": boolSchema("Parse RUNNER_EXPORT_* lines from stdout."),
			}, []string{"script"}),
			Outputs: commandOutputs(),
			Examples: []ActionExample{{
				Title: "Restart service",
				Args:  map[string]any{"script": "systemctl restart app.service", "export_vars": true},
			}},
		},
		{
			Action:      "script.shell",
			Title:       "Stored Shell Script",
			Category:    "script",
			Description: "Run shell script content resolved by the script service or supplied inline.",
			Risk:        "high",
			NodeType:    "action",
			Defaults:    map[string]any{"script_ref": "restore.sh"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"script_ref": envStringSchema("Stored script name"),
				"script":     envStringSchema("Inline shell script"),
				"args": map[string]any{
					"type":        "array",
					"title":       "Arguments",
					"description": "Arguments passed to the script.",
					"items":       map[string]any{"type": "string"},
				},
				"dir":         envStringSchema("Working directory"),
				"env":         envObjectSchema(),
				"export_vars": boolSchema("Parse RUNNER_EXPORT_* lines from stdout."),
			}, nil),
			Outputs: commandOutputs(),
			Examples: []ActionExample{{
				Title: "Run stored restore script",
				Args:  map[string]any{"script_ref": "restore.sh", "args": []string{"--dry-run"}},
			}},
		},
		{
			Action:      "script.python",
			Title:       "Stored Python Script",
			Category:    "script",
			Description: "Run Python script content resolved by the script service or supplied inline.",
			Risk:        "high",
			NodeType:    "action",
			Defaults:    map[string]any{"script_ref": "verify.py"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"script_ref": envStringSchema("Stored script name"),
				"script":     envStringSchema("Inline Python script"),
				"args": map[string]any{
					"type":        "array",
					"title":       "Arguments",
					"description": "Arguments passed to the script.",
					"items":       map[string]any{"type": "string"},
				},
				"dir":         envStringSchema("Working directory"),
				"env":         envObjectSchema(),
				"export_vars": boolSchema("Parse RUNNER_EXPORT_* lines from stdout."),
			}, nil),
			Outputs: commandOutputs(),
			Examples: []ActionExample{{
				Title: "Verify metrics",
				Args:  map[string]any{"script_ref": "verify.py", "args": []string{"order-service"}},
			}},
		},
		{
			Action:       "wait.event",
			Title:        "Wait For Event",
			Category:     "control",
			Description:  "Wait for an external event. The runner module is registered but not implemented yet.",
			Risk:         "low",
			NodeType:     "action",
			RequiredArgs: []string{"event"},
			Defaults:     map[string]any{"timeout": "30m"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"event":   envStringSchema("Event name"),
				"timeout": envStringSchema("Timeout duration"),
			}, []string{"event"}),
			Outputs: []OutputSpec{{Name: "event", Type: "object", Description: "Matched event payload."}},
			Examples: []ActionExample{{
				Title: "Wait for approval event",
				Args:  map[string]any{"event": "approval.resolved", "timeout": "30m"},
			}},
			Experimental: true,
		},
		{
			Action:      "condition.evaluate",
			Title:       "Condition",
			Category:    "control",
			Description: "Evaluate a graph condition node or condition edge. Current runner execution uses step.when for compatible projections.",
			Risk:        "read_only",
			NodeType:    "condition",
			Defaults:    map[string]any{"expression": "vars.ready == true"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"expression": envStringSchema("Condition expression"),
			}, nil),
			Outputs: []OutputSpec{{Name: "result", Type: "boolean", Description: "Condition evaluation result."}},
			Examples: []ActionExample{{
				Title: "Check exported variable",
				Args:  map[string]any{"expression": "vars.disk_free == true"},
			}},
			Experimental: true,
		},
		{
			Action:      "manual.approval",
			Title:       "Manual Approval",
			Category:    "control",
			Description: "Pause a graph run until an operator approves or rejects the node. Requires the graph executor before production execution.",
			Risk:        "medium",
			NodeType:    "manual_approval",
			Defaults:    map[string]any{"subjects": []string{"oncall"}, "timeout": "30m", "on_timeout": "reject"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"subjects": map[string]any{
					"type":  "array",
					"title": "Approvers",
					"items": map[string]any{"type": "string"},
				},
				"timeout":    envStringSchema("Approval timeout"),
				"on_timeout": envStringSchema("Timeout policy"),
			}, nil),
			Outputs: []OutputSpec{{Name: "decision", Type: "string", Description: "Operator decision: approved or rejected."}},
			Examples: []ActionExample{{
				Title: "Require on-call approval",
				Args:  map[string]any{"subjects": []string{"oncall"}, "timeout": "30m", "on_timeout": "reject"},
			}},
			Experimental: true,
		},
		{
			Action:       "workflow.run",
			Title:        "Subflow",
			Category:     "control",
			Description:  "Invoke another saved workflow as a graph node. Requires the graph executor before production execution.",
			Risk:         "medium",
			NodeType:     "subflow",
			RequiredArgs: []string{"workflow"},
			Defaults:     map[string]any{"workflow": "child-workflow"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"workflow": envStringSchema("Workflow name"),
				"vars": map[string]any{
					"type":                 "object",
					"title":                "Input variables",
					"additionalProperties": true,
				},
			}, []string{"workflow"}),
			Outputs: []OutputSpec{{Name: "run_id", Type: "string", Description: "Child workflow run id."}},
			Examples: []ActionExample{{
				Title: "Run child workflow",
				Args:  map[string]any{"workflow": "restore-verify", "vars": map[string]any{"target": "pg-primary"}},
			}},
			Experimental: true,
		},
		{
			Action:      "notify.send",
			Title:       "Notify",
			Category:    "control",
			Description: "Send a notification or trigger an external notification channel.",
			Risk:        "low",
			NodeType:    "action",
			Defaults:    map[string]any{"channel": "webhook", "template": "Runner notification: ${workflow.name}"},
			ArgsSchema: actionArgsSchema(map[string]any{
				"channel": map[string]any{
					"type":        "string",
					"title":       "Channel",
					"description": "Notification channel.",
					"enum":        []string{"webhook", "email", "slack", "pagerduty"},
				},
				"recipients": map[string]any{
					"type":        "array",
					"title":       "Recipients",
					"description": "Notification recipients.",
					"items":       map[string]any{"type": "string"},
				},
				"template": envStringSchema("Message template"),
			}, []string{"template"}),
			Outputs: []OutputSpec{
				{Name: "delivered", Type: "boolean", Description: "Whether the notification was accepted for delivery."},
				{Name: "message_id", Type: "string", Description: "Provider message id when available."},
			},
			Examples: []ActionExample{{
				Title: "Notify on failure",
				Args:  map[string]any{"channel": "webhook", "template": "restore failed: ${node.restore.stderr}"},
			}},
		},
	}
}

func (c *ActionCatalog) Register(spec ActionSpec) error {
	normalized, err := normalizeActionSpec(spec)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.specs[normalized.Action] = normalized
	return nil
}

func (c *ActionCatalog) List(_ context.Context, filter ActionCatalogFilter) []ActionSpec {
	if c == nil {
		c = NewActionCatalog()
	}
	category := strings.TrimSpace(filter.Category)

	c.mu.RLock()
	defer c.mu.RUnlock()
	items := make([]ActionSpec, 0, len(c.specs))
	for _, spec := range c.specs {
		if category != "" && spec.Category != category {
			continue
		}
		if filter.Experimental != nil && spec.Experimental != *filter.Experimental {
			continue
		}
		items = append(items, cloneActionSpec(spec))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Action < items[j].Action
	})
	return items
}

func (c *ActionCatalog) Get(_ context.Context, action string) (ActionSpec, bool) {
	if c == nil {
		c = NewActionCatalog()
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return ActionSpec{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	spec, ok := c.specs[action]
	if !ok {
		return ActionSpec{}, false
	}
	return cloneActionSpec(spec), true
}

func (c *ActionCatalog) ValidateStep(step workflow.Step) []ActionValidationIssue {
	action := strings.TrimSpace(step.Action)
	if action == "" {
		return []ActionValidationIssue{{
			Type:    "validation",
			Field:   "action",
			Message: "action is required",
		}}
	}
	spec, ok := c.Get(context.Background(), action)
	if !ok {
		return []ActionValidationIssue{{
			Type:    "validation",
			Field:   "action",
			Message: fmt.Sprintf("action %q is not in the catalog", action),
		}}
	}

	var issues []ActionValidationIssue
	for _, arg := range spec.RequiredArgs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		if !hasNonEmptyArg(step.Args, arg) {
			issues = append(issues, ActionValidationIssue{
				Type:    "validation",
				Field:   "args." + arg,
				Message: fmt.Sprintf("action %q requires args.%s", action, arg),
			})
		}
	}
	if action == "script.shell" || action == "script.python" {
		hasScript := hasNonEmptyArg(step.Args, "script")
		hasRef := hasNonEmptyArg(step.Args, "script_ref")
		if !hasScript && !hasRef {
			issues = append(issues, ActionValidationIssue{
				Type:    "validation",
				Field:   "args.script",
				Message: fmt.Sprintf("action %q requires args.script or args.script_ref", action),
			})
		}
		if hasScript && hasRef {
			issues = append(issues, ActionValidationIssue{
				Type:    "validation",
				Field:   "args.script_ref",
				Message: fmt.Sprintf("action %q cannot use args.script and args.script_ref together", action),
			})
		}
	}
	return issues
}

func normalizeActionSpec(spec ActionSpec) (ActionSpec, error) {
	spec.Action = strings.TrimSpace(spec.Action)
	spec.Label = strings.TrimSpace(spec.Label)
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Category = strings.TrimSpace(spec.Category)
	spec.Risk = strings.TrimSpace(spec.Risk)
	spec.NodeType = strings.TrimSpace(spec.NodeType)
	if spec.Action == "" {
		return ActionSpec{}, fmt.Errorf("%w: action is required", ErrInvalid)
	}
	if spec.Title == "" {
		return ActionSpec{}, fmt.Errorf("%w: action %q title is required", ErrInvalid, spec.Action)
	}
	if spec.Label == "" {
		spec.Label = spec.Title
	}
	if spec.Category == "" {
		return ActionSpec{}, fmt.Errorf("%w: action %q category is required", ErrInvalid, spec.Action)
	}
	if spec.NodeType == "" {
		spec.NodeType = "action"
	}
	if spec.Risk == "" {
		spec.Risk = "medium"
	}
	spec.Capabilities = normalizeCatalogStringList(spec.Capabilities)
	if len(spec.Capabilities) == 0 {
		spec.Capabilities = defaultActionCapabilities(spec)
	}
	spec.DefaultPorts = normalizeActionDefaultPorts(spec.DefaultPorts)
	if len(spec.DefaultPorts.Inputs) == 0 && len(spec.DefaultPorts.Outputs) == 0 {
		spec.DefaultPorts = defaultActionPorts(spec)
	}
	if len(spec.InputsSchema) == 0 && len(spec.InputSchema) > 0 {
		spec.InputsSchema = append(json.RawMessage{}, spec.InputSchema...)
	}
	if len(spec.InputsSchema) == 0 && len(spec.ArgsSchema) > 0 {
		spec.InputsSchema = append(json.RawMessage{}, spec.ArgsSchema...)
	}
	if len(spec.InputSchema) == 0 && len(spec.InputsSchema) > 0 {
		spec.InputSchema = append(json.RawMessage{}, spec.InputsSchema...)
	}
	if len(spec.OutputsSchema) == 0 && len(spec.OutputSchema) > 0 {
		spec.OutputsSchema = append(json.RawMessage{}, spec.OutputSchema...)
	}
	if len(spec.OutputsSchema) == 0 && len(spec.Outputs) > 0 {
		spec.OutputsSchema = actionOutputsSchema(spec.Outputs)
	}
	if len(spec.OutputSchema) == 0 && len(spec.OutputsSchema) > 0 {
		spec.OutputSchema = append(json.RawMessage{}, spec.OutputsSchema...)
	}
	if len(spec.InputExamples) == 0 && len(spec.Examples) > 0 {
		spec.InputExamples = inputExamplesFromActionExamples(spec.Examples)
	}
	if len(spec.OutputExamples) == 0 && len(spec.Outputs) > 0 {
		spec.OutputExamples = outputExamplesFromSpecs(spec.Outputs)
	}
	if len(spec.ArgsSchema) > 0 && !json.Valid(spec.ArgsSchema) {
		return ActionSpec{}, fmt.Errorf("%w: action %q args_schema must be valid json", ErrInvalid, spec.Action)
	}
	if len(spec.InputSchema) > 0 && !json.Valid(spec.InputSchema) {
		return ActionSpec{}, fmt.Errorf("%w: action %q input_schema must be valid json", ErrInvalid, spec.Action)
	}
	if len(spec.OutputSchema) > 0 && !json.Valid(spec.OutputSchema) {
		return ActionSpec{}, fmt.Errorf("%w: action %q output_schema must be valid json", ErrInvalid, spec.Action)
	}
	if len(spec.InputsSchema) > 0 && !json.Valid(spec.InputsSchema) {
		return ActionSpec{}, fmt.Errorf("%w: action %q inputs_schema must be valid json", ErrInvalid, spec.Action)
	}
	if len(spec.OutputsSchema) > 0 && !json.Valid(spec.OutputsSchema) {
		return ActionSpec{}, fmt.Errorf("%w: action %q outputs_schema must be valid json", ErrInvalid, spec.Action)
	}
	return cloneActionSpec(spec), nil
}

func cloneActionSpec(spec ActionSpec) ActionSpec {
	spec.Defaults = cloneAnyMap(spec.Defaults)
	spec.Capabilities = append([]string{}, spec.Capabilities...)
	spec.DefaultPorts = cloneActionDefaultPorts(spec.DefaultPorts)
	spec.RequiredArgs = append([]string{}, spec.RequiredArgs...)
	spec.Outputs = append([]OutputSpec{}, spec.Outputs...)
	spec.Examples = cloneActionExamples(spec.Examples)
	spec.InputExamples = cloneActionIOExamples(spec.InputExamples)
	spec.OutputExamples = cloneActionIOExamples(spec.OutputExamples)
	if spec.ArgsSchema != nil {
		spec.ArgsSchema = append(json.RawMessage{}, spec.ArgsSchema...)
	}
	if spec.InputSchema != nil {
		spec.InputSchema = append(json.RawMessage{}, spec.InputSchema...)
	}
	if spec.OutputSchema != nil {
		spec.OutputSchema = append(json.RawMessage{}, spec.OutputSchema...)
	}
	if spec.InputsSchema != nil {
		spec.InputsSchema = append(json.RawMessage{}, spec.InputsSchema...)
	}
	if spec.OutputsSchema != nil {
		spec.OutputsSchema = append(json.RawMessage{}, spec.OutputsSchema...)
	}
	return spec
}

func cloneActionDefaultPorts(input ActionDefaultPorts) ActionDefaultPorts {
	return ActionDefaultPorts{
		Inputs:  append([]ActionPortSpec{}, input.Inputs...),
		Outputs: append([]ActionPortSpec{}, input.Outputs...),
	}
}

func normalizeActionDefaultPorts(input ActionDefaultPorts) ActionDefaultPorts {
	return ActionDefaultPorts{
		Inputs:  normalizeActionPorts(input.Inputs),
		Outputs: normalizeActionPorts(input.Outputs),
	}
}

func normalizeActionPorts(input []ActionPortSpec) []ActionPortSpec {
	out := make([]ActionPortSpec, 0, len(input))
	seen := map[string]struct{}{}
	for _, port := range input {
		id := strings.TrimSpace(port.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		label := strings.TrimSpace(port.Label)
		if label == "" {
			label = id
		}
		out = append(out, ActionPortSpec{ID: id, Label: label})
	}
	return out
}

func normalizeCatalogStringList(input []string) []string {
	out := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func defaultActionCapabilities(spec ActionSpec) []string {
	capabilities := []string{"structured_io", "variables"}
	switch strings.TrimSpace(spec.NodeType) {
	case "manual_approval":
		capabilities = append(capabilities, "approval", "branching")
	case "condition":
		capabilities = append(capabilities, "branching")
	case "subflow":
		capabilities = append(capabilities, "subflow")
	default:
		capabilities = append(capabilities, "targets", "timeout", "retries")
		if spec.Risk == "high" || spec.Risk == "medium" {
			capabilities = append(capabilities, "failure_path")
		}
	}
	return normalizeCatalogStringList(capabilities)
}

func defaultActionPorts(spec ActionSpec) ActionDefaultPorts {
	inputs := []ActionPortSpec{{ID: "in", Label: "输入"}}
	var outputs []ActionPortSpec
	switch strings.TrimSpace(spec.NodeType) {
	case "condition":
		outputs = []ActionPortSpec{{ID: "if", Label: "IF"}, {ID: "else", Label: "ELSE"}}
	case "manual_approval":
		outputs = []ActionPortSpec{{ID: "approved", Label: "通过"}, {ID: "rejected", Label: "拒绝"}}
	default:
		switch strings.TrimSpace(spec.Action) {
		case "wait.event":
			outputs = []ActionPortSpec{{ID: "next", Label: "下一步"}, {ID: "timeout", Label: "超时"}}
		case "notify.send":
			outputs = []ActionPortSpec{{ID: "next", Label: "下一步"}}
		default:
			outputs = []ActionPortSpec{{ID: "next", Label: "下一步"}, {ID: "failure", Label: "失败"}}
		}
	}
	return ActionDefaultPorts{Inputs: inputs, Outputs: outputs}
}

func cloneActionExamples(input []ActionExample) []ActionExample {
	if len(input) == 0 {
		return nil
	}
	out := make([]ActionExample, len(input))
	for i, item := range input {
		out[i] = item
		out[i].Args = cloneAnyMap(item.Args)
	}
	return out
}

func cloneActionIOExamples(input []ActionIOExample) []ActionIOExample {
	if len(input) == 0 {
		return nil
	}
	out := make([]ActionIOExample, len(input))
	for i, item := range input {
		out[i] = item
		out[i].Values = cloneAnyMap(item.Values)
	}
	return out
}

func hasNonEmptyArg(args map[string]any, key string) bool {
	if len(args) == 0 {
		return false
	}
	value, ok := args[key]
	if !ok || value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func actionArgsSchema(properties map[string]any, required []string) json.RawMessage {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func actionOutputsSchema(outputs []OutputSpec) json.RawMessage {
	properties := map[string]any{}
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			continue
		}
		property := map[string]any{}
		if typ := jsonSchemaScalarType(output.Type); typ != "" {
			property["type"] = typ
		}
		if strings.TrimSpace(output.Description) != "" {
			property["description"] = output.Description
		}
		properties[name] = property
	}
	raw, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties":           properties,
	})
	return raw
}

func jsonSchemaScalarType(typ string) string {
	switch strings.TrimSpace(typ) {
	case "string", "number", "integer", "boolean", "object", "array":
		return strings.TrimSpace(typ)
	default:
		return ""
	}
}

func inputExamplesFromActionExamples(examples []ActionExample) []ActionIOExample {
	out := make([]ActionIOExample, 0, len(examples))
	for _, example := range examples {
		if len(example.Args) == 0 {
			continue
		}
		out = append(out, ActionIOExample{
			Title:       example.Title,
			Description: example.Description,
			Values:      cloneAnyMap(example.Args),
		})
	}
	return out
}

func outputExamplesFromSpecs(outputs []OutputSpec) []ActionIOExample {
	values := map[string]any{}
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			continue
		}
		values[name] = outputExampleValue(name, output.Type)
	}
	if len(values) == 0 {
		return nil
	}
	return []ActionIOExample{{
		Title:  "Example output",
		Values: values,
	}}
}

func outputExampleValue(name, typ string) any {
	switch strings.TrimSpace(typ) {
	case "boolean":
		return true
	case "integer":
		return 0
	case "number":
		return 0
	case "array":
		return []any{}
	case "object":
		if name == "vars" {
			return map[string]any{"KEY": "value"}
		}
		return map[string]any{}
	case "string":
		fallthrough
	default:
		switch name {
		case "stderr":
			return ""
		case "stdout":
			return "sample stdout"
		case "decision":
			return "approved"
		case "run_id":
			return "run_123"
		default:
			return "value"
		}
	}
}

func envStringSchema(title string) map[string]any {
	return map[string]any{
		"type":  "string",
		"title": title,
	}
}

func envObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"title":                "Environment",
		"additionalProperties": map[string]any{"type": "string"},
	}
}

func boolSchema(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}

func commandOutputs() []OutputSpec {
	return []OutputSpec{
		{Name: "stdout", Type: "string", Description: "Captured standard output."},
		{Name: "stderr", Type: "string", Description: "Captured standard error."},
		{Name: "vars", Type: "object", Description: "Exported variables when export_vars is enabled."},
	}
}
