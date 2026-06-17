# aiops-v2 General Ops Capability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 `docs/2026-06-17-aiops-v2-general-ops-capability-modification-design.zh.md` 落地 aiops-v2 通用运维能力优先改造，让 PG 恢复、PG workflow 生成、Coroot 服务链 RCA 成为通用能力的验收样本，而不是专项硬编码。

**Architecture:** 采用“Operation Frame v2 + 通用能力 contract + 能力包边界 + eval 回归门禁”的分层方案。Core 只理解资源、角色、关系、执行面、观察点、风险、证据和治理状态；PostgreSQL、pg_mon、Coroot 等具体名称只能进入 capability pack、provider adapter、fixture、eval case 或文档边界。RuntimeKernel 通过 Operation Frame 和 capability contract 路由，WorkflowGen 生成资源型 draft workflow，Coroot adapter 输出通用 observability evidence。

**Tech Stack:** Go 1.24.3, aiops-v2 `internal/opsmanual`, `internal/workflowgen`, `internal/runtimekernel`, `internal/integrations/coroot`, `internal/eval`, existing ToolDispatcher / ActionToken / approval governance, Runner visual graph.

---

## 0. 实施边界

- [ ] 不在 core runtime、promptcompiler、router、policy、workflowgen 主路径、opsmanual 主路径或前端状态中加入只服务 PG、pg_mon、Coroot、主机A/B/C、服务A/B/C 的固定分支。
- [ ] `pg_mon` 只作为运行时识别到的 monitor/observer 角色进入 Operation Frame，不作为默认内置集成。
- [ ] PG 能力只能作为首个有状态中间件 capability pack 验证通用 repair flow。
- [ ] Coroot 只能作为首个 observability provider 验证通用 evidence contract。
- [ ] “数据可以不要”只设置风险偏好，不能跳过审批、备份检查、Run Record 或恢复验证。
- [ ] Workflow 生成结果只能进入 draft / pending_review，不能直接标记 verified。
- [ ] 高风险或 destructive 动作审批前不执行，仍然走既有 ToolDispatcher、ActionToken 和 approval。
- [ ] 多主机运维任务必须在对话输入框上方展示已识别/已选择的主机列表；列表交互、主机去重、主机删除、主机状态展示必须可用。
- [ ] 多主机运维任务必须保持“一台主机对应一个 host-bound child agent”，不能把多台主机合并给一个 agent，也不能让 child agent 跨主机执行。
- [ ] 每个阶段都先写失败测试，再实现最小代码，再跑定向测试。

## 1. 文件结构

### 新增后端文件

- `internal/opsmanual/operation_context_test.go`
  - 覆盖 Operation Frame v2 资源角色、关系、执行面、观察点、风险偏好和证据需求。
- `internal/opsmanual/operation_context.go`
  - 放置 Operation Frame v2 的纯类型和通用 helper。
- `internal/opsmanual/resource_role_extractor.go`
  - 从用户文本和 metadata 中抽取通用 host/service/component 角色关系。
- `internal/opsmanual/resource_role_extractor_test.go`
  - 覆盖 A/B 数据节点、C monitor、非 PG 变体和访问方式未知补槽。
- `internal/opsrepair/types.go`
  - 定义通用 `RepairPlan`、`RepairOption`、`RepairStep`、`RepairEvidence`、`RepairVerification`。
- `internal/opsrepair/planner.go`
  - 基于 Operation Frame v2 和只读证据生成通用 repair options。
- `internal/opsrepair/planner_test.go`
  - 覆盖单方案、多方案、证据不足、高风险审批和数据丢失偏好。
- `internal/opsmanual/postgres_capability_pack.go`
  - 仅在 capability pack 边界提供 PostgreSQL 别名、只读 probe 映射和验证断言。
- `internal/opsmanual/postgres_capability_pack_test.go`
  - 确认 PG 专项知识只出现在 capability pack/test 边界。
- `internal/workflowgen/resource_plan_builder.go`
  - 资源型 workflow plan builder，生成 preflight / execute / verify / rollback 阶段。
- `internal/workflowgen/resource_plan_builder_test.go`
  - 覆盖 PG 样例、Redis 变体、secret ref、高风险 review gate。
- `internal/observability/evidence_contract.go`
  - 定义通用 observability evidence contract。
- `internal/observability/evidence_contract_test.go`
  - 覆盖证据 pack JSON round-trip 和低置信缺失证据。
- `internal/integrations/coroot/evidence_mapper.go`
  - 将 Coroot 工具输出映射到通用 observability evidence。
- `internal/integrations/coroot/evidence_mapper_test.go`
  - 覆盖 A->B->C 依赖链、C 服务根因、Coroot 不可用时证据缺失。

### 修改后端文件

- `internal/opsmanual/types.go`
  - `OperationFrame` 增加 roles、relationships、execution_surface、observation_points、risk_preference、evidence_requirements。
- `internal/opsmanual/operation_frame.go`
  - 接入 resource role extractor 和风险偏好解析。
- `internal/opsmanual/capability_registry.go`
  - 增加通用 capability contract metadata，不加入 provider-specific core 分支。
- `internal/opsmanual/capability_registry_test.go`
  - 验证 capability pack 启停和 provider-specific 边界。
- `internal/workflowgen/types.go`
  - 增加 resource workflow 类型字段和 draft review 状态。
- `internal/workflowgen/plan_builder.go`
  - 保留原有 deterministic builder，新增入口根据 Operation Frame 调用 resource plan builder。
- `internal/workflowgen/graph_generator.go`
  - 将资源型 plan 阶段映射为 Runner graph 节点和 workflow vars。
- `internal/workflowgen/builder_agent.go`
  - 生成后保持 draft / pending_review，不自动 verified。
- `internal/runtimekernel/eino_kernel.go`
  - Chat route 使用 Operation Frame 和 capability contract 选择链路，不新增 provider-specific 分支。
- `internal/runtimekernel/model_input.go`
  - 将 Operation Frame v2 摘要和 observability evidence limits 注入模型上下文。
- `internal/runtimekernel/genericity_guard.go`
  - 扩展 provider-specific 名称扫描分类。
- `internal/runtimekernel/genericity_guard_test.go`
  - 扫描 core 文件，禁止新增 PG/pg_mon/Coroot 固定规则。
- `internal/eval/types.go`
  - 增加通用能力期望字段：operation frame、resource roles、capability path、workflow review status、observability evidence。
- `internal/eval/scorer.go`
  - 评分上述字段。
- `internal/eval/mock_agent.go`
  - 让 mock 输出可覆盖新增字段，便于回归 runner 自测。
- `internal/hostops/orchestrator.go`
  - 强化一个 mission 内 host 去重和一主机一 child agent 的约束。
- `internal/hostops/orchestrator_test.go`
  - 覆盖多主机 assignment 生成多个 child agent、重复 host 不重复创建、child agent 不跨 host。
- `internal/hostops/types.go`
  - 如 transport 缺字段，补充 host list / child agent 绑定所需字段；优先复用现有 `HostMention`、`HostOperationMission`、`HostChildAgent`。
- `internal/appui/transport_state.go`
  - 确保 host mission、host mentions、child agents 能投影到前端输入框上方状态面板。
- `internal/appui/transport_projector.go`
  - 投影一主机一 agent 的绑定关系，不只投影 plan 文本。

### 修改前端文件

- `web/src/chat/components/AiopsComposer.tsx`
  - 在 composer 上方接入主机列表和 host mention 状态，不让列表藏在消息正文或 drawer 里。
- `web/src/chat/components/HostMentionComposer.tsx`
  - 保持 `@host` 输入、选择、删除、去重和键盘交互可用。
- `web/src/chat/components/ComposerHostMentionMenu.tsx`
  - 展示可选主机并把选择结果回填到输入状态。
- `web/src/chat/components/HostMentionChip.tsx`
  - 用于主机列表中的单主机 chip，支持删除和状态提示。
- `web/src/chat/components/HostOpsStatusPanel.tsx`
  - 在对话输入框上方展示 active mission 的主机列表、计划状态和子 Agent 状态。
- `web/src/chat/components/HostSubagentStatusRow.tsx`
  - 每个 host-bound child agent 一行或一个紧凑状态项，点击能打开对应 drawer。
- `web/src/chat/components/HostSubagentDrawer.tsx`
  - 显示单个 child agent 的 host、任务、transcript 和 follow-up 输入。
- `web/src/chat/components/HostSubagentTabs.tsx`
  - 多 child agent 时按主机切换，不合并 transcript。
- `web/src/transport/aiopsTransportTypes.ts`
  - 确保 host mission、host mentions、childAgents 类型能表达 host-to-agent 绑定。
- `web/src/transport/aiopsTransportConverter.ts`
  - 保留 host mission / child agent metadata，避免 assistant-ui 转换时丢失输入框上方状态。

### 修改评测与文档

- `testdata/eval_cases/pg-cluster-recovery-chat.json`
  - 增加 Operation Frame、repair flow、approval、verification、non-hardcode 断言。
- `testdata/eval_cases/pg-cluster-workflow-generation.json`
  - 增加 draft workflow、resource stages、pg_mon monitor role、secret ref、review gate 断言。
- `testdata/eval_cases/coroot-service-chain-rca.json`
  - 增加 observability evidence、dependency chain、root cause confidence 断言。
- `testdata/eval_cases/redis-stateful-repair-chat.json`
  - 新增非 PG 同类变体，证明通用 repair flow 可复用。
- `testdata/eval_cases/generic-observability-rca.json`
  - 新增非 Coroot provider fixture，证明 RCA 链路不依赖 Coroot 名称。
- `docs/2026-06-17-aiops-v2-general-ops-capability-modification-design.zh.md`
  - 实施完成后补充落地结果和验证命令。
- `README.md`
  - 仅在必要时补充最终边界，不重复设计文档。

## 2. Task 0：建立 baseline 和工作区记录

**Files:**
- Read: `docs/2026-06-17-aiops-v2-general-ops-capability-modification-design.zh.md`
- Read: `docs/2026-06-17-aiops-v2-pg-coroot-workflow-acceptance.zh.md`
- Read: `README.md`

- [ ] **Step 0.1：记录当前 git 状态**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
git rev-parse HEAD
git status --short
```

Expected:

- 输出当前 commit hash。
- 记录已有未提交变更；后续任务不能回滚无关变更。

- [ ] **Step 0.2：跑现有核心测试 baseline**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/opsmanual ./internal/workflowgen ./internal/runtimekernel ./internal/integrations/coroot ./internal/eval ./cmd/agent-eval ./cmd/agent-eval-case
```

Expected:

- PASS，或记录已有失败包、测试名和失败原因。
- 若失败来自无关既有变更，记录为 baseline blocker，不在本任务中顺手修复。

- [ ] **Step 0.3：跑现有目标 eval baseline**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go run ./cmd/agent-eval -agent mock -cases testdata/eval_cases -priority P1 -out .data/eval_runs/general-ops-capability-baseline-mock
```

Expected:

- runner 能完整执行并输出 report。
- `pg-cluster-recovery-chat`、`pg-cluster-workflow-generation`、`coroot-service-chain-rca` 在真实能力完成前允许失败，作为改造目标。

## 3. Task 1：Operation Frame v2 类型与 JSON 契约

**Files:**
- Modify: `internal/opsmanual/types.go`
- Create: `internal/opsmanual/operation_context.go`
- Create: `internal/opsmanual/operation_context_test.go`
- Modify: `internal/opsmanual/operation_frame_json.go`
- Test: `internal/opsmanual/operation_frame_test.go`

- [ ] **Step 1.1：写 Operation Frame v2 JSON round-trip 测试**

Create `internal/opsmanual/operation_context_test.go` with these tests:

```go
func TestOperationFrameV2JSONRoundTripPreservesRolesAndRiskPreference(t *testing.T) {
	frame := OperationFrame{
		Target: OperationTarget{Type: "postgresql", Name: "pg-cluster"},
		Roles: []OperationResourceRole{
			{ID: "host-a", Kind: ResourceRoleDataNode, ResourceRef: "host-a", UserLabel: "主机A", InferredFrom: "user_input"},
			{ID: "host-c-monitor", Kind: ResourceRoleMonitor, ResourceRef: "host-c", UserLabel: "主机C", RuntimeName: "pg_mon", InferredFrom: "user_input"},
		},
		Relationships: []OperationResourceRelationship{
			{From: "host-c", To: "pg-cluster", Type: RelationshipMonitors},
		},
		ExecutionSurfaceV2: OperationExecutionSurface{Kind: ExecutionSurfaceHostAgent, Resources: []string{"host-a", "host-b"}},
		ObservationPoints: []OperationObservationPoint{
			{Kind: ObservationPointMonitorComponent, ResourceRef: "host-c", Role: "pg_mon", Access: ObservationAccessUnknown},
		},
		RiskPreference: OperationRiskPreference{DataLossAcceptable: true, StillRequiresApproval: true},
		EvidenceRequirements: []string{"cluster_role", "member_health", "observer_health"},
	}
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	var decoded OperationFrame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Roles[1].Kind != ResourceRoleMonitor || decoded.ObservationPoints[0].Role != "pg_mon" {
		t.Fatalf("decoded monitor role = %#v", decoded)
	}
	if !decoded.RiskPreference.DataLossAcceptable || !decoded.RiskPreference.StillRequiresApproval {
		t.Fatalf("risk preference lost: %#v", decoded.RiskPreference)
	}
}
```

Run:

```bash
go test -count=1 ./internal/opsmanual -run 'TestOperationFrameV2JSONRoundTrip'
```

Expected before implementation: FAIL because `OperationFrame.Roles` and related types do not exist.

- [ ] **Step 1.2：实现 Operation Frame v2 最小类型**

Modify `internal/opsmanual/types.go` and add fields to `OperationFrame`:

```go
Roles                []OperationResourceRole         `json:"roles,omitempty"`
Relationships        []OperationResourceRelationship `json:"relationships,omitempty"`
ExecutionSurfaceV2   OperationExecutionSurface       `json:"execution_surface_v2,omitempty"`
ObservationPoints    []OperationObservationPoint     `json:"observation_points,omitempty"`
RiskPreference       OperationRiskPreference         `json:"risk_preference,omitempty"`
EvidenceRequirements []string                        `json:"evidence_requirements,omitempty"`
```

Create `internal/opsmanual/operation_context.go` with constants:

```go
const (
	ResourceRoleDataNode = "data_node"
	ResourceRoleMonitor  = "monitor"
	ResourceRoleProxy    = "proxy"
	ResourceRoleObserver = "observer"
	ResourceRoleExecutor = "executor"

	RelationshipMonitors    = "monitors"
	RelationshipReplicatesTo = "replicates_to"

	ExecutionSurfaceHostAgent = "host_agent"
	ExecutionSurfaceRunner    = "runner"
	ExecutionSurfaceMCP       = "mcp"
	ExecutionSurfaceAPI       = "api"
	ExecutionSurfaceUnknown   = "unknown"

	ObservationPointMonitorComponent = "monitor_component"
	ObservationAccessHostAgent       = "host_agent"
	ObservationAccessHTTP            = "http"
	ObservationAccessUnknown         = "unknown"
)
```

Required structs:

```go
type OperationResourceRole struct {
	ID           string `json:"id,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ResourceRef  string `json:"resource_ref,omitempty"`
	UserLabel    string `json:"user_label,omitempty"`
	RuntimeName  string `json:"runtime_name,omitempty"`
	InferredFrom string `json:"inferred_from,omitempty"`
}

type OperationResourceRelationship struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Type string `json:"type,omitempty"`
}

type OperationExecutionSurface struct {
	Kind      string   `json:"kind,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

type OperationObservationPoint struct {
	Kind        string `json:"kind,omitempty"`
	ResourceRef string `json:"resource_ref,omitempty"`
	Role        string `json:"role,omitempty"`
	Access      string `json:"access,omitempty"`
}

type OperationRiskPreference struct {
	DataLossAcceptable  bool `json:"data_loss_acceptable,omitempty"`
	StillRequiresApproval bool `json:"still_requires_approval,omitempty"`
}
```

- [ ] **Step 1.3：跑 Operation Frame v2 类型测试**

Run:

```bash
go test -count=1 ./internal/opsmanual -run 'TestOperationFrameV2JSONRoundTrip'
```

Expected: PASS.

## 4. Task 2：通用资源角色理解与补槽

**Files:**
- Create: `internal/opsmanual/resource_role_extractor.go`
- Create: `internal/opsmanual/resource_role_extractor_test.go`
- Modify: `internal/opsmanual/operation_frame.go`
- Test: `internal/opsmanual/operation_frame_test.go`

- [ ] **Step 2.1：写 PG 样例角色抽取失败测试**

Create `internal/opsmanual/resource_role_extractor_test.go` with:

```go
func TestBuildOperationFrameAssignsDataNodesAndMonitorRole(t *testing.T) {
	frame := BuildOperationFrame("主机A和主机B的PG主从集群异常,请帮忙恢复,数据可以不要,只需要PG主从集群可以正常运行,他们的pg_mon部署在主机C.", nil)
	if got := roleKindByResource(frame, "主机A"); got != ResourceRoleDataNode {
		t.Fatalf("主机A role = %q, want data_node; frame=%#v", got, frame)
	}
	if got := roleKindByResource(frame, "主机B"); got != ResourceRoleDataNode {
		t.Fatalf("主机B role = %q, want data_node; frame=%#v", got, frame)
	}
	monitor := roleByRuntimeName(frame, "pg_mon")
	if monitor.Kind != ResourceRoleMonitor || monitor.ResourceRef != "主机C" {
		t.Fatalf("pg_mon monitor role = %#v, frame=%#v", monitor, frame)
	}
	if !frame.RiskPreference.DataLossAcceptable || !frame.RiskPreference.StillRequiresApproval {
		t.Fatalf("risk preference = %#v", frame.RiskPreference)
	}
}
```

Add helper functions inside the test file:

```go
func roleKindByResource(frame OperationFrame, resource string) string {
	for _, role := range frame.Roles {
		if role.ResourceRef == resource || role.UserLabel == resource {
			return role.Kind
		}
	}
	return ""
}

func roleByRuntimeName(frame OperationFrame, runtimeName string) OperationResourceRole {
	for _, role := range frame.Roles {
		if role.RuntimeName == runtimeName {
			return role
		}
	}
	return OperationResourceRole{}
}
```

Run:

```bash
go test -count=1 ./internal/opsmanual -run 'TestBuildOperationFrameAssignsDataNodesAndMonitorRole'
```

Expected before implementation: FAIL because roles are not populated.

- [ ] **Step 2.2：写非 PG 同类角色抽取测试**

Add:

```go
func TestBuildOperationFrameUsesGenericMonitorRoleForRedisVariant(t *testing.T) {
	frame := BuildOperationFrame("主机A和主机B的Redis主从集群异常，sentinel监控部署在主机C，只需要集群恢复正常。", nil)
	if got := roleKindByResource(frame, "主机A"); got != ResourceRoleDataNode {
		t.Fatalf("主机A role = %q, want data_node; frame=%#v", got, frame)
	}
	monitor := roleByRuntimeName(frame, "sentinel")
	if monitor.Kind != ResourceRoleMonitor || monitor.ResourceRef != "主机C" {
		t.Fatalf("sentinel monitor role = %#v, frame=%#v", monitor, frame)
	}
	if frame.Target.Type == "postgresql" {
		t.Fatalf("redis variant was polluted by PG target type: %#v", frame.Target)
	}
}
```

Run:

```bash
go test -count=1 ./internal/opsmanual -run 'TestBuildOperationFrameUsesGenericMonitorRoleForRedisVariant'
```

Expected before implementation: FAIL because generic monitor role extraction is missing.

- [ ] **Step 2.3：实现通用资源角色抽取**

Create `internal/opsmanual/resource_role_extractor.go`:

```go
func applyResourceRoleContext(frame *OperationFrame, text string, metadata map[string]any) {
	if frame == nil {
		return
	}
	hostLabels := extractHostLabels(text)
	for _, host := range hostLabels {
		frame.Roles = appendUniqueRole(frame.Roles, OperationResourceRole{
			ID: host, Kind: ResourceRoleDataNode, ResourceRef: host, UserLabel: host, InferredFrom: "user_input",
		})
	}
	for _, monitor := range extractMonitorDeployments(text) {
		frame.Roles = appendUniqueRole(frame.Roles, OperationResourceRole{
			ID: monitor.Component + "-" + monitor.Host,
			Kind: ResourceRoleMonitor, ResourceRef: monitor.Host, UserLabel: monitor.Host,
			RuntimeName: monitor.Component, InferredFrom: "user_input",
		})
		frame.ObservationPoints = append(frame.ObservationPoints, OperationObservationPoint{
			Kind: ObservationPointMonitorComponent, ResourceRef: monitor.Host, Role: monitor.Component, Access: ObservationAccessUnknown,
		})
		frame.Relationships = append(frame.Relationships, OperationResourceRelationship{From: monitor.Host, To: firstNonEmpty(frame.Target.Name, frame.Target.Type, "target"), Type: RelationshipMonitors})
	}
	if dataLossAccepted(text) {
		frame.RiskPreference.DataLossAcceptable = true
		frame.RiskPreference.StillRequiresApproval = true
	}
}
```

Implementation constraints:

- `extractHostLabels` recognizes generic labels like `主机A`、`host-a`、`node-1` without assuming product type.
- `extractMonitorDeployments` recognizes generic “组件部署在主机/host” relationships and stores the component name as runtime context.
- Do not introduce `if pg_mon` or a PG-only branch. The PG sample should pass because `pg_mon部署在主机C` matches the generic monitor deployment pattern.

- [ ] **Step 2.4：接入 BuildOperationFrame**

Modify `internal/opsmanual/operation_frame.go` near the end of `BuildOperationFrameWithCapabilityRegistry`:

```go
applyResourceRoleContext(&frame, text, metadata)
frame.EvidenceRequirements = inferEvidenceRequirements(frame, registry)
```

Run:

```bash
go test -count=1 ./internal/opsmanual -run 'TestBuildOperationFrameAssignsDataNodesAndMonitorRole|TestBuildOperationFrameUsesGenericMonitorRoleForRedisVariant|TestOperationFrame'
```

Expected: PASS.

## 5. Task 3：通用能力优先 Gate 与 hardcode 扫描

**Files:**
- Modify: `internal/runtimekernel/genericity_guard.go`
- Modify: `internal/runtimekernel/genericity_guard_test.go`
- Create: `internal/opsmanual/genericity_guard_test.go`
- Test: `internal/runtimekernel/genericity_guard_test.go`

- [ ] **Step 3.1：扩展 core provider-specific 扫描测试**

Modify `internal/runtimekernel/genericity_guard_test.go` and add:

```go
func TestCoreProductionFilesAvoidScenarioSpecificTerms(t *testing.T) {
	terms := []string{"pg_mon", "主机a", "主机b", "主机c", "服务a", "服务b", "服务c"}
	paths := []string{
		"eino_kernel.go",
		"model_input.go",
		"tool_pack_intent.go",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lower := strings.ToLower(string(data))
		for _, term := range terms {
			if strings.Contains(lower, term) {
				t.Fatalf("%s contains scenario-specific term %q; use generic metadata/capability/resource signals", path, term)
			}
		}
	}
}
```

Run:

```bash
go test -count=1 ./internal/runtimekernel -run 'TestCoreProductionFilesAvoidScenarioSpecificTerms|TestCoreRuntimeProductionFilesAvoidProviderSpecificTerms'
```

Expected: PASS before and after implementation. If it fails, remove provider-specific text from core and move it to allowed boundaries.

- [ ] **Step 3.2：为 opsmanual core 增加边界测试**

Create `internal/opsmanual/genericity_guard_test.go`:

```go
func TestOpsManualCoreAvoidsMonitorProductHardcode(t *testing.T) {
	paths := []string{
		"operation_frame.go",
		"resource_role_extractor.go",
		"capability_registry.go",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "pg_mon") {
			t.Fatalf("%s contains pg_mon hardcode; monitor components must be runtime resource roles", path)
		}
	}
}
```

Run:

```bash
go test -count=1 ./internal/opsmanual -run 'TestOpsManualCoreAvoidsMonitorProductHardcode'
```

Expected: PASS.

## 6. Task 4：通用 Stateful Middleware Repair contract

**Files:**
- Create: `internal/opsrepair/types.go`
- Create: `internal/opsrepair/planner.go`
- Create: `internal/opsrepair/planner_test.go`
- Modify: `internal/opsmanual/types.go`

- [ ] **Step 4.1：写 RepairPlan contract 测试**

Create `internal/opsrepair/planner_test.go`:

```go
func TestPlanStatefulRepairRequiresReadonlyEvidenceBeforeMutatingSteps(t *testing.T) {
	frame := opsmanual.OperationFrame{
		Target: opsmanual.OperationTarget{Type: "postgresql", Name: "pg-cluster"},
		Roles: []opsmanual.OperationResourceRole{
			{ID: "host-a", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-a"},
			{ID: "host-b", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-b"},
		},
		RiskPreference: opsmanual.OperationRiskPreference{DataLossAcceptable: true, StillRequiresApproval: true},
		EvidenceRequirements: []string{"cluster_role", "member_health", "replication_status"},
	}
	plan, err := PlanStatefulRepair(context.Background(), PlanRequest{Frame: frame})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatalf("plan must require approval: %#v", plan)
	}
	if len(plan.Options) == 0 {
		t.Fatalf("expected repair options: %#v", plan)
	}
	for _, option := range plan.Options {
		if len(option.Steps) == 0 || option.Steps[0].Phase != PhasePreflight || !option.Steps[0].ReadOnly {
			t.Fatalf("first step must be readonly preflight: %#v", option.Steps)
		}
	}
}
```

Run:

```bash
go test -count=1 ./internal/opsrepair -run TestPlanStatefulRepairRequiresReadonlyEvidenceBeforeMutatingSteps
```

Expected before implementation: FAIL because package does not exist.

- [ ] **Step 4.2：实现最小 RepairPlan 类型**

Create `internal/opsrepair/types.go`:

```go
package opsrepair

const (
	PhasePreflight = "preflight"
	PhaseExecute   = "execute"
	PhaseVerify    = "verify"
	PhaseRollback  = "rollback"
)

type PlanRequest struct {
	Frame    opsmanual.OperationFrame `json:"frame"`
	Evidence []RepairEvidence         `json:"evidence,omitempty"`
}

type RepairPlan struct {
	ID               string             `json:"id,omitempty"`
	Capability      string             `json:"capability,omitempty"`
	DiagnosisSummary string            `json:"diagnosis_summary,omitempty"`
	Options          []RepairOption    `json:"options,omitempty"`
	RequiresApproval bool              `json:"requires_approval,omitempty"`
	Verification     RepairVerification `json:"verification,omitempty"`
}

type RepairOption struct {
	ID          string       `json:"id,omitempty"`
	Title       string       `json:"title,omitempty"`
	RiskLevel   string       `json:"risk_level,omitempty"`
	DataLoss    bool         `json:"data_loss,omitempty"`
	Steps       []RepairStep `json:"steps,omitempty"`
	WhenToUse   []string     `json:"when_to_use,omitempty"`
}

type RepairStep struct {
	ID         string         `json:"id,omitempty"`
	Phase      string         `json:"phase,omitempty"`
	ReadOnly   bool           `json:"read_only,omitempty"`
	ActionRef  string         `json:"action_ref,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}
```

Use imports:

```go
import "aiops-v2/internal/opsmanual"
```

- [ ] **Step 4.3：实现通用 repair planner 最小路径**

Create `internal/opsrepair/planner.go`:

```go
func PlanStatefulRepair(ctx context.Context, req PlanRequest) (*RepairPlan, error) {
	if strings.TrimSpace(req.Frame.Target.Type) == "" {
		return nil, errors.New("target type is required")
	}
	return &RepairPlan{
		ID: "repair-" + stableFrameID(req.Frame),
		Capability: "stateful_middleware_cluster_repair",
		DiagnosisSummary: "需要先收集只读证据，再选择受治理修复方案。",
		RequiresApproval: true,
		Options: []RepairOption{{
			ID: "rebuild-from-healthy-member",
			Title: "基于健康成员重建异常成员",
			RiskLevel: "high",
			DataLoss: req.Frame.RiskPreference.DataLossAcceptable,
			Steps: []RepairStep{
				{ID: "collect-readonly-evidence", Phase: PhasePreflight, ReadOnly: true, ActionRef: "probe.collect_stateful_cluster_evidence"},
				{ID: "execute-selected-repair", Phase: PhaseExecute, ReadOnly: false, ActionRef: "runner.stateful_cluster_repair"},
				{ID: "verify-cluster-health", Phase: PhaseVerify, ReadOnly: true, ActionRef: "probe.verify_stateful_cluster_health"},
			},
		}},
		Verification: RepairVerification{RequiredEvidence: req.Frame.EvidenceRequirements},
	}, nil
}
```

Implementation constraints:

- `ActionRef` uses generic names.
- No PG command appears in `internal/opsrepair`.
- Provider-specific probe mapping is deferred to capability pack.

- [ ] **Step 4.4：跑 repair contract 测试**

Run:

```bash
go test -count=1 ./internal/opsrepair
```

Expected: PASS.

## 7. Task 5：PostgreSQL 作为首个 capability pack，不进入 core

**Files:**
- Create: `internal/opsmanual/postgres_capability_pack.go`
- Create: `internal/opsmanual/postgres_capability_pack_test.go`
- Modify: `internal/opsmanual/capability_registry.go`
- Test: `internal/opsmanual/capability_registry_test.go`

- [ ] **Step 5.1：写 PG capability pack 边界测试**

Create `internal/opsmanual/postgres_capability_pack_test.go`:

```go
func TestPostgresCapabilityPackContributesProbeMappingsOnlyThroughRegistry(t *testing.T) {
	registry := NewCapabilityRegistry(PostgresCapabilityPack())
	if got := registry.DetectObjectType("PG 主从集群异常"); got != "postgresql" {
		t.Fatalf("DetectObjectType = %q, want postgresql", got)
	}
	probes := registry.PreflightProbesFor("postgresql", "repair")
	if len(probes) == 0 {
		t.Fatalf("expected postgres probes from capability pack")
	}
	for _, probe := range probes {
		if !probe.ReadOnly {
			t.Fatalf("postgres evidence probes must be readonly: %#v", probe)
		}
	}
}
```

Run:

```bash
go test -count=1 ./internal/opsmanual -run TestPostgresCapabilityPackContributesProbeMappingsOnlyThroughRegistry
```

Expected before implementation: FAIL because `PostgresCapabilityPack` or `PreflightProbesFor` is missing.

- [ ] **Step 5.2：实现 PG capability pack**

Create `internal/opsmanual/postgres_capability_pack.go`:

```go
func PostgresCapabilityPack() OpsManualCapabilityPack {
	return OpsManualCapabilityPack{
		ID: "builtin.postgresql",
		BuiltIn: true,
		Enabled: true,
		Priority: 50,
		ObjectAliases: []CapabilityAlias{{Value: "postgresql", Needles: []string{"postgresql", "postgres", "pg", "PG"}}},
		StatefulTargetTypes: []string{"postgresql"},
		ParameterHints: []CapabilityParameterHint{
			{ID: "target_instance", TargetType: "postgresql", Action: "repair", Required: true, Source: "operation_frame"},
			{ID: "execution_surface", TargetType: "postgresql", Action: "repair", Required: true, Source: "operation_frame"},
		},
		PreflightProbes: []CapabilityPreflightProbe{
			{ID: "postgres_member_health", TargetType: "postgresql", Action: "repair", RiskLevel: "low", ReadOnly: true},
			{ID: "postgres_replication_status", TargetType: "postgresql", Action: "repair", RiskLevel: "low", ReadOnly: true},
			{ID: "postgres_storage_health", TargetType: "postgresql", Action: "repair", RiskLevel: "low", ReadOnly: true},
		},
	}
}
```

Modify `DefaultOpsManualCapabilityRegistry` to register this pack through the same pack list path used by existing built-ins, not through a special runtime branch.

- [ ] **Step 5.3：验证 pg_mon 没有变成内置集成**

Run:

```bash
rg -n "pg_mon" internal/opsmanual internal/runtimekernel internal/workflowgen internal/opsrepair
```

Expected:

- `pg_mon` 只允许出现在 `_test.go` 文件或 eval fixture。
- Production code should not contain `pg_mon`.

## 8. Task 6：资源型 Workflow Generation Flow

**Files:**
- Modify: `internal/workflowgen/types.go`
- Create: `internal/workflowgen/resource_plan_builder.go`
- Create: `internal/workflowgen/resource_plan_builder_test.go`
- Modify: `internal/workflowgen/graph_generator.go`
- Modify: `internal/workflowgen/builder_agent.go`
- Test: `internal/workflowgen/graph_generator_test.go`

- [ ] **Step 6.1：写资源型 plan builder 测试**

Create `internal/workflowgen/resource_plan_builder_test.go`:

```go
func TestResourcePlanBuilderCreatesPreflightExecuteVerifyRollbackStages(t *testing.T) {
	frame := opsmanual.BuildOperationFrame("帮我写一个workflow,让主机A和主机B的PG两个节点可以通过主机C的pg_mon形成PG集群", nil)
	builder := ResourcePlanBuilder{}
	plan, err := builder.BuildResourcePlan(context.Background(), BuildResourcePlanRequest{Requirement: frame.RawText, OperationFrame: frame})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReviewStatus != ReviewStatusPendingReview {
		t.Fatalf("ReviewStatus = %q, want pending_review", plan.ReviewStatus)
	}
	if !hasStage(plan, "preflight") || !hasStage(plan, "execute") || !hasStage(plan, "verify") || !hasStage(plan, "rollback") {
		t.Fatalf("missing required stages: %#v", plan.Nodes)
	}
	if !usesSecretRefs(plan) {
		t.Fatalf("resource workflow must use secret refs for credentials: %#v", plan.Inputs)
	}
}
```

Run:

```bash
go test -count=1 ./internal/workflowgen -run TestResourcePlanBuilderCreatesPreflightExecuteVerifyRollbackStages
```

Expected before implementation: FAIL because `ResourcePlanBuilder` and review status do not exist.

- [ ] **Step 6.2：扩展 workflowgen 类型**

Modify `internal/workflowgen/types.go`:

```go
type ReviewStatus string

const (
	ReviewStatusDraft         ReviewStatus = "draft"
	ReviewStatusPendingReview ReviewStatus = "pending_review"
)

type WorkflowGenerationPlan struct {
	// existing fields...
	ReviewStatus ReviewStatus `json:"review_status,omitempty"`
	ResourceKind string       `json:"resource_kind,omitempty"`
	OperationFrame map[string]any `json:"operation_frame,omitempty"`
}
```

Add `NodeKindPreflight`, `NodeKindExecute`, `NodeKindVerify`, `NodeKindRollback` or store stage in `WorkflowPlanNode.Config["stage"]` if changing enum causes too much churn. Prefer `Config["stage"]` for the first iteration to minimize blast radius.

- [ ] **Step 6.3：实现 ResourcePlanBuilder**

Create `internal/workflowgen/resource_plan_builder.go`:

```go
type BuildResourcePlanRequest struct {
	Requirement string
	OperationFrame opsmanual.OperationFrame
	Slots map[string]string
}

type ResourcePlanBuilder struct{}

func (b ResourcePlanBuilder) BuildResourcePlan(ctx context.Context, req BuildResourcePlanRequest) (*WorkflowGenerationPlan, error) {
	if strings.TrimSpace(req.Requirement) == "" {
		return nil, errors.New("requirement is required")
	}
	frame := req.OperationFrame
	return &WorkflowGenerationPlan{
		Version: 1,
		Title: "资源型运维 Workflow 草稿",
		Intent: "generate_resource_ops_workflow",
		ReviewStatus: ReviewStatusPendingReview,
		ResourceKind: frame.Target.Type,
		Trigger: WorkflowTrigger{Type: TriggerTypeManual, Summary: "手动触发"},
		Inputs: []WorkflowIO{
			{ID: "target_resources", Type: "array", Description: "目标资源引用", Required: true},
			{ID: "credential_ref", Type: "secret_ref", Description: "访问凭据 SecretRef", Required: true},
		},
		Nodes: []WorkflowPlanNode{
			resourceStageNode("preflight", "只读预检", true),
			resourceStageNode("execute", "受治理执行", false),
			resourceStageNode("verify", "独立验证", true),
			resourceStageNode("rollback", "回滚或人工接管", false),
		},
		ValidationStrategy: ValidationStrategy{Enabled: true, Provider: ValidationProviderDocker, Scenario: "resource-ops-draft-static-validation", Network: "mock"},
		Risks: []string{"高风险节点必须审批后执行", "凭据只能使用 secret_ref"},
	}
}
```

Constraints:

- Generated plan is draft/pending_review.
- `pg_mon` only appears as a runtime observation point copied from Operation Frame metadata if needed; no fixed pg_mon template.
- Verify stage must not reuse execute output as its only evidence.

- [ ] **Step 6.4：映射资源型 plan 到 Runner graph**

Modify `internal/workflowgen/graph_generator.go`:

- For `Config["stage"] == "preflight"` set step vars `read_only=true`.
- For high-risk `execute` stage set UI metadata `requires_approval=true`.
- Add workflow vars `review_status`, `resource_kind`, `workflow_generation_session_id`.

Run:

```bash
go test -count=1 ./internal/workflowgen -run 'TestResourcePlanBuilder|TestRunnerGraphGenerator'
```

Expected: PASS.

## 9. Task 7：通用 Observability Evidence contract 与 Coroot adapter

**Files:**
- Create: `internal/observability/evidence_contract.go`
- Create: `internal/observability/evidence_contract_test.go`
- Create: `internal/integrations/coroot/evidence_mapper.go`
- Create: `internal/integrations/coroot/evidence_mapper_test.go`
- Modify: `internal/integrations/coroot/tools.go`

- [ ] **Step 7.1：写通用 evidence contract 测试**

Create `internal/observability/evidence_contract_test.go`:

```go
func TestEvidencePackRoundTripPreservesDependencyAndMissingEvidence(t *testing.T) {
	pack := EvidencePack{
		Provider: "synthetic",
		Target: EntityRef{Kind: "service", Name: "service-a"},
		DependencyEdges: []DependencyEdge{{From: "service-a", To: "service-b"}, {From: "service-b", To: "service-c"}},
		Hypotheses: []Hypothesis{{Entity: "service-c", Summary: "dependency saturation", Confidence: "medium"}},
		MissingEvidence: []string{"logs for service-c"},
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EvidencePack
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.DependencyEdges[1].To != "service-c" || decoded.Hypotheses[0].Entity != "service-c" {
		t.Fatalf("decoded evidence pack = %#v", decoded)
	}
}
```

Run:

```bash
go test -count=1 ./internal/observability -run TestEvidencePackRoundTripPreservesDependencyAndMissingEvidence
```

Expected before implementation: FAIL because `EvidencePack` does not exist.

- [ ] **Step 7.2：实现 EvidencePack 类型**

Create `internal/observability/evidence_contract.go`:

```go
type EvidencePack struct {
	Provider        string           `json:"provider,omitempty"`
	Project         string           `json:"project,omitempty"`
	Target          EntityRef        `json:"target,omitempty"`
	TargetStatus    []StatusEvidence `json:"target_status,omitempty"`
	DependencyEdges []DependencyEdge `json:"dependency_edges,omitempty"`
	Incidents       []IncidentEvidence `json:"incidents,omitempty"`
	Metrics         []MetricEvidence  `json:"metrics,omitempty"`
	Logs            []LogEvidence     `json:"logs,omitempty"`
	Traces          []TraceEvidence   `json:"traces,omitempty"`
	Deployments     []DeploymentEvidence `json:"deployments,omitempty"`
	Hypotheses      []Hypothesis      `json:"hypotheses,omitempty"`
	MissingEvidence []string          `json:"missing_evidence,omitempty"`
}
```

Use small structs with `Name`, `Summary`, `Severity`, `Confidence`, `RawRef` fields. Keep this package provider-neutral.

- [ ] **Step 7.3：写 Coroot A->B->C mapper 测试**

Create `internal/integrations/coroot/evidence_mapper_test.go`:

```go
func TestMapCorootRCAContextToEvidencePackPreservesDependencyRootCause(t *testing.T) {
	payload := map[string]any{
		"service": "service-a",
		"dependencies": []any{
			map[string]any{"from": "service-a", "to": "service-b", "status": "degraded"},
			map[string]any{"from": "service-b", "to": "service-c", "status": "degraded"},
		},
		"hypotheses": []any{
			map[string]any{"entity": "service-c", "summary": "CPU saturation", "confidence": "high"},
		},
	}
	pack := MapCorootEvidencePack("env-a", payload)
	if pack.Provider != "coroot" || pack.Target.Name != "service-a" {
		t.Fatalf("pack target = %#v", pack)
	}
	if len(pack.DependencyEdges) != 2 || pack.DependencyEdges[1].To != "service-c" {
		t.Fatalf("edges = %#v", pack.DependencyEdges)
	}
	if pack.Hypotheses[0].Entity != "service-c" {
		t.Fatalf("hypotheses = %#v", pack.Hypotheses)
	}
}
```

Run:

```bash
go test -count=1 ./internal/integrations/coroot -run TestMapCorootRCAContextToEvidencePackPreservesDependencyRootCause
```

Expected before implementation: FAIL because mapper does not exist.

- [ ] **Step 7.4：实现 Coroot evidence mapper**

Create `internal/integrations/coroot/evidence_mapper.go`.

Constraints:

- Mapper accepts generic `map[string]any` or typed Coroot result and emits `observability.EvidencePack`.
- Tool execution still returns existing Coroot display payload for UI, but model-facing payload includes a compact provider-neutral `observability_evidence` section.
- If Coroot is unavailable, populate `MissingEvidence` and do not claim target service is absent.

Run:

```bash
go test -count=1 ./internal/observability ./internal/integrations/coroot
```

Expected: PASS.

## 10. Task 8：RuntimeKernel Chat route 接入通用 contract

**Files:**
- Modify: `internal/runtimekernel/eino_kernel.go`
- Modify: `internal/runtimekernel/model_input.go`
- Create: `internal/runtimekernel/general_ops_contract_test.go`
- Modify: `internal/runtimekernel/model_input_test.go`
- Modify: `internal/runtimekernel/intent_tool_packs_test.go`

- [ ] **Step 8.1：写通用运维请求不直接 final 的测试**

Create `internal/runtimekernel/general_ops_contract_test.go`:

```go
func TestGeneralOpsRepairRequestRequiresEvidenceBeforeFinal(t *testing.T) {
	h := newReactLoopHarness(t)
	h.AddUserMessage("主机A和主机B的PG主从集群异常,请帮忙恢复,数据可以不要,只需要PG主从集群可以正常运行,他们的pg_mon部署在主机C.")
	result := h.Run()
	if result.FinalWithoutTool {
		t.Fatalf("repair request finalized without evidence/tool path: %#v", result)
	}
	if !result.ContainsOperationFrameRole("monitor") {
		t.Fatalf("operation frame should include monitor role: %#v", result)
	}
	if result.ContainsCoreBranchTerm("pg_mon") {
		t.Fatalf("pg_mon must be runtime context, not core branch: %#v", result)
	}
}
```

Before adding the test, inspect `internal/runtimekernel/react_loop_test.go` and reuse its existing harness helpers. If no helper exposes `FinalWithoutTool` or Operation Frame role checks, add small test-local helpers in `general_ops_contract_test.go` that derive those values from `RunTurn` events, tool calls and turn items.

Run:

```bash
go test -count=1 ./internal/runtimekernel -run TestGeneralOpsRepairRequestRequiresEvidenceBeforeFinal
```

Expected before implementation: FAIL because runtime does not expose this contract yet.

- [ ] **Step 8.2：接入 Operation Frame v2 摘要到 model input**

Modify `internal/runtimekernel/model_input.go` so model context includes:

```text
Operation Frame:
- target kind/type/name
- resource roles
- relationships
- execution surface
- observation points
- risk preference
- evidence requirements
```

Constraints:

- Do not write product-specific instructions in model input builder.
- Evidence limits and missing evidence must be explicit.
- `data_loss_acceptable=true` must be accompanied by `still_requires_approval=true`.

- [ ] **Step 8.3：让 Chat route 选择通用 repair/workflow/observability contract**

Modify `internal/runtimekernel/eino_kernel.go`:

- If user asks to recover/repair a stateful cluster, create Operation Frame v2 and require readonly evidence collection before final answer.
- If user asks to “写 workflow”, route to workflow generation path even without explicit `@add_workflow`.
- If user mentions observability provider such as `@coroot`, use tool discovery/provider tool as evidence source, then synthesize via generic evidence contract.

Constraints:

- Routing predicates must be generic: operation type, target kind, requested artifact, evidence provider metadata.
- Do not add `if strings.Contains(text, "pg_mon")`.

- [ ] **Step 8.4：跑 RuntimeKernel 定向测试**

Run:

```bash
go test -count=1 ./internal/runtimekernel -run 'TestGeneralOps|TestCoreProductionFilesAvoid|TestRunTurn_.*Coroot|Test.*Workflow'
```

Expected: PASS.

## 11. Task 9：Eval 字段、scorer 与新增同类变体

**Files:**
- Modify: `internal/eval/types.go`
- Modify: `internal/eval/scorer.go`
- Modify: `internal/eval/mock_agent.go`
- Modify: `testdata/eval_cases/pg-cluster-recovery-chat.json`
- Modify: `testdata/eval_cases/pg-cluster-workflow-generation.json`
- Modify: `testdata/eval_cases/coroot-service-chain-rca.json`
- Create: `testdata/eval_cases/redis-stateful-repair-chat.json`
- Create: `testdata/eval_cases/generic-observability-rca.json`
- Test: `internal/eval/scorer_runner_test.go`

- [ ] **Step 9.1：写 eval scorer 新字段测试**

Add to `internal/eval/scorer_runner_test.go`:

```go
func TestScorerChecksGeneralOpsContractSignals(t *testing.T) {
	c := Case{
		ID: "general-ops-contract",
		Expected: Expected{
			ExpectedResourceRoles: []string{"data_node:host-a", "monitor:host-c"},
			ExpectedCapabilityPath: []string{"stateful_middleware_cluster_repair"},
			ExpectedWorkflowReviewStatus: []string{"pending_review"},
			ExpectedObservabilityEvidence: []string{"dependency_edges", "hypotheses", "missing_evidence"},
		},
	}
	out := RunOutput{Answer: "capability=stateful_middleware_cluster_repair review_status=pending_review dependency_edges hypotheses missing_evidence data_node:host-a monitor:host-c"}
	score := ScoreCase(c, out)
	if !score.Passed {
		t.Fatalf("score should pass: %#v", score.Checks)
	}
}
```

If `Expected` names differ during implementation, keep JSON names stable:

```json
"expectedResourceRoles": [],
"expectedCapabilityPath": [],
"expectedWorkflowReviewStatus": [],
"expectedObservabilityEvidence": []
```

Run:

```bash
go test -count=1 ./internal/eval -run TestScorerChecksGeneralOpsContractSignals
```

Expected before implementation: FAIL because fields do not exist.

- [ ] **Step 9.2：扩展 eval types and scorer**

Modify `internal/eval/types.go`:

```go
ExpectedResourceRoles           []string `json:"expectedResourceRoles,omitempty"`
ExpectedCapabilityPath          []string `json:"expectedCapabilityPath,omitempty"`
ExpectedWorkflowReviewStatus    []string `json:"expectedWorkflowReviewStatus,omitempty"`
ExpectedObservabilityEvidence   []string `json:"expectedObservabilityEvidence,omitempty"`
ExpectedGenericOpsContract      []string `json:"expectedGenericOpsContract,omitempty"`
```

Modify `internal/eval/scorer.go` to score these fields from answer text, tool calls, events and turn items. Do not make answer text the only source if structured events exist.

- [ ] **Step 9.3：新增非 PG 和非 Coroot 变体**

Create `testdata/eval_cases/redis-stateful-repair-chat.json`:

```json
{
  "id": "redis-stateful-repair-chat",
  "category": "general-ops",
  "priority": "P1",
  "input": "主机A和主机B的Redis主从集群异常，请帮忙恢复，只需要Redis集群正常运行，sentinel部署在主机C。",
  "expected": {
    "mustInclude": ["通用", "只读证据", "方案", "验证"],
    "mustNotInclude": ["PostgreSQL专用", "PG专用", "pg_mon"],
    "expectedResourceRoles": ["data_node", "monitor"],
    "expectedCapabilityPath": ["stateful_middleware_cluster_repair"],
    "mustMentionEvidenceLimits": true
  }
}
```

Create `testdata/eval_cases/generic-observability-rca.json`:

```json
{
  "id": "generic-observability-rca",
  "category": "general-ops",
  "priority": "P1",
  "input": "@observability 分析环境A的A服务为什么异常，调用链是A服务->B服务->C服务。",
  "expected": {
    "mustInclude": ["依赖链", "证据", "置信度", "缺失证据"],
    "mustNotInclude": ["只能用Coroot", "Coroot专用"],
    "expectedObservabilityEvidence": ["dependency_edges", "hypotheses"],
    "mustMentionEvidenceLimits": true
  }
}
```

Run:

```bash
go test -count=1 ./internal/eval ./cmd/agent-eval ./cmd/agent-eval-case
go run ./cmd/agent-eval -agent mock -cases testdata/eval_cases -priority P1 -out .data/eval_runs/general-ops-capability-scorer-mock
```

Expected:

- Go tests PASS.
- mock runner completes; target cases may fail until runtime implementation catches up.

## 12. Task 10：多主机输入框主机列表与一主机一 Agent 编排

**Files:**
- Modify: `internal/hostops/orchestrator.go`
- Modify: `internal/hostops/orchestrator_test.go`
- Modify: `internal/appui/transport_state.go`
- Modify: `internal/appui/transport_projector.go`
- Modify: `web/src/chat/components/AiopsComposer.tsx`
- Modify: `web/src/chat/components/HostMentionComposer.tsx`
- Modify: `web/src/chat/components/HostOpsStatusPanel.tsx`
- Modify: `web/src/chat/components/HostSubagentStatusRow.tsx`
- Modify: `web/src/chat/components/HostSubagentDrawer.tsx`
- Modify: `web/src/chat/components/HostSubagentTabs.tsx`
- Modify: `web/src/transport/aiopsTransportTypes.ts`
- Modify: `web/src/transport/aiopsTransportConverter.ts`
- Test: `web/src/chat/components/AiopsComposer.test.tsx`
- Test: `web/src/chat/components/HostMentionComposer.test.tsx`
- Test: `web/src/chat/components/HostOpsStatusPanel.test.tsx`
- Test: `web/src/chat/components/HostSubagentDrawer.test.tsx`
- Test: `web/src/transport/aiopsTransportConverter.test.ts`

- [ ] **Step 10.1：写一主机一 child agent 后端测试**

Add to `internal/hostops/orchestrator_test.go`:

```go
func TestOrchestratorSpawnsOneChildAgentPerMissionHost(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID: "mission-multi-host",
		ThreadID: "thread-1",
		UserTurnID: "turn-1",
		Status: HostMissionStatusSpawningChildren,
		PlanRequired: true,
		PlanAccepted: true,
		Mentions: []HostMention{
			{HostID: "host-a", DisplayName: "主机A", Resolved: true},
			{HostID: "host-b", DisplayName: "主机B", Resolved: true},
		},
		Plan: HostOperationPlan{
			ID: "plan-1",
			Status: PlanStatusAccepted,
			Steps: []PlanStep{{ID: "step-1", HostIDs: []string{"host-a", "host-b"}, ActionType: "read", RiskLevel: "low"}},
		},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatal(err)
	}
	children, err := orchestrator.SpawnChildren(ctx, mission.ID, []ChildAgentAssignment{
		{HostID: "host-a", HostDisplayName: "主机A", Task: "检查主机A", PlanStepID: "step-1"},
		{HostID: "host-b", HostDisplayName: "主机B", Task: "检查主机B", PlanStepID: "step-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("children = %d, want 2: %#v", len(children), children)
	}
	if children[0].HostID == children[1].HostID || children[0].ID == children[1].ID {
		t.Fatalf("child agents must be unique per host: %#v", children)
	}
}
```

Run:

```bash
go test -count=1 ./internal/hostops -run TestOrchestratorSpawnsOneChildAgentPerMissionHost
```

Expected:

- PASS if existing hostops already satisfies the contract.
- If FAIL, fix `SpawnChildren` so assignments are keyed by normalized host id and each unique mission host gets exactly one child agent.

- [ ] **Step 10.2：写重复主机不重复创建 child agent 测试**

Add to `internal/hostops/orchestrator_test.go`:

```go
func TestOrchestratorDeduplicatesRepeatedHostAssignments(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID: "mission-dedupe",
		ThreadID: "thread-1",
		UserTurnID: "turn-1",
		Status: HostMissionStatusSpawningChildren,
		PlanRequired: true,
		PlanAccepted: true,
		Mentions: []HostMention{{HostID: "host-a", DisplayName: "主机A", Resolved: true}},
		Plan: HostOperationPlan{
			ID: "plan-1",
			Status: PlanStatusAccepted,
			Steps: []PlanStep{{ID: "step-1", HostIDs: []string{"host-a"}, ActionType: "read", RiskLevel: "low"}},
		},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatal(err)
	}
	children, err := orchestrator.SpawnChildren(ctx, mission.ID, []ChildAgentAssignment{
		{HostID: "host-a", Task: "检查主机A", PlanStepID: "step-1"},
		{HostID: "host-a", Task: "再次检查主机A", PlanStepID: "step-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1 after dedupe: %#v", len(children), children)
	}
}
```

Run:

```bash
go test -count=1 ./internal/hostops -run TestOrchestratorDeduplicatesRepeatedHostAssignments
```

Expected: PASS after dedupe behavior is guaranteed.

- [ ] **Step 10.3：写输入框上方主机列表 UI 测试**

Add to `web/src/chat/components/HostMentionComposer.test.tsx`:

```tsx
it("renders selected host list above the chat input and supports removal", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <HostMentionComposer
      value="@host-a @host-b 检查两台主机"
      mentions={[
        { raw: "@host-a", hostId: "host-a", displayName: "主机A", resolved: true },
        { raw: "@host-b", hostId: "host-b", displayName: "主机B", resolved: true },
      ]}
      onChange={onChange}
    />,
  );
  const list = screen.getByTestId("composer-host-list");
  expect(within(list).getByText("主机A")).toBeInTheDocument();
  expect(within(list).getByText("主机B")).toBeInTheDocument();
  await user.click(within(list).getByRole("button", { name: /移除 主机A/ }));
  expect(onChange).toHaveBeenCalledWith(expect.not.stringContaining("@host-a"));
});
```

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/components/HostMentionComposer.test.tsx
```

Expected before implementation: FAIL if composer does not expose `composer-host-list` or remove controls.

- [ ] **Step 10.4：实现输入框上方主机列表**

Modify `web/src/chat/components/HostMentionComposer.tsx`:

- Render selected/resolved mentions above the `Textarea`.
- Use `HostMentionChip` for each host.
- Add `data-testid="composer-host-list"`.
- Remove button updates the composer text through `onChange`.
- Deduplicate by `hostId || raw`.
- Keep list visible whenever the pending input contains one or more resolved host mentions.

Required behavior:

```tsx
<div data-testid="composer-host-list" aria-label="已选择主机">
  {uniqueMentions.map((mention) => (
    <HostMentionChip key={mention.hostId || mention.raw} mention={mention} onRemove={() => removeMention(mention)} />
  ))}
</div>
```

Do not put this list only in the drawer or inside the assistant message body. It must sit above the text input/composer area.

- [ ] **Step 10.5：写状态面板一主机一 agent UI 测试**

Add to `web/src/chat/components/HostOpsStatusPanel.test.tsx`:

```tsx
it("renders one host-bound child agent status item per mission host", () => {
  render(
    <HostOpsStatusPanel
      state={{
        hostMissions: {
          "mission-1": {
            id: "mission-1",
            status: "running",
            mentions: [
              { hostId: "host-a", displayName: "主机A", resolved: true },
              { hostId: "host-b", displayName: "主机B", resolved: true },
            ],
            childAgentIds: ["child-host-a", "child-host-b"],
          },
        },
        activeHostMissionId: "mission-1",
        childAgents: {
          "child-host-a": { id: "child-host-a", hostId: "host-a", hostDisplayName: "主机A", status: "running" },
          "child-host-b": { id: "child-host-b", hostId: "host-b", hostDisplayName: "主机B", status: "waiting" },
        },
      } as AiopsTransportState}
    />,
  );
  expect(screen.getByText("主机A")).toBeInTheDocument();
  expect(screen.getByText("主机B")).toBeInTheDocument();
  expect(screen.getByTestId("host-child-agent-child-host-a")).toHaveTextContent("running");
  expect(screen.getByTestId("host-child-agent-child-host-b")).toHaveTextContent("waiting");
});
```

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/components/HostOpsStatusPanel.test.tsx
```

Expected before implementation: FAIL if child agent rows do not expose one item per host/child pair.

- [ ] **Step 10.6：实现 HostOpsStatusPanel / HostSubagentStatusRow 绑定展示**

Modify `web/src/chat/components/HostSubagentStatusRow.tsx`:

- For each `mission.childAgentIds`, look up `state.childAgents[childAgentId]`.
- Render host display name, child agent status and open-drawer control.
- Add `data-testid={"host-child-agent-" + childAgent.id}`.
- If a mission host mention has no child agent yet, render pending host item rather than hiding the host.
- Do not merge multiple child agents into one summary-only row.

Modify `web/src/chat/components/HostOpsStatusPanel.tsx`:

- Keep the panel directly above the composer.
- Show host list even before plan steps exist if the active mission has mentions.
- Keep compact layout; no nested card inside card.

- [ ] **Step 10.7：写 drawer host isolation 测试**

Add to `web/src/chat/components/HostSubagentDrawer.test.tsx`:

```tsx
it("opens transcript for the selected host-bound child agent only", () => {
  render(
    <HostSubagentDrawer
      open
      childAgentId="child-host-b"
      state={{
        childAgents: {
          "child-host-a": { id: "child-host-a", hostId: "host-a", hostDisplayName: "主机A", status: "running" },
          "child-host-b": { id: "child-host-b", hostId: "host-b", hostDisplayName: "主机B", status: "running" },
        },
        childAgentTranscripts: {
          "child-host-a": [{ id: "a-1", content: "host-a transcript" }],
          "child-host-b": [{ id: "b-1", content: "host-b transcript" }],
        },
      } as AiopsTransportState}
    />,
  );
  expect(screen.getByText("主机B")).toBeInTheDocument();
  expect(screen.getByText("host-b transcript")).toBeInTheDocument();
  expect(screen.queryByText("host-a transcript")).not.toBeInTheDocument();
});
```

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/components/HostSubagentDrawer.test.tsx
```

Expected: PASS after drawer uses selected child id and does not mix transcripts.

- [ ] **Step 10.8：跑多主机交互相关测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops ./internal/appui ./internal/server
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/components/AiopsComposer.test.tsx src/chat/components/HostMentionComposer.test.tsx src/chat/components/HostOpsStatusPanel.test.tsx src/chat/components/HostSubagentDrawer.test.tsx src/transport/aiopsTransportConverter.test.ts
```

Expected:

- Backend confirms one host one child agent and cross-host isolation.
- Frontend confirms composer host list, status panel, drawer and transport metadata.

- [ ] **Step 10.9：做浏览器交互验证**

Run local web app, then use browser/Playwright to verify:

1. In the chat composer, type or select two hosts.
2. The selected host list appears directly above the input box.
3. Removing one host updates both chip list and text input.
4. Starting a multi-host ops mission shows one status item per host.
5. Opening a host item opens only that host's child-agent drawer/transcript.

Expected:

- Desktop and mobile viewport text does not overlap.
- Host list remains usable with 2-5 hosts.
- Every visible child agent status maps to exactly one host id.

## 13. Task 11：端到端验收、扫描与文档回写

**Files:**
- Modify: `docs/2026-06-17-aiops-v2-general-ops-capability-modification-design.zh.md`
- Modify: `docs/2026-06-17-aiops-v2-pg-coroot-workflow-acceptance.zh.md`
- Read: `.data/eval_runs/general-ops-capability-server/report.json`

- [ ] **Step 11.1：跑全量相关 Go 测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/opsmanual ./internal/opsrepair ./internal/workflowgen ./internal/observability ./internal/integrations/coroot ./internal/runtimekernel ./internal/eval ./cmd/agent-eval ./cmd/agent-eval-case
```

Expected: PASS.

- [ ] **Step 11.2：跑 genericity 扫描**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
rg -n "if .*postgres|if .*pg_mon|if .*coroot|strings\\.Contains\\(.*postgres|strings\\.Contains\\(.*pg_mon|strings\\.Contains\\(.*coroot" internal web/src
```

Expected:

- 命中只允许出现在 capability pack、provider adapter、fixture、eval case、test 或文档。
- 如果命中 core route / prompt / policy / workflowgen 主路径，必须重构到通用 contract 或能力包边界。

- [ ] **Step 11.3：跑 mock eval 回归**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go run ./cmd/agent-eval -agent mock -cases testdata/eval_cases -priority P1 -out .data/eval_runs/general-ops-capability-final-mock
```

Expected:

- runner 完整执行。
- 新增 scorer 字段能被 report 正常展示。

- [ ] **Step 11.4：跑真实 server eval 回归**

Prerequisite:

- aiops-v2 server 已在 `http://127.0.0.1:8080` 运行。
- Coroot 或 synthetic observability fixture 可用。
- host-agent / runner 测试环境使用受控 lab 或 mock surface，不连接生产环境。

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go run ./cmd/agent-eval -agent server -server-url http://127.0.0.1:8080 -cases testdata/eval_cases -priority P1 -repetitions 3 -out .data/eval_runs/general-ops-capability-server
```

Expected:

- `pg-cluster-recovery-chat` PASS。
- `pg-cluster-workflow-generation` PASS。
- `coroot-service-chain-rca` PASS。
- `redis-stateful-repair-chat` PASS 或达到与 PG 样例同一 capability path 的结构化信号。
- `generic-observability-rca` PASS 或达到与 Coroot 样例同一 evidence contract 的结构化信号。

- [ ] **Step 11.5：回写实施结果**

Update `docs/2026-06-17-aiops-v2-general-ops-capability-modification-design.zh.md` with:

- Implemented modules.
- Verification commands and report paths.
- Known limitations.
- Follow-up tasks if any.

Run:

```bash
git diff --check
```

Expected: no whitespace errors.

## 14. 完成定义

- [ ] Operation Frame v2 能表达 data node、monitor、observer、execution surface、observation point、risk preference 和 evidence requirements。
- [ ] PG 样例中 A/B 被识别为数据节点，C 上的 `pg_mon` 被识别为 monitor/observation point，不需要默认内置集成。
- [ ] Stateful repair flow 先收集只读证据，再产出方案，再受治理执行，再独立验证。
- [ ] Workflow generation 能生成 preflight / execute / verify / rollback 资源型 draft workflow，并保持 pending_review。
- [ ] Coroot 输出被映射为通用 observability evidence，Chat final answer 不依赖 Coroot 专有字段名才能成立。
- [ ] 多主机运维时，对话输入框上方展示主机列表；主机列表支持选择、去重、删除和状态展示。
- [ ] 多主机运维时，一个 host mission 中每台主机对应一个 host-bound child agent；child agent drawer/transcript 不跨主机混合。
- [ ] eval 能验证通用能力路径，并包含至少一个非 PG repair 变体和一个非 Coroot observability 变体。
- [ ] genericity 扫描确认 provider-specific 名称没有泄漏到 core 决策路径。
- [ ] `go test`、mock eval、server eval 和 `git diff --check` 都有记录。
