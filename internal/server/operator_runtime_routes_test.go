package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/operatorruntime"
	"aiops-v2/internal/runtimekernel"
)

type operatorRuntimeAPITestRuntime struct{}

func (operatorRuntimeAPITestRuntime) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (operatorRuntimeAPITestRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (operatorRuntimeAPITestRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestOperatorRuntimeResourcesCreateAndList(t *testing.T) {
	_, baseURL := newOperatorRuntimeAPITestServer(t, operatorruntime.NewMemoryStore())

	resource := validOperatorRuntimeResource()
	var created struct {
		Item operatorruntime.ManagedResource `json:"item"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/resources", resource, http.StatusOK, &created)
	if created.Item.ID != resource.ID {
		t.Fatalf("created item id = %q, want %q", created.Item.ID, resource.ID)
	}

	var listed struct {
		Items []operatorruntime.ManagedResource `json:"items"`
	}
	getOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/resources", &listed)
	if len(listed.Items) != 1 || listed.Items[0].ID != resource.ID || listed.Items[0].Kind != "redis" {
		t.Fatalf("listed resources = %+v, want only %q", listed.Items, resource.ID)
	}
}

func TestOperatorRuntimeValidationReturnsFieldErrors(t *testing.T) {
	_, baseURL := newOperatorRuntimeAPITestServer(t, operatorruntime.NewMemoryStore())

	var failed struct {
		Error       string                       `json:"error"`
		FieldErrors []operatorruntime.FieldError `json:"fieldErrors"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/resources", operatorruntime.ManagedResource{
		ID:   "",
		Name: "missing id",
	}, http.StatusBadRequest, &failed)

	if failed.Error == "" {
		t.Fatal("validation failure returned empty error")
	}
	if len(failed.FieldErrors) != 1 {
		t.Fatalf("fieldErrors len = %d, want 1: %+v", len(failed.FieldErrors), failed.FieldErrors)
	}
	if failed.FieldErrors[0].Field != "id" {
		t.Fatalf("field error field = %q, want id", failed.FieldErrors[0].Field)
	}
}

func TestOperatorRuntimeRuleEnableValidatesAndUpdatesRule(t *testing.T) {
	store := operatorruntime.NewMemoryStore()
	seedOperatorRuntimeCatalog(t, store)
	if err := store.SaveGuardRule(context.Background(), operatorruntime.GuardRule{
		ID:              "rule-no-binding",
		Name:            "Replica guard without workflow",
		ClusterRef:      "cluster-a",
		TemplateRef:     "template-a",
		ProblemTypeRefs: []string{"replica-lag"},
		ActionRefs:      []string{"restart-replica"},
		ScheduleSeconds: 30,
		Enabled:         false,
	}); err != nil {
		t.Fatalf("seed disabled rule without binding: %v", err)
	}
	if err := store.SaveGuardRule(context.Background(), operatorruntime.GuardRule{
		ID:                  "rule-with-binding",
		Name:                "Replica guard",
		ClusterRef:          "cluster-a",
		TemplateRef:         "template-a",
		ProblemTypeRefs:     []string{"replica-lag"},
		ActionRefs:          []string{"restart-replica"},
		WorkflowBindingRefs: []string{"restart-workflow"},
		ScheduleSeconds:     30,
		Enabled:             false,
	}); err != nil {
		t.Fatalf("seed disabled rule with binding: %v", err)
	}
	_, baseURL := newOperatorRuntimeAPITestServer(t, store)

	var failed struct {
		Error string `json:"error"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/rules/rule-no-binding/enable", nil, http.StatusBadRequest, &failed)
	if failed.Error == "" {
		t.Fatal("enable validation failure returned empty error")
	}

	var enabled struct {
		Item operatorruntime.GuardRule `json:"item"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/rules/rule-with-binding/enable", nil, http.StatusOK, &enabled)
	if !enabled.Item.Enabled {
		t.Fatalf("enabled rule = %+v, want enabled", enabled.Item)
	}

	var disabled struct {
		Item operatorruntime.GuardRule `json:"item"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/rules/rule-with-binding/disable", nil, http.StatusOK, &disabled)
	if disabled.Item.Enabled {
		t.Fatalf("disabled rule = %+v, want disabled", disabled.Item)
	}
}

func TestOperatorRuntimeRunApproveAndRejectAppendEvents(t *testing.T) {
	store := operatorruntime.NewMemoryStore()
	if err := store.CreateGuardRun(context.Background(), operatorruntime.GuardRun{
		ID:           "run-a",
		GuardRuleRef: "rule-a",
		State:        operatorruntime.GuardRunWaitingApproval,
	}); err != nil {
		t.Fatalf("seed guard run: %v", err)
	}
	_, baseURL := newOperatorRuntimeAPITestServer(t, store)

	var approved struct {
		Item operatorruntime.GuardRun `json:"item"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/runs/run-a/approve", map[string]string{"message": "ship it"}, http.StatusOK, &approved)
	if len(approved.Item.Events) != 1 || approved.Item.Events[0].Type != "approval.approved" || approved.Item.Events[0].Message != "ship it" {
		t.Fatalf("approved run events = %+v", approved.Item.Events)
	}

	var rejected struct {
		Item operatorruntime.GuardRun `json:"item"`
	}
	postOperatorRuntimeJSON(t, baseURL+"/api/v1/guards/runs/run-a/reject", nil, http.StatusOK, &rejected)
	if len(rejected.Item.Events) != 2 || rejected.Item.Events[1].Type != "approval.rejected" {
		t.Fatalf("rejected run events = %+v", rejected.Item.Events)
	}
}

func newOperatorRuntimeAPITestServer(t *testing.T, store operatorruntime.Store) (*httptest.Server, string) {
	t.Helper()
	service := appui.NewOperatorRuntimeService(store)
	srv := NewHTTPServer(appui.NewServices(operatorRuntimeAPITestRuntime{}, nil, appui.WithOperatorRuntimeService(service)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, ts.URL
}

func seedOperatorRuntimeCatalog(t *testing.T, store operatorruntime.Store) {
	t.Helper()
	ctx := context.Background()
	if err := store.SaveResource(ctx, validOperatorRuntimeCluster()); err != nil {
		t.Fatalf("seed resource: %v", err)
	}
	if err := store.SaveInspectionTemplate(ctx, operatorruntime.InspectionTemplate{
		ID:              "template-a",
		Name:            "Replication template",
		ObjectKind:      operatorruntime.ObjectKindPostgresReplication,
		IntervalSeconds: 30,
		PrimarySQL:      "select 1",
		ReplicaSQL:      "select 1",
		OutputFields: []operatorruntime.InspectionField{
			{Name: operatorruntime.FieldReplicaReceiverRunning, Type: operatorruntime.FieldTypeBool},
			{Name: operatorruntime.FieldReplicaReplayLagSeconds, Type: operatorruntime.FieldTypeNumber},
		},
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if err := store.SaveProblemType(ctx, operatorruntime.ProblemType{
		ID:                "replica-lag",
		DisplayName:       "Replica lag",
		Severity:          operatorruntime.SeverityWarning,
		ForSeconds:        30,
		AutoRepairAllowed: true,
		Conditions: []operatorruntime.ProblemCondition{
			{
				Field:    operatorruntime.FieldReplicaReplayLagSeconds,
				Operator: operatorruntime.OperatorGreaterThan,
				Value:    operatorruntime.KnownNumber(60),
			},
		},
	}); err != nil {
		t.Fatalf("seed problem type: %v", err)
	}
	if err := store.SaveAction(ctx, operatorruntime.ActionCatalogItem{
		ID:          "restart-replica",
		DisplayName: "Restart replica",
		RiskLevel:   operatorruntime.RiskMedium,
		TargetKind:  operatorruntime.TargetKindPostgresReplica,
		Steps: []operatorruntime.ActionStep{
			{ID: "restart", Kind: operatorruntime.ActionStepRestartService, RequiresApproval: true},
		},
	}); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	if err := store.SaveWorkflowBinding(ctx, operatorruntime.WorkflowBinding{
		ID:              "restart-workflow",
		ActionRef:       "restart-replica",
		WorkflowRef:     "wf-restart",
		WorkflowVersion: "v1",
		Capabilities:    []string{"preflight", "act", "verify"},
		VerifyPolicy: operatorruntime.VerifyPolicy{
			ReceiverRunningRequired: true,
			MaxReplayLagSeconds:     30,
			TimeoutSeconds:          300,
			IntervalSeconds:         10,
		},
	}); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
}

func validOperatorRuntimeCluster() operatorruntime.PGCluster {
	return operatorruntime.PGCluster{
		ID:   "cluster-a",
		Name: "Cluster A",
		Primary: operatorruntime.PGInstance{
			ID:   "primary-a",
			Role: operatorruntime.PGRolePrimary,
			Host: "10.0.0.1",
			Port: 5432,
		},
		Replicas: []operatorruntime.PGInstance{
			{
				ID:   "replica-a",
				Role: operatorruntime.PGRoleReplica,
				Host: "10.0.0.2",
				Port: 5432,
			},
		},
		MonitorCredentialRef: "monitor-secret",
		RepairCredentialRef:  "repair-secret",
	}
}

func validOperatorRuntimeResource() operatorruntime.ManagedResource {
	return operatorruntime.ManagedResource{
		ID:   "redis-cache-prod",
		Name: "Redis cache prod",
		Kind: "redis",
		Endpoints: []operatorruntime.ResourceEndpoint{
			{ID: "redis-cache-prod-a", Role: "leader", Host: "10.0.1.10", Port: 6379, ServiceName: "redis"},
			{ID: "redis-cache-prod-b", Role: "replica", Host: "10.0.1.11", Port: 6379, ServiceName: "redis"},
		},
		CredentialRefs: operatorruntime.CredentialRefs{
			Monitor: "redis-monitor-ref",
			Repair:  "redis-repair-ref",
		},
		Tags: []string{"production", "redis"},
	}
}

func getOperatorRuntimeJSON(t *testing.T, url string, target any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", url, resp.StatusCode, operatorRuntimeBodyString(resp))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode GET %s: %v", url, err)
	}
}

func postOperatorRuntimeJSON(t *testing.T, url string, body any, wantStatus int, target any) {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal POST %s: %v", url, err)
		}
		reader = bytes.NewReader(payload)
	}
	resp, err := http.Post(url, "application/json", reader)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s status = %d, want %d, body = %s", url, resp.StatusCode, wantStatus, operatorRuntimeBodyString(resp))
	}
	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("decode POST %s: %v", url, err)
		}
	}
}

func operatorRuntimeBodyString(resp *http.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return fmt.Sprintf("%s", buf.String())
}
