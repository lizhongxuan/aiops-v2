package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aiops-v2/internal/workfloweditor"
)

func TestDefaultRuntimeSettingsMatchesDesign(t *testing.T) {
	settings := DefaultRuntimeSettings()

	if settings.AgentRuntime.IntentFrameRouting != "trace_only" {
		t.Fatalf("IntentFrameRouting = %q, want trace_only", settings.AgentRuntime.IntentFrameRouting)
	}
	if !settings.AgentRuntime.DiagnosticProtocol {
		t.Fatal("DiagnosticProtocol = false, want true")
	}
	if settings.Tooling.ReadOnlyRetryEnabled {
		t.Fatal("ReadOnlyRetryEnabled = true, want false")
	}
	if settings.Tooling.ReadOnlyRetryMaxPerCall != 1 || settings.Tooling.ReadOnlyRetryMaxPerTurn != 3 {
		t.Fatalf("retry limits = %d/%d, want 1/3", settings.Tooling.ReadOnlyRetryMaxPerCall, settings.Tooling.ReadOnlyRetryMaxPerTurn)
	}
	if settings.Tooling.ReadOnlyRetryBackoffBaseMs != 300 || settings.Tooling.ReadOnlyRetryBackoffMaxMs != 2000 {
		t.Fatalf("retry backoff = %d/%d, want 300/2000", settings.Tooling.ReadOnlyRetryBackoffBaseMs, settings.Tooling.ReadOnlyRetryBackoffMaxMs)
	}
	if settings.Workflow.ReferenceGuardMode != "enforce" || settings.Workflow.ValidationProvider != "static" || settings.Workflow.ValidationImage != "python:3.12-slim" {
		t.Fatalf("workflow settings = %+v, want enforce/static/python:3.12-slim", settings.Workflow)
	}
	if settings.Workflow.Rollout.Stage != workfloweditor.WorkflowAgentStageDisabled || settings.Workflow.Rollout.SchemaVersion != workfloweditor.WorkflowRolloutSchemaVersion || settings.Workflow.Rollout.RolloutPercentage != 0 {
		t.Fatalf("workflow rollout = %+v, want explicit disabled default", settings.Workflow.Rollout)
	}
	if settings.OpsManual.AutoRetrieval {
		t.Fatal("OpsManual.AutoRetrieval = true, want false")
	}
	if !settings.Debug.ModelInputTrace || settings.Debug.FinalState || settings.Debug.TransportProjection || settings.Debug.TranscriptProjection {
		t.Fatalf("debug settings = %+v, want trace only", settings.Debug)
	}
	if !settings.PublicWeb.Enabled {
		t.Fatal("PublicWeb.Enabled = false, want true")
	}
}

func TestNormalizeRuntimeSettingsClampsInvalidValues(t *testing.T) {
	settings := NormalizeRuntimeSettings(RuntimeSettings{
		AgentRuntime: RuntimeAgentSettings{
			IntentFrameRouting: "invalid",
		},
		Tooling: RuntimeToolingSettings{
			ReadOnlyRetryEnabled:       true,
			ReadOnlyRetryMaxPerCall:    -1,
			ReadOnlyRetryMaxPerTurn:    99,
			ReadOnlyRetryBackoffBaseMs: -20,
			ReadOnlyRetryBackoffMaxMs:  10,
		},
		Workflow: RuntimeWorkflowSettings{
			ReferenceGuardMode: "broken",
			ValidationProvider: "shell",
			ValidationImage:    "  ",
		},
	})

	if settings.AgentRuntime.IntentFrameRouting != "trace_only" {
		t.Fatalf("IntentFrameRouting = %q, want trace_only", settings.AgentRuntime.IntentFrameRouting)
	}
	if settings.Tooling.ReadOnlyRetryMaxPerCall != 0 || settings.Tooling.ReadOnlyRetryMaxPerTurn != 10 {
		t.Fatalf("retry limits = %d/%d, want 0/10", settings.Tooling.ReadOnlyRetryMaxPerCall, settings.Tooling.ReadOnlyRetryMaxPerTurn)
	}
	if settings.Tooling.ReadOnlyRetryBackoffBaseMs != 300 || settings.Tooling.ReadOnlyRetryBackoffMaxMs != 300 {
		t.Fatalf("retry backoff = %d/%d, want 300/300", settings.Tooling.ReadOnlyRetryBackoffBaseMs, settings.Tooling.ReadOnlyRetryBackoffMaxMs)
	}
	if settings.Workflow.ReferenceGuardMode != "enforce" || settings.Workflow.ValidationProvider != "static" || settings.Workflow.ValidationImage != "python:3.12-slim" {
		t.Fatalf("workflow settings = %+v, want normalized defaults", settings.Workflow)
	}
	if settings.Workflow.Rollout.Stage != workfloweditor.WorkflowAgentStageDisabled || settings.Workflow.Rollout.RolloutPercentage != 0 || settings.Workflow.Rollout.SaltVersion == "" {
		t.Fatalf("workflow rollout = %+v, want fail-closed default", settings.Workflow.Rollout)
	}
}

func TestNormalizeRuntimeSettingsPreservesOnlyValidWorkflowRollout(t *testing.T) {
	settings := DefaultRuntimeSettings()
	settings.Workflow.Rollout = workfloweditor.WorkflowRolloutConfig{
		SchemaVersion:     workfloweditor.WorkflowRolloutSchemaVersion,
		Stage:             workfloweditor.WorkflowAgentStageInternalApply,
		SaltVersion:       "rollout-v7",
		RolloutPercentage: 25,
		AllowTenants:      []string{"runner-default"},
		DenyTenants:       []string{"tenant-denied"},
		Features:          workfloweditor.WorkflowRolloutFeatures{DomainEvents: true},
	}
	normalized := NormalizeRuntimeSettings(settings)
	if normalized.Workflow.Rollout.Stage != workfloweditor.WorkflowAgentStageInternalApply || normalized.Workflow.Rollout.RolloutPercentage != 25 || len(normalized.Workflow.Rollout.AllowTenants) != 1 {
		t.Fatalf("normalized rollout = %+v, want valid config preserved", normalized.Workflow.Rollout)
	}
	settings.Workflow.Rollout.AllowTenants[0] = "mutated"
	if normalized.Workflow.Rollout.AllowTenants[0] != "runner-default" {
		t.Fatalf("normalized rollout aliases caller slice: %+v", normalized.Workflow.Rollout.AllowTenants)
	}

	settings.Workflow.Rollout.RolloutPercentage = 101
	invalid := NormalizeRuntimeSettings(settings)
	if invalid.Workflow.Rollout.Stage != workfloweditor.WorkflowAgentStageDisabled || invalid.Workflow.Rollout.RolloutPercentage != 0 || invalid.Workflow.Rollout.Features.DomainEvents {
		t.Fatalf("invalid rollout = %+v, want fully disabled fallback", invalid.Workflow.Rollout)
	}
}

func TestRuntimeSettingsLegacyJSONWithoutRolloutFailsClosed(t *testing.T) {
	var settings RuntimeSettings
	if err := json.Unmarshal([]byte(`{"workflow":{"referenceGuardMode":"warning","validationProvider":"docker","validationImage":"python:3.12-slim"}}`), &settings); err != nil {
		t.Fatalf("unmarshal legacy runtime settings: %v", err)
	}
	normalized := NormalizeRuntimeSettings(settings)
	if normalized.Workflow.ReferenceGuardMode != RuntimeWorkflowReferenceGuardWarning || normalized.Workflow.Rollout.Stage != workfloweditor.WorkflowAgentStageDisabled || normalized.Workflow.Rollout.SchemaVersion != workfloweditor.WorkflowRolloutSchemaVersion {
		t.Fatalf("legacy normalized settings = %+v, want workflow fields preserved and rollout disabled", normalized.Workflow)
	}
}

func TestJSONFileStoreRuntimeSettingsDefaultsWhenMissing(t *testing.T) {
	store, err := NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer store.Close()

	settings, err := store.GetRuntimeSettings()
	if err != nil {
		t.Fatalf("GetRuntimeSettings() error = %v", err)
	}
	if settings.AgentRuntime.IntentFrameRouting != "trace_only" || !settings.Debug.ModelInputTrace {
		t.Fatalf("settings = %+v, want defaults", settings)
	}
}

func TestJSONFileStoreRuntimeSettingsPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := NewJSONFileStore(dir, time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	settings := DefaultRuntimeSettings()
	settings.AgentRuntime.IntentFrameRouting = "active"
	settings.Tooling.ReadOnlyRetryEnabled = true
	settings.Tooling.ReadOnlyRetryMaxPerCall = 4
	settings.Workflow.ValidationProvider = "docker"
	settings.Workflow.ValidationImage = "python:3.12-bookworm"
	settings.Workflow.Rollout.Stage = workfloweditor.WorkflowAgentStageReadOnly
	settings.Workflow.Rollout.RolloutPercentage = 100
	settings.Debug.ModelInputTrace = false
	if err := first.SaveRuntimeSettings(&settings); err != nil {
		t.Fatalf("SaveRuntimeSettings() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(dir, "runtime-settings.json")); err != nil {
		t.Fatalf("runtime-settings.json was not written: %v", err)
	}

	second, err := NewJSONFileStore(dir, time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() reopen error = %v", err)
	}
	defer second.Close()
	restored, err := second.GetRuntimeSettings()
	if err != nil {
		t.Fatalf("GetRuntimeSettings() after reopen error = %v", err)
	}
	if restored.AgentRuntime.IntentFrameRouting != "active" || !restored.Tooling.ReadOnlyRetryEnabled || restored.Tooling.ReadOnlyRetryMaxPerCall != 4 {
		t.Fatalf("restored = %+v, want saved runtime settings", restored)
	}
	if restored.Workflow.ValidationProvider != "docker" || restored.Workflow.ValidationImage != "python:3.12-bookworm" || restored.Workflow.Rollout.Stage != workfloweditor.WorkflowAgentStageReadOnly {
		t.Fatalf("restored workflow = %+v, want docker/bookworm", restored.Workflow)
	}
	if restored.Debug.ModelInputTrace {
		t.Fatalf("restored Debug.ModelInputTrace = true, want false")
	}
}
