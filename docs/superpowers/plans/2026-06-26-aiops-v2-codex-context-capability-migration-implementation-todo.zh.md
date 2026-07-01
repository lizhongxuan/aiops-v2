# aiops-v2 Codex Context Capability Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 `docs/superpowers/specs/2026-06-26-aiops-v2-codex-context-capability-migration-design.zh.md`，把 aiops-v2 上下文链路收敛为单一事实源，并迁移 Codex 中有价值的 provider request snapshot、结构化 model input、cache key、lineage metadata、Trace v2 和 raw payload ref 能力。

**Architecture:** 不新增第二套 Prompt compiler，不重写 Eino provider adapter，不复制 Codex Rust runtime。实施路径是先增加只读观测结构，再把 `promptinput` 从扁平 `schema.Message` 前置为 `ModelInputItem` 规范结构，最后让 `modeltrace` 写 Trace v2，并逐步删除 v1 重复字段写入。

**Tech Stack:** Go `internal/promptcompiler` / `internal/promptinput` / `internal/modelrouter` / `internal/modeltrace` / `internal/runtimekernel` / `internal/promptdiag` / `internal/eval`, Eino `schema.Message`, Go test, golden JSON trace fixtures.

---

## 1. 实施边界

### 1.1 必须实现

- [ ] `promptcompiler.PromptEnvelope` 继续作为 Prompt 编译事实源，旧 Stable/Dynamic/System/Developer/Tools/Policy 字段只能作为派生兼容视图。
- [ ] `promptinput.ModelInputItem` 成为 aiops-v2 内部模型输入事实源，`schema.Message` 只保留在 Eino adapter 层。
- [ ] `modelrouter.ProviderRequestSnapshot` 能解释 provider 最终请求中的 model、instructions、input、tools、reasoning、stream、prompt cache key 和稳定请求属性 hash。
- [ ] `promptCacheKey` 与 `requestPropertiesHash` 明确写入模型输入 Trace，且不和 `promptFingerprint` 混用。
- [ ] `preview` 只能由 Trace/index 摘要器生成，不参与 Prompt 编译、provider request 构造、缓存判断或业务分支。
- [ ] Trace v2 主文档只保存摘要、规范结构和 raw payload 引用，重型原始请求/响应/工具结果通过 `RawPayloadRef` 读取。
- [ ] v1 Trace reader 保持只读兼容，新 writer 不再输出 v1 的重复 top-level 字段。
- [ ] 增加静态扫描测试，禁止新的上下文来源直接构造 `schema.Message`，禁止新 runtime 写 deprecated Trace 字段。

### 1.2 明确不做

- [ ] 不移植 Codex Rust 类型和 provider 特化逻辑。
- [ ] 不新增 `LitePromptCompiler`、`CodexPromptCompiler` 或任何第二套 Prompt 编译路径。
- [ ] 不把 Codex Lite mode、Azure store 行为、websocket 复用细节硬编码进 aiops-v2 主路径。
- [ ] 不在迁移期长期保留双写 feature flag；每个兼容 adapter 必须有删除条件。
- [ ] 不让 Trace v1 adapter 参与 provider 请求、Prompt 编译、缓存键计算或 runtime 决策。

### 1.3 单 owner 保护线

- [ ] `promptcompiler` 是 Prompt 编译唯一 owner。
- [ ] `promptinput` 是 `ModelInputItem` 生成和 Eino message adapter 唯一 owner。
- [ ] `modelrouter` 是 provider request snapshot、request hash、prompt cache key 规则唯一 owner。
- [ ] `modeltrace` 是 Trace v2 writer、summary、preview、raw payload ref 唯一 owner。
- [ ] `runtimekernel` 只编排这些 owner，不再拼接 Trace payload 或手写 provider request snapshot 字段。

## 2. 文件边界

### 2.1 后端新增文件

- [ ] Create: `internal/modelrouter/provider_request_snapshot.go`  
  定义 `ProviderRequestSnapshot`、`ContextLineageMetadata`、`ToolSpecSnapshot`、`ReasoningOptions`、`TextOptions`、`BuildProviderRequestSnapshot`、`RequestPropertiesHash`、`DefaultPromptCacheKey`。
- [ ] Create: `internal/modelrouter/provider_request_snapshot_test.go`  
  覆盖 request hash、prompt cache key、lineage metadata 和 snapshot 字段稳定性。
- [ ] Create: `internal/promptinput/model_input_item.go`  
  定义 `ModelInputItem`、`ModelContentItem`、`ToolCallItem`、`ToolOutputItem`、`ReasoningItem`、item type 常量和 content hash helper。
- [ ] Create: `internal/promptinput/model_input_adapter_eino.go`  
  实现 `ModelInputItemsToSchema`，把规范 item 转成 Eino `schema.Message`。
- [ ] Create: `internal/promptinput/model_input_item_test.go`  
  覆盖 message、tool call、tool result、reasoning、compaction、多段 content 转换。
- [ ] Create: `internal/modeltrace/trace_v2.go`  
  定义 `TraceDocumentV2`、`TraceSummary`、`PromptTraceV2`、`RuntimeGovernanceTrace`、`RawPayloadRef`、`BuildTraceDocumentV2`。
- [ ] Create: `internal/modeltrace/raw_payload_store.go`  
  定义文件型 raw payload store，负责保存完整 request/response/tool payload 并返回 ref。
- [ ] Create: `internal/modeltrace/trace_v2_test.go`  
  覆盖 v2 JSON shape、preview/content 规则、raw payload ref 和 no duplicated top-level fields。
- [ ] Create: `internal/modeltrace/compat_v1_reader.go`  
  实现只读 v1 -> v2 view adapter，禁止被 writer 或 runtime request path 引用。
- [ ] Create: `internal/modeltrace/static_guard_test.go`  
  扫描 deprecated 写入点和直接构造 `schema.Message` 的上下文来源。

### 2.2 后端修改文件

- [ ] Modify: `internal/promptinput/types.go`  
  将 `BuildResult` 扩展为同时包含 `Items []ModelInputItem` 和兼容 `Messages []*schema.Message`；标记旧 `Message` 为 adapter input only。
- [ ] Modify: `internal/promptinput/builder.go`  
  先构造 `ModelInputItem[]`，再调用 Eino adapter 生成 `schema.Message`。
- [ ] Modify: `internal/promptinput/adapter_eino.go`  
  保留旧 `MessagesToSchema` 作为兼容 wrapper，并让它内部先转 `ModelInputItem`。
- [ ] Modify: `internal/promptinput/trace.go`  
  让 `TraceItem` 由 `ModelInputItem` 派生，保留 ID、type、phase、metadata、content hash。
- [ ] Modify: `internal/runtimekernel/model_input.go`  
  在 `ModelInputDebugTraceRequest` 增加 `ModelInputItems []promptinput.ModelInputItem` 和 `ProviderRequest modelrouter.ProviderRequestSnapshot`，避免 trace writer 重新推断。
- [ ] Modify: `internal/runtimekernel/eino_kernel.go`  
  在 `buildPromptInputWithContextGovernance` 后构造 provider request snapshot，并传入 `writeModelInputDebugTrace`；实际 `chatModel.Stream` 仍使用 adapter 生成的 `schema.Message`。
- [ ] Modify: `internal/modeltrace/trace.go`  
  保留 v1 writer 兼容，但新路径通过 `BuildTraceDocumentV2` 写 schemaVersion 2。
- [ ] Modify: `internal/modeltrace/markdown.go` 或当前 `renderMarkdown` 所在文件  
  Markdown 输出优先展示 v2 summary、provider request、prompt cache key、request hash、raw payload refs。
- [ ] Modify: `internal/promptdiag/trace_index.go`  
  支持 v2 trace index，从 `summary` 和 `providerRequest` 读取 preview、fingerprint、request hash、cache key。
- [ ] Modify: `internal/eval/scorer.go`  
  读取 v2 `providerRequest.promptCacheKey`、`providerRequest.requestPropertiesHash` 和 `promptTrace.promptFingerprint`。

### 2.3 测试与 fixture 文件

- [ ] Create: `internal/modeltrace/testdata/trace_v1_runtime_model_input.json`  
  固化一个旧 v1 Trace fixture，用于只读兼容测试。
- [ ] Create: `internal/modeltrace/testdata/trace_v2_runtime_model_input.golden.json`  
  固化一个 v2 Trace golden，包含 provider request snapshot、summary、raw payload refs。
- [ ] Modify/Create: `internal/runtimekernel/model_input_trace_test.go`  
  增加 v2 Trace 写入、request snapshot、preview 不回流、cache key 可见性测试。
- [ ] Modify/Create: `internal/promptinput/builder_test.go`  
  增加 `BuildResult.Items` 和 `BuildResult.Messages` 一致性测试。
- [ ] Modify/Create: `internal/promptdiag/trace_index_test.go`  
  增加 v1/v2 trace index 混合读取测试。
- [ ] Modify/Create: `internal/eval/scorer_test.go`  
  增加 v2 prompt fingerprint 与 request hash 读取测试。

### 2.4 文档和清理

- [ ] Modify: `docs/superpowers/specs/2026-06-26-aiops-v2-codex-context-capability-migration-design.zh.md`  
  仅当实施中发现职责边界必须调整时同步修订，不在代码里私下分叉。
- [ ] Modify: `docs/superpowers/plans/2026-06-26-aiops-v2-codex-context-capability-migration-implementation-todo.zh.md`  
  每完成一个任务勾选对应步骤，保持当前计划与实际迁移状态一致。

## 3. Phase 0：基线、字段审计和护栏

### Task 0.1：记录当前基线

**Files:**
- Read: `internal/promptinput/builder.go`
- Read: `internal/promptinput/types.go`
- Read: `internal/modeltrace/trace.go`
- Read: `internal/runtimekernel/model_input.go`
- Read: `internal/runtimekernel/eino_kernel.go`
- Read: `internal/promptdiag/trace_index.go`

- [ ] **Step 1: 运行上下文相关 Go 测试**

Run:

```bash
go test ./internal/promptcompiler ./internal/promptinput ./internal/modelrouter ./internal/modeltrace ./internal/runtimekernel ./internal/promptdiag ./internal/eval -count=1
```

Expected: 记录当前失败项；如果失败与本迁移无关，保留原始输出摘要在实施记录中，不先修无关问题。

- [ ] **Step 2: 记录当前直接构造 `schema.Message` 的位置**

Run:

```bash
rg -n "schema\\.(SystemMessage|UserMessage|AssistantMessage|ToolMessage)|&schema\\.Message" internal/promptinput internal/runtimekernel internal/modeltrace internal/spanstream
```

Expected: 至少看到 `internal/promptinput/builder.go`、`internal/promptinput/adapter_eino.go`、`internal/spanstream/compressor.go`。Phase 2 后，除 adapter、compressor 专用摘要路径和测试外，runtime 上下文来源不应直接构造 provider message。

- [ ] **Step 3: 记录 v1 Trace 重复字段**

Run:

```bash
rg -n "VisibleToolCount|PromptCharCount|ModelInputStats|ToolRegistryCharCount|ToolSurfaceTrace|PromptInputTrace|PlanModeState|TaskTodoState" internal/modeltrace internal/runtimekernel
```

Expected: 字段集中在 `internal/modeltrace/trace.go` 和 `internal/runtimekernel/model_input.go`。Phase 5 后，新 writer 不再写这些 top-level 重复字段。

### Task 0.2：增加静态护栏测试骨架

**Files:**
- Create: `internal/modeltrace/static_guard_test.go`

- [ ] **Step 1: 写失败测试**

新增测试函数：

```go
func TestNoRuntimeContextSourceDirectlyConstructsSchemaMessage(t *testing.T)
func TestTraceV2WriterDoesNotWriteDeprecatedTopLevelFields(t *testing.T)
```

第一条扫描 `internal/runtimekernel` 与 `internal/promptinput`，允许列表只有：

```text
internal/promptinput/model_input_adapter_eino.go
internal/promptinput/adapter_eino.go
internal/promptinput/model_input_item_test.go
internal/promptinput/builder_test.go
```

第二条扫描 `internal/modeltrace/trace_v2.go`，禁止出现：

```text
VisibleToolCount
PromptCharCount
ModelInputStats
ToolRegistryCharCount
ToolSurfaceTrace
PlanModeState
TaskTodoState
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
go test ./internal/modeltrace -run 'TestNoRuntimeContextSourceDirectlyConstructsSchemaMessage|TestTraceV2WriterDoesNotWriteDeprecatedTopLevelFields' -count=1
```

Expected: FAIL，失败原因是新测试文件或新 v2 writer 尚不存在。

- [ ] **Step 3: 保留测试为后续阶段验收**

不要为了让骨架测试通过而扩大 allowlist。后续 Phase 2 和 Phase 4 完成后，该测试必须真实通过。

## 4. Phase 1：ProviderRequestSnapshot、cache key 和 request hash

### Task 1.1：定义 provider request snapshot 类型

**Files:**
- Create: `internal/modelrouter/provider_request_snapshot.go`
- Test: `internal/modelrouter/provider_request_snapshot_test.go`

- [ ] **Step 1: 写 `ProviderRequestSnapshot` 字段测试**

测试构造输入：

```go
snapshot := BuildProviderRequestSnapshot(ProviderRequestSnapshotInput{
    Provider: "openai",
    Model: "gpt-5.4",
    Instructions: "system rules",
    Input: []promptinput.ModelInputItem{{ID: "user-1", Type: promptinput.ModelInputItemMessage, Role: "user"}},
    Tools: []ToolSpecSnapshot{{Name: "exec_command", Description: "run command"}},
    ParallelToolCalls: true,
    Reasoning: &ReasoningOptions{Effort: "high"},
    Stream: true,
    ClientMetadata: ContextLineageMetadata{SessionID: "sess-1", ThreadID: "thread-1", TurnID: "turn-1"},
})
```

Assert:

```text
snapshot.Provider == "openai"
snapshot.Model == "gpt-5.4"
snapshot.PromptCacheKey == "thread-1"
snapshot.RequestPropertiesHash != ""
snapshot.ClientMetadata.SessionID == "sess-1"
```

- [ ] **Step 2: 实现类型和 builder**

字段使用设计文档中的名字，并采用 JSON tag：

```go
type ProviderRequestSnapshot struct {
    Provider              string                 `json:"provider,omitempty"`
    Model                 string                 `json:"model,omitempty"`
    Instructions          string                 `json:"instructions,omitempty"`
    Input                 []promptinput.ModelInputItem `json:"input,omitempty"`
    Tools                 []ToolSpecSnapshot     `json:"tools,omitempty"`
    ToolChoice            any                    `json:"toolChoice,omitempty"`
    ParallelToolCalls     bool                   `json:"parallelToolCalls,omitempty"`
    Reasoning             *ReasoningOptions      `json:"reasoning,omitempty"`
    Store                 bool                   `json:"store,omitempty"`
    Stream                bool                   `json:"stream,omitempty"`
    Include               []string               `json:"include,omitempty"`
    ServiceTier           string                 `json:"serviceTier,omitempty"`
    PromptCacheKey        string                 `json:"promptCacheKey,omitempty"`
    RequestPropertiesHash string                 `json:"requestPropertiesHash,omitempty"`
    Text                  *TextOptions           `json:"text,omitempty"`
    ClientMetadata        ContextLineageMetadata `json:"clientMetadata,omitempty"`
}
```

`DefaultPromptCacheKey` 优先级：

```text
ExplicitPromptCacheKey -> ThreadID -> SessionID -> CaseID -> ""
```

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/modelrouter -run 'TestBuildProviderRequestSnapshot|TestDefaultPromptCacheKey' -count=1
```

Expected: PASS。

### Task 1.2：实现稳定请求属性 hash

**Files:**
- Modify: `internal/modelrouter/provider_request_snapshot.go`
- Test: `internal/modelrouter/provider_request_snapshot_test.go`

- [ ] **Step 1: 写 hash 稳定性测试**

测试三个 snapshot：

```text
base: model=gpt-5.4, instructions=A, tools=[exec_command], reasoning=high, input=[user says A]
inputOnlyChanged: 同 base，但 input=[user says B]
toolChanged: 同 base，但 tools=[exec_command, web_search]
```

Assert:

```text
base.RequestPropertiesHash == inputOnlyChanged.RequestPropertiesHash
base.RequestPropertiesHash != toolChanged.RequestPropertiesHash
```

- [ ] **Step 2: 实现 canonical JSON hash**

Hash 覆盖字段：

```text
provider
model
instructions
tools
toolChoice
parallelToolCalls
reasoning
store
stream
include
serviceTier
promptCacheKey
text
```

明确不覆盖：

```text
input
clientMetadata.turnId
clientMetadata.turnStartedAtUnixMs
```

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/modelrouter -run TestRequestPropertiesHash -count=1
```

Expected: PASS。

## 5. Phase 2：ModelInputItem 规范结构和 Eino adapter

### Task 2.1：定义 ModelInputItem 和内容 hash

**Files:**
- Create: `internal/promptinput/model_input_item.go`
- Test: `internal/promptinput/model_input_item_test.go`

- [ ] **Step 1: 写 item JSON 和 hash 测试**

测试覆盖：

```text
message item: type=message, role=user, content text="hello"
tool call item: type=tool_call, tool name=exec_command, arguments={"cmd":"date"}
tool result item: type=tool_result, toolCallId=call-1, content text="Fri"
reasoning item: type=reasoning, summary present, encrypted content omitted from preview
compaction item: type=compaction, contentRef present
```

Assert:

```text
ContentHash(item) != ""
Preview(item, 20) never mutates item.Content
Preview(reasoning) does not expose encrypted content
```

- [ ] **Step 2: 实现类型和常量**

使用常量：

```go
const (
    ModelInputItemMessage    = "message"
    ModelInputItemToolCall   = "tool_call"
    ModelInputItemToolResult = "tool_result"
    ModelInputItemReasoning  = "reasoning"
    ModelInputItemCompaction = "compaction"
)
```

`preview` 不作为 `ModelInputItem` 字段；preview 由 `modeltrace` summary 生成。

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/promptinput -run 'TestModelInputItem|TestModelInputPreview' -count=1
```

Expected: PASS。

### Task 2.2：实现 ModelInputItem -> Eino adapter

**Files:**
- Create: `internal/promptinput/model_input_adapter_eino.go`
- Modify: `internal/promptinput/adapter_eino.go`
- Test: `internal/promptinput/model_input_item_test.go`

- [ ] **Step 1: 写 adapter 测试**

覆盖转换规则：

```text
message/system -> schema.SystemMessage
message/user -> schema.UserMessage
message/assistant with ToolCall -> schema.AssistantMessage(...toolCalls)
tool_result -> schema.ToolMessage(content, toolCallId)
reasoning -> not converted to provider message unless explicitly marked model_visible
compaction -> schema.SystemMessage with semantic_role=compaction and source refs
```

- [ ] **Step 2: 实现 adapter**

函数签名：

```go
func ModelInputItemsToSchema(items []ModelInputItem) ([]*schema.Message, error)
```

旧函数保留为 wrapper：

```go
func MessagesToSchema(messages []Message) ([]*schema.Message, error) {
    return ModelInputItemsToSchema(MessagesToModelInputItems(messages))
}
```

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/promptinput -run 'TestModelInputItemsToSchema|TestMessagesToSchema' -count=1
```

Expected: PASS。

### Task 2.3：让 Builder 先产出 ModelInputItem

**Files:**
- Modify: `internal/promptinput/types.go`
- Modify: `internal/promptinput/builder.go`
- Modify: `internal/promptinput/trace.go`
- Test: `internal/promptinput/builder_test.go`
- Test: `internal/promptinput/trace_test.go`

- [ ] **Step 1: 写 BuildResult 一致性测试**

测试 `Builder{}.Build` 后：

```text
len(result.Items) > 0
len(result.Messages) > 0
result.Trace.Items contains IDs from result.Items
schema messages equal ModelInputItemsToSchema(result.Items)
```

- [ ] **Step 2: 扩展 `BuildResult`**

新增字段：

```go
type BuildResult struct {
    Items    []ModelInputItem
    Messages []*schema.Message
    Trace    PromptInputTrace
}
```

- [ ] **Step 3: 修改 Builder 构造顺序**

顺序必须是：

```text
compiled prompt sections -> ModelInputItem
ops context capsule -> ModelInputItem
memory -> ModelInputItem
current turn history -> ModelInputItem
ModelInputItemsToSchema -> []*schema.Message
buildTrace from ModelInputItem
```

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/promptinput -count=1
```

Expected: PASS。

## 6. Phase 3：runtime 接入 provider snapshot

### Task 3.1：把 snapshot 传入模型输入 trace

**Files:**
- Modify: `internal/runtimekernel/model_input.go`
- Modify: `internal/runtimekernel/eino_kernel.go`
- Test: `internal/runtimekernel/model_input_trace_test.go`

- [ ] **Step 1: 写 runtime trace 测试**

在 `model_input_trace_test.go` 增加测试：

```text
writeModelInputDebugTrace receives ProviderRequestSnapshot
written JSON contains providerRequest.promptCacheKey
written JSON contains providerRequest.requestPropertiesHash
written JSON contains providerRequest.clientMetadata.sessionId
written JSON contains promptTrace.promptFingerprint
```

- [ ] **Step 2: 扩展 `ModelInputDebugTraceRequest`**

新增字段：

```go
ModelInputItems []promptinput.ModelInputItem
ProviderRequest modelrouter.ProviderRequestSnapshot
```

- [ ] **Step 3: 在 `eino_kernel.go` 构造 snapshot**

构造位置在：

```text
promptBuild, modelErr := buildPromptInputWithContextGovernance(...)
modelInput := promptBuild.Messages
```

之后立即生成：

```text
providerRequest := modelrouter.BuildProviderRequestSnapshot(...)
```

输入包括：

```text
Provider/Model from resolved provider config or capabilities
Instructions from promptcompiler.CompiledPromptStableText(compiled)
Input from promptBuild.Items
Tools from compileCtx.AssembledTools
Reasoning from compileCtx.ReasoningEffort
Stream true
ClientMetadata from session.ID, thread metadata, turnID, parent/subagent metadata
```

- [ ] **Step 4: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestModelInputDebugTrace.*ProviderRequest|TestModelInputDebugTrace.*PromptCache' -count=1
```

Expected: PASS。

### Task 3.2：保持真实 provider 请求不变

**Files:**
- Modify: `internal/runtimekernel/eino_kernel.go`
- Test: `internal/runtimekernel/model_response_test.go`

- [ ] **Step 1: 写不改变请求行为测试**

使用已有 mock model 捕获 `generateModelResponse` 输入，Assert：

```text
chatModel.Stream receives promptBuild.Messages
ProviderRequestSnapshot.Input derives from promptBuild.Items
No second prompt build happens between snapshot and Stream
```

- [ ] **Step 2: 保持 Stream 调用原路径**

真实请求仍使用：

```go
stream, streamErr := chatModel.Stream(ctx, input, opts...)
```

其中 `input` 来自 `promptBuild.Messages`，不是由 Trace v2 writer 反向生成。

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/runtimekernel -run 'TestGenerateModelResponse|TestProviderRequestSnapshotDoesNotChangeStreamInput' -count=1
```

Expected: PASS。

## 7. Phase 4：TraceDocumentV2、summary、preview 和 raw payload refs

### Task 4.1：定义 Trace v2 主结构

**Files:**
- Create: `internal/modeltrace/trace_v2.go`
- Test: `internal/modeltrace/trace_v2_test.go`

- [ ] **Step 1: 写 v2 JSON shape golden 测试**

Golden 必须包含：

```json
{
  "schemaVersion": 2,
  "kind": "runtime_model_input",
  "summary": {
    "visibleToolCount": 1,
    "messageCount": 2
  },
  "providerRequest": {
    "promptCacheKey": "thread-1",
    "requestPropertiesHash": "sha256:"
  },
  "promptTrace": {
    "promptFingerprint": {
      "stableHash": "stable-1"
    }
  },
  "rawPayloadRefs": []
}
```

测试还要 Assert 主文档不存在这些 v1 top-level 字段：

```text
visibleToolCount
promptCharCount
modelInputStats
toolRegistryCharCount
toolSurfaceTrace
planModeState
taskTodoState
```

- [ ] **Step 2: 实现 `BuildTraceDocumentV2`**

输入从 `modeltrace.Request` 读取，但只输出 v2 规范字段：

```go
func BuildTraceDocumentV2(req Request) TraceDocumentV2
```

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/modeltrace -run 'TestBuildTraceDocumentV2|TestTraceV2DoesNotWriteDeprecatedTopLevelFields' -count=1
```

Expected: PASS。

### Task 4.2：实现 preview 与 raw payload store

**Files:**
- Create: `internal/modeltrace/raw_payload_store.go`
- Modify: `internal/modeltrace/trace_v2.go`
- Test: `internal/modeltrace/trace_v2_test.go`

- [ ] **Step 1: 写 preview/content 测试**

测试要求：

```text
TraceSummary preview <= 240 chars
preview is derived from content or contentRef metadata
preview does not appear in ProviderRequestSnapshot.Input
large content writes RawPayloadRef
RawPayloadRef has id, kind, path, contentType, bytes, sha256
```

- [ ] **Step 2: 实现 raw payload ref**

字段：

```go
type RawPayloadRef struct {
    ID          string `json:"id"`
    Kind        string `json:"kind"`
    Path        string `json:"path,omitempty"`
    ContentType string `json:"contentType,omitempty"`
    Bytes       int64  `json:"bytes,omitempty"`
    SHA256      string `json:"sha256,omitempty"`
    Preview     string `json:"preview,omitempty"`
}
```

`Preview` 是展示投影，不能被 provider request builder 引用。

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/modeltrace -run 'TestTraceV2Preview|TestRawPayloadStore' -count=1
```

Expected: PASS。

### Task 4.3：让 `modeltrace.Write` 写 v2

**Files:**
- Modify: `internal/modeltrace/trace.go`
- Modify: `internal/modeltrace/trace_v2.go`
- Test: `internal/modeltrace/trace_v2_test.go`
- Test: `internal/runtimekernel/model_input_trace_test.go`

- [ ] **Step 1: 写 writer 测试**

设置 `AIOPS_DEBUG_MODEL_INPUT_TRACE=1` 和临时 trace dir，调用 `modeltrace.Write` 后 Assert：

```text
JSON schemaVersion == 2
providerRequest exists
summary exists
rawPayloadRefs exists
legacy v1 prompt/modelInput full duplicate fields absent from new writer output
Markdown contains Prompt cache key and Request properties hash
```

- [ ] **Step 2: 切换 writer**

`Write` 中：

```text
payload := buildPayload(req)
```

替换为：

```text
doc := BuildTraceDocumentV2(req)
```

保留 v1 `buildPayload` 给 `compat_v1_reader.go` 和旧 fixture 测试使用，不再由新 writer 调用。

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/modeltrace ./internal/runtimekernel -run 'Test.*TraceV2|TestModelInputDebugTrace' -count=1
```

Expected: PASS。

## 8. Phase 5：v1 只读兼容和旧字段清理

### Task 5.1：实现 v1 -> v2 view adapter

**Files:**
- Create: `internal/modeltrace/compat_v1_reader.go`
- Test: `internal/modeltrace/trace_v2_test.go`
- Fixture: `internal/modeltrace/testdata/trace_v1_runtime_model_input.json`

- [ ] **Step 1: 写 v1 fixture 兼容测试**

读取 fixture 后 Assert：

```text
SchemaVersion 1 can be projected to TraceDocumentV2 view
providerRequest may be partial but summary and promptTrace are present
adapter has no Write function
adapter is not imported by runtimekernel
```

- [ ] **Step 2: 实现只读 adapter**

函数签名：

```go
func ProjectV1PayloadToTraceDocumentV2(data []byte) (TraceDocumentV2, error)
```

只做展示映射，不返回可发送 provider request。

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/modeltrace -run 'TestProjectV1PayloadToTraceDocumentV2|TestCompatV1ReaderIsReadOnly' -count=1
```

Expected: PASS。

### Task 5.2：更新 trace index、eval scorer 和 promptdiag 读路径

**Files:**
- Modify: `internal/promptdiag/trace_index.go`
- Modify/Create: `internal/promptdiag/trace_index_test.go`
- Modify: `internal/eval/scorer.go`
- Modify/Create: `internal/eval/scorer_test.go`

- [ ] **Step 1: 写 v2 读取测试**

`trace_index_test.go` Assert：

```text
Trace index reads schemaVersion 2
StableHash comes from promptTrace.promptFingerprint.stableHash
PromptCacheKey comes from providerRequest.promptCacheKey
RequestPropertiesHash comes from providerRequest.requestPropertiesHash
Preview comes from summary, not providerRequest.input preview
```

`scorer_test.go` Assert：

```text
PromptFingerprints still populated from v2 trace
RequestPropertiesHash is available for diagnostics
v1 fixture still readable through compat adapter
```

- [ ] **Step 2: 实现 reader 分支**

读 trace JSON 时：

```text
schemaVersion == 2 -> read TraceDocumentV2
schemaVersion == 1 -> ProjectV1PayloadToTraceDocumentV2
missing schemaVersion -> treat as v1
```

- [ ] **Step 3: 运行测试**

Run:

```bash
go test ./internal/promptdiag ./internal/eval -run 'Test.*Trace.*V2|Test.*PromptFingerprint' -count=1
```

Expected: PASS。

### Task 5.3：删除新 writer 的 v1 重复字段依赖

**Files:**
- Modify: `internal/modeltrace/trace.go`
- Modify: `internal/runtimekernel/model_input.go`
- Test: `internal/modeltrace/static_guard_test.go`

- [ ] **Step 1: 删除或隔离新 writer 对 v1 字段的写入**

新 writer 禁止写：

```text
Prompt.StableHash
VisibleToolCount
PromptCharCount
ModelInputStats
ToolRegistryCharCount
ToolSurfaceTrace
PlanModeState top-level projection
TaskTodoState top-level projection
ModelInput full duplicate
```

- [ ] **Step 2: 保留 deprecated 字段只读注释**

在 v1 payload 相关类型上加注释：

```go
// Deprecated: v1 trace compatibility only. New runtime writes TraceDocumentV2.
```

- [ ] **Step 3: 运行静态护栏**

Run:

```bash
go test ./internal/modeltrace -run 'TestNoRuntimeContextSourceDirectlyConstructsSchemaMessage|TestTraceV2WriterDoesNotWriteDeprecatedTopLevelFields' -count=1
```

Expected: PASS。

## 9. Phase 6：验收测试和清理

### Task 6.1：全量上下文相关测试

**Files:**
- Test only

- [ ] **Step 1: 运行后端相关包测试**

Run:

```bash
go test ./internal/promptcompiler ./internal/promptinput ./internal/modelrouter ./internal/modeltrace ./internal/runtimekernel ./internal/promptdiag ./internal/eval -count=1
```

Expected: PASS。

- [ ] **Step 2: 运行直接构造 provider message 扫描**

Run:

```bash
rg -n "schema\\.(SystemMessage|UserMessage|AssistantMessage|ToolMessage)|&schema\\.Message" internal/promptinput internal/runtimekernel internal/modeltrace
```

Expected: runtime 上下文路径中没有新的直接构造点；允许项只在 adapter、test 或非 runtime provider 摘要路径。

- [ ] **Step 3: 运行 deprecated Trace 字段扫描**

Run:

```bash
rg -n "VisibleToolCount|PromptCharCount|ModelInputStats|ToolRegistryCharCount|ToolSurfaceTrace|PlanModeState|TaskTodoState" internal/modeltrace internal/runtimekernel
```

Expected: v2 writer 不写这些字段；命中只允许出现在 v1 compat 类型、v1 fixture 测试、deprecated 注释和旧 reader。

### Task 6.2：人工检查生成的 Trace v2

**Files:**
- Runtime generated trace under `AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR`

- [ ] **Step 1: 开启 trace 并跑一个最小 turn**

Run:

```bash
AIOPS_DEBUG_MODEL_INPUT_TRACE=1 AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR=/tmp/aiops-trace-v2 go test ./internal/runtimekernel -run TestModelInputDebugTraceRecordsTaskDepthAndReasoningEffort -count=1
```

Expected: `/tmp/aiops-trace-v2` 下生成 schemaVersion 2 JSON。

- [ ] **Step 2: 检查关键字段**

Run:

```bash
rg -n "\"schemaVersion\": 2|\"providerRequest\"|\"promptCacheKey\"|\"requestPropertiesHash\"|\"rawPayloadRefs\"|\"promptFingerprint\"" /tmp/aiops-trace-v2
```

Expected: 每个字段至少命中一次。

- [ ] **Step 3: 检查 preview 不回流**

Run:

```bash
rg -n "\"preview\"" /tmp/aiops-trace-v2
```

Expected: `preview` 只出现在 `summary` 或 `rawPayloadRefs`，不出现在 `providerRequest.input`。

### Task 6.3：最终清理清单

**Files:**
- Modify as needed from previous tasks

- [ ] **Step 1: 确认没有永久迁移 flag**

Run:

```bash
rg -n "TRACE_V2|MODEL_INPUT_ITEM|PROVIDER_REQUEST_SNAPSHOT|CONTEXT_MIGRATION|compat.*write|dual" internal
```

Expected: 没有用于长期双写的 feature flag；compat 只读命中可保留。

- [ ] **Step 2: 确认计划验收项全部满足**

逐项检查设计文档第 11 节：

```text
single context generation chain
provider request explainable from snapshot
preview not used for compile/cache/business branch
content or contentRef remains full source of truth
promptFingerprint/requestPropertiesHash/promptCacheKey separated
new writer does not output v1 duplicate top-level fields
v1 compatibility is read-only
temporary adapter has removal tests
```

- [ ] **Step 3: 最终测试命令**

Run:

```bash
go test ./internal/promptcompiler ./internal/promptinput ./internal/modelrouter ./internal/modeltrace ./internal/runtimekernel ./internal/promptdiag ./internal/eval -count=1
```

Expected: PASS。

- [ ] **Step 4: 提交建议**

当前计划建议按 phase 拆 commit：

```bash
git add internal/modelrouter internal/promptinput internal/runtimekernel internal/modeltrace internal/promptdiag internal/eval docs/superpowers
git commit -m "feat(aiops): add provider request snapshot for context traces"
git commit -m "feat(aiops): introduce structured model input items"
git commit -m "feat(aiops): write trace v2 with raw payload refs"
git commit -m "chore(aiops): remove duplicated context trace writer fields"
```

实际提交前必须根据工作区已有未提交改动重新拆分，不能把无关改动混入。

## 10. 执行顺序建议

推荐按下面顺序执行：

1. Task 0.1 -> Task 1.1 -> Task 1.2：先补 provider request 可观测性。
2. Task 2.1 -> Task 2.2 -> Task 2.3：再收敛模型输入事实源。
3. Task 3.1 -> Task 3.2：把 snapshot 接入 runtime，但保持真实请求不变。
4. Task 4.1 -> Task 4.2 -> Task 4.3：切换 Trace v2 writer。
5. Task 5.1 -> Task 5.2 -> Task 5.3：迁移读路径并删除重复写入。
6. Task 6.1 -> Task 6.2 -> Task 6.3：完成验收和清理。

关键原则：任何阶段如果需要临时兼容，兼容逻辑只能读旧结构，不能写第二套新 runtime 结果。
