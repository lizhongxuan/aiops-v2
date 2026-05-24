package runtimekernel

import "fmt"

type PTLMessageGroup struct {
	ID             string    `json:"id"`
	TurnID         string    `json:"turnId,omitempty"`
	Iteration      int       `json:"iteration,omitempty"`
	ToolRound      int       `json:"toolRound,omitempty"`
	Messages       []Message `json:"messages,omitempty"`
	HasExternalRef bool      `json:"hasExternalRef,omitempty"`
	Protected      bool      `json:"protected,omitempty"`
}

type PTLFallbackOptions struct {
	Attempt     int `json:"attempt"`
	MaxAttempts int `json:"maxAttempts"`
}

type PTLFallbackPlan struct {
	RetainedGroups  []PTLMessageGroup      `json:"retainedGroups,omitempty"`
	DroppedGroupIDs []string               `json:"droppedGroupIds,omitempty"`
	CanRetry        bool                   `json:"canRetry"`
	Event           ContextGovernanceEvent `json:"event"`
}

// PlanPTLFallback is retained for trace/backward-compatibility helpers, but
// production L5 handling no longer retries prompt-too-long compression. It
// returns the original groups and CanRetry=false.
func PlanPTLFallback(groups []PTLMessageGroup, opts PTLFallbackOptions) PTLFallbackPlan {
	_ = opts
	return PTLFallbackPlan{RetainedGroups: append([]PTLMessageGroup(nil), groups...)}
}

// GroupMessagesForPTLFallback groups messages by turn, iteration, and tool
// result round. The newest group is protected so latest user constraints remain
// model-visible.
func GroupMessagesForPTLFallback(messages []Message) []PTLMessageGroup {
	groups := make([]PTLMessageGroup, 0, len(messages))
	toolRound := 0
	for i, msg := range messages {
		if msg.ToolResult != nil {
			toolRound++
		}
		group := PTLMessageGroup{
			ID:             fmt.Sprintf("msg-%03d-round-%03d", i, toolRound),
			ToolRound:      toolRound,
			Messages:       []Message{msg},
			HasExternalRef: messageHasExternalRef(msg),
			Protected:      messageProtectedFromPTLDrop(msg),
		}
		groups = append(groups, group)
	}
	if len(groups) > 0 {
		groups[len(groups)-1].Protected = true
	}
	return groups
}

func messageHasExternalRef(msg Message) bool {
	return msg.ToolResult != nil && len(msg.ToolResult.ExternalReferences) > 0
}

func messageProtectedFromPTLDrop(msg Message) bool {
	if msg.Role == "user" || msg.Role == "system" {
		return true
	}
	if msg.ToolResult != nil && msg.ToolResult.Error != "" {
		return true
	}
	return len(msg.ToolCalls) > 0
}
