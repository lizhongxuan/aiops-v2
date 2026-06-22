package operatorruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEvidenceBuilderBuildsLagEvidence(t *testing.T) {
	match := ProblemMatch{ProblemTypeID: "pg.replication.lag_high", ProblemDisplayName: "PG 复制延迟过高", Since: time.Now(), AutoRepairAllowed: true}
	evidence := BuildEvidence(match, lagInspectionResult(KnownNumber(120)))
	if len(evidence.Items) == 0 {
		t.Fatalf("expected evidence items")
	}
	if evidence.Items[0].Kind != EvidenceSupporting {
		t.Fatalf("expected supporting evidence, got %#v", evidence.Items)
	}
}

func TestEvidenceBuilderBuildsReceiverStoppedEvidence(t *testing.T) {
	result := InspectionResult{
		SnapshotID: "snap-1",
		ClusterID:  "pg-order",
		ReplicaID:  "pg-order-replica-a",
		Fields: map[string]FieldValue{
			FieldReplicaReachable:       KnownBool(true),
			FieldReplicaReceiverRunning: KnownBool(false),
		},
	}
	evidence := BuildEvidence(ProblemMatch{ProblemTypeID: "pg.replication.receiver_stopped"}, result)
	for _, item := range evidence.Items {
		if item.Field == FieldReplicaReceiverRunning && item.Value == "false" {
			return
		}
	}
	t.Fatalf("expected receiver stopped evidence, got %#v", evidence.Items)
}

func TestEvidenceBuilderRedactsSensitiveValues(t *testing.T) {
	result := lagInspectionResult(KnownNumber(120))
	result.Errors = []string{"password=secret token=abc"}
	evidence := BuildEvidence(ProblemMatch{ProblemTypeID: "pg.replication.lag_high"}, result)
	for _, item := range evidence.Items {
		if item.Value == "secret" || item.Value == "abc" {
			t.Fatalf("sensitive value leaked: %#v", evidence.Items)
		}
	}
}

func TestActionSelectorSelectsRecommendedBoundWorkflow(t *testing.T) {
	selected, err := SelectAction(validLagProblem(), []ActionCatalogItem{validAction()}, []WorkflowBinding{validWorkflowBinding()})
	if err != nil {
		t.Fatalf("select action: %v", err)
	}
	if selected.Action.ID != validAction().ID || selected.WorkflowBinding.ID != validWorkflowBinding().ID {
		t.Fatalf("unexpected selection: %#v", selected)
	}
}

func TestActionSelectorBlocksWhenWorkflowBindingMissing(t *testing.T) {
	_, err := SelectAction(validLagProblem(), []ActionCatalogItem{validAction()}, nil)
	if !errors.Is(err, ErrWorkflowBindingMissing) {
		t.Fatalf("expected missing binding error, got %v", err)
	}
}

func TestSafetyDeciderAllowsMediumNonRestart(t *testing.T) {
	action := validAction()
	action.Steps = []ActionStep{{ID: "reload_config", Kind: ActionStepReloadConfig}}
	action.ConfirmationRequiredSteps = nil
	decision := DecideSafety(validGuardRule(), action, validPGCluster().Replicas[0])
	if decision.Decision != DecisionAuto {
		t.Fatalf("expected medium non-restart action to be auto, got %#v", decision)
	}
}

func TestSafetyDeciderRequiresApprovalForRestart(t *testing.T) {
	decision := DecideSafety(validGuardRule(), validAction(), validPGCluster().Replicas[0])
	if decision.Decision != DecisionRequiresApproval {
		t.Fatalf("expected approval decision, got %#v", decision)
	}
}

func TestSafetyDeciderRecordsDecisionReason(t *testing.T) {
	decision := DecideSafety(validGuardRule(), validAction(), validPGCluster().Primary)
	if decision.Reason == "" {
		t.Fatalf("expected safety decision reason")
	}
}

func TestSafetyDeciderBlocksPrimaryTarget(t *testing.T) {
	decision := DecideSafety(validGuardRule(), validAction(), validPGCluster().Primary)
	if decision.Decision != DecisionBlocked {
		t.Fatalf("expected primary target to be blocked, got %#v", decision)
	}
}

func TestSafetyDeciderBlocksMissingRepairCredential(t *testing.T) {
	cluster := validPGCluster()
	cluster.RepairCredentialRef = ""
	decision := DecideSafetyForCluster(validGuardRule(), validAction(), cluster, cluster.Replicas[0])
	if decision.Decision != DecisionBlocked {
		t.Fatalf("expected missing repair credential to be blocked, got %#v", decision)
	}
}

func TestWorkflowInvokerStartsWorkflowWithMappedInputs(t *testing.T) {
	invoker := NewMemoryWorkflowInvoker()
	run, err := invoker.StartWorkflow(context.Background(), WorkflowStartRequest{
		GuardRunID:  "run-1",
		WorkflowRef: "builtin.postgres.replication_reconnect_replica.v1",
		Inputs: map[string]any{
			"resourceId":  "pg-order",
			"replicaHost": "10.0.0.11",
			"password":    "secret",
		},
	})
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	if run.ID == "" || run.Status != WorkflowRunSucceeded {
		t.Fatalf("unexpected workflow run: %#v", run)
	}
	if _, ok := run.Inputs["password"]; ok {
		t.Fatalf("workflow run should redact secret-like inputs: %#v", run.Inputs)
	}
}

func TestWorkflowInvokerRecordsRunFailure(t *testing.T) {
	invoker := NewMemoryWorkflowInvoker()
	run, err := invoker.StartWorkflow(context.Background(), WorkflowStartRequest{
		GuardRunID:  "run-1",
		WorkflowRef: "builtin.postgres.fail.v1",
		Inputs:      map[string]any{"resourceId": "pg-order"},
	})
	if err == nil {
		t.Fatalf("expected workflow failure error")
	}
	if run.Status != WorkflowRunFailed || run.Error == "" {
		t.Fatalf("expected failed run to be recorded, got %#v", run)
	}
}

type staticInspectionRunner struct {
	results []InspectionResult
}

func (s staticInspectionRunner) Inspect(context.Context, PGCluster, InspectionTemplate) ([]InspectionResult, error) {
	return s.results, nil
}

func TestRecoveryVerifierSucceedsWhenReceiverRunningAndLagLow(t *testing.T) {
	result := lagInspectionResult(KnownNumber(3))
	result.Fields[FieldReplicaReceiverRunning] = KnownBool(true)
	verifier := NewRecoveryVerifier(staticInspectionRunner{results: []InspectionResult{result}}, WithRecoveryVerifierSleeper(noopSleeper))
	got, err := verifier.Verify(context.Background(), validPGCluster(), validInspectionTemplate(), validPGCluster().Replicas[0], validWorkflowBinding().VerifyPolicy)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !got.Recovered {
		t.Fatalf("expected recovery success: %#v", got)
	}
}

func TestRecoveryVerifierFailsOnUnknownLag(t *testing.T) {
	result := lagInspectionResult(Unknown(FieldTypeNumber))
	result.Fields[FieldReplicaReceiverRunning] = KnownBool(true)
	verifier := NewRecoveryVerifier(staticInspectionRunner{results: []InspectionResult{result}}, WithRecoveryVerifierSleeper(noopSleeper))
	got, err := verifier.Verify(context.Background(), validPGCluster(), validInspectionTemplate(), validPGCluster().Replicas[0], validWorkflowBinding().VerifyPolicy)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Recovered {
		t.Fatalf("unknown lag should not be accepted as recovered: %#v", got)
	}
}

func TestRecoveryVerifierTimesOut(t *testing.T) {
	result := lagInspectionResult(KnownNumber(120))
	result.Fields[FieldReplicaReceiverRunning] = KnownBool(true)
	verifier := NewRecoveryVerifier(staticInspectionRunner{results: []InspectionResult{result}}, WithRecoveryVerifierSleeper(noopSleeper))
	policy := validWorkflowBinding().VerifyPolicy
	policy.TimeoutSeconds = 30
	policy.IntervalSeconds = 30
	got, err := verifier.Verify(context.Background(), validPGCluster(), validInspectionTemplate(), validPGCluster().Replicas[0], policy)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Recovered || got.Reason != "verification timeout" {
		t.Fatalf("expected timeout, got %#v", got)
	}
}

func TestFeedbackRecorderSummarizesSuccessfulRun(t *testing.T) {
	run := GuardRun{
		ID:            "run-1",
		State:         GuardRunSucceeded,
		ProblemTypeID: "pg.replication.lag_high",
		ActionRef:     "postgres.replication.reconnect_replica.v1",
		WorkflowRun:   &WorkflowRun{ID: "wf-1", Status: WorkflowRunSucceeded},
		Recovery:      &RecoveryVerification{Recovered: true},
	}
	feedback := NewFeedbackRecorder().Record(run)
	if feedback.Result != GuardRunSucceeded || !feedback.Recovered {
		t.Fatalf("unexpected feedback: %#v", feedback)
	}
}

func TestFeedbackRecorderSummarizesFailedRun(t *testing.T) {
	run := GuardRun{
		ID:            "run-1",
		GuardRuleRef:  "guard.pg-order.replication",
		State:         GuardRunFailed,
		ProblemTypeID: "pg.replication.receiver_stopped",
		ActionRef:     "postgres.replication.reconnect_replica.v1",
		WorkflowRun:   &WorkflowRun{ID: "wf-1", Status: WorkflowRunFailed, Error: "exit 1"},
		Recovery:      &RecoveryVerification{Recovered: false, Reason: "receiver still stopped"},
	}
	feedback := NewFeedbackRecorder().Record(run)
	if feedback.Result != GuardRunFailed || feedback.WorkflowStatus != WorkflowRunFailed || feedback.RecoveryReason == "" {
		t.Fatalf("unexpected feedback: %#v", feedback)
	}
}

func TestFeedbackRecorderRedactsSecrets(t *testing.T) {
	run := GuardRun{
		ID:          "run-1",
		State:       GuardRunFailed,
		WorkflowRun: &WorkflowRun{ID: "wf-1", Status: WorkflowRunFailed, Error: "password=secret token=abc"},
	}
	feedback := NewFeedbackRecorder().Record(run)
	if strings.Contains(feedback.WorkflowError, "secret") || strings.Contains(feedback.WorkflowError, "abc") {
		t.Fatalf("feedback leaked secret: %#v", feedback)
	}
}

func noopSleeper(context.Context, time.Duration) error {
	return nil
}
