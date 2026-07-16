# AIOps v2 AI Chat 输出分层与时间线降噪优化实施 TODO 清单

> 状态：待实施
>
> 编写日期：2026-07-15
>
> 原始分析基线：`765a1c7`
>
> 合并后复核日期：2026-07-16
>
> 当前实施基线：`2d37eb1`（完整提交：`2d37eb1b2faae45fd47b260cc94b4f5ba2094c75`）
>
> 适用链路：`TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React`
>
> 实施约束：每个非机械性 commit 最多 5 个生产文件、生产代码增删总量不超过 500 行；每批独立完成 red -> green、生产链路验证和 diff 审查。

## 0. 2026-07-16 合并后复核结论

### 0.1 代码差异结论

以原始分析基线 `765a1c7` 对比当前实施基线 `2d37eb1`：

- [x] `internal/runtimekernel`、`internal/appui`、`web/src/chat`、`web/src/transport` 均无差异，本文记录的 Chat runtime、transport 和 UI 根因仍然成立。
- [x] 合并内容主要是 hardening CI 可复现性、change-budget/harness 自测、self-opt 未跟踪目录收敛、README、HostsPage cache 测试和本文档；没有已经完成本文任一实施 Phase。
- [x] `HostsPage.cache.test.tsx` 与 Chat 输出链路无关，不纳入本计划范围。
- [x] `ProcessTranscript` 的 `agentSteps` fallback 当前没有生产调用者，不是本次混乱来源；本计划不顺手清理该非生产路径。

复核命令：

```bash
git diff --quiet 765a1c7..2d37eb1 -- \
  internal/runtimekernel internal/runtimecontract internal/appui \
  web/src/chat web/src/transport
```

期望退出码为 `0`。实施前如果 HEAD 已不是 `2d37eb1`，必须重新执行该差异复核并更新本节，不能直接沿用下面的文件边界。

### 0.2 合并后新增的强制门禁

- [ ] 本地与 CI 都安装 `rg`、`python3`、`go.mod` 指定的 Go、Node.js 22，并在 `web/` 执行 `npm ci`。
- [ ] 每个 commit 单独满足最多 5 个生产文件、最多 500 行生产代码 churn；测试、fixture 不计入生产文件预算，但仍需审查。
- [ ] runtime 生产代码有变化时，同一 review range 必须修改一个包含 `RunTurn(` 或 `AssistantTransport` 的 story/integration test。
- [ ] `web/src` 可见 UI 有变化时，同一 review range 必须修改 `web/tests` 下包含 `toHaveScreenshot(...)` 的 Playwright spec。
- [ ] snapshot/golden baseline 变化必须单独审查并在已提交 commit 中声明 `baseline` trailer；未提交的 baseline 漂移会被 gate 拒绝。
- [ ] `docs/**` 也会触发 `aiops-v2-hardening` workflow，本文档更新不能绕过正式门禁。

每个 Phase 开始时固定 review base：

```bash
export AIOPS_HARNESS_BASE_REF=<该 Phase 开始前的提交>
scripts/test-aiops-harness-contract-boundaries.sh
scripts/test-aiops-change-budget.sh
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
```

如果确需提交已审查的 screenshot/golden baseline，commit message 必须包含：

```text
AIOps-Change-Exception: baseline
AIOps-Change-Reason: <为什么该可见契约变化符合本计划>
AIOps-Change-Review: <baseline diff 审查记录或设计文档>
```

`mechanical|baseline` exception 只用于真正的机械变更或已审查 baseline，不能为超预算的行为修改提供豁免。

## 1. 目标

把当前 AI Chat 从“运行事件流水账”调整为清晰的四层输出：

```text
单一运行状态
  -> 可选的短 commentary
  -> 紧凑、可展开的 action/tool 过程
  -> 唯一 final answer
```

完成后应满足：

- [ ] 模型尚未完成分类的流式草稿不得作为 `final_answer` 出现在聊天中。
- [ ] `model_call`、等待模型和 reasoning delta 保留在 runtime/trace 中，但不作为普通聊天过程块展示。
- [ ] commentary、tool、approval、artifact、final 按第一次实际发生顺序进入 `blockOrder`，不能再按类型分桶重排。
- [ ] 同一工具调用只保留一个稳定生命周期块；工具意图与工具块通过 typed identity 归组，不按文案去重。
- [ ] 正常完成的 turn 只展示一个最终答案；成功态 FinalContract 不再额外占一张摘要卡。
- [ ] retry、stream error、cancel、approval blocked 等边界仍保留可解释、可审计的输出。
- [ ] 不削弱 evidence gate、permission、approval、ToolDispatcher、post-check 和 FinalContract 安全语义。

## 2. 背景与已确认根因

### 2.1 未分类的流式正文被提前标记为 final

当前 `internal/runtimekernel/runtime_kernel.go` 在收到每个 assistant delta 时，立即写入：

```text
assistant_message
phase=final_answer
streamState=streaming
```

只有 provider response 完成、确认存在 tool calls 后，`commitAssistantOutputForIteration` 才把同一 item 改成 commentary。用户可能看到：

```text
final 草稿出现
  -> 草稿移动到过程区
  -> 工具执行
  -> 新 final 草稿出现
  -> gate retry
  -> 最终答案替换
```

这不是前端偶发现象，而是当前 runtime contract 和测试明确允许的行为。

合并后复核还发现两处必须一起修正的契约分叉：

- 真正持久化的 AgentItem 是 `phase=final_answer + streamState=streaming`，但相邻 debug fields 却写成 `phase=unclassified + streamState=accumulating`；当前 trace 描述与真实 payload 不一致。
- retry helper 和 stream-error 路径也会把未完成草稿硬编码为 `final_answer`；不能只改正常 delta callback。

### 2.2 runtime trace item 被直接升级成聊天内容

每轮模型调用都会产生 `model_call` TurnItem；reasoning summary 会持续写入该 item。`transport_projector.go` 又把它投影为 `kind=reasoning` 的 process block，前端在 turn 运行时自动展开过程区，因此内部运行状态与用户对话混在一起。

### 2.3 `blockOrder` 不是跨类型的真实发生顺序

当前 `projectCanonicalTransportBlocks` 的顺序是：

```text
全部 turn.Process
  -> turn.Final
  -> 全部 turn.AgentUIArtifacts
```

因此从 tool result 生成的 artifact 即使早于 final，也可能被移动到 final 后面。这个顺序是类型分桶顺序，不是事件时间线。

### 2.4 commentary 和工具卡表达同一件事

工具轮通常同时产生：

1. 模型 prelude 或 runtime 生成的 tool intent commentary；
2. tool call/result 过程块；
3. 下一轮 commentary。

目前只有连续 search 和 command 会归组；file、tool、MCP、evidence、commentary 仍可能逐项平铺。

### 2.5 final 有多套可见语义

当前用户可能同时看到：

- FinalContract status/confidence/evidence count；
- final answer 正文；
- RCA、OpsManual、Coroot 等 artifact；
- 与正文重复的 evidence/process 描述。

此外，runtime 同时写 `assistant_message phase=final_answer` 和 `final_response`，transport 当前会让后出现的 `final_response` 再次成为 final 来源，弱化了“assistant message 是唯一正文 item”的边界。

正常完成主要通过 `commitFinalAssistantOutput`，但 approval-denied terminal path 仍在 turn loop 内直接重复写 assistant message 和 `final_response`。目标应是“唯一 typed terminal commit boundary”，不能只假设已经有一个函数覆盖所有终态。

### 2.6 前后端仍存在文本修复式判断

当前代码在 commentary 接纳、evidence-constrained/fallback final 清理路径中，通过“根因、结论、证据、PostgreSQL、WAL”等业务文本标记判断内容；前端还会识别“泄漏的工具过程文本”、结构化 JSON、文本相等和段落长度，再替换或去重用户可见内容。

这些规则只能掩盖协议问题，不能作为长期输出契约。

但 finish reason、未闭合 delimiter/code fence、raw tool-call markup、secret 和危险命令等通用语法或安全边界不是业务词法补丁，必须保留并继续测试。

## 3. 必须保持不变的行为

- [ ] `runtimekernel` 仍是唯一 turn lifecycle driver。
- [ ] 工具执行仍走 `ToolDispatcher -> policy/permission/approval -> tool_result`。
- [ ] React Chat 仍只消费 `aiops.transport.v2` 的 `blockOrder + blocksById`。
- [ ] 不新增 page-local SSE、WebSocket、EventSource、legacy reducer 或第二套 transcript store。
- [ ] 不从 final Markdown/text 推导 process、approval、artifact 或执行状态。
- [ ] 同一 tool call/result 继续使用稳定 block id 原位更新。
- [ ] 运行中断时保留已经生成的有效部分，但不能把未通过 final gate 的草稿宣称为最终结论。
- [ ] 历史 transport state 仍可通过现有 compatibility boundary 读取；不在生产投影主路径长期保留第二套旧逻辑。
- [ ] 不增加任何中间件、厂商、服务名、主机名或故障 case 的核心硬编码。

## 4. 目标输出契约

### 4.1 Runtime assistant message phase

新增 runtime-only phase：

```go
const AssistantMessagePhaseUnclassified AssistantMessagePhase = "unclassified"
```

约束：

- [ ] provider 只返回普通文本 delta、尚不知道是否存在 tool calls 时，写 `phase=unclassified`。
- [ ] `unclassified` item 可持久化到 TurnSnapshot，用于恢复、trace 和错误诊断，但不得进入 Chat `blockOrder`。
- [ ] provider response 含 tool calls 时，同一 item 原位完成为 `phase=commentary`，或按结构化 commentary budget 生成 runtime tool intent。
- [ ] provider response 不含 tool calls 时，继续保持不可见，直到 final gate、evidence boundary 和 retry 决策完成。
- [ ] direct answer、evidence gate、approval denied 等所有终态收敛到唯一 typed terminal commit boundary；该 boundary 才能把 item 完成为 `phase=final_answer` 并同步 `final_response`。
- [ ] retry、stream error、cancel 的未完成草稿保持 `phase=unclassified + streamState=incomplete`（或另一个明确的 typed draft phase），不得借用 `final_answer` 表示“曾经像答案”。
- [ ] provider 未来若能提供可信原生 phase，可另行设计显式 final streaming；本次不通过猜测恢复 final streaming。

### 4.2 Chat 可见性

| TurnItem / 状态 | Chat 展示 | Trace / 审计 |
| --- | --- | --- |
| `model_call running/completed` | 不展示 | 保留 |
| reasoning summary delta/completed | 不展示 | 保留 |
| `assistant_message unclassified` | 不展示 | 保留 |
| `assistant_message commentary complete` | 短过程说明或 action group 标题 | 保留 |
| `assistant_message final_answer complete` | 唯一 final | 保留 |
| `assistant_message replacedByMessageId != ""` | 不作为 final/process 展示 | 保留 |
| tool call/result | 同一稳定过程块 | 保留 |
| approval requested/decided | 按实际位置展示 | 保留 |
| artifact | 在来源 tool/result 的位置展示 | 保留 |
| runtime error | 一条用户可读错误 | 保留原始诊断引用 |

### 4.3 Canonical block order

`blockOrder` 使用“第一次成为用户可见 block 时的位置”作为稳定顺序：

- [ ] 第一次出现 block id 时 append 到 `blockOrder`。
- [ ] 同 id 的 running -> completed、tool_call -> tool_result 只更新 `blocksById[id]`，不移动位置。
- [ ] artifact 在生成它的 tool result 之后插入，不统一移动到 turn 尾部。
- [ ] final 只在 final commit 后插入一次。
- [ ] retry 草稿、unclassified item 和 trace-only item 不占用 `blockOrder` 位置。

示例：

```text
commentary-1
command-call-1        # result 原位更新
artifact-from-call-1
commentary-2
search-call-2         # result 原位更新
final-message
```

### 4.4 FinalContract 展示策略

- [ ] `verified/completed` 且没有限制、失败工具、未检查项时，不渲染独立 FinalContract 卡片。
- [ ] `partial/blocked/needs_evidence/approval_denied/tool_unavailable/failed/cancelled` 必须展示状态和用户可行动的限制。
- [ ] `unknown` 只有在包含 limitation、failed tool、unchecked requirement 等可行动详情时才展示，不能制造无信息状态卡。
- [ ] confidence、checked evidence count 继续保留在 transport contract 中，但成功态默认不抢占 final 正文视觉层级。
- [ ] artifact 与 final 各自保持 typed renderer，不把 artifact 内容复制到 final summary 卡。

## 5. 明确不做

- [ ] 不推翻 `assistant_message + phase + status + streamState` 机制。
- [ ] 不恢复 `assistant_progress`、`assistant_answer` 或独立 `final_answer` TurnItem。
- [ ] 不删除 reasoning/model_call 的 trace 和持久化数据。
- [ ] 不为了“看起来干净”使用 CSS 隐藏脏 block。
- [ ] 不按可见文本相等、关键词、长度相似度做 process-row 去重。
- [ ] 不在本次修改 provider 协议、模型选择、tool routing、permission 或 approval policy。
- [ ] 不把正常业务 artifact 全部塞进一个通用 Markdown final。
- [ ] 不以 PostgreSQL、Coroot 或 nginx 单一 case 作为核心实现判断；这些只能作为验收样例。

## 6. 分批实施总览

| 批次 | 单一行为类别 | 生产文件预算 | 完成标志 |
| --- | --- | ---: | --- |
| Phase 0 | 基线与失败测试 | 0 | 关键混乱行为均有 red test/fixture |
| Phase 1 | runtime 未分类流式消息 | <= 5 | 未确认文本不再提前成为 final |
| Phase 2 | transport 可见性与唯一 final 来源 | <= 3 | trace-only/retry item 不进入 Chat |
| Phase 3 | canonical block 真实顺序 | <= 3 | process/artifact/final 保持首次可见顺序 |
| Phase 4 | typed commentary/tool 归组 | <= 5 | 一次 action 不再显示两层重复叙述 |
| Phase 5 | final UI 与文本修复逻辑清理 | <= 4 | 正常 final 只有正文，异常 contract 仍清晰 |
| Phase 6 | 全链路验收与事实记录 | 0-1 | Go、Vitest、snapshot、browser、eval 全部有证据 |

每一阶段由一个或多个可独立审查的 commit 完成；预算 gate 会逐 commit 核算，而不是只看 Phase 汇总。不得在同一个 patch 中同时修改 runtime phase、transport ordering 和 UI 样式。

## 7. Phase 0：建立行为基线与失败测试（红测完成，截图 spec 补齐中）

### Task 0.1：记录当前生产链路基线

**Read:**

- `internal/runtimekernel/runtime_kernel.go`
- `internal/runtimekernel/assistant_output_commit.go`
- `internal/runtimekernel/assistant_message_boundary.go`
- `internal/appui/transport_projector.go`
- `web/src/transport/aiopsTransportConverter.ts`
- `web/src/chat/components/AiopsThread.tsx`
- `web/src/chat/components/ProcessTranscript.tsx`

### TODO

- [x] 记录修改前 git commit、工作树状态和相关测试结果；首个实施分支必须从 `2d37eb1` 或经重新复核的新基线开始。
- [ ] 保存一份 fixture-driven 当前行为截图，只作为 red 基线，不直接覆盖已批准 snapshot。
- [x] 记录以下当前事实：运行中的 assistant delta 被投影为 final、model_call 被投影为 reasoning、artifact 位于 final 后、成功 FinalContract 单独显示。
- [x] 如果任何事实无法稳定复现，先补 trace/fixture，不直接修改生产语义。
- [x] 记录 runtime payload 与 debug fields 对同一流式 item 的 phase/streamState 不一致事实。
- [x] 确认本 Phase 只改测试/spec；若产生 screenshot 文件，按 baseline trailer 流程单独审查和提交。

### Task 0.2：反转旧契约测试并补最小失败测试

**Modify tests only:**

- `internal/runtimekernel/react_loop_test.go`
- `internal/runtimekernel/assistant_message_contract_test.go`
- `internal/runtimekernel/agent_items_test.go`
- `internal/appui/transport_projector_test.go`
- `web/src/transport/aiopsTransportConverter.test.ts`
- `web/src/chat/components/AiopsThread.test.ts`
- `web/src/chat/components/ProcessTranscript.test.tsx`
- `web/tests/react-shell-snapshot.spec.js`

### TODO

- [x] 反转/重命名现有 `TestRunTurn_PersistsRunningFinalAssistantItemDuringStreaming`：流式草稿应为 unclassified，不再锁定 running final 旧行为。
- [x] 反转/扩展现有 `TestRunTurn_AccumulatesAssistantTextBeforeFinalCommit`：文本可累计，但 final phase 只能在 terminal commit 后出现。
- [x] 反转现有 `TestRunTurn_StreamingModelErrorPreservesIncompleteAssistantMessage`：错误草稿可恢复但不是 final。
- [x] 反转现有 `TestRunTurn_PreservesRetriedAssistantTextAsAnswerDraft` 和 `TestAssistantMessageItemHelpersPreserveRetryLifecycle`：retry draft 保留审计关系但不是 final。
- [x] 新增 `TestRunTurn_ToolPreludeNeverAppearsAsRunningFinal`，覆盖 provider 先流文本、后返回 tool calls 的路径。
- [x] 增加一个真实 `RunTurn(` 或 `AssistantTransport` story/integration case，确保 runtime patch 满足 change-budget gate 的生产链路证明。
- [x] `TestProjectTurn_ModelCallDoesNotEnterChatBlocks`。
- [x] `TestProjectTurn_UsesAssistantMessageAsOnlyTranscriptFinalSource`。
- [x] `TestProjectTurn_PreservesCommentaryToolArtifactFinalOrder`。
- [x] 反转现有 projector 测试 `TestTransportProjectorProjectsStreamingAssistantMessageFinal`、`TestTransportProjectorShowsRunningModelCallPlaceholder`、`TestTransportProjectorRewritesCanceledModelWaitText`。
- [x] 反转 converter 中 `does not duplicate a running final draft...`、`keeps assistant message id stable while final text streams in` 等依赖 running final 的断言。
- [x] 反转 ProcessTranscript 中 `renders streaming final...`、`renders model wait status...` 等将旧行为视为正确的断言。
- [x] `AiopsThread` 正常 verified final 不显示独立 contract 卡。
- [x] `ProcessTranscript` 使用 typed tool identity 归组，不按文案相等去重。
- [ ] 新增运行中、完成、retry、artifact ordering 四张 fixture snapshot。
- [x] 运行测试并确认新增断言在修复前失败；记录失败输出摘要。

### Phase 0 验收

- [x] 新测试失败原因分别指向 runtime、projection、ordering、UI owner，不能全部依赖一个大端到端断言。
- [x] 没有修改生产代码。
- [x] 没有更新 golden/snapshot 来掩盖失败。
- [x] 现有锁定旧行为的测试已显式反转或重命名，没有与新契约互相矛盾的“绿测试”。
- [ ] 新增的 Playwright spec 包含 `toHaveScreenshot(...)`；生成 baseline 前已逐张审查，且未提交工作树不会被误报为 gate 通过。

## 8. Phase 1：修复 runtime 流式阶段分类（已完成：2026-07-16）

### Task 1.1：引入 runtime-only `unclassified` phase

**Production files:**

- Modify: `internal/runtimekernel/assistant_message_contract.go`
- Create: `internal/runtimekernel/assistant_stream_classification.go`
- Modify: `internal/runtimekernel/runtime_kernel.go`
- Modify: `internal/runtimekernel/assistant_output_commit.go`
- Modify: `internal/runtimekernel/agent_items.go`

**Tests:**

- `internal/runtimekernel/assistant_message_contract_test.go`
- `internal/runtimekernel/assistant_output_commit_test.go`
- `internal/runtimekernel/agent_items_test.go`
- `internal/runtimekernel/react_loop_test.go`
- `internal/runtimekernel/cancel_turn_test.go`

### TODO

- [x] 新增 `AssistantMessagePhaseUnclassified`，明确注释它只属于 runtime/persistence，不是 Chat wire phase。
- [x] provider delta callback 将 item 写成 `phase=unclassified + streamState=streaming`。
- [x] 同步修正该 callback 的 debug fields，确保 trace 中的 phase/streamState 与真正持久化 payload 完全一致。
- [x] provider response 有 tool calls 时，把同一 item id 原位完成为 commentary。
- [x] provider response 无 tool calls 时，不在 final gate 之前改变成 `final_answer`。
- [x] final gate allow/constrain 后，通过唯一 typed terminal commit boundary 将同一 item 完成为 final。
- [x] approval-denied 等当前绕过 `commitFinalAssistantOutput` 的终态改走同一个 terminal commit boundary；不在 turn loop 复制完成逻辑。
- [x] retry_once 时，`markAssistantMessageReplacedForRetry` 将旧 item 保持为 unclassified/incomplete/replaced，仅用于审计；下一轮使用新 item id。
- [x] stream error 时，部分文本标记为 `phase=unclassified + streamState=incomplete`（或明确的新 typed draft phase），不提交 FinalOutput。
- [x] 保持 `snapshot.FinalOutput` 在 final commit 之前为空。
- [x] `runtime_kernel.go` 只替换调用点；新的分类职责必须放在小文件中，不能继续扩大核心 turn loop。
- [x] 本 Phase 只处理 phase/retry/error/terminal commit；业务关键词启发式统一留给 Phase 5，避免混合两类行为修改。

### Phase 1 验收

- [x] direct answer：流式期间 AgentItem 是 unclassified，final gate 后同一 item 才成为 final。
- [x] tool turn：任何时刻都不存在“随后又被改成 commentary”的 running final。
- [x] retry：旧草稿保留 hash、文本和 replacedBy 关系，但不成为 committed final。
- [x] cancel/error：不丢失部分输出，也不把部分输出宣称为完成结论。
- [x] approval denied：只经过一个 terminal commit boundary，并且 assistant message 与 `final_response` 不会分叉。
- [x] payload、debug fields、持久化后恢复得到的 phase/streamState 一致。
- [x] Phase 1 修改不涉及 appui 和 React。
- [x] `react_loop_test.go` 中至少一个本 Phase 修改的用例直接调用 `RunTurn(`，满足 runtime story/integration gate。

### Phase 1 验证命令

```bash
go test ./internal/runtimekernel -run 'RunningFinal|AccumulatesAssistantText|StreamingModelError|RetriedAssistantText|AssistantMessageItemHelpers|ToolPrelude|ApprovalDenied' -count=1
go test ./internal/runtimekernel -count=1
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
```

## 9. Phase 2：建立 transport 可见性策略与唯一 final 来源

> 状态：✅ 已完成（2026-07-16）。集中式 presentation policy 已落地，目标回归测试与 Phase 独立变更预算门禁通过。

### Task 2.1：集中 typed presentation policy

**Production files:**

- Create: `internal/appui/transport_presentation_policy.go`
- Modify: `internal/appui/transport_projector.go`

**Tests:**

- Create: `internal/appui/transport_presentation_policy_test.go`
- Modify: `internal/appui/transport_projector_test.go`

### TODO

- [x] 定义集中式、typed 的 Chat 投影判断，不在多个 switch 分支散落可见性规则。
- [x] `model_call running/completed` 不投影为 process block；AgentItems、Timeline、Prompt Trace 继续保留。
- [x] `model_call failed` 不额外制造第二条错误；用户错误只来自规范化 error item。
- [x] `assistant_message phase=unclassified` 不进入 `Process`、`Final` 或 `blockOrder`。
- [x] 扩展 `assistantMessageProjectionPayload`，typed 解析 `boundaryAction`、`replacedByMessageId` 和必要的 stream/boundary facts；当前 projector 不能依赖尚未解析的字段做过滤。
- [x] `replacedByMessageId != ""` 或 `boundaryAction=retry_once` 的 incomplete message 不进入普通 Chat transcript。
- [x] `phase=commentary` 只投影为 commentary process block。
- [x] `phase=final_answer + completed` 是生产 transcript 的唯一 final 正文来源。
- [x] `final_response` 继续保留为 runtime/agent-run 完成事实，但当同一 turn 已有 final assistant message 时，不再覆盖 transcript final id/text。
- [x] 仅对缺少 assistant final 的历史/system turn 保留结构化 fallback；fallback 必须由 item 类型和缺失事实判断，不能按文本猜测。
- [x] 删除或停止使用 `modelCallProcessText` 生成“正在等待模型返回”Chat block 的路径。
- [x] 忙碌状态继续由 turn status 和 runtimeLiveness 驱动。
- [x] 反转现有 running final/model wait projector 测试，不能在新增测试变绿后继续保留旧契约断言。

### Phase 2 验收

- [x] Chat `blocksById` 中不存在 `kind=reasoning` 的 model wait block。
- [x] AgentItems/Prompt Trace 仍能看到 model call 和 reasoning summary。
- [x] 正常 turn 只有一个 transcript final block，id 来自 committed assistant message。
- [x] retry 草稿不会在两轮之间短暂成为 final。
- [x] system turn、历史 turn、失败 turn 的兼容测试仍通过。
- [x] Phase 2 不修改 runtime 和 React。

### Phase 2 验证命令

```bash
go test ./internal/appui -run 'TransportProjector.*(StreamingAssistantMessageFinal|ModelCall|AssistantMessage|FinalContract)|Presentation|FinalSource' -count=1
go test ./internal/appui -count=1
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
```

## 10. Phase 3：让 `blockOrder` 成为真实首次可见顺序

> 状态：✅ 已完成（2026-07-16）。首次可见顺序累加器、collision 稳定修正、审批恢复与 10 次 replay 回归均已通过。

### Task 3.1：增量构建 canonical block order

**Production files:**

- Create: `internal/appui/transport_block_order.go`
- Modify: `internal/appui/transport_projector.go`
- Modify only if contract requires: `internal/appui/transport_state.go`

**Tests:**

- Create: `internal/appui/transport_block_order_test.go`
- Modify: `internal/appui/transport_projector_test.go`

### TODO

- [x] 新增 `upsertCanonicalTransportBlock`：新 id append order，已有 id 只更新 map。
- [x] 每次投影出用户可见 process/commentary/approval/final/artifact 时立即登记 block，而不是 turn 结束后按类型拼接。
- [x] tool call 首次创建位置固定；tool result 更新同一个 block。
- [x] 在处理当前 tool result 时，通过 typed source/toolCall identity 生成并登记 artifact；不能在 turn 末尾事后遍历 `AgentUIArtifacts` 猜来源位置。
- [x] 一个 tool result 生成多个 artifact 时，按 renderer 产出顺序紧跟对应 tool block。
- [x] final 只在 committed final assistant message 处登记。
- [x] 保留 artifact id collision 处理，但 collision 修正不能改变跨类型顺序。
- [x] `projectCanonicalTransportBlocks` 改为一致性收尾：补齐遗漏 block、校验 map，不再重新排序。
- [x] replay 同一 snapshot 多次必须得到完全相同的 order。
- [x] artifact 已持久化、刷新后重投影和 approval resume 三种路径保持相同 source binding 与 order。
- [x] 不使用 `UpdatedAt` 排序，避免 running block 更新后跳到末尾。
- [x] 不使用文案、tool name 或 renderer title 推断顺序。

### 必测顺序

```text
commentary -> command call -> command result(update same id)
           -> approval -> artifact -> commentary -> search -> final
```

### Phase 3 验收

- [x] artifact 不再统一出现在 final 后面。
- [x] tool completion 不改变 tool block 的位置。
- [x] approval blocked/resume 后顺序稳定。
- [x] 相同 snapshot 连续投影 10 次的 `blockOrder` 一致。
- [x] 新鲜投影与持久化 state 重投影得到相同 `blockOrder`。
- [x] 旧 persisted transport state 在 compatibility boundary 读取时不崩溃。
- [x] Phase 3 不修改 runtime 和 React。

### Phase 3 验证命令

```bash
go test ./internal/appui -run 'TransportProjector.*(Artifact|Block)|TransportProjectionPreservesInterleaved|BlockOrder|ArtifactOrder|StableOrder|Approval' -count=10
go test ./internal/appui -count=1
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
```

## 11. Phase 4：typed commentary/tool 归组与过程降噪

### Task 4.1：扩展 process fold 语义

**Production files:**

- Create: `internal/appui/transport_process_grouping.go`
- Modify: `internal/appui/transport_projector.go`
- Modify only if typed contract fields change: `internal/appui/transport_state.go`
- Modify: `web/src/chat/components/ProcessTranscript.tsx`
- Modify only if union changes: `web/src/transport/aiopsTransportTypes.ts`

**Tests:**

- Create: `internal/appui/transport_process_grouping_test.go`
- Modify: `web/src/chat/components/ProcessTranscript.test.tsx`

### TODO

- [ ] 先锁定现状：`groupAssistantTranscriptBlocks` 只按 artifact/final 分段；`groupConsecutiveBlocks` 不使用 `foldGroupId`，只合并连续 search/command。不能把现状误记为已有通用 fold 支持。
- [ ] search、command、file、tool、MCP 使用 typed `kind/foldGroupKind/foldGroupId` 归组。
- [ ] `groupConsecutiveBlocks` 优先按相同 `foldGroupId` 建组，并校验 `foldGroupKind`；相邻但 group id 不同的 block 不得误合并。
- [ ] 迁移 `detectTransportToolBlockKind` 和 `isSearchLikeBlock` 对 tool name/`contains(file)`/search regex 的猜测，改用上游 typed `displayKind`/capability metadata。
- [ ] 不根据 tool name 包含 `file/search/coroot` 等字符串判断分组；如果上游没有足够 typed metadata，停止本 Phase 并先拆出 transport contract 任务，不能在 React 补猜测。
- [ ] 使用 `commentary.toolCallIds` 关联后续 tool blocks。
- [ ] commentary 的 `toolCallIds[]` 与工具块的 `toolCallId` 形成稳定 action group；同一 commentary 关联多个工具时保留数组顺序。
- [ ] `commentarySource=runtime_tool_intent` 且关联 tool 已存在时，将 commentary 作为 action group 标题，而不是独立大段正文。
- [ ] `commentarySource=model_prelude` 可保留为短说明，但同一 iteration 只显示一次。
- [ ] commentary 与 tool 的归组只看 typed identity，不比较两段可见文本是否相同。
- [ ] 同组多个命令/搜索/工具显示一行摘要和可展开详情。
- [ ] running group 默认展开当前 active row；完成后的旧组默认折叠。
- [ ] approval、失败、用户输入请求不能被普通 tool group 折叠掉。
- [ ] tool output preview 继续遵守 materialization/redaction 边界。

### Phase 4 验收

- [ ] 一轮单工具调用最多显示“一条 action 说明 + 一个稳定工具块”，不会重复两段相同意图。
- [ ] 连续多工具默认显示摘要，展开后仍能按原顺序查看每个动作。
- [ ] 失败工具和 approval 始终可见。
- [ ] completed turn 的过程总区继续默认折叠。
- [ ] Phase 4 不改变 runtime phase、tool dispatch 或 final contract。
- [ ] file/tool/MCP 的分组测试只使用 typed metadata；修改可见 UI 的同一 review range 已更新包含 `toHaveScreenshot(...)` 的 Playwright spec。

### Phase 4 验证命令

```bash
go test ./internal/appui -run 'ProcessGroup|FoldGroup|CommentaryTool' -count=1
(cd web && npm test -- ProcessTranscript.test.tsx AiopsThread.test.ts)
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
```

## 12. Phase 5：收敛 final UI 与删除文本修复式逻辑

### Task 5.1：FinalContract 只展示异常和可行动信息

**Production files:**

- Modify: `web/src/chat/components/AiopsThread.tsx`

**Tests:**

- Modify: `web/src/chat/components/AiopsThread.test.ts`
- Modify: `web/tests/react-shell-snapshot.spec.js`

### TODO

- [ ] verified/completed 且无异常详情时，`finalContractSummaryView` 返回 `null`。
- [ ] partial、blocked、needs_evidence、approval_denied、tool_unavailable、failed、cancelled 保持可见。
- [ ] unknown 只有存在可行动详情时显示；只有 schema/confidence 的 unknown 不占独立卡片。
- [ ] failedToolImpacts、uncheckedRequirements、limitations 有内容时保持可见。
- [ ] 不把 checked evidence count 和 confidence 复制成成功态大卡片。
- [ ] final answer 正文仍由 typed final block 渲染。
- [ ] artifact 仍由 typed artifact renderer 渲染，不拼进 final Markdown。

### Task 5.2：删除前端 final 文本修复器

**Production files:**

- Modify: `web/src/chat/components/AiopsThread.tsx`
- Modify: `web/src/chat/components/ProcessTranscript.tsx`
- Modify: `web/src/transport/aiopsTransportConverter.ts`
- Modify only if shared sanitizer is extracted: `web/src/chat/components/MessageMarkdown.tsx`

**Tests:**

- Modify: `web/src/chat/components/AiopsThread.test.ts`
- Modify: `web/src/chat/components/ProcessTranscript.test.tsx`
- Modify: `web/src/transport/aiopsTransportConverter.test.ts`
- Modify: `web/tests/react-shell-snapshot.spec.js`

### TODO

- [ ] 删除通过“let me、evidence refs、RCA 上下文”等业务文本判断工具过程泄漏的逻辑。
- [ ] 删除从 final 文本中的 JSON 行猜测 evidence/artifact 类型的逻辑。
- [ ] 删除按最终可见行文本做 Set 去重的逻辑。
- [ ] 删除 converter 的 `isDuplicateRunningFinalDraft` 文本相等去重和 `latestSubstantialAssistantProcessText` 长度/段落阈值回退；partial/retry/error 可见性改由 typed phase、streamState、boundary facts 决定。
- [ ] converter 的 raw stream error 字符串识别必须改为 typed normalized error；若历史兼容暂不能删除，只能留在明确的 compatibility boundary，并添加退出条件和旧 fixture 测试。
- [ ] 删除或 typed 化 `ProcessTranscript` 的 `isCorootInternalReuseBlock`、`isRuntimeInternalGateText`、`isRiskyOperationalAdviceText`、`isSearchLikeBlock` 文本/正则分流。
- [ ] 危险操作提示和 secret/redaction 安全语义必须迁移到 typed policy/metadata 后再删除文本判断，不能用“清理启发式”为由削弱安全边界。
- [ ] 保留 Markdown 渲染、链接安全、通用 secret redaction 等机器边界能力。
- [ ] 如果清理后暴露脏 final，回到 Phase 1/2 修 runtime/projection，不在 React 增加新补丁。

### Task 5.3：清理相关 backend 内容启发式

**Production files:**

- Modify: `internal/runtimekernel/assistant_message_boundary.go`
- Modify: `internal/runtimekernel/assistant_output_commit.go`
- Modify only if structured facts need completion: `internal/runtimekernel/assistant_message_contract.go`
- Modify: `internal/appui/transport_projector.go`

### TODO

- [ ] commentary 是否接纳只依据 phase、tool calls 和通用展示预算，不依据业务关键词。
- [ ] pending tool/process intent 由 provider tool calls、finish reason、checkpoint 或 structured boundary fact 表达。
- [ ] 删除 PostgreSQL、WAL、pgBackRest 等产品词对 final 完整性的决定权。
- [ ] 删除“包含根因/证据/下一步即可认为 concrete final”的通用核心规则。
- [ ] 删除 projector `sanitizeUserVisibleProcessText` 对 internal gate marker 的 substring 隐藏；上游 typed visibility policy 必须阻止 internal-only item 进入 Chat。
- [ ] 明确业务词法主要位于 commentary 与 evidence-constrained/fallback 清理路径，不误删 `final_completeness_gate.go` 的 finish reason、delimiter、code fence 等通用语法完整性校验。
- [ ] raw tool-call markup、危险操作和 secret 检测继续作为机器/安全边界保留。
- [ ] 为通用咨询、文件分析、主机运维、Web 搜索、数据库 RCA 各加正反例，证明没有单一领域偏置。

### Phase 5 验收

- [ ] 正常回答视觉上只有 final 正文，不再先显示“已验证/置信度高/已采集 N 条证据”卡片。
- [ ] 异常和受限回答仍清晰说明缺失证据、失败工具和限制。
- [ ] React 不再修补泄漏的工具过程文本；上游测试保证该文本不能成为 final。
- [ ] 核心 runtime 不使用业务关键词判断 commentary/final。
- [ ] Phase 5 的 UI 与 runtime 清理分成至少两个独立提交，不混在同一个 patch。
- [ ] 每个 UI commit 都修改包含 `toHaveScreenshot(...)` 的 Playwright spec；backend runtime commit 修改包含 `RunTurn(` 或 `AssistantTransport` 的 story/integration test。

### Phase 5 验证命令

```bash
go test ./internal/runtimekernel -run 'AssistantMessageBoundary|Commentary|Final' -count=1
(cd web && npm test -- aiopsTransportConverter.test.ts AiopsThread.test.ts ProcessTranscript.test.tsx)
(cd web && npm run typecheck && npm run build)
(cd web && npm run test:ui:snapshots)
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
```

## 13. Phase 6：全链路回归、浏览器验收和事实记录

### Task 6.1：完整故事链验证

必须覆盖：

```text
ChatCommand
  -> appui admission
  -> RuntimeKernel
  -> ToolDispatcher/checkpoint
  -> FinalContract
  -> AiopsTransportState
  -> AssistantTransport
  -> assistant-ui React
```

### TODO

- [ ] 无工具直接回答：只显示运行状态，完成后出现一个 final。
- [ ] 单工具读取：一个短 action group、一个工具生命周期块、一个 final。
- [ ] 多轮 command + search：过程紧凑，展开顺序正确，final 唯一。
- [ ] artifact turn：artifact 位于来源 tool 后，final 位于真实提交位置。
- [ ] approval blocked/resume：approval 不被折叠，resume 后 block id 和顺序不变。
- [ ] retry_once：旧草稿从未作为用户 final 闪现，新 final 提交一次。
- [ ] stream error：错误只展示一次；有效部分输出可恢复但明确不是已验证 final。
- [ ] cancel：停止后不残留“正在等待模型返回”。
- [ ] failed tool：工具失败、影响和最终受限结论互不重复但都可追踪。
- [ ] context compaction：状态提示不复制进 final/process。

### Task 6.2：量化验收指标

- [ ] 每个 completed turn 的 `final_answer` block 数量必须等于 1；失败且无答案的 turn 可以为 0。
- [ ] Chat `blockOrder` 中 `model_call/reasoning wait` block 数量必须等于 0。
- [ ] 同一 toolCallId 在普通 transcript 中最多对应一个 tool block。
- [ ] `runtime_tool_intent` commentary 不得与关联工具形成两个独立大段正文。
- [ ] 被替换 draft 的可见 block 数量必须等于 0。
- [ ] artifact 的 block index 必须大于来源 tool index，并保持小于其后发生的 final index。
- [ ] 正常 verified final 的独立 contract summary 数量必须等于 0。
- [ ] 同一 fixture 重放 10 次，`blockOrder` 和 screenshot 均稳定。

### Task 6.3：自动化验证命令

```bash
export AIOPS_HARNESS_BASE_REF=<实施分支的 review base，首轮默认 2d37eb1>

scripts/test-aiops-harness-contract-boundaries.sh
scripts/test-aiops-change-budget.sh
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh

go test ./internal/runtimekernel ./internal/appui ./internal/server -count=1
go test ./... -count=1

(cd web && npm test -- aiopsTransportConverter.test.ts AiopsThread.test.ts ProcessTranscript.test.tsx ChatTransportProvider.test.tsx)
(cd web && npm test -- --run)
(cd web && npm run typecheck && npm run build)
(cd web && npm run test:ui:snapshots)

scripts/aichat-harness-hardening-gate.sh
```

说明：总入口会再次执行 change-budget/boundary 扫描、runtime/appui/server 故事链和 `go test ./...`。这里保留前置自测与定向命令，是为了让失败能够归属到具体 owner，而不是只留下一个总 gate 失败。

Structured streaming 边界扫描：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "snapshot\.toolInvocations|store\.runtime\.turn|processItemsByTurnId|phaseFoldsByTurnId" web/src
rg -n "JSON\.parse\(|markdown heading|summary.*steps.*actions" web/src
```

内容启发式清理扫描：

```bash
rg -n "containsAnyAssistantFinalMarker|finalMessageHasProcessIntent|containsConcreteFinalContent|looksLikeLeakedToolProcessText|readableStructuredEvidenceLine|isDuplicateRunningFinalDraft|latestSubstantialAssistantProcessText|isCorootInternalReuseBlock|isRuntimeInternalGateText|isRiskyOperationalAdviceText|isSearchLikeBlock|sanitizeUserVisibleProcessText" \
  internal/runtimekernel internal/appui web/src/chat web/src/transport
```

期望：

- [ ] structured streaming 前两条扫描在 React Chat 生产路径无命中。
- [ ] final/process UI 不从 final 文本解析结构。
- [ ] 上述业务内容启发式函数已删除，或只剩明确记录、非生产迁移测试引用。
- [ ] 工具分组不通过 tool name、产品名或 command regex 猜 kind；typed metadata 缺失会 fail closed 或保留为未分组 tool。

### Task 6.4：impact 与 P0 regression 证据

```bash
./scripts/self-optimization-lab.sh \
  --standalone \
  --real-aiops-tests \
  --priority P0 \
  --fail-on-regression \
  --max-runs 1 \
  --no-asset-draft
```

- [ ] 使用默认 `testdata/eval_cases` 和 `testdata/self_optimization/eval_cases`；如果 review 使用专用 case，显式传入 `--core-cases`/`--synthetic-cases` 并记录路径。
- [ ] 审查 `.data/self-optimization-lab` 下本轮 `impact-matrix.json` 与 `scorecard.json`，确认 chat-ui/transport/runtime 影响与实际修改一致且无 P0 regression。
- [ ] `.data` 结果只作为交付证据，不提交生成物；真实 provider 不可用时明确记录，不把 offline/mock 结果写成真实模型通过。

### Task 6.5：浏览器与真实 provider 验收

- [ ] 使用 fixture-driven browser 流程审查 running、completed、retry、approval、artifact 五类截图。
- [ ] 使用真实 provider 连续运行至少 3 次“普通咨询 + 一次工具 + 多次工具”场景。
- [ ] 记录每次 final block 数、process group 数、tool block 数和 artifact 顺序。
- [ ] 检查浏览器 console 无 error/warning 回归。
- [ ] 检查刷新、历史恢复后 transcript 与运行结束时一致。
- [ ] 真实 provider 不可用时明确列为未验证，不用 mock 冒充完成。

### Task 6.6：重大 Bug 事实记录

**Modify:**

- `fixbug.md`

### TODO

- [ ] 写入本地修复时间。
- [ ] 记录用户可见现象和影响范围。
- [ ] 记录已确认根因：提前 final、trace 过度投影、类型分桶顺序、重复语义层。
- [ ] 记录实际修改文件和关键契约。
- [ ] 关联最小回归用例、对应 `RunTurn`/`AssistantTransport` harness/story case 和 fixture-driven screenshot spec。
- [ ] 记录本轮真实运行且 exit code 为 0 的验证命令。
- [ ] 记录未运行项、剩余风险和真实 provider 结论。
- [ ] 不写 secret、客户敏感内容或完整高风险命令输出。
- [ ] 复核 `README.md` 中“支持流式回答”的描述：若 final 改为 terminal gate 后一次可见，改成“流式运行状态/工具过程 + 最终结论分区”；开发硬规则仍只维护在 `AGENTS.md`，不复制进 README。

## 14. 兼容与迁移策略

- [ ] 保持 wire schemaVersion 为 `aiops.transport.v2`；本次不新增第二个 transport version。
- [ ] `unclassified` 是 runtime-only phase，不加入前端 `AiopsAssistantMessagePhase` union。
- [ ] 新投影生成真实 `blockOrder`；已有持久化 state 由现有 compatibility boundary 读取，不原地重写历史文件。
- [ ] 历史 `final_response` fallback 仅在不存在 final assistant message 时启用。
- [ ] 稳定 block id 规则不变，避免刷新后 React key 改变。
- [ ] UI snapshot 变化必须逐张审查；只接受与本计划目标直接相关的 baseline diff，并使用 `AIOps-Change-Exception: baseline`、非空 reason/review trailers 提交。
- [ ] baseline 文件不能停留在未提交工作树中宣称通过 change-budget gate；先审查 diff，再按声明流程提交，最后从固定 review base 重跑门禁。
- [ ] 如果线上数据证明需要历史迁移，另写一次性 migration 任务，不把长期兼容分支塞回生产 projector。

## 15. 风险与停止条件

出现以下任一条件时停止当前 patch，回到设计评审：

- [ ] 一次修改需要超过 5 个生产文件或 500 行非机械性代码。
- [ ] 为修复展示必须新增第二套 transcript、stream 或 final selector。
- [ ] 需要通过 CSS、文本正则或工具名关键字隐藏 block。
- [ ] 同一现象已经进行两次定向补丁仍复发。
- [ ] 无法指出 runtime phase、transport visibility、block ordering、React rendering 中唯一 owner。
- [ ] 目标文件存在来源不明且与任务重叠的未提交修改。
- [ ] snapshot 大面积变化且无法逐项解释。
- [ ] `scripts/check-aiops-harness-contract-boundaries.sh` 或 `scripts/check-aiops-change-budget.sh` 失败且无法归属到当前 Phase。
- [ ] runtime production patch 没有 `RunTurn`/`AssistantTransport` 故事证明，或 UI production patch 没有 `toHaveScreenshot(...)` 覆盖。
- [ ] approval、permission、evidence gate 或 ToolDispatcher 行为发生非预期变化。
- [ ] 新方案要求把 trace/projection 反向变成 runtime 决策来源。

## 16. 最终完成清单

### Runtime

- [ ] 未分类流式文本不再提前成为 final。
- [ ] commentary/final 由结构化 response facts 和 final commit 决定。
- [ ] retry、error、cancel 不丢文本，也不误报完成。
- [ ] 核心输出阶段判断不依赖业务关键词。

### Transport

- [ ] Chat 不展示 model wait/reasoning trace item。
- [ ] assistant_message 是唯一 transcript final 正文来源。
- [ ] replaced/incomplete retry draft 不进入普通 transcript。
- [ ] `blockOrder` 是首次可见顺序，并可稳定重放。
- [ ] tool call/result 原位更新；artifact 保持来源位置。

### Frontend

- [ ] running turn 过程简洁、当前动作清楚。
- [ ] completed turn 过程默认折叠。
- [ ] commentary 与工具按 typed identity 归组。
- [ ] 正常 final 不重复显示 contract 状态卡。
- [ ] 异常 final 的限制、失败工具和未检查项仍清楚。
- [ ] 不从 final 文本猜测结构化 UI。

### 验证与交付

- [ ] 新增测试经历 red -> green。
- [ ] Go 定向测试、Go 全量测试通过。
- [ ] Vitest、build、Playwright snapshot 通过。
- [ ] boundary self-test/scan、change-budget self-test/gate 和 `aichat-harness-hardening-gate.sh` 全部通过。
- [ ] 每个 commit 均在 5 个生产文件/500 行预算内；baseline commit 的 exception/reason/review trailers 已审查。
- [ ] browser 完整故事链通过。
- [ ] self-opt P0 regression 无回归，并审查 `impact-matrix.json`、`scorecard.json`。
- [ ] 真实 provider 验收完成或明确标记未验证。
- [ ] `fixbug.md` 已追加事实记录。
- [ ] `README.md` 的流式能力描述与最终可见行为一致，且未复制 `AGENTS.md` 的硬规则。
- [ ] 报告实际修改文件、变更行数、测试命令、未运行项和剩余风险。
- [ ] `git diff` 只包含本计划当前阶段的必要修改。

## 17. 参考资料

- `docs/2026-06-25-aiops-v2-agent-runtime-output-flow-debug.zh.md`
- `docs/2026-06-25-codex-agent-runtime-output-flow-compare.zh.md`
- `docs/2026-06-26-aiops-v2-agent-runtime-single-assistant-message-design.zh.md`
- `docs/superpowers/plans/2026-06-23-aiops-v2-codex-like-ai-chat-runtime-v2-implementation-todo.zh.md`
- `internal/runtimekernel/assistant_message_contract.go`
- `internal/runtimekernel/assistant_output_commit.go`
- `internal/runtimekernel/runtime_kernel.go`
- `internal/appui/transport_projector.go`
- `web/src/transport/aiopsTransportConverter.ts`
- `web/src/chat/components/AiopsThread.tsx`
- `web/src/chat/components/ProcessTranscript.tsx`
