# Agent Runtime 技术分享：从单 Agent 到多 Agent，从静态 Prompt 到 Code-driven Assembly

日期：2026-06-30  
范围：Agent runtime 基础架构、单 Agent 层级、Code-driven Assembly 实时 Prompt 组装、Multi-Agent 协作边界、静态 Prompt Profile 与代码层动态路由判断。  
阅读说明：前半部分不假设读者了解任何具体项目；后半部分才用 `aiops-v2` 的主机 agent / 管理 agent 作为案例。

## 一句话结论

Agent 不等于一段 Prompt。一个真正可用的 Agent 至少由模型、Prompt、上下文、工具、循环控制、权限策略、状态记录和最终输出约束组成。Prompt 决定“模型应该怎么想”，Runtime 决定“模型实际能做什么”。

成熟 Agent 的 Prompt 也不应该是一份固定大模板，而应该是由代码实时组装出来的运行时输入。这个过程可以叫 **Code-driven Assembly**：代码根据本轮任务、用户约束、资源绑定、可见工具、权限状态、历史压缩、证据缺口和风险等级，把正确的 prompt sections、tool schemas、context fragments 和 policy hints 组合起来。

有些 Agent 差异确实只是静态 Prompt Profile 的差异，例如同一套 runtime、同一套工具权限、同一类上下文，只是回答风格、分析重点、输出格式不同。  
但只要差异涉及工具权限、资源绑定、审批策略、并发调度、上下文隔离、证据合约、跨主机安全边界，就不能只靠换 Prompt。它必须落到代码里的动态判断和运行时状态机。

多 Agent 的价值也不是“多开几个模型同时聊天”，而是把复杂任务拆成多个有边界的运行时角色：谁负责规划、谁负责执行、谁能看哪些上下文、谁能调用哪些工具、谁必须等待审批、谁必须返回可验证证据。

## 1. 先从单 Agent 开始

很多人谈 Agent 时容易只看 Prompt：

```text
你是一个 SRE 专家，请一步步分析问题。
```

这只是 Agent 的一小部分。更完整地说，一个单 Agent 是一个围绕大模型构建的运行时系统：

```text
单 Agent = 模型 + Prompt + Context + Tools + Loop + Policy + State + Final Contract
```

它不是一次普通的 LLM 调用，而是一套“让模型能持续感知上下文、选择工具、执行动作、处理失败、生成最终结果”的控制循环。

### 1.1 单 Agent 的基本组成

| 层级 | 作用 | 如果缺失会怎样 |
| --- | --- | --- |
| 模型层 | 调用 LLM，获得推理、文本和 tool call | 只能写固定规则，无法泛化 |
| Prompt 层 | 定义角色、任务方法、输出格式和行为边界 | 模型不知道应该按什么方式工作 |
| Context 层 | 管理用户输入、历史、文件、证据、记忆、环境信息 | 模型看不到足够背景，或上下文爆炸 |
| Tool 层 | 提供搜索、文件、命令、数据库、浏览器、MCP 等能力 | Agent 只能聊天，不能观察和行动 |
| Loop 层 | 控制“模型思考 -> 调工具 -> 看结果 -> 再思考”的循环 | 只能单轮回答，不能迭代解决问题 |
| Policy 层 | 控制权限、审批、风险、资源边界和工具可见性 | 模型可能越权执行或误操作 |
| State 层 | 保存 session、turn、tool result、approval、trace、artifact | 无法恢复、审计、回放和继续任务 |
| Final Contract 层 | 检查最终答案是否满足证据、格式、计划和完成条件 | 容易草草总结、编造结果或遗漏验证 |

这八层合起来，才是一个可控的 Agent Runtime。

### 1.2 单 Agent 的一次请求是怎么跑的

一个单 Agent 处理用户请求，通常不是简单的“输入 -> 输出”，而是下面这个流程：

```text
用户输入
-> 解析任务、目标、约束和风险
-> 读取 session / 历史 / 当前环境
-> 选择本轮 profile 和工具面
-> 编译 prompt 和上下文
-> 调用模型
-> 如果模型要调用工具：校验权限并执行
-> 把工具结果写回状态
-> 再次调用模型或进入总结
-> final gate 检查证据、审批、验证和完成条件
-> 输出最终答案
```

这里有两个关键点。

第一，模型不是 runtime 本身。模型只是循环中的推理引擎。它会建议下一步、生成 tool call、阅读工具结果、组织答案，但它不应该独自决定权限和资源边界。

第二，Prompt 也不是 runtime 本身。Prompt 可以告诉模型“不要执行危险命令”，但真正能阻止危险命令的，是工具可见性、权限校验、审批状态机和执行器。

### 1.3 单 Agent 每一层分别解决什么问题

#### 模型层：负责推理，不负责兜底安全

模型层决定使用哪个模型、哪个 provider、什么上下文长度、是否支持 tool calling、是否支持结构化输出。它回答的是“用什么智能体来推理”。

但模型层不应该承担“这个命令能不能执行”“这个主机能不能访问”这类硬判断。因为模型输出是概率性的，不能作为权限系统。

#### Prompt 层：负责行为说明

Prompt 层告诉模型：

- 你是什么角色。
- 当前任务应该怎么拆。
- 回答应该是什么格式。
- 什么时候要先计划。
- 什么时候要说明不确定性。
- 工具调用前后应该如何解释。

Prompt 适合描述软策略，比如“先列证据，再列假设”。它不适合做硬权限，比如“禁止操作 host-b”。硬权限必须在代码里判断。

#### Context 层：负责让模型看见正确材料

Context 不是把所有东西都塞进 prompt。它要决定：

- 哪些历史消息还相关。
- 哪些工具结果需要保留全文。
- 哪些大输出应该落盘，只给摘要和引用。
- 哪些环境信息每轮都要注入。
- 哪些记忆可能过期，需要重新验证。

一个差的 context 层会导致两种问题：信息不够时模型乱猜；信息太多时模型被噪声淹没。

#### Tool 层：负责让 Agent 能观察和行动

没有工具的 Agent 只是聊天机器人。有工具后，Agent 才能：

- 搜索资料。
- 读取文件。
- 执行命令。
- 查询监控。
- 操作浏览器。
- 调用 API。
- 读写数据库。
- 调用 MCP server。

但工具越强，风险越高。所以工具层必须和 Policy 层绑定，不能只把所有工具 schema 一次性暴露给模型。

#### Loop 层：负责迭代

单次 LLM 调用只能生成一个回答。Agent loop 让模型可以：

1. 先判断缺什么证据。
2. 调工具采集证据。
3. 读取工具结果。
4. 修正假设。
5. 必要时继续调工具。
6. 最后综合答案。

这就是 Agent 和普通 Chat Completion 的核心差异之一：Agent 是一个可持续推进任务的循环，而不是一次文本生成。

#### Policy 层：负责硬边界

Policy 层回答：

- 这个工具本轮可见吗？
- 这个工具调用是否只读？
- 是否需要用户审批？
- 是否越过当前资源范围？
- 是否违反用户明确禁令？
- 是否需要先完成计划或前置检查？

Policy 层越成熟，Prompt 就越轻。因为不需要反复祈祷模型“千万别越权”，runtime 会直接让越权动作不可执行。

#### State 层：负责可恢复和可审计

Agent 运行过程中会产生很多状态：

- 当前 session。
- 当前 turn。
- 模型输入和输出。
- tool call。
- tool result。
- pending approval。
- evidence refs。
- 外部 artifact。
- trace 和审计事件。

如果没有 State 层，Agent 一旦中断、审批、失败、重试或上下文压缩，就很难恢复。用户也无法知道它到底做了什么。

#### Final Contract 层：负责防止草率结束

复杂任务最怕模型提前总结。Final Contract 要检查：

- 是否回答了用户问题。
- 是否引用了证据。
- 是否说明了缺失证据。
- 是否完成了计划。
- 是否验证了执行结果。
- 是否把工具失败误当成系统状态。

这一步让 Agent 不只是“能说”，而是“交付可相信的结果”。

### 1.4 单 Agent 内部也可以有不同静态 Profile

理解了单 Agent 的层级后，就能看清“静态 Prompt Profile”的位置。

同一个单 Agent runtime，可以有多个 profile：

```text
advisor profile:
  偏解释、建议、答疑。

evidence_rca profile:
  偏证据整理、时间线、假设和缺口。

writer profile:
  偏文章、报告、总结。

reviewer profile:
  偏审查、风险、遗漏和改进建议。
```

这些 profile 主要改变 Prompt 层和 Final Contract 层，不一定改变工具权限、资源绑定和执行方式。所以它们可以只是静态 Prompt 或配置。

但如果一个 profile 说“我是主机执行 agent”，那就不只是静态 Prompt 了。它还必须改变工具层、Policy 层、State 层和资源绑定。

## 2. 为什么需要 Code-driven Assembly 实时组装 Prompt

理解单 Agent 的层级之后，还要理解一个更关键的问题：这些层不是静态拼接的。Agent 每一轮请求都处在不同状态里，Prompt 必须由代码实时组装。

**Code-driven Assembly** 指的是：不是提前写一份巨大的固定 Prompt 给所有请求用，而是由 runtime 代码根据当前 turn 的结构化状态，动态决定哪些内容进入模型输入、哪些工具对模型可见、哪些规则必须出现、哪些历史需要压缩或外部化。

它解决的是“Prompt 与真实运行时状态一致”的问题。

### 2.1 固定大 Prompt 为什么不够

最简单的做法是写一个很长的 system prompt：

```text
你是一个通用 Agent。
你可以回答问题、写文章、查资料、执行命令、分析日志、管理主机、处理审批、多 Agent 协作……
任何危险操作前都要小心。
如果用户给了证据就分析证据。
如果用户选择了主机就只操作该主机。
如果多个主机就先计划。
……
```

这种做法很快会失败。

第一，固定大 Prompt 会把所有场景的规则都塞给模型。普通咨询问题也会带着主机执行、多 Agent、审批、证据、工具、工作流规则，模型注意力被无关规则稀释。

第二，固定大 Prompt 无法准确反映本轮真实状态。它可能写着“如果有主机就绑定主机”，但本轮到底有没有合法主机、HostID 是什么、是否通过 inventory 校验、工具是否可见，都不是 Prompt 自己知道的。

第三，固定大 Prompt 无法表达动态权限。用户刚刚拒绝了审批、某个工具本轮被隐藏、某个 MCP server 不健康、上下文进入 small context mode，这些都必须实时注入。

第四，固定大 Prompt 会制造冲突。比如一个全局规则说“必要时使用命令采集证据”，另一个用户约束说“不要执行命令，只分析我贴的输出”。如果没有代码层路由和 metadata，模型只能在文字冲突中猜。

所以固定 Prompt 适合表达稳定原则，不适合表达实时状态。

### 2.2 Prompt 组装不是字符串拼接，而是运行时编译

Code-driven Assembly 更像一次“prompt 编译”：

```text
TurnRequest
-> 解析用户输入、证据、资源、禁令、风险
-> 读取 session state、history、pending approvals、tool discovery
-> 计算 route / profile / mode / resource binding
-> 选择 prompt sections
-> 选择 tool schemas
-> 选择 context fragments
-> 注入 runtime policy
-> 生成最终 model input
```

这不是简单的字符串拼接，而是把 runtime 状态编译成模型可理解的输入。

一个好的 Prompt Assembly 至少要处理这些输入：

| 输入来源 | 进入 Prompt 前需要代码判断什么 |
| --- | --- |
| 用户输入 | 是咨询、诊断、执行、写作、研究还是多资源编排 |
| 用户约束 | 是否禁止联网、禁止执行命令、只允许基于已有证据 |
| 资源选择 | 是否选中了主机、服务、集群、数据库或文件 |
| 资源校验 | 资源是否存在，是否可信，是否和环境事实冲突 |
| 工具状态 | 哪些工具本轮可见，哪些 deferred，哪些被 policy 隐藏 |
| 审批状态 | 是否有 pending approval、rejected approval、approval grant |
| 历史上下文 | 哪些历史仍相关，哪些应该压缩，哪些工具结果要落盘 |
| 证据状态 | 当前是否有足够证据，缺什么证据，证据能否引用 |
| 风险等级 | 本轮是否只读，是否可能变更，是否需要 plan / approval |
| 输出合约 | 本轮应该输出建议、RCA、计划、报告还是最终总结 |

这些信息都不是模型凭空知道的。它们来自代码里的解析器、session store、tool registry、policy engine、approval store、resource inventory、context pipeline 和 trace。

### 2.3 Code-driven Assembly 要保证 Prompt 和工具面一致

Agent 最危险的错位，是 Prompt 说一套，工具面做另一套。

例如：

```text
Prompt: 当前只是 advisory，不要执行命令。
Tools: exec_command 仍然暴露给模型。
```

这时模型虽然被提醒不要执行，但工具 schema 仍然诱导它调用命令。

反过来也会出问题：

```text
Prompt: 你可以检查当前主机。
Tools: 没有任何主机检查工具。
```

这时模型会承诺自己做不到的事情，或者编造检查结果。

Code-driven Assembly 的核心目标之一，就是让 Prompt、Tool Surface、Runtime Policy 三者一致：

```text
profile = advisor
-> prompt 说明只做咨询
-> exec tools 不可见
-> final contract 要说明缺失证据

profile = host_worker + bound_host_id=hostA
-> prompt 说明只操作 hostA
-> host-scoped tools 可见
-> dispatch 校验 requested host == hostA

profile = host_manager
-> prompt 说明负责计划和调度
-> spawn/wait child tools 可见
-> direct host command 不可见或被拒绝
```

这就是为什么 Agent 需要深度代码逻辑：它不是为了“把 prompt 写得复杂”，而是为了让模型看到的世界和 runtime 允许执行的世界一致。

### 2.4 实时组装 Prompt 是为了减少噪声，而不是增加噪声

很多人担心动态组装会让系统变复杂。实际上，它的目标是让模型输入更小、更准。

如果没有 Code-driven Assembly，常见做法是把所有规则都塞进全局 prompt：

- 所有工具使用规则。
- 所有审批规则。
- 所有工作流规则。
- 所有领域知识。
- 所有多 Agent 规则。
- 所有主机安全规则。
- 所有输出格式。

这会让每个请求都背着一大包无关规则。

Code-driven Assembly 的做法相反：

- 没有主机绑定，就不注入主机执行协议。
- 没有多主机任务，就不注入 manager / child agent 协议。
- 没有用户证据，就不注入 evidence-only RCA 约束。
- 没有 public web intent，就不暴露 public web 工具。
- 没有 pending approval，就不注入审批恢复上下文。
- 上下文太大，就只注入摘要和 artifact 引用。

好的动态组装不是让 Prompt 更长，而是让每轮 Prompt 只包含“此刻必要的规则和材料”。

### 2.5 Code-driven Assembly 也是安全机制

Prompt injection 的本质是用户输入试图影响系统规则。如果系统只靠一段静态 Prompt，就很难判断哪些文字是用户任务，哪些是运行时约束。

Code-driven Assembly 可以把输入拆成不同信任级别：

- system / developer rules：高信任，稳定原则。
- runtime policy：高信任，代码生成的当前状态。
- tool schema：高信任，当前可调用能力。
- resource binding：高信任，后端校验后的目标。
- user input：低信任，任务内容和约束。
- tool result：中信任，需要来源、时间和脱敏状态。
- memory / history：中信任，可能过期，需要预算和校验。

这种分层让模型知道：用户可以提出任务和约束，但不能伪造 runtime 权限、工具可见性、资源绑定和审批状态。

### 2.6 一个通用的 Code-driven Assembly 伪代码

可以把动态 Prompt 组装理解成下面的伪代码：

```pseudo
function buildModelInput(turnRequest, sessionState):
    intent = parseIntent(turnRequest.input)
    evidence = extractEvidence(turnRequest.input)
    constraints = extractUserConstraints(turnRequest.input)
    resources = resolveAndValidateResources(turnRequest.metadata, turnRequest.input)

    route = decideRoute(intent, evidence, constraints, resources)
    profile = selectProfile(route)
    mode = selectMode(route)

    tools = assembleTools(profile, mode, route, sessionState.activeSkills)
    tools = applyToolSurfacePolicy(tools, route, constraints, sessionState.approvals)

    context = buildContextWindow(sessionState.history, route, resources)
    context = compactOrExternalizeLargeResults(context)

    sections = []
    sections.add(baseSystemRules())
    sections.add(profileFragment(profile))
    sections.add(runtimeState(route, mode, resources, approvals))
    sections.add(toolIndex(tools.modelVisible))
    sections.add(relevantContext(context))

    if route.requiresEvidence:
        sections.add(evidenceContract(evidence))

    if route.requiresApprovalRecovery:
        sections.add(approvalRecoveryPolicy(sessionState.approvals))

    return ModelInput(sections, tools.modelVisible)
```

这段伪代码说明了一个关键事实：Prompt 是最终产物，不是源头。源头是结构化 runtime state。

### 2.7 Code-driven Assembly 的判断标准

判断一个 Agent 是否真正用了 Code-driven Assembly，可以看这些问题：

1. 本轮可见工具是否由 profile / mode / risk / resource binding 共同决定？
2. Prompt 里的资源 ID 是否来自后端校验，而不是用户随手输入的文本？
3. 用户禁止执行命令时，命令工具是否真的隐藏或被拒绝？
4. 多资源任务是否自动切换到 manager / plan / child agent 协议？
5. pending approval / rejected approval 是否进入下一轮模型输入？
6. 大工具结果是否自动外部化，只把摘要和引用注入模型？
7. 历史上下文是否按当前任务压缩，而不是全部重复注入？
8. 最终回答约束是否根据 route 和 evidence state 动态变化？

如果答案都是“否”，那这个系统大概率只是静态 Prompt 包装的聊天机器人，还不是成熟 Agent Runtime。

## 3. 单 Agent 的能力边界

单 Agent 能解决很多问题：

- 回答知识型问题。
- 读取材料并总结。
- 使用少量工具查证事实。
- 按计划完成一个明确任务。
- 在一个资源边界内做诊断或执行。
- 给出有证据的最终结论。

但单 Agent 一旦承担过多职责，就会变得脆弱。

### 3.1 单 Agent 最容易混淆三种角色

第一种是 advisor：负责解释和建议。

第二种是 operator：负责执行和验证。

第三种是 coordinator：负责拆分任务、调度多个执行者、综合结果。

如果把三种角色都压在一个模型身上，它就会在“分析”“执行”“协调”之间频繁切换。上下文越长，工具越多，资源越多，越容易出错。

### 3.2 单 Agent 最容易遇到四个瓶颈

第一是上下文瓶颈。多个日志、多个主机、多个工具结果都塞给一个模型，最终会让模型注意力分散。

第二是权限瓶颈。同一个 agent 既能计划又能执行，容易在范围不清时直接操作真实资源。

第三是并行瓶颈。多个独立探测本来可以并行，单 Agent 却只能串行推进。

第四是证据归属瓶颈。多个资源的输出混在一起，最终很难说清楚哪个结论来自哪个资源。

这些瓶颈，就是多 Agent 出现的原因。

## 4. 为什么需要多 Agent

### 4.1 单 Agent 会把不同职责混在一起

单 Agent 可以处理简单问题，比如“解释一下 checkpoint 参数”或“基于这段日志判断可能原因”。但当任务变成真实运维场景，单 Agent 很快会遇到职责冲突：

- 它要规划，又要执行。
- 它要同时看多台主机，又要避免跨主机误操作。
- 它要读取很多日志，又要保持上下文不爆。
- 它要探索多个假设，又要给用户稳定结论。
- 它要提出变更，又要执行变更，还要验证恢复。

这些职责如果都塞给一个模型，它很容易出现两类问题：

- 过度行动：还没确认范围就直接执行命令。
- 过度保守：只写分析，不敢推进计划、采集证据和验证。

多 Agent 的本质不是“并行调用模型”，而是把职责显式切开。

### 4.2 Manager Agent 负责计划和协调

Manager Agent 的核心职责是：

- 识别任务目标和资源范围。
- 生成计划并拆分子任务。
- 决定哪些子任务能并行。
- 给每个子 agent 下发自包含任务。
- 等待和检查子 agent 的 evidence report。
- 汇总多路证据，形成最终结论或下一步计划。

Manager 不应该直接跑主机命令。它的权力应该是“调度和判断”，而不是“绕过 worker 执行”。

在生产实现里，这类差异应该落成明确的 manager runtime profile：允许计划、拆分、派发、等待和综合；禁止直接执行资源侧命令或绕过 worker 做变更。

### 4.3 Worker / Host Agent 负责边界内执行

Host Agent 的职责不是“比 manager 更低级”，而是“更靠近资源边界”：

- 只操作绑定的主机。
- 优先执行只读探测。
- 高风险动作必须走审批。
- 收集证据并返回结构化报告。
- 遇到跨主机、跨资源、缺审批或范围不清时停止并上报。

在生产实现里，这类差异应该落成明确的 worker runtime profile：写入绑定资源 ID，允许边界内检查、调用受控工具、申请审批、收集证据和返回报告；禁止操作其他资源、读取 sibling agent 私有上下文、绕过专用工具或擅自修改 manager 计划。

这不是 Prompt 能可靠兜住的边界。跨主机操作这种风险必须由代码校验。

### 4.4 多 Agent 还能解决上下文和并行问题

在复杂 RCA 里，多个子任务天然可以并行：

- 主机 A：检查 CPU、内存、磁盘、systemd。
- 主机 B：检查同一服务的端口、日志、容器状态。
- Coroot / Observability：检查服务拓扑、延迟、错误率、网络。
- Ops Manual：查相关 runbook 或操作手册。

如果只有一个 agent，它需要把所有原始输出都塞进同一个上下文。多 Agent 可以让每个 worker 只保留自己范围内的原始证据，manager 只接收摘要、证据引用和结论。这能显著降低 token 压力，也能降低跨资源信息污染。

## 5. 什么时候只是换静态 Prompt 就够了

静态 Prompt Profile 适合处理“行为风格和认知策略”的差异。它不适合承载硬权限。

### 5.1 典型适用场景

下面这些变化通常可以只用 Prompt Profile：

- 同一个 Agent 在“简洁回答”和“详细分析”之间切换。
- 同一个工具面下，要求输出为 RCA、日报、复盘、学习笔记或技术报告。
- 同一套只读上下文中，切换成 advisor、teacher、reviewer、summarizer。
- 同一权限边界内，调整 reasoning effort、planning policy、answer style。
- 同一资源范围内，要求先列假设，再列证据，再列缺口。

例如：

```text
Profile: advisor
- 回答咨询问题。
- 不运行主机命令。
- 说明缺少哪些证据。

Profile: evidence_rca
- 从用户贴出的日志和命令输出建立时间线。
- 区分事实、假设、缺失证据。
- 不主动执行命令。
```

这类差异是“模型该怎么组织思考和表达”。如果模型犯错，通常后果是回答质量下降，而不是越权操作真实资源。

### 5.2 静态 Prompt 的边界

Prompt 不能可靠解决这些问题：

- “这次能不能调用 `exec_command`？”
- “这个 agent 能不能操作 host-b？”
- “用户说不要执行命令时，工具是否真的应该隐藏？”
- “多台主机是否需要 manager/child agent 协调？”
- “高风险命令是否必须暂停等待审批？”
- “工具结果是否要落盘、脱敏、只传摘要？”
- “审批被拒后是否禁止再次请求同类操作？”

原因很直接：Prompt 是软约束，模型可能忽略、误读或在复杂上下文中遗忘。权限、资源绑定、审批和审计必须是硬逻辑。

## 6. 什么时候必须做代码层动态判断

只要系统需要根据当前输入、上下文、资源状态和风险动态改变行为，就应该进入代码层 runtime 判断。

### 6.1 输入不同，路由不同

同样一句“帮我分析 PostgreSQL 异常”，不同输入上下文应该走不同 runtime：

| 输入形态 | 合理路由 | 原因 |
| --- | --- | --- |
| “pg_auto_failover timeline 为什么会比主库高？” | `chat_advisory` | 无主机绑定，适合解释和公开知识 |
| “不要执行命令，只基于下面输出分析...” | `evidence_rca` | 用户给了证据并禁止执行 |
| “@server-local 检查 PG 状态” | `host_bound_ops` | 单主机明确绑定，可以暴露主机只读工具 |
| “@hostA @hostB 对比 PG 状态” | `multi_host_ops` | 多主机任务，需要 manager 计划和 child agents |
| “@hostA @Coroot 分析 checkout 服务，但拓扑显示服务在 hostB” | `evidence_rca` 或要求澄清 | 目标冲突，不能盲目执行 |

这类判断不能写死在一段 system prompt 里。它必须由 route builder 读取结构化 mention、用户证据、环境事实和禁令后决定。

这类判断应该由 route builder 在代码里完成：默认咨询走 advisory；用户给了证据走 evidence RCA；单个明确资源走 resource-bound worker；多个明确资源走 manager / multi-agent；用户禁止执行时关闭执行工具；环境目标冲突时降级为只读分析或要求澄清。

### 6.2 路由结果要改变 session、mode、host binding

动态判断不能只停留在 metadata 上。它必须改变真实 runtime 请求。

路由结果应该落到真实 runtime request，而不是只写进 prompt：

- 单资源执行：设置 resource ID、resource-bound session、目标绑定 metadata。
- 多资源编排：清空单一 resource ID，进入 workspace / plan / manager mode。
- 默认咨询：不绑定资源，进入普通 chat / advisory mode。

这一步非常关键。否则模型 prompt 里写了“你是 host agent”，但 runtime 仍然没有绑定 host，工具执行时就会出现悬空目标或误用本机。

### 6.3 工具面必须由代码过滤

如果用户只是咨询，不应该把主机执行工具暴露给模型；如果用户明确 `@host`，才应该暴露受控的主机工具。

成熟实现通常有两层工具面控制：

1. 工具装配层：根据 turn metadata 设置 profile、enabled packs、enabled tools，并对工具 metadata 做 transform/filter。
2. 工具可见层：根据 mode、profile、agent role、skill policy、runtime decision 再过滤模型可见工具，并记录 hidden reason 和 surface decisions。

例如命令执行工具即使在 runtime 里注册，也不应该在 advisor / evidence RCA profile 中模型可见。它可以是 runtime-known tool，但不是 model-visible tool。

这就是“代码硬边界”：模型看不到不该看的工具 schema，自然就不会以为自己可以调用。

### 6.4 命令执行必须校验 HostID 和 AgentKind

Prompt 可以告诉 manager “不要直接执行主机命令”，但真正的安全边界要靠 `HostCommandTool` 和 policy。

生产实现里的 host command tool 至少要做两个关键校验：

- 当前调用者必须是 host-bound worker / child agent，manager 不能直接调用。
- requested host 必须等于 bound host，不匹配就拒绝。

这意味着即使某个 manager prompt 被污染、模型误调用了 host command，代码仍然会拒绝。这才是生产系统需要的安全模型。

## 7. 一个具体案例：主机 Agent 和管理 Agent 到底有什么区别

下面用 `aiops-v2` 做案例。读者不需要提前了解这个项目，只需要知道：它是一个面向运维场景的 Agent 系统，用户可以在聊天里咨询问题、贴证据、选择主机、触发只读检查或多主机诊断。这个案例的价值在于，它刚好能说明“有些差异只是 Prompt，有些差异必须是 runtime 代码”。

在这个案例里，`BuildChatRuntimeRoute()` 负责把输入动态路由到 `chat_advisory`、`evidence_rca`、`host_bound_ops` 或 `multi_host_ops`；`applyChatRuntimeRouteHostBinding()` 负责把路由落到 `TurnRequest.HostID`、`SessionType`、`Mode` 和 target metadata；`ApplyToolSurfacePolicy()` 负责过滤模型可见工具；`HostCommandTool.Run()` 和 `EnforceHostBinding()` 负责最终执行边界。

### 7.1 管理 Agent：workspace 级编排者

管理 Agent 对应 `host_manager` / `manager_agent_full_runtime`。

它的运行边界是：

- session type：workspace。
- mode：plan / execute。
- profile：`host_manager`。
- runtime profile：`manager_agent_full_runtime`。
- 工具包：`hostops`。
- 典型工具：`spawn_host_agent`、`send_host_agent_message`、`wait_host_agents`、`stop_host_agent`。
- 禁止：直接执行主机命令、直接修改主机。

它做的是“计划和调度”：

```text
用户：@hostA @hostB 对比 PG 状态

Runtime:
1. 检测到多个 host mentions。
2. 路由为 multi_host_ops。
3. 进入 workspace + plan mode。
4. 启用 hostops tool pack。
5. 使用 host_manager profile。

Manager:
1. 生成多主机计划。
2. 为 hostA/hostB 分配子任务。
3. 调用 spawn_host_agent。
4. 等待每个 host child report。
5. 综合多主机证据。
```

这里如果只换 Prompt，让同一个 agent “假装自己是 manager”，会有明显问题：它仍然可能看到或调用直接主机执行工具，仍然可能把 hostA 和 hostB 的上下文混在一起，也没有强制的 child agent 生命周期和 evidence report。

### 7.2 主机 Agent：host-bound 执行者

主机 Agent 对应 `host_worker` / `host_agent_full_runtime`。

它的运行边界是：

- session type：host。
- mode：execute / inspect。
- profile：`host_worker`。
- runtime profile：`host_agent_full_runtime`。
- bound host：明确的 `HostID`。
- 可见工具：主机范围内的只读 inspection / host command。
- 禁止：操作其他 host、读取其他 child agent 私有上下文、绕过 host command tool。

它做的是“边界内执行”：

```text
Manager 分配任务：
hostA: 检查 PostgreSQL 进程、端口、磁盘、日志摘要。

Host child:
1. 只在 hostA 范围内工作。
2. 使用 HostCommandTool 执行只读命令。
3. 高风险动作请求审批。
4. 返回 HostTaskReport。
5. 报告 evidence refs、错误、blocker、下一步。
```

`CreateHostChildAgent()` 会复用完整 Host Agent runtime，并额外注入 manager 分配的 host task prompt asset，包括 `bound_host_id`、`plan_step_id`、risk、goal、constraints、evidence requirements 和 report contract。

这说明 host child 不是“另一段静态 prompt 文案”，而是带着独立 session、HostID、任务上下文和工具面运行的 worker。

### 7.3 主机侧 host-agent daemon 与 LLM host worker 不是一回事

还要避免一个常见混淆：

- LLM host worker / host child：模型 runtime 里的主机绑定执行 agent。
- 主机侧 host-agent daemon：部署在目标主机或远端执行侧的 runner，用来注册、心跳和执行命令。

当 `exec_command` 在远程主机上执行时，runtime 会通过 `HostAgentCommandRunner.RunHostAgentCommand()` 发送到主机侧 host-agent，或者在有 SSH 凭证时走只读 SSH fallback。LLM host worker 是“决策者和工具调用者”，主机侧 host-agent daemon 是“命令执行通道”。

两者都叫 host agent 会让架构讨论混乱。更清晰的命名方式是：

- `host_worker`：LLM runtime profile。
- `host_child`：manager 派发出来的 host-bound child agent。
- `host-agent daemon`：主机侧 runner/transport。

## 8. 静态 Prompt Profile 与动态 Runtime 的分工

可以用一句话判断：

```text
如果差异只影响“模型怎么说、怎么组织思考”，用 Prompt Profile。
如果差异影响“模型能做什么、对哪个资源做、怎么审批、怎么审计”，用 Runtime 代码。
```

### 8.1 Prompt Profile 应该负责什么

Prompt Profile 负责软策略：

- 角色说明：advisor、evidence RCA、host worker、host manager。
- 思考方式：先证据后假设，先计划后执行。
- 输出结构：结论、证据、风险、下一步。
- 风格：简洁、详细、教学、报告。
- 领域提醒：主机执行前确认 host scope，RCA 要分清事实和推断。

`aiops-v2` 的 `profile_fragments.go` 就是典型例子：

- `advisor`：回答咨询，不运行主机命令。
- `evidence_rca`：从用户证据和只读工具证据构建时间线。
- `host_worker`：只在绑定 host scope 内操作。
- `host_manager`：复杂主机任务先计划，委派子 agent，等待结果后综合。

这些 prompt fragment 有价值，但它们只是 runtime policy 的“自然语言投影”。

### 8.2 Runtime 代码应该负责什么

Runtime 负责硬策略：

- route selection：本轮到底是什么模式。
- host binding：是否绑定 HostID，绑定哪台。
- session/mode：workspace 还是 host，chat/inspect/plan/execute。
- tool surface：哪些工具模型可见，哪些工具隐藏。
- permission check：只读、变更、高风险、审批、拒绝。
- context isolation：worker 能看什么，不能看什么。
- scheduler：哪些 child agent 并行，哪些排队。
- evidence contract：工具结果如何变成可引用证据。
- audit trace：每一步为什么允许或拒绝。

这些不能靠“请模型遵守”来实现。

### 8.3 为什么只靠 Prompt 会失败

只靠 Prompt 会遇到四个失败模式：

1. 注意力失败：长上下文中，模型忘了某条规则。
2. 语义误读：模型把“可以分析 host”理解成“可以执行 host 命令”。
3. 工具诱导：只要工具 schema 可见，模型就倾向于调用它。
4. 安全失守：Prompt 被用户输入覆盖或诱导时，缺少硬拒绝。

所以生产级 Agent 的设计原则是：

```text
Prompt 负责引导正确行为；
Runtime 负责让错误行为不可执行。
```

## 9. 一个可复用的设计框架

设计一个新的 Agent Profile 时，不要先写 prompt。应该先填这张表：

| 维度 | 需要回答的问题 |
| --- | --- |
| 角色 | 它是 advisor、planner、manager、worker、verifier 还是 researcher？ |
| 资源边界 | 它绑定 workspace、host、service、cluster、database 还是 public web？ |
| 工具边界 | 它能看到哪些工具？哪些工具只 runtime 注册但模型不可见？ |
| 风险边界 | 它能做只读、低风险、中风险还是高风险动作？ |
| 审批边界 | 哪些动作需要 user approval 或 manager approval？ |
| 上下文边界 | 它能读取哪些历史、证据、artifact 和 sibling agent 输出？ |
| 输出合约 | 它返回自由文本、plan、HostTaskReport、EvidenceEnvelope 还是 final answer？ |
| 失败策略 | 缺证据、权限拒绝、工具失败、跨资源请求时怎么处理？ |

填完这张表后，再决定哪些部分是 prompt，哪些部分是 runtime。

### 9.1 如果是 Advisor Agent

可以偏 prompt：

- profile：`advisor`
- session：workspace
- mode：chat
- tools：默认不暴露 host exec
- 输出：解释、假设、缺失证据
- runtime：只要隐藏危险工具即可

### 9.2 如果是 Evidence RCA Agent

需要 prompt + runtime：

- profile：`evidence_rca`
- session：workspace
- mode：inspect
- tools：只读证据工具，可选 observability
- 输出：时间线、事实、假设、缺口
- runtime：必须尊重“不要执行命令”，不能因为用户贴了日志就默认绑定本机

### 9.3 如果是 Host Worker Agent

必须 runtime 强约束：

- profile：`host_worker`
- session：host
- mode：execute / inspect
- binding：必须有 HostID
- tools：host-scoped command / inspection
- 输出：HostTaskReport / evidence refs
- runtime：必须校验 HostID，跨 host 拒绝，高风险审批

### 9.4 如果是 Host Manager Agent

必须 runtime + orchestration：

- profile：`host_manager`
- session：workspace
- mode：plan / execute
- tools：spawn/send/wait/stop host agents
- 输出：计划、调度状态、综合结论
- runtime：不能直接暴露 host command；必须通过 child agent 执行

## 10. 从案例反推架构启示

### 10.1 不要把复杂能力都塞进全局 Prompt

如果一个规则只在 host manager 场景适用，就应该进入 host manager profile 或 hostops tool pack；如果只在 evidence RCA 适用，就进入 evidence RCA profile；如果只在某个 workflow skill 激活后适用，就由 skill activation 注入。

全局 developer prompt 越大，模型越容易在普通 chat 中被不相关规则干扰。

### 10.2 Route metadata 要成为唯一事实来源

`aiops-v2` 现在已经有不错的结构化 metadata：

- `aiops.route.mode`
- `aiops.route.requiresHostBinding`
- `aiops.route.allowsExecCommand`
- `aiops.target.binding`
- `aiops.target.hostId`
- `profile`
- `toolProfile`
- `runtimeProfile`

后续应该继续强化这个方向：前端文本、prompt 文案、UI 展示都只是投影，真正决定执行边界的是 route metadata 和 runtime state。

### 10.3 Host binding 要 fail closed

主机绑定不能靠 raw text 猜测。理想链路是：

```text
前端选择 @host
-> 隐藏 structured mention metadata
-> 后端 HostRepository 复核
-> route = host_bound_ops / multi_host_ops
-> TurnRequest.HostID / target.hostId
-> Selected Host Inventory 注入
-> tool surface 暴露 host-scoped tools
-> dispatch 时带 ToolExecutionContext.HostID
-> host-agent / SSH 执行通道
```

任何一步验证失败，都应该降级为 advisory / evidence RCA 或要求用户重新选择，而不是用 raw `@xxx` 文本继续猜。

### 10.4 多主机必须走 manager -> child agents

多主机任务不能让一个模型拿着多个 HostID 直接执行命令。正确模式是：

```text
Manager:
  计划、拆分、调度、等待、综合

Host child A:
  只在 hostA 内采集证据

Host child B:
  只在 hostB 内采集证据

Manager:
  汇总 hostA/hostB evidence report
```

这样才能避免跨主机误操作，也能让证据归属清晰。

### 10.5 Tool surface 比 prompt 更重要

如果不该执行命令，最好的方式不是在 prompt 里写十遍“不要执行命令”，而是让 `exec_command` 不出现在当前 model-visible tools 里。

Prompt 是提醒，tool surface 是行为边界。

## 11. 判断一个 Agent 设计是否成熟

可以用下面的问题自检：

1. 如果模型忽略 prompt，它还能不能越权？
2. 如果用户输入里出现 prompt injection，它能不能越过资源边界？
3. 如果工具 schema 可见，模型是否真的被允许调用？
4. 如果审批被拒，runtime 会不会阻止重复请求或绕路执行？
5. 如果有多个 host，证据能不能追溯到具体 host？
6. 如果 child agent 失败，manager 能不能知道失败原因和缺口？
7. 如果上下文很大，manager 是否只接收必要摘要和 evidence refs？
8. 如果最终答案缺证据，final gate 是否能阻止过度结论？

只要这些问题的答案依赖“模型应该会遵守 prompt”，这个设计就还不够成熟。

## 12. 结语

Agent Runtime 的演进方向不是把 Prompt 写得越来越长，而是把 Prompt 中反复出现的规则沉淀成运行时协议：

- “不要直接执行命令”沉淀成 tool surface 隐藏和 dispatcher 拒绝。
- “只操作当前主机”沉淀成 HostID binding 和 cross-host deny。
- “多主机先计划”沉淀成 manager profile、plan mode 和 hostops child agents。
- “需要审批”沉淀成 permission decision、pending approval 和 resume turn。
- “必须有证据”沉淀成 evidence refs、final evidence gate 和 trace。

Prompt 仍然重要，因为它告诉模型如何理解任务、如何组织行动、如何输出结果。但 Prompt 不应该独自承担安全、权限、调度和审计。

真正可靠的 Agent Runtime，是让模型在清晰的运行时轨道里发挥推理能力：该咨询时咨询，该诊断时诊断，该绑定主机时绑定主机，该派发子 agent 时派发子 agent，该等待审批时等待审批。  
多 Agent 的价值，也正在这里：不是让系统看起来更复杂，而是让复杂任务的职责、边界和证据变得可控。

## 附：aiops-v2 关键代码索引

| 主题 | 文件 |
| --- | --- |
| Chat 路由模式与 host binding | `internal/appui/chat_runtime_route.go` |
| route 到 tool/profile metadata | `internal/appui/chat_tool_surface.go` |
| chat 前门组装 TurnRequest | `internal/appui/chat_service.go` |
| runtime profile 定义 | `internal/runtimekernel/agent_runtime_profile.go` |
| prompt profile fragment | `internal/promptcompiler/profile_fragments.go` |
| runtime state prompt | `internal/promptcompiler/runtime_policy_prompt.go` |
| prompt compile context enrichment | `internal/runtimekernel/runtime_kernel.go` |
| tool metadata/profile filter | `internal/tooling/turn_metadata_filter.go` |
| tool surface policy | `internal/tooling/surface_policy.go` |
| tool registry profile visibility | `internal/tooling/registry.go` |
| exec command 远程 host-agent 执行 | `internal/integrations/localtools/register.go` |
| hostops manager tools | `internal/hostops/tools.go` |
| hostops orchestrator | `internal/hostops/orchestrator.go` |
| agent factory / host child prompt asset | `internal/agentmgr/factory.go` |
| agent manager kernel adapter | `internal/agentmgr/kernel_adapter.go` |
| host command hard boundary | `internal/hostops/host_command_tool.go` |
| host binding policy | `internal/hostops/policy.go` |
