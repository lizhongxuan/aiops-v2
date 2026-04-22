package permissions

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

// Action classifies the outcome of a permission decision.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
	ActionAsk   Action = "ask"
)

// Request captures the unified tool permission input.
type Request struct {
	Tool        tooling.ToolMetadata
	SessionType string
	Mode        string
	Arguments   json.RawMessage
}

// Decision is the outcome of a permission evaluation.
type Decision struct {
	Action Action
	Reason string
}

// RuleSource tracks where a permission rule originated from.
type RuleSource string

const (
	RuleSourceUserSettings    RuleSource = "userSettings"
	RuleSourceProjectSettings RuleSource = "projectSettings"
	RuleSourceLocalSettings   RuleSource = "localSettings"
	RuleSourceFlagSettings    RuleSource = "flagSettings"
	RuleSourcePolicySettings  RuleSource = "policySettings"
	RuleSourcePolicyRemote    RuleSource = "policyRemote"
	RuleSourcePolicyMachine   RuleSource = "policyMachine"
	RuleSourcePolicyManaged   RuleSource = "policyManaged"
	RuleSourcePolicyUser      RuleSource = "policyUser"
	RuleSourceCLIArg          RuleSource = "cliArg"
	RuleSourceCommand         RuleSource = "command"
	RuleSourceSession         RuleSource = "session"
)

// UpdateDestination identifies where permission changes should be persisted.
type UpdateDestination string

const (
	UpdateDestinationUserSettings    UpdateDestination = "userSettings"
	UpdateDestinationProjectSettings UpdateDestination = "projectSettings"
	UpdateDestinationLocalSettings   UpdateDestination = "localSettings"
	UpdateDestinationSession         UpdateDestination = "session"
	UpdateDestinationCLIArg          UpdateDestination = "cliArg"
)

// Matcher describes the conditions a rule must satisfy.
type Matcher struct {
	ToolNames     []string
	Sources       []tooling.ToolSource
	SessionTypes  []string
	Modes         []string
	Destinations  []string
	InputContains []string
}

// Matches reports whether the request satisfies the matcher.
func (m Matcher) Matches(req Request) bool {
	if len(m.ToolNames) > 0 && !matchesToolName(m.ToolNames, req.Tool) {
		return false
	}
	if len(m.Sources) > 0 && !matchesSource(m.Sources, req.Tool) {
		return false
	}
	if len(m.SessionTypes) > 0 && !matchesString(m.SessionTypes, req.SessionType) {
		return false
	}
	if len(m.Modes) > 0 && !matchesString(m.Modes, req.Mode) {
		return false
	}
	if len(m.Destinations) > 0 && !matchesDestinations(m.Destinations, req.Arguments) {
		return false
	}
	if len(m.InputContains) > 0 && !matchesInputContains(m.InputContains, req.Arguments) {
		return false
	}
	return true
}

// Rule binds a matcher to a permission action.
type Rule struct {
	Name    string
	Source  RuleSource
	Matcher Matcher
	Action  Action
	Reason  string
}

// Engine evaluates permission rules in registration order.
type Engine struct {
	mu    sync.RWMutex
	rules []Rule
}

// NewEngine creates a permission engine with a defensive copy of rules.
func NewEngine(rules []Rule) *Engine {
	return &Engine{rules: normalizeRules(rules)}
}

// Rules returns a defensive copy of the configured rules.
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneRules(e.rules)
}

// Decide evaluates the request using first-match-wins, defaulting to allow.
func (e *Engine) Decide(ctx context.Context, req Request) Decision {
	_ = ctx

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, rule := range e.rules {
		if !rule.Matcher.Matches(req) {
			continue
		}
		return Decision{Action: rule.Action, Reason: rule.Reason}
	}
	return Decision{Action: ActionAllow}
}

func cloneRules(rules []Rule) []Rule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]Rule, len(rules))
	for i, rule := range rules {
		out[i] = cloneRule(rule)
	}
	return out
}

func cloneRule(rule Rule) Rule {
	rule.Matcher = cloneMatcher(rule.Matcher)
	return rule
}

func cloneMatcher(m Matcher) Matcher {
	m.ToolNames = append([]string(nil), m.ToolNames...)
	m.Sources = append([]tooling.ToolSource(nil), m.Sources...)
	m.SessionTypes = append([]string(nil), m.SessionTypes...)
	m.Modes = append([]string(nil), m.Modes...)
	m.Destinations = append([]string(nil), m.Destinations...)
	m.InputContains = append([]string(nil), m.InputContains...)
	return m
}

func normalizeRules(rules []Rule) []Rule {
	out := cloneRules(rules)
	sort.SliceStable(out, func(i, j int) bool {
		return ruleSourceRank(out[i].Source) > ruleSourceRank(out[j].Source)
	})
	return out
}

func matchesToolName(toolNames []string, meta tooling.ToolMetadata) bool {
	if meta.Name == "" && len(meta.Aliases) == 0 {
		return false
	}
	for _, candidate := range toolNames {
		if meta.Name == candidate {
			return true
		}
		for _, alias := range meta.Aliases {
			if alias == candidate {
				return true
			}
		}
	}
	return false
}

func matchesSource(sources []tooling.ToolSource, meta tooling.ToolMetadata) bool {
	for _, source := range sources {
		if meta.HasSource(source) {
			return true
		}
	}
	return false
}

func matchesString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func matchesInputContains(parts []string, input json.RawMessage) bool {
	text := string(input)
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func matchesDestinations(expected []string, input json.RawMessage) bool {
	destinations := extractDestinations(input)
	if len(destinations) == 0 {
		return false
	}
	for _, want := range expected {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		matched := false
		for _, destination := range destinations {
			if destinationMatches(want, destination) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func destinationMatches(expected, actual string) bool {
	if strings.HasSuffix(expected, "*") {
		return strings.HasPrefix(actual, strings.TrimSuffix(expected, "*"))
	}
	return actual == expected
}

func extractDestinations(input json.RawMessage) []string {
	if len(input) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil
	}

	var out []string
	var walk func(key string, value any)
	walk = func(key string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			for childKey, childValue := range typed {
				walk(strings.ToLower(strings.TrimSpace(childKey)), childValue)
			}
		case []any:
			for _, item := range typed {
				walk(key, item)
			}
		case string:
			if isDestinationKey(key) && strings.TrimSpace(typed) != "" {
				out = append(out, typed)
			}
		}
	}
	walk("", decoded)
	return out
}

func isDestinationKey(key string) bool {
	switch key {
	case "path", "paths", "cwd", "directory", "directories", "additionaldirectories", "file", "files", "url", "uri", "target", "targets", "host", "server", "destination", "destinations":
		return true
	default:
		return false
	}
}

func ruleSourceRank(source RuleSource) int {
	switch source {
	case RuleSourceSession:
		return 70
	case RuleSourceCommand:
		return 60
	case RuleSourceCLIArg:
		return 50
	case RuleSourcePolicyUser:
		return settings.Rank(settings.SourcePolicySettings)*10 + settings.PolicyRank(settings.PolicySourceUser)
	case RuleSourcePolicyManaged:
		return settings.Rank(settings.SourcePolicySettings)*10 + settings.PolicyRank(settings.PolicySourceManaged)
	case RuleSourcePolicyMachine:
		return settings.Rank(settings.SourcePolicySettings)*10 + settings.PolicyRank(settings.PolicySourceMachine)
	case RuleSourcePolicyRemote:
		return settings.Rank(settings.SourcePolicySettings)*10 + settings.PolicyRank(settings.PolicySourceRemote)
	case RuleSourcePolicySettings:
		return settings.Rank(settings.SourcePolicySettings) * 10
	case RuleSourceFlagSettings:
		return settings.Rank(settings.SourceFlagSettings) * 10
	case RuleSourceLocalSettings:
		return settings.Rank(settings.SourceLocalSettings) * 10
	case RuleSourceProjectSettings:
		return settings.Rank(settings.SourceProjectSettings) * 10
	case RuleSourceUserSettings, "":
		return settings.Rank(settings.SourceUserSettings) * 10
	default:
		return -1
	}
}
