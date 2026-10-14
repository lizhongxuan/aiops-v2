# Fix Bug 记录

## 2026-10-14 03:56 - 待审批 Runner 操作可被新 turn 覆盖，确认后界面未显示处理中

- 修复时间：2026-10-14 03:56 UTC
- Bug 现象：已有非终态 pending approval/evidence 的会话若再收到普通 `RunTurn`，会继续 admission 并可能替换当前 turn；Runner Candidate Apply 在用户确认后也没有对所有 approval 来源一致显示等待状态，重复确认可发出多次决策，焦点可能落在已禁用按钮上。
- 根因：`RuntimeKernel.RunTurn` 只把 running regular turn 视为可追加输入，没有先拦截挂起的 controlled approval/evidence turn；Candidate Apply 的 artifact 分支在更新 pending 状态前提前返回，只有 server-controlled 分支做了防重复。
- 修复方式：`runtime_kernel.go` 在新 turn admission 前阻止覆盖非终态 controlled/pending approval/pending evidence 状态并返回原 turn ID；Runner Apply 统一 artifact/server-controlled decision 的身份去重和待处理状态，等待权威 transport 消费/移除 approval 后才恢复，pending 状态提供可访问提示和焦点目标。
- 验证结果：回归用例覆盖两种 pending 状态，保证原 turn/approval 未被覆盖、无重复消息且未执行 mutation；`go test ./internal/runtimekernel ./internal/appui ./internal/server -count=1` 通过，Runner 两个 ActionBar Vitest `36/36`、`npm run typecheck`、Chromium Apply pending Playwright `1/1` 通过；snapshot 已目检。完整命令与环境限制见实施清单第 7 节。
- 风险与后续：跨进程/多实例 admission 未在本次验证。Runner UI 仍依赖服务端 transport 权威 approval 状态；本次截图只验证 Chromium 的该 fixture。

## 2026-07-21 - Workflow 只读请求被 revision 哈希中的 `df` 误判为主机执行

- 修复时间：2026-07-21
- Bug 现象：真实 full-stack HARNESS-003 偶发把 `workflow.get_context` 只读请求终止为“缺少明确且已验证的执行目标”。本次触发 revision 为十六进制 digest，其中恰好包含字符对 `df`。
- 根因：Intent classifier 对 `df/top/free` 等短命令使用任意子串匹配，同时 Workflow Agent 只在 metadata 禁用了 host tool，没有清理已经推导出的 `local_runtime/host_exec` typed intent；冻结的 Workflow artifact 也没有投影成 admission 可识别的 verified resource binding。
- 修复方式：短 ASCII 命令改为完整 token 匹配，opaque ID/digest 不再参与子串命中；Workflow Agent 入口确定性移除 host-only scope/risk/capability 并重新计算 mutation policy；认证后的 tenant/workflow context 同时生成 verified `workflow` resource binding，写意图仍保留 `write` 风险和审批，主机执行继续禁止。
- 验证结果：精确 digest 回归测试、Workflow route/Turn context/resource binding 定向测试通过；Root 相关全包与 vet 通过；真实 full-stack 重跑为 `3 passed / 33 planned skipped`，HARNESS-003 完成 `Workflow panel -> RuntimeKernel -> workflow.get_context -> final -> reload resume`。
- 风险与后续：token classifier 仍是规则式预分类，只用于能力/风险候选，不得代替 ToolDispatcher 和 Workflow Application 的可信授权；未来增加短命令词时必须继续使用 token 边界测试。

## 2026-07-21 - Workflow 真实浏览器回归暴露动作目录、快速删除与 Undo 候选契约分裂

- 修复时间：2026-07-21
- Bug 现象：Runner 动作面板会显示服务端实际不允许保存的 `tcp_ping`；用户快速从 Shell 节点切到 Python 节点后立即按 Delete，可能删掉上一个节点；History Undo 虽能返回合法 Candidate，前端却先以“无效预览”拒绝，修正预览解析后“放弃修改”仍没有请求服务端。
- 根因：动作目录只依据 Catalog 项存在性，没有与 Runtime `allowed_actions` 做 fail-closed 交集；Inspector 用 passive effect 切换草稿，选择变更到 effect 执行间保留了旧节点的破坏性 callback；Undo 的服务端 inverse 对可选空字段和 `clientRef` 使用了内部合法表达，而浏览器只接受严格外部 Patch 表达，且 `validation.basis.semanticHash` 实际描述候选投影，Workspace 却把它错误地与 committed semanticHash 比较。
- 修复方式：Runner API 明确投影规范化 `allowed_actions`，前端 Catalog 只展示同时有权限且可执行的动作；Inspector 在 layout effect 中同步切换节点草稿；Undo artifact 边界把服务端 inverse 规范化成严格请求形态并保留未知字段拒绝，Runner 后续 inverse 同时写稳定 `clientRef`；Candidate freshness 继续以不可变 `changeSet.base` 全 revision 和 validation basis resourceVersion 绑定，不再混淆候选与 committed semanticHash，且同步绑定失败会显示可恢复错误。
- 验证结果：内置浏览器实测面板只剩 Shell/Python、Add/Move/Delete 均落到权威 revision，快速切换后立即 Delete 删除了当前 Python 而保留 Shell；History Undo 正确显示 4 个 ghost/diff 操作。Web 定向 Catalog/Inspector/Artifact/Workspace 测试与 typecheck 通过，其中 Workspace/Artifact 共 `28/28`；Runner API/Application/Edit 定向测试通过。最终全量三浏览器、Go 和门禁结果记录在 Workflow 实施 TODO。
- 风险与后续：Undo/Redo Candidate 的最终 Apply 仍必须由同一 Transport turn 提供精确 Grant/Approval；History 面板不能自行签发批准。该完整高风险 Undo/Redo 旅程仍保持为未完成验收项。

## 2026-07-21 - 动态 Workflow ResourceLock 接受非规范资源键

- 修复时间：2026-07-21
- Bug 现象：动态 Workflow MCP 调用传入带前导或尾随空白的资源锁键时，Dispatcher 会先 Trim 再接受并实际执行；调用参数本身并非 canonical key，却能进入与规范键相同的互斥区，破坏了“非规范 trusted scope 必须 fail closed”的安全契约。
- 根因：`internal/runtimekernel/mutation_safety.go` 的动态 `ResourceLockResolver` 只校验了 Trim 后的结果，没有比较 resolver 原始返回值与规范化值，因此把输入纠正误当成了输入验证。
- 修复方式：动态锁解析后同时校验原值、规范值和 trusted tenant/workflow binding；原始值与 canonical 表达不完全一致、为空或越过可信租户范围时在 acquire 和 handler 前拒绝。静态注册与动态 MCP adapter 共用同一 Dispatcher safety gate，annotation 仍仅是提示，不能覆盖授权或 verified approval。
- 验证结果：新增的非规范空白键红测在修复前会实际调用 handler，修复后返回安全错误且 handler 未执行；已运行定向 workfloweditor/tooling race、runtimekernel/workfloweditor/tooling/mcp-runtime 全包非 race 和对应 `go vet`，均 exit 0；另有静态/动态 MCP、伪造 Grant/Approval/reference、跨租户锁、结果预算外置和 CommitStore 双实例 CAS 回归覆盖。
- 风险与后续：全量 `internal/runtimekernel` race 仍会命中 3 个既有 streaming 测试对 `CurrentTurn/AgentItems` 的无锁读取，和本次动态锁调用链无重叠；最终总门禁前需独立修复或将其作为明确基线处理。

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

## 2026-07-14 12:13 - 真实 no-tool 终态被 unknown 证据状态误判为仍在运行

- 修复时间：2026-07-14 12:13
- Bug 现象：最终代码二进制调用真实 GLM-5.1 后，Runtime 已原子提交 completed turn、final_response 和完整回答，AssistantTransport 也已清空 liveness；但 eval 报“stream ended before typed terminal”。同一 final block 还同时显示 `streamState=complete` 与 `status=running`。
- 根因：`FinalContract.status=unknown` 在 simple-read/no-tool 场景表示合法的“未做证据验证”结论，不是执行生命周期；AppUI 却把它映射为 running process，eval 终态集合也排除了 unknown，把证据分类和 lifecycle 混为一谈。
- 修复方式：canonical final block 保留 `finalContract.status=unknown` 作为事实分类，同时将 block process status 映射为 completed；eval 将 unknown 视为合法终态，但仍要求 turn completed/failed/canceled/blocked、transport state、pending、liveness、active tool 全部一致，不能靠 unknown 单独宣告完成。
- 验证结果：两个 RED 分别稳定观察到 `running/complete` 矛盾和 typed terminal error；修复后 focused AppUI/Eval 测试与 `go test ./internal/appui ./internal/eval -count=1` 通过。真实 GLM 最终基线冒烟在重建二进制后复验。
- 风险与后续：empty/missing finalContract、running turn、active tool、pending approval/evidence 和非空 liveness 仍 fail closed；unknown 仅表示最终回答未被验证，不再承担流式生命周期语义。

## 2026-07-14 17:51 - Coroot 特殊输入丢失、过程文案重复和失败工具循环

- 修复时间：2026-07-14 17:51
- Bug 现象：用户输入“帮我看下@Coroot 情况”并按 Enter 后，特殊标签和 Coroot route metadata 丢失；工具前说明同时出现在过程区与输入框上方的可变底部区域，运行结束后底部副本消失；`read_context_artifact` 在同一类 `context artifact not found` 失败下连续换 ID 重试四次。
- 根因：Composer 的自定义发送按钮是唯一 metadata 组装入口，但 `ComposerPrimitive.Input submitOnEnter` 走了框架默认提交；特殊 mention 正则又把中文字符当作禁止边界。即使只补 legacy Coroot flags，服务端 typed admission 仍会因为缺少可信的 `aiops.input.mentions.v1` capability binding 而覆盖路由。TransportProjector 每次重放完整 canonical snapshot 时只清空 Process，未清空旧的 running Final，造成上一帧草稿残留。failure signature 的 seen count 只在单个 tool call 内统计，新的 call ID 会重新从 1 开始；此外 `read_context_artifact` 的 4096 字节 inline budget 小于其最大 bounded read 加结构化包装后的 4900 字节，工具结果会递归 spill 成新的 artifact，诱发模型用不同格式反复读取。
- 修复方式：把按钮与 Enter 统一到同一个 `submitComposerMessage`，由它一次性组装 typed mention/host/capability metadata，并为直接输入的 allowlisted `@Coroot` 生成可信的 `aiops.input.mentions.v1` capability binding；Shift+Enter 保留换行、输入法 composing 不提交，并允许中文紧邻特殊 mention，同时继续拒绝 ASCII 单词/邮箱内误触发。TransportProjector 在重放 snapshot 前同时清空 Process 与 Final，只由当前 canonical AgentItems 重建。RuntimeKernel 在 turn 范围统计规范化 failure signature；第三次同类失败触发既有 `switch_path` 规则，把 guidance 写入 tool result 并从下一 Step 工具面隐藏该工具；context artifact inline result budget 调整为 8192 字节，确保最大 bounded read 连同结构化包装仍可直接返回。
- 验证结果：三项 RED 分别复现 Enter 无 command、旧 running Final 残留、第三次失败后工具仍可见；context artifact 额外 RED 证明 4096 字节预算会把 4900 字节的 bounded result 再次 spill。GREEN 后 `go test ./internal/runtimekernel ./internal/appui -count=1`、Web typecheck 和 124 个 Vitest 文件/908 tests 全部通过；Playwright 用中文紧邻的 `@Coroot`、真实 Enter 操作验证特殊标签与 typed metadata，focused browser suite 3/3 通过。最新生产二进制的全新真实回合中，服务端确认 `mentionValidation=confirmed`、Coroot route/tool allowed 均为 true；Coroot 登录超时后只调用一次 `coroot_list_services` 即输出受限终态。内置浏览器展开 completed process 后过程文案计数 1、失败工具计数 1、`read_context_artifact` 计数 0，console error/warn 为 0。
- 风险与后续：
入第二套 timeline 或 route 事实源。

## 2026-07-16 17:14 - AI Chat 未分类草稿提前成为 final、过程错序并重复展示

- 修复时间：2026-07-16 17:14（Asia/Shanghai）。
- Bug 现象与影响：模型仍在生成或准备调用工具时，未分类草稿会先出现在最终回答区；`model_call`/reasoning 等 trace 被提升为普通聊天过程；tool、artifact、final 按类型分桶后可能错序；commentary、工具卡与成功态 FinalContract 又会重复表达同一事实。无工具、单工具、多工具、retry、stream error、cancel、approval、artifact 和失败工具场景均可能出现闪烁、重复或顺序混乱。
- 已确认根因：Runtime 在 provider 完成分类前写入 `phase=final_answer`；Transport 没有集中 typed visibility policy，并在收尾时按类型重建 `blockOrder`；commentary 与 tool 缺少稳定 action identity；React/converter 通过可见文本、业务关键词和 JSON 行修复上游协议问题，形成多套 final/process 语义。
- 修复方式：Runtime 将增量草稿持久化为不可见的 `unclassified`，只在结构化 response facts、evidence boundary 和唯一 terminal commit 后提交 final；Transport 以 typed policy 隐藏 trace/retry draft，并按 block 第一次可见位置增量登记顺序，tool call/result 原位更新、artifact 紧跟来源；commentary 与 tool 通过 `toolCallIds`、`displayKind`、`foldGroupId` 归组；React 只渲染 canonical typed blocks，正常 verified final 隐藏独立成功卡，异常限制继续可见。P0 门禁同时修复了 case ID 被 secret scanner 误报为 API key 的边界问题。
- 关键修改：`internal/runtimekernel/assistant_output_commit.go`、`assistant_message_boundary.go`、`runtime_kernel.go`、`internal/appui/transport_presentation_policy.go`、`transport_block_order.go`、`transport_process_grouping.go`、`transport_projector.go`、`web/src/transport/aiopsTransportConverter.ts`、`web/src/chat/components/AiopsThread.tsx`、`ProcessTranscript.tsx`；故事链、Vitest、Playwright fixture/screenshot 和 rollout replay golden 均同步更新。
- 验证结果：新增测试先复现再修复；`go test ./... -count=1`、124 个 Vitest 文件/915 tests、typecheck、build、Playwright 23/23、artifact 顺序 screenshot 10/10、boundary/change-budget self-test 与固定基线 gate、`scripts/aichat-harness-hardening-gate.sh` 均以 exit code 0 通过。P0 self-opt run `self-opt-20260716T091424Z-1` 的真实 AIOps core 8/8、synthetic 5/5、selfopt 6/6，`overall=1`、`gate=pass`、`worse=0`，secret scan 通过。
- 真实页面验收：终端 Playwright 与应用内浏览器均使用真实 provider 操作页面。普通咨询为 1 final/0 process/0 tool；单工具为 1 final/1 process group/1 tool；双工具为 1 final/1 process group/2 tools。刷新与历史恢复后正文和分组一致，浏览器 console 为 0 error/0 warning；运行中未出现未分类 final，完成后的过程默认折叠。
- 剩余风险：前端 build 仍有既有的大 chunk 警告，`npm ci` 报告 16 个既有依赖审计项（4 high）；要求执行证据但未生成 VerificationReport 的只读主机回答会按设计显示可行动的 `needs_evidence` 限制卡。依赖升级不属于本计划范围。未变更 provider 协议、工具路由、permission、approval、evidence gate 或 ToolDispatcher；未记录凭据、主机敏感信息或完整高风险命令输出。

## 2026-07-21 17:54 - Workflow 批量同类操作被误判为重复审批消费

- 修复时间：2026-07-21 17:54（Asia/Shanghai）。
- Bug 现象：一个合法 ChangeSet 含两个 `update_node` 等同类 operation 时，旧 HTTP 兼容 Apply 在审批消费阶段返回 duplicate operation，或用去重后的 kind 数错误执行最大操作数预算；真实批量修改无法原子应用。
- 根因：审批 envelope 把 operation kind 集合同时当成“授权类别”和“实际操作数量”，同类 operation 既会触发重复值校验，又使数量预算失真。
- 修复方式：审批绑定保存稳定排序且去重的 operation kind 集合，独立持久化 `OperationCount` 作为实际数量预算；读取旧 envelope 时保留兼容 fallback。兼容 HTTP 仍只生成一次 Preview → Approval → Apply/Receipt 原子事务，不拆分或重放子操作。
- 验证结果：RED 稳定复现 `duplicate workflow approval consumption operation`；修复后 Runner approval/apply/legacy 定向 race、service 全包、旧 preview/apply → 新 manual → reload → replay 集成故事、root HTTP boundary 和 vet 均通过。
- 风险与后续：旧 envelope 没有 `OperationCount` 时只能回退到 kind 数；新写入全部携带真实数量。operation kind 仍是授权范围而不是操作顺序，精确顺序由不可变 ChangeSet digest 约束。

## 2026-07-21 17:54 - Workflow Agent 工具面与 typed 失败在 Runtime 链路中丢失

- 修复时间：2026-07-21 17:54（Asia/Shanghai）。
- Bug 现象：真实 Chat→RuntimeKernel→Dispatcher→Workflow MCP 故事中，`workflow.apply_patch` 因 permission scope 未被识别而不出现在工具面；workflow route 同时混入 public web/ops 工具。即使 handler 返回合法 typed DomainError/Receipt，Dispatcher、model feedback 或 AgentItem 仍会把它覆盖为 `side_effect_unknown` 或只保留 tool identity，导致模型和页面失去可恢复事实。
- 根因：Workflow apply 使用了未纳入通用 argument-scoped 规则的自定义 permission scope；workflow profile metadata 只做增量合并而非权威收敛；mutation safety 把所有 `isError` 一律当成未知副作用；`failedToolResultForModel` 与 approval resume AgentItem 没有保留已校验的 `structuredContent/isError`。
- 修复方式：Apply 使用通用 `argument_scoped` permission；workflow_agent profile 明确只启用 workflow_editor 并关闭 web/ops 检索。Dispatcher 先验证 structured result：合法 typed domain failure 原样传播，只有缺少可信结构的 mutation failure 才进入未知副作用保护；model feedback 与正常/审批恢复 AgentItem 统一写入 canonical structuredContent/isError。
- 验证结果：新增工具面、profile、typed mutation failure、model result、AgentItem 单测均先红后绿；完整生产内存故事覆盖 read→preview→approval checkpoint→apply→Receipt→AiopsTransportState，以及 expired/revoked grant、伪最终 transaction、Secret 脱敏。三个相关包全量、三组 race、vet 和 diff-check 全部通过。
- 风险与后续：typed result 只有在 output schema 验证通过后才被信任；无结构或非法结构的 mutation error 仍保守标记 side-effect unknown。页面 renderer 仍必须消费 typed artifact，不得重新解析 final 文本。

## 2026-07-21 17:54 - Workflow WAL 恢复顺序与主进程事件投影未完整接线

- 修复时间：2026-07-21 17:54（Asia/Shanghai）。
- Bug 现象：WAL 文件枚举顺序不稳定时，事件恢复可能按目录顺序而不是 commit generation 投影；同时 Runner Event Facade、Replay、Outbox 和 AppSnapshot projector 虽已实现，主进程未创建 Facade，也没有在用户打开 Workflow 后激活订阅，页面无法收到真实 committed event。
- 根因：恢复代码隐含依赖文件系统枚举顺序；事件模块只完成了组件级 API，production bootstrap 缺少 lifecycle owner 与 active WorkflowKey 的惰性绑定。
- 修复方式：WAL recovery 在投影前按 immutable snapshot generation 稳定排序；主进程在 Runner Application 创建后打开唯一 WorkflowEventFacade，在 authoritative graph 首次打开或 manual command 成功后按 trusted WorkflowKey 惰性绑定现有 AppSnapshot broadcaster，关闭时按 subscription→facade→Runner 顺序释放。投影失败不篡改已提交 mutation response，下一次打开通过 durable replay 恢复。
- 验证结果：乱序 WAL、replay/outbox、慢订阅、gap/snapshot resync、主 `/ws` AppSnapshot 投影、graph-open 单次绑定和 shutdown 的定向 race 测试均通过；Runner service/app、root appui/server、main wiring、vet 和 diff-check exit 0。
- 风险与后续：订阅仅为当前进程投影，持久事实仍在 Runner Event Log/outbox；页面不得直接订阅 feature-local WebSocket/SSE/polling。后续前端 Provider 只消费现有 AssistantTransport/AppSnapshot 事实。

## 2026-07-21 22:08 - 画布节点修改被重复编译投影误判为校验漂移

- 修复时间：2026-07-21 22:08（Asia/Shanghai）。
- Bug 现象：用户在节点配置中修改 Shell 脚本并点击保存，服务端返回 `compiled workflow does not match the validated graph`；页面正确显示“未保存”，刷新后修改丢失。新增节点和首次保存正常，因此只有已从 YAML 回读、同时带有 visual node 与 compiled workflow step 两份投影的 Graph 会稳定复现。
- 根因：semantic hash 同时计算权威 visual node 的 `Step/Handler` 与 `Workflow.Steps/Handlers` 编译副本。画布先更新 node，下一次 compile 才重建副本，导致同一语义在 compile 前后得到不同 hash，并被 pre-commit gate 当成陈旧或篡改。
- 修复方式：semantic hash 只以 typed visual node/edge contract 为权威输入，排除可由其确定性再生的 `Workflow.Steps/Handlers`；layout hash 和其他 Workflow contract 字段保持不变。新增真实 compile → parse → node edit → recompile round-trip 回归，并验证仅修改陈旧编译副本不改变 identity。
- 验证结果：hash 定向测试、Apply/Manual race、Runner service 全包和 vet 均通过；部署后还必须用 browser-in-app 重复“编辑脚本→保存→刷新”验证持久化，未完成前不把手工编辑任务标为完成。
- 风险与后续：任何新增的 compiled projection 都必须进入同一字段分类表；不能通过放宽 pre-commit 校验掩盖 identity 漂移。

## 2026-07-21 22:08 - Workflow AI Stop 被活动流命令队列延迟，Follow AI 读取了错误状态源

- 修复时间：2026-07-21 22:08（Asia/Shanghai）。
- Bug 现象：真实 Workflow AI 回合生成中点击“停止”后，回答仍继续到正常完成；同时 AI 面板显示“正在处理”，Dock 却提示 Follow AI 仅在生成期间可用。
- 根因：Stop 通过普通 `sendCommand(aiops.stop)` 进入当前活动流之后的命令队列，无法抢占正在执行的 run；Dock 又从 Workspace task 派生 generationActive，而不是读取页面唯一 AssistantTransport 的 activity。
- 修复方式：Stop 直接调用唯一 Assistant runtime 的 `thread.cancelRun()`，由现有 Transport onCancel 投影 canceled；在既有 WorkflowTransportBoundary 内增加只读 activity context，Dock 与 Panel 共同消费同一 Transport busy/canStop 事实，不复制 task state。
- 验证结果：新增红测先复现延迟 Stop 和双状态源，最终 Panel/Shell/Boundary/selectors/Page 六个文件 43 项定向测试、Web 全量 153 files/1095 tests、typecheck 与 build 通过；真实浏览器 Stop/Follow AI 仍需在新二进制部署后复验。
- 风险与后续：取消只改变当前 run lifecycle，不得另行构造 completed final；任何新页面容器必须复用既有 Boundary，不能创建第二个 Assistant runtime/provider。

## 2026-07-22 12:08 - History Candidate 无受控审批入口且 Apply 混淆 Runtime/Canvas 会话

- 修复时间：2026-07-22 12:08（Asia/Shanghai）。
- Bug 现象：History Undo/Redo 可以生成持久 Candidate，但页面无法通过正常 AssistantTransport 请求精确审批；补齐入口后，审批会成功而 Apply 返回 `workflow apply identity does not match trusted execution context`。
- 根因：History 页面缺少只接受 Candidate selector 的服务端受控回合入口；Apply adapter 又把 Runtime session 当成 Canvas session 传入业务校验，而二者在真实页面本来就是不同身份。
- 修复方式：新增严格的 Candidate Apply Approval endpoint，只从持久 Candidate 重建唯一 `workflow.apply_patch`，由 RuntimeKernel 创建 PendingApproval；浏览器只恢复对应 AssistantTransport。Apply 时 CanvasSession 只能取受信 Candidate，RuntimeSession 只用于执行/审批绑定。ActionToken、mutation hash、身份与审批字段均不下发浏览器。
- 验证结果：Go 覆盖伪造、stale、重启、并发、拒绝、过期与幂等；`PW-012` 真实完成两次持久编辑→Undo→精确审批 Apply→刷新→Redo→再次精确审批 Apply→刷新，两次请求体均无 mutation authority 字段。证据：`${TMPDIR}/workflow-canvas-evidence/full-stack-20260722T040727Z-48763/`，94 个文本文件脱敏扫描通过。
- 风险与后续：任何页面发起的 Apply 都只能选择服务端已有 Candidate，不能重新提交 changeSet/previewHash/expectedRevision；直接修改后的 Candidate 再授权仍由 PW-006 单独验收。

## 2026-07-22 12:08 - Receipt 与 AppSnapshot 先后到达时已提交 Candidate 残留为 stale ghost

- 修复时间：2026-07-22 12:08（Asia/Shanghai）。
- Bug 现象：Apply 已产生新 revision 且画布显示提交后的内容，但旧 Candidate 状态仍显示“候选预览已阻止：stale candidate basis”，误导用户认为修改尚未保存，并阻断后续 Redo。
- 根因：Workspace reducer 只在 `transaction.applied` 事件到达时清除 Candidate；Receipt 与 AppSnapshot 已经精确对账但事件投影延后或被 snapshot sequence 覆盖时，没有等价的确定性清理路径。
- 修复方式：Reducer 在 Receipt 和 committed snapshot 任一顺序到达时，以同一 workflow、精确 changeSet 和 revisionAfter 三重匹配清除 Candidate/approval；不相关、conflict 或 rejected Receipt 保留 Candidate。事件路径继续幂等生效。
- 验证结果：新增 reducer 双顺序与拒绝场景测试 7/7；`PW-012` 真实 Apply 后 Candidate ghost 消失，Undo/Redo 与两次刷新 revision 一致。
- 风险与后续：只允许 exact receipt/revision 对账清理，不能因为任意 committed revision 变化而丢弃可重算的冲突 Candidate。

## 2026-07-22 12:08 - 已请求审批无法停止且取消态被误报为过期

- 修复时间：2026-07-22 12:08（Asia/Shanghai）。
- Bug 现象：Workflow 工具流已经停在 requested approval 时，生成流不再 busy，页面因此隐藏“停止”；即使服务端取消，卡片仍可能按 expired 文案展示。
- 根因：可停止性只看活动流 busy，没有把 pending/requested/blocked approval 纳入；UI 又把 canceled 复用为 expired 展示状态。
- 修复方式：待审批状态始终提供停止；先取消活动 Assistant runtime，若流已阻塞结束则发送权威 `aiops.stop`，服务端持久化 canceled。Approval 卡新增独立 canceled 语义，移除确认/拒绝动作并明确“本次修改不会应用”。
- 验证结果：Panel/selector/card 定向测试通过；`PW-014` 真实创建审批、停止、确认 canceled、刷新后仍 canceled，committed revision 未变化。证据：`${TMPDIR}/workflow-canvas-evidence/full-stack-20260722T040750Z-49372/`，78 个文本文件脱敏扫描通过。
- 风险与后续：取消不得本地伪造终态或清除服务端 Candidate；用户后续仍可从持久 Candidate 发起新的精确审批。

## 2026-07-22 13:18 - 受控单工具回合把结构化工具失败标记为已完成

- 修复时间：2026-07-22 13:18（Asia/Shanghai）。
- Bug 现象：服务端受控创建的单工具回合在 handler 返回合法 `isError=true` 的 typed DomainError 时，工具结果虽保留错误内容，但 Invocation、Tool AgentItem 和回合终态仍可能显示 completed；页面会把真实失败误报成成功。
- 根因：受控回合的终态只检查 Go `error`，没有把已通过 schema 校验的 `RecordedToolResult.IsError` 纳入生命周期；通用 Runtime 写回 Invocation 时也遗漏了同一失败事实。
- 修复方式：受控回合和通用工具结果落库统一将 `IsError` 视为失败终态，保留 canonical structured content 给模型和 Transport，同时将 Invocation、Tool AgentItem 与回合 phase 对齐为 failed/error；重放同一 tool call 只读取已记录结果，绝不重新执行 mutation。
- 验证结果：新增 typed failure 与 replay 回归测试；`go test ./internal/runtimekernel` 和 `go test -race ./internal/runtimekernel` 分别通过（race 用时约 146 秒）。
- 风险与后续：只有通过 output schema 验证的 typed error 可作为可信 DomainError；结构缺失或不合法的 mutation 失败仍必须进入 side-effect unknown 保护，页面不得从自由文本猜测成功/失败。

## 2026-07-22 13:55 - Workflow Candidate 混用 Runtime 与 Canvas 会话导致修订/放弃被拒绝

- 修复时间：2026-07-22 13:55（Asia/Shanghai）。
- Bug 现象：Workflow Chat 能生成 Candidate，但用户随后在画布直接修订或放弃时返回 403；History 与 Agent Candidate 在同一页面表现不一致。
- 根因：工具预览把 Runtime session 写进持久 Candidate 的 `CanvasSessionID`，而浏览器后续使用真实 `canvas-*` 会话；可信 Turn scope 只携带 Runtime session，没有独立冻结服务端 Canvas session。
- 修复方式：Turn trusted scope、context binding、AssistantTransport metadata 与工具执行上下文分别携带 Canvas/Runtime 会话；Candidate 固定保存受信 CanvasSessionID，AgentSessionID 继续绑定 Runtime session，Apply/Revise/Abandon 不允许二者互相冒充；旧直接 Runtime artifact 只保留显式兼容 fallback。
- 验证结果：AppUI scope、AssistantTransport 伪造覆盖、workflow.preview_patch 身份分离及 server/integration 全包测试通过；真实 `PW-022/025` 完成精确字段修订及 Candidate 保留/放弃，组合证据 118 个文本文件脱敏通过。
- 风险与后续：任何新入口必须同时声明两类 session 身份及来源；浏览器不得提交或覆盖可信 Canvas/Runtime binding。

## 2026-07-22 13:55 - 边上插入节点为无端口旧节点制造虚假 handle

- 修复时间：2026-07-22 13:55（Asia/Shanghai）。
- Bug 现象：关闭 Workflow Agent capability 后，用户从可见边插入动作节点，服务端返回 `INVALID_PATCH`；原图端点本身没有声明端口时稳定复现。
- 根因：前端 graph adapter 无条件给原边端点写入 `next/in`，把 UI 便利默认值当成已声明的 typed port；服务端正确拒绝指向不存在 handle 的连线。
- 修复方式：只有端点节点确实声明对应 port 时才写 `source_port/target_port`；新插入 catalog 节点继续保留自身声明的 `in/next`，不放宽服务端 Graph 校验。
- 验证结果：adapter 定向测试 25/25 与 typecheck 通过；真实 `PW-026` 在 disabled rollout 下完成可见边插入、原子断线/加点/重连、持久化与校验，28 个证据文本脱敏通过。
- 风险与后续：其他自动连线入口也必须从权威节点 port 声明派生 handle，不能用节点类型猜测并写回协议字段。

## 2026-07-22 13:55 - 布局保存重编译镜像被误判为 Candidate 语义冲突

- 修复时间：2026-07-22 13:55（Asia/Shanghai）。
- Bug 现象：Agent Candidate 存在时，用户移动不相交节点后继续修订，服务端返回 `STALE_REVISION`、`conflictingPaths=["/graph"]`；semanticHash 未变化仍无法安全 rebase。
- 根因：rebase invariant 虽已排除权威 Graph 的 position/viewport，却仍哈希 `Workflow.XRunnerUI/XRunnerGraph/Steps/Handlers` 编译镜像。布局持久化会重建这些副本及归一化元数据，导致镜像变化冒充未分类 Graph 漂移。
- 修复方式：rebase invariant 排除可由 typed visual Graph 确定性生成的四类编译投影；语义仍由 semanticHash 约束，布局只允许已列出的 position/viewport 且必须与 Candidate touched path 不相交，Description 等未分类字段继续 fail closed。
- 验证结果：新增完整 YAML compile→parse→布局修改→compile→parse 回归；安全 rebase、同字段/port/semantic/未分类冲突矩阵、Runner service 全包/race/vet 均通过。真实 `PW-008` 返回 rebased Candidate v2，和 `PW-017/022/025` 组合 4/4 通过。
- 风险与后续：新增 compiled projection 必须同时进入 semantic hash 与 rebase invariant 的字段分类审计；不得通过忽略 `/graph` 冲突或放宽 touched path 来解决镜像漂移。

## 2026-07-22 14:14 - Assistant UI 临时 Thread ID 污染 Workflow 稳定会话

- 修复时间：2026-07-22 14:14（Asia/Shanghai）。
- Bug 现象：一次 Workflow Preview 成功、下一次 typed Tool 失败后，第三次 Preview Tool 已真实生成新 Candidate，但画布仍停在旧 Candidate；刷新 Transport 数据能看到新 artifact，前端却没有 reconcile。
- 根因：后续 add-message 把 Assistant UI 顶层 `__LOCALID_*` 临时 threadId 优先写回 Transport state，覆盖领域态稳定的 `workflow:<id>`。严格 selector 正确检测到 thread scope 不匹配并 fail closed，因此整轮 artifact 不进入 Workspace。
- 修复方式：current schema 与 legacy initial-state 分支都以已有 `state.ThreadID` 为权威，顶层 request threadId 只在状态为空时 fallback；不放宽前端 workflow/thread/session 隔离规则。
- 验证结果：Go 回归覆盖稳定 state + 临时 request ID 的后续消息，Web Panel 覆盖两个 distinct revision-one Candidate 依次 reconcile，Reducer 覆盖同 revision 不同 candidate/previewHash 的正确替换；定向 Go、Web 22 tests 与 typecheck 通过。
- 风险与后续：框架本地 ID 只能参与尚未建立领域 thread 的首次初始化；一旦服务端已发布稳定 thread，所有后续命令、流式 delta 和 selector 必须保持该身份。

## 2026-07-22 14:23 - 大型 Candidate 审阅导航把用户错误带离 Review 且无法定位布局操作

- 修复时间：2026-07-22 14:23（Asia/Shanghai）。
- Bug 现象：用户在 100 项 Candidate 中点击“上一项/下一项”或某条变更后，右侧 Dock 会从 Review 自动切到 Configuration；遇到 `update_layout` 时又没有可聚焦对象，连续审阅在后半段中断。
- 根因：页面把所有 Workflow focus event 都解释成画布显式选择，没有区分事件来自 Review 还是 Canvas；Review 的定位器只识别 node/edge/update-node 等操作，没有为 `update_layout` 从 payload 选择稳定节点。
- 修复方式：focus 事件保留来源语义：Review 发起的定位保持 Review tab，用户显式点击候选画布节点才进入 Configuration；`update_layout` 按 payload 中首个稳定节点精确聚焦，不从自由文本猜测对象。
- 验证结果：RunnerStudioPage 与 WorkflowChangeReview 定向测试 30/30、typecheck 和语法检查通过；真实 `PW-028` 在 W4 的 500 节点/700 连线、100 项 Candidate、36 次真实 revise 下连续执行 99 次 Next 到 100/100，最终定位 `node-064`，committed Graph 始终不变并满足交互性能阈值。证据：`${TMPDIR}/workflow-canvas-evidence/full-stack-20260722T060944Z-31300/`，66 个文本文件脱敏扫描通过。
- 风险与后续：后续新增可定位 operation 必须在 typed operation→对象映射中显式定义；不能再次把“定位对象”和“切换工作区标签”耦合，也不能用 operation 文案解析作为 fallback。

## 2026-07-22 15:16 - 修订后的 Agent Candidate 无法按最后版本精确再授权

- 修复时间：2026-07-22 15:16（Asia/Shanghai）。
- Bug 现象：Agent 生成 Candidate v1 后，用户在 Inspector 和画布把它依次修订为 v2/v3；页面能显示最新版，却无法通过受控审批入口应用，或服务端错误地把 Candidate 新语义与旧 Base 语义比较后拒绝。
- 根因：历史审批入口只接受 revision 1，且没有从持久 store 重建完整 parent/operation lineage；审批校验还错误要求 Candidate validation semantic hash 等于 committed base semantic hash，而合法语义修改本来就应产生新 hash。
- 修复方式：只允许 `history-<64hex>/v1` 或 `candidate-<24hex>/v>=2` 的严格 selector；服务端从 store 重建同一身份、同一 base、连续 parent、normalized operation 前缀和完整 ValidationBasis，并对最新 Candidate 重算 PreviewHash。祖先统一投影 superseded，旧 revision 不能获得 Apply 资格。
- 验证结果：正负 Go 用例覆盖断链、伪造、越权、非法/空 semantic hash 与 unsafe revision；`PW-006` 真实完成 v1→Inspector v2→ArrowRight v3→Meta+Enter 精确审批→Apply→刷新，只有 v3 被批准且脚本/位置均持久化。证据：`${TMPDIR}/workflow-canvas-evidence/full-stack-20260722T070614Z-6368/`，44 个文本文件脱敏扫描通过。
- 风险与后续：浏览器永远只能提交 Candidate selector，不能提交 lineage、operations、hash 或任何 mutation authority；新增 Candidate 来源必须有独立、可审计的 ID 语法和同等重建校验。

## 2026-07-22 15:16 - 同一 Runtime 审批在 blocked tool 与 approval block 上重复渲染

- 修复时间：2026-07-22 15:16（Asia/Shanghai）。
- Bug 现象：Apply 已只创建一个 PendingApproval，但页面同时在 blocked tool 行和 canonical approval 行各显示一张相同确认卡，用户会误以为需要确认两次。
- 根因：每个 ordered block 都调用带 turn-level fallback 的 approval selector；没有 `approvalId` 的 blocked tool 因而取到本轮第一个 pending approval，随后显式 approval block 又按 ID 取到同一事实。
- 修复方式：approvalId 存在时只由对应 block 消费；同一轮已有显式 approval owner 时，其他 block 禁止使用 fallback。为旧数据保留唯一恢复路径：只有 blocked tool、没有 approval block 时仍在该 tool 上显示一张可操作卡。
- 验证结果：新增“tool + explicit approval = 1 card”和“tool-only recovery = 1 card”两条组件回归，Panel 16/16、typecheck 通过；最终 `PW-006` 真实页面只出现一张审批卡并成功确认。
- 风险与后续：多个并行 approval 必须各自携带唯一 approvalId；turn-level fallback 仅用于单一 owner 的兼容恢复，不能作为常规 block 关联机制。

## 2026-07-22 15:16 - 最终门禁被测试日志竞争和 Node/Vitest 重复收集干扰

- 修复时间：2026-07-22 15:16（Asia/Shanghai）。
- Bug 现象：Runner API race 在审计测试中报告并发读写 `bytes.Buffer`；Web 全量 1166 个断言全部通过，Vitest 却因两个 `node:test` helper 文件没有 Vitest suite 而退出 1。
- 根因：审计 helper 把异步 RunService 与 HTTP 日志写入无锁 Buffer，同时测试读取 `String()`；另两个真实 Node HTTP/process helper 自测沿用 `.test.js` 命名，却未从 Vitest 收集范围排除。
- 修复方式：审计 sink 改为读写共享 RWMutex 的 `lockedAuditLogBuffer`；Vitest 精确排除两项 Node helper，自测继续由 full-stack 启动器和显式 `node --test` 执行，未删除或跳过断言。
- 验证结果：Runner API 及五包 race 全部通过；Node helper 8/8、Vitest 155 files/1166 tests、typecheck/build 均通过。
- 风险与后续：任何异步日志捕获都必须让 Write/String 使用同一同步原语；新增非 Vitest 自测要使用明确命名或同步更新测试路由，避免同一文件被错误 runner 收集。

## 2026-07-22 19:13 - 固定结构大纲挤压工作流画布且无法收起

- 修复时间：2026-07-22 19:13（Asia/Shanghai）。
- Bug 现象：桌面 Workflow 编辑器同时显示左侧结构大纲和右侧 ContextDock 时，中间画布被明显压缩；在 `1095×863` 真实页面中，结构大纲固定占用 `216px`，画布只剩 `429px`，用户没有释放空间的入口。
- 根因：`WorkflowContextDock` 的桌面网格把 Outline 写死为 `216px`，没有折叠状态、恢复控制或对应 Canvas resize 通知；移动端虽隐藏 Outline，但桌面无法按任务需要调整这一列。
- 修复方式：由唯一 `WorkflowContextDock` 持有桌面 Outline 折叠状态；网格列宽改由 CSS 变量控制，收起时精确变为 `0px`，大纲进入 `inert + aria-hidden + visibility:hidden`，保留不参与网格宽度的浮动恢复按钮。按钮使用原生 button、`aria-expanded/aria-controls` 和可见 focus ring，状态变化复用统一 Canvas resize 事件；移动端仍使用原有单列模式。
- 验证结果：组件回归 red→green `10/10`、Web 全量 `155 files / 1168 tests`、typecheck、production build、展开/收起两张 Playwright screenshot baseline 通过；响应式 suite 在 Chromium/Firefox/WebKit `27/27`。重新部署后在 Codex 内置浏览器 `1095×863` 实测 Outline `216→0px`、Canvas `429→645px`，恢复后回到 `429px`，焦点保持在切换按钮，console log 与错误层均为 0。
- 风险与后续：折叠选择当前只保留在本次页面生命周期，刷新后默认展开；若后续需要跨会话记忆，应使用版本化用户偏好 schema，不能散落未治理的 localStorage key。完整 200% zoom、读屏人工主路径仍归 T3.12 统一验收。
