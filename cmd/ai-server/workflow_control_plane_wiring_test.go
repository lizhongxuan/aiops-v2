package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/observability"
	"aiops-v2/internal/runnerembed"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
	"aiops-v2/internal/workfloweditor"
	runnerservice "runner/server/service"
	workflowedit "runner/workflow/edit"
)

func TestRegisterWorkflowControlPlaneUsesRunnerApplicationAndCoreSix(t *testing.T) {
	dataDir := t.TempDir()
	runtime, err := runnerembed.NewRuntime(context.Background(), runnerembed.Options{DataDir: dataDir})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	registry := mcp.NewRegistry()
	settings := workflowControlPlaneRuntimeSettings(workfloweditor.WorkflowAgentStageGeneral)
	settings.Workflow.Rollout.Features.PlanIR = true
	application, err := registerWorkflowControlPlane(registry, runtime, filepath.Join(dataDir, "secrets"), settings)
	if err != nil {
		t.Fatalf("registerWorkflowControlPlane() error = %v", err)
	}
	if application == nil || application != runtime.WorkflowEditApplication() {
		t.Fatal("production wiring did not preserve the Runner-owned WorkflowEditApplication")
	}

	tools := registry.ListServerTools("runner")
	names := make([]string, 0, len(tools))
	for _, workflowTool := range tools {
		names = append(names, workflowTool.Metadata().Name)
	}
	sort.Strings(names)
	want := []string{
		"workflow.apply_patch",
		"workflow.compile_plan",
		"workflow.get_action_catalog",
		"workflow.get_context",
		"workflow.preview_patch",
		"workflow.validate",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("runner tools = %v, want Core6 %v", names, want)
	}

	assembler := tooling.NewAssembler(tooling.NewRegistry(), registry)
	assembled := assembler.AssembleToolsWithOptions("workspace", "plan", tooling.AssembleOptions{
		TenantID:     runnerservice.VisualWorkflowDefaultTenantID,
		UserID:       "local-user",
		Profile:      runtimekernel.RuntimePromptProfileWorkflowAgent,
		EnabledPacks: []string{"workflow_editor"},
	})
	if len(assembled) != len(want) {
		assembledNames := make([]string, 0, len(assembled))
		for _, workflowTool := range assembled {
			assembledNames = append(assembledNames, workflowTool.Metadata().Name)
		}
		t.Fatalf("assembled workflow tools = %v, want %v", assembledNames, want)
	}
}

func TestRegisterWorkflowControlPlaneFailsClosedAndGatesMutation(t *testing.T) {
	dataDir := t.TempDir()
	runtime, err := runnerembed.NewRuntime(context.Background(), runnerembed.Options{DataDir: dataDir})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	for _, tc := range []struct {
		name  string
		stage workfloweditor.WorkflowAgentStage
		want  []string
	}{
		{name: "disabled", stage: workfloweditor.WorkflowAgentStageDisabled, want: []string{}},
		{name: "read only", stage: workfloweditor.WorkflowAgentStageReadOnly, want: []string{"workflow.get_action_catalog", "workflow.get_context", "workflow.preview_patch", "workflow.validate"}},
		{name: "internal apply", stage: workfloweditor.WorkflowAgentStageInternalApply, want: []string{"workflow.apply_patch", "workflow.get_action_catalog", "workflow.get_context", "workflow.preview_patch", "workflow.validate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := mcp.NewRegistry()
			secretDir := filepath.Join(dataDir, "secrets-"+strings.ReplaceAll(tc.name, " ", "-"))
			application, registerErr := registerWorkflowControlPlane(registry, runtime, secretDir, workflowControlPlaneRuntimeSettings(tc.stage))
			if registerErr != nil {
				t.Fatalf("registerWorkflowControlPlane() error = %v", registerErr)
			}
			if application != runtime.WorkflowEditApplication() {
				t.Fatal("registration did not return the Runner-owned application")
			}
			names := make([]string, 0)
			for _, tool := range registry.ListServerTools("runner") {
				names = append(names, tool.Metadata().Name)
			}
			sort.Strings(names)
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("registered tools = %v, want %v", names, tc.want)
			}
			if tc.stage == workfloweditor.WorkflowAgentStageDisabled {
				if _, statErr := os.Stat(filepath.Join(secretDir, workflowResourceSigningKeyFile)); !os.IsNotExist(statErr) {
					t.Fatalf("disabled rollout created signing material: %v", statErr)
				}
			}
		})
	}
}

func TestWorkflowCapabilitySnapshotFromRuntimeSettingsFailsClosed(t *testing.T) {
	missing := workflowCapabilitySnapshotFromRuntimeSettings(store.RuntimeSettings{})
	if missing.Stage != workfloweditor.WorkflowAgentStageDisabled || missing.Capabilities.Any() {
		t.Fatalf("missing rollout snapshot = %+v, want disabled", missing)
	}
	invalid := workflowControlPlaneRuntimeSettings(workfloweditor.WorkflowAgentStageGeneral)
	invalid.Workflow.Rollout.RolloutPercentage = 101
	snapshot := workflowCapabilitySnapshotFromRuntimeSettings(invalid)
	if snapshot.Stage != workfloweditor.WorkflowAgentStageDisabled || snapshot.Capabilities.Any() {
		t.Fatalf("invalid rollout snapshot = %+v, want disabled", snapshot)
	}
}

func workflowControlPlaneRuntimeSettings(stage workfloweditor.WorkflowAgentStage) store.RuntimeSettings {
	settings := store.DefaultRuntimeSettings()
	settings.Workflow.Rollout.Stage = stage
	settings.Workflow.Rollout.RolloutPercentage = 100
	return settings
}

func TestWorkflowResourceSigningKeyIsStableAndPrivate(t *testing.T) {
	secretDir := t.TempDir()
	first, err := loadOrCreateWorkflowResourceSigningKey(secretDir)
	if err != nil {
		t.Fatalf("first key load error = %v", err)
	}
	second, err := loadOrCreateWorkflowResourceSigningKey(secretDir)
	if err != nil {
		t.Fatalf("second key load error = %v", err)
	}
	if len(first) != 32 || !reflect.DeepEqual(first, second) {
		t.Fatalf("signing key length/stability = %d/%t, want 32/true", len(first), reflect.DeepEqual(first, second))
	}
	info, err := os.Stat(filepath.Join(secretDir, workflowResourceSigningKeyFile))
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("key permissions = %o, want owner-only", info.Mode().Perm())
	}
}

func TestWorkflowCommandScopeResolverRejectsForgedAuthCookie(t *testing.T) {
	manager := auth.NewManager(nil)
	resolver := newWorkflowCommandScopeResolver(manager, runnerservice.VisualWorkflowDefaultTenantID)
	request := httptest.NewRequest("POST", "/api/runner-studio/workflow-commands", nil)
	request.AddCookie(auth.SessionCookie("forged-cookie"))
	if _, err := resolver.ResolveWorkflowCommandScope(context.Background(), request, "workflow-a"); err == nil {
		t.Fatal("forged auth cookie unexpectedly produced trusted workflow scope")
	}

	request = httptest.NewRequest("POST", "/api/runner-studio/workflow-commands", nil)
	scope, err := resolver.ResolveWorkflowCommandScope(context.Background(), request, "workflow-a")
	if err != nil {
		t.Fatalf("local trusted scope error = %v", err)
	}
	if scope.TenantID != runnerservice.VisualWorkflowDefaultTenantID || scope.ActorID != "local-user" || scope.CanvasSessionID == "" || scope.SessionID == "" {
		t.Fatalf("local trusted scope = %#v", scope)
	}
}

func TestProductionMainHasNoLegacyWorkflowMemoryFallback(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, forbidden := range []string{"workfloweditor.NewService(nil)", "WithWorkflowEditorService("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("production main still contains legacy workflow fallback %q", forbidden)
		}
	}
	if !strings.Contains(string(source), "WithWorkflowCandidateApplyApproval(workflowApplication, kernel)") {
		t.Fatal("production main does not wire the durable Candidate store and RuntimeKernel into the controlled apply-approval endpoint")
	}
}

func TestWorkflowObservabilityWiringAttachesApplicationObserver(t *testing.T) {
	observability.ResetWorkflowMetricsForTest()
	dataDir := t.TempDir()
	runtime, err := runnerembed.NewRuntime(context.Background(), runnerembed.Options{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	application, err := registerWorkflowControlPlane(mcp.NewRegistry(), runtime, filepath.Join(dataDir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	wireWorkflowObservability(application, nil)
	_, _ = application.ValidateChange(context.Background(), workflowedit.ValidateRequest{
		SchemaVersion: "invalid", WorkflowKey: workflowedit.WorkflowKey{TenantID: runnerservice.VisualWorkflowDefaultTenantID, WorkflowID: "workflow-observer"},
	})
	snapshot := observability.CurrentWorkflowMetricsSnapshot()
	if snapshot.Operations[string(runnerservice.WorkflowEditObserveValidate)].Count != 1 {
		t.Fatalf("workflow metrics = %#v", snapshot)
	}
}
