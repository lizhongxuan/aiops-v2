package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

const HostInternalDispatchReasonRuntimeControl = "runtime_control"

type HostInternalDispatchReason struct {
	Code string `json:"code"`
}

type StepToolRouter struct {
	RegisteredTools             []string                              `json:"registeredTools,omitempty"`
	ModelVisibleTools           []string                              `json:"modelVisibleTools,omitempty"`
	DispatchableTools           []string                              `json:"dispatchableTools,omitempty"`
	HiddenReasons               map[string][]string                   `json:"hiddenReasons,omitempty"`
	HostInternalDispatchReasons map[string]HostInternalDispatchReason `json:"hostInternalDispatchReasons,omitempty"`
	PolicyHash                  string                                `json:"policyHash,omitempty"`
	Fingerprint                 string                                `json:"fingerprint,omitempty"`
}

type StepToolRouterInput struct {
	Registered                  []string
	ModelVisible                []string
	Dispatchable                []string
	HiddenReasons               map[string][]string
	HostInternalDispatchReasons map[string]HostInternalDispatchReason
	PolicyHash                  string
	SourceFingerprint           string
}

func BuildStepToolRouter(input StepToolRouterInput) (StepToolRouter, error) {
	router := StepToolRouter{
		RegisteredTools:             uniqueSortedTraceStrings(input.Registered),
		ModelVisibleTools:           uniqueSortedTraceStrings(input.ModelVisible),
		DispatchableTools:           uniqueSortedTraceStrings(input.Dispatchable),
		HiddenReasons:               copyRuntimeToolHiddenReasons(input.HiddenReasons),
		HostInternalDispatchReasons: cloneHostInternalDispatchReasons(input.HostInternalDispatchReasons),
		PolicyHash:                  strings.TrimSpace(input.PolicyHash),
	}
	router.Fingerprint = agentassembly.StableHash("step-tool-router", map[string]any{
		"registered": router.RegisteredTools, "modelVisible": router.ModelVisibleTools,
		"dispatchable": router.DispatchableTools, "hiddenReasons": router.HiddenReasons,
		"hostInternalDispatchReasons": router.HostInternalDispatchReasons,
		"policyHash":                  router.PolicyHash, "sourceFingerprint": strings.TrimSpace(input.SourceFingerprint),
	})
	if err := router.Validate(); err != nil {
		return StepToolRouter{}, err
	}
	return router, nil
}

func (router StepToolRouter) Validate() error {
	if strings.TrimSpace(router.Fingerprint) == "" {
		return fmt.Errorf("step tool router fingerprint is required")
	}
	registered := stepToolNameSet(router.RegisteredTools)
	visible := stepToolNameSet(router.ModelVisibleTools)
	dispatchable := stepToolNameSet(router.DispatchableTools)
	if err := validateStepToolSafeNameCollisions(router.RegisteredTools); err != nil {
		return err
	}
	for name := range visible {
		if _, ok := registered[name]; !ok {
			return fmt.Errorf("model visible tool %q is not registered in step", name)
		}
		if _, ok := dispatchable[name]; !ok {
			return fmt.Errorf("model visible tool %q is not dispatchable in step", name)
		}
	}
	for name := range dispatchable {
		if _, ok := registered[name]; !ok {
			return fmt.Errorf("dispatchable tool %q is not registered in step", name)
		}
		if _, ok := visible[name]; ok {
			continue
		}
		reason, ok := stepToolInternalReason(router.HostInternalDispatchReasons, name)
		if !ok || strings.TrimSpace(reason.Code) != HostInternalDispatchReasonRuntimeControl {
			return fmt.Errorf("dispatchable tool %q is not model visible and lacks typed host-internal reason", name)
		}
	}
	return nil
}

func (router StepToolRouter) AllowsModelDispatch(name string) bool {
	return stepToolListContains(router.ModelVisibleTools, name) && stepToolListContains(router.DispatchableTools, name)
}

func (router StepToolRouter) AllowsHostInternalDispatch(name string) bool {
	if !stepToolListContains(router.DispatchableTools, name) || stepToolListContains(router.ModelVisibleTools, name) {
		return false
	}
	reason, ok := stepToolInternalReason(router.HostInternalDispatchReasons, name)
	return ok && strings.TrimSpace(reason.Code) == HostInternalDispatchReasonRuntimeControl
}

func (router StepToolRouter) hiddenReason(name string) string {
	for candidate, reasons := range router.HiddenReasons {
		if !stepToolNamesEqual(candidate, name) || len(reasons) == 0 {
			continue
		}
		if reason := strings.TrimSpace(reasons[0]); reason != "" {
			return reason
		}
	}
	return ""
}

func stepToolNameSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if key := stepToolNameKey(name); key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func stepToolListContains(names []string, candidate string) bool {
	for _, name := range names {
		if stepToolNamesEqual(name, candidate) {
			return true
		}
	}
	return false
}

func stepToolNamesEqual(left, right string) bool {
	return stepToolNameKey(left) != "" && stepToolNameKey(left) == stepToolNameKey(right)
}

func stepToolNameKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToLower(tooling.ProviderSafeToolName(name))
}

func validateStepToolSafeNameCollisions(names []string) error {
	seen := map[string]string{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		key := stepToolNameKey(name)
		if key == "" {
			continue
		}
		if previous, ok := seen[key]; ok && previous != name {
			return fmt.Errorf("registered step tools %q and %q collide as provider name %q", previous, name, key)
		}
		seen[key] = name
	}
	return nil
}

func stepToolInternalReason(reasons map[string]HostInternalDispatchReason, name string) (HostInternalDispatchReason, bool) {
	for candidate, reason := range reasons {
		if stepToolNamesEqual(candidate, name) {
			return reason, true
		}
	}
	return HostInternalDispatchReason{}, false
}

func cloneHostInternalDispatchReasons(input map[string]HostInternalDispatchReason) map[string]HostInternalDispatchReason {
	if len(input) == 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]HostInternalDispatchReason, len(keys))
	for _, key := range keys {
		name := strings.TrimSpace(key)
		if name != "" {
			out[name] = HostInternalDispatchReason{Code: strings.TrimSpace(input[key].Code)}
		}
	}
	return out
}

func cloneStepToolRouter(router StepToolRouter) StepToolRouter {
	return StepToolRouter{
		RegisteredTools:             append([]string(nil), router.RegisteredTools...),
		ModelVisibleTools:           append([]string(nil), router.ModelVisibleTools...),
		DispatchableTools:           append([]string(nil), router.DispatchableTools...),
		HiddenReasons:               copyRuntimeToolHiddenReasons(router.HiddenReasons),
		HostInternalDispatchReasons: cloneHostInternalDispatchReasons(router.HostInternalDispatchReasons),
		PolicyHash:                  strings.TrimSpace(router.PolicyHash),
		Fingerprint:                 strings.TrimSpace(router.Fingerprint),
	}
}

func modelVisibleToolsForStep(registered []promptcompiler.Tool, router StepToolRouter) []promptcompiler.Tool {
	out := make([]promptcompiler.Tool, 0, len(router.ModelVisibleTools))
	for _, toolDef := range registered {
		if toolDef == nil || !stepToolListContains(router.ModelVisibleTools, toolDef.Metadata().Name) {
			continue
		}
		out = append(out, toolDef)
	}
	return out
}
