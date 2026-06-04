package toolfailure

import "strings"

type ClassificationInput struct {
	Source  string
	Outcome string
	Error   string
}

type Decision struct {
	Kind         ToolFailureKind
	Action       HandlingAction
	Reason       string
	Retryable    bool
	RequiresUser bool
	Terminal     bool
}

type Classifier struct{}

func NewClassifier() Classifier {
	return Classifier{}
}

func (Classifier) Classify(input ClassificationInput) Decision {
	errText := strings.ToLower(strings.TrimSpace(input.Error))
	source := strings.ToLower(strings.TrimSpace(input.Source))
	outcome := strings.ToLower(strings.TrimSpace(input.Outcome))

	if strings.Contains(errText, "tool not found") {
		return Decision{Kind: KindToolNotFound, Action: ActionFeedErrorToModel, Reason: input.Error}
	}
	if strings.Contains(errText, "invalid argument") || strings.Contains(errText, "invalid arguments") {
		return Decision{Kind: KindInvalidArguments, Action: ActionFeedErrorToModel, Reason: input.Error, RequiresUser: true}
	}
	if strings.Contains(errText, "session expired") || strings.Contains(errText, "mcp session expired") {
		return Decision{Kind: KindMCPSessionExpired, Action: ActionFeedErrorToModel, Reason: input.Error, RequiresUser: true}
	}
	if source == "mcp" && (strings.Contains(errText, "server unavailable") || strings.Contains(errText, "connection refused") || strings.Contains(errText, "connection reset")) {
		return Decision{Kind: KindMCPServerUnavailable, Action: ActionFeedErrorToModel, Reason: input.Error, RequiresUser: true}
	}
	if source == "policy" && outcome == "tool_denied" {
		return Decision{Kind: KindPolicyDenied, Action: ActionFailTurn, Reason: input.Error, Terminal: true}
	}
	if strings.Contains(errText, "deadline exceeded") || strings.Contains(errText, "timeout") {
		return Decision{Kind: KindTimeout, Action: ActionFeedErrorToModel, Reason: input.Error}
	}
	return Decision{Kind: KindToolBusinessError, Action: ActionFeedErrorToModel, Reason: input.Error}
}
