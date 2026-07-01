package appui

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/runtimecontract"
)

func TestBuildEvidenceEnvelopePlainQuestionHasNoUserEvidence(t *testing.T) {
	envelope := BuildEvidenceEnvelope("为什么状态会变高？", nil, nil)

	if envelope.HasUserProvidedEvidence {
		t.Fatalf("HasUserProvidedEvidence = true, want false: %#v", envelope)
	}
	if len(envelope.WeakSignals) != 0 {
		t.Fatalf("WeakSignals = %#v, want none", envelope.WeakSignals)
	}
}

func TestBuildEvidenceEnvelopeLogProducesWeakSignalOnly(t *testing.T) {
	input := "2026-06-23 10:00:01 ERROR: upstream timed out\n2026-06-23 10:00:02 WARNING: retry failed"
	envelope := BuildEvidenceEnvelope(input, nil, nil)

	if !envelope.HasUserProvidedEvidence {
		t.Fatalf("HasUserProvidedEvidence = false, want true")
	}
	if !containsEvidenceKind(envelope.EvidenceKinds, runtimecontract.EvidenceKindLog) {
		t.Fatalf("EvidenceKinds = %#v, want log", envelope.EvidenceKinds)
	}
	if !containsWeakSignal(envelope.WeakSignals, runtimecontract.WeakSignalLogLikeText) {
		t.Fatalf("WeakSignals = %#v, want log-like weak signal", envelope.WeakSignals)
	}
}

func TestBuildEvidenceEnvelopeConfigSignalUsesGenericNames(t *testing.T) {
	input := "restore_command = 'cp /archive/%f %p'\nrecovery_target_timeline = 'latest'"
	envelope := BuildEvidenceEnvelope(input, nil, nil)

	if !containsWeakSignal(envelope.WeakSignals, runtimecontract.WeakSignalConfigLikeText) {
		t.Fatalf("WeakSignals = %#v, want config-like weak signal", envelope.WeakSignals)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(strings.ToLower(string(data)), "postgres") {
		t.Fatalf("EvidenceEnvelope JSON must not include product-specific names: %s", data)
	}
}

func TestBuildIntentFrameExplainDoesNotRequestHostExec(t *testing.T) {
	envelope := BuildEvidenceEnvelope("不要执行命令，只解释这段日志含义：ERROR: upstream timed out", nil, nil)
	frame := BuildIntentFrame("不要执行命令，只解释这段日志含义：ERROR: upstream timed out", envelope, nil)

	if frame.Kind != runtimecontract.IntentKindExplain {
		t.Fatalf("Kind = %q, want explain", frame.Kind)
	}
	if runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) {
		t.Fatalf("RiskBudget = %#v, must not include host_exec", frame.RiskBudget)
	}
	if !hasIntentConstraint(frame, "no_host_exec") {
		t.Fatalf("Constraints = %#v, want no_host_exec", frame.Constraints)
	}
}

func TestBuildIntentFrameHostExecIsRiskBudgetNotApproval(t *testing.T) {
	envelope := BuildEvidenceEnvelope("帮我读取本机状态并定位为什么不断重启", nil, nil)
	frame := BuildIntentFrame("帮我读取本机状态并定位为什么不断重启", envelope, nil)

	if frame.Kind != runtimecontract.IntentKindDiagnose && frame.Kind != runtimecontract.IntentKindVerify {
		t.Fatalf("Kind = %q, want diagnose or verify", frame.Kind)
	}
	if !runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) {
		t.Fatalf("RiskBudget = %#v, want host_exec request risk", frame.RiskBudget)
	}
	if len(frame.Capabilities) == 0 {
		t.Fatalf("Capabilities = %#v, want candidate only", frame.Capabilities)
	}
	for _, candidate := range frame.Capabilities {
		if candidate.Name == "approved_host_exec" {
			t.Fatalf("Capabilities = %#v, must not encode approval as granted capability", frame.Capabilities)
		}
	}
}

func TestBuildIntentFrameHostResourceReadRequestsHostRuntime(t *testing.T) {
	envelope := BuildEvidenceEnvelope("查看 CPU 情况", nil, nil)
	frame := BuildIntentFrame("查看 CPU 情况", envelope, nil)

	if frame.Kind != runtimecontract.IntentKindVerify {
		t.Fatalf("Kind = %q, want verify for host resource read", frame.Kind)
	}
	if !runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime) {
		t.Fatalf("DataScopes = %#v, want local_runtime", frame.DataScopes)
	}
	if !runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) {
		t.Fatalf("RiskBudget = %#v, want host_exec request risk", frame.RiskBudget)
	}
}

func TestBuildIntentFrameHostResourceExplanationDoesNotRequestHostRuntime(t *testing.T) {
	envelope := BuildEvidenceEnvelope("解释 Linux load average 是什么", nil, nil)
	frame := BuildIntentFrame("解释 Linux load average 是什么", envelope, nil)

	if runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) {
		t.Fatalf("RiskBudget = %#v, should not request host_exec for pure explanation", frame.RiskBudget)
	}
	if runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime) {
		t.Fatalf("DataScopes = %#v, should not include local_runtime for pure explanation", frame.DataScopes)
	}
}

func containsEvidenceKind(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsWeakSignal(values []runtimecontract.WeakSignal, want string) bool {
	for _, value := range values {
		if value.Name == want {
			return true
		}
	}
	return false
}

func hasIntentConstraint(frame runtimecontract.IntentFrame, name string) bool {
	for _, constraint := range frame.Constraints {
		if constraint.Name == name {
			return true
		}
	}
	return false
}
