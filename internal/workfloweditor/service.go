package workfloweditor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"runner/workflow/visual"
)

type WorkflowRecord struct {
	ID        string
	Revision  string
	Graph     visual.Graph
	UpdatedAt time.Time
}

type WorkflowStore interface {
	GetWorkflow(ctx context.Context, id string) (WorkflowRecord, error)
	SaveWorkflow(ctx context.Context, record WorkflowRecord) error
}

type MemoryWorkflowStore struct {
	mu        sync.Mutex
	workflows map[string]WorkflowRecord
}

func NewMemoryWorkflowStore() *MemoryWorkflowStore {
	return &MemoryWorkflowStore{workflows: map[string]WorkflowRecord{}}
}

func (s *MemoryWorkflowStore) PutWorkflow(record WorkflowRecord) WorkflowRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workflows == nil {
		s.workflows = map[string]WorkflowRecord{}
	}
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = strings.TrimSpace(record.Graph.Workflow.Name)
	}
	if record.Revision == "" {
		record.Revision = RevisionDigest(record.Graph)
	}
	record.UpdatedAt = time.Now().UTC()
	s.workflows[record.ID] = record
	return record
}

func (s *MemoryWorkflowStore) GetWorkflow(_ context.Context, id string) (WorkflowRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.workflows[strings.TrimSpace(id)]
	if !ok {
		return WorkflowRecord{}, fmt.Errorf("workflow %q not found", id)
	}
	record.Graph = CloneGraph(record.Graph)
	return record, nil
}

func (s *MemoryWorkflowStore) SaveWorkflow(_ context.Context, record WorkflowRecord) error {
	s.PutWorkflow(record)
	return nil
}

type Service struct {
	store           WorkflowStore
	sessions        *SessionStore
	planner         WorkflowEditPlanner
	mu              sync.Mutex
	appliedPatchIDs map[string]bool
	undoSnapshots   map[string]visual.Graph
	patchQueue      map[string]WorkflowPatch
}

type ServiceOption func(*Service)

func WithEditPlanner(planner WorkflowEditPlanner) ServiceOption {
	return func(s *Service) {
		if planner != nil {
			s.planner = planner
		}
	}
}

func NewService(store WorkflowStore, opts ...ServiceOption) *Service {
	if store == nil {
		store = NewMemoryWorkflowStore()
	}
	service := &Service{
		store:           store,
		sessions:        NewSessionStore(),
		planner:         DefaultWorkflowEditPlanner{},
		appliedPatchIDs: map[string]bool{},
		undoSnapshots:   map[string]visual.Graph{},
		patchQueue:      map[string]WorkflowPatch{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) Sessions() *SessionStore {
	return s.sessions
}

func (s *Service) CreateSession(_ context.Context, req CreateSessionRequest) (WorkflowAISession, error) {
	return s.sessions.Start(req), nil
}

func (s *Service) GetSnapshot(ctx context.Context, req GetSnapshotRequest) (WorkflowSnapshot, error) {
	record, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return WorkflowSnapshot{}, err
	}
	describe := describeGraph(record.ID, record.Revision, record.Graph)
	return WorkflowSnapshot{
		WorkflowID:     record.ID,
		Revision:       record.Revision,
		RevisionDigest: RevisionDigest(record.Graph),
		Graph:          record.Graph,
		Validation:     WorkflowPatchValidation{Valid: true},
		ManualBinding:  manualBindingFromGraph(record.Graph),
		Describe:       describe,
	}, nil
}

func (s *Service) GetStep(ctx context.Context, req GetStepRequest) (WorkflowStepSnapshot, error) {
	record, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return WorkflowStepSnapshot{}, err
	}
	for i := range record.Graph.Nodes {
		if record.Graph.Nodes[i].ID == req.NodeID {
			node := record.Graph.Nodes[i]
			return WorkflowStepSnapshot{WorkflowID: record.ID, Revision: record.Revision, Node: &node}, nil
		}
	}
	return WorkflowStepSnapshot{}, fmt.Errorf("node %q not found", req.NodeID)
}

func (s *Service) Describe(ctx context.Context, req DescribeRequest) (DescribeResult, error) {
	if req.Graph != nil {
		return describeGraph(req.WorkflowID, RevisionDigest(*req.Graph), *req.Graph), nil
	}
	record, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return DescribeResult{}, err
	}
	return describeGraph(record.ID, record.Revision, record.Graph), nil
}

func (s *Service) ProposeEditPlan(ctx context.Context, req ProposeEditPlanRequest) (WorkflowEditPlan, error) {
	planner := s.planner
	if planner == nil {
		planner = DefaultWorkflowEditPlanner{}
	}
	record, _ := s.store.GetWorkflow(ctx, strings.TrimSpace(req.WorkflowID))
	describe := DescribeResult{}
	if strings.TrimSpace(record.ID) != "" {
		describe = describeGraph(record.ID, record.Revision, record.Graph)
	}
	plan, err := planner.BuildWorkflowEditPlan(ctx, WorkflowEditPlanningRequest{
		WorkflowID:      strings.TrimSpace(req.WorkflowID),
		DrawerSessionID: strings.TrimSpace(req.DrawerSessionID),
		Message:         strings.TrimSpace(req.Message),
		Describe:        describe,
	})
	if err != nil {
		return WorkflowEditPlan{}, err
	}
	plan, err = normalizeWorkflowEditPlan(plan, req)
	if err != nil {
		return WorkflowEditPlan{}, err
	}
	if strings.TrimSpace(req.DrawerSessionID) != "" {
		if session, ok := s.sessions.Get(req.DrawerSessionID); ok {
			session.CurrentPlan = &plan
			session.StepBudget.RemainingPlanItems = len(plan.Items)
			s.sessions.Save(session)
		}
	}
	return plan, nil
}

func (s *Service) ProposePatch(_ context.Context, req ProposePatchRequest) (WorkflowPatch, error) {
	if strings.TrimSpace(req.DrawerSessionID) != "" {
		if _, err := s.sessions.RecordPatchReview(req.DrawerSessionID); err != nil {
			return WorkflowPatch{}, err
		}
	}
	patch := WorkflowPatch{
		ID:           stableID("patch", firstNonEmpty(req.PlanID, req.ItemID, req.Message, time.Now().UTC().String())),
		WorkflowID:   strings.TrimSpace(req.WorkflowID),
		BaseRevision: strings.TrimSpace(req.BaseRevision),
		Summary:      firstNonEmpty(req.Message, "AI proposed workflow patch"),
		Reason:       strings.TrimSpace(req.Message),
		Operations: []WorkflowPatchOperation{{
			Op: PatchUpdateWorkflowMetadata,
			Fields: map[string]any{
				"last_ai_plan_item": firstNonEmpty(req.ItemID, req.Message, "workflow_edit"),
			},
		}},
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.patchQueue[patch.ID] = patch
	s.mu.Unlock()
	if strings.TrimSpace(req.DrawerSessionID) != "" {
		if session, ok := s.sessions.Get(req.DrawerSessionID); ok {
			session.PatchQueue = append(session.PatchQueue, patch)
			s.sessions.Save(session)
		}
	}
	return patch, nil
}

func (s *Service) ValidatePatch(_ context.Context, req ValidatePatchRequest) (WorkflowPatchValidation, error) {
	return ValidateWorkflowPatch(req), nil
}

func (s *Service) PreviewPatch(ctx context.Context, req PreviewPatchRequest) (WorkflowPatchPreview, error) {
	workflowID := firstNonEmpty(req.WorkflowID, req.Patch.WorkflowID)
	record, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return WorkflowPatchPreview{}, err
	}
	next, effect, err := ApplyPatchToGraph(record.Graph, req.Patch)
	if err != nil {
		return WorkflowPatchPreview{}, err
	}
	return WorkflowPatchPreview{PatchID: req.Patch.ID, Graph: next, Effect: effect}, nil
}

func (s *Service) DetectPatchEffect(ctx context.Context, req DetectPatchEffectRequest) (PatchEffectResult, error) {
	workflowID := firstNonEmpty(req.WorkflowID, req.Patch.WorkflowID)
	record, err := s.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return PatchEffectResult{}, err
	}
	s.mu.Lock()
	applied := copyBoolMap(s.appliedPatchIDs)
	s.mu.Unlock()
	return DetectWorkflowPatchEffect(record.Graph, req.Patch, applied), nil
}

func (s *Service) ApplyPatch(ctx context.Context, req ApplyPatchRequest) (WorkflowPatchResult, error) {
	if strings.TrimSpace(req.WorkflowID) == "" {
		return WorkflowPatchResult{}, fmt.Errorf("workflow_id is required")
	}
	if strings.TrimSpace(req.BaseRevision) == "" {
		return WorkflowPatchResult{}, fmt.Errorf("base_revision is required")
	}
	if strings.TrimSpace(req.UserConfirmationID) == "" {
		return WorkflowPatchResult{}, fmt.Errorf("user_confirmation_id is required")
	}
	if strings.TrimSpace(req.DrawerSessionID) == "" {
		return WorkflowPatchResult{}, fmt.Errorf("drawer_session_id is required")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return WorkflowPatchResult{}, fmt.Errorf("reason is required")
	}
	patch := req.Patch
	if strings.TrimSpace(patch.ID) == "" {
		patch.ID = strings.TrimSpace(req.PatchID)
	}
	if strings.TrimSpace(patch.ID) == "" {
		return WorkflowPatchResult{}, fmt.Errorf("patch_id is required")
	}
	validation := ValidateWorkflowPatch(ValidatePatchRequest{
		WorkflowID:                req.WorkflowID,
		BaseRevision:              req.BaseRevision,
		Patch:                     patch,
		AllowFullGraphReplacement: req.AllowFullGraphReplacement,
		SecondConfirmationID:      req.SecondConfirmationID,
	})
	if !validation.Valid {
		return WorkflowPatchResult{}, fmt.Errorf("invalid patch: %s", strings.Join(validation.Errors, "; "))
	}
	record, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return WorkflowPatchResult{}, err
	}
	if record.Revision != req.BaseRevision {
		return WorkflowPatchResult{}, fmt.Errorf("stale revision: got %s want %s", req.BaseRevision, record.Revision)
	}
	s.mu.Lock()
	if s.appliedPatchIDs[patch.ID] {
		s.mu.Unlock()
		return WorkflowPatchResult{}, fmt.Errorf("duplicate patch %q", patch.ID)
	}
	s.mu.Unlock()
	next, effect, err := ApplyPatchToGraph(record.Graph, patch)
	if err != nil {
		return WorkflowPatchResult{}, err
	}
	revisionAfter := RevisionDigest(next)
	nextRecord := WorkflowRecord{ID: record.ID, Revision: revisionAfter, Graph: next, UpdatedAt: time.Now().UTC()}
	if err := s.store.SaveWorkflow(ctx, nextRecord); err != nil {
		return WorkflowPatchResult{}, err
	}
	checkpoint := UndoCheckpointRef{
		ID:             stableID("undo", patch.ID+"-"+record.Revision),
		WorkflowID:     record.ID,
		PatchID:        patch.ID,
		RevisionBefore: record.Revision,
		RevisionAfter:  revisionAfter,
		CreatedAt:      time.Now().UTC(),
	}
	s.mu.Lock()
	s.appliedPatchIDs[patch.ID] = true
	s.undoSnapshots[checkpoint.ID] = record.Graph
	s.mu.Unlock()
	if session, ok := s.sessions.Get(req.DrawerSessionID); ok {
		session.WorkflowID = record.ID
		session.ActiveRevision = revisionAfter
		session.UndoStack = append(session.UndoStack, checkpoint)
		s.sessions.Save(session)
	}
	describe := describeGraph(record.ID, revisionAfter, next)
	return WorkflowPatchResult{
		PatchID:        patch.ID,
		WorkflowID:     record.ID,
		RevisionBefore: record.Revision,
		RevisionAfter:  revisionAfter,
		Effect:         effect,
		Describe:       describe,
		UndoCheckpoint: checkpoint,
		Audit: []WorkflowAuditEvent{{
			Type:               "workflow.patch.applied",
			PatchID:            patch.ID,
			UserConfirmationID: req.UserConfirmationID,
			DrawerSessionID:    req.DrawerSessionID,
			Reason:             req.Reason,
			CreatedAt:          time.Now().UTC(),
		}},
	}, nil
}

func (s *Service) UndoLastAIPatch(ctx context.Context, req UndoLastAIPatchRequest) (UndoPatchResult, error) {
	session, ok := s.sessions.Get(req.DrawerSessionID)
	if !ok {
		return UndoPatchResult{}, fmt.Errorf("workflow ai session %q not found", req.DrawerSessionID)
	}
	if len(session.UndoStack) == 0 {
		return UndoPatchResult{}, fmt.Errorf("undo stack is empty")
	}
	checkpoint := session.UndoStack[len(session.UndoStack)-1]
	record, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return UndoPatchResult{}, err
	}
	if record.Revision != checkpoint.RevisionAfter {
		return UndoPatchResult{}, fmt.Errorf("cannot undo after manual interleaving: current revision %s checkpoint revision %s", record.Revision, checkpoint.RevisionAfter)
	}
	s.mu.Lock()
	previous, ok := s.undoSnapshots[checkpoint.ID]
	s.mu.Unlock()
	if !ok {
		return UndoPatchResult{}, fmt.Errorf("undo checkpoint snapshot %q not found", checkpoint.ID)
	}
	revisionAfter := RevisionDigest(previous)
	if err := s.store.SaveWorkflow(ctx, WorkflowRecord{ID: req.WorkflowID, Revision: revisionAfter, Graph: previous, UpdatedAt: time.Now().UTC()}); err != nil {
		return UndoPatchResult{}, err
	}
	session.UndoStack = session.UndoStack[:len(session.UndoStack)-1]
	session.ActiveRevision = revisionAfter
	s.sessions.Save(session)
	return UndoPatchResult{
		WorkflowID:     req.WorkflowID,
		RevisionBefore: record.Revision,
		RevisionAfter:  revisionAfter,
		UndoCheckpoint: checkpoint,
		Describe:       describeGraph(req.WorkflowID, revisionAfter, previous),
	}, nil
}

func (s *Service) CreateDraftFromConfirmedPlan(ctx context.Context, req WorkflowDraftFromPlanRequest) (WorkflowDraftFromPlanResult, error) {
	if strings.TrimSpace(req.UserConfirmationID) == "" {
		return WorkflowDraftFromPlanResult{}, fmt.Errorf("user_confirmation_id is required for confirmed workflow draft creation")
	}
	if len(req.Plan.Nodes) == 0 {
		return WorkflowDraftFromPlanResult{}, fmt.Errorf("confirmed workflow generation plan is required")
	}
	adapter := WorkflowGenAdapter{}
	result, err := adapter.CreateDraftFromPlan(ctx, req)
	if err != nil {
		return WorkflowDraftFromPlanResult{}, err
	}
	workflowID := firstNonEmpty(result.Graph.Workflow.Name, stableID("workflow", req.Plan.Title))
	result.WorkflowID = workflowID
	result.Revision = RevisionDigest(result.Graph)
	result.Validation = WorkflowPatchValidation{Valid: true}
	result.Describe = describeGraph(workflowID, result.Revision, result.Graph)
	result.Published = false
	result.Executed = false
	if err := s.store.SaveWorkflow(ctx, WorkflowRecord{ID: workflowID, Revision: result.Revision, Graph: result.Graph, UpdatedAt: time.Now().UTC()}); err != nil {
		return WorkflowDraftFromPlanResult{}, err
	}
	if strings.TrimSpace(req.DrawerSessionID) != "" {
		if session, ok := s.sessions.Get(req.DrawerSessionID); ok {
			session.WorkflowID = workflowID
			session.ActiveRevision = result.Revision
			s.sessions.Save(session)
		}
	}
	return result, nil
}

func describeGraph(workflowID, revision string, graph visual.Graph) DescribeResult {
	nodeIDs := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	return DescribeResult{
		WorkflowID: workflowID,
		Revision:   revision,
		Summary:    fmt.Sprintf("%s has %d nodes and %d edges", firstNonEmpty(graph.Workflow.Name, workflowID, "workflow"), len(graph.Nodes), len(graph.Edges)),
		NodeCount:  len(graph.Nodes),
		EdgeCount:  len(graph.Edges),
		NodeIDs:    sortedUnique(nodeIDs),
	}
}

func manualBindingFromGraph(graph visual.Graph) *ManualBindingSummary {
	if graph.UI == nil {
		return nil
	}
	raw, ok := graph.UI["ops_manual_candidate"].(map[string]any)
	if !ok {
		return nil
	}
	return &ManualBindingSummary{
		CandidateID:    stringFromAny(raw["candidate_id"]),
		ManualID:       stringFromAny(raw["manual_id"]),
		ReviewStatus:   stringFromAny(raw["review_status"]),
		WorkflowDigest: stringFromAny(raw["workflow_digest"]),
	}
}

func stableID(prefix, seed string) string {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		seed = "default"
	}
	seed = strings.ToLower(seed)
	var b strings.Builder
	for _, r := range seed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 && b.String()[b.Len()-1] != '-' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "item"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return prefix + "-" + out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
