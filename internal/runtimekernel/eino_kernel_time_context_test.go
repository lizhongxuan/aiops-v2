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
	}, SessionTypeHost, "server-local", nil, now)

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
	}, SessionTypeHost, "web-01", nil, now)

	if ctx.HostContext != "web-01" {
		t.Fatalf("HostContext = %q, want web-01", ctx.HostContext)
	}
	if len(ctx.ExtraSections) != 0 {
		t.Fatalf("ExtraSections length = %d, want 0 for remote host", len(ctx.ExtraSections))
	}
}

func TestEnrichCompileContextInjectsOpsManualOptOutMetadata(t *testing.T) {
	ctx := enrichCompileContext(promptcompiler.CompileContext{
		SessionType: string(SessionTypeHost),
		Mode:        string(ModeChat),
	}, SessionTypeHost, "server-local", map[string]string{
		"opsManualAction":      "skip_ops_manual",
		"opsManualSkipped":     "true",
		"opsManualManualId":    "manual-pg-backup-ubuntu",
		"opsManualWorkflowId":  "workflow-pg-backup",
		"opsManualManualTitle": "PostgreSQL 备份 Ubuntu 运维手册",
	}, time.Date(2026, 4, 23, 21, 5, 0, 0, time.UTC))

	var section promptcompiler.PromptSection
	for _, candidate := range ctx.ExtraSections {
		if candidate.Title == "Ops Manual Opt-Out" {
			section = candidate
			break
		}
	}
	if section.Title == "" {
		t.Fatalf("ExtraSections = %#v, want Ops Manual Opt-Out section", ctx.ExtraSections)
	}
	for _, want := range []string{
		"User explicitly skipped operations manuals",
		"Do not call search_ops_manuals, resolve_ops_manual_params, or run_ops_manual_preflight",
		"ordinary safe read-only investigation",
		"manual-pg-backup-ubuntu",
		"workflow-pg-backup",
	} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("Ops Manual Opt-Out section = %q, want %q", section.Content, want)
		}
	}
}

func TestEnrichCompileContextInjectsSessionBindingMetadata(t *testing.T) {
	ctx := enrichCompileContext(promptcompiler.CompileContext{
		SessionType: string(SessionTypeHost),
		Mode:        string(ModeChat),
	}, SessionTypeHost, "redis-01", map[string]string{
		"aiops.target.kind":    "host",
		"aiops.target.hostId":  "redis-01",
		"aiops.environment":    "prod",
		"aiops.coroot.project": "prod-main",
	}, time.Date(2026, 4, 23, 21, 5, 0, 0, time.UTC))

	if len(ctx.ExtraSections) != 1 {
		t.Fatalf("ExtraSections length = %d, want session binding only", len(ctx.ExtraSections))
	}
	section := ctx.ExtraSections[0]
	if section.Title != "Session Binding" {
		t.Fatalf("section title = %q, want Session Binding", section.Title)
	}
	for _, want := range []string{"Environment: prod", "Coroot project: prod-main", "pass the bound project"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("Session Binding section = %q, want %q", section.Content, want)
		}
	}
}
