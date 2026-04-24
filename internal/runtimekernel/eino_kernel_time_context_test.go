package runtimekernel

import (
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/promptcompiler"
)

func TestEnrichCompileContextInjectsServerLocalTime(t *testing.T) {
	now := time.Date(2026, 4, 23, 21, 5, 0, 0, time.FixedZone("CST", 8*60*60))
	ctx := enrichCompileContext(promptcompiler.CompileContext{
		SessionType: string(SessionTypeHost),
		Mode:        string(ModeChat),
	}, SessionTypeHost, "server-local", now)

	if ctx.HostContext != "server-local" {
		t.Fatalf("HostContext = %q, want server-local", ctx.HostContext)
	}
	if len(ctx.ExtraSections) != 1 {
		t.Fatalf("ExtraSections length = %d, want 1", len(ctx.ExtraSections))
	}
	if ctx.ExtraSections[0].Title != "Current Time" {
		t.Fatalf("ExtraSections[0].Title = %q, want Current Time", ctx.ExtraSections[0].Title)
	}
	if !strings.Contains(ctx.ExtraSections[0].Content, "2026-04-23 21:05:00 CST +0800") {
		t.Fatalf("Current Time section = %q, want formatted server time", ctx.ExtraSections[0].Content)
	}
}

func TestEnrichCompileContextSkipsRemoteHostTimeInjection(t *testing.T) {
	now := time.Date(2026, 4, 23, 21, 5, 0, 0, time.UTC)
	ctx := enrichCompileContext(promptcompiler.CompileContext{
		SessionType: string(SessionTypeHost),
		Mode:        string(ModeChat),
	}, SessionTypeHost, "web-01", now)

	if ctx.HostContext != "web-01" {
		t.Fatalf("HostContext = %q, want web-01", ctx.HostContext)
	}
	if len(ctx.ExtraSections) != 0 {
		t.Fatalf("ExtraSections length = %d, want 0 for remote host", len(ctx.ExtraSections))
	}
}
