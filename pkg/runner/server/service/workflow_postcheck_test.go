package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"runner/workflow/edit"
)

func TestWorkflowPostCheckStructuredConfigurationPersistsOnlyNames(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	facts := WorkflowPostCheckFacts{
		Key: receipt.Key, TransactionID: receipt.TransactionID, Basis: receipt.Validation.Basis,
		Outcome:       WorkflowPostCheckConfigurationRequired,
		Diagnostics:   []edit.WorkflowDiagnostic{{Code: "AUTH", Severity: "error", Message: "password=do-not-persist"}},
		Configuration: WorkflowConfigurationRequirements{SecretRefs: []string{"secret://workflow/ssh"}, Environments: []string{"production"}},
	}
	settled, err := fixture.app.RecordPostCheck(context.Background(), facts)
	if err != nil || settled.Status != "configuration_required" {
		t.Fatalf("RecordPostCheck() = %#v, %v", settled, err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if err != nil || len(snapshot.LifecycleArtifacts) != 1 {
		t.Fatalf("post-check snapshot = %#v, %v", snapshot, err)
	}
	var artifact workflowPostCheckArtifact
	if err := json.Unmarshal(snapshot.LifecycleArtifacts[0].Payload, &artifact); err != nil {
		t.Fatal(err)
	}
	encoded := string(snapshot.LifecycleArtifacts[0].Payload)
	if strings.Contains(encoded, "do-not-persist") || len(artifact.Configuration.SecretRefs) != 1 || artifact.Configuration.SecretRefs[0] != "secret://workflow/ssh" || len(artifact.Configuration.Environments) != 1 || artifact.Configuration.Environments[0] != "production" {
		t.Fatalf("unsafe configuration artifact: %s", encoded)
	}
}

func TestWorkflowPostCheckRejectsConfigurationValueDisguisedAsName(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{
		Key: receipt.Key, TransactionID: receipt.TransactionID, Basis: receipt.Validation.Basis,
		Outcome:       WorkflowPostCheckConfigurationRequired,
		Configuration: WorkflowConfigurationRequirements{SecretRefs: []string{"api-token=do-not-persist"}},
	})
	if err == nil {
		t.Fatal("configuration value was accepted as a secretRef name")
	}
	snapshot, snapErr := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if snapErr != nil || snapshot.Receipt.Status != "verifying" || len(snapshot.LifecycleArtifacts) != 0 {
		t.Fatalf("rejected configuration mutated lifecycle: %#v, %v", snapshot, snapErr)
	}
}
