package workfloweditor

import (
	"encoding/json"
	"strings"
	"testing"

	"runner/server/service"
	contract "runner/workflow/edit"
)

func TestWorkflowResourcesV2AreOpaqueScopedAndRevisionAddressed(t *testing.T) {
	app, backend := newWorkflowReadFixture(t)
	resources, err := NewWorkflowResourcesV2(app)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := WorkflowContextResourceV2{Key: contract.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}, ActorID: "user-a", SessionID: "session-a", Revision: "revision-a", Query: service.WorkflowContextQuery{View: service.WorkflowContextFullView, Limit: 20}}
	ref, err := resources.BindContext(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ref.URI, "tenant-a") || strings.Contains(ref.URI, "workflow-a") || strings.Contains(ref.URI, "user-a") || ref.Version != "revision-a" {
		t.Fatalf("resource reference is not opaque/revision addressed: %#v", ref)
	}
	if _, err := resources.Read(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), ref.URI+"tampered"); err == nil {
		t.Fatal("tampered resource URI must fail closed")
	}
	content, err := resources.Read(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), ref.URI)
	if err != nil {
		t.Fatal(err)
	}
	var result contract.WorkflowContext
	if err := json.Unmarshal(content.Content, &result); err != nil || result.Revision.ResourceVersion != "revision-a" || len(result.Graph) == 0 {
		t.Fatalf("resource content/result = %s / %#v / %v", content.Content, result, err)
	}
	for _, ctx := range []struct{ tenant, user, workflow string }{{"tenant-b", "user-a", "workflow-a"}, {"tenant-a", "user-b", "workflow-a"}, {"tenant-a", "user-a", "workflow-b"}} {
		if _, err := resources.Read(workflowTrustedContext(ctx.tenant, ctx.user, ctx.workflow), ref.URI); err == nil || !strings.Contains(err.Error(), "resource unavailable") {
			t.Fatalf("cross-scope Read(%+v) error = %v", ctx, err)
		}
	}
	backend.mu.Lock()
	backend.snapshot.Revision.ResourceVersion = "revision-b"
	backend.mu.Unlock()
	if _, err := resources.Read(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), ref.URI); err == nil || !strings.Contains(err.Error(), "resource unavailable") {
		t.Fatalf("stale resource error = %v", err)
	}
}
