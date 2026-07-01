package promptcompiler

import (
	"strings"
	"testing"
)

func TestDiagnosticProtocolSectionContentAndLength(t *testing.T) {
	section := renderDeveloperSection(developerSection{
		title: "Diagnostic Protocol",
		lines: diagnosticProtocolLines(),
	})

	if len(strings.Join(diagnosticProtocolLines(), "\n")) > 2200 {
		t.Fatalf("diagnostic protocol content length = %d, want <= 2200", len(strings.Join(diagnosticProtocolLines(), "\n")))
	}

	for _, want := range []string{
		"## Diagnostic Protocol",
		"诊断协议",
		"证据矩阵",
		"工具失败语义",
		"证据边界校准",
		"输出契约",
		"安全边界",
		"choose only the sections the user needs",
		"do not force a fixed template",
		"Put section label and first sentence on the same line",
		"permission denied != 服务正常",
		"policy blocked != 目标系统状态",
		"timeout != 根因",
		"command not allowed != 证据不存在",
		"empty output != 一定无异常",
		"single failed observability source",
		"target is known",
		"aggregate evidence",
		"independent drill-down",
		"external dependency edge",
		"not a terminal root cause",
		"resolve dependency identity",
		"endpoint/port",
		"restart-loop RCA",
		"health/readiness/liveness probes",
		"candidate cause",
		"restart policy",
		"结论",
		"证据边界",
		"关键证据",
		"仍缺少的证据",
		"下一步",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("diagnostic protocol missing %q:\n%s", want, section)
		}
	}
}

func TestDiagnosticProtocolSectionPlacement(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	if strings.Contains(modelInput, "## Diagnostic Protocol") {
		t.Fatalf("diagnostic protocol must not be rendered in the model envelope:\n%s", modelInput)
	}
	for _, want := range []string{"base.contract", "runtime.state", "tool.surface"} {
		if !strings.Contains(modelInput, want) {
			t.Fatalf("model envelope missing compact section %q:\n%s", want, modelInput)
		}
	}
}

func TestDiagnosticProtocolCanBeDisabledByCompileContext(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{DisableDiagnosticProtocol: true})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	if strings.Contains(modelInput, "## Diagnostic Protocol") {
		t.Fatalf("diagnostic protocol should be disabled:\n%s", modelInput)
	}
	if !strings.Contains(modelInput, "base.contract") || !strings.Contains(modelInput, "tool.surface") {
		t.Fatalf("compact envelope sections should remain:\n%s", modelInput)
	}
}

func TestDiagnosticProtocolAvoidsDomainSpecificRunbookContent(t *testing.T) {
	section := strings.Join(diagnosticProtocolLines(), "\n")
	for _, forbidden := range []string{
		"redis-cli",
		"kubectl",
		"CrashLoopBackOff",
		"RDB",
		"Sentinel",
		"StatefulSet",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("diagnostic protocol should stay generic, found %q:\n%s", forbidden, section)
		}
	}
}
