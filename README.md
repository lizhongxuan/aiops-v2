# AIOps Codex V2 — Eino Agent 后端

基于 [Eino Agent Framework](https://github.com/cloudwego/eino)（含 ADK）重写的 AIOps Codex AI Server 后端。

前端页面仍在 `aiops-codex/` 目录，本项目只包含后端 AI Server。前后端通过 HTTP/WebSocket/gRPC API 通信，所有端点路径、方法和 JSON 结构保持向后兼容。

## 架构概览

```
cmd/ai-server/main.go          # 启动入口，组装所有组件
internal/
├── runtimekernel/              # RuntimeKernel — 唯一的 turn 运行时内核
├── capability/                 # Capability Registry — 六类能力统一注册
├── promptcompiler/             # PromptCompiler — 四层结构化 Prompt 编译
├── policyengine/               # PolicyEngine — 显式策略硬约束
├── projection/                 # Projection — 生命周期事件 → 六类投影
├── modelrouter/                # ModelRouter — LLM Provider 路由与 Fallback
├── agentmgr/                   # AgentManager — Multi-Agent 编排（ADK）
├── spanstream/                 # SpanTree + MultiplexedStream 多路复用流
├── server/                     # HTTP/WebSocket/gRPC API 兼容层
├── store/                      # 数据持久化（内存 + JSON 异步写盘）
├── extensions/                 # Extension 挂载（Coroot/Lab/Generator）
│   ├── coroot/
│   ├── lab/
│   └── generator/
└── integration/                # 集成测试
```

## 快速开始

```bash
cd aiops-v2
go test ./...                   # 运行全部测试
go build ./cmd/ai-server        # 编译
AIOPS_DATA_DIR=.data ./ai-server # 启动（需配置 LLM Provider）
```

---

## ⚠️ 模块注册规则（必读）

本项目采用**注册制架构**。所有新增模块必须通过项目中定义的统一接口注册，禁止绕过注册机制直接嵌入核心逻辑。

如果遇到现有接口无法满足的特殊情况，**必须先与用户确认方案后才能添加代码**。

### 1. 新增 Tool（工具）

所有 tool 必须实现 `capability.ToolRuntime` 接口并通过 `capability.Registry` 注册。

```go
// 必须实现的接口（internal/capability/registry.go）
type ToolRuntime interface {
    Description() string
    CheckPermissions(ctx context.Context) error
    IsReadOnly() bool
    IsDestructive() bool
    IsConcurrencySafe() bool
    Display() ToolDisplayPayload
    InputSchema() json.RawMessage
    Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}
```

注册方式：

```go
registry.Register(capability.Entry{
    ID:          "my-tool/action",
    Name:        "my_tool.action",
    Kind:        capability.KindTool,  // 或 KindMCPTool
    Description: "工具描述",
    Tool:        &myToolImpl{},
    Visibility: capability.Visibility{
        SessionTypes: []string{"host", "workspace"},
        Modes:        []string{"inspect", "execute"},
    },
})
```

禁止事项：
- 禁止在 RuntimeKernel、PromptCompiler 或 PolicyEngine 中硬编码 tool 调用逻辑
- 禁止创建不经过 Registry 的平行 tool 池
- `KindTool` 类型的 Entry 必须提供非 nil 的 `ToolRuntime` 实现

### 2. 新增 Agent 类型

所有 Agent 类型必须通过 `AgentDefinition` 注册到 `AgentFactory`。

```go
// 注册新的 AgentKind（internal/agentmgr/definition.go）
factory.RegisterDefinition(&agentmgr.AgentDefinition{
    Kind:          agentmgr.AgentKindWorker,  // 使用已有 Kind 或新增
    Name:          "my-agent",
    PromptTemplate: "my_agent_v1",
    MaxIterations:  20,
    Model:          "gpt-4o-mini",
    CapabilityScope: agentmgr.CapabilityScope{
        Kinds: []capability.Kind{capability.KindTool, capability.KindMCPTool},
    },
})
```

禁止事项：
- 禁止绕过 `AgentFactory` 直接创建 Eino ADK Agent 实例
- 禁止在 `AgentManager` 外部管理 Agent 生命周期
- 新增 `AgentKind` 常量时必须同步更新 `IsValid()` 方法

### 3. 新增 LLM Model Provider

所有 LLM Provider 必须实现 Eino 的 `model.ChatModel` 接口并注册到 `ModelRouter`。

```go
// Provider 必须实现（github.com/cloudwego/eino/components/model）
type ChatModel interface {
    Generate(ctx context.Context, msgs []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, msgs []*schema.Message, opts ...Option) (*StreamReader[*schema.Message], error)
    BindTools(tools []*schema.ToolInfo) error
}
```

注册方式：

```go
// 在 cmd/ai-server/main.go 的 buildProviders() 中注册
providers["my_provider"] = myProviderChatModel

// 配置 Fallback
fallbacks := []modelrouter.FallbackEntry{
    {Primary: "my_provider", Fallback: "openai"},
}

// 按 AgentKind 配置
router.SetAgentKindConfig(modelrouter.AgentKindWorker, modelrouter.AgentKindConfig{
    Provider: "my_provider",
    Model:    "my-model-v1",
})
```

禁止事项：
- 禁止在 RuntimeKernel 中直接调用 LLM API，必须通过 `ModelRouter.GetModel()` 获取
- 禁止绕过 Fallback 机制

### 4. 新增 Capability 类型

当前支持六类 Capability Kind：`tool`、`skill`、`mcp_tool`、`ui_surface`、`mode_rule`、`workspace`。

如需新增 Kind：
1. 在 `capability.Kind` 常量中添加
2. 更新 `AllKinds()` 和 `IsValid()`
3. **必须与用户确认后才能添加**

### 5. 新增 PolicyEngine 策略

策略引擎包含四个维度，每个维度通过接口扩展：

| 维度 | 接口 | 文件 |
|------|------|------|
| Mode 能力边界 | `ModePolicy` | `policyengine/mode.go` |
| 用户权限 | `PermissionEvaluator` | `policyengine/types.go` |
| 证据收集 | `EvidenceEvaluator` | `policyengine/types.go` |
| 完成检查 | `CompletionEvaluator` | `policyengine/completion.go` |

新增 Mode 策略示例：

```go
// 实现 ModePolicy 接口
type MyModePolicy struct{}

func (p *MyModePolicy) CheckCapability(toolName string, toolKind CapabilityKind) PolicyDecision {
    // 定义该 mode 下的能力允许/禁止边界
}

// 注册到 Engine
engine.ModePolicy["my_mode"] = &MyModePolicy{}
```

禁止事项：
- 禁止在 Prompt 中用文字描述来替代 PolicyEngine 的硬约束
- 禁止跳过三层策略管道（CapabilityPolicy → PermissionPolicy → EvidencePolicy）
- 新增 Mode 时必须同步更新 PromptCompiler 的 RuntimePolicyPrompt 层

### 6. 新增 Extension

所有非核心能力（如 Coroot、Lab、Generator）必须通过 Extension 接口挂载。

```go
// 必须实现的接口（internal/capability/extension.go）
type Extension interface {
    Name() string
    Register(registry *Registry) error
    Unregister(registry *Registry) error
}
```

注册方式：

```go
extManager := capability.NewExtensionManager(registry)
extManager.Register(myExtension)
```

禁止事项：
- Extension 只能通过 `Capability Registry` 注册能力和通过 `Projection` 发射事件
- Extension 禁止反向主导 RuntimeKernel、PromptCompiler 或 PolicyEngine 的设计
- Extension 禁止直接访问 Store 或 Session 数据

### 7. 新增 Prompt 规则

所有 Prompt 规则必须归属到 PromptCompiler 的四层之一：

| 层 | 职责 | 文件 |
|----|------|------|
| System Prompt | 环境与角色 | `promptcompiler/system_rules.go` |
| Developer Instructions | 运行约束 | `promptcompiler/developer_rules.go` |
| Tool Prompt Set | 能力描述 | `promptcompiler/tool_registry.go` |
| Runtime Policy Prompt | 策略约束 | `promptcompiler/runtime_policy_prompt.go` |

禁止事项：
- 禁止在 loop nudge 或局部 helper 中注入 prompt 规则
- Tool Prompt 只能包含 capability、constraints、result shape、approval note，禁止包含 answer style
- 禁止散写 prompt，所有 prompt 必须通过 `PromptCompiler.Compile()` 输出

### 8. 新增 Projection 投影类型

当前支持六类投影：`toolInvocations`、`runtime.activity`、`cards`、`approvals`、`evidence`、`snapshot`。

新增投影类型需要：
1. 在 `projection/projector.go` 中添加对应的处理器
2. 定义新的 `EventType` 常量
3. 实现 `Subscriber` 接口的对应回调方法
4. **必须与用户确认后才能添加**

---

## 通用规则

1. **注册制优先**：所有模块通过统一接口注册，不允许平行能力池或硬编码旁路
2. **接口隔离**：各层通过接口通信，禁止跨层直接引用实现
3. **特殊情况必须确认**：如果现有接口无法满足需求，必须先与用户讨论方案，确认后才能修改接口或添加代码
4. **测试覆盖**：新增模块必须包含单元测试；涉及正确性属性的必须添加 PBT（`pgregory.net/rapid`）
5. **不修改 aiops-codex/**：所有代码变更限定在 `aiops-v2/` 目录内
6. **Business Logic Baseline**：任何影响四层语义分层、workspace 状态隔离或 tool lifecycle 真源的改动，必须先更新设计文档并标注"保留/替换/有意变更"

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
- 通过 `Entry.Visibility` 控制可见性
- 通过 `ToolRuntime.IsReadOnly()` / `IsDestructive()` 声明属性
- 通过 `ModePolicy.CheckCapability()` 定义边界
- 通过 `DenyRule` 模式匹配（规划中）过滤

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
go test ./internal/capability/...       # 单包测试
go test -run TestProperty ./...         # 只跑属性测试
go test -count=1 ./...                  # 清缓存跑
```
