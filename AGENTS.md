# 仓库执行指令

## 硬规则摘要

在修改代码前先看到关键约束。这些是硬规则，不是建议。

### 通用 AIOps 能力优先

- `aiops-v2` 是通用 AIOps runtime，不是某个场景专用机器人。示例 case 只能作为验证样本。
- 不要在核心行为中硬编码中间件、厂商、服务名、主机名、namespace、cluster、故障名、固定拓扑或演示 case。
- 产品特定行为应放在 plugin、skill、MCP adapter、capability pack、Runner action、renderer、fixture、eval case 或文档中。
- Provider 特定逻辑必须限制在对应 plugin/capability pack 内，不能把 provider 名称泄漏到核心决策路径。

### 禁止在核心代码中做场景补丁式字符串语义

- 核心代码不得用关键字表或 `strings.Contains(...)` 这类业务字符串语义来决定用户意图、tool surface、证据类型、路由模式、任务深度、final 完整性、web search 或 ops manual 执行。
- 业务语义必须通过结构化契约表达，例如 `IntentFrame`、`OperationFrame`、`EvidenceEnvelope`、tool metadata、runtime facts、capability metadata 和 policy outputs。
- 固定字符串解析只允许用于机器边界，例如 URL/path/host mention/secret 格式、schema/version 解析、命令安全 allow/deny、sandbox/approval/policy、redaction 和 fixture assertions。
- 如果确实无法避免启发式判断，必须限制在 plugin/capability/adapter 范围内，只返回候选信号而不是最终决策，并用正例、反例、跨产品 eval 覆盖，同时说明为什么不能用 schema/tool metadata/model classification/policy 替代。
- Code review 必须拒绝新增的 “样例关键字 -> 能力路径” 补丁。

### Runtime 主链

- `runtimekernel` 是唯一的 turn lifecycle driver。不要新增第二套 turn loop、workspace runtime path、tool execution bypass 或 feature-local executor path。
- 标准 action 链路是：

```text
model -> assistant/tool_use checkpoint -> ToolDispatcher -> policy/permission/approval -> tool_result checkpoint -> next iteration
```

- `NeedApproval`、`NeedEvidence`、blocked、resume、cancel、progress、partial results 和 tool results 都必须由明确的 checkpoint/pending/resumable state 表达。
- Feature 代码不得直接调用 tool executor、shell executor、K8s client、Coroot tool 或其他执行 client，从而绕过 `ToolDispatcher`。
- Trim、compaction、spill、summary 和 tool-result budgeting 必须走统一 runtime context pipeline。
- 稳定 prompt 与动态 prompt 分层属于 `promptcompiler`；tool surface 变化必须来自 iteration-level assemble/refresh，不能通过旁路 tool prompt 注入。

### Tool / Skill / MCP / Plugin 边界

- `Tool` 是唯一可被模型调用的对象。内置 tool、编排 tool 和动态 MCP tool 必须进入同一套 tool registry / assembly 语义。
- `SkillDefinition`、`AgentDefinition`、`MCPServerConfig`、settings、hooks 和 permissions 不是 tool，不能被直接倒进 tool pool。
- `PromptCompiler`、`RuntimeKernel` 和 agent assembly 必须共享同一个 `AssembledTools`。
- 动态 MCP tool 必须通过 `mcp.Registry.OnServerConnected(...)` 进入 runtime，并用 `IsMCP + MCPInfo` 表达来源。
- 新外部集成默认应采用 MCP server + Skill + plugin manifest，而不是硬编码进核心。
- Provider/product 名称、命令、taxonomy、UI component 或兼容 schema 应放在 built-in plugin、capability pack、provider adapter、renderer component、fixture 或 migration doc 中，不要放进核心生产入口。
- Runner action 必须通过 `runner_actions` plugin/action registry 接入，不能只靠扩展默认 catalog。
- Agent 到 UI 的 artifact 必须携带稳定的 `type`、`renderer` 和 `schemaVersion`；前端 renderer 不能通过 tool name、MCP server id 或 artifact type prefix 猜行为。
- LLM-backed domain generation/summarization 必须使用 `ModelRouter` 或 skill/provider injection；domain 代码不得直接读取 provider env var 并调用 SDK。

### AI Chat / Host Mention 规则

- 单主机会话默认是可执行 ops 模式，不是只读 chat。
- 对明确 `@host` 的一次性清晰 `exec`，可以在当前 AI Chat turn 中通过受治理的 `exec_command` 执行；只有复杂主机任务才应该启动 host-bound child agent。
- 每台 host 最多只能有一个 host-bound child agent。Host-agent command 仍必须回到 AI Chat approval，并展示 host identity、command、risk、scope、approval id 和 resume target。
- 同个会话中, 没有显式 mention 就没有对应 context 或 tool surface：
  - 没有 `@ip` / `@主机` -> 不请求、不读取、不连接主机
  - 没有 `@ops_graph` -> 不提供 OpsGraph context/tools
  - 没有 `@coroot` -> 不提供 Coroot tools
  - 没有 `@ops_manus` / `@ops_manuals` -> 不提供 `search_ops_manuals`
- 这些 mention 必须表示为结构化 special mention，不能当普通关键词文本处理。

### Assistant 输出 / Transport / UI 单一事实源

- React Chat 生产状态只有一条路径：

```text
TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React
```

- `AiopsTransportState.schemaVersion` 是 `aiops.transport.v2`。
- React Chat 生产 transcript 形态是 `turn.blockOrder + turn.blocksById`。
- 不要重新引入 `process[] + final`、`metadata.unstable_state`、page-local SSE/WebSocket stream、legacy `agent_event` reducer、`AgentEventProjection`、`codexProcessTranscript`、`ChatProcessFold` 或 assistant-final Markdown/text parser。
- 所有 assistant-visible output 必须进入同一个有序 timeline：commentary、commands、search、files、MCP、approvals、artifacts 和 final answer。
- `assistant_message phase=commentary` 用于保留事件顺序，应该保持短；长篇 RCA/result 必须放在 `phase=final_answer`。
- 前端不得解析 assistant final Markdown/text 来推断结构化 UI、执行状态、approval 状态、workflow 状态、process card 或 action。
- Busy/working 状态必须来自 `AiopsTransportState.status`、turn status 和 `runtimeLiveness`，不能来自卡片标题或本地 flag。
- Process-row 去重必须使用 typed semantic key，不能用可见文案。
- 不要用 CSS 或组件条件隐藏上游脏数据；必须修 projection/runtime 源头。

Structured streaming review 检查：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "snapshot\\.toolInvocations|store\\.runtime\\.turn|processItemsByTurnId|phaseFoldsByTurnId" web/src
rg -n "JSON\\.parse\\(|markdown heading|summary.*steps.*actions" web/src
```

前两条命令在 React Chat 生产路径中必须没有命中。JSON/Markdown 命中只有在 settings、fixtures、envelopes 或 API clients 这类正常场景中才可以，不能用于从 final text 派生 process UI。

### Web 产品面

- 页面和组件不得散落业务 `fetch(...)` 调用。必须使用专门 API client，例如 `web/src/pages/*Api.ts`、`web/src/lib/*` 或 domain API client。
- Chat / Protocol 生产 streaming 必须使用 `AssistantTransport`；不要新增 page-local `WebSocket`、SSE、EventSource、polling 或 reducer 路径。
- `internal/server` 只负责 transport：route、decode、status code、cookie、ws framing。业务 projection 和 command translation 属于 `internal/appui`。
- `/api/v1/state` 不是 React Chat process UI 的事实源。
- Terminal 必须使用自己的 `/api/v1/terminal/ws`；不要把 terminal streaming 混进 AssistantTransport。
- 已删除的 legacy Vue/store/router/realtime 路径不得作为兼容层恢复。

### OpsManual / Runner / Run Record

- AI Chat 必须先提取 Operation Frame，再根据 object、operation type、platform、execution surface、permission、parameters 和 verification evidence 判断是否可执行。
- `search_ops_manuals` 可以返回受控 decision，例如 `direct_execute`、`need_info`、`adapt`、`reference_only` 或 `no_match`；高分本身不能触发执行。
- 引用 Runner Workflow 的 verified manual 必须携带 `workflow_digest`；执行前必须校验 digest。
- Workflow reverse-generation 必须按规则生成结构化字段：`operation`、`workflow_ref`、`parameter_rules`、`risk_policy`、`validation_report`。LLM 只能润色 `document_markdown` 和 `user_summary`。
- Candidate manual 只能进入 `draft`、`pending_review` 或 `needs_fix`，绝不能自动变成 `verified`。
- Preflight、approval、execution 和 verification outcome 必须写入 Run Record。

### 安全

- 非只读生产动作必须有明确 risk、evidence、approval 和 verification path。
- 高风险动作只能通过 tool + policy + approval 路径展示和执行。模型文本不能声称已经执行。
- Secret、API key、secret ref、Authorization header 和敏感参数不得进入日志、README、fixture、截图、LLM output 或生成的 manual text。
- Prompt 文本不能替代硬策略。Allow/deny/ask 决策必须属于 `PolicyEngine` / `PermissionEngine`。

### 禁止的工程反模式

- 不要用 `if toolName == "xxx"` 或关键词过滤修补 projection/runtime 行为。
- 不要围绕某个场景反复堆本地 if/else 或 regex 补丁。如果一个问题需要超过两次定向修补，必须停下来重新审视接口和设计。
- 不要新增硬编码实例匹配，例如 `strings.Contains(toolName, "coroot")` 或 `if mode == "special_debug_mode"`。
- 不要用 prompt 文本补偿缺失的 policy。
- 不要把临时特殊处理直接塞进 `eino_kernel.go` / `RunTurn`。
- 如果现有接口无法支撑改动，必须先停下来讨论设计，再修改接口或新增代码。

### AI 生成代码与 Agent Harness 变更控制

以下规则适用于 AI 或人工对 agent runtime、prompt、tool surface、policy、approval、context、final、transport 和 eval 的修改。

#### 修改前必须建立行为契约

- 在修改代码前，必须先写清：用户可见现象、确认后的根因、生产调用链、权威事实源、必须保持不变的行为、失败测试和验收命令。推断必须标记为推断，不能把猜测直接实现成补丁。
- 默认先复现并新增一个能在修复前失败的回归测试。无法稳定复现时，只允许增加不改变行为的 trace、diagnostic 或 fixture；不得直接修改业务语义。
- 必须区分生产路径、trace、projection、fixture 和 test helper。trace 或 projection 只能描述生产事实，不能反向成为 runtime 决策来源；test helper 不得复制一套与生产不同的 turn loop、tool dispatch 或 finalization。
- 每次只修一个行为类别。不得把 prompt 调整、runtime 重构、tool 接入、UI 改版和无关清理混在同一个修改中。

#### 变更预算与停止条件

- 非机械性修改默认不得超过 5 个生产文件，生产代码增删总量不得超过 500 行。超过预算时必须停止实现，先写设计方案和分阶段实施清单，并获得确认。
- 已超过 800 行的高频核心文件不得继续承载新的独立职责。必须先用生产路径测试锁住行为，再按 owner 边界机械拆分；拆分阶段不得同时改变业务行为。
- 不得顺手修改任务范围外的代码、测试基线、文案、依赖或格式。发现相邻问题时记录为独立任务，不在当前补丁中扩张。
- 同一现象经过两次定向修补仍复发、修改需要跨越三个以上 runtime 层、或无法指出唯一 owner 时，必须停止补丁式修复，回到接口/契约设计。
- 如果目标文件存在无法确认来源的未提交修改且与本次任务重叠，必须停止并先说明冲突，不能覆盖或重写用户已有改动。

#### Harness 权威事实源

- Turn 级不变量必须由 typed `TurnContext` / `TurnAssembly` 表达：intent、target、agent/profile、permission envelope、approval policy、context policy、capability envelope、loop policy 和 final contract。不得仅通过 `map[string]string` 或 prompt 文本传递关键控制语义。
- 每次模型采样必须有不可变的 typed `StepContext`：精确 model input、prompt fingerprint、model-visible tools、dispatchable tools、permission/tool policy hash 和 provider request snapshot。
- `AgentAssemblySnapshot`、`HarnessTurnTrace`、Prompt Trace 和 UI projection 必须从实际 Turn/Step 控制对象派生。禁止先在多处完成决策，再为 trace 拼装一个看似一致的快照。
- 模型可见工具和 runtime 可执行工具必须属于同一个 step tool surface。Dispatcher 必须校验 step fingerprint、target binding、permission snapshot 和 approval/action token；不在当前 step 中的调用一律 fail closed。
- 新增关键控制字段时必须先进入 typed contract 或集中定义的 metadata schema，并明确单一 writer、合法 reader、生命周期和默认值。禁止在多个目录散落新的 `"aiops.*"` 字符串字面量。
- `FinalContract.status`、evidence、approval、performed actions、post-check 和 limitations 必须由结构化 runtime 状态计算；不得从 assistant 文本关键词反推完成、验证、失败或审批状态。

#### 测试与回归要求

- 所有 agent 行为修改必须有生产 `RunTurn` 或公开 HTTP/AssistantTransport 入口的集成测试；只有 helper/unit test 不能作为完成依据。
- 回归测试必须经历 red -> green。必要时临时撤销修复确认测试确实失败，再恢复修复；不能用“新增测试当前是绿的”代替回归证明。
- 不得为了让修改通过而删除、跳过、放宽既有断言，或直接更新 golden/snapshot。确需改变契约或用户可见行为时，必须先说明旧行为、新行为和兼容影响，并单独审查 baseline diff。
- 涉及 route/tool/approval/final/transport 的修改至少验证一条完整故事链：`ChatCommand -> appui admission -> RuntimeKernel -> ToolDispatcher/checkpoint -> FinalContract -> AiopsTransportState`。
- 涉及 UI 的修改必须保留 fixture-driven screenshot snapshot；涉及 prompt、tool surface 或 policy 的高风险修改必须运行 P0 eval、baseline comparison 和多次重复测试。真实 provider 不可用时必须明确报告未验证项，不能用 mock 结果冒充真实模型结论。
- 每个生产重大 bug 都必须同时留下：最小回归用例、对应 harness/story case、`fixbug.md` 事实记录和可复制的验证命令。

#### AI 交付证据

- AI 完成修改时必须报告：根因证据、实际修改文件、变更行数、失败测试、通过测试、未运行测试、剩余风险、生产链路验证结果和 `git diff` 范围。
- “测试通过”只能指本轮实际运行且退出码为 0 的命令；不得把局部测试描述成全量测试，不得用静态检查代替运行时、浏览器或真实 provider 验证。
- 没有失败复现、没有生产路径测试、超过变更预算、或存在未解释的 baseline/snapshot 漂移时，不得声称任务已完成。

### 重大 Bug 修复记录

- 修复重大 bug 时，必须在同一次修改中追加维护 `fixbug.md`；如果文件不存在，先创建该文件。
- 重大 bug 包括但不限于：影响核心 runtime/tool/approval/transport 主链、导致页面主流程不可用、造成错误执行或审批绕过、数据丢失、权限/安全风险、部署不可用、反复出现的严重回归。
- `fixbug.md` 只记录事实，不写复盘散文。每条记录必须包含：
  - 修复时间：使用本地日期时间，例如 `2026-07-03 21:30`。
  - Bug 现象：用户或系统看到的问题，以及影响范围。
  - 根因：确认后的技术原因；如果只是推断，必须标明“推断”。
  - 修复方式：改了哪些关键路径、策略或文件，为什么这样修。
  - 验证结果：跑了哪些测试、命令或浏览器验证，修复后的效果是什么。
  - 风险与后续：残余风险、可能回归点、观察项；没有已知风险也要写“暂无已知风险”。
- 记录中不得写入 API key、密码、Authorization header、secret ref 明文、客户敏感数据或完整高风险命令输出。
- 对重大 bug 的交付不能只改代码不写记录；缺少 `fixbug.md` 记录视为未完成。

## UI 截图快照覆盖

`aiops-v2` 遵循 Codex 对用户可见 UI 改动的要求：任何影响可见 UI 的改动都必须包含 screenshot snapshot 覆盖。

前端 UI 改动要求：

- 在 `web/tests/*snapshot*.spec.js` 或其他 Playwright UI spec 中新增或更新 snapshot 测试，并调用 `expect(...).toHaveScreenshot(...)`。
- 优先通过 `web/tests/helpers/uiFixtureHarness.js` 使用 fixture-driven 测试，让截图稳定且不依赖真实 LLM。
- 不要把 `page.screenshot({ path })` 当作覆盖。那些文件只是调试产物，UI 回归时不会让测试失败。
- 接受 snapshot 更新前必须审查 diff。

命令：

```bash
cd web
npm run test:ui:snapshots
npm run test:ui:snapshots:update
```

只有当 UI 改动是预期行为且新 baseline 已经审查过，才使用 `test:ui:snapshots:update`。

## AssistantTransport 结构化 Streaming

`aiops-v2` 的 structured streaming 在 React Chat 生产路径中只有一条链路：

```text
TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React
```

处理 chat、protocol、process UI、runtime item、approval、MCP surface 或 replay 的 agent，必须直接扩展 `AiopsTransportState` 和 AssistantTransport state ops。不要新增 `StructuredResponsePatch`、`emit_response_events`、`StructuredResponsePanel`、page-local SSE/WebSocket stream、legacy `agent_event` reducer、`AgentEventProjection` selector、`codexProcessTranscript`、`ChatProcessFold`，也不要为 `summary/steps/actions` 增加 assistant-final-text parser。

`AiopsTransportState.schemaVersion` 是 `aiops.transport.v2`。React Chat 生产 transcript 只有一种形态：`turn.blockOrder + turn.blocksById`。不要重新引入 `turn.process`、`turn.final`、`metadata.unstable_state` transcript payload、page-local chat SSE/WebSocket stream，或通过 final Markdown/text 解析 process UI。

交付 structured streaming 相关工作前，运行：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "JSON\\.parse\\(|markdown heading|summary.*steps.*actions" web/src
```

前两条命令在 React Chat 生产路径中应该没有命中。JSON/Markdown 命令可以命中 settings、fixtures、transport envelopes 或 API clients 的正常 JSON 解析，但不能命中从 assistant final Markdown/text 派生 process UI 的代码。
