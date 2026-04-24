package projection

import (
	"encoding/json"
	"time"

	"aiops-v2/internal/runtimekernel"
)

// ---------------------------------------------------------------------------
// Projector – routes LifecycleEvents to the correct projection handler and
// notifies subscribers. The Projector performs NO business reasoning (Req 7.7).
// ---------------------------------------------------------------------------

// Projector accepts LifecycleEvents from RuntimeKernel, routes them to the
// correct projection handler based on EventType, and notifies subscribers.
type Projector struct {
	subscribers []Subscriber
}

// NewProjector creates a Projector with the given subscribers.
func NewProjector(subscribers ...Subscriber) *Projector {
	return &Projector{
		subscribers: subscribers,
	}
}

// AddSubscriber registers a new subscriber.
func (p *Projector) AddSubscriber(s Subscriber) {
	p.subscribers = append(p.subscribers, s)
}

// Emit accepts a LifecycleEvent and routes it to the appropriate projection
// handler based on EventType. It then notifies all subscribers with the
// projected data. Emit does NOT perform business reasoning or mutate state
// beyond projection (Req 7.7).
func (p *Projector) Emit(event runtimekernel.LifecycleEvent) {
	switch event.Type {
	case runtimekernel.EventToolStarted, runtimekernel.EventToolProgress,
		runtimekernel.EventToolCompleted, runtimekernel.EventToolFailed:
		inv := projectToolInvocation(event)
		for _, s := range p.subscribers {
			s.OnToolInvocation(inv)
		}

	case runtimekernel.EventActivityUpdate:
		activity := projectActivity(event)
		for _, s := range p.subscribers {
			s.OnActivity(activity)
		}

	case runtimekernel.EventCardGenerated:
		card := projectCard(event)
		for _, s := range p.subscribers {
			s.OnCard(card)
		}

	case runtimekernel.EventApprovalNeeded, runtimekernel.EventApprovalDecided:
		approval := projectApproval(event)
		for _, s := range p.subscribers {
			s.OnApproval(approval)
		}

	case runtimekernel.EventEvidenceCollected:
		evidence := projectEvidence(event)
		for _, s := range p.subscribers {
			s.OnEvidence(evidence)
		}

	case runtimekernel.EventTurnComplete:
		snapshot := projectSnapshot(event)
		for _, s := range p.subscribers {
			s.OnSnapshot(snapshot)
		}

	case runtimekernel.EventTurnStarted, runtimekernel.EventAssistantIntent, runtimekernel.EventAssistantFinalDelta,
		runtimekernel.EventPhaseEnd, runtimekernel.EventProcessSummary, runtimekernel.EventTurnError, runtimekernel.EventTurnAborted:
		for _, s := range p.subscribers {
			if receiver, ok := s.(TurnLifecycleSubscriber); ok {
				receiver.OnRuntimeLifecycleEvent(event)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Subscriber – receives projected data from the Projector.
// ---------------------------------------------------------------------------

// Subscriber receives projected data notifications from the Projector.
type Subscriber interface {
	OnToolInvocation(inv ToolInvocation)
	OnActivity(activity ActivityStats)
	OnCard(card Card)
	OnApproval(approval Approval)
	OnEvidence(evidence Evidence)
	OnSnapshot(snapshot Snapshot)
}

type TurnLifecycleSubscriber interface {
	OnRuntimeLifecycleEvent(event runtimekernel.LifecycleEvent)
}

// ---------------------------------------------------------------------------
// Six projection data structures
// ---------------------------------------------------------------------------

// ToolInvocationStatus represents the lifecycle state of a tool invocation.
type ToolInvocationStatus string

const (
	ToolInvocationStarted   ToolInvocationStatus = "started"
	ToolInvocationProgress  ToolInvocationStatus = "progress"
	ToolInvocationCompleted ToolInvocationStatus = "completed"
	ToolInvocationFailed    ToolInvocationStatus = "failed"
)

// ToolInvocation represents a tool execution lifecycle projection.
type ToolInvocation struct {
	ID        string               `json:"id"`
	SessionID string               `json:"sessionId"`
	TurnID    string               `json:"turnId"`
	ToolName  string               `json:"toolName"`
	Status    ToolInvocationStatus `json:"status"`
	Args      json.RawMessage      `json:"args,omitempty"`
	Result    string               `json:"result,omitempty"`
	Error     string               `json:"error,omitempty"`
	StartedAt time.Time            `json:"startedAt"`
	EndedAt   *time.Time           `json:"endedAt,omitempty"`
}

// ActivityStats represents runtime.activity aggregation projection.
// Counts are derived from runtime aggregation, not UI text lines.
type ActivityStats struct {
	SessionID      string `json:"sessionId"`
	TurnID         string `json:"turnId"`
	SearchCount    int    `json:"searchCount"`
	BrowseCount    int    `json:"browseCount"`
	CommandCount   int    `json:"commandCount"`
	FileReadCount  int    `json:"fileReadCount"`
	FileWriteCount int    `json:"fileWriteCount"`
}

// Card represents a UI card data projection.
type Card struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	Type      string          `json:"type"`
	Title     string          `json:"title,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

// ApprovalStatus represents the state of an approval record.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
)

// Approval represents an approval record projection, independent from
// assistant text (Req 7.4, 12.3).
type Approval struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	TurnID    string         `json:"turnId"`
	ToolName  string         `json:"toolName"`
	Command   string         `json:"command,omitempty"`
	HostID    string         `json:"hostId,omitempty"`
	Status    ApprovalStatus `json:"status"`
	Operator  string         `json:"operator,omitempty"`
	Decision  string         `json:"decision,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	DecidedAt *time.Time     `json:"decidedAt,omitempty"`
}

// Evidence represents an evidence record projection, independent from
// assistant text (Req 7.5).
type Evidence struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	Type      string          `json:"type"`
	Summary   string          `json:"summary,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

// Snapshot represents a state snapshot projection.
type Snapshot struct {
	SessionID string          `json:"sessionId"`
	TurnID    string          `json:"turnId"`
	State     json.RawMessage `json:"state,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Internal projection helpers – pure data extraction, no business logic.
// ---------------------------------------------------------------------------

func projectToolInvocation(event runtimekernel.LifecycleEvent) ToolInvocation {
	inv := ToolInvocation{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		StartedAt: event.Timestamp,
	}

	// Extract payload fields if present.
	if event.Payload != nil {
		var payload struct {
			ID       string          `json:"id"`
			ToolName string          `json:"toolName"`
			Args     json.RawMessage `json:"args"`
			Result   string          `json:"result"`
			Error    string          `json:"error"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		inv.ID = payload.ID
		inv.ToolName = payload.ToolName
		inv.Args = payload.Args
		inv.Result = payload.Result
		inv.Error = payload.Error
	}

	switch event.Type {
	case runtimekernel.EventToolStarted:
		inv.Status = ToolInvocationStarted
	case runtimekernel.EventToolProgress:
		inv.Status = ToolInvocationProgress
	case runtimekernel.EventToolCompleted:
		inv.Status = ToolInvocationCompleted
		t := event.Timestamp
		inv.EndedAt = &t
	case runtimekernel.EventToolFailed:
		inv.Status = ToolInvocationFailed
		t := event.Timestamp
		inv.EndedAt = &t
	}

	return inv
}

func projectActivity(event runtimekernel.LifecycleEvent) ActivityStats {
	stats := ActivityStats{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
	}
	if event.Payload != nil {
		_ = json.Unmarshal(event.Payload, &stats)
		// Restore session/turn from event (payload may not carry them).
		stats.SessionID = event.SessionID
		stats.TurnID = event.TurnID
	}
	return stats
}

func projectCard(event runtimekernel.LifecycleEvent) Card {
	card := Card{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		CreatedAt: event.Timestamp,
	}
	if event.Payload != nil {
		var payload struct {
			ID    string          `json:"id"`
			Type  string          `json:"type"`
			Title string          `json:"title"`
			Data  json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		card.ID = payload.ID
		card.Type = payload.Type
		card.Title = payload.Title
		card.Data = payload.Data
	}
	return card
}

func projectApproval(event runtimekernel.LifecycleEvent) Approval {
	approval := Approval{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		CreatedAt: event.Timestamp,
	}
	if event.Payload != nil {
		var payload struct {
			ID       string `json:"id"`
			ToolName string `json:"toolName"`
			Command  string `json:"command"`
			HostID   string `json:"hostId"`
			Status   string `json:"status"`
			Operator string `json:"operator"`
			Decision string `json:"decision"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		approval.ID = payload.ID
		approval.ToolName = payload.ToolName
		approval.Command = payload.Command
		approval.HostID = payload.HostID
		approval.Status = ApprovalStatus(payload.Status)
		approval.Operator = payload.Operator
		approval.Decision = payload.Decision
	}

	if event.Type == runtimekernel.EventApprovalNeeded {
		approval.Status = ApprovalPending
	}
	return approval
}

func projectEvidence(event runtimekernel.LifecycleEvent) Evidence {
	evidence := Evidence{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		CreatedAt: event.Timestamp,
	}
	if event.Payload != nil {
		var payload struct {
			ID      string          `json:"id"`
			Type    string          `json:"type"`
			Summary string          `json:"summary"`
			Data    json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		evidence.ID = payload.ID
		evidence.Type = payload.Type
		evidence.Summary = payload.Summary
		evidence.Data = payload.Data
	}
	return evidence
}

func projectSnapshot(event runtimekernel.LifecycleEvent) Snapshot {
	snapshot := Snapshot{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		Timestamp: event.Timestamp,
	}
	if event.Payload != nil {
		snapshot.State = event.Payload
	}
	return snapshot
}
