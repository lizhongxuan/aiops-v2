package terminalpolicy

import (
	"fmt"
	"path/filepath"
	"strings"
)

const SchemaVersion = "aiops.terminal_policy/v1"

type RuleEffect string

const (
	RuleEffectAllow            RuleEffect = "allow"
	RuleEffectDeny             RuleEffect = "deny"
	RuleEffectApprovalRequired RuleEffect = "approval_required"
)

type PolicyAction string

const (
	PolicyActionAllow        PolicyAction = "allow"
	PolicyActionDeny         PolicyAction = "deny"
	PolicyActionNeedApproval PolicyAction = "need_approval"
	PolicyActionDefault      PolicyAction = "default"
)

type PolicySource string

const (
	PolicySourceHardDeny PolicySource = "hard_deny"
	PolicySourceUser     PolicySource = "user"
	PolicySourceBuiltin  PolicySource = "builtin"
	PolicySourceDefault  PolicySource = "default"
)

type CommandRequest struct {
	Command string
	Args    []string
	HostID  string
	Agent   string
}

type Decision struct {
	Action PolicyAction `json:"action"`
	Source PolicySource `json:"source"`
	RuleID string       `json:"ruleId,omitempty"`
	Reason string       `json:"reason,omitempty"`
}

type Config struct {
	SchemaVersion string `json:"schemaVersion"`
	Rules         []Rule `json:"rules"`
}

type Rule struct {
	ID         string     `json:"id"`
	Effect     RuleEffect `json:"effect"`
	Command    string     `json:"command"`
	Args       []string   `json:"args,omitempty"`
	ArgsPrefix []string   `json:"argsPrefix,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Disabled   bool       `json:"disabled,omitempty"`
}

type Provider interface {
	Evaluate(req CommandRequest) Decision
}

type Engine struct {
	config Config
}

func NewEngine(config Config) *Engine {
	config = normalizeConfig(config)
	return &Engine{config: config}
}

func DefaultConfig() Config {
	return Config{SchemaVersion: SchemaVersion, Rules: []Rule{}}
}

func (e *Engine) Config() Config {
	if e == nil {
		return DefaultConfig()
	}
	return cloneConfig(e.config)
}

func (e *Engine) Evaluate(req CommandRequest) Decision {
	command := strings.TrimSpace(req.Command)
	args := append([]string(nil), req.Args...)
	if IsHardDeniedCommand(command, args) {
		return Decision{
			Action: PolicyActionDeny,
			Source: PolicySourceHardDeny,
			Reason: "command is blocked by hard-deny safety policy",
		}
	}
	config := DefaultConfig()
	if e != nil {
		config = e.config
	}
	for _, effect := range []RuleEffect{RuleEffectDeny, RuleEffectApprovalRequired, RuleEffectAllow} {
		for _, rule := range config.Rules {
			if rule.Disabled || normalizeEffect(rule.Effect) != effect || !ruleMatches(rule, command, args) {
				continue
			}
			action := PolicyActionAllow
			switch effect {
			case RuleEffectDeny:
				action = PolicyActionDeny
			case RuleEffectApprovalRequired:
				action = PolicyActionNeedApproval
			}
			return Decision{
				Action: action,
				Source: PolicySourceUser,
				RuleID: strings.TrimSpace(rule.ID),
				Reason: strings.TrimSpace(rule.Reason),
			}
		}
	}
	if IsAllowedReadOnlyTerminal(command, args) || IsAllowedHostInspectionTerminal(command, args) {
		return Decision{Action: PolicyActionAllow, Source: PolicySourceBuiltin, Reason: "builtin read-only terminal policy"}
	}
	return Decision{Action: PolicyActionDefault, Source: PolicySourceDefault, Reason: "no terminal policy rule matched"}
}

func ValidateConfig(config Config) error {
	config = normalizeConfig(config)
	seen := map[string]bool{}
	for i, rule := range config.Rules {
		if rule.Disabled {
			continue
		}
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			return fmt.Errorf("rule[%d].id is required", i)
		}
		if seen[id] {
			return fmt.Errorf("rule %q is duplicated", id)
		}
		seen[id] = true
		effect := normalizeEffect(rule.Effect)
		switch effect {
		case RuleEffectAllow, RuleEffectDeny, RuleEffectApprovalRequired:
		default:
			return fmt.Errorf("rule %q has unsupported effect %q", id, rule.Effect)
		}
		if strings.TrimSpace(rule.Command) == "" {
			return fmt.Errorf("rule %q command is required", id)
		}
		if len(rule.Args) > 0 && len(rule.ArgsPrefix) > 0 {
			return fmt.Errorf("rule %q cannot set both args and argsPrefix", id)
		}
		if effect == RuleEffectAllow && IsHardDeniedCommand(rule.Command, firstNonEmptyArgs(rule.Args, rule.ArgsPrefix)) {
			return fmt.Errorf("rule %q attempts to allow a hard-denied command", id)
		}
		if !safeRuleTokens(rule.Command, rule.Args, rule.ArgsPrefix) {
			return fmt.Errorf("rule %q contains unsafe command tokens", id)
		}
	}
	return nil
}

func IsHardDeniedCommand(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if wrappedCommand, wrappedArgs, ok := unwrapReadOnlyShell(base, args); ok {
		return IsHardDeniedCommand(wrappedCommand, wrappedArgs)
	}
	switch base {
	case "rm", "reboot", "shutdown", "halt", "poweroff", "mkfs", "dd", "chmod", "chown":
		return true
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\n\r`$<>;|") {
			return true
		}
	}
	return false
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.SchemaVersion) == "" {
		config.SchemaVersion = SchemaVersion
	}
	config.Rules = append([]Rule(nil), config.Rules...)
	for i := range config.Rules {
		config.Rules[i].ID = strings.TrimSpace(config.Rules[i].ID)
		config.Rules[i].Effect = normalizeEffect(config.Rules[i].Effect)
		config.Rules[i].Command = strings.TrimSpace(config.Rules[i].Command)
		config.Rules[i].Args = cloneTrimmedStrings(config.Rules[i].Args)
		config.Rules[i].ArgsPrefix = cloneTrimmedStrings(config.Rules[i].ArgsPrefix)
		config.Rules[i].Reason = strings.TrimSpace(config.Rules[i].Reason)
	}
	return config
}

func normalizeEffect(effect RuleEffect) RuleEffect {
	return RuleEffect(strings.ToLower(strings.TrimSpace(string(effect))))
}

func ruleMatches(rule Rule, command string, args []string) bool {
	if filepath.Base(strings.TrimSpace(command)) != filepath.Base(strings.TrimSpace(rule.Command)) {
		return false
	}
	if len(rule.Args) > 0 {
		return sameStrings(args, rule.Args)
	}
	if len(rule.ArgsPrefix) > 0 {
		return hasPrefixStrings(args, rule.ArgsPrefix)
	}
	return true
}

func safeRuleTokens(command string, args []string, argsPrefix []string) bool {
	if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\x00\n\r`$<>;|") {
		return false
	}
	for _, value := range append(append([]string{}, args...), argsPrefix...) {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\n\r`$<>;|") {
			return false
		}
	}
	return true
}

func firstNonEmptyArgs(primary []string, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func cloneConfig(config Config) Config {
	config = normalizeConfig(config)
	config.Rules = append([]Rule(nil), config.Rules...)
	return config
}

func cloneTrimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.TrimSpace(left[i]) != strings.TrimSpace(right[i]) {
			return false
		}
	}
	return true
}

func hasPrefixStrings(values []string, prefix []string) bool {
	if len(values) < len(prefix) {
		return false
	}
	for i := range prefix {
		if strings.TrimSpace(values[i]) != strings.TrimSpace(prefix[i]) {
			return false
		}
	}
	return true
}
