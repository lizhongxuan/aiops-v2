package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	WorkflowErrorCodeInUse                    = "workflow_in_use"
	WorkflowErrorCodeReferencedByCandidates   = "workflow_referenced_by_candidates"
	WorkflowErrorCodeVersionLocked            = "workflow_version_locked"
	WorkflowErrorCodeDigestMismatch           = "workflow_digest_mismatch"
	workflowReferenceStatusVerified           = "verified"
	workflowReferenceStatusCandidate          = "candidate"
	workflowReferenceStatusDraft              = "draft"
	workflowReferenceStatusManualCandidate    = "manual_candidate"
	workflowReferenceStatusManualDraft        = "manual_draft"
	workflowReferenceStatusOpsManualCandidate = "ops_manual_candidate"
)

var (
	ErrWorkflowInUse                  = fmt.Errorf("%w: workflow is referenced by verified ops manual", ErrConflict)
	ErrWorkflowReferencedByCandidates = fmt.Errorf("%w: workflow is referenced by draft or candidate ops manual", ErrConflict)
	ErrWorkflowVersionLocked          = fmt.Errorf("%w: workflow version is locked by verified ops manual", ErrConflict)
	ErrWorkflowDigestMismatch         = fmt.Errorf("%w: workflow digest mismatch", ErrConflict)
)

type WorkflowReferenceChecker interface {
	ReferencesForWorkflow(ctx context.Context, workflowID string) ([]WorkflowReference, error)
}

type WorkflowReference struct {
	ManualID string `json:"manual_id"`
	Status   string `json:"status"`
	Title    string `json:"title,omitempty"`
}

type WorkflowDeleteDecision struct {
	Allowed    bool                `json:"allowed"`
	ErrorCode  string              `json:"error_code,omitempty"`
	Message    string              `json:"message,omitempty"`
	References []WorkflowReference `json:"references,omitempty"`
}

type WorkflowGuardError struct {
	code       string
	message    string
	cause      error
	references []WorkflowReference
}

func (e *WorkflowGuardError) Error() string {
	if strings.TrimSpace(e.message) != "" {
		return e.message
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return e.code
}

func (e *WorkflowGuardError) Unwrap() error {
	return e.cause
}

func (e *WorkflowGuardError) Code() string {
	return e.code
}

func (e *WorkflowGuardError) Message() string {
	return e.message
}

func (e *WorkflowGuardError) WorkflowReferences() []WorkflowReference {
	return append([]WorkflowReference{}, e.references...)
}

func (s *WorkflowService) SetWorkflowReferenceChecker(checker WorkflowReferenceChecker) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.referenceChecker = checker
}

func (s *WorkflowService) SetWorkflowReferenceGuardMode(mode WorkflowReferenceGuardMode) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.referenceGuardMode = normalizeWorkflowReferenceGuardMode(mode)
}

func (s *WorkflowService) workflowReferenceGuardMode() WorkflowReferenceGuardMode {
	if s == nil {
		return WorkflowReferenceGuardModeEnforce
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return normalizeWorkflowReferenceGuardMode(s.referenceGuardMode)
}

func normalizeWorkflowReferenceGuardMode(mode WorkflowReferenceGuardMode) WorkflowReferenceGuardMode {
	switch WorkflowReferenceGuardMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case WorkflowReferenceGuardModeWarn:
		return WorkflowReferenceGuardModeWarn
	default:
		return WorkflowReferenceGuardModeEnforce
	}
}

func (s *WorkflowService) workflowReferences(ctx context.Context, workflowID string) ([]WorkflowReference, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	checker := s.referenceChecker
	s.mu.Unlock()
	if checker == nil {
		return nil, nil
	}
	refs, err := checker.ReferencesForWorkflow(ctx, strings.TrimSpace(workflowID))
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowReference, 0, len(refs))
	for _, ref := range refs {
		ref.ManualID = strings.TrimSpace(ref.ManualID)
		ref.Status = strings.TrimSpace(ref.Status)
		ref.Title = strings.TrimSpace(ref.Title)
		if ref.ManualID == "" && ref.Status == "" {
			continue
		}
		out = append(out, ref)
	}
	return out, nil
}

func (s *WorkflowService) WorkflowReferenceWarnings(ctx context.Context, workflowID string) ([]WorkflowGuardWarning, error) {
	if s.workflowReferenceGuardMode() != WorkflowReferenceGuardModeWarn {
		return nil, nil
	}
	refs, err := s.workflowReferences(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if !hasVerifiedWorkflowReference(refs) {
		return nil, nil
	}
	return []WorkflowGuardWarning{{
		Code:       WorkflowErrorCodeVersionLocked,
		Message:    "该 workflow 被 verified 运维手册引用；当前为 warning 模式，已允许编辑，但执行前仍会校验 workflow_digest",
		References: append([]WorkflowReference{}, refs...),
	}}, nil
}

func (s *WorkflowService) ensureWorkflowVersionMutable(ctx context.Context, workflowID string) error {
	refs, err := s.workflowReferences(ctx, workflowID)
	if err != nil {
		return err
	}
	if hasVerifiedWorkflowReference(refs) {
		if s.workflowReferenceGuardMode() == WorkflowReferenceGuardModeWarn {
			return nil
		}
		return workflowVersionLockedError(refs)
	}
	return nil
}

func (s *WorkflowService) ensureWorkflowDeleteAllowed(ctx context.Context, workflowID string) error {
	refs, err := s.workflowReferences(ctx, workflowID)
	if err != nil {
		return err
	}
	return deleteDecisionError(EvaluateWorkflowDeleteReferences(refs))
}

func EvaluateWorkflowDeleteReferences(refs []WorkflowReference) WorkflowDeleteDecision {
	if len(refs) == 0 {
		return WorkflowDeleteDecision{Allowed: true}
	}
	if hasVerifiedWorkflowReference(refs) {
		return WorkflowDeleteDecision{
			Allowed:    false,
			ErrorCode:  WorkflowErrorCodeInUse,
			Message:    "该工作流被 verified 运维手册引用，不能删除",
			References: append([]WorkflowReference{}, refs...),
		}
	}
	return WorkflowDeleteDecision{
		Allowed:    false,
		ErrorCode:  WorkflowErrorCodeReferencedByCandidates,
		Message:    "该工作流被 draft/candidate 运维手册引用，请先解除引用或删除候选",
		References: append([]WorkflowReference{}, refs...),
	}
}

func hasVerifiedWorkflowReference(refs []WorkflowReference) bool {
	for _, ref := range refs {
		if strings.EqualFold(strings.TrimSpace(ref.Status), workflowReferenceStatusVerified) {
			return true
		}
	}
	return false
}

func deleteDecisionError(decision WorkflowDeleteDecision) error {
	if decision.Allowed {
		return nil
	}
	switch decision.ErrorCode {
	case WorkflowErrorCodeInUse:
		return &WorkflowGuardError{
			code:       WorkflowErrorCodeInUse,
			message:    decision.Message,
			cause:      ErrWorkflowInUse,
			references: decision.References,
		}
	default:
		return &WorkflowGuardError{
			code:       WorkflowErrorCodeReferencedByCandidates,
			message:    decision.Message,
			cause:      ErrWorkflowReferencedByCandidates,
			references: decision.References,
		}
	}
}

func workflowVersionLockedError(refs []WorkflowReference) error {
	return &WorkflowGuardError{
		code:       WorkflowErrorCodeVersionLocked,
		message:    "该 workflow 被 verified 运维手册引用，请创建新版本",
		cause:      ErrWorkflowVersionLocked,
		references: refs,
	}
}

func workflowDigestMismatchError() error {
	return &WorkflowGuardError{
		code:    WorkflowErrorCodeDigestMismatch,
		message: "workflow_digest 与当前工作流内容不一致，请重新审核运维手册",
		cause:   ErrWorkflowDigestMismatch,
	}
}

func DigestWorkflowContent(raw []byte) string {
	return workflowYAMLChecksum(raw)
}

func VerifyWorkflowDigest(expected string, raw []byte) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	if expected != DigestWorkflowContent(raw) {
		return workflowDigestMismatchError()
	}
	return nil
}

func IsWorkflowDigestMismatch(err error) bool {
	return errors.Is(err, ErrWorkflowDigestMismatch)
}
