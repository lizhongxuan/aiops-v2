# Agent Runtime 中的 Code-driven Assembly：新增 Agent 不能只写 Prompt

> Code-driven Assembly 是 Runtime 在每一轮请求中，用代码根据结构化状态动态装配 prompt、context、tools、policy、loop 和 final contract 的过程。


```text
产品经理: 
我原来的 Agent Runtime 已经能跑了。
那我再加一个合同审核 Agent、财务分析 Agent、工单分派 Agent，是不是只要多写几个 system prompt？
```

这个想法很常见，也很危险。

如果 Agent 只是聊天助手，换 Prompt 可能够用。但只要 Agent 开始接触真实业务系统、私有上下文、工具调用、审批流程、结构化输出和回归评估，Agent 就不再是一段 Prompt，而是一个运行时角色。

一个运行时角色至少包含这些东西：

- 它什么时候被选中。
- 它能看什么上下文。
- 它能看见哪些工具。
- 它能不能执行动作。
- 它执行动作前是否需要审批。
- 它每一步如何停止、重试、降级。
- 它最后必须交付什么结构。
- 它出错后如何追踪和修复。
- 它优化以后，如何证明没有影响其他 Agent。

这些问题都不是静态 Prompt 能独立解决的。

没有 Code-driven Assembly，多个 Agent 很容易变成“多个不同人设的聊天窗口”。有了 Code-driven Assembly，Agent 才能成为一个可治理、可复现、可测试、可持续优化的运行时组件。

## 一、先把 Agent Runtime 拆开看

要讲清楚 Code-driven Assembly，先要把单个 Agent Runtime 的层次讲清楚。很多争论来自一个误解：把 Agent Runtime 等同于 LLM 调用，或者等同于 Prompt。

一个实用的 Agent Runtime 至少有 9 层：

```text
User Turn
  -> Router
  -> AgentDefinition
  -> Assembly
  -> PromptCompiler
  -> Model Call
  -> Tool Loop
  -> Final Projector
  -> Trace / Eval
```

每层职责不同。

| 层级 | 负责什么 | 常见错误 |
| --- | --- | --- |
| User Turn | 接收用户输入、附件、选择资源、会话状态 | 只把用户文本当输入 |
| Router | 决定哪个 Agent 处理 | 用关键词硬分流 |
| AgentDefinition | 描述 Agent 的稳定身份和默认能力 | 只有一个 prompt 文件 |
| Assembly | 动态决定本轮上下文、工具、策略、循环 | 分散在各处临时拼 |
| PromptCompiler | 把结构化 sections 编译成模型输入 | 各业务代码自己拼字符串 |
| Model Call | 调模型 | 把模型当成权限和流程引擎 |
| Tool Loop | 执行工具、观察结果、重试或停止 | 模型可见工具和实际可执行工具不一致 |
| Final Projector | 把模型输出投影成产品结果 | 直接把模型自由文本返回给用户 |
| Trace / Eval | 记录、复现、诊断、回归 | 只存最终答案，不存装配过程 |

这里最容易被低估的是 Assembly 层。

Router 只回答“谁来处理”。PromptCompiler 只回答“怎么把输入变成 message”。ToolDispatcher 只回答“工具怎么执行”。真正连接业务状态和模型输入的是 Assembly。

如果没有 Assembly 层，代码里就会出现大量这种逻辑：

```go
if strings.Contains(input, "合同") {
    prompt += "你是合同审核专家"
    tools = append(tools, readContractTool)
}

if strings.Contains(input, "付款") {
    prompt += "请注意付款风险"
    tools = append(tools, paymentTool)
}
```

这段代码看起来能跑，但工程上有几个致命问题：

- 路由依据是脆弱的字符串，不是结构化事实。
- 工具暴露绕过了统一权限。
- PromptCompiler 看不到真实 section 来源。
- Trace 无法解释为什么本轮能调用这个工具。
- 复制到第二个 Agent 后，逻辑开始分叉。
- 后续修一个 Agent，很容易影响另一个 Agent。

所以，成熟 Runtime 里需要一个明确的装配层：

```text
Structured Runtime State
  -> AgentAssemblySpec
  -> CompiledPrompt + AssembledTools + LoopPolicy + FinalContract
```

这就是 Code-driven Assembly 的位置。

## 二、四类工程：Prompt、Context、Harness、Loop

讨论新增 Agent 时，最好把工作拆成四类工程。

| 工程 | 通俗理解 | 主要产物 | 能不能多个 Agent 共用 |
| --- | --- | --- | --- |
| Prompt Engineering | 岗位说明书 | profile、口径、输出语气、方法论 | 基础片段可共用，岗位 profile 独占 |
| Context Engineering | 本轮材料包 | history、resource、retrieval、evidence、summary | 管道可共用，选择策略独占 |
| Harness Engineering | 运行底座 | session、tool registry、权限、审批、adapter、trace | 大部分共用 |
| Loop Engineering | 工作流 | plan、act、observe、verify、retry、stop | 框架可共用，策略通常独占 |

这四类工程的边界非常重要。

### 1. Prompt Engineering 解决“怎么说”

Prompt profile 适合放稳定规则：

- 这个 Agent 的职责边界。
- 它的分析方法。
- 它的输出格式。
- 它不能做什么。
- 它面对不确定性时如何表达。

例如合同审核 Agent 的 profile 可以写：

```text
你负责识别合同风险。
重点检查主体、金额、交付、验收、违约、终止、争议解决、数据安全和合规条款。
每个风险必须包含条款位置、风险原因、建议动作。
你不能批准合同，不能发起付款，不能修改合同原文。
```

这些内容稳定，可以放静态 Prompt。

但 profile 不应该承担这些工作：

- 判断用户是否有权限看合同。
- 判断本轮合同 ID 是否可信。
- 判断付款工具是否可见。
- 判断是否需要人工审批。
- 判断工具调用预算是否耗尽。
- 判断某个检索结果是否应该注入。

这些是 Runtime 事实，必须由代码决定。

### 2. Context Engineering 解决“给它看什么”

很多 Agent 答错，不是因为 Prompt 写得差，而是因为上下文装错了。

常见上下文错误有三类：

- 缺材料：模型不知道关键事实，只能猜。
- 塞太多：无关历史、旧结论、过期材料干扰判断。
- 混边界：一个 Agent 看到了另一个 Agent 的私有过程或工具结果。

Context Engineering 要解决的是：

```text
本轮 Agent 需要哪些材料？
哪些材料可以原文进入？
哪些材料只能摘要？
哪些材料必须带来源？
哪些材料必须禁止进入？
```

这不是 Prompt 文案问题，而是选择、裁剪、排序、压缩和引用治理问题。

### 3. Harness Engineering 解决“怎么跑”

Harness 是 Agent 的运行底座，包括：

- session 管理。
- turn 管理。
- model provider。
- tool registry。
- tool dispatcher。
- permission engine。
- approval gate。
- memory/retrieval。
- trace。
- eval。
- adapter。

这部分应该尽量共用。新增 Agent 不应该重新写一套模型调用、工具执行、审批和日志。

如果每个 Agent 都自己实现工具调用，很快就会出现：

- timeout 策略不一致。
- 错误格式不一致。
- 权限判断不一致。
- 工具结果进入 Prompt 的方式不一致。
- trace 无法横向比较。

所以，Harness 应该公共化。

### 4. Loop Engineering 解决“按什么流程工作”

Loop 是 Agent 的工作节奏。

不同 Agent 的 loop 可以完全不同：

```text
问答型 Agent:
  read context -> answer

查询型 Agent:
  understand question -> choose query tool -> execute -> explain result

审核型 Agent:
  read document -> extract items -> check rules -> produce findings

执行型 Agent:
  inspect -> plan -> request approval -> execute -> verify -> report

管理型 Agent:
  decompose -> delegate -> wait -> merge -> verify
```

这也是为什么多个 Agent 不能只靠不同静态 Prompt 区分。

如果两个 Agent 的工具面、上下文、loop、final contract 都一样，只是 system prompt 不同，那它们本质上只是两个 profile。如果它们的工作流、权限、证据要求和停止条件不同，它们才是 runtime 意义上的不同 Agent。

## 三、Code-driven Assembly 到底装配什么

Code-driven Assembly 的核心不是“用代码拼 Prompt 字符串”。它应该输出一组同源的运行时产物。

推荐把输出抽象成：

```text
AgentAssemblySpec:
  agent identity
  route reason
  resource binding
  prompt sections
  context sources
  visible tools
  dispatchable tools
  loop policy
  final contract
  budget
  approval policy
  trace labels
```

注意这里有两个关键词：**同源** 和 **结构化**。

### 1. 同源

模型可见的工具、Runtime 实际可执行的工具、Trace 里记录的工具，必须来自同一个装配结果。

不能出现这种情况：

```text
Prompt 里告诉模型可以调用 A、B。
Eino tool pool 里注册了 A、B、C。
Runtime dispatcher 实际允许 A、D。
Trace 里只记录了 A。
```

这种系统一旦出问题，很难诊断。

正确做法是：

```text
ToolRegistry
  -> ToolSurfacePolicy
  -> AssembledTools
       -> prompt visible tools
       -> Eino tool pool
       -> runtime dispatch allowlist
       -> trace fingerprint
```

同一份 AssembledTools 生成四个投影。

### 2. 结构化

Prompt section 不应该只是字符串数组，至少要带元信息：

```go
type PromptSection struct {
    ID          string
    Role        string
    Content     string
    Source      string
    Priority    int
    TokenBudget int
    Fingerprint string
}
```

为什么要这么麻烦？

因为当模型答错时，你需要知道：

- 哪个 section 影响了它。
- section 来自哪个 profile 或代码分支。
- 这段内容是不是本轮新注入的。
- 它占了多少 token。
- 它有没有被裁剪。
- 它和上一个版本有什么 diff。

没有结构化 section，Prompt Trace 就只能看到一大坨文本。

看不见来源，就无法治理。

## 四、一个新增 Agent 的装配链路

在已有 Runtime 中新增一个 Agent，建议先不要写 Prompt，而是先画出这条链路：

```text
TurnRequest / SessionState
  -> RouteDecision
  -> AgentDefinition
  -> AgentAssemblySpec
  -> PromptCompiler
  -> ModelInput
  -> Framework Adapter
  -> Runtime Loop
  -> Final Projector
  -> Trace / Eval
```

下面逐段拆。

### 1. TurnRequest：不要只传用户文本

TurnRequest 至少应该包含：

```go
type TurnRequest struct {
    UserInput       string
    SessionID       string
    UserID          string
    Attachments     []AttachmentRef
    SelectedResource *ResourceRef
    RouteMetadata   map[string]string
    UserConstraints []Constraint
}
```

用户文本只是输入的一部分。很多关键事实来自 UI、业务系统或上游流程，例如：

- 用户当前选中了哪个文件。
- 用户从哪个页面发起请求。
- 用户是否上传了附件。
- 当前任务类型是什么。
- 用户的角色和权限是什么。
- 会话里是否有待审批动作。

如果这些事实不进入 TurnRequest，后面只能靠 LLM 从自然语言里猜。

### 2. RouteDecision：用结构化信号选 Agent

路由不应该只靠关键词。

不推荐：

```text
if input contains "合同" then ContractReviewAgent
```

推荐：

```text
if selectedResource.kind == "contract"
or attachment.mimeType in contractMimeTypes
or routeMetadata.taskType == "contract_review"
then ContractReviewAgent
```

关键词可以参与意图分类，但不能作为执行边界的唯一事实源。

路由结果也要进入 trace：

```go
type RouteDecision struct {
    AgentID string
    Reason  string
    Signals []RouteSignal
    Confidence float64
}
```

否则你只知道某个 Agent 答错了，不知道为什么是它在答。

### 3. AgentDefinition：定义稳定身份

AgentDefinition 是新增 Agent 的注册信息。

```yaml
id: contract_review_agent
kind: reviewer
profile: contract_risk_review
description: Review contract clauses and produce risk findings.
defaultMode: review
maxIterations: 6
toolScopes:
  - contract.read
  - contract.compare
  - clause.search
finalContract: ContractReviewReport
```

它只放稳定信息，不放本轮动态状态。

例如：

- 可以放默认 profile。
- 可以放默认 tool scope。
- 可以放最大循环次数。
- 可以放默认 final contract。

不应该放：

- 本轮合同 ID。
- 本轮审批状态。
- 本轮用户权限快照。
- 本轮检索结果。
- 本轮工具健康状态。

这些应该在 Assembly 阶段动态注入。

### 4. AgentAssemblySpec：把本轮事实装配起来

这是核心对象。

```go
type AgentAssemblySpec struct {
    AgentID  string
    Kind     string
    Profile  string
    Mode     string

    Route    RouteDecision
    Resource ResourceBinding
    Context  ContextSpec
    Tools    ToolSurfaceSpec
    Prompt   PromptSpec
    Loop     LoopPolicy
    Final    FinalContract
    Budget   BudgetSpec
    Trace    TraceSpec
}
```

它有两个作用。

第一，它是 PromptCompiler 的输入。

第二，它是 Runtime Loop 的执行依据。

这点很关键：Code-driven Assembly 不是只服务 Prompt，它同时服务工具、权限、循环、最终输出和诊断。

## 五、Assembly 函数应该怎么写

推荐把装配写成一个明确的函数，而不是散落在 controller、Eino adapter、工具代码和 prompt 文件里。

伪代码如下：

```go
func AssembleAgentTurn(ctx context.Context, req TurnRequest, state SessionState) (AgentAssemblySpec, error) {
    route := routeAgent(req, state)

    agentDef, err := agentRegistry.Get(route.AgentID)
    if err != nil {
        return AgentAssemblySpec{}, err
    }

    resource, err := resolveResource(req, route, agentDef)
    if err != nil {
        return AgentAssemblySpec{}, failClosed("resource_not_resolved", err)
    }

    permission := permissionEngine.Snapshot(ctx, req.UserID, resource)
    mode := decideMode(req, state, agentDef, permission)

    toolCandidates := toolRegistry.List(agentDef.ToolScopes)
    tools := assembleTools(ToolAssemblyInput{
        Agent:      agentDef,
        Mode:       mode,
        Resource:   resource,
        Permission: permission,
        State:      state,
    }, toolCandidates)

    contextSpec := assembleContext(ContextAssemblyInput{
        Agent:    agentDef,
        Mode:     mode,
        Resource: resource,
        State:    state,
        Tools:    tools,
        Budget:   req.ContextBudget,
    })

    loop := selectLoopPolicy(agentDef, mode, req, state)
    final := selectFinalContract(agentDef, mode, req)

    prompt := assemblePromptSections(PromptAssemblyInput{
        BaseContract: baseRuntimeContract(),
        Profile:      profileRegistry.Get(agentDef.Profile),
        RuntimeState: runtimeStateSection(req, state, resource, permission),
        ToolSurface:  toolSurfaceSection(tools),
        Context:      contextSpec,
        Loop:         loop,
        Final:        final,
    })

    return AgentAssemblySpec{
        AgentID:  agentDef.ID,
        Kind:     agentDef.Kind,
        Profile:  agentDef.Profile,
        Mode:     mode,
        Route:    route,
        Resource: resource,
        Context:  contextSpec,
        Tools:    tools,
        Prompt:   prompt,
        Loop:     loop,
        Final:    final,
        Budget:   buildBudget(agentDef, req, state),
        Trace:    buildTrace(route, resource, tools, contextSpec, prompt),
    }, nil
}
```

这段伪代码背后有几个工程原则。

### 原则一：资源绑定必须 fail closed

如果用户说“帮我审核这个合同”，但系统无法确认“这个合同”是哪一个资源，不能让模型自己猜。

错误做法：

```text
模型根据聊天历史猜一个合同。
```

正确做法：

```text
Assembly 阶段解析 selectedResource / attachment / task metadata。
解析失败则 blocked，要求用户明确选择资源。
```

资源绑定必须是代码事实，不是 Prompt 里的提醒。

### 原则二：工具面必须由代码过滤

不要把危险工具暴露给模型，然后在 Prompt 里写“除非必要，不要调用”。

模型可见工具就是模型的行动空间。

如果一个 Agent 不应该发起付款、删除数据、提交审批，那这些工具就不应该进入它的 visible tools。

推荐结构：

```go
type ToolSurfaceSpec struct {
    VisibleTools      []ToolRef
    DispatchAllowlist []ToolRef
    HiddenTools       []ToolRef
    ApprovalRequired  []ToolRef
    Fingerprint       string
}
```

其中：

- `VisibleTools` 给模型看。
- `DispatchAllowlist` 给 Runtime 执行器用。
- `ApprovalRequired` 给审批网关用。
- `Fingerprint` 给 trace 和回归测试用。

### 原则三：动态上下文必须按 Agent 选择

不同 Agent 需要的上下文不一样。

合同审核 Agent 需要合同正文、模板、条款库、审批要求。  
财务分析 Agent 需要财务报表、口径说明、时间范围、汇率规则。  
工单分派 Agent 需要队列状态、人员职责、SLA、任务优先级。

不能把所有 session history、所有检索结果、所有工具输出塞给所有 Agent。

推荐结构：

```go
type ContextSpec struct {
    Sources       []ContextSource
    Excluded      []ContextSource
    EvidenceRules EvidenceRules
    TokenBudget   int
    TrimStrategy  string
}
```

关键不是“多给上下文”，而是“给对上下文”。

### 原则四：每轮都要重新装配

Agent 不是启动时装配一次就结束。

每次模型调用前都可能需要重新 Assembly，因为运行时状态会变化：

- 工具刚返回新结果。
- 用户刚批准或拒绝某个动作。
- 上下文预算耗尽。
- 外部系统不可用。
- 子 Agent 刚回传结果。
- 风险等级从低变高。

因此 Assembly 应该是 per-turn 甚至 per-step 的，而不是只在 NewAgent 时执行。

### 原则五：装配结果必须可追踪、可 diff、可回放

一次错误回答至少要能回答这些问题：

- 本轮为什么路由到这个 Agent？
- 本轮绑定了哪个资源？
- 本轮模型看到了哪些工具？
- 哪些工具虽然注册了但被隐藏？
- 哪些上下文进了 prompt？
- 哪些上下文被裁剪？
- Prompt 每个 section 来自哪里？
- Loop 为什么停止？
- Final contract 是否通过？

如果回答不了，优化就只能靠改 Prompt 猜。

## 六、静态 Prompt 与动态 Assembly 的分工

不是所有东西都要动态化。成熟设计应该同时承认两点：

1. 静态 Prompt 很重要。
2. 只有静态 Prompt 远远不够。

可以放静态 Prompt 的内容：

- 稳定角色定义。
- 通用分析框架。
- 输出格式说明。
- 对不确定性的表达方式。
- 通用安全边界。
- 对证据、假设、结论的写作要求。

必须由 Code-driven Assembly 注入的内容：

- 当前 agent kind。
- 当前 mode。
- 当前绑定资源。
- 当前权限状态。
- 当前可见工具。
- 当前审批状态。
- 当前上下文来源。
- 当前证据缺口。
- 当前 loop 状态。
- 当前 budget。
- 当前 final contract。

一个简单判断标准：

> 如果内容在不同请求、不同用户、不同资源、不同权限、不同工具状态下会变化，就不应该写死在静态 Prompt 里。

再进一步：

> 如果内容会影响权限、工具、资源、审批、执行边界，就不能只写在 Prompt 里，必须有代码约束。

### 哪些东西绝不能只写进 Prompt

有些规则可以在 Prompt 里说明，但必须在代码里执行。

| 规则类型 | 为什么不能只靠 Prompt | Runtime 应该怎么做 |
| --- | --- | --- |
| 权限 | “请不要读取无权限文档”没有安全性 | 权限引擎先过滤，模型看不到无权限材料 |
| 工具 | “不要随便调用危险工具”不能限制行动空间 | ToolSurfacePolicy 决定 visible tools 和 dispatch allowlist |
| 上下文 | “只参考相关材料”不能自动完成检索治理 | ContextPipeline 做 retrieval、ranking、trimming、source control |
| 多 Agent 隔离 | 私有上下文一旦进 prompt，再提醒已经晚了 | Assembly 阶段按 agent kind / mode / resource 隔离上下文 |
| 学习资产 | 全局注入经验会污染其他 Agent | learned asset 必须带作用域、版本、来源、hash、过期策略和 eval case |

这部分是 Code-driven Assembly 的硬边界：Prompt 可以解释规则，但不能成为规则的唯一执行者。

## 七、多个 Agent 之间，哪些共用，哪些独占

新增 Agent 时，不要一上来复制一个旧 Agent 改名。先分清公共组件和独占组件。

### 可以共用的部分

这些应该尽量由 Runtime 统一提供：

| 公共组件 | 为什么共用 |
| --- | --- |
| ModelRouter | 统一模型选择、降级、成本控制 |
| PromptCompiler | 统一 section 编译、token 预算、trace |
| ToolRegistry | 统一工具定义、schema、metadata |
| ToolDispatcher | 统一执行、超时、错误、重试 |
| PermissionEngine | 统一权限判断 |
| ApprovalGate | 统一高风险动作审批 |
| ContextPipeline | 统一 history 裁剪、摘要、引用、证据管理 |
| SessionStore | 统一会话、checkpoint、resume |
| TraceStore | 统一 prompt/tool/route/final trace |
| EvalRunner | 统一 baseline/current 回归 |
| FrameworkAdapter | 统一适配 Eino、LangGraph、AutoGen 等框架 |

这些是 Harness，不应该每个 Agent 自己造。

### 必须独占的部分

这些必须按 Agent 隔离：

| 独占项 | 为什么不能共用 |
| --- | --- |
| Profile | 岗位职责不同 |
| Route Rule | 触发条件不同 |
| Resource Binding | 绑定资源类型不同 |
| Tool Surface Policy | 行动空间不同 |
| Context Source Selector | 需要材料不同 |
| Evidence Contract | 证据要求不同 |
| Loop Policy | 工作流程不同 |
| Final Contract | 输出结构不同 |
| Eval Cases | 准确性标准不同 |
| Learned Assets | 经验只在适用范围内生效 |

这里的隔离不是为了代码洁癖，而是为了防止优化互相污染。

例如你为了提升合同审核 Agent 的准确率，增加了“必须检查违约条款”的规则。如果这条规则进入全局 base prompt，报告生成 Agent、客服 Agent、查询 Agent 都可能被污染。

正确做法是：

```text
contract_review.profile.clause_risk
只对 contract_review_agent 生效。
只在 selectedResource.kind == contract 时生效。
只在 review mode 生效。
有独立版本、hash、eval case。
```

## 八、添加新的 Agent 到底怎么设计

下面给一个可复用的设计模板。假设你已有 Runtime，现在要新增一个 `ContractReviewAgent`。

### 1. 先写 Agent 边界，不先写 Prompt

先回答 5 个问题：

```text
它负责什么？
它不负责什么？
它需要什么资源？
它能做哪些动作？
它输出什么结果？
```

例如：

```text
ContractReviewAgent 负责：
  - 阅读合同正文。
  - 对照模板和规则识别风险。
  - 输出风险清单和建议。

ContractReviewAgent 不负责：
  - 批准合同。
  - 修改合同原文。
  - 发起付款。
  - 代表法务做最终判断。
```

这一步决定了它是不是一个独立 Agent。

如果只是“同一个问答 Agent 换一种口吻回答合同问题”，那可能只需要一个 profile。  
如果它有独立资源、独立工具、独立证据要求、独立 final contract，那就应该是一个新 Agent。

### 2. 定义 AgentDefinition

```yaml
id: contract_review_agent
kind: reviewer
profile: contract_risk_review
defaultMode: review
resourceKinds:
  - contract
toolScopes:
  - contract.read
  - contract.compare
  - clause.search
maxIterations: 6
finalContract: ContractReviewReport
```

这个文件或配置解决“系统里有这个 Agent”。

### 3. 定义 Route Rule

推荐使用结构化信号：

```text
selectedResource.kind == contract
or attachment.detectedType == contract
or task.type == contract_review
```

关键词只做辅助：

```text
userIntent contains contract_review
```

不要让“用户提到了合同两个字”直接变成执行边界。

### 4. 定义 Resource Binding

```go
type ResourceBinding struct {
    Kind       string
    ID         string
    Version    string
    Permission PermissionSnapshot
}
```

绑定失败时，应该返回 blocked：

```text
无法确定要审核的合同，请先选择合同或上传合同文件。
```

不要让模型从历史消息里猜一个文件。

### 5. 定义 Tool Surface

```yaml
visibleTools:
  - read_contract
  - read_contract_template
  - compare_contract_versions
  - search_clause_library
  - create_review_comment

hiddenTools:
  - approve_contract
  - initiate_payment
  - modify_contract_source
```

这里最重要的是 hiddenTools。

不是在 Prompt 里说“你不能付款”，而是装配时根本不给付款工具。

### 6. 定义 Context Selector

```yaml
contextSources:
  - contract.body
  - contract.metadata
  - contract.template
  - contract.previous_versions
  - customer.summary
  - approval.requirements

exclude:
  - unrelated_chat_history
  - other_customer_contracts
  - private_notes_without_permission
```

Context Selector 要有明确排除项。否则 Runtime 很容易为了“更聪明”而塞入过多材料。

### 7. 定义 Loop Policy

```yaml
loop:
  kind: read_extract_check_report
  maxIterations: 6
  stopWhen:
    - contract_body_loaded
    - findings_completed
    - final_contract_satisfied
  retryWhen:
    - tool_timeout
    - missing_clause_location
  blockWhen:
    - no_contract_body
    - no_read_permission
```

Loop Policy 是让 Agent 像一个流程，而不是一次性生成文本。

### 8. 定义 Final Contract

```json
{
  "summary": "string",
  "risks": [
    {
      "level": "high|medium|low",
      "clause": "string",
      "location": "string",
      "reason": "string",
      "suggestion": "string",
      "evidenceRef": "string"
    }
  ],
  "missingInformation": ["string"],
  "recommendedNextActions": ["string"]
}
```

Final Contract 不只是给模型看的格式说明，还应该在代码里校验。

校验失败时，可以：

- 要求模型修复结构。
- 进入 verifier。
- 返回 blocked。
- 降级为人工复核。

### 9. 工程落地：从“只加 Prompt”改到 Code-driven Assembly

如果已有 Runtime 里新增 Agent 的方式还是“复制一个旧 Agent，然后换 system prompt”，可以按下面的路径改造。

改造前通常是这样：

```text
agent_name = contract_review_agent
system_prompt = "你是合同审核专家，请检查合同风险。"
tools = all_registered_tools
context = session_history + retrieved_docs
loop = default_react_loop
final = free_text
```

这个版本的问题很直接：

- 工具默认太宽，模型可能看到不该看的动作。
- 上下文默认太宽，历史消息和无关检索结果会污染判断。
- 没有资源绑定，模型可能从聊天历史里猜“这个合同”。
- 没有 final contract，输出格式靠模型自觉。
- 没有独立 eval case，优化后无法证明没有影响其他 Agent。

改造后应该变成：

```text
AgentDefinition:
  id = contract_review_agent
  kind = reviewer
  profile = contract_risk_review

RouteRule:
  selectedResource.kind == contract
  or attachment.detectedType == contract
  or task.type == contract_review

ResourceBinding:
  contract_id = validated_contract_id
  permission = read_only_snapshot

ToolSurface:
  visible = read_contract, compare_contract_versions, search_clause_library
  hidden = approve_contract, initiate_payment, modify_contract_source

ContextSelector:
  include = contract.body, contract.template, approval.requirements
  exclude = unrelated_chat_history, other_customer_contracts

LoopPolicy:
  read -> extract -> check -> report

FinalContract:
  ContractReviewReport

Eval:
  route_cases + tool_cases + context_cases + final_cases + leakage_cases
```

这才是新增 Agent 的最小工程闭环：Prompt 只是其中一个 profile，真正决定稳定性的，是 route、resource、tools、context、loop、final 和 eval 一起被装配。

## 九、常见反模式

这一节只做收束，不再重复前面的理论。

| 反模式 | 问题 |
| --- | --- |
| 新增 Agent 等于新增 Prompt 文件 | 只新增了 profile，没有新增运行时边界 |
| 在 Adapter 里偷偷拼业务规则 | 绕过 Runtime 的装配、权限和 trace |
| 工具默认全量暴露 | 把行动空间交给模型自由判断 |
| 上下文越多越好 | 增加污染和冲突，降低证据密度 |
| 用 Prompt 补权限漏洞 | Prompt 只能解释边界，不能执行边界 |
| 所有经验都写进全局 Prompt | 优化一个 Agent 时污染其他 Agent |
| 只测目标 Agent | 无法证明路由、profile、工具、上下文没有泄漏 |

## 结语：成熟 Agent Runtime 的分水岭

早期 Agent 系统的核心问题是“模型能不能答”。成熟 Agent Runtime 的核心问题是“系统能不能稳定地把正确的材料、正确的工具、正确的流程、正确的边界交给正确的 Agent”。

静态 Prompt 仍然重要，但它只解决岗位说明书问题。

Code-driven Assembly 解决的是运行时治理问题：

- 把自然语言请求落到结构化事实。
- 把 Agent 身份落到资源、工具、上下文和流程。
- 把 Prompt 从手写文本变成可追踪的编译产物。
- 把多 Agent 从“多个角色扮演”变成可隔离、可验证、可持续优化的工程系统。

所以，当一个已有 Runtime 的项目要新增 Agent 时，真正应该问的不是：

```text
这个 Agent 的 Prompt 怎么写？
```

而是：

```text
这个 Agent 的 Code-driven Assembly 怎么设计？
```

这才是 Agent Runtime 进入工程化阶段的分水岭。
