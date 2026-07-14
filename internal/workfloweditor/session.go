package workfloweditor

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	sessionSchemaVersion      = "workflow_ai_session/v1"
	defaultPatchReviewBudget  = 3
	sessionStatusActive       = "active"
	sessionStatusBudgetPaused = "budget_paused"
)

type SessionStore struct {
	mu       sync.Mutex
	seq      int
	sessions map[string]WorkflowAISession
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: map[string]WorkflowAISession{}}
}

func (s *SessionStore) Start(req CreateSessionRequest) WorkflowAISession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]WorkflowAISession{}
	}
	id := strings.TrimSpace(req.DrawerSessionID)
	if id == "" {
		s.seq++
		id = fmt.Sprintf("workflow-ai-%d", s.seq)
	}
	now := time.Now().UTC()
	intent := req.Intent
	if intent == "" {
		intent = SessionIntentEdit
	}
	session := WorkflowAISession{
		SchemaVersion:  sessionSchemaVersion,
		ID:             id,
		WorkflowID:     strings.TrimSpace(req.WorkflowID),
		BaseRevision:   strings.TrimSpace(req.BaseRevision),
		ActiveRevision: strings.TrimSpace(req.BaseRevision),
		Intent:         intent,
		Status:         sessionStatusActive,
		StepBudget: StepBudget{
			MaxPatchReviewsPerTurn: defaultPatchReviewBudget,
		},
		ToolLogRef: "workflow-ai-tool-log/" + id,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.sessions[id] = session
	return session
}

func (s *SessionStore) Get(id string) (WorkflowAISession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(id)]
	return session, ok
}

func (s *SessionStore) Save(session WorkflowAISession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]WorkflowAISession{}
	}
	session.UpdatedAt = time.Now().UTC()
	s.sessions[strings.TrimSpace(session.ID)] = session
}

func (s *SessionStore) RecordPatchReview(id string) (WorkflowAISession, error) {
	session, ok := s.Get(id)
	if !ok {
		return WorkflowAISession{}, fmt.Errorf("workflow ai session %q not found", id)
	}
	if session.StepBudget.MaxPatchReviewsPerTurn <= 0 {
		session.StepBudget.MaxPatchReviewsPerTurn = defaultPatchReviewBudget
	}
	if session.StepBudget.UsedPatchReviews >= session.StepBudget.MaxPatchReviewsPerTurn {
		session.Status = sessionStatusBudgetPaused
		s.Save(session)
		return session, fmt.Errorf("budget_paused")
	}
	session.StepBudget.UsedPatchReviews++
	if session.StepBudget.UsedPatchReviews >= session.StepBudget.MaxPatchReviewsPerTurn {
		session.Status = sessionStatusBudgetPaused
	}
	s.Save(session)
	return session, nil
}

func (s *SessionStore) Continue(id string) (WorkflowAISession, error) {
	session, ok := s.Get(id)
	if !ok {
		return WorkflowAISession{}, fmt.Errorf("workflow ai session %q not found", id)
	}
	session.Status = sessionStatusActive
	session.StepBudget.UsedPatchReviews = 0
	s.Save(session)
	return session, nil
}
