# AIOps Codex V2 — Eino Agent 后端

基于 [Eino Agent Framework](https://github.com/cloudwego/eino)（含 ADK）重写的 AIOps Codex AI Server 后端。

前端页面仍在 `aiops-codex/` 目录，本项目只包含后端 AI Server。前后端通过 HTTP/WebSocket/gRPC API 通信，所有端点路径、方法和 JSON 结构保持向后兼容。

## 架构概览

```
cmd/ai-server/main.go          # 启动入口，组装所有组件
internal/
├── runtimekernel/              # RuntimeKernel — 唯一的 turn 运行时内核
├── tooling/                    # 统一 Tool 抽象与 tool assembly 真源
├── commands/                   # Prompt/slash/local command registry
├── skills/                     # Skill catalog（通过 command surface 暴露）
├── agents/                     # Agent definition registry
├── mcp/                        # MCP server registry
├── integrations/               # 内建 integrations（direct registration）
├── plugins/                    # Plugin spec/loader/registrar 装配层
├── promptcompiler/             # PromptCompiler — 四层结构化 Prompt 编译
├── policyengine/               # PolicyEngine — 显式策略硬约束
├── projection/                 # Projection — 生命周期事件投影
├── modelrouter/                # ModelRouter — LLM Provider 路由与 Fallback
├── agentmgr/                   # AgentManager — Multi-Agent 编排（ADK）
├── spanstream/                 # SpanTree + MultiplexedStream 多路复用流
├── server/                     # HTTP/WebSocket/gRPC API 兼容层
├── store/                      # 数据持久化（内存 + JSON 异步写盘）
├── settings/                   # settings precedence / governance 聚合
├── hooks/                      # tool / turn lifecycle hooks
├── permissions/                # tool 执行权限治理
└── integration/                # 集成测试
```

## 当前注册模型（2026-04）

- `Tool` 是唯一模型可调用对象；`PromptCompiler`、`RuntimeKernel`、`AgentFactory` 共享同一份 `AssembledTools`。
- `skills` 不直接进入 tool pool，而是先进入 `SkillRegistry`，再通过 `CommandRegistry.ListSkillLikePromptCommands()` 暴露给 `SkillTool`。
- `AgentDefinition`、`MCPServerConfig`、`settings / hooks / permissions` 都不是 tool；它们分别进入独立 registry 或治理层。
- 旧 `capability` 兼容层已经删除；主运行链只认 `tooling.Tool`、`tooling.Registry`、`mcp.Registry`、`commands/skills/agents/...` 各自独立 registry。

## 快速开始

```bash
cd aiops-v2
./scripts/start.sh              # 推荐：构建 web/dist + ai-server，并由 ai-server 托管前端
```

如果只想手动执行各步骤：

```bash
go test ./...                   # 运行全部测试
go build ./cmd/ai-server        # 编译
AIOPS_DATA_DIR=.data ./ai-server # 启动（需配置 LLM Provider）
```

常用启动覆盖：

```bash
AIOPS_HTTP_ADDR=:18080 ./scripts/start.sh
SKIP_WEB_BUILD=1 SKIP_GO_BUILD=1 ./scripts/start.sh
./scripts/start.sh --dry-run
```

---

## 长时间运行 Runtime Guardrails

这部分约束的是 `runtimekernel` 主运行链。新增 tool、prompt、checkpoint、session 恢复、streaming、compaction 时，必须先满足这里，再谈局部功能。

### 1. 主运行链只有一条

- 长时间运行 turn 只能由 `runtimekernel.EinoKernel` 驱动
- host 路径的标准链路是：
  `model -> assistant/tool_use checkpoint -> ToolDispatcher -> progress/tool_result checkpoint -> next iteration`
- 不允许再造第二套 runtime kernel、第二套 turn loop、第二套 tool execution 旁路
- 不允许在 feature 代码里直接调用 tool executor 绕开 `ToolDispatcher`

### 2. Checkpoint / Suspend / Resume 是硬约束

- assistant response 和 parsed `tool_use` 必须先落 checkpoint，再进入工具执行
- `progress`、`partial result`、`tool_result` 必须支持增量 checkpoint，不能只在整批 tool 结束后一次性落盘
- `NeedApproval` / `NeedEvidence` 必须进入显式 `suspended` 或 `resumable` 状态
- 不允许出现“逻辑上 blocked，但 runtime 没有 checkpoint/pending state”的隐式中断

### 3. Context / Result 只有一套治理路径

- trim、compaction、spill、summary 必须在统一 context pipeline 中决策
- tool result budget 只有一套标准语义：
  `MaxInlineResultBytes`、`ResultSpillPolicy`、`SummarizeLargeResult`
- runtime 结果承载只允许三段策略：
  小结果内联；中结果摘要/预览内联且全文外溢；大结果只回灌摘要与引用
- `ToolResult` 可以带 `blob/card/file` 引用，但这些引用仍要进入 runtime message / checkpoint / session external references 主路径
- 不允许额外引入“只给 UI 看”的隐藏结果存储，或者另一套私有 compaction 状态

### 4. Prompt / Tool 刷新必须按 iteration 收敛

- stable prompt 与 dynamic prompt 的分层只能在 `promptcompiler` 中完成
- tool surface 只能来自 iteration 级 assemble/refresh，不允许手写额外 tool prompt side channel
- permission mode、hooks、MCP topology 造成的 tool 可见性变化，必须通过下一 iteration 的 tool refresh 生效

### 5. 目前仍未完成的边界

- `WorkspaceTask` 的创建/消费主路径仍主要属于后续 multi-agent 生命周期层；当前已具备 store-backed 持久化与 restart reconcile，但不是日常 host turn 主链的一部分

### 6. 未来新增功能的落地规则

- `docs/longtime_design_0422.md` 与 `docs/longtime_todo_0422.md` 的实施顺序固定按 `P0 -> P4` 表达；后续增强项只能追加在 `P4` 之后，不能重排主线
- workspace/host 都必须继续走共享的 `RunTurn -> runHostIterationLoop` 主链；禁止重新引入 legacy workspace runtime path 或其他临时旁路
- 如果新增功能需要 prompt、tool、checkpoint、session 恢复、streaming、compaction 变化，先改主链和设计文档，再落具体 feature

以后新增功能，如果会碰到这些边界，先改设计和主链，不要在边缘逻辑里补兼容分支。

---

## Web Product Surface Guardrails

本节约束 `web/` 与 `ai-server` 的正式产品面。新增页面、接口、实时事件、登录、terminal、settings、hosts、MCP、agent profile 时必须遵守这里，不允许重新把旧项目协议当成永久兼容层。

### 1. 前端只有一套数据入口

- 页面、组件、`store.js` 不允许直接 `fetch(...)`
- 页面、组件、`store.js` 不允许直接 `new WebSocket(...)`
- HTTP 只能经由 `web/src/api/*`
- realtime 只能经由 `web/src/realtime/*`
- `store.js` 是状态中心，不是协议中心；它只能调用 `api/realtime` client

### 2. 后端只有一套 Web API 应用层

- `internal/server` 只做 transport：路由、decode、status code、cookie、ws framing
- 业务投影和命令翻译只能放在 `internal/appui`
- `internal/server` 不允许直接拼 runtime/session/store/mcp/auth/terminal 的业务对象给前端
- 不允许新增 `webcompat`、legacy pusher、私有 snapshot builder 或页面专用 handler

### 3. Chat / Protocol 只能走 RuntimeKernel 主链

- 新消息只能进入 `runtimekernel.RunTurn -> runHostIterationLoop`
- approval decision、choice answer、evidence follow-up 都必须回到同一条 `ResumeTurn` path
- stop/cancel 只能进入 `runtimekernel.CancelTurn`
- 不允许为 protocol workspace、chat 页面或某张 UI card 新造第二套 orchestrator

### 4. Snapshot / WS 只有一套真相源

- `/api/v1/state` 与主 `/ws` 必须共享 `appui.SnapshotBuilder`
- 主 `/ws` 只推前端可消费的 `appui.StateSnapshot`、规范化 `agent_event` 和 heartbeat
- terminal 必须使用独立 `/api/v1/terminal/ws`，不能混进主 `/ws`
- 已删除的 legacy `WebSocketPusher` / `LegacyMessage` 不能重新引入

### 5. Agent Event 是唯一实时运行事件

- Chat、Protocol、host-agent、subagent、tool、MCP、approval、diff、browser 验证的运行过程只能进入统一 `AgentEvent`
- 旧 `turn_event` 必须在 AgentEvent 切换同批删除，不能作为生产 adapter 或新功能依赖
- 运行态 UI 只能由 `AgentEventProjection` 推导，页面不能直接从 `cards/toolInvocations` 猜 `Working`、agent 状态或 process fold
- `AgentEvent` 必须有 `eventId`、`seq`、`kind`、`phase`、`status`、`visibility` 和 typed payload，不能靠文案正则判断状态
- `internal/server` 不允许拼业务事件；事件 normalize、append、replay、projection 只能放在 `internal/appui`
- 前端只能有一套运行事件 reducer；本地 optimistic 状态也必须表示成同一套 `AgentEvent`
- Busy/Working 必须由 liveness 集合推导：active turns、active agents、active command streams、pending approvals、pending user inputs
- 主 UI 不显示 UUID/call-id/raw id，只显示 agent handle/name、阶段摘要、diff stats、approval/artifact 入口

新增或修改 Chat / Protocol / 工作台 UI 时必须同时满足：

- 后端新增运行过程：先扩展 `internal/appui/agent_event*.go` 的 event contract / normalizer / projector，再由 `/ws` 推 `agent_event`
- 前端新增运行过程：先扩展 `web/src/events/agentEventReducer.js` 和 `web/src/events/agentEventProjection.js`，页面只消费 projection selector
- 本地 optimistic、send pending、stop、failed、approval blocked、tool progress 都必须写成本地或后端 `AgentEvent`，不能只改页面局部变量或 `runtime.turn`
- 单机会话和协作工作台必须共用同一套 projection selector；允许布局不同，不允许状态模型不同
- `snapshot.cards` 只能作为持久会话内容 / card artifact 输入，不能作为 running/busy/approval/tool progress 的真相源
- `snapshot.toolInvocations` 只能作为历史证据/详情兼容输入，不能驱动主 Chat 工作流里的实时 process fold
- 单机会话过程 UI 必须收敛到一套 Codex-style transcript projection；不能并行用 `LiveStatusCard`、`statusCard`、process cards 或 page-level activity label 生成同一轮运行过程
- 命令、搜索、文件、MCP 等过程行必须使用 typed fields（例如 `displayKind`、`inputSummary`、`command`、`outputPreview`）渲染；不能靠 UI 文案正则重新分类
- 过程行去重必须使用 typed semantic key（例如 `displayKind + command/inputSummary/queries/results`），不能只按可见文案去重，否则会把多次同名搜索或命令错误折叠掉
- `已记录 X 条过程细项`、`明细已折叠`、`处理失败 X 条明细`、`准备上下文`、`编译提示词`、`准备工具`、`调用模型` 这类不面向用户的过程文案不得从上游生成；不得用组件条件或 CSS 隐藏作为修复
- 如果新增字段需要“正在执行/等待审批/已停止/失败/Working”文案，必须能从 `RuntimeLiveness` 或 projection row 推导，不能靠 card title/status 文案正则
- Codex 原生过程 UI 规范以 `docs/superpowers/specs/2026-04-29-codex-native-process-ux-design.zh.md` 和 `docs/superpowers/specs/2026-04-29-codex-native-process-ux-todo.zh.md` 为准；修改单机会话过程 UI 时必须同步更新对应验收项
- 真实 LLM Playwright 回归只能通过 `AIOPS_TEST_LLM_BASE_URL`、`AIOPS_TEST_LLM_API_KEY`、`AIOPS_TEST_LLM_MODEL` 注入配置；API key 不允许写入源码、fixture、README、验收报告或截图命名
- 代码评审前必须运行：

```bash
rg "turn_event|applyTurnEvent|turnEventsById|processItemsByTurnId|phaseFoldsByTurnId|assistantDraftsByTurnId|finalMessagesByTurnId" web/src internal/server internal/appui
rg "snapshot\\.toolInvocations|store\\.runtime\\.turn|processItemsByTurnId|phaseFoldsByTurnId" web/src/App.vue web/src/pages web/src/components
```

第一条在生产代码中必须无匹配。第二条如果有匹配，必须证明它不是运行态真相源；否则先迁移到 `AgentEventProjection`。

### 6. Approval UX 单入口规则

这次出现“已经有底部 `codex-approval-inline`，但仍显示对话审核 UI 卡片”的根因不是缺组件，而是审批展示路径没有归口：

- `ChatStream` 内有 `ApprovalDock`，会根据 `approvalDock` 渲染对话流里的审核卡片。
- `ChatPage` 底部 composer slot 内有 `codex-approval-inline`，设计目标是替换输入框。
- 旧 fallback `auth-overlay-dock` 仍保留，用于部分历史 card/MCP 审核场景。
- projection approval 和 snapshot approval 曾经分别走不同 computed，导致单机会话里同一个 pending approval 可以同时进入 `ChatStream` 和 composer。
- 早期 approval payload 只有 `title/reason`，真实命令被混在 reason 或缺失，UI 只能显示 `exec_command` 这类工具名。

以后任何审批 UX 改动必须遵守：

- 单机会话的用户审核入口只能是底部 `codex-approval-inline`，并替换输入框；不能在对话流中再渲染 `ApprovalDock` 或其他审核卡片。
- 协作工作台可以保留右侧/流程区审批视图，但必须显式由 workspace layout 消费，不能复用到 single-host 主聊天流。
- `ChatStream.approvalDock` 在 single-host 场景必须为空；如果需要展示等待审批状态，只能通过 composer approval 或轻量状态文案表达。
- `auth-overlay-dock` 只能作为 workspace 或特殊 MCP fallback；新增 single-host approval 不允许接入它。
- 审批展示必须使用真实业务对象：命令审批显示 `command`，文件审批显示 file/path/diff 摘要；禁止把 `exec_command`、`shell_command`、`code_mode` 等工具名当成用户可审内容。
- 后端 approval contract 必须分离 `command` 和 `reason`：`command` 给用户审核，`reason` 给策略解释；不能把策略原因塞进 command，也不能把 command 塞进 reason。
- projection、snapshot、local optimistic approval 必须归一到同一个 composer approval selector；页面模板里不能再并行判断多套 pending approval 来源。
- 同一个 approval id 在页面上只能有一个可点击决策入口；如果同时出现 `codex-approval-inline` 和 `approval-dock`，视为 P0 UI 回归。
- 审批按钮提交必须继续走统一 decision API，不允许组件私有化决策逻辑或直接改 store 状态。

修改审批相关代码时必须补或更新以下验证：

- 前端单元测试要断言 single-host pending approval 时：`[data-testid="approval-dock"]` 不存在，`[data-testid="codex-approval-inline"]` 存在，`.omnibar-wrapper` 不存在。
- 前端单元测试要断言显示真实 `command`，且不显示 `exec_command` 这类工具名。
- reducer/projector 测试要覆盖 `approval.payload.command` 从后端事件进入 projection。
- 后端 snapshot/projector 测试要覆盖 `PendingApproval.Command` 与 `Reason` 分离。
- Playwright 验证要至少检查一次真实或 fixture pending approval：approval dock 数量为 0、inline 数量为 1、输入框被替换、点击同意会发送 `/api/v1/approvals/:id/decision`。

### 7. 正式域边界

- auth 真相源只能来自 `internal/auth`
- terminal 真相源只能来自 `internal/terminal`
- hosts/settings/agent profiles/skills/agent mcps 只能通过 `Store` 和对应 `appui` service 写入
- MCP runtime 状态只能通过 `internal/mcp.Registry` 和 `appui.MCPService` 投影，不允许页面直接改 runtime registry

新增 Web 功能的最小接入顺序：

1. 先在 `web/src/api/*` 或 `web/src/realtime/*` 定义唯一前端入口
2. 再在 `internal/appui` 定义 service/DTO/command translation
3. 最后在 `internal/server` 补 transport handler
4. 如果涉及 chat/protocol 中断恢复，必须补 runtimekernel resume/cancel 测试
5. 如果涉及运行过程展示，必须先扩展 [`docs/superpowers/specs/2026-04-29-codex-native-process-ux-design.zh.md`](./docs/superpowers/specs/2026-04-29-codex-native-process-ux-design.zh.md)（中文验收规范）或 [`docs/superpowers/specs/2026-04-29-codex-native-process-ux-design.md`](./docs/superpowers/specs/2026-04-29-codex-native-process-ux-design.md) 定义的 `AgentEvent` contract，再实现 normalizer/projector/reducer；不要用组件条件或 CSS 隐藏旧模块来替代上游数据清理

---

## ⚠️ Registration Upgrade Guardrails

本节是当前仓库的硬约束，不是建议项。它来自两部分依据：

- [`docs/registration-upgrade-todo.md`](./docs/registration-upgrade-todo.md) 的阶段目标与统一验收清单
- 本地 `../claude code/` 源码的对应实现面，重点参考：
  - `Tool.ts`
  - `tools.ts`
  - `commands.ts`
  - `skills/loadSkillsDir.ts`
  - `types/plugin.ts`
  - `utils/settings/constants.ts`

以后新增功能，如果会碰到注册、装配、治理、prompt 或 agent 编排，先看这节。看完还觉得需要新增第二套模型、第二条主路径或新的 runtime interface，先停下来确认方案。

### 1. 不可破坏的架构不变量

1. `Tool` 是唯一模型可调用对象。
   - `PromptCompiler`、`RuntimeKernel`、`AgentFactory` 只能消费 `AssembledTools`
   - `SkillDefinition`、`AgentDefinition`、`MCPServerConfig`、settings、hooks、permissions 都不是 tool

2. tool assembly 只有一个真源。
   - 必须通过统一装配路径生成 tool pool
   - 内置 tools、编排 tools、MCP dynamic tools 必须在同一套 registry 语义下组装
   - 同名冲突时，内置 tool 优先于 dynamic MCP tool

3. `ToolMetadata` 必须走 traits-first，而不是 kind-first。
   - 主字段是 `Name / Aliases / SearchHint / ShouldDefer / AlwaysLoad / IsMCP / IsLSP / MCPInfo`
   - `Origin` 只是兼容/展示字段，不是新的主分流轴
   - 不允许新增围绕 `Origin=builtin|mcp|meta` 的运行时分支

4. skills 只能通过 command surface 暴露给模型。
   - `skills.Registry` 存定义
   - `commands.CommandRegistry` 发布 skill-like prompt commands
   - `SkillTool` 消费 command surface，不直接消费孤立 `SkillDefinition`

5. agents、MCP servers、plugins 都是独立 registry，不是平行 tool 模型。
   - `AgentDefinition` 进 `internal/agents`
   - `AgentTool` 才是编排型 tool 视图
   - `MCPServerConfig` 进 `internal/mcp`
   - MCP 在 runtime 上表达为 tool source：`IsMCP + MCPInfo`

6. governance 只治理 tool，不定义 tool 类型。
   - `PermissionEngine`、`HookRegistry`、`FeatureFlags`、`settings.Governance` 只能影响暴露、审批、阻断、目录范围、MCP allowlist
   - 它们不能创建新的 capability 类别，也不能偷偷拼出第二套 tool pool

7. plugin / extension 只是装配层，不是 runtime 核心接口。
   - 可分发的组件面是 `commands / skills / agents / hooks / mcp / lsp / output styles / settings`
   - `RuntimeKernel`、`PromptCompiler`、`AgentFactory` 不应直接感知 plugin/extension

8. 旧兼容层已经删除，不允许重建。
   - 不允许重新创建 `internal/capability`
   - 不允许新增 `ExtensionManager` / `MCPServerManager`
   - 不允许新增 `LegacyToolRuntime` / `NewLegacyToolAdapter`
   - 不允许重新发明 `VisibleCapabilities` / `MCPPromptAssets` 一类旁路

### 2. Source Precedence And Governance

- settings/customization 的低到高覆盖顺序是：
  `userSettings -> projectSettings -> localSettings -> flagSettings -> policySettings`
- `policySettings` 的内部优先级只允许在 `internal/settings` 内部定义和合并，业务代码不得重新实现
- agent definition source precedence 是：
  `built-in -> plugin -> userSettings -> projectSettings -> flagSettings -> policySettings`
- `strictPluginOnlyCustomization` 生效时，只有 admin-trusted sources 还能继续往受控 surface 写内容
- 当前 admin-trusted sources 是：
  `plugin`、`built-in`/`builtin`、`bundled`、`policySettings`
- `allowedMcpServers` 与 `additionalDirectories` 属于治理层，不允许散落到单个 feature 的私有配置里

### 3. 新增功能时的接入规则

**新增 Tool**

- 优先实现 `internal/tooling.Tool`，简单场景直接用 `tooling.StaticTool`
- builtin/static tool 只允许 `tooling.Registry.Register(...)`
- dynamic MCP tool 只允许 `mcp.Registry.OnServerConnected(...)`
- tool 的可见性、defer/load 行为、MCP/LSP 来源、审批/权限都必须通过 metadata + assembly / execution pipeline 表达
- 不能在 `RuntimeKernel`、`PromptCompiler`、`PolicyEngine` 里给某个新 tool 写硬编码旁路

**新增 Skill**

- skill definition 放进 `internal/skills`
- skill-like command 放进 `internal/commands`
- 让 `SkillTool` 通过 command surface 发现它
- 不要再把 skill 当成另一种 tool kind 或 capability 树节点

**新增 Agent**

- 定义进 `internal/agents`
- 执行和调度留在 `internal/agentmgr`
- 如果模型需要编排它，暴露 `AgentTool` 风格视图，而不是把 `AgentDefinition` 本身塞进 tool pool
- agent scope 过滤必须围绕 assembled tools 与 metadata traits，不允许退回旧 `Kind*` 主轴

**新增 MCP 能力**

- 注册/管理放在 `internal/mcp`
- 运行时表达必须是 dynamic tool + `IsMCP + MCPInfo`
- 不允许新增 “第二种 MCP tool 模型” 或 “专供 MCP 的 prompt 旁路”

**新增 Plugin / Extension 能力**

- 只允许贡献到已有 registry surface：`commands / skills / agents / hooks / mcp / lsp / output styles / settings`
- builtin integration 只允许通过 `cmd/ai-server/registerBuiltinIntegrations(...)` 直连目标 registry
- plugin 只允许通过 `plugins.ManifestLoader + Registrar`
- 不允许让 plugin/extension 反向控制 `RuntimeKernel`、`PromptCompiler`、`AgentFactory`
- 不允许因为 plugin 需要而新增第二套注册模型或运行时接口

**新增 Prompt / Policy / Governance 规则**

- tool prompt 只能来自 `AssembledTools`
- 非 tool 的补充上下文只能走 `SkillPromptAssets` 或 `ExtraSections`
- prompt 文案不能替代硬策略；真正的 allow / deny / ask 必须进 `PolicyEngine` / `PermissionEngine`
- 新 mode / policy / governance source 若改变行为，必须同步补装配与回归测试

**新增 LLM Provider**

- 必须实现 Eino `model.ChatModel`
- 必须通过 `ModelRouter` 接入
- 不能在 `RuntimeKernel` 里直接调 provider SDK

### 4. 明确禁止的做法

- 禁止创建平行 tool 池、局部 tool 列表或 “仅 prompt 可见 / runtime 不可执行” 的旁路
- 禁止在主运行链路新增任何 capability-kind 风格分类轴
- 禁止重新引入 `VisibleCapabilities -> PromptCompiler` 这类二次筛选模型
- 禁止重新引入 legacy `MCPPromptAssets` 之类的 MCP prompt side channel
- 禁止把 `SkillDefinition`、`AgentDefinition`、`MCPServerConfig`、hooks、permissions、settings 直接塞进 tool pool
- 禁止让 plugin/extension 直接修改 `RuntimeKernel`、`PromptCompiler`、`AgentFactory` 的主逻辑
- 禁止在 prompt helper、loop nudge 或局部 command 里散写系统级规则
- 禁止为了新功能重建 `capability`、legacy adapter、compat manager

### 4.1 以后不允许重新引入的旧入口

- 不允许重新创建 `internal/capability/*`
- 不允许新增 `ExtensionManager` / `MCPServerManager`
- 不允许新增 `LegacyToolRuntime` / `NewLegacyToolAdapter`
- 不允许让 `SkillPromptAssets` 绕过 `CommandRegistry.ListSkillLikePromptCommands()`
- 不允许在 `internal/agents` / `internal/agentmgr` 定义层重新引入 `CapabilityKinds`、`Hosts`、`HostScope`
- 不允许新增 `policyengine.CheckCapability(...)` 或等价 wrapper

### 5. 需要改设计而不是直接写代码的情况

出现下面任一情况，先更新设计文档并确认，再继续编码：

- 你觉得现有 `ToolMetadata` traits 不够表达需求
- 你想新增新的 capability kind / source kind / runtime interface
- 你想让某类对象既不是 tool、又要被模型直接调用
- 你想让 plugin/extension 直接参与 runtime 决策
- 你想绕过统一 source precedence 或 governance merge 逻辑

### 6. 提交前自检清单

- 新增的模型可调用对象是否真的是 `Tool`，而不是别的定义类型
- `PromptCompiler`、`RuntimeKernel`、`AgentFactory` 是否复用了同一份装配结果
- 是否使用了 `ToolMetadata` traits，而不是新增 kind 分支
- skills 是否通过 command surface 暴露
- MCP 是否通过 `IsMCP + MCPInfo` 表达，而不是通过并列 kind 表达
- plugin/extension 是否只做分发，不做 runtime 主逻辑
- governance / precedence 是否复用了现有聚合逻辑，而不是局部重写
- 是否补了单元测试；跨层不变量是否补了 property tests
- 是否运行了 `go test ./...`
- 是否运行了 `go build ./cmd/ai-server`

## 通用规则

1. 注册制优先：所有模块通过统一接口注册，不允许平行能力池或硬编码旁路
2. 接口隔离：各层通过接口通信，禁止跨层直接引用实现
3. 特殊情况必须确认：如果现有接口无法满足需求，必须先讨论方案，再修改接口或添加代码
4. 测试覆盖：新增模块必须包含单元测试；涉及跨层正确性不变量的必须补 `pgregory.net/rapid` property tests
5. 不修改 `aiops-codex/`：所有后端代码变更限定在 `aiops-v2/`
6. 任何影响四层 prompt 语义、tool lifecycle 真源、workspace/host 隔离、source precedence 的变更，都必须同步更新设计/README

---

## 🚫 禁止的工程反模式

以下行为在本项目中被严格禁止。遇到问题时，应该从架构层面解决，而不是用局部 hack 绕过。

### 禁止局部字符串过滤优化

不要因为某个 tool 的输出有问题，就在 Projection 或 RuntimeKernel 中加入针对特定 tool 名称的 `if toolName == "xxx"` 过滤逻辑。这类代码会迅速腐化为不可维护的 switch-case 地狱。

正确做法：修改该 tool 的 `Display()` 或 `Execute()` 实现，让它从源头输出正确的结构化数据。

### 禁止陷入小范围优化循环

不要因为一个具体场景的失败，反复调整某个 if-else 分支或正则表达式。如果一个问题需要超过 2 次针对性修补，说明设计有缺陷，应该退一步重新审视接口设计。

正确做法：
- 先写一个 eval case 复现问题
- 分析是 prompt / tool / policy / model 哪个维度的问题
- 在对应维度的注册接口层面修复
- 用 eval case 验证修复效果

### 禁止硬编码专用名称匹配

不要在通用逻辑中出现 `strings.Contains(toolName, "coroot")` 或 `if mode == "special_debug_mode"` 这类针对特定实例的硬编码判断。

正确做法：
- 通过 `tooling.Visibility` 或 `Tool.IsEnabled(...)` 控制可见性
- 通过 `Tool.IsReadOnly()` / `IsDestructive()` / `IsConcurrencySafe()` 声明属性
- 通过 `ModePolicy.CheckTool()` 和 `tooling.ToolMetadata` 定义边界
- 通过参数/metadata/source 规则过滤

### 禁止在 Prompt 中补偿代码缺陷

不要因为 PolicyEngine 没有正确拦截某个操作，就在 prompt 中加一句"你不能执行 xxx"来补偿。Prompt 是建议，PolicyEngine 是硬约束。

正确做法：在 `ModePolicy` 或 `PermissionEvaluator` 中添加对应的检查规则。

### 禁止绕过注册机制的"快速修复"

不要因为赶时间，直接在 `eino_kernel.go` 的 `RunTurn` 中插入特殊处理逻辑。所有能力必须通过 Registry 注册，所有策略必须通过 PolicyEngine 执行。

如果现有接口确实无法满足需求，**停下来，跟用户确认方案**，而不是绕过架构。

---

## 📚 扩展文档

- [注册规则升级设计方案](docs/registration-upgrade-design.md) — 基于 claude code 源码分析的注册机制增强计划
- [Agent 调优指南](docs/agent-tuning-guide.md) — prompt/tool/policy/model 调优流程与 eval 框架

## 测试

```bash
go test ./...                           # 全量测试
go test -run TestProperty ./...         # 只跑属性测试
go test -count=1 ./...                  # 清缓存跑
```
