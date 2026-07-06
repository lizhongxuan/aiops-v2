# Fix Bug 记录

## 2026-07-03 12:48 - 特殊输入确认误拦截普通工作流确认

- 修复时间：2026-07-03 12:48
- Bug 现象：新增特殊输入短期记忆确认 flow 后，用户发送“确认生成工作流候选：AI 新闻摘要工作流”这类普通业务确认时，被特殊输入确认逻辑提前消费，导致 Runner Workflow 候选生成缺少“静态验证通过”和 Docker provider 边界输出。
- 根因：`internal/appui/chat_service.go` 中的特殊输入确认 intent 只按文本包含“确认 / 就用 / 是的 / 没错”判断，没有校验当前会话是否存在待确认特殊输入候选，也没有要求来自 UI 的结构化特殊输入确认命令。
- 修复方式：收紧 `specialInputIntentFromContent` 的确认条件，只有 `aiops.specialInput.command=confirm`、显式特殊输入 target key，或当前 session 存在 active raw typed fact 时，才进入 `IntentConfirm` 并消费当前 turn；普通业务确认继续走原有受控 workflow 生成路径。
- 验证结果：已运行 `go test ./internal/appui -run 'WorkflowDraftFromConfirmation|SpecialInput.*(Correction|Forget|Confirm)|TransportCommandsSpecialInput' -count=1` 和 `go test ./internal/specialinputmemory ./internal/appui ./internal/runtimekernel ./internal/modeltrace ./internal/promptinput -run 'SpecialInput|WorldState|MemoryReadPlanTrace|PromptTrace|ExecutionScopeGuard|RoleBinding|TransportCommandsSpecialInput|Correction|Forget|Confirm|PendingConfirmation|Tombstone' -count=1`，均通过；普通 workflow confirmation 不再被特殊输入确认拦截，特殊输入确认测试仍通过。
- 风险与后续：暂无已知风险。后续若新增新的确认型业务命令，需要继续避免用纯文本关键词抢占业务 flow。

## 2026-07-03 13:24 - 真实模型输入 trace 丢失特殊输入 world state

- 修复时间：2026-07-03 13:24
- Bug 现象：真实 glm-5.1 远程主机验证中，turn 能正确路由并在 `remote-120-77-239-90` 执行工具，但 `.data/model-input-traces/...json` 只能看到 `aiops.target.hostId`，看不到顶层 `specialInputWorldState` / `MemoryReadPlan`，不满足可追踪执行要求。
- 根因：`modeltrace.Request` 和 redaction 中已有 `SpecialInputWorldState`，但 `internal/modeltrace/trace.go` 的 JSON `payload` struct 没有对应字段，`buildPayload` 也没有把已清洗的 world state 投影到顶层 payload；原测试只用字符串检查嵌套 `promptInputTrace`，没有锁住真实落盘入口。
- 修复方式：为 `payload` 增加 `specialInputWorldState` 顶层字段，并在 `buildPayload` 中从 `redactedPromptTrace.SpecialInputWorldState` clone 写入；同时把 `TestWriteIncludesSpecialInputWorldStateInPromptTrace` 改成从 `Request.SpecialInputWorldState` 入口写入并解析 JSON，要求顶层和 `promptInputTrace` 两处都存在。
- 验证结果：已运行 `go test ./internal/modeltrace -run 'SpecialInputWorldState|PromptTrace' -count=1`，通过；后续 trace JSON 能直接暴露 special input world state，Prompt Trace 页面和文件排查不再只能依赖 metadata。
- 风险与后续：新增字段只写入已存在的 redacted world state，风险较低；需要在后续真实 UI/assistant transport 流程中继续确认 `SpecialInputReadPlan` 本身确实传入 runtime snapshot。

## 2026-07-03 17:01 - Final 合约证据 ID 与模型超时原始错误外露

- 修复时间：2026-07-03 17:01
- Bug 现象：AI 对话最终合约摘要会直接显示 `checkedEvidenceRefs` 中的内部 tool call id；模型服务连接超时时，页面会显示 provider 请求地址、`/chat/completions`、TLS 握手等底层错误细节，并出现“约 20s”这类容易被误解为总模型等待预算的文案。
- 根因：`web/src/chat/components/AiopsThread.tsx` 直接渲染 `checkedEvidenceRefs.join(...)`；`internal/modelrouter/provider_adapter.go` 使用 `%w` 把 provider 原始 timeout 错误拼进用户可见错误；`internal/appui/transport_projector.go` 和 `web/src/transport/transportErrorMessage.ts` 对中文前缀的 raw model timeout 缺少兜底清洗。
- 修复方式：Final 合约摘要只显示证据数量，不渲染内部 evidence ref；modelrouter 返回带安全 `Error()` 文案且保留 `Unwrap()` 的 timeout 错误；transport projection 和前端 transport runtime 增加 model timeout 脱敏兜底；新增 Playwright snapshot 覆盖 final contract 摘要隐私边界。
- 验证结果：已运行 `go test ./internal/modelrouter ./internal/appui -count=1`、`npm --prefix web test -- --run AiopsThread aiopsTransportRuntime aiopsTransportConverter`、`npm --prefix web run typecheck`、`npm --prefix web run test:ui -- react-shell-snapshot.spec.js --project=chromium -g "final contract summary hides raw evidence refs"`，均通过；内置浏览器刷新当前页面后未发现 raw evidence call id、provider URL、TLS 原始错误或“正在等待模型返回”残留。
- 风险与后续：`npm --prefix web run test:ui:snapshots` 完整集合目前仍有 4 个既有 snapshot 断言漂移，涉及旧文案、旧 class、外溢证据隐私展示和 ops manual merged card，新增 final contract 摘要用例通过；这些漂移不属于本次 evidence/timeout 修复范围，需要后续单独整理旧 snapshot baseline。

## 2026-07-03 17:25 - 特殊输入真实浏览器测试暴露的状态恢复与 trace 问题

- 修复时间：2026-07-03 17:25
- Bug 现象：真实 LLM 浏览器测试报告暴露 4 个问题：短期记忆 context bar 不稳定显示；Prompt Trace v2 缺少 `specialInputWorldState`；`@120.77.239.90。` 这类中文标点后的特殊输入会出现前端“处理中”但用户难以判断是否进入后端 turn；模型失败或超时后 composer 可能继续被 assistant-ui running 状态锁住。
- 根因：`TransportProjector.ProjectTurnSnapshot` 在投影任意后续 turn 时会用 nil `SpecialInputReadPlan` 覆盖已有 `SpecialInputContext`；Trace v2 只写了 `promptInputTrace`，没有顶层 `specialInputWorldState`；typed host mention fallback 缺少 address 元数据，降低了后端 host binding 可追踪性；composer 的运行态直接合并 assistant-ui running，没有在 transport failed/canceled 后解除禁用。
- 修复方式：Transport projection 只在 turn 携带 `SpecialInputReadPlan` 时刷新 context，后续普通 turn 保留上一轮短期记忆；Trace v2 schema 与 `writeRuntimeStepTrace` 增加顶层 `specialInputWorldState`，并把 Request 的 world state 同步 merge 到 `promptInputTrace`；前端 host mention display/fallback 补齐 `address`，中文标点后的 typed mention 可提交结构化 host metadata；composer 在 transport 终态失败/取消后恢复输入，context bar 从空消息容器移出，避免被 `empty:hidden` 隐藏。
- 验证结果：已运行 `go test ./internal/appui ./internal/modeltrace ./internal/runtimekernel -run 'SpecialInput|TraceDocumentV2|ModelInputTraceRequestCarriesSpecialInputWorldState|WriteRuntimeStepTraceV2CarriesSpecialInputWorldState|TransportProjector|Sanitiz|Timeout' -count=1`、`npm --prefix web test -- --run AiopsComposer AiopsThread aiopsTransportRuntime aiopsTransportConverter`、`npm --prefix web run typecheck`、`npm --prefix web run test:ui -- e2e/special-input-memory.spec.js --project=chromium`、`npm --prefix web run build`，均通过；内置浏览器 `?fixture=special-input-memory` 可见 context bar，无“正在等待模型返回”残留，提交 `@120.77.239.90。现在只读看根分区和 inode，不要改` 后页面进入真实 turn，终态后 composer 恢复可输入。
- 风险与后续：当前内置浏览器 `4176` 的 API 代理到已有 `18080` 旧 `ai-server` 进程，未重启后端，因此浏览器里的真实 trace 文件不能作为新 Go 代码的最终证据；后端 trace 修复已用 `writeRuntimeStepTrace` v2 文件级测试覆盖。若需要完整端到端验证，应重启或单独拉起新后端后再执行真实 LLM trace 页面检查。

## 2026-07-06 10:29 - Final 合约内部低置信度误显示为对话状态

- 修复时间：2026-07-06 10:29
- Bug 现象：普通对话中，前端会在回答上方显示“未确认 置信度低”；用户追问“你显示的置信度是什么”时，模型看不到该 UI 元数据，回答自己没有显示置信度，造成页面可见状态与模型可见上下文不一致。
- 根因：`web/src/chat/components/AiopsThread.tsx` 的 `FinalContractSummary` 只要 `finalContract.schemaVersion` 和 `confidence=low` 存在就渲染摘要，即使 status 为 `unknown` 且没有证据数量、未完成检查、工具限制或 limitations；这些字段属于 transport/debug 元数据，不会进入下一轮 provider request 的历史正文。
- 修复方式：收紧 `finalContractSummaryView` 显示条件，`unknown` 且没有用户可行动细节时返回 `null`，保留 `verified` 证据数量、`tool_unavailable`、`needs_evidence`、limitations 等有用户价值的结构化状态；新增单测和 Playwright screenshot 覆盖该 UI 边界。
- 验证结果：已运行 `npm --prefix web test -- --run AiopsThread aiopsTransportConverter`、`npm --prefix web run test:ui -- react-shell-snapshot.spec.js --project=chromium -g "final contract summary"`、`npm --prefix web run typecheck`、`npm --prefix web run build`，均通过；内置浏览器刷新 `http://127.0.0.1:18080/` 后未发现“未确认”或“置信度低”误显示，控制台无 error。
- 风险与后续：只隐藏无用户可行动细节的 `unknown` 内部校准摘要，不影响有证据、有工具限制或有未完成检查的 final contract 卡片；后续如果需要解释置信度，应设计显式的帮助入口或 Prompt Trace debug 入口，而不是让模型猜测前端 metadata。
