package service

import (
	"context"
	"fmt"
	"strings"

	"runner/server/store/agentstore"
	"runner/workflow"
)

type Preprocessor struct {
	scripts        *ScriptService
	agents         *AgentService
	allowedActions map[string]struct{}
}

func NewPreprocessor(scripts *ScriptService, agents *AgentService, allowedActions []string) *Preprocessor {
	whitelist := map[string]struct{}{}
	for _, action := range allowedActions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		whitelist[action] = struct{}{}
	}
	return &Preprocessor{
		scripts:        scripts,
		agents:         agents,
		allowedActions: whitelist,
	}
}

func (p *Preprocessor) Process(ctx context.Context, wf *workflow.Workflow) error {
	if wf == nil {
		return fmt.Errorf("%w: empty workflow", ErrInvalid)
	}
	if err := p.validateActions(wf); err != nil {
		return err
	}
	if err := p.resolveScriptRefs(ctx, wf); err != nil {
		return err
	}
	if err := p.resolveAgentAddress(ctx, wf); err != nil {
		return err
	}
	return nil
}

func (p *Preprocessor) validateActions(wf *workflow.Workflow) error {
	if len(p.allowedActions) == 0 {
		return nil
	}
	for _, step := range wf.Steps {
		if _, ok := p.allowedActions[strings.TrimSpace(step.Action)]; !ok {
			return fmt.Errorf("%w: action %q is not allowed", ErrInvalid, step.Action)
		}
	}
	for _, handler := range wf.Handlers {
		if _, ok := p.allowedActions[strings.TrimSpace(handler.Action)]; !ok {
			return fmt.Errorf("%w: handler action %q is not allowed", ErrInvalid, handler.Action)
		}
	}
	return nil
}

func (p *Preprocessor) resolveScriptRefs(ctx context.Context, wf *workflow.Workflow) error {
	for i := range wf.Steps {
		step := &wf.Steps[i]
		action := strings.TrimSpace(step.Action)
		if action != "script.shell" && action != "script.python" {
			continue
		}
		if step.Args == nil {
			step.Args = map[string]any{}
		}
		if step.Args["script"] != nil && step.Args["script_ref"] != nil {
			return fmt.Errorf("%w: step %q cannot use script and script_ref together", ErrInvalid, step.Name)
		}

		refRaw, hasRef := step.Args["script_ref"]
		if !hasRef || refRaw == nil {
			continue
		}
		if p.scripts == nil {
			return fmt.Errorf("%w: script service is not configured", ErrUnavailable)
		}
		ref := strings.TrimSpace(fmt.Sprint(refRaw))
		if ref == "" {
			return fmt.Errorf("%w: step %q script_ref is empty", ErrInvalid, step.Name)
		}
		record, err := p.scripts.Get(ctx, ref)
		if err != nil {
			if err == ErrNotFound {
				return fmt.Errorf("%w: script %q not found", ErrNotFound, ref)
			}
			return err
		}
		switch action {
		case "script.shell":
			if strings.TrimSpace(record.Language) != "shell" {
				return fmt.Errorf("%w: script %q language mismatch for action %s", ErrInvalid, ref, action)
			}
		case "script.python":
			if strings.TrimSpace(record.Language) != "python" {
				return fmt.Errorf("%w: script %q language mismatch for action %s", ErrInvalid, ref, action)
			}
		}
		step.Args["script"] = record.Content
		delete(step.Args, "script_ref")
	}
	return nil
}

func (p *Preprocessor) resolveAgentAddress(ctx context.Context, wf *workflow.Workflow) error {
	if p.agents == nil {
		return nil
	}
	resolvedAgentByHost := map[string]*agentstore.AgentRecord{}
	for hostName, host := range wf.Inventory.Hosts {
		address := strings.TrimSpace(host.Address)
		if address == "" {
			address = hostName
		}
		if !strings.HasPrefix(strings.ToLower(address), "agent://") {
			continue
		}
		record, err := p.agents.Resolve(ctx, address)
		if err != nil {
			return err
		}
		host.Address = record.Address
		if host.Vars == nil {
			host.Vars = map[string]any{}
		}
		if strings.TrimSpace(record.Token) != "" {
			host.Vars["RUNNER_AGENT_TOKEN"] = record.Token
		}
		host.Vars["RUNNER_AGENT_HEARTBEAT"] = true
		wf.Inventory.Hosts[hostName] = host
		resolvedAgentByHost[hostName] = record
	}
	for _, step := range wf.Steps {
		targetHosts := resolveTargetHostNames(step, wf.Inventory)
		for _, hostName := range targetHosts {
			record, ok := resolvedAgentByHost[hostName]
			if !ok {
				continue
			}
			if !p.agents.SupportsAction(record, step.Action) {
				return fmt.Errorf("%w: agent %q does not support action %q", ErrInvalid, record.ID, step.Action)
			}
		}
	}
	return nil
}

func resolveTargetHostNames(step workflow.Step, inv workflow.Inventory) []string {
	hostNames := map[string]struct{}{}
	if len(step.Targets) == 0 {
		for host := range inv.Hosts {
			hostNames[host] = struct{}{}
		}
	}
	for _, target := range step.Targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := inv.Hosts[target]; ok {
			hostNames[target] = struct{}{}
			continue
		}
		if group, ok := inv.Groups[target]; ok {
			for _, host := range group.Hosts {
				hostNames[host] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(hostNames))
	for host := range hostNames {
		out = append(out, host)
	}
	return out
}
