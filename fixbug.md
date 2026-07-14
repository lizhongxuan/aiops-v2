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

## 2026-07-14 01:13 - Final typed facts 未接 worker、历史审批和注入 completion policy

- 修复时间：2026-07-14 01:13
- Bug 现象：manager 已生成 running worker 后，模型可在未调用等待/聚合工具时直接完成 turn；带 `partial_result` 的工具结果还能覆盖变更无目标、计划、覆盖度或 completion policy 的硬阻断；session 曾拒绝一次审批后，后续无关 turn 可能继续显示 `approval_denied`；自定义 CompletionPolicy 的结论可能与已提交 FinalContract 不一致。
- 根因：typed worker final gate 只有 helper 和单测，没有 production caller；partial 分支排在硬阻断之前；`RejectedApprovals` 没有 TurnID 且 final builder只检查 session slice 非空；`BuildFinalRuntimeFacts` 硬编码 `DefaultCompletionEvaluator`，未消费 RuntimeKernel 注入的 evaluator。
- 修复方式：通过中立 `agentruntime` machine-contract adapter 校验真实 tool call name 与 hostops schema version 一一绑定，再聚合最新 worker status；任意非终态（含 spawning/queued/approval_required/unknown）在真实 finalization 前触发一次 typed retry，仍 pending 时以 `pending_worker_evidence` 硬阻断，failed/cancelled 则以 `non_completed_worker_evidence` 降为 partial。把 target/approval/plan/coverage/runtime approval/completion policy 硬阻断置于 partial 之前，但保留 partial 优先于单纯 missing verification report；RejectedApproval 新增 TurnID 并只向同 turn final 投影；RuntimeKernel 使用当前 run context 和完整 TurnState 只调用一次实际注入的 CompletionEvaluator，FinalContract、TurnResult、TurnComplete event 与 span terminal 复用同一个决定。
- 验证结果：RED 已分别复现 premature worker final、partial 覆盖五类硬阻断、历史拒绝污染和注入 evaluator 未被调用；修复后 focused RuntimeKernel 测试与 `multi_host_manager` 公开 AssistantTransport story 通过，story 在 spawn 后加入一次虚假完成回答并验证 Runtime 强制继续调用 `wait_host_agents`。`go test ./internal/... -count=1`、`scripts/check-aiops-harness-contract-boundaries.sh` 和 `scripts/aichat-harness-hardening-gate.sh` 全部通过，Web gate 为 117 + 18 tests。
- 风险与后续：worker gate 目前消费 `aiops.hostops.child/v1` 与 `aiops.hostops.wait/v1` 两个 versioned machine contract，且拒绝普通工具伪造同名 schema；其他 agent backend 必须先在中立 adapter 定义 tool source 与 schema 的精确绑定，不能靠回答文本或任意 JSON 字段接入。暂无已知审批绕过风险。

## 2026-07-14 02:32 - 缺少目标的变更请求直到 Final 阶段才被文本补判

- 修复时间：2026-07-14 02:32
- Bug 现象：没有绑定目标的部署、安装或重启请求仍会进入 Prompt 编译和模型采样，Runtime 最后再从用户或回答文本推断变更意图；移除文本门禁后，同类请求可能只显示 `needs_evidence`，并且浪费 provider 调用。
- 根因：AppUI 已生成 `IntentFrame`，但只序列化进兼容 metadata；`TurnRequest`、`RuntimeTurnContext` 和首轮 `depthReq` 没有直接传递 typed intent。Final/verification 门禁因此依赖 task depth、metadata 和用户文本重复推导，无法在 admission 阶段依据冻结事实 fail closed。
- 修复方式：把 active-route 修正后的 `IntentFrame` 直接写入 `TurnRequest` 并传到 `AdmissionInput`；Final、verification 和资源去重只消费已验证的 `TurnAssembly`。为 `mutation + no verified target` 增加稳定 admission sentinel，首轮在 Prompt/provider 前生成 `FinalContract.status=blocked` 的结构化安全结果；P0 story 显式断言 provider 调用为 0、无 iteration 和无 Prompt hash。
- 验证结果：RED 分别复现了 typed intent 字段缺失、部署未分类、最终门禁被 metadata/prose 改写以及 no-target mutation 仍调用 provider；修复后 `go test ./internal/runtimekernel -count=1` 与 `go test ./internal/server -run '^TestAssistantTransportStories/mutation_missing_target$|^TestAssistantTransportStoryCorpusCoversP0Contract$' -count=1` 通过，公开 AssistantTransport 中保留 user/final timeline 和三个稳定 limitation，provider 调用为 0。
- 风险与后续：自然语言候选分类仍位于 AppUI admission adapter，Runtime 不包含业务关键词；未识别或低置信意图不会被 Final 文本启发式升级为变更。其他 assembly failure（例如缺少 rollback policy）仍需复用相同的 typed structured-failure 模式，不能恢复文本补判。

## 2026-07-14 02:35 - Role scope 约束受 metadata 开关控制且同会话继承时丢失

- 修复时间：2026-07-14 02:35
- Bug 现象：当前 turn 即使已经冻结 resource role binding，dispatcher 的 role guard 仍可能因 metadata/env 开关未启用而完全关闭；同会话只继承 typed target、未重复提交 role 字段时，新的 TurnAssembly 会丢失已有 role 与 conflict 约束，形成权限范围放宽。
- 根因：`RoleBindingConflicts` 只保存在可变 SessionState，没有进入 AdmissionFacts/TurnAssembly/hash；`BuildRuntimeTurnContext` 只读取当前 request 的 role bindings；dispatcher guard 的 Enabled 又由兼容 metadata/env 决定，而不是冻结控制事实。
- 修复方式：AdmissionFacts 新增规范化、带完整性 hash 的 role conflicts，TurnAssembly admission-control hash 同步覆盖；只有当前 request 未替换 target/role 且有效 typed session target 被继承时，才把 session role facts带入 admission。dispatcher role guard 对每个有效 frozen assembly 默认启用，从该 assembly 读取 bindings/conflicts；缺失或无效 assembly 对 mutation 保持 fail closed。
- 验证结果：RED 证明 role conflicts 不存在于 typed schema、同会话 carryover 丢失且无 metadata 时 guard 关闭；实现后运行 `go test ./internal/runtimecontract ./internal/agentassembly ./internal/runtimekernel ./internal/server -count=1` 全部通过，并验证 conflict 会改变 TurnAssembly hash、阻止 mutation，历史 session role 不会覆盖当前 frozen facts。
- 风险与后续：role conflict 目前允许冻结用于只读诊断，但所有 mutating/approval-required tool 都会被 dispatcher 拒绝；调用方必须通过新的、无冲突的 typed role facts 开启后续变更，不能靠清除 metadata 开关绕过。

## 2026-07-14 02:43 - 历史 ResourceIdentity 被当前 Turn target 重新命名

- 修复时间：2026-07-14 02:43
- Bug 现象：同一会话先读取 host-A 资源、再切换到 host-B 时，模型输入去重会把当前 B 的 target hash 套到整段历史 tool messages；旧 A 资源可能被重新登记进 B namespace，导致跨主机错误去重或复用 source/content。
- 根因：`TargetIdentityHash` 在读取历史时才由当前 TurnAssembly 批量注入，ToolResult 产生和持久化时没有记录其所属冻结 target；组合历史因此丢失每条 resource 的来源 namespace。
- 修复方式：ToolResult 增加 typed `TargetIdentityHash`，在统一 `materializeToolResult` 入口从当时已验证的 TurnAssembly 写入，普通与 approval-resume 路径自动复用；历史去重只读取每条 ToolResult 自身的 hash，不再接受当前 turn 的批量覆盖。旧记录没有该字段时保留空 namespace，不猜测为当前目标。
- 验证结果：RED 使用同一 session 的 `[host-A tool message, host-B tool message]` 组合历史复现两条记录落入同一 namespace；修复后两个 miss、两个 target namespace 与各自 source/content 独立，重复 B 才命中 B。生产 writer 测试同时验证物化结果写入 host-A 冻结 identity；`go test ./internal/runtimekernel ./internal/server -count=1` 通过。
- 风险与后续：升级前持久化的旧 ToolResult 没有 target hash，会继续位于空 namespace；这是安全兼容降级，不会把旧记录冒充为新 target。若未来迁移旧数据，只能依据当时的 canonical TurnAssembly/rollout 回填，不能根据当前 session target 推断。

## 2026-07-14 03:16 - Permission、ActionToken 与 FinalContract 的兼容回退可绕过冻结事实

- 修复时间：2026-07-14 03:16
- Bug 现象：ToolDispatcher 未显式配置 permission binding 时，mutation 会跳过 hash 校验；旧 PendingApproval 缺少 ActionToken 时，ResumeTurn 会就地重建 token 并在同一次请求执行；持久化 AgentItem 中伪造的 `FinalContract.status=verified` 即使没有 evidence 或仍有未完成 post-check，也会被 Transport/UI 直接显示为 verified。
- 根因：permission binding 被设计为由 caller 可选启用，dispatcher 为 legacy 单测保留了 mutation fail-open；approval resume 把 tokenless 旧记录当作可透明迁移数据，而不是缺失权威事实；`BuildFinalContract` 虽已保证新写入事实一致，hydration/projector 边界却只按 schemaVersion 反序列化并信任 status，没有复验 typed invariant。
- 修复方式：所有 mutation 在 dispatcher 内默认要求 expected/current permission hash 且完全一致，未绑定、缺失、current-only 或 mismatch 均在 executor 前返回 `permission_binding_invalid`，read-only 保持兼容；普通 approval 与 pending-evidence 缺 ActionToken 时统一返回 `approval_context_stale(token)` 并重发带服务端 token 的新 approval，第一次 resume 的 executor 调用为 0，只有第二次 fresh resume 可执行；FinalContract 增加 typed `Validate` / `NormalizeForProjection`，persisted verified 若缺 checked evidence、存在 unchecked requirement 或 required post-check 未完成，会在 hydration 边界降为 `needs_evidence` 并附加 `invalid_verified_contract_facts`，不解析回答文本。
- 验证结果：RED 分别复现 unbound mutation 未返回 permission binding 错误、nil-token 首次 resume 直接执行、三类 malformed verified 被 UI 接受；修复后 focused permission/approval/final/transport 测试和 `go test ./internal/runtimekernel ./internal/appui -count=1` 全部通过。AssistantTransport `approval_denied` story 的 typed process 状态由旧 `completed` 更新为 `rejected`，focused 连跑 3 次通过；完整 hardening gate 在该更新后通过，包含 Go、Web 117+18 tests 与 boundary self-test。
- 风险与后续：旧 tokenless approval 不再透明升级，客户端需要处理一次 stale/reissue 后再提交 fresh approval；旧 malformed verified 记录会显示 needs-evidence 而不是中断整条 turn。未绑定的 mutation 生产扩展点不再具有兼容通道，新增 dispatcher caller 必须显式提供 frozen permission binding。

## 2026-07-14 05:45 - Replay 只比较 JSON 且控制身份受 wall-clock 污染

- 修复时间：2026-07-14 05:45
- Bug 现象：初版 replay runner 可以把预期 rollout 原样作为 backend 输出而“通过”，reference fixture 也只有路径、不能真实执行；接入真实 RuntimeKernel 后，同一 story 会因消息时间、随机 message ID、checkpoint ID、approval ID 和 ActionToken expiry 产生不同 Step/event hash。Contract 的 final hash还误用了普通 SHA256，与 Runtime 的 normalized control hash 算法不一致。
- 根因：canonical rollout 只有 hash/count，没有可重建的 typed TurnAssembly/Step/Final/ActionToken sidecar；测试没有强制 provider、dispatcher、approval 和 AppUI projector 走生产路径；Step hash错误包含 wall-clock/随机消息 ID，checkpoint/approval ID直接来自 UnixNano；replay normalizer逐 event 创建 ID 映射，无法保持跨事件引用。
- 修复方式：增加 production-neutral、deep-frozen `ReplayArtifactSink`，在 provider/final commit 前捕获 TurnAssembly、RuntimeStepContext、FinalRuntimeFacts/FinalContract 和已验证 ActionToken；reference overlay 校验 AssistantTransport story 路径与 SHA，并由真实 scripted provider、ToolRegistry、RuntimeKernel、TransportCommandHandler 和 TransportProjector hydrate 后双跑。Step control hash排除 wall-clock和随机 message ID；checkpoint/approval ID改由冻结控制事实稳定派生；ActionToken 原始 Hash/expiry仍先严格验证，只在 replay 对比时归一 wall-clock并重算 token hash。event 比较使用跨事件双射 normalizer，tool/approval/control 字段继续逐项保留。
- 验证结果：`approval_resume` 与 `tool_not_found` 真实 provider/full-story replay 连跑 10 次稳定；prompt/tool/permission 篡改在 Contract、Provider Fixture、Full-story 三模式均报告首个 sequence/kind/expected/actual hash/owner。`go test ./internal/eval -count=1`、`go test ./internal/runtimekernel -count=1`、全部 AssistantTransport stories、focused race 和 `go vet` 通过。
- 风险与后续：reference JSON 是带 SHA 的 source overlay，权威 command/provider/tool/transport 断言仍来自既有 AssistantTransport story，hydrate 只生成内存 sidecar，不落第二套输入真相。ActionToken expiry 不能从安全 hash 中删除；任何新增时间派生控制字段都必须提供 typed sidecar和显式 normalizer，禁止直接忽略 hash。

## 2026-07-14 07:12 - Prompt Trace 需要从错误文本猜 Step revision 与审批漂移

- 修复时间：2026-07-14 07:12
- Bug 现象：Prompt Trace 要展示 `stepRevisionKind` 和 approval token mismatch field，但 canonical rollout 没有记录这两个 typed facts；若 UI 从 error/status 文本推断，会重新产生第二套控制解释链。
- 根因：Prompt event 只保存 model input hash/count，stale approval 虽在 Runtime 中生成 typed `ApprovalContextStaleError.MismatchFields`，重发 `approval_requested` 时却没有把字段提交到 canonical event。
- 修复方式：Prompt event 直接记录 StepReference transition 的 `stepRevisionKind(s)`；stale reissue 通过专用 recorder helper把 server-owned mismatch fields写入同一个 approval_requested event。Prompt Trace API 仅 allowlist 投影这些字段，UI 禁止从 outcome/errorClass/status 推断 divergence。
- 验证结果：RED 先证明首个 Prompt 缺 initial revision、nil-token reissue 缺 mismatchFields；GREEN 后 focused Runtime 测试重复 10 次通过，真实 approval/tool-not-found replay baseline 按新增 typed facts更新并恢复稳定通过。
- 风险与后续：新增 Prompt payload 会有意改变 canonical event hash；所有持久 replay baseline 必须随 schema 内事实扩充显式审查更新，不能由测试运行时自基线覆盖。

## 2026-07-14 11:20 - Deprecated Prompt shadow 参与生产控制与哈希

- 修复时间：2026-07-14 11:20
- Bug 现象：Prompt v2 已切为生产输入后，deprecated shadow parity 仍可能影响 provider 前校验，并把 legacy 对比结果混入 Step control hash；旧路径的漂移会让真实执行被阻断或导致同一 v2 输入得到不同控制身份。
- 根因：观测用 shadow trace 与生产 Prompt/Step 控制边界没有彻底隔离，parity 结果既被消费为 gate，也参与持久控制哈希。
- 修复方式：shadow 只保留为只读诊断 trace；provider、dispatcher、StepReference 和 replay 控制身份只消费 PromptEnvelope v2 及其 typed facts，禁止 shadow 决定运行结果。
- 验证结果：新增 cutover/hash 矩阵覆盖 shadow 开关与内容变化，确认 provider request、Step hash 和工具面不变；`go test ./internal/promptcompiler ./internal/promptinput ./internal/runtimekernel -count=1` 通过。
- 风险与后续：shadow 字段仍可用于迁移审计，但不得重新出现在 production gate、provider request 或 hash 输入中。

## 2026-07-14 11:20 - AppUI 复制 Turn loop 并直接提交终态

- 修复时间：2026-07-14 11:20
- Bug 现象：RuntimeKernel 之外存在 `RunTurnWithRecorder/executeAgent` 复制循环，AppUI 的 migration、repair 与 workflow generation 分支还会自行构造 terminal `TurnSnapshot`；修复一个路径时容易让另一路径的 lifecycle、rollout、final facts 或 transport 不一致。
- 根因：缺少“RuntimeKernel 是唯一 lifecycle writer”的可执行边界，确定性系统回复没有正式 runtime gateway，调用层只能复制 turn commit 逻辑。
- 修复方式：删除复制 turn loop；新增 `CommitSystemTurn`，由 RuntimeKernel 统一提交 user/assistant、checkpoint、final facts、canonical transport 与 owner trace；三个 AppUI 分支全部改走该入口，并以 AST boundary test 禁止生产 AppUI 构造或写入 `TurnSnapshot/CurrentTurn/TurnHistory`。
- 验证结果：`go test ./internal/runtimekernel ./internal/appui -count=1` 通过；hardening gate 固定执行 `TestAppUIRuntimeLifecycleHasUniqueWriter`，重复引入第二 writer 会直接失败。
- 风险与后续：SystemTurn 只接受 partial/blocked/needs_evidence 与有限 domain facts，不能借此伪造 verified、approval_denied、tool_unavailable、model 或 tool 执行事实。

## 2026-07-14 11:20 - Mutation 工具可在缺少声明式回滚时进入 Provider

- 修复时间：2026-07-14 11:20
- Bug 现象：工具注册了 mutation/approval 元数据但未声明可验证回滚能力时，模型仍能看到并选择该工具，直到执行阶段才暴露回滚问题。
- 根因：ToolSurface 指纹与 TurnAssembly admission 没有把 rollback readiness 作为生产控制事实，回滚校验发生得过晚。
- 修复方式：Tool metadata 增加声明式 rollback strategy/reference；ToolSurface fingerprint 纳入 readiness；TurnAssembly 在 provider sampling 前 fail closed，缺少声明的 mutation 不进入模型输入、审批或 dispatcher。
- 验证结果：公开 `mutation_missing_rollback` AssistantTransport story 断言 providerCallCount=0、无 visible/actual tools、turn failed，并保留 canonical `system/failed` 错误块；Runtime golden 与完整 story corpus 均通过。
- 风险与后续：声明式 ready 只证明存在回滚契约，不证明回滚执行成功；真实 mutation 仍必须经过 ActionToken、permission、approval 和 post-check。

## 2026-07-14 11:20 - AssistantTransport 同时暴露 process/final 与 blocks 双协议

- 修复时间：2026-07-14 11:20
- Bug 现象：Go wire、流式 delta、converter metadata 和 React 同时消费 `process/final` 与 `blockOrder/blocksById`，同一回答存在两套顺序与终态来源；更新 A 路径会让 B 路径出现错序、重复或 FinalContract 不同步。
- 根因：canonical transcript 没有贯穿 `TurnItem -> TransportProjector -> AssistantTransport -> React`，前端还依赖 `metadata.unstable_state` 作为第二状态通道。
- 修复方式：生产 wire 仅输出 ordered blocks；server append-text 只更新 final block text，并保证完整 FinalContract 的 text/answerText 同步；converter 只在边界一次性迁移旧缓存，React 仅消费 typed data parts，Protocol 页面复用同一 block 顺序。
- 验证结果：Go JSON 测试断言 wire 不含 legacy `process/final`；Server delta、Web converter/runtime/render tests 和 124 files/905 tests 全部通过；Playwright snapshot fixture 已切为 native canonical blocks。
- 风险与后续：TypeScript 类型暂保留 legacy 字段作为旧缓存输入，兼容投影只能存在于 converter 边界，任何生产 writer 或 React consumer 重新读取它们都应由门禁拒绝。

## 2026-07-14 11:20 - AssistantTransport 审批故事在异步恢复完成前断言

- 修复时间：2026-07-14 11:20
- Bug 现象：approval decision HTTP 先返回 accepted、Runtime 随后异步执行 ResumeTurn；故事 runner 立即断言旧 projection，造成 approval resume/denied 偶发失败或错误刷新。
- 根因：测试 harness 把 command ack 当作 runtime terminal，把传输层接收语义和执行完成语义混为一谈。
- 修复方式：审批命令后轮询 published session snapshot 直到 canonical turn terminal，再通过空 command 请求刷新 transport projection；所有故事断言统一从 blocks 中读取消息、过程和 FinalContract。
- 验证结果：`go test ./internal/server -run '^TestAssistantTransportStories$' -count=1` 与 semantic shell corpus 通过，approval resume/denied、partial mutation、missing rollback 等场景稳定使用真实 runtime/transport 链。
- 风险与后续：轮询只用于测试 harness，不改变生产 ack 语义；UI 仍应依据后续 AssistantTransport state/delta 展示执行进度与终态。

## 2026-07-14 11:46 - Canonical blocks 切换后过程头、产物卡和普通工具输出回归

- 修复时间：2026-07-14 11:46
- Bug 现象：真实 Playwright 页面验证发现三类回归：尚未出现 tool block、只有 running final block 时缺少“过程”头；连续的 Ops Manual 搜索与预检产物被拆成两张卡；普通 command/tool 输出只要包含数字 `401` 或文字 `upstream timeout` 就被误渲染为传输失败。
- 根因：React 展示层把 canonical block 的物理边界直接当作卡片边界，且 running final 没有生成过程占位；`normalizeProcessTypedContent` 对所有 typed output 使用宽泛的字符串错误启发式，把业务输出内容误当成 transport error。
- 修复方式：pending turn 在只有 running final 时仍投影过程头；仅在 React presentation 层合并相邻、语义匹配的 Ops Manual search/preflight artifact，控制数据仍保持原始 canonical block；transport error 文案归一化只处理 assistant/system failed 文本，command/tool typed content 保持原样。
- 验证结果：新增/更新 Vitest 与 native canonical block Playwright fixture；`npm --prefix web test -- --run` 通过 124 个测试文件、907 个测试；`npx playwright test tests/react-shell-snapshot.spec.js tests/agentHarnessPromptTrace.snapshot.spec.js --project=chromium` 通过 13/13；browser-in-app 真实打开 Prompt Trace 详情并切换控制链，DOM 断言和视觉截图通过，控制台 0 条 error/warn。
- 风险与后续：artifact 合并只改变展示组合，不写回 transport 或 control facts；错误归一化继续对明确的 assistant/system failed block 生效。后续新增 typed block 必须显式定义展示语义，不能恢复跨类型的全文关键词猜测。
