package runtimekernel

import "testing"

func TestManagerSynthesisGateUsesTypedManagerRefNotAnswerText(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-manager")
	snapshot.Metadata["managerSynthesis.workerOutputRefs"] = "synthetic-worker-output-1,synthetic-worker-output-2"
	snapshot.Metadata["managerSynthesis.managerAnswerRef"] = "synthetic-manager-answer-1"

	gate := EvaluateManagerSynthesisGate(snapshot, "synthetic-worker-output-1: raw finding\nsynthetic-worker-output-2: raw finding")

	if gate.Action != "allow_final" {
		t.Fatalf("Action = %q, want allow_final from typed manager ref", gate.Action)
	}
	if len(gate.WorkerOutputRefs) != 2 {
		t.Fatalf("WorkerOutputRefs = %#v, want both synthetic worker refs", gate.WorkerOutputRefs)
	}
}

func TestManagerSynthesisGateRequiresTypedManagerRefRegardlessOfAnswer(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-manager-missing")
	snapshot.Metadata["managerSynthesis.workerOutputRefs"] = "synthetic-worker-output-1"

	claim := EvaluateManagerSynthesisGate(snapshot, "manager synthesis completed")
	blocker := EvaluateManagerSynthesisGate(snapshot, "manager synthesis missing")
	if claim.Action != "require_manager_synthesis" || blocker.Action != claim.Action {
		t.Fatalf("answer text changed manager gate: claim=%#v blocker=%#v", claim, blocker)
	}
}

func TestManagerSynthesisGateAllowsManagerAnswerWithWorkerRefs(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-manager-allow")
	snapshot.Metadata["managerSynthesis.workerOutputRefs"] = "synthetic-worker-output-1,synthetic-worker-output-2"
	snapshot.Metadata["managerSynthesis.managerAnswerRef"] = "synthetic-manager-answer-1"

	gate := EvaluateManagerSynthesisGate(snapshot, "Manager synthesis ref synthetic-manager-answer-1 reports consolidated synthetic outcome.")

	if gate.Action != "allow_final" {
		t.Fatalf("Action = %q, want allow_final", gate.Action)
	}
	if gate.ManagerAnswerRef != "synthetic-manager-answer-1" {
		t.Fatalf("ManagerAnswerRef = %q, want synthetic-manager-answer-1", gate.ManagerAnswerRef)
	}
}
