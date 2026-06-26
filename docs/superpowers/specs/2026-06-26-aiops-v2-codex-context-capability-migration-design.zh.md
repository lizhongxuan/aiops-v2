# aiops-v2 借鉴 Codex 上下文能力的设计方案

日期：2026-06-26
状态：设计方案
范围：`aiops-v2` 上下文编译、模型输入、Trace、缓存键、压缩与旧结构清理

## 1. 结论

`aiops-v2` 不应该直接移植 `codex/` 的 Prompt 文本或 Rust 实现，而应该借鉴 Codex 在上下文运行时里的几个稳定能力：

1. Provider 请求结构与内部 Prompt 编译结果分离。
2. 用结构化 `ResponseItem`/`ContentItem` 表达模型可见输入，而不是只用扁平 `role/content`。
3. 明确记录 `prompt_cache_key`、请求属性快照和客户端元数据，让缓存命中、请求复用、Trace 解释有同一套依据。
4. 把重型原始请求/响应/工具结果从主 Trace 图里拆出去，只保留引用和摘要。
5. 把上下文压缩、线程/窗口/父子关系作为运行时一等概念，而不是散落在若干 Trace 字段里。

迁移目标不是再造一套 Codex Runtime，而是把 `aiops-v2` 现有 `promptcompiler`、`promptinput`、`modeltrace`、`runtimekernel` 的职责边界收紧。最终系统中每类上下文数据只允许一个写入者、一个规范结构、若干只读投影。

## 2. 当前问题

`aiops-v2` 已经有较完整的上下文治理能力，包括 Stable/Dynamic 分层、PromptSection、ContextDedupe、ContextGovernance、PromptInputTrace 和工具面追踪。但现在的问题是结构过度展开：

1. `internal/modeltrace/trace.go` 的 Trace payload 同时承载 provider request、prompt trace、governance、工具面、任务状态、诊断信息和兼容字段。
2. `internal/promptinput/types.go` 的 `Message` 仍是扁平 `Role/Content/ToolCalls/ToolResult`，会丢失模型输入项的类型、ID、phase、internal metadata、reasoning item、multimodal content 等信息。
3. `internal/promptcompiler/types.go` 已有 `PromptEnvelope` 与 `CompiledPrompt`，但 `CompiledPrompt` 又保留 Stable/Dynamic/System/Developer/Tools/Policy 兼容视图，长期看会形成两条 Prompt 逻辑。
4. `promptFingerprint` 与 section cache 可以解释“编译产物是否变化”，但还不能完整解释“provider 看到的请求属性是否可复用、是否有利于 prompt cache 命中”。
5. Trace 中很多统计字段是从同一批数据派生出来的快照，例如 `visibleToolCount`、`promptCharCount`、`modelInputStats`、`toolRegistryCharCount`。这些字段如果由多个路径写入，会造成调试时的版本漂移。

因此，迁移要解决的核心问题不是“preview 或 content 怎么裁剪”，而是建立一条规范上下文链路：上下文来源 -> 编译计划 -> 模型输入项 -> provider 请求快照 -> Trace 摘要/原始 payload 引用。

## 3. Codex 可借鉴能力

| Codex 能力 | 借鉴价值 | aiops-v2 落点 |
| --- | --- | --- |
| `Prompt` 与 `ResponsesApiRequest` 分离 | 内部 Prompt 编译和 provider 请求协议解耦 | 新增 `ProviderRequestSnapshot`，由唯一 adapter 从编译结果生成 |
| `ResponseItem`/`ContentItem` | 保留消息、工具调用、reasoning、图片、压缩等结构语义 | 新增 `ModelInputItem`/`ModelContentItem`，再适配到 Eino `schema.Message` |
| `prompt_cache_key` | 缓存命中依据可解释，和线程/会话绑定 | 在请求快照中显式保存 `promptCacheKey` 与 `requestPropertiesHash` |
| websocket 请求属性比较 | 复用连接时只比较稳定请求属性，不把每轮 input 混进去 | 为 OpenAI/Eino 请求生成稳定属性 hash，用于 Trace 与调试 |
| `client_metadata` | session/thread/window/turn/parent/subagent/workspace 都有统一归属 | 新增 `ContextLineageMetadata`，供 request 与 trace 共用 |
| raw payload refs | 主 Trace 不被大块原始内容污染 | Trace v2 只保存 `rawPayloadRefs`，原始请求/响应/工具结果外置 |
| compaction item | 压缩是上下文替换事件，不是普通日志文本 | 将 compaction 作为 `ModelInputItem` 类型与 ContextSource 事件 |

不建议移植的部分：

1. 不直接移植 Codex Rust 类型名和 provider 特化实现。
2. 不引入第二套 Prompt compiler。
3. 不把 Codex 的 Lite mode、Azure store 细节硬编码进 AIOps 主链路。
4. 不绕过现有 AIOps 的上下文治理、工具治理和诊断 Trace。

## 4. 目标架构

### 4.1 单一上下文链路

```text
ContextSourceRegistry
  -> ContextCompilePlan
  -> promptcompiler.PromptEnvelope
  -> promptinput.ModelInputItem[]
  -> modelrouter.ProviderRequestSnapshot
  -> modeltrace.TraceDocumentV2
```

每一层只负责自己的输出：

| 层 | 责任 | 禁止事项 |
| --- | --- | --- |
| `ContextSourceRegistry` | 注册系统、开发者、用户、工具、环境、压缩、诊断等上下文来源 | 不拼 provider 消息 |
| `ContextCompilePlan` | 决定上下文来源的顺序、预算、缓存策略和保留策略 | 不写 Trace payload |
| `PromptEnvelope` | 表达稳定层、动态层、section、fingerprint | 不保留长期兼容字段作为第二事实源 |
| `ModelInputItem[]` | 表达模型可见输入的规范结构 | 不直接调用 provider |
| `ProviderRequestSnapshot` | 表达 provider 最终看到的请求属性 | 不重新编译 Prompt |
| `TraceDocumentV2` | 保存摘要、索引、治理信息和 raw payload 引用 | 不复制所有大字段全文 |

### 4.2 新增核心结构

#### `ProviderRequestSnapshot`

建议字段：

```go
type ProviderRequestSnapshot struct {
    Provider              string
    Model                 string
    Instructions          string
    Input                 []ModelInputItem
    Tools                 []ToolSpecSnapshot
    ToolChoice            any
    ParallelToolCalls     bool
    Reasoning             *ReasoningOptions
    Store                 bool
    Stream                bool
    Include               []string
    ServiceTier           string
    PromptCacheKey        string
    RequestPropertiesHash string
    Text                  *TextOptions
    ClientMetadata        ContextLineageMetadata
}
```

`RequestPropertiesHash` 只覆盖 provider 请求中除本轮 input 和非稳定 metadata 以外的稳定属性，例如 model、instructions、tools、tool choice、parallel tool calls、reasoning、include、service tier、text、prompt cache key。它用于解释请求复用和 prompt cache 的稳定前缀条件，不替代现有 `promptFingerprint`。

#### `ModelInputItem`

建议字段：

```go
type ModelInputItem struct {
    ID       string
    Type     string
    Role     string
    Content  []ModelContentItem
    Phase    string
    ToolCall *ToolCallItem
    ToolOut  *ToolOutputItem
    Reasoning *ReasoningItem
    Metadata map[string]any
}
```

`schema.Message` 只作为 provider adapter 的输出格式，不再作为 `aiops-v2` 内部上下文的规范结构。

#### `TraceDocumentV2`

建议结构：

```go
type TraceDocumentV2 struct {
    SchemaVersion     int
    Kind              string
    CreatedAt         time.Time
    TraceID           string
    SessionID         string
    TurnID            string
    Summary           TraceSummary
    ProviderRequest   ProviderRequestSnapshot
    PromptTrace       PromptTrace
    RuntimeGovernance RuntimeGovernanceTrace
    RawPayloadRefs    []RawPayloadRef
    Diagnostics       DiagnosticTrace
}
```

Trace v2 的主原则是：可搜索字段保留摘要，重型字段保留引用。UI 需要展示的 preview 应该从 `Summary` 或索引生成，完整 content 通过 raw payload ref 延迟读取。

## 5. preview 与 content 的定位

在新结构下，`preview` 不应该是业务事实源，也不应该参与缓存命中判断。它只服务三类场景：

1. 列表页、Trace 索引、调试摘要的快速展示。
2. 避免把超长 prompt、工具结果、网页内容、诊断日志塞进主 Trace。
3. 在不读取 raw payload 的情况下快速判断这条上下文项是什么。

`content` 才是可被编译、发送、校验或还原的完整内容。缓存命中率主要由稳定前缀、工具定义、instructions、provider 请求属性、`prompt_cache_key` 和消息顺序决定。`preview` 只能减少存储和 UI 读取成本，不能提高 provider prompt cache 命中率。

因此规则应明确：

| 字段 | 是否业务事实源 | 是否参与 prompt cache | 写入者 |
| --- | --- | --- | --- |
| `content` | 是 | 间接参与，取决于是否进入 provider input | `ModelInputItem` 生成链路 |
| `preview` | 否 | 否 | Trace/index 摘要器 |
| `contentRef` | 是，指向完整内容 | 间接参与，读取后还原 content | raw payload store |
| `stableHash`/`contentHash` | 是，用于一致性校验 | 可用于本地 dedupe/cache | 规范内容序列化器 |

这能避免“preview 是裁断版 content，所以是不是另一套上下文”的误解。preview 是展示投影，不能回流到 prompt 编译或 provider 请求。

## 6. 迁移方案

### Phase 0：冻结职责边界

1. 明确 `promptcompiler` 是唯一 Prompt 编译入口。
2. 明确 `promptinput` 是唯一模型输入项生成入口。
3. 明确 `modelrouter`/provider adapter 是唯一 provider request 生成入口。
4. 明确 `modeltrace` 只消费规范结构并生成 Trace，不反向拼装 Prompt。

验收点：代码搜索中不能存在新的 `buildPrompt...`、`compilePrompt...`、`modelInput...` 旁路实现。

### Phase 1：新增 provider 请求快照，不改变线上行为

在现有请求发送路径旁边生成 `ProviderRequestSnapshot`：

1. 从当前 model、instructions、messages、tools、reasoning、stream 等参数生成快照。
2. 计算 `PromptCacheKey`，默认绑定 session/thread/case 级稳定 ID。
3. 计算 `RequestPropertiesHash`。
4. 写入 Trace，但不改变真实请求发送。

验收点：同一 turn 的真实 provider 请求与 snapshot 可逐字段对齐。

### Phase 2：引入 `ModelInputItem`，收敛扁平 `Message`

新增 `ModelInputItem` 作为内部规范结构：

1. 用户消息、系统消息、开发者指令、工具调用、工具结果、reasoning、compaction 都转成 item。
2. `schema.Message` 只由 adapter 从 `ModelInputItem[]` 转换。
3. 旧 `promptinput.Message` 进入 deprecated 状态，只允许在 adapter 层存在。

验收点：新增上下文来源只能产出 `ModelInputItem`，不能直接追加 `schema.Message`。

### Phase 3：接入缓存键与稳定请求属性

补齐缓存可解释性：

1. `promptFingerprint` 继续表示编译产物 fingerprint。
2. `requestPropertiesHash` 表示 provider 请求稳定属性 hash。
3. `promptCacheKey` 表示 provider prompt cache 的逻辑分组 key。
4. Trace UI 同时展示三者，分别解释“编译是否变了”、“请求属性是否变了”、“缓存分组是否一致”。

验收点：一次上下文变化能明确落到 section fingerprint 变化、request property 变化或 input-only 变化。

### Phase 4：Trace v2 拆分主文档与 raw payload

新增 Trace v2 writer：

1. 主文档只保存 `ProviderRequestSnapshot`、`PromptTrace`、`RuntimeGovernance`、`Summary`、`RawPayloadRefs`。
2. 原始 request、response、tool invocation、tool result、terminal/runtime event 存入 raw payload store。
3. `preview` 由 summary/indexer 生成，不能被编译链路读取。

验收点：主 Trace 文件大小与工具结果、网页内容长度解耦。

### Phase 5：删除重复兼容字段

按读路径迁移后删除或降级以下字段：

| 字段/结构 | 处理方式 |
| --- | --- |
| `CompiledPrompt.Stable/Dynamic/System/Developer/Tools/Policy` | 改为 `PromptEnvelope` 的只读投影，最终删除写入 |
| `modeltrace.Prompt.StableHash` | 用 `PromptFingerprint` 或 section hash 替代 |
| top-level `visibleToolCount`、`promptCharCount`、`toolRegistryCharCount` | 改为 `TraceSummary` 派生 |
| top-level plan/tool/agent 重复状态 | 迁移到 `RuntimeGovernance` 或 `PromptTrace` |
| `ToolSurfaceTrace` 与 `PromptInputTrace` 重复字段 | 保留一个规范来源，另一个由 UI adapter 派生 |
| `modelInput` 全量复制 | 迁移为 `ProviderRequestSnapshot.Input` + raw payload ref |

验收点：同一个概念在 Trace JSON 中只有一个规范字段；兼容 adapter 只读旧格式，不参与新写入。

### Phase 6：删除临时 adapter 与旧测试依赖

1. v1 Trace reader 保留只读兼容。
2. v2 writer 不再输出 v1 重复字段。
3. 删除依赖旧 top-level 字段的测试和 UI fallback。
4. 增加防回归测试，禁止新代码写入 deprecated 字段。

验收点：旧字段只在 v1 reader、migration adapter 或测试 fixture 中出现。

## 7. 字段取舍清单

| 类别 | 字段/能力 | 决策 | 原因 |
| --- | --- | --- | --- |
| 新增 | `ProviderRequestSnapshot` | 增加 | 解释 provider 实际请求与缓存属性 |
| 新增 | `ModelInputItem` | 增加 | 替代扁平 `role/content`，承载工具、reasoning、compaction、多模态 |
| 新增 | `ContextLineageMetadata` | 增加 | 统一 session/thread/window/turn/parent/subagent/workspace |
| 新增 | `RawPayloadRef` | 增加 | 主 Trace 减重，完整内容延迟读取 |
| 保留 | `PromptEnvelope` | 保留为 Prompt 编译事实源 | 已符合 Stable/Dynamic/section 需求 |
| 保留 | `PromptFingerprint` | 保留 | 表示编译产物 fingerprint，不和 request hash 混用 |
| 降级 | `schema.Message` | 降级为 provider adapter DTO | 不再作为内部上下文事实源 |
| 降级 | `preview` | 降级为摘要投影 | 不参与编译、缓存、业务判断 |
| 删除 | `CompiledPrompt` 中重复兼容视图写入 | 删除 | 防止 Envelope 与旧字段双写漂移 |
| 删除 | Trace top-level 派生统计字段写入 | 删除 | 改由 `TraceSummary` 派生，避免重复写入 |
| 删除 | PromptTrace 与 ToolSurfaceTrace 重复字段 | 合并后删除一侧 | 保证工具面只有一个规范来源 |

## 8. 兼容策略

1. `schemaVersion=1` 的旧 Trace 继续可读，但只能通过 read-only adapter 映射到 v2 view。
2. 新 Trace 只写 `schemaVersion=2`。
3. v1 adapter 不得被 provider 请求、Prompt 编译或缓存逻辑调用。
4. 所有 deprecated 字段需要在 Go struct tag 或注释中标记删除阶段。
5. 迁移期允许 UI 同时读 v1/v2，但不允许 runtime 同时写两套结构。

## 9. 测试与验证

必须补齐以下测试：

1. `ProviderRequestSnapshot` golden test：真实请求与 snapshot 字段一致。
2. `RequestPropertiesHash` 稳定性测试：input-only 变化不影响稳定属性 hash，tools/instructions/reasoning 变化必须影响 hash。
3. `PromptCacheKey` 测试：同一 session/thread 默认稳定，不同 thread 默认不同，显式 override 生效。
4. `ModelInputItem` adapter 测试：message、tool call、tool result、reasoning、compaction 都能正确转成 provider 消息。
5. Trace v2 golden test：主文档没有旧 top-level 重复字段，大内容进入 raw payload ref。
6. 旧 Trace 兼容测试：v1 fixture 能通过只读 adapter 展示，但不会进入新 writer。
7. 静态扫描测试：禁止新增直接构造 `schema.Message` 的上下文来源，禁止新增 deprecated Trace 字段写入。

## 10. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 迁移后形成 `ModelInputItem` 与旧 `Message` 双写 | `Message` 只保留在 adapter 包，增加静态扫描 |
| Trace UI 依赖旧 top-level 字段 | 先提供 v2 view model，再删除旧 writer |
| 过度照搬 Codex provider 细节 | 只迁移抽象能力，不迁移 Rust/provider 特化逻辑 |
| 缓存解释混淆 | 文档和 UI 同时展示 `promptFingerprint`、`requestPropertiesHash`、`promptCacheKey` 的不同含义 |
| raw payload 外置导致排障不方便 | Trace summary 保留 preview、hash、size、content type、ref id，支持按需展开 |
| AIOps 现有治理能力被弱化 | `RuntimeGovernance` 继续保留策略、预算、工具面、验证状态，只调整存储边界 |

## 11. 验收标准

1. `aiops-v2` 只有一条上下文生成主链路：`ContextSourceRegistry -> PromptEnvelope -> ModelInputItem -> ProviderRequestSnapshot`。
2. provider 真实请求可以从 `ProviderRequestSnapshot` 完整解释，不需要再从多个 Trace 字段拼。
3. `preview` 不参与 Prompt 编译、缓存判断或业务分支，只作为展示摘要。
4. `content` 或 `contentRef` 是完整内容事实源，具备 hash 校验。
5. `promptFingerprint`、`requestPropertiesHash`、`promptCacheKey` 三者职责清晰，不互相替代。
6. 新 writer 不再输出 v1 的重复 top-level 字段。
7. 旧 Trace 兼容逻辑只读，不参与新 runtime。
8. 删除旧字段和临时 adapter 有明确测试保护，不能留下永久 fallback。

## 12. 推荐实施顺序

推荐先做 Phase 1 和 Phase 3，因为它们只增加观测能力，风险最低，且能立刻回答“provider 到底看到了什么”“缓存命中为什么变化”。随后做 Phase 2，把 `ModelInputItem` 作为唯一模型输入事实源。最后推进 Trace v2 和旧字段删除。

不建议先大规模删除旧字段。先用 snapshot 和 hash 把行为观测补齐，再按读写路径逐个收口，能避免迁移期间出现不可解释的上下文差异。
