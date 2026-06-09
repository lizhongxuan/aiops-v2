package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"

	"github.com/cloudwego/eino/schema"
)

func TestERPSREExecCommandGovernanceThroughDispatcher(t *testing.T) {
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	secret := []byte("runtime-erp-sre-secret")
	tool := newGovernedExecCommandTestTool(t, secret, now)
	dispatcher := NewToolDispatcher(
		assembledToolLookup{byName: map[string]tooling.Tool{"exec_command": tool}},
		nil,
		&testMockEventEmitter{},
	)
	mutationInput := json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"],"intent":"restart after runbook diagnosis"}`)

	missingToken := dispatcher.Dispatch(context.Background(), "sess-1", "turn-1", ToolCall{
		ID:        "call-exec-no-token",
		Name:      "exec_command",
		Arguments: mutationInput,
	}, SessionTypeHost, ModeExecute)
	if !missingToken.Blocked || missingToken.Outcome != "evidence_needed" || missingToken.Source != "tool" {
		t.Fatalf("missing token dispatch = %#v, want tool evidence_needed", missingToken)
	}

	inputHash, err := actionproposal.NormalizedInputHash(mutationInput)
	if err != nil {
		t.Fatalf("NormalizedInputHash() error = %v", err)
	}
	token, err := actionproposal.NewSigner(secret, func() time.Time { return now }).Sign(actionproposal.ActionTokenClaims{
		SessionID:      "sess-1",
		TurnID:         "turn-1",
		IncidentID:     "inc-erp-1",
		ToolName:       "exec_command",
		InputHash:      inputHash,
		Source:         actionproposal.SourceRunbook,
		Risk:           actionproposal.RiskHigh,
		Reason:         "runbook guarded restart",
		RunbookID:      "order-submit-slow",
		RunbookStepID:  "restart-report-service",
		ExpectedEffect: "release db connections",
		Rollback:       "stop and escalate",
		ExpiresAt:      now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	withToken := json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"],"intent":"restart after runbook diagnosis","actionToken":` + strconvQuote(token) + `}`)
	needsApproval := dispatcher.Dispatch(context.Background(), "sess-1", "turn-1", ToolCall{
		ID:        "call-exec-token",
		Name:      "exec_command",
		Arguments: withToken,
	}, SessionTypeHost, ModeExecute)
	if !needsApproval.Blocked || needsApproval.Outcome != "approval_needed" || needsApproval.Approval == nil {
		t.Fatalf("token dispatch = %#v, want approval_needed", needsApproval)
	}
	if needsApproval.Metadata.Name != "exec_command" || needsApproval.Approval.Command == "" || needsApproval.Approval.RunbookID != "order-submit-slow" {
		t.Fatalf("approval payload = %#v, want real command and runbook metadata", needsApproval.Approval)
	}
}

func TestERPSREApprovalApprovedResumeContinuesAndDeniedCleansPending(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-approval",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"/tmp/erp-sre","content":"approved"}`,
				},
			}}),
			schema.AssistantMessage("approved path continued", nil),
		},
	}
	executed := 0
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "write_file", Description: "Write file"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: syntheticPassVerificationReportContent(t, "vr-synthetic-erp-approval", "synthetic approved write")}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{SessionID: "sess-erp-approval", SessionType: SessionTypeHost, Mode: ModeExecute, TurnID: "turn-erp-approval", Input: "make governed change"})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if blocked.Status != "blocked" || executed != 0 {
		t.Fatalf("blocked=%#v executed=%d, want blocked before execution", blocked, executed)
	}
	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{SessionID: "sess-erp-approval", TurnID: "turn-erp-approval", Decision: "approved"})
	if err != nil {
		t.Fatalf("ResumeTurn approved error = %v", err)
	}
	if resumed.Status != "completed" || resumed.Output != "approved path continued" || executed != 1 {
		t.Fatalf("approved resume result=%#v executed=%d", resumed, executed)
	}

	deniedModel := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-denied",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"/tmp/erp-sre-denied","content":"denied"}`,
				},
			}}),
		},
	}
	deniedExecuted := 0
	deniedTool := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "write_file", Description: "Write file"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			deniedExecuted++
			return tooling.ToolResult{Content: "should not run"}, nil
		},
	}
	deniedKernel := newLoopKernel(t, deniedModel, []tooling.Tool{deniedTool}, nil, policyengine.NewDefaultModePolicies())
	deniedBlocked, err := deniedKernel.RunTurn(context.Background(), TurnRequest{SessionID: "sess-erp-denied", SessionType: SessionTypeHost, Mode: ModeExecute, TurnID: "turn-erp-denied", Input: "make denied change"})
	if err != nil {
		t.Fatalf("RunTurn denied setup error = %v", err)
	}
	if deniedBlocked.Status != "blocked" || deniedExecuted != 0 {
		t.Fatalf("denied setup result=%#v executed=%d, want blocked before execution", deniedBlocked, deniedExecuted)
	}
	deniedSession := deniedKernel.sessions.Get("sess-erp-denied")
	if deniedSession == nil || len(deniedSession.PendingApprovals) != 1 {
		t.Fatalf("pending approvals before deny = %#v, want one", deniedSession)
	}
	approvalID := deniedSession.PendingApprovals[0].ID
	denied, err := deniedKernel.ResumeTurn(context.Background(), ResumeRequest{SessionID: deniedSession.ID, TurnID: "turn-erp-denied", ApprovalID: approvalID, Decision: "denied"})
	if err != nil {
		t.Fatalf("ResumeTurn denied error = %v", err)
	}
	if denied.Status != "blocked" {
		t.Fatalf("denied result = %#v, want blocked", denied)
	}
	deniedSession = deniedKernel.sessions.Get(deniedSession.ID)
	if got := len(deniedSession.PendingApprovals); got != 0 {
		t.Fatalf("pending approvals after deny = %d, want 0", got)
	}
	if deniedExecuted != 0 {
		t.Fatalf("denied tool executions = %d, want 0", deniedExecuted)
	}
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

type governedExecCommandInput struct {
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	ActionToken string   `json:"actionToken"`
}

func newGovernedExecCommandTestTool(t *testing.T, secret []byte, now time.Time) tooling.Tool {
	t.Helper()
	signer := actionproposal.NewSigner(secret, func() time.Time { return now })
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "exec_command", Description: "Governed exec command"},
		InputSchemaData: json.RawMessage(`{
			"type":"object",
			"properties":{
				"command":{"type":"string"},
				"args":{"type":"array","items":{"type":"string"}},
				"intent":{"type":"string"}
			}
		}`),
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			var req governedExecCommandInput
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: err.Error()}
			}
			if req.Command == "" {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "command is required"}
			}
			execCtx, _ := tooling.ToolExecutionContextFrom(ctx)
			token := strings.TrimSpace(firstNonEmpty(req.ActionToken, execCtx.ActionToken))
			if token == "" {
				return tooling.PermissionDecision{
					Action: tooling.PermissionActionNeedEvidence,
					Reason: "non-read-only terminal command requires a signed ActionToken",
				}
			}
			hashInput := input
			if len(execCtx.SanitizedInput) > 0 {
				hashInput = execCtx.SanitizedInput
			}
			inputHash, err := actionproposal.NormalizedInputHash(hashInput)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: err.Error()}
			}
			claims, err := signer.Verify(token, actionproposal.ActionTokenClaims{
				SessionID: execCtx.SessionID,
				TurnID:    execCtx.TurnID,
				ToolName:  "exec_command",
				InputHash: inputHash,
			})
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: err.Error()}
			}
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "runbook action requires approval",
				Approval: &tooling.PermissionApprovalPayload{
					Command:        strings.TrimSpace(req.Command + " " + strings.Join(req.Args, " ")),
					Reason:         claims.Reason,
					Risk:           string(claims.Risk),
					Source:         string(claims.Source),
					RunbookID:      claims.RunbookID,
					RunbookStep:    claims.RunbookStepID,
					ExpectedEffect: claims.ExpectedEffect,
					Rollback:       claims.Rollback,
				},
			}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "executed"}, nil
		},
	}
}
