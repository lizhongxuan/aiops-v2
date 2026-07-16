# AIOps v2 AI Chat 输出分层与时间线降噪优化实施 TODO 清单

> 状态：待实施
>
> 编写日期：2026-07-15
>
> 基线提交：`765a1c7`
>
> 适用链路：`TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React`
>
> 实施约束：每一批非机械性修改最多 5 个生产文件、生产代码增删不超过 500 行；每批独立完成 red -> green、生产链路验证和 diff 审查。

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

### 2.6 前后端仍存在文本修复式判断

当前代码通过“根因、结论、证据、PostgreSQL、WAL”等文本标记判断 commentary/final 完整性，并在前端识别“泄漏的工具过程文本”或结构化 JSON，再替换成另一段用户文案。

这些规则只能掩盖协议问题，不能作为长期输出契约。

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
- [ ] 只有 `commitFinalAssistantOutput` 可以把 item 完成为 `phase=final_answer`。
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
- [ ] `partial/blocked/needs_evidence/approval_denied/tool_unavailable/failed` 必须展示状态和用户可行动的限制。
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
| Phase 4 | typed commentary/tool 归组 | <= 4 | 一次 action 不再显示两层重复叙述 |
| Phase 5 | final UI 与文本修复逻辑清理 | <= 4 | 正常 final 只有正文，异常 contract 仍清晰 |
| Phase 6 | 全链路验收与事实记录 | 0-1 | Go、Vitest、snapshot、browser、eval 全部有证据 |

每一阶段必须独立提交和验收。不得在同一个 patch 中同时修改 runtime phase、transport ordering 和 UI 样式。

## 7. Phase 0：建立行为基线与失败测试

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

- [ ] 记录修改前 git commit、工作树状态和相关测试结果。
- [ ] 保存一份 fixture-driven 当前行为截图，只作为 red 基线，不直接覆盖已批准 snapshot。
- [ ] 记录以下当前事实：运行中的 assistant delta 被投影为 final、model_call 被投影为 reasoning、artifact 位于 final 后、成功 FinalContract 单独显示。
- [ ] 如果任何事实无法稳定复现，先补 trace/fixture，不直接修改生产语义。

### Task 0.2：新增最小失败测试

**Modify tests only:**

- `internal/runtimekernel/react_loop_test.go`
- `internal/runtimekernel/assistant_message_contract_test.go`
- `internal/appui/transport_projector_test.go`
- `web/src/transport/aiopsTransportConverter.test.ts`
- `web/src/chat/components/AiopsThread.test.ts`
- `web/src/chat/components/ProcessTranscript.test.tsx`
- `web/tests/react-shell-snapshot.spec.js`

### TODO

- [ ] `TestRunTurn_StreamingDeltaRemainsUnclassifiedUntilResponseShapeKnown`。
- [ ] `TestRunTurn_ToolPreludeNeverAppearsAsRunningFinal`。
- [ ] `TestRunTurn_RetryDraftNeverBecomesCommittedFinalBlock`。
- [ ] `TestProjectTurn_ModelCallDoesNotEnterChatBlocks`。
- [ ] `TestProjectTurn_UsesAssistantMessageAsOnlyTranscriptFinalSource`。
- [ ] `TestProjectTurn_PreservesCommentaryToolArtifactFinalOrder`。
- [ ] `AiopsThread` 正常 verified final 不显示独立 contract 卡。
- [ ] `ProcessTranscript` 使用 typed tool identity 归组，不按文案相等去重。
- [ ] 新增运行中、完成、retry、artifact ordering 四张 fixture snapshot。
- [ ] 运行测试并确认新增断言在修复前失败；记录失败输出摘要。

### Phase 0 验收

- [ ] 新测试失败原因分别指向 runtime、projection、ordering、UI owner，不能全部依赖一个大端到端断言。
- [ ] 没有修改生产代码。
- [ ] 没有更新 golden/snapshot 来掩盖失败。

## 8. Phase 1：修复 runtime 流式阶段分类

### Task 1.1：引入 runtime-only `unclassified` phase

**Production files:**

- Modify: `internal/runtimekernel/assistant_message_contract.go`
- Create: `internal/runtimekernel/assistant_stream_classification.go`
- Modify: `internal/runtimekernel/runtime_kernel.go`
- Modify: `internal/runtimekernel/assistant_output_commit.go`
- Modify: `internal/runtimekernel/assistant_message_boundary.go`

**Tests:**

- `internal/runtimekernel/assistant_message_contract_test.go`
- `internal/runtimekernel/assistant_output_commit_test.go`
- `internal/runtimekernel/react_loop_test.go`

### TODO

- [ ] 新增 `AssistantMessagePhaseUnclassified`，明确注释它只属于 runtime/persistence，不是 Chat wire phase。
- [ ] provider delta callback 将 item 写成 `phase=unclassified + streamState=streaming`。
- [ ] provider response 有 tool calls 时，把同一 item id 原位完成为 commentary。
- [ ] provider response 无 tool calls 时，不在 final gate 之前改变成 `final_answer`。
- [ ] final gate allow/constrain 后，由 `commitFinalAssistantOutput` 将同一 item 完成为 final。
- [ ] retry_once 时，旧 item 标记 incomplete/replaced，仅用于审计；下一轮使用新 item id。
- [ ] stream error 时，部分文本标记为 incomplete partial，不提交 FinalOutput。
- [ ] 保持 `snapshot.FinalOutput` 在 final commit 之前为空。
- [ ] 把 commentary 接纳规则收敛为通用展示预算，例如最大字符数和换行数；删除“根因、结论、证据”等业务词表判断。
- [ ] 保留 raw tool-call markup、secret、危险命令等机器/安全边界校验，不把它们误删为普通内容启发式。
- [ ] `runtime_kernel.go` 只替换调用点；新的分类职责必须放在小文件中，不能继续扩大核心 turn loop。

### Phase 1 验收

- [ ] direct answer：流式期间 AgentItem 是 unclassified，final gate 后同一 item 才成为 final。
- [ ] tool turn：任何时刻都不存在“随后又被改成 commentary”的 running final。
- [ ] retry：旧草稿保留 hash、文本和 replacedBy 关系，但不成为 committed final。
- [ ] cancel/error：不丢失部分输出，也不把部分输出宣称为完成结论。
- [ ] Phase 1 修改不涉及 appui 和 React。

### Phase 1 验证命令

```bash
go test ./internal/runtimekernel -run 'AssistantMessage|StreamingDelta|ToolPrelude|RetryDraft' -count=1
go test ./internal/runtimekernel -count=1
```

## 9. Phase 2：建立 transport 可见性策略与唯一 final 来源

### Task 2.1：集中 typed presentation policy

**Production files:**

- Create: `internal/appui/transport_presentation_policy.go`
- Modify: `internal/appui/transport_projector.go`

**Tests:**

- Create: `internal/appui/transport_presentation_policy_test.go`
- Modify: `internal/appui/transport_projector_test.go`

### TODO

- [ ] 定义集中式、typed 的 Chat 投影判断，不在多个 switch 分支散落可见性规则。
- [ ] `model_call running/completed` 不投影为 process block；AgentItems、Timeline、Prompt Trace 继续保留。
- [ ] `model_call failed` 不额外制造第二条错误；用户错误只来自规范化 error item。
- [ ] `assistant_message phase=unclassified` 不进入 `Process`、`Final` 或 `blockOrder`。
- [ ] `replacedByMessageId != ""` 或 `boundaryAction=retry_once` 的 incomplete message 不进入普通 Chat transcript。
- [ ] `phase=commentary` 只投影为 commentary process block。
- [ ] `phase=final_answer + completed` 是生产 transcript 的唯一 final 正文来源。
- [ ] `final_response` 继续保留为 runtime/agent-run 完成事实，但当同一 turn 已有 final assistant message 时，不再覆盖 transcript final id/text。
- [ ] 仅对缺少 assistant final 的历史/system turn 保留结构化 fallback；fallback 必须由 item 类型和缺失事实判断，不能按文本猜测。
- [ ] 删除或停止使用 `modelCallProcessText` 生成“正在等待模型返回”Chat block 的路径。
- [ ] 忙碌状态继续由 turn status 和 runtimeLiveness 驱动。

### Phase 2 验收

- [ ] Chat `blocksById` 中不存在 `kind=reasoning` 的 model wait block。
- [ ] AgentItems/Prompt Trace 仍能看到 model call 和 reasoning summary。
- [ ] 正常 turn 只有一个 transcript final block，id 来自 committed assistant message。
- [ ] retry 草稿不会在两轮之间短暂成为 final。
- [ ] system turn、历史 turn、失败 turn 的兼容测试仍通过。
- [ ] Phase 2 不修改 runtime 和 React。

### Phase 2 验证命令

```bash
go test ./internal/appui -run 'Presentation|ModelCall|AssistantMessage|FinalSource' -count=1
go test ./internal/appui -count=1
```

## 10. Phase 3：让 `blockOrder` 成为真实首次可见顺序

### Task 3.1：增量构建 canonical block order

**Production files:**

- Create: `internal/appui/transport_block_order.go`
- Modify: `internal/appui/transport_projector.go`
- Modify only if contract requires: `internal/appui/transport_state.go`

**Tests:**

- Create: `internal/appui/transport_block_order_test.go`
- Modify: `internal/appui/transport_projector_test.go`

### TODO

- [ ] 新增 `upsertCanonicalTransportBlock`：新 id append order，已有 id 只更新 map。
- [ ] 每次投影出用户可见 process/commentary/approval/final/artifact 时立即登记 block，而不是 turn 结束后按类型拼接。
- [ ] tool call 首次创建位置固定；tool result 更新同一个 block。
- [ ] 一个 tool result 生成多个 artifact 时，按 renderer 产出顺序紧跟对应 tool block。
- [ ] final 只在 committed final assistant message 处登记。
- [ ] 保留 artifact id collision 处理，但 collision 修正不能改变跨类型顺序。
- [ ] `projectCanonicalTransportBlocks` 改为一致性收尾：补齐遗漏 block、校验 map，不再重新排序。
- [ ] replay 同一 snapshot 多次必须得到完全相同的 order。
- [ ] 不使用 `UpdatedAt` 排序，避免 running block 更新后跳到末尾。
- [ ] 不使用文案、tool name 或 renderer title 推断顺序。

### 必测顺序

```text
commentary -> command call -> command result(update same id)
           -> approval -> artifact -> commentary -> search -> final
```

### Phase 3 验收

- [ ] artifact 不再统一出现在 final 后面。
- [ ] tool completion 不改变 tool block 的位置。
- [ ] approval blocked/resume 后顺序稳定。
- [ ] 相同 snapshot 连续投影 10 次的 `blockOrder` 一致。
- [ ] 旧 persisted transport state 在 compatibility boundary 读取时不崩溃。
- [ ] Phase 3 不修改 runtime 和 React。

### Phase 3 验证命令

```bash
go test ./internal/appui -run 'BlockOrder|ArtifactOrder|StableOrder|Approval' -count=10
go test ./internal/appui -count=1
```

## 11. Phase 4：typed commentary/tool 归组与过程降噪

### Task 4.1：扩展 process fold 语义

**Production files:**

- Create: `internal/appui/transport_process_grouping.go`
- Modify: `internal/appui/transport_projector.go`
- Modify: `web/src/chat/components/ProcessTranscript.tsx`
- Modify only if union changes: `web/src/transport/aiopsTransportTypes.ts`

**Tests:**

- Create: `internal/appui/transport_process_grouping_test.go`
- Modify: `web/src/chat/components/ProcessTranscript.test.tsx`

### TODO

- [ ] search、command、file、tool、MCP 使用 typed `kind/foldGroupKind/foldGroupId` 归组。
- [ ] 不根据 tool name 包含 `file/search/coroot` 等字符串判断分组。
- [ ] 使用 `commentary.toolCallIds` 关联后续 tool blocks。
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

### Phase 4 验证命令

```bash
go test ./internal/appui -run 'ProcessGroup|FoldGroup|CommentaryTool' -count=1
cd web && npm test -- ProcessTranscript.test.tsx
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
- [ ] partial、blocked、needs_evidence、approval_denied、tool_unavailable、failed 保持可见。
- [ ] failedToolImpacts、uncheckedRequirements、limitations 有内容时保持可见。
- [ ] 不把 checked evidence count 和 confidence 复制成成功态大卡片。
- [ ] final answer 正文仍由 typed final block 渲染。
- [ ] artifact 仍由 typed artifact renderer 渲染，不拼进 final Markdown。

### Task 5.2：删除前端 final 文本修复器

**Production files:**

- Modify: `web/src/chat/components/AiopsThread.tsx`
- Modify only if shared sanitizer is extracted: `web/src/chat/components/MessageMarkdown.tsx`

### TODO

- [ ] 删除通过“let me、evidence refs、RCA 上下文”等业务文本判断工具过程泄漏的逻辑。
- [ ] 删除从 final 文本中的 JSON 行猜测 evidence/artifact 类型的逻辑。
- [ ] 删除按最终可见行文本做 Set 去重的逻辑。
- [ ] 保留 Markdown 渲染、链接安全、通用 secret redaction 等机器边界能力。
- [ ] 如果清理后暴露脏 final，回到 Phase 1/2 修 runtime/projection，不在 React 增加新补丁。

### Task 5.3：清理相关 runtime 内容启发式

**Production files:**

- Modify: `internal/runtimekernel/assistant_message_boundary.go`
- Modify: `internal/runtimekernel/assistant_output_commit.go`
- Modify only if structured facts need completion: `internal/runtimekernel/assistant_message_contract.go`

### TODO

- [ ] commentary 是否接纳只依据 phase、tool calls 和通用展示预算，不依据业务关键词。
- [ ] pending tool/process intent 由 provider tool calls、finish reason、checkpoint 或 structured boundary fact 表达。
- [ ] 删除 PostgreSQL、WAL、pgBackRest 等产品词对 final 完整性的决定权。
- [ ] 删除“包含根因/证据/下一步即可认为 concrete final”的通用核心规则。
- [ ] raw tool-call markup、危险操作和 secret 检测继续作为机器/安全边界保留。
- [ ] 为通用咨询、文件分析、主机运维、Web 搜索、数据库 RCA 各加正反例，证明没有单一领域偏置。

### Phase 5 验收

- [ ] 正常回答视觉上只有 final 正文，不再先显示“已验证/置信度高/已采集 N 条证据”卡片。
- [ ] 异常和受限回答仍清晰说明缺失证据、失败工具和限制。
- [ ] React 不再修补泄漏的工具过程文本；上游测试保证该文本不能成为 final。
- [ ] 核心 runtime 不使用业务关键词判断 commentary/final。
- [ ] Phase 5 的 UI 与 runtime 清理分成至少两个独立提交，不混在同一个 patch。

### Phase 5 验证命令

```bash
go test ./internal/runtimekernel -run 'AssistantMessageBoundary|Commentary|Final' -count=1
(cd web && npm test -- AiopsThread.test.ts ProcessTranscript.test.tsx)
(cd web && npm run test:ui:snapshots)
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
go test ./internal/runtimekernel ./internal/appui ./internal/server -count=1
go test ./... -count=1

cd web
npm test -- aiopsTransportConverter.test.ts AiopsThread.test.ts ProcessTranscript.test.tsx ChatTransportProvider.test.tsx
npm run build
npm run test:ui:snapshots
```

Structured streaming 边界扫描：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "snapshot\.toolInvocations|store\.runtime\.turn|processItemsByTurnId|phaseFoldsByTurnId" web/src
rg -n "JSON\.parse\(|markdown heading|summary.*steps.*actions" web/src
```

内容启发式清理扫描：

```bash
rg -n "containsAnyAssistantFinalMarker|finalMessageHasProcessIntent|containsConcreteFinalContent|looksLikeLeakedToolProcessText|readableStructuredEvidenceLine" internal/runtimekernel web/src/chat
```

期望：

- [ ] structured streaming 前两条扫描在 React Chat 生产路径无命中。
- [ ] final/process UI 不从 final 文本解析结构。
- [ ] 上述业务内容启发式函数已删除，或只剩明确记录、非生产迁移测试引用。

### Task 6.4：浏览器与真实 provider 验收

- [ ] 使用 fixture-driven browser 流程审查 running、completed、retry、approval、artifact 五类截图。
- [ ] 使用真实 provider 连续运行至少 3 次“普通咨询 + 一次工具 + 多次工具”场景。
- [ ] 记录每次 final block 数、process group 数、tool block 数和 artifact 顺序。
- [ ] 检查浏览器 console 无 error/warning 回归。
- [ ] 检查刷新、历史恢复后 transcript 与运行结束时一致。
- [ ] 真实 provider 不可用时明确列为未验证，不用 mock 冒充完成。

### Task 6.5：重大 Bug 事实记录

**Modify:**

- `fixbug.md`

### TODO

- [ ] 写入本地修复时间。
- [ ] 记录用户可见现象和影响范围。
- [ ] 记录已确认根因：提前 final、trace 过度投影、类型分桶顺序、重复语义层。
- [ ] 记录实际修改文件和关键契约。
- [ ] 记录本轮真实运行且 exit code 为 0 的验证命令。
- [ ] 记录未运行项、剩余风险和真实 provider 结论。
- [ ] 不写 secret、客户敏感内容或完整高风险命令输出。

## 14. 兼容与迁移策略

- [ ] 保持 wire schemaVersion 为 `aiops.transport.v2`；本次不新增第二个 transport version。
- [ ] `unclassified` 是 runtime-only phase，不加入前端 `AiopsAssistantMessagePhase` union。
- [ ] 新投影生成真实 `blockOrder`；已有持久化 state 由现有 compatibility boundary 读取，不原地重写历史文件。
- [ ] 历史 `final_response` fallback 仅在不存在 final assistant message 时启用。
- [ ] 稳定 block id 规则不变，避免刷新后 React key 改变。
- [ ] UI snapshot 变化必须逐张审查；只接受与本计划目标直接相关的 baseline diff。
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
- [ ] browser 完整故事链通过。
- [ ] 真实 provider 验收完成或明确标记未验证。
- [ ] `fixbug.md` 已追加事实记录。
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
