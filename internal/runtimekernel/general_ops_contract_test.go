package runtimekernel

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
)

func TestGeneralOpsModelInputIncludesOperationFrameV2Context(t *testing.T) {
	input, err := buildModelInput([]Message{{
		Role:    "user",
		Content: "主机A和主机B的PG主从集群异常,请帮忙恢复,数据可以不要,只需要PG主从集群可以正常运行,他们的pg_mon部署在主机C.",
	}}, promptcompiler.CompiledPrompt{})
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	var joined strings.Builder
	for _, msg := range input {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	got := joined.String()
	for _, want := range []string{
		"Operation Frame v2",
		"capability_path: stateful_middleware_cluster_repair",
		"generic_ops_contract: read_only_evidence_first,approval_before_mutation",
		"recommended_tool_flow: search_ops_manuals -> run_ops_manual_preflight",
		"monitor",
		"pg_mon",
		"data_loss_acceptable=true",
		"still_requires_approval=true",
		"observer_health",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("model input missing %q:\n%s", want, got)
		}
	}
}

func TestGeneralOpsModelInputIncludesObservabilityRCAContract(t *testing.T) {
	input, err := buildModelInput([]Message{{
		Role:    "user",
		Content: "@observability 分析环境A的A服务为什么异常，调用链是A服务->B服务->C服务。",
	}}, promptcompiler.CompiledPrompt{})
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	got := joinedModelInputContent(input)
	for _, want := range []string{
		"Observability RCA contract",
		"capability_path: observability_dependency_chain_rca",
		"generic_ops_contract: provider_neutral_observability,read_only_evidence_first",
		"observability_evidence: dependency_edges,hypotheses,missing_evidence",
		"dependency_chain_from_user: A服务->B服务->C服务",
		"provider_project_rule: environment_hint is not a provider project",
		"output_signals: capability_path=observability_dependency_chain_rca",
		"chain_candidate_template: for a chain X->Y->Z",
		"依赖链",
		"缺失证据",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("model input missing %q:\n%s", want, got)
		}
	}
}

func TestGeneralOpsModelInputDirectsExplicitCorootRCAEvidenceCollection(t *testing.T) {
	input, err := buildModelInput([]Message{{
		Role:    "user",
		Content: "@coroot 分析环境A的A服务,为什么异常",
	}}, promptcompiler.CompiledPrompt{})
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	got := joinedModelInputContent(input)
	for _, want := range []string{
		"provider_hint: explicit observability provider requested: coroot",
		"first_tool: provider aggregate RCA context tool if visible",
		"target_service: A服务",
		"environment_hint: A",
		"omit provider project when ambiguous",
		"high confidence only with Coroot edge evidence",
		"read-only Coroot evidence first",
		"high confidence only with provider dependency edge evidence",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("model input missing %q:\n%s", want, got)
		}
	}
}

func joinedModelInputContent(input []*schema.Message) string {
	var joined strings.Builder
	for _, msg := range input {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	return joined.String()
}
