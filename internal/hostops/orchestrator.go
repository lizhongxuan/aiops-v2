package hostops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/observability"
	"aiops-v2/internal/opssemantic"
)

var (
	ErrHostOutsideMission      = errors.New("host is outside the host operation mission")
	ErrChildSpawnerUnavailable = errors.New("host child spawner is unavailable")
)

type ChildAgentAssignment struct {
	HostID               string                   `json:"hostId"`
	HostAddress          string                   `json:"hostAddress,omitempty"`
	HostDisplayName      string                   `json:"hostDisplayName,omitempty"`
	Role                 string                   `json:"role,omitempty"`
	Task                 string                   `json:"task"`
	SessionID            string                   `json:"sessionId,omitempty"`
	ParentAgentID        string                   `json:"parentAgentId,omitempty"`
	PlanStepID           string                   `json:"planStepId,omitempty"`
	Constraints          []string                 `json:"constraints,omitempty"`
	RiskLevel            opssemantic.OpsRiskLevel `json:"riskLevel,omitempty"`
	EvidenceRequirements []string                 `json:"evidenceRequirements,omitempty"`
}

type SpawnHostChildRequest struct {
	ChildAgentID           string
	MissionID              string
	ParentAgentID          string
	SessionID              string
	HostID                 string
	HostAddress            string
	HostDisplayName        string
	Role                   string
	Task                   string
	PlanStepID             string
	Constraints            []string
	RiskLevel              opssemantic.OpsRiskLevel
	EvidenceRequirements   []string
	RuntimeContext         HostAgentRuntimeContext
	ContextSummary         string
	ContextRefs            []ContextRef
	ContextDecisionTraceID string
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
	messages   AgentMessageStore
	scheduler  *HostSubTaskScheduler
}

func NewOrchestrator(store MissionStore, transcript TranscriptStore, spawner ChildSpawner) *Orchestrator {
	o := &Orchestrator{store: store, transcript: transcript, spawner: spawner}
	if scheduleStore, ok := store.(hostSubTaskScheduleStore); ok {
		o.scheduler = NewHostSubTaskScheduler(scheduleStore)
	}
	return o
}

func (o *Orchestrator) WithAgentMessageStore(messages AgentMessageStore) *Orchestrator {
	if o != nil {
		o.messages = messages
	}
	return o
}

func (o *Orchestrator) CreatePlan(ctx context.Context, missionID string) (HostOperationMission, error) {
	if o == nil || o.store == nil {
		return HostOperationMission{}, ErrMissionNotFound
	}
	mission, err := o.store.GetMission(ctx, strings.TrimSpace(missionID))
	if err != nil {
		return HostOperationMission{}, err
	}
	plan, err := BuildPlanForMission(mission)
	if err != nil {
		observability.RecordOpsMetric(observability.OpsMetricPlanGeneration, false)
		return HostOperationMission{}, err
	}
	mission.Plan = plan
	mission.PlanRequired = true
	mission.PlanAccepted = false
	mission.Status = HostMissionStatusWaitingPlanAcceptance
	if err := o.store.SaveMission(ctx, mission); err != nil {
		observability.RecordOpsMetric(observability.OpsMetricPlanGeneration, false)
		return HostOperationMission{}, err
	}
	observability.RecordOpsMetric(observability.OpsMetricPlanGeneration, true)
	o.appendMissionAudit(ctx, mission.ID, TranscriptItem{
		Type:    TranscriptItemManagerMessage,
		Content: "host operation plan created",
		Status:  string(mission.Status),
		Payload: map[string]any{
			"missionId": mission.ID,
			"planId":    mission.Plan.ID,
			"version":   mission.Plan.Version,
		},
	})
	return o.store.GetMission(ctx, mission.ID)
}

func (o *Orchestrator) AcceptPlan(ctx context.Context, missionID, planID string) error {
	if o == nil || o.store == nil {
		return ErrMissionNotFound
	}
	mission, err := o.store.GetMission(ctx, strings.TrimSpace(missionID))
	if err != nil {
		return err
	}
	if strings.TrimSpace(planID) != "" && mission.Plan.ID != "" && strings.TrimSpace(planID) != mission.Plan.ID {
		return ErrMissionNotFound
	}
	mission.PlanAccepted = true
	mission.Status = HostMissionStatusSpawningChildren
	if mission.Plan.ID != "" {
		now := time.Now().UTC()
		mission.Plan.Status = PlanStatusAccepted
		mission.Plan.AcceptedAt = &now
	}
	if err := o.store.SaveMission(ctx, mission); err != nil {
		observability.RecordOpsMetric(observability.OpsMetricPlanAcceptance, false)
		return err
	}
	observability.RecordOpsMetric(observability.OpsMetricPlanAcceptance, true)
	o.appendMissionAudit(ctx, mission.ID, TranscriptItem{
		Type:    TranscriptItemManagerMessage,
		Content: "host operation plan accepted",
		Status:  string(mission.Status),
		Payload: map[string]any{
			"missionId": mission.ID,
			"planId":    mission.Plan.ID,
			"version":   mission.Plan.Version,
		},
	})
	return nil
}

func (o *Orchestrator) RevisePlan(ctx context.Context, missionID string, req PlanRevisionRequest) (HostOperationMission, error) {
	if o == nil || o.store == nil {
		return HostOperationMission{}, ErrMissionNotFound
	}
	mission, err := o.store.GetMission(ctx, strings.TrimSpace(missionID))
	if err != nil {
		return HostOperationMission{}, err
	}
	revised, err := ReviseMissionPlan(mission, req)
	if err != nil {
		return HostOperationMission{}, err
	}
	if err := o.store.SaveMission(ctx, revised); err != nil {
		return HostOperationMission{}, err
	}
	o.appendMissionAudit(ctx, revised.ID, TranscriptItem{
		Type:    TranscriptItemManagerMessage,
		Content: "host operation plan revised",
		Status:  string(revised.Status),
		Payload: map[string]any{
			"missionId": revised.ID,
			"planId":    revised.Plan.ID,
			"version":   revised.Plan.Version,
			"reason":    req.Reason,
		},
	})
	return o.store.GetMission(ctx, mission.ID)
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
		assignment = bindAssignmentPlanStep(assignment, mission)
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
			ChildAgentID:         childAgentIDFor(mission.ID, assignment.HostID),
			MissionID:            mission.ID,
			ParentAgentID:        firstNonEmptyString(assignment.ParentAgentID, mission.ManagerAgentID),
			SessionID:            firstNonEmptyString(assignment.SessionID, "host-child:"+mission.ID+":"+assignment.HostID),
			HostID:               assignment.HostID,
			HostAddress:          assignment.HostAddress,
			HostDisplayName:      assignment.HostDisplayName,
			Role:                 assignment.Role,
			Task:                 assignment.Task,
			PlanStepID:           firstNonEmptyString(assignment.PlanStepID, "unplanned"),
			Constraints:          append([]string(nil), assignment.Constraints...),
			RiskLevel:            assignment.RiskLevel,
			EvidenceRequirements: append([]string(nil), assignment.EvidenceRequirements...),
		}
		step := planStepByID(mission.Plan.Steps, req.PlanStepID)
		if len(req.EvidenceRequirements) > 0 {
			step.EvidenceRequired = append([]string(nil), req.EvidenceRequirements...)
		}
		if req.RiskLevel != "" {
			step.RiskLevel = req.RiskLevel
		}
		runtimeCtx, decisionTrace, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
			MissionID:          req.MissionID,
			ParentAgentID:      req.ParentAgentID,
			HostAgentID:        req.ChildAgentID,
			SessionID:          req.SessionID,
			HostID:             req.HostID,
			HostAddress:        req.HostAddress,
			HostDisplayName:    req.HostDisplayName,
			PlanStep:           step,
			Goal:               req.Task,
			Constraints:        req.Constraints,
			AllowedToolScopes:  []string{"host_command"},
			AllowedSkillScopes: []string{"host_bound"},
			AllowedMCPScope:    []string{"bound_host", "authorized_artifact"},
			CompletionContract: "HostTaskReport must cite validated evidence refs from HostCommandTool or sanitized artifacts.",
		})
		if err != nil {
			return nil, err
		}
		req.RuntimeContext = runtimeCtx
		req.ContextSummary = runtimeCtx.Goal
		req.ContextRefs = append([]ContextRef(nil), runtimeCtx.ContextRefs...)
		req.ContextDecisionTraceID = decisionTrace.SourceID
		o.appendMissionAudit(ctx, mission.ID, TranscriptItem{
			Type:    TranscriptItemManagerMessage,
			Content: "host agent context built",
			Status:  "host_agent.context.built",
			Payload: map[string]any{
				"missionId":              mission.ID,
				"childAgentId":           req.ChildAgentID,
				"hostId":                 req.HostID,
				"planStepId":             req.PlanStepID,
				"contextDecisionTraceId": decisionTrace.SourceID,
			},
		})
		if o.scheduler != nil {
			decision, err := o.scheduler.Schedule(ctx, HostSubTask{
				ID:                   req.ChildAgentID + ":" + req.PlanStepID,
				MissionID:            req.MissionID,
				PlanStepID:           req.PlanStepID,
				HostAgentID:          req.ChildAgentID,
				HostID:               req.HostID,
				Goal:                 req.Task,
				Constraints:          req.Constraints,
				ActionType:           step.ActionType,
				RiskLevel:            req.RiskLevel,
				EvidenceRequirements: req.EvidenceRequirements,
			})
			if err != nil {
				return nil, err
			}
			if decision.Status == HostSubTaskStatusQueued || decision.Status == HostSubTaskStatusCancelled || decision.Status == HostSubTaskStatusSuperseded {
				o.appendMissionAudit(ctx, mission.ID, TranscriptItem{
					Type:    TranscriptItemManagerMessage,
					Content: "host subtask scheduled",
					Status:  "manager.host_subtask." + string(decision.Status),
					Payload: map[string]any{
						"missionId":       mission.ID,
						"childAgentId":    req.ChildAgentID,
						"hostId":          req.HostID,
						"planStepId":      req.PlanStepID,
						"activeSubTaskId": decision.ActiveSubTaskID,
						"blockingReason":  decision.BlockingReason,
					},
				})
				continue
			}
		}
		if o.messages != nil {
			_, _ = o.messages.Append(ctx, AgentMessage{
				MissionID:     mission.ID,
				FromAgentID:   firstNonEmptyString(req.ParentAgentID, "manager"),
				ToAgentID:     req.ChildAgentID,
				Type:          AgentMessageHostSubTaskAssigned,
				CorrelationID: req.PlanStepID,
				Payload: HostSubTaskAssignedPayload{
					SubTaskID:              req.ChildAgentID + ":" + req.PlanStepID,
					RuntimeContextRef:      decisionTrace.SourceID,
					ContextDecisionTraceID: decisionTrace.SourceID,
					SourcePlanStepID:       req.PlanStepID,
					Summary:                runtimeCtx.Goal,
				},
				SourceRefs: []string{decisionTrace.SourceID},
			})
		}
		child, err := o.spawner.SpawnHostChild(ctx, req)
		if err != nil {
			observability.RecordOpsMetric(observability.OpsMetricHostAgentCreation, false)
			return nil, err
		}
		child = normalizeSpawnedChild(child, req)
		if current, getErr := o.store.GetChildAgent(ctx, child.ID); getErr == nil {
			child = mergeSpawnedChildUpdate(current, child)
		}
		if err := o.store.SaveChildAgent(ctx, child); err != nil {
			observability.RecordOpsMetric(observability.OpsMetricHostAgentCreation, false)
			return nil, err
		}
		observability.RecordOpsMetric(observability.OpsMetricHostAgentCreation, true)
		_ = o.attachChildToPlanStep(ctx, mission.ID, assignment.PlanStepID, child.ID)
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
		o.appendMissionAudit(ctx, mission.ID, TranscriptItem{
			Type:    TranscriptItemManagerMessage,
			Content: "host child agent created",
			Status:  string(child.Status),
			Payload: map[string]any{
				"missionId":    mission.ID,
				"childAgentId": child.ID,
				"hostId":       assignment.HostID,
				"planStepId":   assignment.PlanStepID,
			},
		})
		childrenByHost[key] = child
		children = append(children, child)
	}
	return children, nil
}

func planStepByID(steps []PlanStep, stepID string) PlanStep {
	stepID = strings.TrimSpace(stepID)
	for _, step := range steps {
		if strings.TrimSpace(step.ID) == stepID {
			return step
		}
	}
	return PlanStep{ID: stepID, HostIDs: nil}
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

func (o *Orchestrator) appendMissionAudit(ctx context.Context, missionID string, item TranscriptItem) {
	if o == nil || o.transcript == nil || strings.TrimSpace(missionID) == "" {
		return
	}
	_ = o.transcript.Append(ctx, MissionAuditTranscriptID(missionID), item)
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

func bindAssignmentPlanStep(assignment ChildAgentAssignment, mission HostOperationMission) ChildAgentAssignment {
	if assignment.PlanStepID != "" {
		return assignment
	}
	key := assignmentHostKey(assignment)
	for _, step := range mission.Plan.Steps {
		for _, hostID := range step.HostIDs {
			if normalizedHostKey(hostID) != key {
				continue
			}
			assignment.PlanStepID = step.ID
			assignment.RiskLevel = firstNonEmptyRisk(assignment.RiskLevel, step.RiskLevel)
			if len(assignment.EvidenceRequirements) == 0 {
				assignment.EvidenceRequirements = append([]string(nil), step.EvidenceRequired...)
			}
			return assignment
		}
	}
	return assignment
}

func (o *Orchestrator) attachChildToPlanStep(ctx context.Context, missionID, planStepID, childAgentID string) error {
	if o == nil || o.store == nil || strings.TrimSpace(planStepID) == "" || strings.TrimSpace(childAgentID) == "" {
		return nil
	}
	mission, err := o.store.GetMission(ctx, strings.TrimSpace(missionID))
	if err != nil {
		return err
	}
	changed := false
	for i := range mission.Plan.Steps {
		if strings.TrimSpace(mission.Plan.Steps[i].ID) != strings.TrimSpace(planStepID) {
			continue
		}
		if !stringSliceContains(mission.Plan.Steps[i].ChildAgentIDs, childAgentID) {
			mission.Plan.Steps[i].ChildAgentIDs = append(mission.Plan.Steps[i].ChildAgentIDs, childAgentID)
			changed = true
		}
		if mission.Plan.Steps[i].Status == PlanStepStatusPending {
			mission.Plan.Steps[i].Status = PlanStepStatusRunning
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return o.store.SaveMission(ctx, mission)
}

func firstNonEmptyRisk(values ...opssemantic.OpsRiskLevel) opssemantic.OpsRiskLevel {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeSpawnedChild(child HostChildAgent, req SpawnHostChildRequest) HostChildAgent {
	child.ID = firstNonEmptyString(child.ID, req.ChildAgentID)
	child.MissionID = firstNonEmptyString(child.MissionID, req.MissionID)
	child.ParentAgentID = firstNonEmptyString(child.ParentAgentID, req.ParentAgentID)
	child.SessionID = firstNonEmptyString(child.SessionID, req.SessionID)
	child.HostID = firstNonEmptyString(child.HostID, req.HostID)
	child.HostAddress = firstNonEmptyString(child.HostAddress, req.HostAddress)
	child.HostDisplayName = firstNonEmptyString(child.HostDisplayName, req.HostDisplayName)
	child.Role = firstNonEmptyString(child.Role, req.Role)
	child.Task = firstNonEmptyString(child.Task, req.Task)
	if req.PlanStepID != "" && !stringSliceContains(child.PlanStepIDs, req.PlanStepID) {
		child.PlanStepIDs = append(child.PlanStepIDs, req.PlanStepID)
	}
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
	update.HostDisplayName = firstNonEmptyString(update.HostDisplayName, current.HostDisplayName)
	update.Role = firstNonEmptyString(update.Role, current.Role)
	update.Task = firstNonEmptyString(update.Task, current.Task)
	update.PlanStepIDs = append([]string(nil), current.PlanStepIDs...)
	update.StartedAt = firstNonZeroTime(update.StartedAt, current.StartedAt)
	return update
}

func mergeSpawnedChildUpdate(current HostChildAgent, update HostChildAgent) HostChildAgent {
	merged := mergeChildAgentUpdate(current, update)
	if len(current.PlanStepIDs) == 0 {
		merged.PlanStepIDs = append([]string(nil), update.PlanStepIDs...)
	}
	if isTerminalChildAgentStatus(current.Status) && !isTerminalChildAgentStatus(update.Status) {
		merged.Status = current.Status
		merged.LastOutputPreview = firstNonEmptyString(current.LastOutputPreview, merged.LastOutputPreview)
		merged.Error = firstNonEmptyString(current.Error, merged.Error)
		merged.UpdatedAt = firstNonZeroTime(current.UpdatedAt, merged.UpdatedAt)
		if current.CompletedAt != nil {
			completedAt := *current.CompletedAt
			merged.CompletedAt = &completedAt
		}
	}
	return merged
}

func isTerminalChildAgentStatus(status HostChildAgentStatus) bool {
	switch status {
	case HostChildAgentStatusCompleted, HostChildAgentStatusFailed, HostChildAgentStatusCancelled:
		return true
	default:
		return false
	}
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
