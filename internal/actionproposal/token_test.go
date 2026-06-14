package actionproposal

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizedInputHashIsStableForJSONKeyOrderAndCommandAliases(t *testing.T) {
	a, err := NormalizedInputHash(json.RawMessage(`{"args":["-h"],"command":"df","workingDir":"/tmp","actionToken":"ignored"}`))
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	b, err := NormalizedInputHash(json.RawMessage(`{"workingDir":"/tmp","cmd":"df -h"}`))
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if a != b {
		t.Fatalf("hash mismatch for equivalent command input:\n%s\n%s", a, b)
	}
}

func TestNormalizedInputHashIgnoresAuditOnlyFields(t *testing.T) {
	a, err := NormalizedInputHash(json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"],"intent":"audit text","actionToken":"token-1"}`))
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	b, err := NormalizedInputHash(json.RawMessage(`{"cmd":"systemctl restart erp-report.service"}`))
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if a != b {
		t.Fatalf("hash mismatch when only audit-only fields differ:\n%s\n%s", a, b)
	}
}

func TestSignerRejectsTamperedExpiredAndCrossScopeTokens(t *testing.T) {
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	signer := NewSigner([]byte("test-secret"), func() time.Time { return now })
	inputHash, err := NormalizedInputHash(json.RawMessage(`{"command":"date","args":[]}`))
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	claims := ActionTokenClaims{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		TenantID:   "tenant-a",
		UserID:     "user-a",
		IncidentID: "inc-1",
		ToolName:   "exec_command",
		InputHash:  inputHash,
		Source:     SourceRunbook,
		Risk:       RiskLow,
		ExpiresAt:  now.Add(time.Minute),
	}
	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := signer.Verify(token, claims); err != nil {
		t.Fatalf("Verify(valid) error = %v", err)
	}
	if _, err := signer.Verify(token+"x", claims); err == nil {
		t.Fatal("Verify(tampered) error = nil")
	}
	wrongSession := claims
	wrongSession.SessionID = "sess-2"
	if _, err := signer.Verify(token, wrongSession); err == nil {
		t.Fatal("Verify(cross session) error = nil")
	}
	wrongTenant := claims
	wrongTenant.TenantID = "tenant-b"
	if _, err := signer.Verify(token, wrongTenant); err == nil {
		t.Fatal("Verify(cross tenant) error = nil")
	}
	wrongUser := claims
	wrongUser.UserID = "user-b"
	if _, err := signer.Verify(token, wrongUser); err == nil {
		t.Fatal("Verify(cross user) error = nil")
	}
	wrongTool := claims
	wrongTool.ToolName = "k8s.restart_workload"
	if _, err := signer.Verify(token, wrongTool); err == nil {
		t.Fatal("Verify(cross tool) error = nil")
	}
	wrongInput := claims
	wrongInput.InputHash = "sha256:bad"
	if _, err := signer.Verify(token, wrongInput); err == nil {
		t.Fatal("Verify(cross input) error = nil")
	}

	expiredSigner := NewSigner([]byte("test-secret"), func() time.Time { return now.Add(2 * time.Minute) })
	if _, err := expiredSigner.Verify(token, claims); err == nil {
		t.Fatal("Verify(expired) error = nil")
	}
}

func TestInMemoryStoreReturnsProposalByToken(t *testing.T) {
	store := NewInMemoryStore()
	proposal := ActionProposal{
		SessionID:   "sess-1",
		TurnID:      "turn-1",
		IncidentID:  "inc-1",
		Source:      SourceFallback,
		ToolName:    "exec_command",
		ToolInput:   json.RawMessage(`{"command":"date"}`),
		Risk:        RiskLow,
		ActionToken: "token-1",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	store.Put(proposal)
	got, ok := store.Get("token-1")
	if !ok {
		t.Fatal("Get(token-1) ok = false")
	}
	if got.SessionID != proposal.SessionID || got.ToolName != proposal.ToolName {
		t.Fatalf("proposal = %#v, want %#v", got, proposal)
	}
}
