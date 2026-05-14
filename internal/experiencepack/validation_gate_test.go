package experiencepack

import "testing"

func TestValidationGateRejectsDangerousShellTokens(t *testing.T) {
	for _, token := range []string{"`", "$(", ";", "&", "|", ">", "<"} {
		if _, err := CompileValidationString("runner.readonly_probe:echo " + token + " unsafe"); err == nil {
			t.Fatalf("expected token %q to be rejected", token)
		}
	}
}

func TestValidationGateCompilesWhitelistValidators(t *testing.T) {
	task, err := CompileValidationString("coroot.metric_check:service=checkout,metric=p95")
	if err != nil {
		t.Fatalf("compile validation: %v", err)
	}
	if task.Validator != "coroot.metric_check" || task.Mode != "read_only" || task.TimeoutSeconds != 180 {
		t.Fatalf("unexpected task: %#v", task)
	}
}

func TestValidationGateRejectsUnknownValidators(t *testing.T) {
	if _, err := CompileValidationString("shell.exec:rm -rf"); err == nil {
		t.Fatal("unknown validator should be rejected")
	}
}

func TestValidationGateReportRedactsSensitiveArgs(t *testing.T) {
	gene := testGene("gene_validation_redaction")
	gene.Validation = []string{"runner.readonly_probe:token=secret-token,service=checkout"}
	gene.AssetID = MustHashCanonicalJSON(gene)

	report := CheckValidationGate(gene)
	if !report.Passed || !report.Redacted {
		t.Fatalf("report = %#v, want passed redacted", report)
	}
	if got := report.CompiledTasks[0].Args["token"]; got != "[REDACTED]" {
		t.Fatalf("token arg = %#v, want redacted", got)
	}
	if got := report.CompiledTasks[0].Args["service"]; got != "checkout" {
		t.Fatalf("service arg = %#v, want checkout", got)
	}
}
