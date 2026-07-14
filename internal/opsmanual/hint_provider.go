package opsmanual

import (
	"context"
	"strings"
	"time"
)

type HintProvider interface {
	ManualHints(context.Context, HintQuery) ([]ManualHint, error)
	ParamHints(context.Context, HintQuery) ([]ParamHint, error)
}

type HintQuery struct {
	Text           string
	OperationFrame OperationFrame
	Manual         OpsManual
	Requirement    ParamRequirement
	SessionID      string
	ProjectID      string
	Now            time.Time
	Limit          int
}

type ManualHint struct {
	ManualID    string
	WorkflowID  string
	ObjectType  string
	Action      string
	TargetAlias string
	Text        string
	Source      string
	Redacted    bool
	Score       float64
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type ParamHint struct {
	ParamID                     string
	Value                       any
	Label                       string
	ObjectType                  string
	Action                      string
	TargetAlias                 string
	Source                      string
	Redacted                    bool
	RequiresCurrentConfirmation bool
	Confidence                  float64
	Evidence                    string
	CreatedAt                   time.Time
	ExpiresAt                   time.Time
}

type NoopHintProvider struct{}

func (NoopHintProvider) ManualHints(context.Context, HintQuery) ([]ManualHint, error) {
	return nil, nil
}

func (NoopHintProvider) ParamHints(context.Context, HintQuery) ([]ParamHint, error) {
	return nil, nil
}

func hintScopeMatches(hintValue string, current string) bool {
	hintValue = strings.TrimSpace(hintValue)
	current = strings.TrimSpace(current)
	return hintValue == "" || current == "" || equalFold(hintValue, current)
}

func hintActionMatches(hintValue string, current string) bool {
	hintValue = strings.TrimSpace(hintValue)
	current = strings.TrimSpace(current)
	return hintValue == "" || current == "" || operationsCompatibleForSearch(hintValue, current)
}

func cloneManualHints(in []ManualHint) []ManualHint {
	if len(in) == 0 {
		return nil
	}
	out := make([]ManualHint, len(in))
	copy(out, in)
	return out
}

func cloneParamHints(in []ParamHint) []ParamHint {
	if len(in) == 0 {
		return nil
	}
	out := make([]ParamHint, len(in))
	copy(out, in)
	return out
}
