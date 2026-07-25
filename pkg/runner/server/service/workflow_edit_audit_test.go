package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestBuildWorkflowEditAuditCorrelatesAndRecursivelyRedacts(t *testing.T) {
	secrets := []string{"password-sentinel", "token-sentinel", "credential-sentinel", "prompt-sentinel", "bearer-sentinel"}
	details := map[string]any{
		"nodeCount": 3,
		"password":  secrets[0],
		"nested": []any{
			map[string]any{"api_token": secrets[1]},
			map[string]any{"credential": secrets[2]},
		},
		"rawInput":          map[string]any{"command": secrets[3]},
		"authorizationText": "Bearer " + secrets[4],
		"secretRef":         "vault://workflow/reference",
	}
	input := WorkflowEditAuditInput{
		Key: edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}, Actor: "mcp", Channel: "mcp", Operation: "apply",
		UserID: "user-1", SessionID: "session-1", TurnID: "turn-1", ToolCallID: "tool-call-1", ChangeSetID: "change-1", TransactionID: "transaction-1",
		BeforeRevision: edit.Revision{ResourceVersion: "rv-before", SemanticHash: "semantic-before", LayoutHash: "layout-before"},
		AfterRevision:  edit.Revision{ResourceVersion: "rv-after", SemanticHash: "semantic-after", LayoutHash: "layout-after"},
		Details:        details, OccurredAt: time.Date(2026, 7, 21, 9, 0, 0, 123, time.FixedZone("test", 8*60*60)),
	}
	record, err := BuildWorkflowEditAudit(input)
	if err != nil {
		t.Fatal(err)
	}
	details["password"] = "mutated-after-build"
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range append(secrets, "mutated-after-build") {
		if strings.Contains(text, secret) {
			t.Fatalf("audit leaked %q: %s", secret, text)
		}
	}
	for _, expected := range []string{"tenant-a", "user-1", "session-1", "turn-1", "tool-call-1", "change-1", "transaction-1", "rv-before", "rv-after", workflowAuditRedacted, workflowAuditOmitted, "vault://workflow/reference"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("audit missing %q: %s", expected, text)
		}
	}
	if record.OccurredAt.Location() != time.UTC || record.AuditID == "" {
		t.Fatalf("audit identity/time = %q %v", record.AuditID, record.OccurredAt)
	}
	envelope, err := BuildWorkflowEditAuditEnvelope(input)
	if err != nil || envelope.Kind != workflowEditAuditKind || envelope.SchemaVersion != edit.SchemaVersion || !json.Valid(envelope.Payload) {
		t.Fatalf("audit envelope = %#v err=%v", envelope, err)
	}
}

func TestBuildWorkflowEditAuditRejectsIncompleteCorrelation(t *testing.T) {
	_, err := BuildWorkflowEditAudit(WorkflowEditAuditInput{
		Key: edit.WorkflowKey{TenantID: "tenant", WorkflowID: "workflow"}, Actor: "manual", Channel: "manual", Operation: "save",
		UserID: "user", SessionID: "session", ChangeSetID: "change", TransactionID: "transaction",
		BeforeRevision: edit.Revision{ResourceVersion: "rv-before"}, OccurredAt: time.Now(),
	})
	if err == nil {
		t.Fatal("BuildWorkflowEditAudit accepted a missing after revision")
	}
}
