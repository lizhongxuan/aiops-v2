package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/specialinputmemory"
	"aiops-v2/internal/terminalpolicy"
	"aiops-v2/internal/tooling"
)

type ExecutionScopeGuardConfig struct {
	Enabled          bool
	Grants           []specialinputmemory.ExecutionScopeGrant
	ValidationHashes map[string]string
}

func (d *ToolDispatcher) WithExecutionScopeGuard(config ExecutionScopeGuardConfig) *ToolDispatcher {
	d.executionScopeGuard = normalizeExecutionScopeGuardConfig(config)
	return d
}

func normalizeExecutionScopeGuardConfig(config ExecutionScopeGuardConfig) ExecutionScopeGuardConfig {
	config.Grants = append([]specialinputmemory.ExecutionScopeGrant(nil), config.Grants...)
	if config.ValidationHashes != nil {
		cp := map[string]string{}
		for key, value := range config.ValidationHashes {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" {
				continue
			}
			cp[key] = value
		}
		config.ValidationHashes = cp
	}
	return config
}

func executionScopeGuardConfigFromSnapshot(snapshot *TurnSnapshot) ExecutionScopeGuardConfig {
	if snapshot == nil || snapshot.SpecialInputReadPlan == nil || snapshot.SpecialInputReadPlan.ActiveExecutionScope == nil {
		return ExecutionScopeGuardConfig{}
	}
	grant := *snapshot.SpecialInputReadPlan.ActiveExecutionScope
	if grant.Status == "" {
		grant.Status = specialinputmemory.GrantStatusActive
	}
	return normalizeExecutionScopeGuardConfig(ExecutionScopeGuardConfig{
		Enabled: true,
		Grants:  []specialinputmemory.ExecutionScopeGrant{grant},
	})
}

func (d *ToolDispatcher) checkExecutionScopeGuard(tc ToolCall, meta tooling.ToolMetadata) (string, bool) {
	if d == nil || !d.executionScopeGuard.Enabled {
		return "", false
	}
	args := executionScopeGuardArgs(tc.Arguments)
	action := executionScopeGuardAction(meta, tc.Arguments)
	requiresHostScope := args.hostID != "" || executionScopeToolTargetsHost(meta)
	if !requiresHostScope {
		return "", false
	}
	grant, ok := d.matchExecutionScopeGrant(args.hostID)
	if !ok {
		return "execution_scope_guard: no active execution scope grant for host target", true
	}
	if args.hostID != "" && !executionScopeSameHost(args.hostID, grant) {
		return fmt.Sprintf("execution_scope_guard: requested host %s differs from granted host %s", args.hostID, grant.ResourceID), true
	}
	if expected, ok := d.executionScopeGuard.ValidationHashes[grant.ResourceID]; ok {
		if strings.TrimSpace(grant.ValidationHash) != "" && strings.TrimSpace(grant.ValidationHash) != expected {
			return "execution_scope_guard: validation hash drift for granted host", true
		}
	}
	if expected, ok := d.executionScopeGuard.ValidationHashes[grant.CanonicalKey]; ok {
		if strings.TrimSpace(grant.ValidationHash) != "" && strings.TrimSpace(grant.ValidationHash) != expected {
			return "execution_scope_guard: validation hash drift for granted host", true
		}
	}
	if !grant.Allows(action) {
		return fmt.Sprintf("execution_scope_guard: action %s is not allowed by active grant", action), true
	}
	return "", false
}

func (d *ToolDispatcher) matchExecutionScopeGrant(hostID string) (specialinputmemory.ExecutionScopeGrant, bool) {
	now := time.Now()
	grants := specialinputmemory.ActiveGrants(d.executionScopeGuard.Grants)
	for _, grant := range grants {
		if grant.ResourceKind != "" && grant.ResourceKind != specialinputmemory.ResourceKindHost {
			continue
		}
		if grant.Expired(now) {
			continue
		}
		if hostID == "" || executionScopeSameHost(hostID, grant) {
			return grant, true
		}
	}
	return specialinputmemory.ExecutionScopeGrant{}, false
}

type executionScopeToolArgs struct {
	hostID string
}

func executionScopeGuardArgs(raw json.RawMessage) executionScopeToolArgs {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return executionScopeToolArgs{}
	}
	return executionScopeToolArgs{
		hostID: firstExecutionScopeString(payload, "hostId", "host_id", "targetHostId", "target_host_id", "target", "resourceId", "resource_id"),
	}
}

func firstExecutionScopeString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case fmt.Stringer:
			if strings.TrimSpace(typed.String()) != "" {
				return strings.TrimSpace(typed.String())
			}
		}
	}
	return ""
}

func executionScopeSameHost(hostID string, grant specialinputmemory.ExecutionScopeGrant) bool {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return false
	}
	return strings.EqualFold(hostID, strings.TrimSpace(grant.ResourceID)) ||
		strings.EqualFold("host:"+hostID, strings.TrimSpace(grant.CanonicalKey))
}

func executionScopeToolTargetsHost(meta tooling.ToolMetadata) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(meta.Name)), "host.") {
		return true
	}
	discovery := meta.EffectiveDiscovery()
	for _, value := range append(append([]string(nil), discovery.TargetKinds...), discovery.ResourceTypes...) {
		if strings.EqualFold(strings.TrimSpace(value), specialinputmemory.ResourceKindHost) {
			return true
		}
	}
	return false
}

func executionScopeGuardAction(meta tooling.ToolMetadata, rawArgs json.RawMessage) string {
	if executionScopeToolIsTerminalCommand(meta) {
		if command, args, ok := executionScopeTerminalCommand(rawArgs); ok && terminalpolicy.IsReadOnlyCommand(command, args) {
			return specialinputmemory.ActionExecLowRisk
		}
	}
	governance := meta.EffectiveGovernance(4096)
	if governance.Mutating || meta.Layer == tooling.ToolLayerMutation {
		return specialinputmemory.ActionMutate
	}
	if governance.RequiresApproval {
		return specialinputmemory.ActionMutate
	}
	discovery := meta.EffectiveDiscovery()
	for _, op := range discovery.OperationKinds {
		switch strings.ToLower(strings.TrimSpace(op)) {
		case "write", "delete", "modify", "create", "update", "restart", "stop", "start":
			return specialinputmemory.ActionMutate
		case "run", "execute", "exec", "command":
			return specialinputmemory.ActionExecLowRisk
		case "inspect", "observe", "diagnose":
			return specialinputmemory.ActionInspect
		case "read", "list", "search", "query", "summarize":
			return specialinputmemory.ActionRead
		}
	}
	name := strings.ToLower(strings.TrimSpace(meta.Name))
	switch {
	case strings.Contains(name, "exec"), strings.Contains(name, "command"), strings.Contains(name, "shell"):
		return specialinputmemory.ActionExecLowRisk
	case strings.Contains(name, "inspect"), strings.Contains(name, "status"), strings.Contains(name, "metric"):
		return specialinputmemory.ActionInspect
	default:
		return specialinputmemory.ActionExecLowRisk
	}
}

func executionScopeToolIsTerminalCommand(meta tooling.ToolMetadata) bool {
	names := append([]string{meta.Name}, meta.Aliases...)
	for _, name := range names {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "exec_command", "terminal_command", "shell_command":
			return true
		}
	}
	return false
}

func executionScopeTerminalCommand(raw json.RawMessage) (string, []string, bool) {
	var payload struct {
		Command string   `json:"command"`
		Cmd     string   `json:"cmd"`
		Args    []string `json:"args"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return "", nil, false
	}
	command := strings.TrimSpace(payload.Command)
	args := append([]string(nil), payload.Args...)
	if command == "" {
		command = strings.TrimSpace(payload.Cmd)
	}
	if command == "" {
		return "", nil, false
	}
	if len(args) == 0 {
		parsedCommand, parsedArgs, ok := terminalpolicy.SplitCommandLine(command)
		if ok {
			command = parsedCommand
			args = parsedArgs
		}
	}
	return command, args, true
}
