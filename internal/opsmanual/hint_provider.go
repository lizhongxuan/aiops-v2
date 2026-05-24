package opsmanual

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/memory"
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

type MemoryHintStore interface {
	Search(context.Context, memory.Query) ([]memory.Item, error)
}

type LocalMemoryHintProvider struct {
	Store     MemoryHintStore
	Scope     memory.Scope
	SessionID string
	ProjectID string
	Limit     int
}

func NewLocalMemoryHintProvider(store MemoryHintStore) LocalMemoryHintProvider {
	return LocalMemoryHintProvider{Store: store, Limit: 5}
}

func (p LocalMemoryHintProvider) ManualHints(ctx context.Context, query HintQuery) ([]ManualHint, error) {
	items, err := p.search(ctx, query, memory.KindOpsManualManualHint)
	if err != nil {
		return nil, err
	}
	now := hintNow(query.Now)
	out := make([]ManualHint, 0, len(items))
	for _, item := range items {
		if !memoryHintItemUsable(item, now) || !hintScopeMatches(item.ObjectType, query.OperationFrame.Target.Type) || !hintActionMatches(item.OperationAction, query.OperationFrame.Operation.Action) {
			continue
		}
		if strings.TrimSpace(item.ManualID) == "" {
			continue
		}
		out = append(out, ManualHint{
			ManualID:    strings.TrimSpace(item.ManualID),
			WorkflowID:  strings.TrimSpace(item.WorkflowID),
			ObjectType:  strings.TrimSpace(item.ObjectType),
			Action:      strings.TrimSpace(item.OperationAction),
			TargetAlias: strings.TrimSpace(item.TargetAlias),
			Text:        strings.TrimSpace(item.Text),
			Source:      "memory_hint",
			Redacted:    true,
			Score:       0.6,
			CreatedAt:   item.CreatedAt,
			ExpiresAt:   item.ExpiresAt,
		})
	}
	return out, nil
}

func (p LocalMemoryHintProvider) ParamHints(ctx context.Context, query HintQuery) ([]ParamHint, error) {
	items, err := p.search(ctx, query, memory.KindOpsManualParamHint)
	if err != nil {
		return nil, err
	}
	now := hintNow(query.Now)
	out := make([]ParamHint, 0, len(items))
	for _, item := range items {
		if !memoryHintItemUsable(item, now) || !hintScopeMatches(item.ObjectType, query.OperationFrame.Target.Type) || !hintActionMatches(item.OperationAction, query.OperationFrame.Operation.Action) {
			continue
		}
		if strings.TrimSpace(item.ParamID) != strings.TrimSpace(query.Requirement.ID) || strings.TrimSpace(item.ParamValue) == "" {
			continue
		}
		out = append(out, ParamHint{
			ParamID:                     strings.TrimSpace(item.ParamID),
			Value:                       strings.TrimSpace(item.ParamValue),
			Label:                       firstNonEmpty(strings.TrimSpace(item.ParamLabel), strings.TrimSpace(item.ParamValue)),
			ObjectType:                  strings.TrimSpace(item.ObjectType),
			Action:                      strings.TrimSpace(item.OperationAction),
			TargetAlias:                 strings.TrimSpace(item.TargetAlias),
			Source:                      "memory_hint",
			Redacted:                    true,
			RequiresCurrentConfirmation: true,
			Confidence:                  0.72,
			Evidence:                    "memory hint requires current confirmation/discovery",
			CreatedAt:                   item.CreatedAt,
			ExpiresAt:                   item.ExpiresAt,
		})
	}
	return out, nil
}

func (p LocalMemoryHintProvider) search(ctx context.Context, query HintQuery, kind memory.Kind) ([]memory.Item, error) {
	if p.Store == nil {
		return nil, nil
	}
	scope := p.Scope
	sessionID := firstNonEmpty(strings.TrimSpace(p.SessionID), strings.TrimSpace(query.SessionID))
	projectID := firstNonEmpty(strings.TrimSpace(p.ProjectID), strings.TrimSpace(query.ProjectID))
	if scope == "" {
		if sessionID != "" {
			scope = memory.ScopeSession
		} else {
			scope = memory.ScopeProject
		}
	}
	limit := query.Limit
	if limit <= 0 {
		limit = p.Limit
	}
	if limit <= 0 {
		limit = 5
	}
	text := strings.TrimSpace(fmt.Sprintf("%s %s %s %s %s",
		query.Text,
		query.OperationFrame.Target.Type,
		query.OperationFrame.Operation.Action,
		query.OperationFrame.Target.Name,
		query.Requirement.ID,
	))
	items, err := p.Store.Search(ctx, memory.Query{
		Scope:     scope,
		SessionID: sessionID,
		ProjectID: projectID,
		Text:      text,
		Limit:     limit,
	})
	if err != nil {
		if err == memory.ErrDisabled {
			return nil, nil
		}
		return nil, err
	}
	out := make([]memory.Item, 0, len(items))
	for _, item := range items {
		if item.Kind == kind {
			out = append(out, item)
		}
	}
	return out, nil
}

func memoryHintItemUsable(item memory.Item, now time.Time) bool {
	if item.Stale || !item.Redacted {
		return false
	}
	if !item.ExpiresAt.IsZero() && !item.ExpiresAt.After(now) {
		return false
	}
	return true
}

func hintNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now
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
