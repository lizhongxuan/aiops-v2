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

## 2026-07-10 13:30 - 隔离 Worktree 与 CI 基线不可重复

- 修复时间：2026-07-10 13:30
- Bug 现象：从当前提交创建隔离 worktree 后，`go test ./...` 因 `cmd/ai-server` 缘故无法编译，AppUI 路由回归测试依赖本机被忽略的 Playwright 输出文件；Web 全量测试另有 26 个陈旧断言失败，导致同一提交在原 checkout 与干净 worktree 中结果不一致。
- 根因：用户级全局 ignore 的 `ai-server` 规则误隐藏了 `cmd/ai-server/ssh_command_runner.go`，实际运行依赖没有进入 Git；AppUI 测试直接读取 `output/playwright` 调试产物；部分 Web 测试未跟随 QueryClient provider、Host 简化字段、Transport 可见状态、归档动作去重和页面内确认对话框的现行契约更新。
- 修复方式：把实际运行依赖的 SSH runner 源码纳入分支；将 PG rollout 输入迁移到不含凭证的 `internal/appui/testdata`；只更新陈旧测试夹具和断言，不修改对应生产行为，也不恢复已删除的旧字段、`window.confirm` 或重复归档按钮。
- 验证结果：已运行 `go test ./...`，全部 Go 包通过；已运行 `npm --prefix web test -- --run`，123 个测试文件、895 个测试全部通过；已运行 `npm --prefix web run build`，构建成功。定向 red-green 证据分别覆盖缺失 runner、不可移植 fixture、QueryClient provider 和 6 个陈旧 UI 断言。
- 风险与后续：SSH runner 仍保留原 checkout 的既有行为，本次只解决源码未被追踪的问题；用户级全局 ignore 规则仍存在，提交时需要对该文件使用精确强制暂存。Web 测试仍输出 jsdom Canvas 非致命警告，依赖审计仍有既有告警，均未在本次基线修复中扩张处理。

## 2026-07-12 02:38 - AssistantTransport 并发读取 Runtime 可变 Turn 状态

- 修复时间：2026-07-12 02:38
- Bug 现象：AssistantTransport streaming 在 RuntimeKernel 异步执行 turn 时并发读取同一个 `SessionState` / `TurnSnapshot` 指针；`go test -race ./internal/server -run '^TestAssistantTransportStories$' -count=1` 在基础回答、审批、取消、压缩和工具故事中报告多组 data race，可能让 transport fingerprint、投影和关闭判断看到撕裂状态。
- 根因：`SessionManager` 的锁只保护 session map，本身不保护 map 中可变指针指向的内容；`Get` 将 RuntimeKernel 正在修改的对象直接交给 AssistantTransport 轮询，而 `assistantTransportSessionTurns` 的浅拷贝发生得太晚，且仍共享嵌套 slice、map 和 pointer。
- 修复方式：保留 RuntimeKernel 为 turn lifecycle 唯一 writer；由 `SessionManager` 在 session 创建、repository hydrate 和每次 `Update` 时 deep-clone 并原子发布只读 snapshot，删除 session 时同步清理；clone 失败会清除旧 snapshot 并记录显式 publish error，禁止继续返回可能仍为 running 的陈旧状态。AssistantTransport 的 command reprojection、streaming、resume、主 turn 和 host-child fingerprint/projection 全部改读 published snapshot并传播读取错误；入口遇到不支持 snapshot capability 的 session source 立即返回 503，不启动 runtime；故事 runner 的诊断读取也对 publish error 明确失败。
- 验证结果：确定性 RED 证明 published snapshot 缺失、repository hydrate 不发布快照、无效 `json.RawMessage` clone 失败时会遗留旧 snapshot；public HTTP RED 证明缺少 snapshot capability 时旧入口不会立即 503。实现后运行 session snapshot、公开 AssistantTransport API 和 14 个 story 的 race 与 focused 非 race 测试，均通过且无 race；无 snapshot capability 的请求返回 503 且 runtime 未启动，clone 失败不返回旧 snapshot。
- 风险与后续：deep-clone 在 session publish checkpoint 增加与 session 大小相关的序列化成本，需要继续观察超长会话；`GetSnapshot` 的只读约束由 API 契约保证，调用方不得修改返回对象。AssistantTransport 已不再使用共享 working pointer，其他仍直接调用 `SessionSource.Get` 的非 transport 读路径可在后续 race audit 中单独治理。

## 2026-07-12 12:30 - 未完成后置校验误标 verified 与证据恢复被审批校验拒绝

- 修复时间：2026-07-12 12:30
- Bug 现象：变更动作只声明 `requiredPostChecks`、尚未产生 completed `postChecks` 时，FinalContract 仍可能输出 `verified/high`；同时 AppUI 对 `pending_evidence` 的“接受并继续”会携带 evidence ID 与 `Decision=approved`，Runtime 却只在 `PendingApprovals` 中查找该 ID，导致恢复在执行前失败。
- 根因：`classifyFinalContractStatus` 与置信度计算没有比较 required/completed post-check 集合；`ResumeTurn` 无条件复用普通 approval 的精确匹配函数，没有为 `TurnResumeStatePendingEvidence` 建立独立、fail-closed 的 evidence ID + toolCall ID 绑定。
- 修复方式：集中计算 outstanding required post-check；只要仍有未完成项，FinalContract 降为 `needs_evidence`，置信度最高为 medium，所有声明项完成后才允许 `verified`。证据恢复按 snapshot resume state 分流，精确校验 evidence ID、turn 与 pending toolCall，错误或陈旧 ID 继续 fail closed；普通 approval 的原有精确匹配保持不变。
- 验证结果：RED 已复现 `verified/high` 与 `approval "evidence-1" is not pending`；实现后运行 `go test ./internal/runtimekernel -count=1`、相关 race 测试和公开 `RunTurn -> pending evidence -> ResumeTurn -> ToolDispatcher -> FinalContract` 回归链均通过。AssistantTransport `approval_resume` story 的受控 baseline 由 `verified/high` 更新为 `needs_evidence/medium`，且 required/completed post-check 仍分别投影。
- 风险与后续：当前 `PostChecks` 的 completed 事实仍必须由真实验证执行写入，不能把模型文本或声明本身当作完成；后续新增 verifier 时必须复用同一 typed 集合语义并补充 AssistantTransport 故事。

## 2026-07-12 13:10 - Host Manager 编排工具被误判为主机变更

- 修复时间：2026-07-12 13:10
- Bug 现象：已接受多主机计划后，manager 调用 `spawn_host_agent` 仍在 ToolDispatcher/RuntimeKernel 的主机 mutation rollback gate 被阻断，报告 `rollback_contract_invalid`，真实 host-bound child lifecycle 无法启动；工具输出同时只有 `childAgentId`，AssistantTransport 无法按 `id` 投影 child 状态。
- 根因：spawn/send/stop 三个 manager lifecycle 工具虽然明确“不直接执行或修改主机”，metadata 却声明为 `LayerMutation + Mutating + RiskMedium`，把编排控制面变化错误等同于主机数据面变更；child result contract 也缺少 transport 所需的 identity/session/host/lifecycle 字段。
- 修复方式：把 manager lifecycle 工具归为 low-risk core orchestration control，保持 `IsReadOnly=false` 并继续由 Orchestrator 的 `PlanAccepted` gate 控制实际执行；真实 child 的 host command/policy/approval/rollback 路径完全不变。child result 增加 transport-compatible `id` 等完整字段，并保留 `childAgentId` 兼容字段。
- 验证结果：RED 精确命中错误 governance 与缺失 DTO；PlanAccepted 拒绝测试在修复前后都通过。修复后 `go test ./internal/hostops -count=1`、`go test -race ./internal/hostops -count=1` 与 diff check 通过；后续由 public AssistantTransport multi-host story 验证生产 manager/child 链。
- 风险与后续：manager send/stop 不再触发主机 mutation approval，但它们只能操作既有 mission/child，且 child 的实际主机动作仍由独立 worker RuntimeKernel 治理；必须保留真实 multi-host story 防止再次退化为 StaticTool 外壳。

## 2026-07-12 13:45 - 多主机部分失败被 FinalContract 误标 verified

- 修复时间：2026-07-12 13:45
- Bug 现象：`wait_host_agents` 成功返回完整聚合 JSON，其中 host-a completed、host-b failed，但 Runtime 只看到“工具调用无 Error”，把 invocation 标为 completed，最终输出 `verified/high`。
- 根因：`ToolResult` 没有表达“调用成功但业务聚合仅部分完成”的 typed outcome；Runtime 只能在 completed/failed 二元状态中选择，FinalEvidence 因而丢失 child partial failure。
- 修复方式：新增兼容默认值的 typed `ToolResultOutcome`；hostops wait 按 child typed status 写入 complete/partial，仍保留完整 JSON 且不伪造 Go error。Runtime 无损物化 outcome、写入 canonical tool_result AgentItem，并把 invocation 标为 terminal `partial/partial_result`；FinalContract 从 invocation 事实降为 `partial/medium`。
- 验证结果：RED public story 真实观察到 failed child 与 `verified/high` 同时存在；新增 `RunTurn` 回归证明 partial 内容继续反馈模型、canonical item/invocation/final 三处一致。`go test ./internal/hostops ./internal/runtimekernel -count=1` 和新增路径 race 测试通过；后续 AssistantTransport story负责验证 manager/child/transport 全链。
- 风险与后续：未知 outcome 归一化为 partial 以 fail safe；旧工具未写 outcome 时保持空值并按 complete 兼容，不向所有旧 AgentItem 注入新字段。当前 partial impact 通用文案仍偏向普通工具失败，另以独立小阶段调整为聚合结果语义。

## 2026-07-13 16:20 - 模型超时恢复误走旧工具审批分支

- 修复时间：2026-07-13 16:20
- Bug 现象：turn 已完成一次只读工具调用、下一次模型调用超时后，从 `model_timeout` checkpoint 恢复会把历史 iteration 中已完成的 tool call 当作 pending approval tool call，可能重复派发旧工具；引入 typed Step cause 校验后该错误被 fail-closed 为“approval resume cause 缺少 approval/tool-call id”。
- 根因：`ResumeTurn` 只要 `pendingToolCall(snapshot)` 能从历史 iteration 找到任意 tool call 就进入 approval 分支；该 helper 在没有 `PendingApprovals` / `PendingEvidence` target 时会返回最新历史调用，没有先验证当前 snapshot 确实存在待恢复 gate。
- 修复方式：新增 `gatedPendingToolCall`，只有 snapshot 存在 pending approval 或 pending evidence 时才允许进入工具恢复分支；普通 `model_timeout` checkpoint 进入模型重试分支，并以 `model_retry_resumed` typed StepRevision 连接 provider 调用前已持久化的失败 Step。
- 验证结果：RED 由 `go test ./internal/runtimekernel -count=1` 的 `TestRunTurn_ModelTimeoutBecomesRecoverableAndResumeContinues` 复现；修复后该测试与 `TestRunTurn_BlockedToolCallCanResume`、StepRevision 聚焦测试和 runtimekernel 全包均通过，并断言 retry previous/next hash、TurnAssembly hash 与 `model_retry_resumed` revision。
- 风险与后续：旧 snapshot 若只保留历史 tool call、却丢失 pending approval/evidence typed state，将不再猜测并重放该工具，按 fail-closed 进入对应 checkpoint 恢复或校验失败；暂无已知审批绕过风险。

## 2026-07-13 16:25 - Context compaction 拆分工具因果组并静默丢消息

- 修复时间：2026-07-13 16:25
- Bug 现象：上下文压缩按单条消息移动边界时，可能把同一 assistant 的多个 tool call 与部分 tool result 分到摘要两侧，形成 orphan tool result；summary 加入后若再次超预算，旧逻辑还会从 retained 头部逐条删除，但不扩大 `CompactedSegment.EndIndex` 或重算摘要，导致消息既不在摘要覆盖范围也不在模型输入中。
- 根因：`SplitContextForCompaction`、hard-keep 回填和 summary 后预算回退分别以消息数量为单位，没有统一的 tool causal group；最后一次回退发生在 refs、summary 和 segment 已生成之后，是第二个无事实记录的丢弃 writer。
- 修复方式：在 `context.go` 集中构建 assistant tool calls + 紧随匹配 tool results 的原子 causal group；初始 split、hard-keep 边界与 summary 预算预留只移动完整 group。额外预算压缩在 refs/summary/segment 生成前扩大 compactable prefix，删除生成后的逐消息静默回退，确保 `TrimmedCount`、`TruncatedAt` 和 segment 范围覆盖所有被摘要替代的消息。
- 验证结果：RED 分别复现了 `assistant-tools/tool-result-a` 被压缩而 `tool-result-b` 被保留，以及 `msg-3` 既未压缩也未保留；修复后运行 causal group 多预算性质测试、coverage 回归、全部 context compaction 聚焦测试和相关 race 测试均通过。
- 风险与后续：当 hard-kept 最小后缀或单个 causal group 本身超过模型预算时，原子性优先，context window 可能暂时高于目标预算；后续应以独立任务增加 typed oversized-group spill/拒绝策略，禁止重新引入逐消息切割。

## 2026-07-13 16:56 - Resume metadata 可绕过冻结的 TurnAssembly profile

- 修复时间：2026-07-13 16:56
- Bug 现象：pending approval 恢复请求可通过 `metadata.profile`、`toolProfile` 或 `agentProfile` 改变恢复 Step 的 Prompt 编译 profile 和 tool-surface policy；随后 Step builder 又把 admission facts 覆盖为冻结的 TurnAssembly，导致实际工具面已经变化，但 StepReference 仍显示原 profile/TurnAssembly hash。
- 根因：`ResumeTurn` 在校验 immutable admission metadata 前先 merge 客户端 resume metadata；`buildRuntimeStepContext` 的冻结 admission 回填发生在 Prompt 编译和 StepToolRouter 构建之后，只保证 trace facts 一致，不能撤销上游已发生的 control drift。
- 修复方式：在任何 approval decision、tool replay、Prompt 编译或 provider 调用前校验 ResumeRequest 中所有 admission-control metadata；profile aliases、agent kind 和 permission profile直接对比冻结 TurnAssembly，其余 control key 必须与原 snapshot metadata 精确一致。`runtimecontract` 导出统一 key 分类，runtime 不新增第二份控制 key 字符串表。
- 验证结果：RED 证明 profile drift resume 返回 nil 且会继续执行；修复后生产 `RunTurn -> pending approval -> ResumeTurn` 回归逐一验证三个 profile alias 均返回 `immutable control metadata drift`，tool execution 和 model call 计数不变，pending approval 未被消费；合法 approval resume、model-timeout resume、rollback/choice resume、runtimecontract/runtimekernel 全包和 focused race 均通过。
- 风险与后续：当前只阻止 `runtimecontract` 注册的 admission-control keys；其他会改变 capability surface 的兼容 metadata 仍应在 Task 9 ActionToken/current-world revalidation 中通过 router、permission 和 target hash 拒绝，不能依赖客户端自律。

## 2026-07-13 18:10 - 审批恢复自比较旧指纹并跳过当前权限判定

- 修复时间：2026-07-13 18:10
- Bug 现象：AppUI 在审批决定中回显旧 arguments、tool surface 和 permission hash，Runtime 再拿这些客户端回显值与同一份 PendingApproval 比较；匹配后 `DispatchApproved` 跳过当前 permission、policy 与 hook permission gate，导致审批后世界已经变化时仍可能执行旧动作。
- 根因：审批记录没有统一的服务端 binding；Resume 把客户端 metadata 当作当前事实；dispatcher 只接收 `approved=true` 布尔值，无法证明批准的是同一 turn、tool call、参数、目标、路由、权限、回滚和 checkpoint。
- 修复方式：PendingApproval 创建时冻结带完整性 hash 和 expiry 的服务端 ActionToken binding；Resume 在清除 pending approval 或执行工具前，从当前 Runtime Step 重算七类事实并校验，任何漂移都返回稳定 `approval_context_stale` 且只记录 mismatch field。`DispatchApproved` 必须接收 verified typed authorization，并始终重跑 permission、policy 和 hook gate，只有重复 NeedApproval 可由同一 binding 满足，Deny/Evidence 始终优先。AppUI 只发送 approval/checkpoint/decision，不再携带 authority metadata；模型参数 token 也不能覆盖已有可信执行上下文。
- 验证结果：表驱动真实 `RunTurn -> PendingApproval -> ResumeTurn` 测试覆盖 arguments、target、真实 registry/router、permission、rollback、checkpoint、expiry 漂移，全部在 executor 前拒绝并由服务端重发新 approval ID、checkpoint 和 binding；expiry 用例再次批准新记录后只执行一次。另覆盖无 ActionToken 的旧审批参数漂移、过期 pending evidence 原子迁移为普通 reapproval 并再次成功恢复、缺失 authorization、hook 改参、Deny/Evidence 优先和模型伪造 token；AppUI 测试验证只调用 ResumeTurn、metadata 为空且 reapproval 使用 pending_approval 状态。
- 风险与后续：旧持久化 approval 没有 ActionToken 时从原服务端 PendingApproval 冻结兼容 binding；只有本来没有 approval binding 的 pending evidence 使用同一组服务端当前事实生成兼容 binding，二者都不读取客户端 hash。ActionToken 是 binding 而非 bearer credential，不能单独授予权限。后续 Task 20 继续用公开 AssistantTransport、真实模型和浏览器审批故事验证整条链。
