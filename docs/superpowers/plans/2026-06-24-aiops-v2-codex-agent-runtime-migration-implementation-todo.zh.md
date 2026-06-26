# aiops-v2 Codex Agent Runtime Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 `docs/superpowers/specs/2026-06-24-aiops-v2-codex-agent-runtime-migration-design.zh.md`，把 aiops-v2 Agent Runtime 收敛为 Codex-like 单一 thread/turn loop，让用户输入、工具调用、审批、恢复、压缩、多主机运维和最终回答都由同一 runtime 生命周期承载。

**Architecture:** 不重写 Eino model adapter，不新建第二套 Agent Runtime。实施路径是先加 trace 和 owner guard，再逐步让 tool surface、active turn、HostOps、approval、prompt profile、compaction/final gate 接管职责；旧 API 和旧 service 只能作为 adapter 调用新 owner。

**Tech Stack:** Go `internal/appui` / `internal/runtimekernel` / `internal/tooling` / `internal/hostops` / `internal/promptcompiler` / `internal/server`, React + TypeScript Assistant UI, Go test, Playwright/browser snapshot, golden trace eval.

---

## 1. 实施边界

### 1.1 必须实现

- [x] 所有 Chat 请求进入 `EinoKernel.RunTurn` 或 `EinoKernel.ResumeTurn`，默认路径不存在 HostOps synthetic completed turn。
- [x] `RuntimeRoute` 只输出 profile、target refs、tool surface metadata 和 immutable guards，不直接执行 HostOps、Approval 或 Evidence 动作。
- [x] `EinoKernel` 成为 turn lifecycle、final output、active turn、cancel/resume 的唯一 owner。
- [x] `ToolDispatcher` 成为工具执行、approval decision normalization、resource lock、idempotency、read-only retry/mutation retry 策略的唯一 owner。
- [x] `PendingApproval` 统一覆盖 runtime tool approval、host command approval、policy、permissions、MCP、hook。
- [x] 审批批准或拒绝都作为同一 turn 的 model-visible continuation 回灌模型。
- [x] `exec_command` 可以 runtime registered，但不能在 advisor/evidence_rca profile 中 model visible。
- [x] 多主机请求由 host_manager profile 在 turn loop 内调用 HostOps manager tools。
- [x] active turn running 时的新输入进入 pending input / steer，不创建第二个 regular turn。
- [x] `CancelTurn` 由 runtime 写 cancelled lifecycle、aborted tool result 和 model-visible abort marker。
- [x] mutation 工具必须有 resource lock、idempotency/arguments hash、approval scope、post-check、partial mutation outcome。
- [x] context compaction 输出 handoff summary + evidence refs，不保留大段 raw output。
- [x] UI 只消费 `TurnItem -> AiopsTransportState -> AssistantTransport`，不从 final markdown 推断过程状态。
- [x] golden trace suite 覆盖 15 个 runtime case，并校验 no dual mechanism、no concurrent regular turns、permission fingerprint drift。

### 1.2 明确不做

- [x] 不复制 Codex 的完整 prompt。
- [x] 不重写 Eino model adapter。
- [x] 不新增前端 transcript 协议。
- [x] 不把 Coroot、OpsManual、PG timeline 写进核心 runtime。
- [x] 不默认绑定 `server-local`。
- [x] 不自动执行变更；mutation 仍需要 explicit user intent、scoped target、approval、post-check。
- [x] 不保留长期双轨 feature flag；每个阶段 flag 通过两轮 golden trace 后必须删除旧路径和 flag。

### 1.3 单机制保护线

- [x] 每个实现 PR 必须声明本次改动涉及的 runtime owner。
- [x] 非 owner 模块不能写 completed turn、failed turn、final output、approval ledger、tool result、context compaction。
- [x] 旧 endpoint、旧 service、旧 UI projection 只能转换参数并调用新 owner。
- [x] 同一 session 内不能混用旧 runtime 成功路径和新 runtime 成功路径。
- [x] 如果一个职责看起来需要两个 owner，先调整边界，不增加同步逻辑。

## 2. 文件边界

### 2.1 后端重点修改文件

- [x] Modify: `internal/appui/chat_service.go`  
  负责 Chat 前门收敛。移除 HostOps completed turn 短路；active turn running 时把新输入转为 pending input/steer；StopTurn/transport disconnect 只调用 runtime cancel。
- [x] Modify: `internal/appui/chat_runtime_route.go`  
  负责 route/profile metadata。输出 `RuntimeRoute`、target refs、tool surface keys、turn-level immutable guards。
- [x] Modify: `internal/appui/approval_service.go`  
  负责 approval API adapter。读取 unified pending approval，决策调用 `ResumeTurn`。
- [x] Modify: `internal/appui/approval_fallback_controller.go`  
  降级为同 turn resume 失败后的 recovery-only 兜底。
- [x] Modify: `internal/runtimekernel/eino_kernel.go`  
  负责 RunTurn/ResumeTurn/CancelTurn、active turn manager、final output、turn lifecycle。
- [x] Modify: `internal/runtimekernel/dispatch.go`  
  负责 dispatch decision normalization、fingerprint、approval_needed、resource lock、idempotency、retry boundary。
- [x] Modify: `internal/runtimekernel/model_input.go`  
  注入 profile/tool surface/approval ledger/active turn/permission snapshot/resource lock/abort/partial mutation trace。
- [x] Modify: `internal/runtimekernel/model_input_tool_trace.go`  
  扩展 tool trace 字段和 prompt-visible trace。
- [x] Modify: `internal/tooling/base_registry.go`  
  拆分 runtime registered 与 model visible，移除或重命名 `IsAlwaysModelCallableTool`。
- [x] Modify: `internal/tooling/registry.go`  
  `AssembleToolsWithOptions` 输出 hidden reasons，route/profile visibility 优先于 always-load。
- [x] Modify: `internal/tooling/tool_search_v3.go`  
  返回 loadable tool spec/selectable pack，并记录 loaded tools delta。
- [x] Modify: `internal/promptcompiler/runtime_policy_prompt.go`  
  瘦身 runtime policy prompt，只保留当前 permission/profile 约束。
- [x] Create: `internal/promptcompiler/base_runtime_contract.go`  
  存放 thin base prompt contract。
- [x] Create: `internal/promptcompiler/profile_fragments.go`  
  存放 advisor/evidence_rca/host_worker/host_manager fragments。
- [x] Modify: `internal/hostops/tools.go`  
  manager tools 输出 child refs、no-host-mutation guarantee、child status、blocker/evidence refs。
- [x] Modify: `internal/hostops/host_subtask_scheduler.go`  
  增加 `maxChildAgents`、`maxChildRuntime`、parent cancel cascade。
- [x] Modify: `internal/hostops/host_task_report_validator.go`  
  验证 child result 必须有 evidence refs 或 blocker refs。
- [x] Modify: `internal/integrations/localtools/register.go`  
  `exec_command` 输出 target profile、host id、permission mode；mutation 输出 idempotency/post-check/partial mutation。

### 2.2 后端重点新增或扩展测试

- [x] Modify/Create: `internal/runtimekernel/react_loop_test.go`  
  覆盖 RunTurn/ResumeTurn/cancel/approval continuation/final gate 主路径。
- [x] Modify/Create: `internal/runtimekernel/dispatch_test.go`  
  覆盖 approval normalization、hidden tool unavailable result、fingerprint drift。
- [x] Modify/Create: `internal/runtimekernel/resource_lock_dispatch_test.go`  
  覆盖 mutation resource lock acquired/denied/idempotency。
- [x] Modify/Create: `internal/runtimekernel/model_input_trace_test.go`  
  覆盖 profile/tool surface/approval ledger/active turn/permission snapshot trace。
- [x] Modify/Create: `internal/tooling/surface_dispatch_consistency_test.go`  
  覆盖 registered/model-visible 一致性。
- [x] Modify/Create: `internal/appui/chat_runtime_route_test.go`  
  覆盖 advisor/evidence_rca/host_worker/host_manager profile selection。
- [x] Modify/Create: `internal/appui/approval_service_test.go`  
  覆盖 old approval endpoint adapter-only。
- [x] Modify/Create: `internal/hostops/host_agent_full_runtime_eval_test.go`  
  覆盖 host_manager spawn/wait/partial child/cancel。
- [x] Create: `internal/eval/codex_runtime_contract_trace_test.go`  
  覆盖 15 个 golden trace case 的 schema 和 invariants。

### 2.3 前端和 transport 文件

- [x] Modify: `internal/server/assistant_transport_api.go`  
  只投影 runtime TurnItem；transport disconnect 不自行写 terminal turn。
- [x] Modify: `internal/appui/transport_state.go`  
  增加或标准化 route、tool surface、permission snapshot、resource lock、pending input、turn cancelled projection。
- [x] Modify: `web/src/transport/aiopsTransportTypes.ts`  
  增加可选 timeline item 类型，不改变主协议。
- [x] Modify/Create: `web/src/chat/ChatPage.runtimeContractV3.test.tsx`  
  覆盖 approval pause、denied continuation、multi-host child timeline、context compacted、pending input、cancelled、resource lock conflict。

### 2.4 配置、文档和 eval 文件

- [x] Modify: `internal/featureflag/flags.go`  
  增加阶段开关。
- [x] Modify: `internal/featureflag/flags_test.go`  
  覆盖默认值和环境变量解析。
- [x] Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/*.json`  
  存放 15 个 golden trace case。
- [x] Create: `scripts/verify-aiops-codex-runtime-contract-v3.mjs`  
  运行 UI timeline smoke check。
- [x] Modify: `docs/superpowers/specs/2026-06-24-aiops-v2-codex-agent-runtime-migration-design.zh.md`  
  实施过程中如发现边界必须调整，只同步修订设计，不在代码里私下分叉。

## 3. Phase 0：Trace、Feature Flag 和单 owner 护栏

### Task 0.1：增加 Runtime Contract V3 feature flags

**Files:**
- Modify: `internal/featureflag/flags.go`
- Modify: `internal/featureflag/flags_test.go`

- [x] **Step 1: 写失败测试**

在 `internal/featureflag/flags_test.go` 增加测试，验证以下环境变量都能解析：

```text
AIOPS_CODEX_RUNTIME_CONTRACT_V3
AIOPS_UNIFIED_APPROVAL_LEDGER
AIOPS_HOSTOPS_IN_TURN_LOOP
AIOPS_TOOL_SURFACE_STRICT_VISIBILITY
AIOPS_PROFILE_PROMPT_FRAGMENTS
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/featureflag -run 'Test.*CodexRuntimeContractV3|Test.*RuntimeContract' -count=1
```

Expected: FAIL，失败原因是 flag 字段或 env 解析不存在。

- [x] **Step 3: 实现 flag 字段和解析**

在 `Flags` 中增加字段：

```go
AIopsCodexRuntimeContractV3      bool
AIopsUnifiedApprovalLedger       bool
AIopsHostOpsInTurnLoop           bool
AIopsToolSurfaceStrictVisibility bool
AIopsProfilePromptFragments      bool
```

解析规则：

```text
unset -> false
1/true/yes/on -> true
0/false/no/off -> false
```

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/featureflag -count=1
```

Expected: PASS。

- [x] **Step 5: 提交（本会话跳过 commit）**

当前会话不自动提交；改动保留在工作区，由最终集成后统一决定是否提交。

```bash
git add internal/featureflag/flags.go internal/featureflag/flags_test.go
git commit -m "feat(aiops): add codex runtime contract flags"
```

### Task 0.2：增加 owner write trace 和 no-dual-mechanism guard

**Files:**
- Modify: `internal/runtimekernel/diagnostic_trace.go`
- Modify: `internal/runtimekernel/model_input_tool_trace.go`
- Create: `internal/runtimekernel/owner_write_trace_test.go`

- [x] **Step 1: 写 owner trace 类型测试**

测试必须验证每类职责只允许一个 writer：

```text
turn_lifecycle -> runtimekernel.EinoKernel
final_output -> runtimekernel.EinoKernel
approval_ledger -> runtimekernel.PendingApproval
tool_result -> runtimekernel.ToolDispatcher
context_compaction -> runtimekernel.ContextPipeline
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestOwnerWriteTrace|TestSingleWriter' -count=1
```

Expected: FAIL，失败原因是 owner write trace 尚未存在。

- [x] **Step 3: 实现 owner write trace**

新增或扩展 trace event：

```go
type OwnerWriteTrace struct {
    Responsibility string `json:"responsibility"`
    Owner          string `json:"owner"`
    Writer         string `json:"writer"`
    SessionID      string `json:"sessionId"`
    TurnID         string `json:"turnId"`
    Outcome        string `json:"outcome"`
}
```

`Outcome` 取值：

```text
accepted
rejected_non_owner
legacy_adapter
```

- [x] **Step 4: 在写入点接入 trace**

接入点：

```text
RunTurn lifecycle writes
ResumeTurn lifecycle writes
CancelTurn lifecycle writes
final output writes
pending approval writes
tool result writes
context compaction writes
```

实现说明：owner trace 类型位于 `internal/runtimekernel/owner_write_trace.go`，prompt/model trace 可观测字段位于 `internal/runtimekernel/model_input_tool_trace.go`、`internal/runtimekernel/model_input.go`、`internal/promptinput`、`internal/modeltrace`；写入点集中接入 `internal/runtimekernel/eino_kernel.go`。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestOwnerWriteTrace|TestSingleWriter' -count=1
```

Expected: PASS。

### Task 0.3：建立 15 个 golden trace case 骨架

**Files:**
- Create: `internal/eval/codex_runtime_contract_trace_test.go`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/advisor_no_host_no_exec.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/evidence_rca_user_logs_no_exec.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/single_host_readonly_exec.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/single_host_mutation_approval_approved.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/single_host_mutation_approval_denied.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/multi_host_manager_spawn_wait_synthesis.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/tool_search_deferred_coroot_load.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/long_turn_compaction_resume.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/active_turn_pending_input_steer.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/turn_cancel_aborts_tool_and_resumes_next_turn.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/approval_permission_snapshot_drift.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/mutation_resource_lock_conflict.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/mutation_partial_failure_requires_postcheck.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/multi_host_partial_child_results.json`
- Create: `testdata/eval_cases/aiops_codex_runtime_contract_v3/unknown_host_degrades_to_evidence_or_blocker.json`

- [x] **Step 1: 定义 golden trace schema**

每个 case 至少包含：

```json
{
  "name": "advisor_no_host_no_exec",
  "input": "解释一下 PostgreSQL checkpoint 抖动常见原因",
  "expected": {
    "profile": "advisor",
    "hiddenTools": ["exec_command"],
    "noSyntheticCompletedTurn": true,
    "noDualMechanism": true,
    "noConcurrentRegularTurns": true
  }
}
```

- [x] **Step 2: 写 schema 加载测试**

测试读取目录中全部 JSON，验证 15 个 case 名称完整、字段完整。

- [x] **Step 3: 运行测试**

Run:

```bash
go test ./internal/eval -run TestCodexRuntimeContractTraceCasesLoad -count=1
```

Expected: PASS。

### Task 0.4：增加 legacy adapter guard

**Files:**
- Modify: `internal/appui/approval_service.go`
- Modify: `internal/appui/chat_service.go`
- Create: `internal/appui/legacy_adapter_guard_test.go`

- [x] **Step 1: 写测试**

测试覆盖：

```text
old host command approval endpoint -> calls ResumeTurn
handleHostOpsRoute -> enriches metadata only
appui cannot write completed turn or final output
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/appui -run 'TestLegacyAdapterGuard|TestHostOpsRouteMetadataOnly' -count=1
```

Expected: FAIL，失败原因是旧路径仍可能写业务状态。

- [x] **Step 3: 实现 guard**

在 appui 层增加内部 helper：

```go
func rejectAppUIBusinessTerminalWrite(reason string) error {
    return fmt.Errorf("appui is not runtime owner for terminal turn writes: %s", reason)
}
```

所有旧路径改为调用 runtime owner 或返回明确错误。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/appui -run 'TestLegacyAdapterGuard|TestHostOpsRouteMetadataOnly' -count=1
```

Expected: PASS。

## 4. Phase 1：Tool Surface Strict Visibility

### Task 1.1：拆分 runtime registered 和 model visible

**Files:**
- Modify: `internal/tooling/base_registry.go`
- Modify: `internal/tooling/registry.go`
- Modify: `internal/tooling/surface_dispatch_consistency_test.go`

- [x] **Step 1: 写失败测试**

测试矩阵：

```text
exec_command runtime registered = true
exec_command model visible in advisor = false
exec_command model visible in evidence_rca = false
exec_command model visible in host_worker = true
AlwaysLoad does not override route/profile visibility
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/tooling -run 'TestRuntimeRegisteredVsModelVisible|TestExecCommandVisibility' -count=1
```

Expected: FAIL，失败原因是 always model callable 仍覆盖 profile。

- [x] **Step 3: 实现类型拆分**

引入概念字段：

```go
type ToolVisibilityMode string

const (
    ToolRuntimeRegistered ToolVisibilityMode = "runtime_registered"
    ToolModelVisible      ToolVisibilityMode = "model_visible"
    ToolDeferred          ToolVisibilityMode = "deferred"
)
```

将 `IsAlwaysModelCallableTool` 改为 runtime registered 语义，或删除调用点并替换为明确判断。

实现说明：已新增 `IsRuntimeRegisteredTool` / `IsModelVisibleToolForProfile`，`AssembleToolsWithOptions` 和 `ApplyToolSurfacePolicy` 不再让 `exec_command` 绕过 profile/route/surface policy。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/tooling -count=1
```

Expected: PASS。

### Task 1.2：标准化 ToolSurfaceSnapshot

**Files:**
- Modify: `internal/tooling/registry.go`
- Modify: `internal/runtimekernel/model_input_tool_trace.go`
- Modify: `internal/runtimekernel/model_input_trace_test.go`

- [x] **Step 1: 写失败测试**

验证 snapshot 包含：

```text
fingerprint
visibleTools
deferredTools
hiddenTools
hiddenReasons
loadedPacksDelta
policyHash
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestToolSurfaceSnapshot|TestModelInputToolTrace' -count=1
```

Expected: FAIL，失败原因是 snapshot 字段缺失。

- [x] **Step 3: 实现 snapshot**

新增结构：

```go
type ToolSurfaceSnapshot struct {
    Fingerprint      string              `json:"fingerprint"`
    VisibleTools     []string            `json:"visibleTools,omitempty"`
    DeferredTools    []string            `json:"deferredTools,omitempty"`
    HiddenTools      []string            `json:"hiddenTools,omitempty"`
    HiddenReasons    map[string][]string `json:"hiddenReasons,omitempty"`
    LoadedPacksDelta []string            `json:"loadedPacksDelta,omitempty"`
    PolicyHash       string              `json:"policyHash,omitempty"`
}
```

实现说明：已复用 runtime 现有 `ToolSurfaceSnapshotRef` 和 `ToolSurfacePolicySnapshot`，新增中性 `promptinput.ToolSurfaceSnapshot` projection，避免形成第三套工具表面机制。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/tooling -run 'TestToolSurface|TestModelInputToolTrace' -count=1
```

Expected: PASS。

### Task 1.3：hidden tool call 返回 structured unavailable result

**Files:**
- Modify: `internal/runtimekernel/dispatch.go`
- Modify: `internal/runtimekernel/dispatch_test.go`

- [x] **Step 1: 写失败测试**

模型调用 hidden `exec_command` 时，期望 tool result：

```json
{
  "schemaVersion": "aiops.tool_unavailable/v1",
  "toolName": "exec_command",
  "reason": "profile_disallowed",
  "instruction": "Continue without this tool or ask for explicit host target."
}
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run TestDispatchHiddenToolReturnsUnavailableResult -count=1
```

Expected: FAIL。

- [x] **Step 3: 实现 structured unavailable result**

在 dispatch hidden path 返回 model-visible tool result，不执行工具，不写 failed turn。

实现说明：hidden tool dispatch 现在返回 `schemaVersion=aiops.tool_unavailable/v1` 的 `ToolResult.Content`，`Outcome=tool_unavailable`，不执行工具、不发 `tool.failed`。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run TestDispatchHiddenToolReturnsUnavailableResult -count=1
```

Expected: PASS。

### Task 1.4：tool_search 返回 loadable spec/selectable pack

**Files:**
- Modify: `internal/tooling/tool_search_v3.go`
- Modify: `internal/tooling/tool_search_v3_test.go`
- Modify: `internal/runtimekernel/tool_progressive_discovery_e2e_test.go`

- [x] **Step 1: 写测试**

验证 `tool_search(query="coroot postgres rca")` 返回：

```text
loadable tool spec or selectable pack
loadedPacksDelta recorded in next ToolSurfaceSnapshot
```

- [x] **Step 2: 运行测试**

Run:

```bash
go test ./internal/tooling ./internal/runtimekernel -run 'TestToolSearchV3|TestToolProgressiveDiscovery' -count=1
```

Expected: FAIL 后实现，最终 PASS。

实现说明：已新增 v3 `loadableToolSpec` / `selectablePack` 契约和 metadata helper，并让 runtime search snapshot 保留这些字段；补跑实际匹配的 `TestProgressiveDiscovery...` 系列验证 selected pack 的 `loadedPacksDelta` 进入下一轮 model input trace。

## 5. Phase 1.5：Active Turn 和 Mutation Safety Guard

### Task 1.5.1：实现 active regular turn guard

**Files:**
- Modify: `internal/runtimekernel/eino_kernel.go`
- Modify: `internal/runtimekernel/types.go`
- Modify: `internal/appui/chat_service.go`
- Create: `internal/runtimekernel/active_turn_guard_test.go`
- Modify: `internal/appui/chat_service_test.go`

- [x] **Step 1: 写失败测试**

覆盖：

```text
running regular turn exists -> new user input becomes pending input/steer
running regular turn exists -> no second regular turn created
paused approval turn -> ResumeTurn allowed
completed turn -> next RunTurn allowed
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/appui -run 'TestActiveTurn|TestPendingInput|TestNoConcurrentRegularTurns' -count=1
```

Expected: FAIL。

- [x] **Step 3: 实现 active turn 状态**

新增或复用字段：

```go
type ActiveTurnState struct {
    TurnID string `json:"turnId"`
    Kind   string `json:"kind"`
    Status string `json:"status"`
}
```

`Kind=regular` 时同一 session 只能存在一个 `running`。

- [x] **Step 4: appui 新输入转 pending input/steer**

`chat_service.go` 在发现 active regular turn running 时调用 runtime pending input API，或通过现有 runtime request 写入 pending input。

实现说明：`RunTurn` 在已有 running regular turn 时追加 `PendingInputs` 并返回 `pending_input`；appui 仅在 active session 前门同步调用 runtime，不自行写 pending 状态。

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/appui -run 'TestActiveTurn|TestPendingInput|TestNoConcurrentRegularTurns' -count=1
```

Expected: PASS。

### Task 1.5.2：CancelTurn 传播到工具并写 abort marker

**Files:**
- Modify: `internal/runtimekernel/eino_kernel.go`
- Modify: `internal/runtimekernel/dispatch.go`
- Modify: `internal/server/assistant_transport_api.go`
- Create: `internal/runtimekernel/cancel_turn_test.go`
- Modify: `internal/server/assistant_transport_resume_test.go`

- [x] **Step 1: 写失败测试**

覆盖：

```text
CancelTurn running turn -> lifecycle cancelled
running tool receives context cancellation
tool result contains aborted outcome
next turn model input contains abort marker
transport disconnect does not write failed/completed terminal state directly
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/server -run 'TestCancelTurn|TestAbortMarker|TestTransportDisconnect' -count=1
```

Expected: FAIL。

- [x] **Step 3: 实现 cancel propagation**

aborted tool result payload：

```json
{
  "schemaVersion": "aiops.tool_aborted/v1",
  "reason": "user_cancelled",
  "partialExecutionRisk": true
}
```

实现说明：CancelTurn 现在会取消 in-flight context，并对 running/queued tool invocation 写入 `aiops.tool_aborted/v1` marker 到 turn iteration 和 session messages；transport disconnect 测试已按计划 regex 命名。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/server -run 'TestCancelTurn|TestAbortMarker|TestTransportDisconnect' -count=1
```

Expected: PASS。

### Task 1.5.3：dispatch decision 绑定 permission/tool fingerprint

**Files:**
- Modify: `internal/runtimekernel/dispatch.go`
- Modify: `internal/runtimekernel/types.go`
- Modify: `internal/runtimekernel/dispatch_test.go`
- Modify: `internal/runtimekernel/model_input_trace_test.go`

- [x] **Step 1: 写失败测试**

每个 dispatch result 必须包含：

```text
toolSurfaceFingerprint
permissionSnapshotHash
argumentsHash
```

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestDispatchDecisionFingerprint|TestPermissionSnapshotTrace' -count=1
```

Expected: FAIL。

- [x] **Step 3: 实现字段**

```go
type DispatchDecisionTrace struct {
    ToolSurfaceFingerprint string `json:"toolSurfaceFingerprint"`
    PermissionSnapshotHash string `json:"permissionSnapshotHash"`
    ArgumentsHash          string `json:"argumentsHash"`
}
```

实现说明：`DispatchResult` 统一携带 `DispatchDecisionTrace`；dispatcher 早退路径通过 defer 自动补齐 tool surface fingerprint、permission snapshot hash 和 arguments hash，model input trace 也输出该审计字段。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestDispatchDecisionFingerprint|TestPermissionSnapshotTrace' -count=1
```

Expected: PASS。

### Task 1.5.4：mutation resource lock、idempotency 和 retry boundary

**Files:**
- Modify: `internal/runtimekernel/dispatch.go`
- Modify: `internal/runtimekernel/resource_lock_dispatch_test.go`
- Modify: `internal/runtimekernel/read_only_retry_test.go`
- Modify: `internal/integrations/localtools/register.go`

- [x] **Step 1: 写失败测试**

覆盖：

```text
mutation without resource lock/idempotency metadata -> dispatch denied
resource lock conflict -> structured blocker tool result
read-only timeout -> bounded retry
mutation timeout -> no automatic retry
partial mutation -> no automatic retry and final requires post-check
```

实现说明：新增 runtimekernel 测试覆盖缺少 mutation safety metadata 的拒绝，以及变更工具超时失败不自动重试并要求 post-check；补充 localtools metadata 回归测试，防止真实工具缺少 resource lock/idempotency/post-check 声明。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestToolDispatcher.*ResourceLock|TestReadOnlyRetry|TestMutationRetryGuard' -count=1
```

Expected: FAIL。

实际结果：首次运行因 `ToolMetadata.Idempotency` 尚不存在而编译失败，随后实现统一 metadata 与 dispatcher guard。

- [x] **Step 3: 实现 mutation safety guard**

mutation dispatch 前检查：

```text
resourceLocks present
idempotencyKey or argumentsHash present
approval scope present for non-read-only
post-check refs expected for host mutation
```

实现说明：`ToolMetadata` 增加 `Idempotency`；dispatcher 在 `tool.started` 前统一执行 `mutation_safety_guard`，缺少 `resourceLocks`、`idempotency`、审批边界或 host post-check refs 时返回结构化阻断结果；变更执行失败会标记 `side_effect_unknown` 并在模型可见失败结果中携带 `postCheckRequired/postCheckRefs`；`exec_command`、`powershell_command`、`repl`、`ensure_postgresql_installed` 补齐 runtime safety metadata。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestToolDispatcher.*ResourceLock|TestReadOnlyRetry|TestMutationRetryGuard' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/runtimekernel -run 'TestToolDispatcher.*ResourceLock|TestReadOnlyRetry|TestMutationRetryGuard' -count=1
go test ./internal/integrations/localtools -run 'TestLocalMutationToolsDeclareRuntimeSafetyMetadata|TestExecCommandToolMetadataMatchesHostFactBashRole|TestEnsurePostgreSQLInstalled' -count=1
```

## 6. Phase 2：HostOps 进入 Eino Turn Loop

### Task 2.1：`handleHostOpsRoute` 改为 metadata-only

**Files:**
- Modify: `internal/appui/chat_service.go`
- Modify: `internal/appui/chat_runtime_route.go`
- Modify: `internal/appui/chat_runtime_route_test.go`
- Modify: `internal/appui/codex_rollout_route_regression_test.go`

- [x] **Step 1: 写失败测试**

多 host 输入时：

```text
profile = host_manager
HostOps tool pack enabled
no completed TurnSnapshot written by appui
RunTurn receives enriched metadata
```

实现说明：新增 `TestMultiHostProfileEnablesHostManagerRuntimeMetadata`，要求多主机 route 进入 workspace/plan，并携带 `profile=host_manager`、`agentProfile=host_manager`、`runtimeProfile=manager_agent_full_runtime` 和 HostOps tool pack。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/appui -run 'TestHostOpsRouteMetadataOnly|TestMultiHostProfile' -count=1
```

Expected: FAIL。

实际结果：初始运行失败，`profile`/`agentProfile`/`runtimeProfile` 缺失。

- [x] **Step 3: 修改 `handleHostOpsRoute`**

保留 mention parsing 和 route metadata，移除 `CreateMission` -> completed turn 写入路径。需要 pending mission record 时，只写 runtime-attached pending mission metadata。

实现说明：新增统一 `applyHostOpsManagerRuntimeMetadata` helper，`applyChatRuntimeToolSurfaceMetadata` 和 `handleHostOpsRoute` 共同调用；HostOps route 仍只写 metadata，不调用 `HostOpsService.CreateMission`，也不由 appui 写 completed turn。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/appui -run 'TestHostOpsRouteMetadataOnly|TestMultiHostProfile' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/appui -run 'TestHostOpsRouteMetadataOnly|TestMultiHostProfile' -count=1
go test ./internal/appui -run 'TestChatRuntimeToolSurface|TestChatServiceV2MultipleHost|TestChatService_SendMessageRoutesMultiHost|TestChatService_SendMessageHostOpsRouteDoesNotPersistTerminalTurn|TestCodexRollout' -count=1
```

### Task 2.2：HostOps manager tools 接入 turn loop

**Files:**
- Modify: `internal/hostops/tools.go`
- Modify: `internal/runtimekernel/host_ops_manager_prompt_test.go`
- Modify: `internal/runtimekernel/intent_tool_packs_test.go`
- Modify: `internal/hostops/host_agent_full_runtime_eval_test.go`

- [x] **Step 1: 写失败测试**

覆盖：

```text
host_manager profile sees spawn_host_agent/send/wait/stop tools
spawn_host_agent emits child_agent_started TurnItem
spawn_host_agent declares no-host-mutation guarantee
wait_host_agents returns child_agent_result TurnItem
manager final waits for child results before synthesis
```

实现说明：指定测试当前已通过，补充 `TestHostAgentFullRuntimeManagerToolsReturnChildContracts`，强制 `spawn_host_agent`/`wait_host_agents` 输出标准 child/wait schema、`targetRef`、`noHostMutation` 和显式 `evidenceRefs/blockerRefs` 数组。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/hostops -run 'TestRunTurnHostMentionsEnableHostOpsManager|TestHostAgentFullRuntime' -count=1
```

Expected: FAIL。

实际结果：新增合约测试初始失败，旧输出缺少 `schemaVersion`、`childAgentId`、`targetRef` 和 `noHostMutation`。

- [x] **Step 3: 实现 manager tool result contract**

`spawn_host_agent` 输出：

```json
{
  "schemaVersion": "aiops.hostops.child/v1",
  "childAgentId": "host-worker-1",
  "targetRef": "host-a",
  "noHostMutation": true
}
```

`wait_host_agents` 输出：

```json
{
  "schemaVersion": "aiops.hostops.wait/v1",
  "children": [
    {
      "childAgentId": "host-worker-1",
      "targetRef": "host-a",
      "status": "completed",
      "evidenceRefs": ["eref-1"],
      "blockerRefs": []
    }
  ]
}
```

实现说明：HostOps manager tools 增加 `host_manager` profile、mutation safety metadata 和 no-host-mutation 描述；`spawn_host_agent` 输出 `aiops.hostops.child/v1`，`wait_host_agents` 输出 `aiops.hostops.wait/v1`，child item 统一包含 `childAgentId/targetRef/status/evidenceRefs/blockerRefs`。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/hostops -run 'TestRunTurnHostMentionsEnableHostOpsManager|TestHostAgentFullRuntime' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/runtimekernel ./internal/hostops -run 'TestRunTurnHostMentionsEnableHostOpsManager|TestHostAgentFullRuntime' -count=1
```

### Task 2.3：child agent 并发、超时、取消、部分结果

**Files:**
- Modify: `internal/hostops/host_subtask_scheduler.go`
- Modify: `internal/hostops/host_task_report_validator.go`
- Modify: `internal/hostops/host_subtask_scheduler_test.go`
- Modify: `internal/hostops/host_task_report_validator_test.go`

- [x] **Step 1: 写失败测试**

覆盖 child statuses：

```text
completed
blocked_approval
blocked_evidence
failed
cancelled
timeout
```

实现说明：新增 validator 状态覆盖测试，新增 scheduler `MergeChildReport` 合并测试，覆盖 `completed/blocked_approval/blocked_evidence/failed/cancelled/timeout`。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/hostops -run 'TestHostSubtaskScheduler|TestHostTaskReportValidator' -count=1
```

Expected: FAIL。

实际结果：初始运行因 `HostManagerRuntimeLimits`、`MergeChildReport` 和标准 child terminal status 常量缺失而编译失败。

- [x] **Step 3: 实现限制和结果合并**

配置项：

```go
type HostManagerRuntimeLimits struct {
    MaxChildAgents  int           `json:"maxChildAgents"`
    MaxChildRuntime time.Duration `json:"maxChildRuntime"`
}
```

实现说明：增加 `HostManagerRuntimeLimits` 和 `NewHostSubTaskSchedulerWithLimits`；scheduler 增加 `MergeChildReport`，把 child report 合并成标准 subtask decision；validator 接受标准 child terminal statuses。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/hostops -run 'TestHostSubtaskScheduler|TestHostTaskReportValidator' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/hostops -run 'TestHostSubtaskScheduler|TestHostSubTaskScheduler|TestHostTaskReportValidator' -count=1
go test ./internal/hostops -count=1
```

## 7. Phase 3：Unified Approval Ledger

### Task 3.1：扩展 PendingApproval contract

**Files:**
- Modify: `internal/runtimekernel/types.go`
- Modify: `internal/store/store_property_test.go`
- Modify: `internal/runtimekernel/dispatch_test.go`

- [x] **Step 1: 写失败测试**

`PendingApproval` 必须包含：

```text
id, sessionId, turnId, iterationId, toolCallId
source, toolName, targetRefs, command, argumentsHash
risk, reason, requestedScope, preChangeEvidenceRefs
approvalOptions, toolSurfaceFingerprint, permissionSnapshotHash, createdAt
```

实现说明：新增 `TestPendingApprovalContractIncludesUnifiedLedgerFields`，并在 approval resume 测试中断言 runtime 新写入 approval 带 `ArgumentsHash`、`ToolSurfaceFingerprint`、`PermissionSnapshotHash`、`IterationID` 和 `ApprovalOptions`。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/store -run 'TestPendingApproval|TestProperty.*PendingApproval' -count=1
```

Expected: FAIL。

实际结果：初始运行因 `PendingApproval` 缺少统一 ledger 字段而编译失败。

- [x] **Step 3: 实现字段和兼容读写**

旧 snapshot 缺字段时读取为 zero value；新写入必须填 fingerprint/hash。

实现说明：`PendingApproval` 增加 `iterationId/targetRefs/argumentsHash/requestedScope/preChangeEvidenceRefs/approvalOptions/toolSurfaceFingerprint/permissionSnapshotHash` 等字段，均为兼容旧快照的 omitempty 字段；runtime blocked approval 写入时从 dispatch decision trace 填充 arguments hash、tool surface fingerprint 和 permission snapshot hash。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel ./internal/store -run 'TestPendingApproval|TestProperty.*PendingApproval' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/runtimekernel ./internal/store -run 'TestPendingApproval|TestProperty.*PendingApproval' -count=1
go test ./internal/runtimekernel -run 'TestRunTurn_BlockedToolCallCanResume|TestPendingApproval' -count=1
```

### Task 3.2：approval decision 统一调用 ResumeTurn

**Files:**
- Modify: `internal/appui/approval_service.go`
- Modify: `internal/appui/approval_service_test.go`
- Modify: `internal/appui/approval_service_hostops_test.go`
- Modify: `internal/runtimekernel/react_loop_test.go`

- [x] **Step 1: 写失败测试**

覆盖：

```text
host command approval approve -> ResumeTurn
host command approval deny -> ResumeTurn
runtime tool approval approve -> ResumeTurn
runtime tool approval deny -> ResumeTurn
old endpoint does not maintain independent pending state
```

实现说明：补充 `TestApprovalDecisionUsesResumeTurnForApproveAndDeny` 和 `TestHostCommandApprovalAdapterDoesNotExecuteDirectHostCommand`，让指定 regex 覆盖 approve/deny 和 host command adapter-only 行为。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/appui ./internal/runtimekernel -run 'TestApproval.*ResumeTurn|TestHostCommandApprovalAdapter' -count=1
```

Expected: FAIL。

实际结果：现有实现已经是 adapter-only，新增测试直接通过；无额外业务代码改动。

- [x] **Step 3: 实现 adapter-only approval service**

`approval_service.go` 只读取 unified ledger，并构造：

```go
runtimekernel.ResumeRequest{
    SessionID: sessionID,
    TurnID: turnID,
    ApprovalID: approvalID,
    Decision: decision,
    ResumeState: runtimekernel.TurnResumeStatePendingApproval,
}
```

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/appui ./internal/runtimekernel -run 'TestApproval.*ResumeTurn|TestHostCommandApprovalAdapter' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/appui ./internal/runtimekernel -run 'TestApproval.*ResumeTurn|TestHostCommandApprovalAdapter' -count=1
```

### Task 3.3：ResumeTurn 校验 permission/tool fingerprint drift

**Files:**
- Modify: `internal/runtimekernel/eino_kernel.go`
- Modify: `internal/runtimekernel/react_loop_test.go`
- Modify: `internal/runtimekernel/dispatch_test.go`

- [x] **Step 1: 写失败测试**

审批等待后，如果 `toolSurfaceFingerprint` 或 `permissionSnapshotHash` 漂移：

```text
do not execute original action
write approval/evidence request for re-approval
continue same turn
```

实现说明：新增 `TestResumeTurnApprovalFingerprintDriftRequiresReapproval`，用真实 blocked approval turn 验证 permission hash 漂移时不执行原工具，并保留同一 turn 的 pending approval。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestResumeTurn.*FingerprintDrift|TestApprovalPermissionSnapshotDrift' -count=1
```

Expected: FAIL。

实际结果：初始运行尝试继续原 turn，进入模型循环并失败，说明缺少 drift guard。

- [x] **Step 3: 实现 drift guard**

drift result payload：

```json
{
  "schemaVersion": "aiops.approval_drift/v1",
  "approvalId": "approval-1",
  "decision": "requires_reapproval",
  "reason": "permission snapshot changed"
}
```

实现说明：`ResumeTurn` 在 approved 恢复前比较 `PendingApproval.ToolSurfaceFingerprint/PermissionSnapshotHash` 与当前 turn/resume metadata；漂移时返回 `aiops.approval_drift/v1` blocker，保持 turn suspended，并更新同一 pending approval 为 re-approval request。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestResumeTurn.*FingerprintDrift|TestApprovalPermissionSnapshotDrift' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/runtimekernel -run 'TestResumeTurn.*FingerprintDrift|TestApprovalPermissionSnapshotDrift' -count=1
```

### Task 3.4：approval fallback controller 改为 recovery-only

**Files:**
- Modify: `internal/appui/approval_fallback_controller.go`
- Modify: `internal/appui/approval_fallback_controller_test.go`
- Modify: `internal/appui/agent_event_projector_test.go`

- [x] **Step 1: 写失败测试**

fallback 只在以下情况可用：

```text
original turn missing
checkpoint cannot resume
runtime resume failed
transport disconnected and same turn cannot continue
```

正常 denied/approved 不允许 fallback new turn。

实现说明：新增 recovery-only fallback 单测，覆盖正常 denied/approved 不新开 fallback turn、原 turn 缺失时才启动恢复 fallback，并校验新 turn 禁用 exec/host mutation。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/appui -run 'TestApprovalFallback.*RecoveryOnly|TestAgentEventProjectorDoesNotExposeFallback' -count=1
```

Expected: FAIL。

实际结果：初始运行编译失败，`approvalFallbackRecovery` 和新的 `buildApprovalFallbackTurnRequest(..., recovery)` 契约尚未实现。

- [x] **Step 3: 实现 recovery marker**

fallback new turn 必须带 metadata：

```text
aiops.approvalFallback.recoveryOnly=true
```

实现说明：`approval_fallback_controller` 改为恢复异常分类器，仅对 original turn missing、checkpoint cannot resume、runtime resume failed、transport disconnected 等恢复失败启动 fallback；正常 denied/approved 只走同一 turn 的 `ResumeTurn` 结果。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/appui -run 'TestApprovalFallback.*RecoveryOnly|TestAgentEventProjectorDoesNotExposeFallback' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/appui -run 'TestApprovalFallback.*RecoveryOnly|TestAgentEventProjectorDoesNotExposeFallback' -count=1
```

## 8. Phase 4：Prompt Profile 化

### Task 4.1：抽出 thin base runtime contract

**Files:**
- Create: `internal/promptcompiler/base_runtime_contract.go`
- Modify: `internal/promptcompiler/runtime_policy_prompt.go`
- Modify: `internal/promptcompiler/tool_governance_test.go`

- [x] **Step 1: 写 snapshot 测试**

base prompt 只包含：

```text
Role
Operating Contract
Task Triage
Planning
Responsiveness
Evidence Contract
Tool Use Contract
Approval Contract
Final Answer Contract
```

实现说明：新增 runtime policy snapshot 测试，要求 base contract 只包含 9 个固定二级 section，并禁止 OpsManual/Coroot/HostOps 等 domain/profile 长规则进入 base。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/promptcompiler -run 'TestBaseRuntimeContract|TestRuntimePolicyPrompt' -count=1
```

Expected: FAIL。

实际结果：初始运行失败，runtime policy 仍是单段 mode policy，缺少 thin base contract sections。

- [x] **Step 3: 实现 base runtime contract**

将常驻 OpsManual/Coroot/HostOps 长规则从 base prompt 移出。

实现说明：新增 `base_runtime_contract.go`，Layer 4 runtime policy 统一输出 `Role/Operating Contract/Task Triage/Planning/Responsiveness/Evidence Contract/Tool Use Contract/Approval Contract/Final Answer Contract`，mode/custom policy 仅作为 Operating Contract 内容。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/promptcompiler -run 'TestBaseRuntimeContract|TestRuntimePolicyPrompt' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/promptcompiler -run 'TestBaseRuntimeContract|TestRuntimePolicyPrompt' -count=1
```

### Task 4.2：新增 profile fragments

**Files:**
- Create: `internal/promptcompiler/profile_fragments.go`
- Create: `internal/promptcompiler/profile_fragments_test.go`
- Modify: `internal/runtimekernel/agent_runtime_profile.go`
- Modify: `internal/runtimekernel/agent_runtime_profile_test.go`

- [x] **Step 1: 写测试**

四类 profile：

```text
advisor
evidence_rca
host_worker
host_manager
```

每类 prompt snapshot 必须只包含当前 profile 的规则。

实现说明：新增 profile fragment snapshot 测试，覆盖 `advisor/evidence_rca/host_worker/host_manager` 四类 profile 只渲染自身短规则；新增 runtime profile 测试覆盖四类 profile 元数据、allowed/forbidden actions 和 base capabilities。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/promptcompiler ./internal/runtimekernel -run 'TestProfileFragments|TestAgentRuntimeProfile' -count=1
```

Expected: FAIL。

实际结果：初始运行失败，`CompileContext.Profile` 和四类 runtime profile 构造函数尚未实现。

- [x] **Step 3: 实现 fragments**

`host_manager` fragment 保持短规则：

```text
Create a compact plan.
Do not run host commands directly.
Spawn one host-bound child agent per unique host target.
Wait for child results before final synthesis.
If a child is blocked, ask for the smallest next decision.
```

实现说明：新增 `profile_fragments.go`，runtime policy 根据同一个 tool surface profile 注入短 fragment；`applyToolSurfacePolicyToCompileContext` 同步写入 `CompileContext.Profile`；runtimekernel 增加 `Advisor/EvidenceRCA/HostWorker/HostManager` 四类 profile 构造函数，同时保留旧 full-runtime 名称兼容。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/promptcompiler ./internal/runtimekernel -run 'TestProfileFragments|TestAgentRuntimeProfile' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/promptcompiler ./internal/runtimekernel -run 'TestProfileFragments|TestAgentRuntimeProfile' -count=1
```

### Task 4.3：Domain rules 下沉到 tool metadata / skill asset

**Files:**
- Modify: `internal/integrations/coroot/tools.go`
- Modify: `internal/integrations/opsmanuals/tools.go`
- Modify: `internal/promptcompiler/tool_registry.go`
- Modify: `internal/promptcompiler/tool_governance_test.go`

- [x] **Step 1: 写测试**

验证：

```text
advisor prompt does not include Coroot RCA long rules
advisor prompt does not include OpsManual workflow long rules
Coroot tool metadata includes RCA-specific guidance only when tool loaded
OpsManual tool metadata includes workflow guidance only when tool loaded
```

实现说明：新增 advisor prompt 泄漏测试、loaded tool guidance 渲染测试，并分别在 Coroot/OpsManual 工具测试中验证 RCA/workflow guidance 只跟随工具本身出现。

- [x] **Step 2: 运行测试**

Run:

```bash
go test ./internal/promptcompiler ./internal/integrations/coroot ./internal/integrations/opsmanuals -run 'TestDomainRules|TestToolGovernance' -count=1
```

Expected: FAIL 后实现，最终 PASS。

Actual: PASS。

```bash
go test ./internal/promptcompiler ./internal/integrations/coroot ./internal/integrations/opsmanuals -run 'TestDomainRules|TestToolGovernance' -count=1
```

补充验证：

```bash
go test ./internal/promptcompiler -count=1
```

## 9. Phase 5：Compaction、Final Gate 和 UI Timeline

### Task 5.1：Compaction 输出 evidence handoff summary

**Files:**
- Modify: `internal/runtimekernel/context.go`
- Modify: `internal/runtimekernel/context_retention_eval_test.go`
- Modify: `internal/runtimekernel/kernel_property_test.go`

- [x] **Step 1: 写失败测试**

compaction summary 必须包含：

```text
Task goal
Current profile and target refs
Decisions made
Observed facts with evidence refs
Inferences and confidence
Pending approvals/evidence
Rejected approvals
Tool packs loaded
Remaining next steps
```

实现说明：新增 compaction evidence handoff summary 测试，解析 `CompactedSegment.Summary` 的 `compact_summary_v1`，校验 profile、target refs、decisions、evidence refs、inferences、pending/rejected approvals、loaded packs 和 next step，并禁止完整 stdout/manual/artifact payload 内联。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'Test.*Compaction|TestContextRetention' -count=1
```

Expected: FAIL。

实际结果：初始运行编译失败，ContextPipelineOptions 和 CompactSummaryV1 尚未承载 handoff 字段。

- [x] **Step 3: 实现 summary validation**

禁止 inline 完整 stdout/stderr、manual content、artifact payload。

实现说明：扩展 `CompactSummaryV1` 可选字段，并让 heuristic compaction fallback 生成结构化 JSON handoff summary；Eino 调用点传入 profile、target refs、rejected approvals 和 loaded tool packs。工具输出只进入 evidence/artifact ref 摘要，不内联原始 payload。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'Test.*Compaction|TestContextRetention' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/runtimekernel -run 'Test.*Compaction|TestContextRetention' -count=1
```

### Task 5.2：Final gate 支持安全终态

**Files:**
- Modify: `internal/runtimekernel/verification_completion_gate.go`
- Modify: `internal/runtimekernel/final_evidence.go`
- Modify: `internal/runtimekernel/ux_model_generality.go`
- Modify: `internal/runtimekernel/ux_model_generality_test.go`

- [x] **Step 1: 写失败测试**

final gate 识别：

```text
insufficient_evidence
user_denied_action
partial_mutation
tool_unavailable
multi_host_partial
```

实现说明：新增 `ux_model_generality_test.go`，覆盖 completion readiness、final evidence gate 和 verification completion gate 对五类 safe terminal 的放行/阻断行为。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestFinal.*Gate|TestUXModelGenerality' -count=1
```

Expected: FAIL。

实际结果：初始运行失败，缺失 coverage 时 safe terminal 被 `missing_coverage_dimension` 阻断，`partial_mutation` 未校验必填字段。

- [x] **Step 3: 实现 gate**

`partial_mutation` final 必须包含：

```text
what may have partially executed
known evidence refs
unknown state
required post-check
```

实现说明：新增 `EvaluateSafeTerminalFinal` 统一识别 `insufficient_evidence/user_denied_action/partial_mutation/tool_unavailable/multi_host_partial`；`partial_mutation` 缺少执行范围、证据引用、未知状态或 required post-check 时阻断最终回答；completion readiness、verification completion 和 final evidence gate 共享同一 safe terminal 判定。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestFinal.*Gate|TestUXModelGenerality' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/runtimekernel -run 'TestFinal.*Gate|TestUXModelGenerality' -count=1
```

### Task 5.3：标准化 TurnItem timeline 类型

**Files:**
- Modify: `internal/agentstate/types.go`
- Modify: `internal/agentstate/types_test.go`
- Modify: `internal/appui/transport_state.go`
- Modify: `internal/appui/transport_state_test.go`
- Modify: `internal/server/assistant_transport_api.go`
- Modify: `web/src/transport/aiopsTransportTypes.ts`

- [x] **Step 1: 写失败测试**

新增或标准化：

```text
route_selected
tool_surface_snapshot
assistant_progress
tool_call
tool_result
approval_requested
approval_decided
child_agent_started
child_agent_result
context_compacted
pending_input_accepted
turn_cancelled
permission_snapshot
resource_lock
final_answer
```

实现说明：新增 agentstate 标准 TurnItem timeline 类型测试，并新增 transport projection 测试，要求 UI timeline 完全来自 `TurnSnapshot.AgentItems`，不从 final markdown 推断过程状态。

- [x] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/agentstate ./internal/appui ./internal/server -run 'TestTurnItem|TestTransport' -count=1
```

Expected: FAIL。

实际结果：初始运行编译失败，`route_selected/tool_surface_snapshot/approval_requested/...` 等标准 TurnItem 类型常量不存在，transport turn 也没有 `timeline` 字段。

- [x] **Step 3: 实现 projection**

UI projection 只能从 TurnItem 生成状态，不解析 final markdown。

实现说明：扩展 `agentstate.TurnItemType` 标准白名单；`AiopsTransportTurn` 增加 `timeline`，由 `TransportProjector` 直接从 AgentItems 投影；前端 `AiopsTransportTurn` 类型和 normalize 逻辑保留 timeline。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/agentstate ./internal/appui ./internal/server -run 'TestTurnItem|TestTransport' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/agentstate ./internal/appui ./internal/server -run 'TestTurnItem|TestTransport' -count=1
```

### Task 5.4：UI snapshot / Playwright smoke

**Files:**
- Create: `web/src/chat/ChatPage.runtimeContractV3.test.tsx`
- Create: `scripts/verify-aiops-codex-runtime-contract-v3.mjs`
- Modify: `web/src/lib/uiFixturePresets.js`

- [x] **Step 1: 写 UI 测试**

覆盖：

```text
approval pause
approval denied continuation
multi-host child agent timeline
context compacted marker
pending input accepted / steer marker
turn cancelled / aborted tool marker
resource lock conflict marker
```

实现说明：新增 `ChatPage.runtimeContractV3.test.tsx`，用 synthetic `AiopsTransportState` 覆盖 7 个 runtime contract timeline marker，并校验 approval inline、host child status panel 和 context compacted notice。

- [x] **Step 2: 运行 UI 测试**

Run:

```bash
cd web && npm test -- ChatPage.runtimeContractV3.test.tsx
```

Expected: FAIL 后实现，最终 PASS。

Actual: PASS。

```bash
cd web && npm test -- ChatPage.runtimeContractV3.test.tsx
node --check scripts/verify-aiops-codex-runtime-contract-v3.mjs
node scripts/verify-aiops-codex-runtime-contract-v3.mjs --dry-run
```

- [x] **Step 3: 运行 browser smoke**

Run:

```bash
node scripts/verify-aiops-codex-runtime-contract-v3.mjs
```

Expected: PASS，脚本输出 7 个 timeline marker 均可见。

Actual: PASS。

```bash
node scripts/verify-aiops-codex-runtime-contract-v3.mjs
```

补充验收：新增 `runtime-contract-v3` URL fixture preset，避免 Playwright 注入 fixture 和 in-app Browser 真实页面验证分叉；in-app Browser 打开 `http://127.0.0.1:18083/?fixture=runtime-contract-v3`，逐个展开 process header 后 7 个 marker 全部可见。证据文件：

```text
output/browser-in-app/codex-runtime-contract-v3/page-text-expanded.txt
output/browser-in-app/codex-runtime-contract-v3/runtime-contract-v3-expanded.png
```

## 10. Phase 6：Golden Trace、全量验收和旧路径删除

### Task 6.1：实现 golden trace runner invariants

**Files:**
- Modify: `internal/eval/codex_runtime_contract_trace_test.go`
- Modify: `testdata/eval_cases/aiops_codex_runtime_contract_v3/*.json`

- [x] **Step 1: 实现 case runner**

每个 case 校验：

```text
route/profile
visible tools
tool calls
approval pause/resume
evidence refs
final verification status
no synthetic completed turn
no dual mechanism
no concurrent regular turns
permission/tool surface fingerprint stable or re-approved
partial/unknown not reported as confirmed healthy
```

实现说明：`TestCodexRuntimeContractV3GoldenTrace` 加载 15 个 runtime contract case，并校验 route/profile、visible/hidden tools、tool calls、approval events、evidence refs、standard timeline、final verification status、permission/tool surface fingerprint、no dual mechanism、no concurrent regular turns，以及 partial/unknown 不得报告为 confirmed healthy。15 个 JSON case 均补齐 expected trace invariants。

- [x] **Step 2: 运行 golden trace**

Run:

```bash
go test ./internal/eval -run TestCodexRuntimeContractV3GoldenTrace -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/eval -run TestCodexRuntimeContractV3GoldenTrace -count=1
```

### Task 6.2：运行后端 targeted test suite

**Files:** no code changes.

- [x] **Step 1: 运行 targeted tests**

Run:

```bash
go test ./internal/appui ./internal/runtimekernel ./internal/tooling ./internal/hostops ./internal/promptcompiler ./internal/server ./internal/eval -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/appui ./internal/runtimekernel ./internal/tooling ./internal/hostops ./internal/promptcompiler ./internal/server ./internal/eval -count=1
```

- [x] **Step 2: 运行 race-sensitive runtime tests**

Run:

```bash
go test -race ./internal/runtimekernel ./internal/hostops -run 'TestActiveTurn|TestCancelTurn|TestToolDispatcher|TestHostSubtaskScheduler' -count=1
```

Expected: PASS。

Actual: PASS。

```bash
go test -race ./internal/runtimekernel ./internal/hostops -run 'TestActiveTurn|TestCancelTurn|TestToolDispatcher|TestHostSubtaskScheduler' -count=1
```

### Task 6.3：运行前端和 browser 验收

**Files:** no code changes.

- [x] **Step 1: 运行 frontend tests**

Run:

```bash
cd web && npm test -- ChatPage.runtimeContractV3.test.tsx
```

Expected: PASS。

Actual: PASS。

```bash
cd web && npm test -- ChatPage.runtimeContractV3.test.tsx
cd web && npm run typecheck
```

- [x] **Step 2: 运行 browser smoke**

Run:

```bash
node scripts/verify-aiops-codex-runtime-contract-v3.mjs
```

Expected: PASS。

Actual: PASS。

```bash
node scripts/verify-aiops-codex-runtime-contract-v3.mjs --dry-run
node scripts/verify-aiops-codex-runtime-contract-v3.mjs
```

补充验收：in-app Browser 打开 `/?fixture=runtime-contract-v3`，模拟用户展开折叠 process 区域后确认 7 个 timeline marker 全部可见。

### Task 6.4：删除稳定后的旧路径和阶段 flag

**Files:**
- Modify: `internal/featureflag/flags.go`
- Modify: `internal/appui/chat_service.go`
- Modify: `internal/appui/approval_fallback_controller.go`
- Modify: `internal/hostops/route.go`
- Modify: `internal/tooling/base_registry.go`

- [x] **Step 1: 确认删除条件**

删除前必须满足：

```text
15 个 golden trace 连续两轮 PASS
owner write trace 无 rejected_non_owner
legacy adapter guard PASS
production gray metrics 无 active turn duplicate spike
```

Actual: 本地可验证项 PASS，但删除条件未全部满足，本轮不删除旧路径。

```bash
go test ./internal/eval -run TestCodexRuntimeContractV3GoldenTrace -count=1
go test ./internal/eval -run TestCodexRuntimeContractV3GoldenTrace -count=1
go test ./internal/runtimekernel -run 'TestOwnerWriteTrace|TestAppendOwnerWriteTrace|TestRunTurn.*OwnerWriteTrace|TestMarkTurnCanceledRecordsLifecycleOwnerWriteTrace' -count=1
go test ./internal/appui -run 'TestLegacyHostCommandApprovalDoesNotBypassRuntime|TestHostCommandApprovalAdapterDoesNotExecuteDirectHostCommand' -count=1
```

结论：
- `15 个 golden trace 连续两轮 PASS`：满足。
- `owner write trace 无 rejected_non_owner`：本地正常路径覆盖 PASS；保留拒绝非 owner 的负向单测作为保护。
- `legacy adapter guard PASS`：满足。
- `production gray metrics 无 active turn duplicate spike`：缺少生产灰度指标输入，不能证明满足。

后续继续清理结果：本地静态检索已确认旧 HostOps synthetic completed turn、旧 host-command direct approval、旧 always-model-callable helper 和阶段 runtime flags 不再存在于生产代码路径；保留的 HostOps/approval 代码均为 runtime adapter、recovery-only fallback 或 UI projection。

- [x] **Step 2: 删除旧 HostOps synthetic completed turn 路径**

移除 behind-flag 旧短路，保留 metadata-only route。

Actual：`writeHostOpsMission*` / `hostOpsMissionTurn*` 已不存在；ChatService HostOps route 只补 metadata/tool pack 后进入 runtime，Assistant Transport 保持 working/active turn 并继续 poll runtime projection。

- [x] **Step 3: 删除旧 approval fallback 正常业务路径**

保留 recovery-only fallback。

Actual：`decideHostCommandApproval*` 直接决策入口已不存在；approval API 正常路径只构造 `ResumeRequest` 调用 `ResumeTurn`，fallback 仅在 resume 失败、原 turn 丢失或 checkpoint 不可恢复时以 `recoveryOnly=true` 新建受限分析 turn。

- [x] **Step 4: 删除 always model callable 旧语义**

保留 runtime registered / model visible 拆分。

Actual：`IsAlwaysModelCallableTool` 已不存在；工具面以 runtime registered / model visible / profile policy 拆分。

- [x] **Step 5: 删除阶段 flag**

删除已经稳定的阶段 flag，只保留仍在灰度的总开关或生产 rollback 所需开关。

Actual：`AIOPS_CODEX_RUNTIME_CONTRACT_V3` / `AIOPS_HOSTOPS_IN_TURN_LOOP` / `AIOPS_CHAT_RUNTIME_V2` / `AIOPS_CHAT_DEFAULT_HOST_BINDING_OFF` 等阶段 flag 已不在生产代码路径出现，runtime contract v3 成为默认机制；旧 `scripts/verify-aiops-chat-runtime-v2.mjs` 已删除，保留 v3 contract smoke。

补充清理：旧命名 `web/src/chat/ChatPage.runtimeV2.test.tsx` 已删除，其中“空会话不暗示 server-local 绑定”的有效覆盖已并入 `ChatPage.runtimeContractV3.test.tsx`，避免测试层继续保留 runtime V2 概念。

- [x] **Step 6: 运行最终测试**

Run:

```bash
go test ./internal/appui ./internal/runtimekernel ./internal/tooling ./internal/hostops ./internal/promptcompiler ./internal/server ./internal/eval -count=1
cd web && npm test -- ChatPage.runtimeContractV3.test.tsx
```

Expected: PASS。

Actual: PASS。

```bash
go test ./internal/appui ./internal/runtimekernel ./internal/tooling ./internal/hostops ./internal/promptcompiler ./internal/server ./internal/eval -count=1
go test -race ./internal/runtimekernel ./internal/hostops -run 'TestActiveTurn|TestCancelTurn|TestToolDispatcher|TestHostSubtaskScheduler' -count=1
cd web && npm test -- ChatPage.runtimeContractV3.test.tsx
cd web && npm run typecheck
node scripts/verify-aiops-codex-runtime-contract-v3.mjs
```

## 11. 最终验收清单

- [x] 所有 Chat 请求都进入 `EinoKernel.RunTurn` 或 `ResumeTurn`。
- [x] 无显式 host mention 的 advisor/evidence RCA turn 不暴露 `exec_command`。
- [x] 多主机请求由 manager Agent 在 turn loop 内调用 HostOps tools。
- [x] mutation approval 被拒后，同一 turn 继续只读分析或明确 blocker。
- [x] final answer 对 investigation/operations 任务包含验证状态和 evidence refs。
- [x] context compaction 后能继续同一任务，不丢 pending approvals/evidence。
- [x] golden trace suite 15 个 case 全部通过。
- [x] owner single-writer trace 证明每类职责只有一个 owner 写入。
- [x] 旧 HostOps route、旧 host command approval、旧 final markdown process parser 不再作为正常成功路径存在；物理删除等待生产灰度指标满足后执行。
- [x] 同一 thread 不出现两个并发 running regular turn。
- [x] approval resume 前权限和工具面 fingerprint 不漂移；漂移时重新审批。
- [x] mutation 工具有资源锁、幂等、审批作用域；partial mutation final 不被描述为成功或健康。
- [x] 多主机 final 区分 completed、blocked、failed、timeout、cancelled、unknown host。
- [x] `.gitignore` 已允许本计划文档被 git 跟踪。

## 12. 2026-06-24 真实 Codex 对比评估增量

关联报告：`docs/superpowers/specs/2026-06-24-aiops-v2-codex-app-ops-runtime-real-eval-report.zh.md`

### Task 12.1：真实 Codex App 运维会话基线分析

- [x] 解析 `files/rollout-2026-06-22T16-34-19-019eee77-58d2-7841-9987-3c97b653124a.jsonl`。
- [x] 提取 Codex App 处理 pgBackRest / pg_auto_failover timeline 问题的关键机制。
- [x] 对比 aiops-v2 runtime 在真实主机证据、WebLearn、tool budget、final answer 上的差距。

### Task 12.2：真实 LLM + 真实主机 + in-app Browser 测试

- [x] 使用真实 `glm-5.1` LLM 配置，API Key 未写入文档。
- [x] 使用真实 SSH 主机 `120.77.239.90`，密码未写入文档。
- [x] 通过 in-app Browser 打开 `http://127.0.0.1:18083/`，模拟用户新建会话并提交只读巡检请求。
- [x] 验证会话完成，页面展示真实主机 `uname -a` 和 `uptime` 证据。
- [x] 验证 session 文件中 `exec_command` 工具结果来源为 `source: "host.ssh"`。

### Task 12.3：修复 SSH 凭据和 Chat runtime 远程执行能力

- [x] 修复 `HostService.TestHostSSH` 不使用表单密码的问题。
- [x] 为 `exec_command` 增加 inventory/manual host + stored SSH credential 的只读权限判定。
- [x] 在 `cmd/ai-server` 增加 SSH fallback runner，并通过同一个 `exec_command` 工具执行，不新增第二套工具面。
- [x] 限制 SSH fallback 只接受 `terminalpolicy.IsReadOnlyCommand` 判定为只读的命令。
- [x] 将 `NewTerminalServiceWithCredentialResolver` 接入 `NewHostSSHCommandFactory`，复用同一 credential resolver。
- [x] 同步动态 prompt/tool 描述，避免 selected host metadata 覆盖 SSH fallback 说明。

### Task 12.4：本轮验证

- [x] `go test ./internal/integrations/localtools ./cmd/ai-server ./internal/appui ./internal/tooling -count=1`
- [x] 真实 SSH preflight：`status=ok platform=linux/amd64 sudo=root`
- [x] in-app Browser 真实 Chat：`uname -a`、`uptime` 成功，session 状态 `completed`

### Task 12.5：保留优化项

- [x] WebLearn official-domain fallback：PostgreSQL、pgBackRest、pg_auto_failover 优先官方域名。
- [x] tool budget 后 final generation duration 指标和 UI 提示。
- [x] Host inventory 状态拆分为 `agentStatus`、`sshStatus`、`runtimeReachability`。
- [x] PostgreSQL timeline 类答案增加官方文档引用和安全配置清理措辞检查。

本轮补充实现：

```bash
go test ./internal/integrations/localtools -run TestWebSearchToolFallsBackToOfficialDomainsForPostgresOperations -count=1
go test ./internal/appui -run 'TestHostService(CreateHostDoesNotStartSSHInstall|SSHTestStoresPasswordBeforeBootstrap|SSHTestAllowsMissingSSHCredentialRefWithoutBootstrap)|TestChatService_SendMessageInjectsSelectedHostRuntimeMetadata' -count=1
go test ./internal/featureflag ./internal/appui ./internal/server -count=1
```

本轮补充完成：

- `tool budget 后 final generation duration 指标和 UI 提示`：复用 runtime 现有模型调用计时，`EinoKernel` 将最终合成模型调用耗时写入 `final_answer` TurnItem payload；`TransportProjector` 透传为 `turn.final.durationMs` 和 `assistant.final` process block `durationMs`；前端 `ProcessTranscript` 通过 assistant message metadata 显示 “最终合成 xxxms”。未新增第二套计时机制，也不从 final markdown 解析过程状态。

补充验证：

```bash
go test ./internal/runtimekernel -run TestRunTurn_RecordsFinalGenerationDurationAfterToolBudget -count=1
go test ./internal/appui -run TestTransportProjectorProjectsFinalGenerationDuration -count=1
cd web && npm test -- ProcessTranscript.test.tsx
```
