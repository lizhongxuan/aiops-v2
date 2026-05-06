package runbooks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
)

type Service struct {
	catalog *Catalog
	signer  *actionproposal.Signer
	store   InstanceStore
	now     func() time.Time
}

type StartRequest struct {
	RunbookID  string         `json:"runbookId"`
	IncidentID string         `json:"incidentId,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
}

type NextActionRequest struct {
	InstanceID string `json:"instanceId"`
	SessionID  string `json:"sessionId"`
	TurnID     string `json:"turnId"`
}

type ObserveResultRequest struct {
	InstanceID    string         `json:"instanceId"`
	StepID        string         `json:"stepId"`
	ToolResultRef string         `json:"toolResultRef,omitempty"`
	EvidenceRef   string         `json:"evidenceRef,omitempty"`
	EvidencePatch map[string]any `json:"evidencePatch,omitempty"`
	Failed        bool           `json:"failed,omitempty"`
	FailureReason string         `json:"failureReason,omitempty"`
}

type CloseRequest struct {
	InstanceID string `json:"instanceId"`
	Reason     string `json:"reason,omitempty"`
}

type CloseSummary struct {
	InstanceID string   `json:"instanceId"`
	Status     string   `json:"status"`
	Completed  []string `json:"completed,omitempty"`
	Skipped    []string `json:"skipped,omitempty"`
	Failed     []string `json:"failed,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

func NewService(catalog *Catalog, signer *actionproposal.Signer, store InstanceStore, now func() time.Time) *Service {
	if store == nil {
		store = NewInMemoryInstanceStore()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{catalog: catalog, signer: signer, store: store, now: now}
}

func (s *Service) List() []Runbook {
	if s == nil {
		return nil
	}
	return s.catalog.List()
}

func (s *Service) GetRunbook(id string) (Runbook, bool) {
	if s == nil {
		return Runbook{}, false
	}
	return s.catalog.Get(id)
}

func (s *Service) Match(req MatchRequest) []Candidate {
	return s.catalog.Match(req)
}

func (s *Service) Start(req StartRequest) (RunbookInstance, error) {
	rb, ok := s.catalog.Get(req.RunbookID)
	if !ok {
		return RunbookInstance{}, fmt.Errorf("runbook %q not found", req.RunbookID)
	}
	now := s.now()
	instance := RunbookInstance{
		ID:            fmt.Sprintf("rbi_%d", now.UnixNano()),
		RunbookID:     rb.ID,
		IncidentID:    req.IncidentID,
		Status:        "running",
		Context:       cloneMap(req.Context),
		Evidence:      cloneMap(req.Evidence),
		StepProgress:  map[string]StepProgress{},
		CreatedAtUnix: now.Unix(),
		UpdatedAtUnix: now.Unix(),
	}
	for _, step := range rb.Steps {
		instance.StepProgress[step.ID] = StepProgress{State: StepPending}
	}
	s.store.Put(instance)
	return instance, nil
}

func (s *Service) NextAction(req NextActionRequest) (actionproposal.ActionProposal, bool, error) {
	instance, rb, err := s.load(req.InstanceID)
	if err != nil {
		return actionproposal.ActionProposal{}, false, err
	}
	step, ok, err := s.nextStep(&instance, rb)
	if err != nil || !ok {
		s.store.Put(instance)
		return actionproposal.ActionProposal{}, ok, err
	}
	rendered, err := RenderValue(step.Input, instance.Context)
	if err != nil {
		return actionproposal.ActionProposal{}, false, err
	}
	input, err := json.Marshal(rendered)
	if err != nil {
		return actionproposal.ActionProposal{}, false, err
	}
	inputHash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		return actionproposal.ActionProposal{}, false, err
	}
	expiresAt := s.now().Add(15 * time.Minute)
	risk := step.Risk
	if risk == "" {
		risk = rb.Risk
	}
	claims := actionproposal.ActionTokenClaims{
		SessionID:        req.SessionID,
		TurnID:           req.TurnID,
		IncidentID:       instance.IncidentID,
		ToolName:         step.Tool,
		InputHash:        inputHash,
		Source:           actionproposal.SourceRunbook,
		Risk:             risk,
		Reason:           firstNonEmpty(step.Title, step.ID),
		RunbookID:        rb.ID,
		RunbookStepID:    step.ID,
		RunbookStepTitle: step.Title,
		ExpectedEffect:   step.ExpectedEffect,
		Rollback:         step.Rollback,
		ExpiresAt:        expiresAt,
	}
	token, err := s.signer.Sign(claims)
	if err != nil {
		return actionproposal.ActionProposal{}, false, err
	}
	instance.StepProgress[step.ID] = StepProgress{State: StepProposed, Reason: "action proposal generated"}
	instance.UpdatedAtUnix = s.now().Unix()
	s.store.Put(instance)
	return actionproposal.ActionProposal{
		SessionID:        req.SessionID,
		TurnID:           req.TurnID,
		IncidentID:       instance.IncidentID,
		Source:           actionproposal.SourceRunbook,
		ToolName:         step.Tool,
		ToolInput:        input,
		Risk:             risk,
		ApprovalRequired: step.ApprovalRequired,
		Reason:           firstNonEmpty(step.Title, step.ID),
		RunbookID:        rb.ID,
		RunbookStepID:    step.ID,
		RunbookStepTitle: step.Title,
		ExpectedEffect:   step.ExpectedEffect,
		Rollback:         step.Rollback,
		Verification:     verificationProposal(step.Verify, instance.Context),
		ActionToken:      token,
		ExpiresAt:        expiresAt,
	}, true, nil
}

func (s *Service) ObserveResult(req ObserveResultRequest) error {
	instance, rb, err := s.load(req.InstanceID)
	if err != nil {
		return err
	}
	next, ok, err := s.nextStep(&instance, rb)
	if err != nil {
		return err
	}
	if ok && next.ID != req.StepID {
		return fmt.Errorf("step %q cannot be observed before required step %q", req.StepID, next.ID)
	}
	progress := instance.StepProgress[req.StepID]
	if req.Failed {
		progress.State = StepFailed
		progress.Reason = req.FailureReason
	} else {
		progress.State = StepObserved
	}
	progress.ToolResultRef = req.ToolResultRef
	progress.EvidenceRef = req.EvidenceRef
	instance.StepProgress[req.StepID] = progress
	if instance.Evidence == nil {
		instance.Evidence = map[string]any{}
	}
	for key, value := range req.EvidencePatch {
		instance.Evidence[key] = value
	}
	instance.UpdatedAtUnix = s.now().Unix()
	s.store.Put(instance)
	return nil
}

func (s *Service) Close(req CloseRequest) (CloseSummary, error) {
	instance, rb, err := s.load(req.InstanceID)
	if err != nil {
		return CloseSummary{}, err
	}
	summary := CloseSummary{InstanceID: instance.ID, Status: "closed", Reason: req.Reason}
	for _, step := range rb.Steps {
		switch instance.StepProgress[step.ID].State {
		case StepObserved:
			summary.Completed = append(summary.Completed, step.ID)
		case StepSkipped:
			summary.Skipped = append(summary.Skipped, step.ID)
		case StepFailed:
			summary.Failed = append(summary.Failed, step.ID)
		}
	}
	instance.Status = "closed"
	instance.UpdatedAtUnix = s.now().Unix()
	s.store.Put(instance)
	return summary, nil
}

func (s *Service) Instances(status string) []RunbookInstance {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.List(status)
}

func (s *Service) load(instanceID string) (RunbookInstance, Runbook, error) {
	instance, ok := s.store.Get(instanceID)
	if !ok {
		return RunbookInstance{}, Runbook{}, fmt.Errorf("runbook instance %q not found", instanceID)
	}
	rb, ok := s.catalog.Get(instance.RunbookID)
	if !ok {
		return RunbookInstance{}, Runbook{}, fmt.Errorf("runbook %q not found", instance.RunbookID)
	}
	return instance, rb, nil
}

func (s *Service) nextStep(instance *RunbookInstance, rb Runbook) (Step, bool, error) {
	for _, step := range rb.Steps {
		progress := instance.StepProgress[step.ID]
		if progress.State == StepObserved || progress.State == StepSkipped || progress.State == StepFailed {
			continue
		}
		conditionOK, err := EvalCondition(step.Condition, instance.Evidence)
		if err != nil {
			return Step{}, false, err
		}
		if !conditionOK {
			instance.StepProgress[step.ID] = StepProgress{State: StepSkipped, Reason: "condition=false"}
			continue
		}
		return step, true, nil
	}
	return Step{}, false, nil
}

func verificationProposal(verify []VerifyStep, context map[string]any) []actionproposal.VerificationStep {
	out := make([]actionproposal.VerificationStep, 0, len(verify))
	for _, item := range verify {
		rendered, err := RenderValue(item.Input, context)
		if err != nil {
			continue
		}
		data, _ := json.Marshal(rendered)
		out = append(out, actionproposal.VerificationStep{ToolName: item.Tool, Input: data})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
