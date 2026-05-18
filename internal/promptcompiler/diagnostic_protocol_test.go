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
		"置信度校准",
		"输出契约",
		"安全边界",
		"permission denied != 服务正常",
		"policy blocked != 目标系统状态",
		"timeout != 根因",
		"command not allowed != 证据不存在",
		"empty output != 一定无异常",
		"结论",
		"置信度",
		"支持证据",
		"反向证据",
		"缺失证据",
		"最小风险下一步",
		"需要审批的动作",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("diagnostic protocol missing %q:\n%s", want, section)
		}
	}
}

func TestDiagnosticProtocolSectionPlacement(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{}), "\n\n")
	evidence := strings.Index(developer, "## Evidence and Inference")
	diagnostic := strings.Index(developer, "## Diagnostic Protocol")
	investigation := strings.Index(developer, "## AIOps Investigation Loop")
	if evidence == -1 || diagnostic == -1 || investigation == -1 {
		t.Fatalf("developer instructions missing required sections:\n%s", developer)
	}
	if !(evidence < diagnostic && diagnostic < investigation) {
		t.Fatalf("Diagnostic Protocol placement invalid: evidence=%d diagnostic=%d investigation=%d", evidence, diagnostic, investigation)
	}
}

func TestDiagnosticProtocolCanBeDisabledByCompileContext(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{DisableDiagnosticProtocol: true}), "\n\n")
	if strings.Contains(developer, "## Diagnostic Protocol") {
		t.Fatalf("diagnostic protocol should be disabled:\n%s", developer)
	}
	if !strings.Contains(developer, "## Evidence and Inference") || !strings.Contains(developer, "## AIOps Investigation Loop") {
		t.Fatalf("other developer sections should remain:\n%s", developer)
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
