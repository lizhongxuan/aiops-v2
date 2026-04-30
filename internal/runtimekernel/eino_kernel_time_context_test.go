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
	if len(ctx.ExtraSections) != 2 {
		t.Fatalf("ExtraSections length = %d, want 2", len(ctx.ExtraSections))
	}
	if ctx.ExtraSections[0].Title != "Current Time" {
		t.Fatalf("ExtraSections[0].Title = %q, want Current Time", ctx.ExtraSections[0].Title)
	}
	if !strings.Contains(ctx.ExtraSections[0].Content, "2026-04-23 21:05:00 CST +0800") {
		t.Fatalf("Current Time section = %q, want formatted server time", ctx.ExtraSections[0].Content)
	}
	if ctx.ExtraSections[1].Title != "Server-local Port Safety" {
		t.Fatalf("ExtraSections[1].Title = %q, want Server-local Port Safety", ctx.ExtraSections[1].Title)
	}
	for _, want := range []string{"127.0.0.1:8080", "do not bind", "check host ports"} {
		if !strings.Contains(ctx.ExtraSections[1].Content, want) {
			t.Fatalf("Server-local Port Safety section = %q, want %q", ctx.ExtraSections[1].Content, want)
		}
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
