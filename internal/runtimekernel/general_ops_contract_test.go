package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
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

func TestGeneralOpsModelInputIncludesDatabaseReplicationRecoveryEvidenceProfile(t *testing.T) {
	input, err := buildModelInput([]Message{{
		Role: "user",
		Content: `请只基于下面证据分析 PG timeline 异常，不要执行主机命令。

主机A:
$ pg_controldata /var/lib/postgresql/15/main | egrep 'Database cluster state|Latest checkpoint|TimeLineID'
Database cluster state:               in production
Latest checkpoint's TimeLineID:        7
Latest checkpoint's PrevTimeLineID:    6
$ psql -Atc 'select pg_is_in_recovery()'
f
$ test -f /var/lib/postgresql/15/main/standby.signal; echo $?
1

主机B:
$ pg_controldata /var/lib/postgresql/15/main | egrep 'Database cluster state|Latest checkpoint|TimeLineID'
Database cluster state:               in archive recovery
Latest checkpoint's TimeLineID:        9
Latest checkpoint's PrevTimeLineID:    8
$ psql -Atc 'select pg_is_in_recovery()'
t
$ test -f /var/lib/postgresql/15/main/standby.signal; echo $?
0
日志片段:
2026-06-23 10:13:02 CST [415] FATAL: requested timeline 7 is not a child of this server's history`,
	}}, promptcompiler.CompiledPrompt{})
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	got := joinedModelInputContent(input)
	for _, want := range []string{
		"Database replication/recovery RCA evidence profile",
		"capability_path: stateful_database_replication_recovery_rca",
		"user_evidence_first,no_host_execution_when_prohibited",
		"recovery_source_config",
		"affected_standby_label: B",
		"safe_rebuild_requirement",
		"preserve user-provided resource labels",
		"safe rebuild direction",
		"validation/check commands",
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
		Content: "分析环境A的A服务,为什么异常",
		Metadata: map[string]string{
			"aiops.mentions.observabilityProvider": "coroot",
		},
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
		"verified conclusion only with Coroot edge evidence",
		"read-only Coroot evidence first",
		"mark RCA as evidence-limited unless provider dependency edge evidence",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("model input missing %q:\n%s", want, got)
		}
	}
}

func TestGeneralOpsObservabilityContextIgnoresConnectionStringUserInfo(t *testing.T) {
	got := generalOpsObservabilityContext(`连接串 postgres://autoctl_node@172.25.1.91:5433/pg_auto_failover?sslmode=prefer 报错，请分析。`)
	if strings.Contains(got, "provider_hint") || strings.Contains(got, "172 edge evidence") || strings.Contains(got, "capability_path: observability_dependency_chain_rca") {
		t.Fatalf("connection string leaked into observability provider hint:\n%s", got)
	}
}

func joinedModelInputContent(input []promptinput.ModelInputItem) string {
	var joined strings.Builder
	for _, msg := range input {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	return joined.String()
}
