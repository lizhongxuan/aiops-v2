package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

type interactionAPITestRuntime struct {
	runReq    runtimekernel.TurnRequest
	runCh     chan runtimekernel.TurnRequest
	resumeReq runtimekernel.ResumeRequest
	cancelReq runtimekernel.CancelRequest
}

func (r *interactionAPITestRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.runReq = req
	if r.runCh != nil {
		r.runCh <- req
	}
	return runtimekernel.TurnResult{
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "completed",
	}, nil
}

func waitForAPIRunTurn(t *testing.T, runtime *interactionAPITestRuntime) runtimekernel.TurnRequest {
	t.Helper()
	if runtime.runCh == nil {
		t.Fatal("runtime.runCh is nil")
	}
	select {
	case req := <-runtime.runCh:
		return req
	case <-time.After(time.Second):
		t.Fatal("RunTurn was not called")
		return runtimekernel.TurnRequest{}
	}
}

func (r *interactionAPITestRuntime) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	r.resumeReq = req
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (r *interactionAPITestRuntime) CancelTurn(_ context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	r.cancelReq = req
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "cancelled",
	}, nil
}

func TestChatAPI_StopApprovalDecisionAndChoiceAnswer(t *testing.T) {
	now := time.Now().UTC()
	runtime := &interactionAPITestRuntime{}
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-live", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-live",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:          "choice-1",
			SessionID:   session.ID,
			TurnID:      "turn-live",
			Iteration:   1,
			Sequence:    1,
			Kind:        "approval_needed",
			Lifecycle:   runtimekernel.TurnLifecycleSuspended,
			ResumeState: runtimekernel.TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-1",
			SessionID: session.ID,
			TurnID:    "turn-live",
			Iteration: 1,
			ToolName:  "exec_command",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	srv := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	stopResp, err := http.Post(ts.URL+"/api/v1/chat/stop", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/v1/chat/stop error = %v", err)
	}
	defer stopResp.Body.Close()
	if stopResp.StatusCode != http.StatusOK {
		t.Fatalf("stop status = %d, want 200", stopResp.StatusCode)
	}
	if runtime.cancelReq.SessionID != session.ID || runtime.cancelReq.TurnID != "turn-live" {
		t.Fatalf("CancelTurn request = %+v, want session %q turn-live", runtime.cancelReq, session.ID)
	}

	approvalBody, _ := json.Marshal(map[string]string{"decision": "accept"})
	approvalResp, err := http.Post(ts.URL+"/api/v1/approvals/approval-1/decision", "application/json", bytes.NewReader(approvalBody))
	if err != nil {
		t.Fatalf("POST /api/v1/approvals/:id/decision error = %v", err)
	}
	defer approvalResp.Body.Close()
	if approvalResp.StatusCode != http.StatusOK {
		t.Fatalf("approval decision status = %d, want 200", approvalResp.StatusCode)
	}
	if runtime.resumeReq.ApprovalID != "approval-1" || runtime.resumeReq.Decision != "approved" {
		t.Fatalf("ResumeTurn approval request = %+v, want approval-1/approved", runtime.resumeReq)
	}

	choiceBody, _ := json.Marshal(map[string][]any{"answers": []any{map[string]any{"value": "continue", "label": "Continue"}}})
	choiceResp, err := http.Post(ts.URL+"/api/v1/choices/choice-1/answer", "application/json", bytes.NewReader(choiceBody))
	if err != nil {
		t.Fatalf("POST /api/v1/choices/:id/answer error = %v", err)
	}
	defer choiceResp.Body.Close()
	if choiceResp.StatusCode != http.StatusOK {
		t.Fatalf("choice answer status = %d, want 200", choiceResp.StatusCode)
	}
	if runtime.resumeReq.SessionID != session.ID {
		t.Fatalf("ResumeTurn sessionId = %q, want %q", runtime.resumeReq.SessionID, session.ID)
	}
	if runtime.resumeReq.TurnID != "turn-live" {
		t.Fatalf("ResumeTurn turnId = %q, want turn-live", runtime.resumeReq.TurnID)
	}
	if got := runtime.resumeReq.Metadata["choice.answer.0"]; got == "" {
		t.Fatal("ResumeTurn metadata[choice.answer.0] is empty")
	}
	if got := runtime.resumeReq.Metadata["resume.input"]; got == "" {
		t.Fatal("ResumeTurn metadata[resume.input] is empty")
	}
}

func TestChatAPI_MessageAliasResumesPendingEvidenceTurn(t *testing.T) {
	now := time.Now().UTC()
	runtime := &interactionAPITestRuntime{}
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-evidence", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-evidence",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-followup-1",
			SessionID:  session.ID,
			TurnID:     "turn-evidence",
			Iteration:  1,
			ToolName:   "readonly_host_inspect",
			ToolCallID: "call-followup",
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
	}
	session.PendingEvidence = append([]runtimekernel.PendingEvidence(nil), session.CurrentTurn.PendingEvidence...)
	sessions.Update(session)

	srv := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"sessionId": "sess-evidence",
		"message":   "补充证据 follow-up",
		"metadata":  map[string]string{"source": "protocol-composer"},
	})
	resp, err := http.Post(ts.URL+"/api/v1/chat/message", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/chat/message error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/chat/message status = %d, want 200", resp.StatusCode)
	}
	if runtime.runReq.TurnID != "" {
		t.Fatalf("RunTurn request = %+v, want no new turn", runtime.runReq)
	}
	if runtime.resumeReq.SessionID != "sess-evidence" || runtime.resumeReq.TurnID != "turn-evidence" {
		t.Fatalf("ResumeTurn request = %+v, want sess-evidence/turn-evidence", runtime.resumeReq)
	}
	if runtime.resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		t.Fatalf("ResumeState = %q, want pending_evidence", runtime.resumeReq.ResumeState)
	}
	if got := runtime.resumeReq.Metadata["resume.input"]; got != "补充证据 follow-up" {
		t.Fatalf("metadata[resume.input] = %q, want message alias content", got)
	}
}

func TestChatAPI_MessageEchoesClientIDsAndPassesThemToRuntime(t *testing.T) {
	runtime := &interactionAPITestRuntime{runCh: make(chan runtimekernel.TurnRequest, 1)}
	sessions := runtimekernel.NewSessionManager()
	srv := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"sessionId":       "sess-client",
		"message":         "需要即时反馈",
		"clientMessageId": "client-msg-1",
		"clientTurnId":    "client-turn-1",
	})
	resp, err := http.Post(ts.URL+"/api/v1/chat/message", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/chat/message error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/chat/message status = %d, want 200", resp.StatusCode)
	}

	var payload ChatMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	runReq := waitForAPIRunTurn(t, runtime)
	if runReq.ClientMessageID != "client-msg-1" {
		t.Fatalf("RunTurn ClientMessageID = %q, want client-msg-1", runReq.ClientMessageID)
	}
	if runReq.ClientTurnID != "client-turn-1" {
		t.Fatalf("RunTurn ClientTurnID = %q, want client-turn-1", runReq.ClientTurnID)
	}
	if payload.ClientMessageID != "client-msg-1" {
		t.Fatalf("response clientMessageId = %q, want client-msg-1", payload.ClientMessageID)
	}
	if payload.ClientTurnID != "client-turn-1" {
		t.Fatalf("response clientTurnId = %q, want client-turn-1", payload.ClientTurnID)
	}
	if !payload.Accepted || payload.Status != "accepted" {
		t.Fatalf("accepted/status = %v/%q, want true/accepted", payload.Accepted, payload.Status)
	}
	if payload.Output != "" {
		t.Fatalf("response output = %q, want empty accepted-only response", payload.Output)
	}
}
