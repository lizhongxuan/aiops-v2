# aiops-v2 Agent Runtime 分层设计方案

日期：2026-06-26
状态：设计方案
范围：`aiops-v2` Agent Runtime 分层、Eino 边界、Codex 分层借鉴、旧路径收口
关联文档：
- `docs/superpowers/specs/2026-06-24-aiops-v2-codex-agent-runtime-migration-design.zh.md`
- `docs/superpowers/specs/2026-06-26-aiops-v2-codex-context-capability-migration-design.zh.md`
- `docs/superpowers/plans/2026-06-26-aiops-v2-codex-context-capability-migration-implementation-todo.zh.md`

## 1. 设计结论

`aiops-v2` 当前的 `EinoKernel` 命名容易让人误判：Eino 并不是 Agent Runtime 的核心状态机。实际代码里，turn lifecycle、session state、context pipeline、prompt compiler、tool surface、approval/resume、final gate 和 UI projection 都是 `aiops-v2` 自己实现的；Eino 主要承担 provider adapter、message DTO、tool schema adapter 和 streaming 调用。

因此，迁移方向不是删除 Eino，也不是把 runtime 改成 Eino ADK 的完整 agent loop，而是：

1. 保留 Eino 作为模型与工具适配层。
2. 把 Agent Runtime 分层显式命名为 `RuntimeKernel` / `TurnLoop` / `ProviderAdapter`，降低 `EinoKernel` 的概念权重。
3. 学习 Codex 的分层边界：Session/TurnContext/StepContext/ToolRouter/Prompt/ProviderRequest/Trace 分开，各层单一 owner。
4. 一次性收敛 aiops-v2 现有的多处兼容视图和旧字段，避免“Eino message 是事实源”和“aiops runtime message 是事实源”并存。

本方案的目标是让后续上下文迁移、Trace v2、ProviderRequestSnapshot 和 ModelInputItem 能落在清晰层次里，而不是继续堆到 `runtimekernel/eino_kernel.go`。

本方案不设计长期兼容模式，不保留两套 agent runtime 逻辑。最终只能存在一条事实链：

```text
TurnRequest
  -> RuntimeTurnContext
  -> RuntimeStepContext
  -> ModelInputItem[]
  -> ProviderRequestSnapshot
  -> ProviderAdapter
  -> TraceDocumentV2 / TurnItem Projection
```

旧路径在同一次切换中删除或改为测试 fixture，不进入生产 runtime：

1. 删除 `CompileForEino` 作为 runtime 接口。
2. 删除 `promptinput.BuildResult.Messages` 作为内部事实源。
3. 删除 `modeltrace.Request.ModelInput []*schema.Message`。
4. 删除 v1 trace writer 和 v1 runtime reader。
5. `schema.Message` 只允许出现在 provider adapter、Eino tool adapter 和 adapter 测试里。

## 2. Codex 的 Agent Runtime 分层

Codex 的分层可以概括为 8 层：

```text
Client / Protocol
  -> Session
  -> TurnContext
  -> StepContext
  -> ToolRouter
  -> Prompt
  -> Provider Request / ModelClientSession
  -> Rollout Trace / Raw Payload
```

### 2.1 Client / Protocol 层

Codex 把客户端操作和内部 runtime 通过 protocol item 分开。`TurnInput`、`ResponseItem`、`TurnItem`、`EventMsg` 等协议结构是跨 UI、core、trace 的边界对象。

可借鉴点：

1. UI 与 runtime 不共享内部状态结构。
2. 模型输入项用结构化 `ResponseItem`，不是单纯 `role/content`。
3. protocol item 可以同时服务模型输入、UI timeline、rollout reconstruction。

aiops-v2 对应位置：

```text
appui.ChatCommand / TurnResponse / AgentEvent
runtimekernel.TurnRequest / TurnResult
```

当前问题是 `TurnRequest` 是 typed contract，但模型输入、Trace 和 UI projection 还没有统一的结构化 item 边界。

### 2.2 Session 层

Codex `Session` 是 thread 级 owner：保存 thread id、configuration、conversation manager、active turn、input queue、services、guardian/review 等。Session 明确声明一个 session 同时最多一个 running task，并支持用户输入打断。

可借鉴点：

1. Session 是 thread state owner。
2. Active turn 与 input queue 是 session 的一部分。
3. Session configuration 和 runtime mutable state 分开。

aiops-v2 对应位置：

```text
runtimekernel.SessionState
runtimekernel.SessionManager
runtimekernel.ActiveTurnState
PendingApprovals / PendingEvidence / TurnHistory
```

aiops-v2 已经具备类似能力，但需要把“active turn 控制”和“旧 appui 路径”进一步收口到 `runtimekernel`。

### 2.3 TurnContext 层

Codex `TurnContext` 是单个 turn 的不可变或准不可变配置快照：model info、provider、reasoning、session source、environment selections、permission profile、network、dynamic tools、metadata state、skills 等。

可借鉴点：

1. turn 启动时冻结请求相关配置，避免运行中静默漂移。
2. permission、sandbox、environment、model capability 都从 TurnContext 派生。
3. provider request metadata 从 TurnContext 生成，不从 Trace 反推。

aiops-v2 当前对应位置分散在：

```text
TurnRequest.Metadata
SessionState
CompileContext
ToolSurfaceSnapshot
TaskDepth
permissions.Engine / policyengine.Engine
```

建议新增 `RuntimeTurnContext`，作为每个 turn 的规范快照，持有 route、profile、permissions、model capabilities、tool surface policy、context budget 和 lineage metadata。

### 2.4 StepContext 层

Codex `StepContext` 是每次 sampling request 的快照：包含 `TurnContext`、environment snapshot、loaded AGENTS.md 等。它的职责是确保“本次模型请求看到的上下文、工具、环境”一致。

可借鉴点：

1. 一个 turn 可以有多个 sampling step。
2. step 级状态和 turn 级状态分开。
3. 每次模型请求的环境、工具、上下文都来自同一个 step snapshot。

aiops-v2 当前对应位置在 `runHostIterationLoop` 的 iteration 局部变量里，包括 context pipeline 结果、compileCtx、toolSurfaceSnapshot、promptBuild 和 trace。

建议新增 `RuntimeStepContext`，承载：

```text
turnContext
iteration
contextState
compiledPrompt
modelInputItems
toolSurfaceSnapshot
providerRequestSnapshot
trace refs
```

### 2.5 ToolRouter 层

Codex `ToolRouter` 同时持有 registered tool registry 和 model-visible specs。模型看到的工具来自 `router.model_visible_specs()`，工具调用也通过同一个 router dispatch。

可借鉴点：

1. 工具注册和工具可见性可以分离，但必须由同一个 router 输出。
2. 模型可见 schema 与 runtime 可 dispatch 工具来自同一个 step context。
3. deferred tools、dynamic tools、MCP tools、extension tools 都统一进入 router。

aiops-v2 当前对应位置：

```text
tooling.Registry / Assembler
tooling.AssembleToolsWithOptions
tooling.AssembleEinoToolPool
runtimekernel dispatch
ToolSurfaceSnapshot
```

建议把 `ToolSurfaceSnapshot` 升级为 `RuntimeToolRouterSnapshot`，明确区分：

```text
registeredTools
modelVisibleTools
dispatchableTools
hiddenReasons
policyHash
```

### 2.6 Prompt 层

Codex `Prompt` 是 provider 请求前的内部模型输入结构，包含 `input Vec<ResponseItem>`、`tools Vec<ToolSpec>`、`parallel_tool_calls`、`base_instructions`、`output_schema`。它不是 provider request 本身。

可借鉴点：

1. 内部 Prompt 与 provider request 分离。
2. Prompt 只表达模型输入和工具面，不承担 runtime owner 职责。
3. provider-specific 字段在 client adapter 层补齐。

aiops-v2 当前对应位置：

```text
promptcompiler.CompiledPrompt
promptinput.BuildResult
schema.Message[]
modeltrace.Request
```

建议用上下文设计文档中的 `ModelInputItem[]` 和 `ProviderRequestSnapshot` 取代 `schema.Message[]` 作为事实源。

### 2.7 Provider Request / ModelClientSession 层

Codex `ResponsesApiRequest` 才是 provider 请求，包含 model、instructions、input、tools、tool_choice、parallel_tool_calls、reasoning、store、stream、include、service_tier、prompt_cache_key、text、client_metadata。它还比较除 input/client_metadata 外的稳定请求属性，用于 websocket/request reuse。

可借鉴点：

1. provider request 是 adapter 输出，不由 prompt compiler 直接产生。
2. `prompt_cache_key` 和 request properties 是一等字段。
3. client metadata 包含 session/thread/turn/window/parent/subagent/workspace 信息。

aiops-v2 当前 provider request 由 Eino `chatModel.Stream(ctx, input, opts...)` 隐式生成，缺少可观察的 canonical request snapshot。

建议新增：

```text
modelrouter.ProviderRequestSnapshot
modelrouter.RequestPropertiesHash
modelrouter.ContextLineageMetadata
```

### 2.8 Rollout Trace / Raw Payload 层

Codex rollout trace 把 reduced graph 和 raw payload 分开。主 trace 保存事件、引用和可索引摘要，重型 inference request/response、tool invocation/result、terminal event 通过 `RawPayloadRef` 外置。

可借鉴点：

1. Trace 不是 runtime 事实源，只是观测结果。
2. 大 payload 外置，主图保留 ref。
3. inference trace 不影响 provider 请求成功失败。

aiops-v2 当前 `modeltrace` v1 payload 同时包含 provider request、prompt trace、diagnostic trace、governance、工具面、agent 状态和重复统计字段。

建议按已有上下文迁移设计推进 Trace v2。

## 3. aiops-v2 当前分层评估

当前 aiops-v2 实际上有 7 层：

```text
AppUI / Transport
  -> Runtime Contract
  -> Session / Turn State
  -> Turn Loop
  -> Context / Prompt / Model Input
  -> Tool Surface / Dispatch / Policy
  -> Eino Provider Adapter
```

### 3.1 已经做得好的部分

1. `RuntimeGateway` 已把 appui 和 runtimekernel 隔开，只暴露 `RunTurn/ResumeTurn/CancelTurn`。
2. `TurnRequest` / `TurnResult` 是 aiops-v2 自己的 typed contract，没有绑定 Eino。
3. `SessionState` 已保存 active turn、pending approval、pending evidence、tool discovery、skill activation、context governance。
4. `runHostIterationLoop` 已把 context pipeline、prompt compile、tool surface、model call、tool result 回灌放进一个循环。
5. `AgentConfigRunner` 已让 child agent 走同一 `RunTurn` 路径。

### 3.2 当前主要问题

1. `EinoKernel` 名字掩盖了 aiops-v2 自己的 runtime owner，容易让设计继续围绕 Eino 类型展开。
2. `runtimekernel/eino_kernel.go` 过大，包含 turn lifecycle、context pipeline、prompt build、tool dispatch、model stream、final gate、trace 写入。
3. `promptinput.Builder` 直接产出 `schema.Message[]`，导致 Eino DTO 成为内部模型输入事实源。
4. `modeltrace.Request` 直接接受 `schema.Message[]` 和大量 top-level 派生字段，Trace writer 有反向解释 prompt 的倾向。
5. provider request 由 Eino 隐式构造，缺少 aiops-v2 自己可观测、可 hash、可测试的 `ProviderRequestSnapshot`。
6. tool surface 已有 snapshot，但 registered/model-visible/dispatchable 的关系还没有像 Codex ToolRouter 那样成为明确边界。

## 4. 目标分层

建议把 aiops-v2 Agent Runtime 目标分为 9 层：

```text
L0 App / Transport Adapter
L1 Runtime Contract
L2 Session Store / Thread State
L3 RuntimeTurnContext
L4 RuntimeStepContext
L5 Context Compiler / Prompt Compiler / ModelInput
L6 Tool Router / Dispatcher / Approval Gates
L7 Provider Adapter
L8 Trace / Projection / Diagnostics
```

### L0：App / Transport Adapter

Owner：`internal/appui`、`internal/server`

职责：

1. 接收 UI/API 请求。
2. 解析 chat route、host mention、用户证据、client ids。
3. 构造 `runtimekernel.TurnRequest`。
4. 渲染 `TurnItem -> AiopsTransportState -> AssistantTransport`。

禁止事项：

1. 不直接写 completed/failed turn。
2. 不直接执行 HostOps mission。
3. 不直接拼模型输入。
4. 不从 final markdown 推断 runtime state。

### L1：Runtime Contract

Owner：`internal/runtimekernel/types.go`

职责：

1. 定义 `TurnRequest`、`TurnResult`、`ResumeRequest`、`CancelRequest`。
2. 定义外部能调用 runtime 的最小 typed contract。
3. 保持与 Eino provider DTO 解耦。

建议：

1. 保持当前 contract。
2. 后续新增 `RuntimeTurnContextSnapshot` 只作为 trace/diagnostic 输出，不让 appui 构造内部字段。

### L2：Session Store / Thread State

Owner：`runtimekernel.SessionManager`、`SessionState`

职责：

1. 保存 thread 级 conversation、current turn、turn history、active turn。
2. 保存 pending approvals/evidence、context governance、tool/skill/MCP activation。
3. 作为 user turn、resume、cancel 的状态事实源。

建议：

1. 明确 `SessionState` 等价于 Codex thread/session。
2. active regular turn 的追加输入必须进入 runtime input queue 或 pending input，不允许 appui 另起完成路径。

### L3：RuntimeTurnContext

Owner：`runtimekernel`

新增结构建议：

```go
type RuntimeTurnContext struct {
    SessionID string
    TurnID string
    ClientTurnID string
    ClientMessageID string
    SessionType SessionType
    Mode Mode
    Route RuntimeRouteSnapshot
    Profile string
    HostID string
    Model modelrouter.ModelCapabilities
    Permission RuntimePermissionSnapshot
    ContextBudget ContextBudgetThresholds
    ToolPolicyHash string
    Lineage modelrouter.ContextLineageMetadata
    Metadata map[string]string
}
```

职责：

1. 冻结单个 turn 的 route/profile/permission/model/context budget。
2. 给 prompt compiler、tool router、provider snapshot、trace 共享同一份 turn 视图。
3. 防止运行中多个模块重算 route 或权限。

### L4：RuntimeStepContext

Owner：`runtimekernel`

新增结构建议：

```go
type RuntimeStepContext struct {
    Turn RuntimeTurnContext
    Iteration int
    ContextState ContextPipelineResult
    Compiled promptcompiler.CompiledPrompt
    ModelInput []promptinput.ModelInputItem
    ToolSurface RuntimeToolRouterSnapshot
    ProviderRequest modelrouter.ProviderRequestSnapshot
}
```

职责：

1. 表示一次 sampling/model call 的完整快照。
2. 确保 context、prompt、tools、provider request、trace 来自同一个 step。
3. 支持 retry、resume、mid-turn compaction 后重新建 step。

### L5：Context Compiler / Prompt Compiler / ModelInput

Owner：

```text
context pipeline -> runtimekernel context modules
prompt compiler -> internal/promptcompiler
model input -> internal/promptinput
```

职责：

1. `ContextPipeline` 只决定保留、压缩、外置和治理事件。
2. `PromptCompiler` 只产出 `PromptEnvelope` / `CompiledPrompt`。
3. `PromptInput` 只产出 `ModelInputItem[]`，Eino `schema.Message[]` 只是 adapter 输出。

禁止事项：

1. prompt compiler 不构造 provider request。
2. promptinput 不写 Trace。
3. Eino message 不作为内部事实源。

### L5.1：ModelInputItem 到 schema.Message 的准确度方案

`ModelInputItem -> schema.Message` 是迁移中最大的准确度风险，因为任何 role、tool call id、tool result、metadata 或消息顺序丢失，都会改变模型行为。解决方案不是保留旧 `schema.Message` 路径，而是把 `ModelInputItem` 设计成 provider message 的无损超集，再把 Eino 转换器做成唯一、严格、可审计的出口。

`ModelInputItem` 必须至少覆盖：

```go
type ModelInputItem struct {
    ID string
    ProviderRole string
    SemanticRole string
    Content string
    ContentParts []ModelInputContentPart
    Name string
    ToolCalls []ModelInputToolCall
    ToolCallID string
    ToolResult *ModelInputToolResult
    Source ModelInputSource
    Phase string
    CacheGroup string
    Metadata map[string]string
}
```

转换规则：

1. `ModelInputItem` 是内部事实源；`schema.Message` 是 Eino adapter DTO。
2. `ModelInputItemsToEinoMessages` 必须是唯一生产转换函数，放在 `internal/modelrouter` 或 provider adapter 边界。
3. 转换器遇到无法映射的字段必须返回错误，不能静默丢弃。
4. adapter 必须生成 `ProviderMessageAudit`，记录 item id、role、tool call id、content hash、metadata hash 和转换后的 provider message hash。
5. `ProviderRequestSnapshot` 同时保存 `modelInputHash` 和 `providerMessagesHash`，用于证明 provider 实际输入来自同一批 item。
6. `prompt_cache_key` 只基于稳定前缀、工具表、provider 参数和明确 cache group 计算，不混入 trace id、turn id、时间戳、stream metrics 等易变字段。

必须补齐的测试门禁：

1. Golden parity：用现有典型 turn fixture 生成新 `ModelInputItem[]`，确认转换后的 Eino messages 与迁移前 provider messages 在 role、content、tool_calls、tool_call_id、name、extra 维度一致。该测试只用于迁移校验，不保留旧生产路径。
2. Round-trip：对支持的 provider message 类型执行 `ModelInputItem -> schema.Message -> ModelInputItemForTest`，确认无字段丢失。
3. Strict validation：缺少 tool call id、非法 role、无法 JSON 化的 tool arguments、未知 content part type 都必须失败。
4. Hash audit：`ProviderRequestSnapshot.providerMessagesHash` 必须等于实际送入 Eino adapter 的 messages hash。
5. Static guard：除白名单 adapter 文件和测试外，`internal/promptcompiler`、`internal/promptinput`、`internal/modeltrace`、`internal/runtimekernel` 不允许 import `github.com/cloudwego/eino/schema`。

### L6：Tool Router / Dispatcher / Approval Gates

Owner：

```text
tool surface -> internal/tooling
dispatch / approval / resource lock -> runtimekernel
```

建议结构：

```go
type RuntimeToolRouterSnapshot struct {
    RegisteredTools []string
    ModelVisibleTools []string
    DispatchableTools []string
    HiddenReasons map[string][]string
    PolicyHash string
    Fingerprint string
}
```

职责：

1. 输出模型可见工具 schema。
2. 校验模型调用的工具是否 dispatchable。
3. 统一 approval、permission、resource lock、idempotency。
4. 把 denied/approval_needed/tool_error 作为同一 turn 的 model-visible continuation。

### L7：Provider Adapter

Owner：`internal/modelrouter`

职责：

1. 选择 provider/model。
2. 把 `ProviderRequestSnapshot` 转为具体 provider/Eino 调用。
3. 执行 streaming/generate fallback。
4. 解析 provider-specific reasoning/usage/finish reason。

Eino 定位：

```text
Eino belongs here.
```

Eino 可以继续提供：

1. `model.ChatModel`
2. `schema.Message`
3. `schema.ToolInfo`
4. `tool.BaseTool`
5. `model.Option`

但 Eino 不应该拥有：

1. turn lifecycle
2. session state
3. context governance
4. prompt truth source
5. trace schema
6. approval/resume semantics

### L8：Trace / Projection / Diagnostics

Owner：

```text
Trace -> internal/modeltrace
Projection -> internal/appui / internal/projection
Diagnostics -> internal/promptdiag / internal/eval
```

职责：

1. 消费 RuntimeTurnContext、RuntimeStepContext、ProviderRequestSnapshot。
2. 写 Trace v2、summary、raw payload refs。
3. 给 UI/eval/promptdiag 提供只读投影。

禁止事项：

1. 不反向拼 Prompt。
2. 不反向生成 provider request。
3. 不把 preview 回流进 runtime。

## 5. Codex 分层可移植性判断

| Codex 分层能力 | 是否移植 | aiops-v2 做法 |
| --- | --- | --- |
| Session 作为 thread owner | 借鉴 | `SessionState` 保持 owner，补强 active turn/input queue 约束 |
| TurnContext | 移植概念 | 新增 `RuntimeTurnContext`，冻结 turn 级 route/profile/permission/model |
| StepContext | 移植概念 | 新增 `RuntimeStepContext`，冻结 iteration/model-call 级上下文 |
| ResponseItem 作为模型输入 | 借鉴 | 用无损 `ModelInputItem`，不直接复制 Rust enum，也不保留 `schema.Message` 事实源 |
| ToolRouter | 借鉴 | 强化 `ToolSurfaceSnapshot` 为 router snapshot，统一 visible/dispatchable |
| Prompt 与 provider request 分离 | 移植 | `CompiledPrompt/ModelInputItem` 与 `ProviderRequestSnapshot` 分离 |
| ResponsesApiRequest 字段 | 部分移植 | 保留通用字段，Eino-specific 调用仍由 adapter 转换 |
| prompt_cache_key/request property compare | 移植 | 新增 `promptCacheKey` 与 `requestPropertiesHash` |
| Rollout raw payload refs | 移植 | Trace v2 增加 `RawPayloadRef`，旧 trace writer 不进入最终 runtime |
| Codex exact prompt / Rust runtime | 不移植 | aiops-v2 保留 AIOps prompt、Eino provider、Go runtime |
| Codex Lite mode/Azure store/websocket reuse | 暂不移植 | 只保留字段可观测，不硬编码 provider 特化 |

## 6. 修改方案

这次迁移采用“一步到位的单 runtime 切换”，不是灰度兼容方案。实现可以拆成多个提交，但最终交付不允许保留旧 runtime 入口、旧 prompt input 事实源、旧 trace writer 或旧 Eino-facing compiler 接口。

切换原则：

1. 不加 runtime feature flag。
2. 不保留旧 `RunTurn` 成功路径。
3. 不双写 v1/v2 trace。
4. 不让 appui/promptdiag/eval 读取旧 trace 作为生产诊断来源。
5. 不让 `schema.Message` 穿透到 `promptcompiler`、`promptinput`、`runtimekernel`、`modeltrace`。
6. 历史 trace 文件不迁移，调试页面只读取新 v2 index 和 v2 document。

### Step 1：建立最终类型和删除边界

修改：

1. 新增 `RuntimeTurnContext`、`RuntimeStepContext`、`RuntimeToolRouterSnapshot`。
2. 新增 `ModelInputItem`、`ProviderRequestSnapshot`、`ProviderResponse`、`TraceDocumentV2`。
3. 从 `promptcompiler.Compiler` 接口删除 `CompileForEino`。
4. 从 `promptinput.BuildResult` 删除 `Messages []*schema.Message`。
5. 从 `modeltrace.Request` 删除 `ModelInput []*schema.Message`。

验收：

1. `promptcompiler` 和 `promptinput` 不再 import Eino schema。
2. runtime 内部只能传 `ModelInputItem[]` 和 `ProviderRequestSnapshot`。
3. 旧接口删除后，编译错误必须集中暴露所有待迁移调用点。

### Step 2：改造 PromptInput 为无损事实源

修改：

1. `promptinput.Builder.Build` 只返回 `ModelInputItem[]` 和 semantic trace。
2. prompt section、ops context capsule、memory、history、tool result、assistant tool call 都生成结构化 item。
3. 每个 item 必须携带稳定 `ID`、`ProviderRole`、`SemanticRole`、`Source`、`Phase` 和 hash 所需字段。
4. `PromptInputTrace` 从 `ModelInputItem[]` 派生，不再从 `schema.Message[]` 派生。

验收：

1. item 顺序等于 provider message 顺序。
2. system/developer/tool/runtime context/history/tool result 的语义层不丢失。
3. Prompt diff 以 item id 和 content hash 对齐，不依赖 markdown 反推。

### Step 3：把 Provider Adapter 收口到 modelrouter

修改：

1. 在 `modelrouter` 新增唯一 Eino message adapter：`ModelInputItemsToEinoMessages`。
2. 在 `modelrouter` 新增 `ProviderAdapter.Call(ctx, ProviderRequestSnapshot)`。
3. 把 `chatModel.Stream/Generate`、stream stats、finish reason、usage、reasoning extra 解析移出 `runtimekernel`。
4. `runtimekernel` 只调用 provider adapter，不直接接触 `schema.Message` 或 `model.ChatModel`。

验收：

1. `ProviderRequestSnapshot.providerMessagesHash` 等于实际送入 Eino 的 messages hash。
2. adapter audit 能定位每条 provider message 来自哪个 `ModelInputItem`。
3. runtime 没有第二条 model call 路径。

### Step 4：重写 TurnLoop 到 StepContext

修改：

1. 将 `runHostIterationLoop` 拆成 `buildRuntimeTurnContext`、`buildRuntimeStepContext`、`executeRuntimeStep`。
2. context pipeline、prompt compiler、tool router、provider snapshot、trace 都从同一个 step context 读取。
3. retry、resume、tool continuation、final gate 都复用同一 turn loop。
4. 将 `EinoKernel` 重命名为 `RuntimeKernel`，构造函数改为 `NewRuntimeKernel`；不保留旧 alias。

验收：

1. 代码里只有一个 runtime kernel 类型和一个 turn loop。
2. `AgentConfigRunner`、host agent、workspace agent 都走同一个 `RunTurn/ResumeTurn/CancelTurn`。
3. appui、hostops、approval service 不能绕过 runtime 写 completed/failed turn。

### Step 5：收敛 ToolRouterSnapshot

修改：

1. `tooling` 输出 registered/modelVisible/dispatchable/hiddenReasons/policyHash/fingerprint。
2. provider request 的 tools 来自 `ModelVisibleTools`。
3. dispatcher 只接受同一个 `RuntimeStepContext.ToolSurface.DispatchableTools`。
4. hidden/denied/approval_needed/tool_error 必须作为同一 turn 的 model-visible continuation。

验收：

1. 模型可见工具和可 dispatch 工具来自同一 snapshot。
2. hidden tool 调用不会执行，只返回结构化 unavailable result。
3. Trace v2 能解释每个工具为什么可见、隐藏、只可摘要或需要审批。

### Step 6：切 Trace v2 和前端投影

修改：

1. 删除 v1 trace writer。
2. `modeltrace` 只写 `TraceDocumentV2`、summary index 和 raw payload refs。
3. promptdiag、eval、PromptTrace UI 只读 v2 view model。
4. 主 Chat UI 继续消费 `TurnItem -> transport -> UI`，不读取 trace 作为 runtime 状态。

验收：

1. Trace v2 只消费 runtime 快照，不反向拼 prompt 或 provider request。
2. PromptTrace 页面能展示 step、provider request、tool surface、raw payload refs。
3. `preview` 只作为 UI/diagnostic 摘要，不能进入 prompt、provider request 或 runtime state。

### Step 7：删除旧代码和加静态门禁

修改：

1. 删除 `promptcompiler/eino_format.go` 或将其内容迁到 provider adapter 测试 fixture。
2. 删除旧 `modeltrace.Request` v1 writer 路径。
3. 删除 runtime 中直接调用 Eino `chatModel.Stream/Generate` 的函数。
4. 增加 `rg`/Go test 静态门禁，限制 `schema.Message` import 白名单。

验收：

1. `rg "CompileForEino|BuildResult.*Messages|ModelInput.*schema.Message|chatModel.Stream|chatModel.Generate"` 不命中生产 runtime 旧路径。
2. `go test ./internal/runtimekernel ./internal/promptinput ./internal/promptcompiler ./internal/modelrouter ./internal/modeltrace ./internal/appui` 通过。
3. 没有 v1/v2 双写、旧 reader fallback、旧 runtime alias。

## 7. 目标目录与文件边界

建议新增或重构：

```text
internal/runtimekernel/runtime_kernel.go
internal/runtimekernel/runtime_turn_context.go
internal/runtimekernel/runtime_step_context.go
internal/runtimekernel/turn_loop.go
internal/runtimekernel/tool_router_snapshot.go
internal/modelrouter/provider_adapter.go
internal/modelrouter/provider_request_snapshot.go
internal/modelrouter/provider_response.go
internal/modelrouter/eino_message_adapter.go
internal/promptinput/model_input_item.go
internal/promptinput/model_input_validation.go
internal/modeltrace/trace_v2.go
internal/modeltrace/raw_payload.go
```

文件职责：

| 文件 | 职责 |
| --- | --- |
| `runtime_kernel.go` | 唯一 RuntimeKernel 类型、构造函数、公共 runtime gateway 实现 |
| `runtime_turn_context.go` | turn 级冻结快照 |
| `runtime_step_context.go` | iteration/model-call 级快照 |
| `turn_loop.go` | RunTurn/ResumeTurn shared loop |
| `tool_router_snapshot.go` | 工具注册、可见、可 dispatch、隐藏原因快照 |
| `provider_adapter.go` | provider 调用统一入口 |
| `provider_request_snapshot.go` | provider 请求规范快照、cache key 和 hash |
| `provider_response.go` | provider 输出、usage、finish reason、stream metrics |
| `eino_message_adapter.go` | 唯一 `ModelInputItem -> schema.Message` 转换器 |
| `model_input_item.go` | 内部模型输入事实源 |
| `model_input_validation.go` | item 校验、hash、golden/round-trip 测试辅助 |
| `trace_v2.go` | Trace v2 writer 和 view |
| `raw_payload.go` | raw request/response/tool result 外置引用 |

需要删除或迁移：

| 旧文件/旧接口 | 处理方式 |
| --- | --- |
| `promptcompiler.CompileForEino` | 删除 runtime 接口 |
| `promptcompiler/eino_format.go` | 删除或迁到 adapter 测试 fixture |
| `promptinput.BuildResult.Messages` | 删除 |
| `modeltrace.Request.ModelInput []*schema.Message` | 删除 |
| runtime 内直接 `chatModel.Stream/Generate` | 移到 `modelrouter.ProviderAdapter` |
| v1 trace writer/reader runtime import | 删除 |
| `EinoKernel` / `NewEinoKernel` | 重命名为 `RuntimeKernel` / `NewRuntimeKernel`，不保留 alias |

## 8. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 一步到位切换范围大 | 在一个迁移分支内分提交推进，但最终不加 feature flag、不留 fallback；每个提交尽量保持编译可定位 |
| `ModelInputItem -> schema.Message` 转换丢字段 | `ModelInputItem` 作为 provider message 无损超集；adapter strict validation；golden parity；round-trip；hash audit |
| Eino DTO 与 ModelInputItem 双写 | 删除 `BuildResult.Messages`；`schema.Message` 只允许 adapter 白名单 import；静态扫描保护 |
| RuntimeTurnContext 与 TurnRequest 字段重复漂移 | TurnRequest 是外部 contract，RuntimeTurnContext 只能由 runtime builder 派生 |
| ToolSurfaceSnapshot 和 dispatcher 状态漂移 | provider tools 与 dispatcher 都读取同一个 `RuntimeStepContext.ToolSurface` |
| Trace v1/v2 双写残留 | 删除 v1 writer 和 runtime reader；PromptTrace UI 只接 v2 index/view |
| PromptTrace UI 被新 trace schema 破坏 | UI/API 在同一切换中改到 v2 view model；不保留 v1 tab 或 fallback |
| `prompt_cache_key` 不稳定导致缓存命中率下降 | cache key 只包含稳定 prefix、工具表、provider 参数和 cache group；排除 turn id、trace id、时间戳、stream metrics |
| 重命名 RuntimeKernel 造成大面积 churn | 与单 runtime 切换同批完成；用编译错误驱动调用点更新，不保留 `EinoKernel` alias |

## 9. 验收标准

1. Agent Runtime 可以用 L0-L8 分层解释，每层有唯一 owner。
2. 代码里只有一个 runtime kernel 类型：`RuntimeKernel`。
3. 代码里只有一条 turn loop：`RunTurn/ResumeTurn/CancelTurn -> RuntimeTurnContext -> RuntimeStepContext -> ProviderAdapter`。
4. Eino 只在 provider/tool adapter 层是核心依赖，不再作为 runtime 内部事实源。
5. 每次 model call 都有 `RuntimeStepContext`，能解释 context、prompt、tools、model input、provider request、trace。
6. `ProviderRequestSnapshot` 可以说明 provider 实际看到什么，并带有 `modelInputHash`、`providerMessagesHash`、`requestPropertiesHash`、`promptCacheKey`。
7. `ModelInputItem` 是 promptinput、ProviderRequestSnapshot、Trace v2、promptdiag 的事实源。
8. `schema.Message` 只出现在 `internal/modelrouter/eino_message_adapter.go`、Eino provider adapter、`internal/tooling/eino_adapter.go` 和 adapter 测试白名单里。
9. Tool visible/dispatchable 状态来自同一 snapshot。
10. Trace v2 只读 runtime 快照，不反向拼 prompt 或 provider request。
11. v1 trace writer、v1 runtime reader、旧 Eino-facing compiler 接口不存在。
12. appui、hostops、approval service 不能绕过 `RunTurn/ResumeTurn/CancelTurn` 写最终 turn 状态。

## 10. 推荐实施顺序

推荐按单次切换顺序推进：

1. 新增最终类型：`RuntimeTurnContext`、`RuntimeStepContext`、`ModelInputItem`、`ProviderRequestSnapshot`、`ProviderResponse`、`TraceDocumentV2`。
2. 删除旧接口并让编译暴露调用点：`CompileForEino`、`BuildResult.Messages`、`modeltrace.Request.ModelInput []*schema.Message`。
3. 改 `promptinput.Builder`，让它只产出 `ModelInputItem[]`。
4. 在 `modelrouter` 实现唯一 Eino adapter 和 `ProviderAdapter.Call`。
5. 改 runtime turn loop：用 StepContext 构造 provider request，不直接调用 Eino。
6. 收敛 ToolRouterSnapshot：provider-visible 和 dispatchable 同源。
7. 切 Trace v2、promptdiag、eval、PromptTrace UI。
8. 重命名 `EinoKernel` 为 `RuntimeKernel`，删除旧 alias 和旧 constructor。
9. 删除旧文件、旧 reader、旧 writer、旧 adapter。
10. 加静态扫描、golden parity、round-trip、hash audit 和核心 runtime 回归测试。

这样做的价值最大：最终只有一套 agent runtime 逻辑，所有模块都围绕 `RuntimeStepContext` 和 `ProviderRequestSnapshot` 对齐。迁移期可以用测试 fixture 对比旧 provider messages，但旧生产路径不能留在最终代码里。
