package policyengine

import (
	"strings"
	"testing"
	"time"
)

func TestWhitelistManagerLifecycleAndCommandMatching(t *testing.T) {
	now := time.Now().UTC()
	manager := NewWhitelistManager()
	if err := manager.Create(WhitelistEntry{
		ID:        "entry-1",
		HostID:    "host-1",
		ToolName:  "service_restart",
		Command:   "systemctl restart api",
		TTL:       time.Hour,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	entry, ok := manager.Get("entry-1")
	if !ok || entry.ExpiresAt == nil || !entry.Enabled || entry.Revoked {
		t.Fatalf("Get() = %#v, %v; want enabled entry with expiry", entry, ok)
	}
	if !manager.IsAuthorized("host-1", "service_restart", "systemctl restart api", now.Add(time.Minute)) {
		t.Fatal("expected exact command to be authorized")
	}
	if manager.IsAuthorized("host-1", "service_restart", "systemctl restart db", now.Add(time.Minute)) {
		t.Fatal("different command should not be authorized")
	}
	if manager.IsAuthorized("host-1", "service_restart", "systemctl restart api", now.Add(2*time.Hour)) {
		t.Fatal("expired entry should not be authorized")
	}

	if !manager.Disable("entry-1") || manager.IsAuthorized("host-1", "service_restart", "systemctl restart api", now.Add(time.Minute)) {
		t.Fatal("disabled entry should not authorize")
	}
	if !manager.Enable("entry-1") || !manager.IsAuthorized("host-1", "service_restart", "systemctl restart api", now.Add(time.Minute)) {
		t.Fatal("enabled entry should authorize again")
	}
	if !manager.Revoke("entry-1") || manager.IsAuthorized("host-1", "service_restart", "systemctl restart api", now.Add(time.Minute)) {
		t.Fatal("revoked entry should not authorize")
	}
	if manager.Disable("missing") || manager.Enable("missing") || manager.Revoke("missing") {
		t.Fatal("missing entry lifecycle calls should return false")
	}
}

func TestWhitelistManagerRejectsHighRiskCommandWithoutTTL(t *testing.T) {
	err := NewWhitelistManager().Create(WhitelistEntry{
		ID:        "danger",
		HostID:    "host-1",
		ToolName:  "command_exec",
		Command:   "rm -rf /",
		CreatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected high-risk command without TTL to fail")
	}
	if _, ok := err.(*HighRiskNoTTLError); !ok || !strings.Contains(err.Error(), "rm -rf /") {
		t.Fatalf("error = %T %v, want HighRiskNoTTLError with command", err, err)
	}
}

func TestGatewayPolicyApprovalPaths(t *testing.T) {
	now := time.Now().UTC()
	manager := NewWhitelistManager()
	if err := manager.Create(WhitelistEntry{
		ID:        "allow-restart",
		HostID:    "host-1",
		ToolName:  "service_restart",
		Command:   "systemctl restart api",
		TTL:       time.Hour,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gateway := GatewayPolicy{Whitelist: manager}

	if decision := gateway.CheckApproval("file_read", "", "host-1", now); decision.Action != PolicyActionAllow {
		t.Fatalf("structured read decision = %#v, want allow", decision)
	}
	if decision := gateway.CheckApproval("service_restart", "systemctl restart api", "host-1", now); decision.Action != PolicyActionAllow {
		t.Fatalf("whitelisted restart decision = %#v, want allow", decision)
	}
	if decision := gateway.CheckApproval("service_restart", "systemctl restart db", "host-1", now); decision.Action != PolicyActionNeedApproval || decision.Approval == nil {
		t.Fatalf("non-whitelisted restart decision = %#v, want approval", decision)
	}
	if decision := gateway.CheckApproval("command_exec", "rm -rf /tmp/demo", "host-1", now); decision.Action != PolicyActionNeedApproval || !strings.Contains(decision.Reason, "high-risk") {
		t.Fatalf("high-risk shell decision = %#v, want high-risk approval", decision)
	}
}

func TestTerminalCommandHelpersAndPolicyActionValidity(t *testing.T) {
	if !isReadOnlyTerminalCommand([]byte(`{"command":"ls -la"}`)) {
		t.Fatal("expected ls to be read-only terminal command")
	}
	cmd, ok := terminalCommandFromArgs([]byte(`{"cmd":"date +%F"}`))
	if !ok || cmd != "date" {
		t.Fatalf("terminalCommandFromArgs() = %q, %v; want date, true", cmd, ok)
	}
	if _, ok := terminalCommandFromArgs([]byte(`{"command":"date && rm -rf /tmp/nope"}`)); ok {
		t.Fatal("shell operators should reject terminal command parsing")
	}
	if !PolicyActionAllow.IsValid() || PolicyAction("bogus").IsValid() {
		t.Fatal("PolicyAction.IsValid returned unexpected result")
	}
}
