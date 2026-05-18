package service

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowServicePersistsSaveNoteMetadata(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	raw := testWorkflowYAML("note-demo", "echo initial")

	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:     "note-demo",
		RawYAML:  []byte(raw),
		SaveNote: "initial import",
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	record, err := svc.Get(context.Background(), "note-demo")
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got := record.SaveNote; got != "initial import" {
		t.Fatalf("save note after create = %q, want %q", got, "initial import")
	}

	if err := svc.Update(context.Background(), "note-demo", &WorkflowRecord{
		Name:     "note-demo",
		RawYAML:  []byte(testWorkflowYAML("note-demo", "echo changed")),
		SaveNote: "raised timeout after dry run",
	}); err != nil {
		t.Fatalf("update workflow with save note: %v", err)
	}
	record, err = svc.Get(context.Background(), "note-demo")
	if err != nil {
		t.Fatalf("get workflow after note update: %v", err)
	}
	if got := record.SaveNote; got != "raised timeout after dry run" {
		t.Fatalf("save note after update = %q, want %q", got, "raised timeout after dry run")
	}

	if err := svc.Update(context.Background(), "note-demo", &WorkflowRecord{
		Name:    "note-demo",
		RawYAML: []byte(testWorkflowYAML("note-demo", "echo changed again")),
	}); err != nil {
		t.Fatalf("update workflow without save note: %v", err)
	}
	record, err = svc.Get(context.Background(), "note-demo")
	if err != nil {
		t.Fatalf("get workflow after metadata-preserving update: %v", err)
	}
	if got := record.SaveNote; got != "raised timeout after dry run" {
		t.Fatalf("save note should be preserved when omitted, got %q", got)
	}
}

func TestVisualWorkflowPublishStateMachineRequiresValidatedGraphHash(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "publish-demo",
		RawYAML: []byte(testWorkflowYAML("publish-demo", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	record, err := svc.Get(context.Background(), "publish-demo")
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got := record.Status; got != WorkflowStatusDraft {
		t.Fatalf("status after create = %q, want %q", got, WorkflowStatusDraft)
	}

	if _, err := svc.Publish(context.Background(), "publish-demo", WorkflowPublishOptions{SaveNote: "too early"}); !isInvalidPublishValidationError(err) {
		t.Fatalf("publish before validation error = %v", err)
	}
	validated, err := svc.ValidateWorkflow(context.Background(), "publish-demo", WorkflowValidateOptions{Actor: "sre"})
	if err != nil {
		t.Fatalf("validate workflow: %v", err)
	}
	if got := validated.Status; got != WorkflowStatusValidated {
		t.Fatalf("status after validate = %q, want %q", got, WorkflowStatusValidated)
	}
	if validated.ValidatedGraphHash == "" || validated.ValidatedAt.IsZero() || validated.ValidatedBy != "sre" {
		t.Fatalf("validated metadata missing: %+v", validated)
	}

	if _, err := svc.Publish(context.Background(), "publish-demo", WorkflowPublishOptions{SaveNote: "too early"}); !isInvalidDryRunValidationError(err) {
		t.Fatalf("publish before dry run error = %v", err)
	}
	dryRunPassed, err := svc.MarkDryRunPassed(context.Background(), "publish-demo", WorkflowDryRunOptions{
		Actor:             "sre",
		ExpectedGraphHash: validated.ValidatedGraphHash,
	})
	if err != nil {
		t.Fatalf("mark dry run passed: %v", err)
	}
	if dryRunPassed.Status != WorkflowStatusDryRunPassed || dryRunPassed.DryRunGraphHash != validated.ValidatedGraphHash || dryRunPassed.DryRunAt.IsZero() {
		t.Fatalf("dry run metadata missing: %+v", dryRunPassed)
	}

	published, err := svc.Publish(context.Background(), "publish-demo", WorkflowPublishOptions{SaveNote: "ready for prod", RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("publish workflow: %v", err)
	}
	if got := published.Status; got != WorkflowStatusPublished {
		t.Fatalf("status after publish = %q, want %q", got, WorkflowStatusPublished)
	}
	if published.PublishedAt.IsZero() {
		t.Fatal("published_at should be set after publish")
	}
	if got := published.SaveNote; got != "ready for prod" {
		t.Fatalf("publish save note = %q", got)
	}
	if published.PublishedGraphHash == "" || published.PublishedGraphHash != published.ValidatedGraphHash {
		t.Fatalf("published graph hash mismatch: %+v", published)
	}

	if err := svc.Update(context.Background(), "publish-demo", &WorkflowRecord{
		Name:    "publish-demo",
		RawYAML: []byte(testWorkflowYAML("publish-demo", "echo changed")),
	}); err != nil {
		t.Fatalf("update workflow after publish: %v", err)
	}
	record, err = svc.Get(context.Background(), "publish-demo")
	if err != nil {
		t.Fatalf("get workflow after update: %v", err)
	}
	if got := record.Status; got != WorkflowStatusDraft {
		t.Fatalf("status after update = %q, want %q", got, WorkflowStatusDraft)
	}
	if record.ValidatedGraphHash != "" || !record.ValidatedAt.IsZero() {
		t.Fatalf("semantic update should clear validation metadata: %+v", record)
	}
}

func TestWorkflowServicePublishRequiresRiskAcknowledgement(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "publish-risk",
		RawYAML: []byte(testWorkflowYAMLWithAction("publish-risk", "script.shell", "script", "echo risky")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	validateForPublish(t, svc, "publish-risk")

	if _, err := svc.Publish(context.Background(), "publish-risk", WorkflowPublishOptions{}); !isInvalidRiskAcknowledgementError(err) {
		t.Fatalf("publish high-risk workflow without acknowledgement error = %v", err)
	}

	if _, err := svc.Publish(context.Background(), "publish-risk", WorkflowPublishOptions{RiskAcknowledged: true}); err != nil {
		t.Fatalf("publish high-risk workflow with acknowledgement: %v", err)
	}
}

func TestWorkflowServicePublishEnforcesDryRunPrechecks(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "publish-capability",
		RawYAML: []byte(testWorkflowYAMLWithCapabilityMismatch("publish-capability")),
	}); err != nil {
		t.Fatalf("create capability workflow: %v", err)
	}
	validateForPublish(t, svc, "publish-capability")
	if _, err := svc.Publish(context.Background(), "publish-capability", WorkflowPublishOptions{RiskAcknowledged: true, WarningAcknowledged: true}); !isInvalidCapabilityError(err) {
		t.Fatalf("publish with capability mismatch error = %v", err)
	}

	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "publish-warning",
		RawYAML: []byte(testWorkflowYAML("publish-warning", "echo ${missing_token}")),
	}); err != nil {
		t.Fatalf("create warning workflow: %v", err)
	}
	validateForPublish(t, svc, "publish-warning")
	if _, err := svc.Publish(context.Background(), "publish-warning", WorkflowPublishOptions{RiskAcknowledged: true}); !isInvalidWarningAcknowledgementError(err) {
		t.Fatalf("publish with dry-run warning error = %v", err)
	}
	if _, err := svc.Publish(context.Background(), "publish-warning", WorkflowPublishOptions{RiskAcknowledged: true, WarningAcknowledged: true}); err != nil {
		t.Fatalf("publish with acknowledged warning: %v", err)
	}
}

func TestVisualWorkflowGraphHashLayoutChangePreservesValidatedState(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	visualSvc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	graph := sampleVisualGraph()
	graph.Workflow.Name = "layout-preserve"
	created, err := visualSvc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{})
	if err != nil {
		t.Fatalf("create visual graph: %v", err)
	}
	validated, err := workflowSvc.ValidateWorkflow(context.Background(), created.Name, WorkflowValidateOptions{Actor: "designer"})
	if err != nil {
		t.Fatalf("validate visual workflow: %v", err)
	}
	if validated.Status != WorkflowStatusValidated || validated.ValidatedGraphHash == "" {
		t.Fatalf("validated metadata mismatch: %+v", validated)
	}

	loaded, err := visualSvc.GetGraph(context.Background(), created.Name)
	if err != nil {
		t.Fatalf("get visual graph: %v", err)
	}
	loaded.Nodes[1].Position.X += 120
	loaded.Layout.Viewport.Zoom = 0.8
	if _, err := visualSvc.SaveGraph(context.Background(), created.Name, loaded); err != nil {
		t.Fatalf("save layout-only graph: %v", err)
	}
	record, err := workflowSvc.Get(context.Background(), created.Name)
	if err != nil {
		t.Fatalf("get layout-only saved workflow: %v", err)
	}
	if record.Status != WorkflowStatusValidated {
		t.Fatalf("layout-only save should preserve validated status, got %+v", record)
	}
	if record.ValidatedGraphHash != validated.ValidatedGraphHash {
		t.Fatalf("layout-only save should keep semantic validated hash: before=%s after=%s", validated.ValidatedGraphHash, record.ValidatedGraphHash)
	}
}

func TestVisualWorkflowPublishAIGeneratedDraftCannotBypassValidation(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "ai-draft",
		RawYAML: []byte(testWorkflowYAML("ai-draft", "echo generated")),
		Labels:  map[string]string{"source": "ai"},
	}); err != nil {
		t.Fatalf("create ai draft: %v", err)
	}

	if _, err := svc.Publish(context.Background(), "ai-draft", WorkflowPublishOptions{}); !isInvalidPublishValidationError(err) {
		t.Fatalf("ai generated draft should require validation before publish, got %v", err)
	}
}

func TestWorkflowServiceHistoryAndRollback(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "history-demo",
		RawYAML: []byte(testWorkflowYAML("history-demo", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := svc.Update(context.Background(), "history-demo", &WorkflowRecord{
		Name:    "history-demo",
		RawYAML: []byte(testWorkflowYAML("history-demo", "echo updated")),
	}); err != nil {
		t.Fatalf("update workflow: %v", err)
	}

	versions, err := svc.ListVersions(context.Background(), "history-demo")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("version count = %d, want 2: %+v", len(versions), versions)
	}
	var initialVersionID string
	for _, version := range versions {
		if strings.Contains(string(version.RawYAML), "echo initial") {
			initialVersionID = version.ID
			break
		}
	}
	if initialVersionID == "" {
		t.Fatalf("initial version not found: %+v", versions)
	}

	rolledBack, err := svc.Rollback(context.Background(), "history-demo", initialVersionID, WorkflowRollbackOptions{SaveNote: "rollback to initial"})
	if err != nil {
		t.Fatalf("rollback workflow: %v", err)
	}
	if !strings.Contains(string(rolledBack.RawYAML), "echo initial") {
		t.Fatalf("rollback yaml mismatch:\n%s", string(rolledBack.RawYAML))
	}
	if rolledBack.Status != WorkflowStatusDraft {
		t.Fatalf("rollback status = %q, want draft", rolledBack.Status)
	}
	if rolledBack.SaveNote != "rollback to initial" {
		t.Fatalf("rollback save note = %q", rolledBack.SaveNote)
	}

	versions, err = svc.ListVersions(context.Background(), "history-demo")
	if err != nil {
		t.Fatalf("list versions after rollback: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("version count after rollback = %d, want 3", len(versions))
	}
}

func TestWorkflowServiceExportAndImportBundle(t *testing.T) {
	sourceSvc := NewWorkflowService(t.TempDir())
	if err := sourceSvc.Create(context.Background(), &WorkflowRecord{
		Name:     "bundle-demo",
		RawYAML:  []byte(testWorkflowYAML("bundle-demo", "echo initial")),
		Labels:   map[string]string{"env": "prod"},
		SaveNote: "created for export",
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := sourceSvc.Update(context.Background(), "bundle-demo", &WorkflowRecord{
		Name:     "bundle-demo",
		RawYAML:  []byte(testWorkflowYAML("bundle-demo", "echo exported")),
		SaveNote: "ready to bundle",
	}); err != nil {
		t.Fatalf("update workflow: %v", err)
	}

	bundle, err := sourceSvc.ExportBundle(context.Background(), "bundle-demo")
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}
	if bundle.Name != "bundle-demo" || !strings.Contains(bundle.YAML, "echo exported") {
		t.Fatalf("bundle current workflow mismatch: %+v", bundle)
	}
	if len(bundle.Versions) != 2 {
		t.Fatalf("bundle versions = %d, want 2", len(bundle.Versions))
	}

	targetSvc := NewWorkflowService(t.TempDir())
	imported, err := targetSvc.ImportBundle(context.Background(), bundle, WorkflowImportOptions{SaveNote: "imported bundle"})
	if err != nil {
		t.Fatalf("import bundle: %v", err)
	}
	if imported.Status != WorkflowStatusDraft {
		t.Fatalf("imported status = %q, want draft", imported.Status)
	}
	if imported.SaveNote != "imported bundle" {
		t.Fatalf("imported save note = %q", imported.SaveNote)
	}
	if imported.Labels["env"] != "prod" {
		t.Fatalf("imported labels mismatch: %+v", imported.Labels)
	}
	if !strings.Contains(string(imported.RawYAML), "echo exported") {
		t.Fatalf("imported yaml mismatch:\n%s", string(imported.RawYAML))
	}
	versions, err := targetSvc.ListVersions(context.Background(), "bundle-demo")
	if err != nil {
		t.Fatalf("list imported versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("imported versions = %d, want 2", len(versions))
	}
}

func testWorkflowYAML(name, cmd string) string {
	return testWorkflowYAMLWithAction(name, "script.shell", "script", cmd)
}

func testWorkflowYAMLWithCapabilityMismatch(name string) string {
	return `version: v0.1
name: ` + name + `
inventory:
  hosts:
    app-01:
      address: agent://app-01
      vars:
        capabilities: [script.python]
steps:
  - name: run
    targets: [app-01]
    action: script.shell
    args:
      script: echo ok
`
}

func testWorkflowYAMLWithAction(name, action, argKey, argValue string) string {
	return `version: v0.1
name: ` + name + `
steps:
  - name: run
    action: ` + action + `
    args:
      ` + argKey + `: ` + argValue + `
`
}

func isInvalidRiskAcknowledgementError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "risk_acknowledged")
}

func isInvalidWarningAcknowledgementError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "warning_acknowledged")
}

func isInvalidCapabilityError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "capability")
}

func isInvalidPublishValidationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "validated_graph_hash")
}

func isInvalidDryRunValidationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "dry_run_passed")
}

func validateForPublish(t *testing.T, svc *WorkflowService, name string) {
	t.Helper()
	validated, err := svc.ValidateWorkflow(context.Background(), name, WorkflowValidateOptions{Actor: "test"})
	if err != nil {
		t.Fatalf("validate %s for publish: %v", name, err)
	}
	if _, err := svc.MarkDryRunPassed(context.Background(), name, WorkflowDryRunOptions{
		Actor:             "test",
		ExpectedGraphHash: validated.ValidatedGraphHash,
	}); err != nil {
		t.Fatalf("mark %s dry-run passed for publish: %v", name, err)
	}
}
