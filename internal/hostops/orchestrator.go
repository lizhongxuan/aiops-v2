package hostops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrHostOutsideMission      = errors.New("host is outside the host operation mission")
	ErrChildSpawnerUnavailable = errors.New("host child spawner is unavailable")
)

type ChildAgentAssignment struct {
	HostID          string `json:"hostId"`
	HostAddress     string `json:"hostAddress,omitempty"`
	HostDisplayName string `json:"hostDisplayName,omitempty"`
	Role            string `json:"role,omitempty"`
	Task            string `json:"task"`
	SessionID       string `json:"sessionId,omitempty"`
	ParentAgentID   string `json:"parentAgentId,omitempty"`
}

type SpawnHostChildRequest struct {
	ChildAgentID    string
	MissionID       string
	ParentAgentID   string
	SessionID       string
	HostID          string
	HostAddress     string
	HostDisplayName string
	Role            string
	Task            string
}

type ChildSpawner interface {
	SpawnHostChild(ctx context.Context, req SpawnHostChildRequest) (HostChildAgent, error)
	SendMessage(ctx context.Context, childAgentID, content string) (HostChildAgent, error)
	Stop(ctx context.Context, childAgentID string) (HostChildAgent, error)
}

type Orchestrator struct {
	store      MissionStore
	transcript TranscriptStore
	spawner    ChildSpawner
}

func NewOrchestrator(store MissionStore, transcript TranscriptStore, spawner ChildSpawner) *Orchestrator {
	return &Orchestrator{store: store, transcript: transcript, spawner: spawner}
}

func (o *Orchestrator) AcceptPlan(ctx context.Context, missionID, planID string) error {
	if o == nil || o.store == nil {
		return ErrMissionNotFound
	}
	mission, err := o.store.GetMission(ctx, strings.TrimSpace(missionID))
	if err != nil {
		return err
	}
	mission.PlanAccepted = true
	mission.Status = HostMissionStatusSpawningChildren
	if strings.TrimSpace(planID) != "" {
		mission.UpdatedAt = time.Now().UTC()
	}
	return o.store.SaveMission(ctx, mission)
}

func (o *Orchestrator) SpawnChildren(ctx context.Context, missionID string, assignments []ChildAgentAssignment) ([]HostChildAgent, error) {
	if o == nil || o.store == nil {
		return nil, ErrMissionNotFound
	}
	mission, err := o.store.GetMission(ctx, strings.TrimSpace(missionID))
	if err != nil {
		return nil, err
	}
	if err := EnforcePlanGate(mission, OperationRiskMutating); err != nil {
		return nil, err
	}
	if o.spawner == nil {
		return nil, ErrChildSpawnerUnavailable
	}

	existing, err := o.store.ListChildAgents(ctx, mission.ID)
	if err != nil {
		return nil, err
	}
	childrenByHost := map[string]HostChildAgent{}
	for _, child := range existing {
		if key := childHostKey(child); key != "" {
			childrenByHost[key] = child
		}
	}

	allowed := missionAllowedHostKeys(mission)
	children := make([]HostChildAgent, 0, len(assignments))
	for _, assignment := range assignments {
		assignment = normalizeAssignment(assignment, mission)
		key := assignmentHostKey(assignment)
		if key == "" {
			return nil, fmt.Errorf("child assignment hostId is required")
		}
		if !allowed[key] {
			return nil, fmt.Errorf("%w: %s", ErrHostOutsideMission, assignment.HostID)
		}
		if existingChild, ok := childrenByHost[key]; ok {
			children = append(children, existingChild)
			continue
		}

		req := SpawnHostChildRequest{
			ChildAgentID:    childAgentIDFor(mission.ID, assignment.HostID),
			MissionID:       mission.ID,
			ParentAgentID:   firstNonEmptyString(assignment.ParentAgentID, mission.ManagerAgentID),
			SessionID:       firstNonEmptyString(assignment.SessionID, "host-child:"+mission.ID+":"+assignment.HostID),
			HostID:          assignment.HostID,
			HostAddress:     assignment.HostAddress,
			HostDisplayName: assignment.HostDisplayName,
			Role:            assignment.Role,
			Task:            assignment.Task,
		}
		child, err := o.spawner.SpawnHostChild(ctx, req)
		if err != nil {
			return nil, err
		}
		child = normalizeSpawnedChild(child, req)
		if err := o.store.SaveChildAgent(ctx, child); err != nil {
			return nil, err
		}
		o.appendTranscript(ctx, child.ID, TranscriptItem{
			Type:    TranscriptItemManagerMessage,
			Content: assignment.Task,
			Status:  string(child.Status),
			Payload: map[string]any{
				"missionId": mission.ID,
				"hostId":    assignment.HostID,
				"role":      assignment.Role,
			},
		})
		childrenByHost[key] = child
		children = append(children, child)
	}
	return children, nil
}

func (o *Orchestrator) SendMessage(ctx context.Context, childAgentID, content string) (HostChildAgent, error) {
	childAgentID = strings.TrimSpace(childAgentID)
	content = strings.TrimSpace(content)
	if childAgentID == "" {
		return HostChildAgent{}, ErrChildAgentNotFound
	}
	if o == nil || o.store == nil || o.spawner == nil {
		return HostChildAgent{}, ErrChildSpawnerUnavailable
	}
	child, err := o.spawner.SendMessage(ctx, childAgentID, content)
	if err != nil {
		return HostChildAgent{}, err
	}
	current, getErr := o.store.GetChildAgent(ctx, childAgentID)
	if getErr == nil {
		child = mergeChildAgentUpdate(current, child)
	}
	child.LastInputPreview = firstNonEmptyString(child.LastInputPreview, content)
	if err := o.store.SaveChildAgent(ctx, child); err != nil {
		return HostChildAgent{}, err
	}
	o.appendTranscript(ctx, child.ID, TranscriptItem{Type: TranscriptItemUserFollowup, Content: content, Status: string(child.Status)})
	return child, nil
}

func (o *Orchestrator) Stop(ctx context.Context, childAgentID string) (HostChildAgent, error) {
	childAgentID = strings.TrimSpace(childAgentID)
	if childAgentID == "" {
		return HostChildAgent{}, ErrChildAgentNotFound
	}
	if o == nil || o.store == nil || o.spawner == nil {
		return HostChildAgent{}, ErrChildSpawnerUnavailable
	}
	child, err := o.spawner.Stop(ctx, childAgentID)
	if err != nil {
		return HostChildAgent{}, err
	}
	current, getErr := o.store.GetChildAgent(ctx, childAgentID)
	if getErr == nil {
		child = mergeChildAgentUpdate(current, child)
	}
	if child.Status == "" {
		child.Status = HostChildAgentStatusCancelled
	}
	now := time.Now().UTC()
	child.CompletedAt = &now
	if err := o.store.SaveChildAgent(ctx, child); err != nil {
		return HostChildAgent{}, err
	}
	o.appendTranscript(ctx, child.ID, TranscriptItem{Type: TranscriptItemManagerMessage, Content: "stop host child agent", Status: string(child.Status)})
	return child, nil
}

func (o *Orchestrator) WaitChildren(ctx context.Context, missionID string) ([]HostChildAgent, error) {
	if o == nil || o.store == nil {
		return nil, ErrMissionNotFound
	}
	return o.store.ListChildAgents(ctx, strings.TrimSpace(missionID))
}

func (o *Orchestrator) appendTranscript(ctx context.Context, childAgentID string, item TranscriptItem) {
	if o == nil || o.transcript == nil || strings.TrimSpace(childAgentID) == "" {
		return
	}
	_ = o.transcript.Append(ctx, childAgentID, item)
}

func missionAllowedHostKeys(mission HostOperationMission) map[string]bool {
	allowed := map[string]bool{}
	for _, mention := range mission.Mentions {
		for _, key := range mentionHostKeys(mention) {
			allowed[key] = true
		}
	}
	return allowed
}

func mentionHostKeys(mention HostMention) []string {
	values := []string{mention.HostID, mention.Address, mention.DisplayName, strings.TrimPrefix(mention.Raw, "@")}
	keys := make([]string, 0, len(values))
	for _, value := range values {
		if key := normalizedHostKey(value); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func assignmentHostKey(assignment ChildAgentAssignment) string {
	return normalizedHostKey(firstNonEmptyString(assignment.HostID, assignment.HostAddress, assignment.HostDisplayName))
}

func childHostKey(child HostChildAgent) string {
	return normalizedHostKey(firstNonEmptyString(child.HostID, child.HostAddress))
}

func normalizedHostKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return value
}

func normalizeAssignment(assignment ChildAgentAssignment, mission HostOperationMission) ChildAgentAssignment {
	assignment.HostID = strings.TrimSpace(assignment.HostID)
	assignment.HostAddress = strings.TrimSpace(assignment.HostAddress)
	assignment.HostDisplayName = strings.TrimSpace(assignment.HostDisplayName)
	assignment.Role = strings.TrimSpace(assignment.Role)
	assignment.Task = strings.TrimSpace(assignment.Task)
	assignment.SessionID = strings.TrimSpace(assignment.SessionID)
	assignment.ParentAgentID = strings.TrimSpace(assignment.ParentAgentID)
	if assignment.Task == "" {
		assignment.Task = "Operate on the assigned host and report evidence to the manager."
	}
	if assignment.HostID == "" {
		assignment.HostID = firstNonEmptyString(assignment.HostAddress, assignment.HostDisplayName)
	}
	for _, mention := range mission.Mentions {
		if assignmentHostKey(assignment) == normalizedHostKey(firstNonEmptyString(mention.HostID, mention.Address, mention.DisplayName, strings.TrimPrefix(mention.Raw, "@"))) {
			assignment.HostID = firstNonEmptyString(assignment.HostID, mention.HostID, mention.Address, mention.DisplayName)
			assignment.HostAddress = firstNonEmptyString(assignment.HostAddress, mention.Address)
			assignment.HostDisplayName = firstNonEmptyString(assignment.HostDisplayName, mention.DisplayName)
			return assignment
		}
	}
	return assignment
}

func normalizeSpawnedChild(child HostChildAgent, req SpawnHostChildRequest) HostChildAgent {
	child.ID = firstNonEmptyString(child.ID, req.ChildAgentID)
	child.MissionID = firstNonEmptyString(child.MissionID, req.MissionID)
	child.ParentAgentID = firstNonEmptyString(child.ParentAgentID, req.ParentAgentID)
	child.SessionID = firstNonEmptyString(child.SessionID, req.SessionID)
	child.HostID = firstNonEmptyString(child.HostID, req.HostID)
	child.HostAddress = firstNonEmptyString(child.HostAddress, req.HostAddress)
	child.Role = firstNonEmptyString(child.Role, req.Role)
	child.Task = firstNonEmptyString(child.Task, req.Task)
	if child.Status == "" {
		child.Status = HostChildAgentStatusSpawning
	}
	if child.StartedAt.IsZero() {
		child.StartedAt = time.Now().UTC()
	}
	return child
}

func mergeChildAgentUpdate(current HostChildAgent, update HostChildAgent) HostChildAgent {
	if update.ID == "" {
		update.ID = current.ID
	}
	update.MissionID = firstNonEmptyString(update.MissionID, current.MissionID)
	update.ParentAgentID = firstNonEmptyString(update.ParentAgentID, current.ParentAgentID)
	update.SessionID = firstNonEmptyString(update.SessionID, current.SessionID)
	update.HostID = firstNonEmptyString(update.HostID, current.HostID)
	update.HostAddress = firstNonEmptyString(update.HostAddress, current.HostAddress)
	update.Role = firstNonEmptyString(update.Role, current.Role)
	update.Task = firstNonEmptyString(update.Task, current.Task)
	update.PlanStepIDs = append([]string(nil), current.PlanStepIDs...)
	update.StartedAt = firstNonZeroTime(update.StartedAt, current.StartedAt)
	return update
}

func childAgentIDFor(missionID, hostID string) string {
	return "host-child-" + sanitizeIDPart(missionID) + "-" + sanitizeIDPart(hostID)
}

func sanitizeIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
