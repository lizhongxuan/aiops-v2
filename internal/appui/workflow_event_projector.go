package appui

import (
	"sync"
	"time"

	"runner/server/service"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

const (
	WorkflowWorkspaceSnapshotKey     = "workflowWorkspace"
	WorkflowWorkspaceSchemaVersion   = "aiops.workflow.workspace.v1"
	workflowProjectionEventRetention = 128
)

type WorkflowEventGap struct {
	ExpectedSequence uint64 `json:"expectedSequence"`
	ReceivedSequence uint64 `json:"receivedSequence"`
}

type WorkflowCommittedProjection struct {
	WorkflowKey edit.WorkflowKey `json:"workflowKey"`
	Revision    edit.Revision    `json:"revision"`
	Graph       visual.Graph     `json:"graph"`
	Sequence    uint64           `json:"sequence"`
	LoadedAt    time.Time        `json:"loadedAt"`
}

type WorkflowAuthoritativeSnapshot struct {
	WorkflowKey         edit.WorkflowKey                               `json:"workflowKey"`
	Revision            edit.Revision                                  `json:"revision"`
	Graph               visual.Graph                                   `json:"graph"`
	Sequence            uint64                                         `json:"sequence"`
	LoadedAt            time.Time                                      `json:"loadedAt"`
	RunGateFacts        *service.WorkflowRunGateProjection             `json:"runGateFacts,omitempty"`
	Receipts            []service.WorkflowReceiptProjection            `json:"receipts,omitempty"`
	ValidationArtifacts []service.WorkflowValidationArtifactProjection `json:"validationArtifacts,omitempty"`
	LifecycleArtifacts  []service.WorkflowLifecycleArtifactProjection  `json:"lifecycleArtifacts,omitempty"`
	DurableHistory      []service.WorkflowDurableHistoryProjection     `json:"durableHistory,omitempty"`
}

// WorkflowEventProjection is canonical AppSnapshot state, not a second page
// stream. The browser receives it only through the existing main /ws snapshot.
type WorkflowEventProjection struct {
	SchemaVersion          string                                         `json:"schemaVersion"`
	WorkflowKey            *edit.WorkflowKey                              `json:"workflowKey,omitempty"`
	Committed              *WorkflowCommittedProjection                   `json:"committed,omitempty"`
	Events                 []edit.WorkflowDomainEvent                     `json:"events"`
	LastSequence           uint64                                         `json:"lastSequence"`
	Gap                    *WorkflowEventGap                              `json:"gap,omitempty"`
	SnapshotResyncRequired bool                                           `json:"snapshotResyncRequired"`
	RunGateFacts           *service.WorkflowRunGateProjection             `json:"runGateFacts,omitempty"`
	Receipts               []service.WorkflowReceiptProjection            `json:"receipts,omitempty"`
	ValidationArtifacts    []service.WorkflowValidationArtifactProjection `json:"validationArtifacts,omitempty"`
	LifecycleArtifacts     []service.WorkflowLifecycleArtifactProjection  `json:"lifecycleArtifacts,omitempty"`
	DurableHistory         []service.WorkflowDurableHistoryProjection     `json:"durableHistory,omitempty"`
}

type WorkflowEventProjector struct {
	mu               sync.Mutex
	projection       WorkflowEventProjection
	buffered         map[uint64]edit.WorkflowDomainEvent
	seenEvents       map[string]struct{}
	seenTransactions map[string]struct{}
}

func NewWorkflowEventProjector() *WorkflowEventProjector {
	return &WorkflowEventProjector{
		projection: WorkflowEventProjection{SchemaVersion: WorkflowWorkspaceSchemaVersion, Events: []edit.WorkflowDomainEvent{}},
		buffered:   map[uint64]edit.WorkflowDomainEvent{}, seenEvents: map[string]struct{}{}, seenTransactions: map[string]struct{}{},
	}
}

func (p *WorkflowEventProjector) Apply(event edit.WorkflowDomainEvent) WorkflowEventProjection {
	p.mu.Lock()
	defer p.mu.Unlock()
	if event.Key.Validate() != nil || event.Sequence == 0 || event.EventID == "" {
		return p.cloneLocked()
	}
	if p.projection.WorkflowKey != nil && *p.projection.WorkflowKey != event.Key {
		return p.cloneLocked()
	}
	if _, ok := p.seenEvents[event.EventID]; ok || event.TransactionID != "" && p.transactionSeen(event.TransactionID) || event.Sequence <= p.projection.LastSequence {
		return p.cloneLocked()
	}
	event.Type = canonicalProjectedWorkflowEventType(event.Type)
	event.Payload = append([]byte(nil), event.Payload...)
	key := event.Key
	p.projection.WorkflowKey = &key
	if event.Sequence > p.projection.LastSequence+1 {
		p.buffered[event.Sequence] = event
		p.setGapLocked()
		return p.cloneLocked()
	}
	p.appendLocked(event)
	for {
		next, ok := p.buffered[p.projection.LastSequence+1]
		if !ok {
			break
		}
		delete(p.buffered, next.Sequence)
		p.appendLocked(next)
	}
	p.setGapLocked()
	return p.cloneLocked()
}

func (p *WorkflowEventProjector) Resync(snapshot WorkflowAuthoritativeSnapshot) WorkflowEventProjection {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.projection.WorkflowKey != nil && *p.projection.WorkflowKey == snapshot.WorkflowKey && snapshot.Sequence < p.projection.LastSequence {
		return p.cloneLocked()
	}
	if p.projection.WorkflowKey != nil && *p.projection.WorkflowKey != snapshot.WorkflowKey {
		p.projection = WorkflowEventProjection{SchemaVersion: WorkflowWorkspaceSchemaVersion, Events: []edit.WorkflowDomainEvent{}}
		p.buffered, p.seenEvents, p.seenTransactions = map[uint64]edit.WorkflowDomainEvent{}, map[string]struct{}{}, map[string]struct{}{}
	}
	key := snapshot.WorkflowKey
	committed := WorkflowCommittedProjection{
		WorkflowKey: snapshot.WorkflowKey, Revision: snapshot.Revision, Graph: snapshot.Graph,
		Sequence: snapshot.Sequence, LoadedAt: snapshot.LoadedAt,
	}
	if committed.LoadedAt.IsZero() {
		committed.LoadedAt = time.Now().UTC()
	}
	p.projection.WorkflowKey, p.projection.Committed, p.projection.LastSequence = &key, &committed, snapshot.Sequence
	p.projection.RunGateFacts = cloneWorkflowRunGateProjection(snapshot.RunGateFacts)
	p.projection.Receipts = cloneWorkflowReceiptProjections(snapshot.Receipts)
	p.projection.ValidationArtifacts = cloneWorkflowValidationArtifactProjections(snapshot.ValidationArtifacts)
	p.projection.LifecycleArtifacts = append([]service.WorkflowLifecycleArtifactProjection(nil), snapshot.LifecycleArtifacts...)
	p.projection.DurableHistory = append([]service.WorkflowDurableHistoryProjection(nil), snapshot.DurableHistory...)
	for sequence := range p.buffered {
		if sequence <= snapshot.Sequence {
			delete(p.buffered, sequence)
		}
	}
	for {
		next, ok := p.buffered[p.projection.LastSequence+1]
		if !ok {
			break
		}
		delete(p.buffered, next.Sequence)
		p.appendLocked(next)
	}
	p.setGapLocked()
	return p.cloneLocked()
}

func (p *WorkflowEventProjector) ProjectAppSnapshot(snapshot StateSnapshot) StateSnapshot {
	p.mu.Lock()
	projection := p.cloneLocked()
	p.mu.Unlock()
	config := make(map[string]any, len(snapshot.Config)+1)
	for key, value := range snapshot.Config {
		config[key] = value
	}
	config[WorkflowWorkspaceSnapshotKey] = projection
	snapshot.Config = config
	return snapshot
}

func (p *WorkflowEventProjector) appendLocked(event edit.WorkflowDomainEvent) {
	p.projection.Events = append(p.projection.Events, event)
	if len(p.projection.Events) > workflowProjectionEventRetention {
		p.projection.Events = append([]edit.WorkflowDomainEvent(nil), p.projection.Events[len(p.projection.Events)-workflowProjectionEventRetention:]...)
	}
	p.projection.LastSequence = event.Sequence
	p.seenEvents[event.EventID] = struct{}{}
	if event.TransactionID != "" {
		p.seenTransactions[event.TransactionID] = struct{}{}
	}
}

func (p *WorkflowEventProjector) transactionSeen(id string) bool {
	_, ok := p.seenTransactions[id]
	return ok
}

func (p *WorkflowEventProjector) setGapLocked() {
	if len(p.buffered) == 0 {
		p.projection.Gap, p.projection.SnapshotResyncRequired = nil, false
		return
	}
	received := ^uint64(0)
	for sequence := range p.buffered {
		if sequence < received {
			received = sequence
		}
	}
	p.projection.Gap = &WorkflowEventGap{ExpectedSequence: p.projection.LastSequence + 1, ReceivedSequence: received}
	p.projection.SnapshotResyncRequired = true
}

func (p *WorkflowEventProjector) cloneLocked() WorkflowEventProjection {
	result := p.projection
	result.Events = append([]edit.WorkflowDomainEvent(nil), p.projection.Events...)
	result.RunGateFacts = cloneWorkflowRunGateProjection(p.projection.RunGateFacts)
	result.Receipts = cloneWorkflowReceiptProjections(p.projection.Receipts)
	result.ValidationArtifacts = cloneWorkflowValidationArtifactProjections(p.projection.ValidationArtifacts)
	result.LifecycleArtifacts = append([]service.WorkflowLifecycleArtifactProjection(nil), p.projection.LifecycleArtifacts...)
	result.DurableHistory = append([]service.WorkflowDurableHistoryProjection(nil), p.projection.DurableHistory...)
	if p.projection.WorkflowKey != nil {
		value := *p.projection.WorkflowKey
		result.WorkflowKey = &value
	}
	if p.projection.Committed != nil {
		value := *p.projection.Committed
		result.Committed = &value
	}
	if p.projection.Gap != nil {
		value := *p.projection.Gap
		result.Gap = &value
	}
	return result
}

func cloneWorkflowRunGateProjection(value *service.WorkflowRunGateProjection) *service.WorkflowRunGateProjection {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.PostCheck != nil {
		postCheck := *value.PostCheck
		postCheck.Diagnostics = append([]edit.WorkflowDiagnostic(nil), value.PostCheck.Diagnostics...)
		postCheck.Configuration.SecretRefs = append([]string(nil), value.PostCheck.Configuration.SecretRefs...)
		postCheck.Configuration.Environments = append([]string(nil), value.PostCheck.Configuration.Environments...)
		cloned.PostCheck = &postCheck
	}
	return &cloned
}

func cloneWorkflowReceiptProjections(values []service.WorkflowReceiptProjection) []service.WorkflowReceiptProjection {
	result := append([]service.WorkflowReceiptProjection(nil), values...)
	for index := range result {
		result[index].Validation.Diagnostics = append([]edit.WorkflowDiagnostic{}, values[index].Validation.Diagnostics...)
		if values[index].RevisionAfter != nil {
			revision := *values[index].RevisionAfter
			result[index].RevisionAfter = &revision
		}
	}
	return result
}

func cloneWorkflowValidationArtifactProjections(values []service.WorkflowValidationArtifactProjection) []service.WorkflowValidationArtifactProjection {
	result := append([]service.WorkflowValidationArtifactProjection(nil), values...)
	for index := range result {
		result[index].Diagnostics = append([]edit.WorkflowDiagnostic{}, values[index].Diagnostics...)
	}
	return result
}

func canonicalProjectedWorkflowEventType(value string) string {
	if len(value) > len("workflow.") && value[:len("workflow.")] == "workflow." {
		value = value[len("workflow."):]
	}
	if value == "graph_committed" {
		return "transaction.applied"
	}
	return value
}
