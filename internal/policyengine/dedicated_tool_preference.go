package policyengine

import (
	"encoding/json"
	"reflect"
	"strings"

	"aiops-v2/internal/terminalpolicy"
)

// DedicatedToolPreferenceAction is the policy result for shell fallback when
// visible dedicated tools may cover the same generic capability.
type DedicatedToolPreferenceAction string

const (
	DedicatedToolPreferenceAllow                     DedicatedToolPreferenceAction = "allow"
	DedicatedToolPreferenceRequireReason             DedicatedToolPreferenceAction = "require_reason"
	DedicatedToolPreferenceRejectPreferDedicatedTool DedicatedToolPreferenceAction = "reject_prefer_dedicated_tool"
)

// DedicatedToolPreferenceDecision describes whether shell fallback may proceed.
type DedicatedToolPreferenceDecision struct {
	Action         DedicatedToolPreferenceAction `json:"action"`
	Reason         string                        `json:"reason,omitempty"`
	PreferredTools []string                      `json:"preferredTools,omitempty"`
}

type shellFallbackCapability struct {
	readOnly   bool
	mutating   bool
	capability string
	resources  map[string]struct{}
	operations map[string]struct{}
}

type dedicatedToolDiscoveryView struct {
	name       string
	mutating   bool
	capability string
	resources  map[string]struct{}
	operations map[string]struct{}
}

// EvaluateDedicatedToolPreference prefers visible dedicated tools over raw
// shell fallback using only generic discovery metadata and mutating/read-only
// traits. The generic parameter lets policyengine consume current
// tooling.ToolMetadata values and future metadata structs that add Discovery
// without depending on integration packages.
func EvaluateDedicatedToolPreference[T any](shellToolName string, arguments json.RawMessage, visibleTools []T, fallbackReason string) DedicatedToolPreferenceDecision {
	if !isTerminalCommandTool(shellToolName) {
		return DedicatedToolPreferenceDecision{Action: DedicatedToolPreferenceAllow}
	}
	shellCapability, ok := classifyShellFallback(arguments)
	if !ok {
		return DedicatedToolPreferenceDecision{Action: DedicatedToolPreferenceAllow}
	}

	var preferred []string
	for _, tool := range visibleTools {
		discovery, ok := discoveryViewFromToolMetadata(tool)
		if !ok {
			continue
		}
		if sameToolName(discovery.name, shellToolName) {
			continue
		}
		if dedicatedToolMatchesShellFallback(discovery, shellCapability) {
			preferred = append(preferred, discovery.name)
		}
	}
	if len(preferred) == 0 {
		return DedicatedToolPreferenceDecision{Action: DedicatedToolPreferenceAllow}
	}
	if shellCapability.mutating {
		return DedicatedToolPreferenceDecision{
			Action:         DedicatedToolPreferenceRejectPreferDedicatedTool,
			Reason:         "visible dedicated tool covers the same mutating capability",
			PreferredTools: preferred,
		}
	}
	if strings.TrimSpace(fallbackReason) == "" {
		return DedicatedToolPreferenceDecision{
			Action:         DedicatedToolPreferenceRequireReason,
			Reason:         "visible dedicated tool covers the same read-only capability",
			PreferredTools: preferred,
		}
	}
	return DedicatedToolPreferenceDecision{
		Action:         DedicatedToolPreferenceAllow,
		PreferredTools: preferred,
	}
}

func sameToolName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func classifyShellFallback(arguments json.RawMessage) (shellFallbackCapability, bool) {
	req, ok := terminalCommandRequestFromArgs(arguments)
	if !ok {
		return shellFallbackCapability{}, false
	}
	command := strings.ToLower(strings.TrimSpace(req.command))
	readOnly := terminalpolicy.IsReadOnlyCommand(req.command, req.args)
	capability := "write"
	if readOnly {
		capability = "read"
	}
	result := shellFallbackCapability{
		readOnly:   readOnly,
		mutating:   !readOnly,
		capability: capability,
		resources:  make(map[string]struct{}),
		operations: make(map[string]struct{}),
	}
	switch command {
	case "curl", "wget":
		result.add("network", "read")
		if !readOnly {
			result.operations = setFromStrings("write")
		}
	case "ls":
		result.add("file", "list")
	case "find":
		result.add("file", "search")
	case "cat", "head", "tail", "stat", "file", "wc", "pwd":
		result.add("file", "read")
	case "grep", "rg":
		result.add("file", "search")
	case "ps", "pgrep", "top":
		result.add("process", "list")
	case "df", "du", "free", "uptime", "date", "nproc", "lscpu", "sw_vers":
		result.add("system", "read")
	case "ifconfig", "ip", "netstat", "ss":
		result.add("network", "read")
	case "touch", "mkdir", "cp", "mv", "tee":
		result.add("file", "write")
	case "rm", "rmdir":
		result.add("file", "delete")
	case "chmod", "chown":
		result.add("file", "update")
	default:
		if readOnly {
			result.add("system", "read")
		} else {
			result.add("system", "execute")
			result.capability = "execute"
		}
	}
	return result, true
}

func (c shellFallbackCapability) add(resource, operation string) {
	c.resources[resource] = struct{}{}
	c.operations[operation] = struct{}{}
}

func discoveryViewFromToolMetadata(tool any) (dedicatedToolDiscoveryView, bool) {
	value := reflect.Indirect(reflect.ValueOf(tool))
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return dedicatedToolDiscoveryView{}, false
	}
	name := stringField(value, "Name")
	mutating := boolField(value, "Mutating")
	discovery := fieldByName(value, "Discovery")
	if !discovery.IsValid() {
		return dedicatedToolDiscoveryView{}, false
	}
	discovery = reflect.Indirect(discovery)
	if !discovery.IsValid() || discovery.Kind() != reflect.Struct {
		return dedicatedToolDiscoveryView{}, false
	}
	view := dedicatedToolDiscoveryView{
		name:       name,
		mutating:   mutating,
		capability: normalizeDiscoveryToken(stringField(discovery, "CapabilityKind")),
		resources:  setFromStrings(stringSliceField(discovery, "ResourceTypes")...),
		operations: setFromStrings(stringSliceField(discovery, "OperationKinds")...),
	}
	if view.capability == "write" || view.capability == "execute" {
		view.mutating = true
	}
	if view.name == "" || view.capability == "" || len(view.resources) == 0 || len(view.operations) == 0 {
		return dedicatedToolDiscoveryView{}, false
	}
	return view, true
}

func dedicatedToolMatchesShellFallback(tool dedicatedToolDiscoveryView, shell shellFallbackCapability) bool {
	if shell.readOnly && tool.mutating {
		return false
	}
	if shell.mutating && !tool.mutating {
		return false
	}
	if tool.capability != shell.capability {
		return false
	}
	return intersects(tool.resources, shell.resources) && intersects(tool.operations, shell.operations)
}

func fieldByName(value reflect.Value, name string) reflect.Value {
	if value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	field := value.FieldByName(name)
	if field.IsValid() {
		return field
	}
	for i := 0; i < value.NumField(); i++ {
		child := value.Field(i)
		if value.Type().Field(i).Anonymous {
			if found := fieldByName(reflect.Indirect(child), name); found.IsValid() {
				return found
			}
		}
	}
	return reflect.Value{}
}

func stringField(value reflect.Value, name string) string {
	field := fieldByName(value, name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return strings.TrimSpace(field.String())
}

func boolField(value reflect.Value, name string) bool {
	field := fieldByName(value, name)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}
	return field.Bool()
}

func stringSliceField(value reflect.Value, name string) []string {
	field := fieldByName(value, name)
	if !field.IsValid() || field.Kind() != reflect.Slice || field.Type().Elem().Kind() != reflect.String {
		return nil
	}
	out := make([]string, 0, field.Len())
	for i := 0; i < field.Len(); i++ {
		out = append(out, field.Index(i).String())
	}
	return out
}

func setFromStrings(values ...string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, value := range values {
		value = normalizeDiscoveryToken(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func normalizeDiscoveryToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func intersects(left, right map[string]struct{}) bool {
	for key := range left {
		if _, ok := right[key]; ok {
			return true
		}
	}
	return false
}
