# aiops-v2 Agent Runtime Layering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 `2026-06-26-aiops-v2-agent-runtime-layering-design.zh.md` 一步到位切换到单一 Agent Runtime 分层，删除旧 Eino-facing runtime 事实源。

**Architecture:** 最终事实链为 `TurnRequest -> RuntimeTurnContext -> RuntimeStepContext -> ModelInputItem[] -> ProviderRequestSnapshot -> ProviderAdapter -> TraceDocumentV2 / TurnItem Projection`。`schema.Message` 只允许存在于 Eino provider adapter、Eino tool adapter 和 adapter 测试白名单中；runtime、promptinput、promptcompiler、modeltrace 不再把 Eino DTO 当事实源。

**Tech Stack:** Go runtime (`internal/runtimekernel`, `internal/promptinput`, `internal/promptcompiler`, `internal/modelrouter`, `internal/modeltrace`), CloudWeGo Eino provider adapter, React/TypeScript PromptTrace UI, Go tests, Vitest/React tests, `rg` static guards.

---

## Scope Rules

- [x] 最终代码不加 runtime feature flag。
- [x] 最终代码不保留旧 `RunTurn` 成功路径。
- [x] 最终代码不双写 v1/v2 trace。
- [x] 最终代码不保留 `EinoKernel` alias。
- [x] 最终代码不让 `schema.Message` 穿透到 `promptcompiler`、`promptinput`、`runtimekernel`、`modeltrace`。
- [x] 历史 trace 文件不迁移；PromptTrace UI 只读新 v2 index 和 v2 document。

## File Structure

**Create**

- `internal/runtimekernel/runtime_kernel.go`：唯一 `RuntimeKernel` 类型、构造函数、公共 runtime gateway 实现。
- `internal/runtimekernel/runtime_turn_context.go`：turn 级冻结快照和 builder。
- `internal/runtimekernel/runtime_step_context.go`：iteration/model-call 级快照和 builder。
- `internal/runtimekernel/tool_router_snapshot.go`：工具注册、可见、可 dispatch、隐藏原因快照。
- `internal/modelrouter/provider_adapter.go`：provider 调用统一入口。
- `internal/modelrouter/provider_request_snapshot.go`：provider 请求规范快照、cache key 和 hash。
- `internal/modelrouter/provider_response.go`：provider 输出、usage、finish reason、stream metrics。
- `internal/modelrouter/eino_message_adapter.go`：唯一 `ModelInputItem -> schema.Message` 转换器。
- `internal/promptinput/model_input_item.go`：内部模型输入事实源。
- `internal/promptinput/model_input_validation.go`：item 校验、hash、测试辅助。
- `internal/modeltrace/trace_v2.go`：Trace v2 writer 和 view。
- `internal/modeltrace/raw_payload.go`：raw request/response/tool result 外置引用。
- `scripts/verify-agent-runtime-single-path.sh`：静态门禁脚本。

**Modify**

- `internal/promptcompiler/types.go`：删除 `CompileForEino` 接口。
- `internal/promptcompiler/compiler.go`：保留 `Compile` 作为唯一 prompt compiler runtime 接口。
- `internal/promptinput/types.go`：删除 Eino schema import 和 `BuildResult.Messages`。
- `internal/promptinput/builder.go`：只产出 `ModelInputItem[]` 和 trace。
- `internal/runtimekernel/eino_kernel.go`：拆分为 `RuntimeKernel`、turn loop、provider call 边界；最终删除直接 Eino model call。
- `internal/runtimekernel/model_input.go`：改为消费 `ModelInputItem[]` 和 Trace v2。
- `internal/runtimekernel/agent_config_runner.go`：child agent 走同一 `RuntimeKernel` 和 provider adapter。
- `internal/modeltrace/trace.go`：删除 v1 writer 或从生产 runtime 断开。
- `internal/promptdiag/*`：读取 Trace v2 view model。
- `web/src/pages/PromptTracePage.tsx`：只读取 v2 index/document。
- `web/src/utils/promptTraceViewModel.*`：解析 Trace v2。
- `.gitignore`：加入本实施计划白名单。

**Delete**

- `internal/promptcompiler/eino_format.go`。
- v1 trace writer 的生产入口。
- runtime 中直接调用 `chatModel.Stream` / `chatModel.Generate` 的函数。
- `EinoKernel` / `NewEinoKernel` 旧类型和 constructor 名称。

---

### Task 1: Add Final Runtime Context Types

**Files:**
- Create: `internal/runtimekernel/runtime_turn_context.go`
- Create: `internal/runtimekernel/runtime_step_context.go`
- Test: `internal/runtimekernel/runtime_context_test.go`

- [x] **Step 1: Write failing tests for turn and step context snapshots**

Add `internal/runtimekernel/runtime_context_test.go`:

```go
package runtimekernel

import (
	"testing"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

func TestBuildRuntimeTurnContextFreezesRequestMetadata(t *testing.T) {
	req := TurnRequest{
		SessionType:     SessionTypeHost,
		Mode:            ModeChat,
		SessionID:       "session-1",
		TurnID:          "turn-1",
		ClientTurnID:    "client-turn-1",
		ClientMessageID: "client-message-1",
		HostID:          "host-a",
		Metadata: map[string]string{
			"profile":          RuntimePromptProfileHostWorker,
			"reasoningEffort":  "high",
			"approvalPolicy":   "on-request",
			"runtimeRoute":     "host",
			"contextBudgetKey": "host-default",
		},
	}
	session := &SessionState{ID: "session-1", Type: SessionTypeHost, Mode: ModeChat, HostID: "host-a"}

	ctx, err := BuildRuntimeTurnContext(req, session, RuntimeTurnContextOptions{
		Model: modelrouter.ModelCapabilities{Provider: "openai", Model: "gpt-4.1", MaxContextTokens: 200000},
		ContextBudget: RuntimeContextBudgetSnapshot{
			MaxTokens: 200000,
			TargetTokens: 120000,
		},
	})
	if err != nil {
		t.Fatalf("BuildRuntimeTurnContext() error = %v", err)
	}

	req.Metadata["profile"] = "mutated-after-build"
	if ctx.Profile != RuntimePromptProfileHostWorker {
		t.Fatalf("Profile = %q, want frozen %q", ctx.Profile, RuntimePromptProfileHostWorker)
	}
	if ctx.SessionID != "session-1" || ctx.TurnID != "turn-1" || ctx.HostID != "host-a" {
		t.Fatalf("unexpected ids: %#v", ctx)
	}
	if ctx.Model.Model != "gpt-4.1" {
		t.Fatalf("model = %q, want gpt-4.1", ctx.Model.Model)
	}
	if ctx.Permission.ApprovalPolicy != "on-request" {
		t.Fatalf("approval policy = %q", ctx.Permission.ApprovalPolicy)
	}
}

func TestRuntimeStepContextOwnsModelInputProviderRequestAndToolSurface(t *testing.T) {
	turn := RuntimeTurnContext{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Profile:     RuntimePromptProfileHostWorker,
	}
	item := promptinput.ModelInputItem{
		ID:           "item-user",
		ProviderRole: promptinput.ProviderRoleUser,
		SemanticRole: "user_request",
		Content:      "check nginx",
	}
	step := RuntimeStepContext{
		Turn:      turn,
		Iteration: 2,
		Compiled:  promptcompiler.CompiledPrompt{},
		ModelInput: []promptinput.ModelInputItem{item},
		ToolSurface: RuntimeToolRouterSnapshot{
			RegisteredTools:   []string{"exec_command"},
			ModelVisibleTools: []string{"exec_command"},
			DispatchableTools: []string{"exec_command"},
			PolicyHash:        "policy-hash",
			Fingerprint:       "tool-fp",
		},
	}
	if err := step.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := step.ModelInput[0].ID; got != "item-user" {
		t.Fatalf("model input id = %q", got)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/runtimekernel -run 'TestBuildRuntimeTurnContextFreezesRequestMetadata|TestRuntimeStepContextOwnsModelInputProviderRequestAndToolSurface'
```

Expected: FAIL with undefined `BuildRuntimeTurnContext`, `RuntimeTurnContext`, `RuntimeStepContext`, or `RuntimeToolRouterSnapshot`.

- [x] **Step 3: Add runtime turn context types**

Create `internal/runtimekernel/runtime_turn_context.go`:

```go
package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/modelrouter"
)

type RuntimeRouteSnapshot struct {
	Route   string `json:"route,omitempty"`
	HostID  string `json:"hostId,omitempty"`
	Profile string `json:"profile,omitempty"`
}

type RuntimePermissionSnapshot struct {
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	PermissionHash string `json:"permissionHash,omitempty"`
}

type RuntimeContextBudgetSnapshot struct {
	MaxTokens    int `json:"maxTokens,omitempty"`
	TargetTokens int `json:"targetTokens,omitempty"`
}

type RuntimeLineageSnapshot struct {
	ParentSessionID string `json:"parentSessionId,omitempty"`
	ParentTurnID    string `json:"parentTurnId,omitempty"`
	AgentKind       string `json:"agentKind,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
}

type RuntimeTurnContext struct {
	SessionID       string                         `json:"sessionId"`
	TurnID          string                         `json:"turnId"`
	ClientTurnID    string                         `json:"clientTurnId,omitempty"`
	ClientMessageID string                         `json:"clientMessageId,omitempty"`
	SessionType     SessionType                    `json:"sessionType"`
	Mode            Mode                           `json:"mode"`
	Route           RuntimeRouteSnapshot           `json:"route"`
	Profile         string                         `json:"profile,omitempty"`
	HostID          string                         `json:"hostId,omitempty"`
	Model           modelrouter.ModelCapabilities  `json:"model"`
	Permission      RuntimePermissionSnapshot      `json:"permission"`
	ContextBudget   RuntimeContextBudgetSnapshot   `json:"contextBudget"`
	ToolPolicyHash  string                         `json:"toolPolicyHash,omitempty"`
	Lineage         RuntimeLineageSnapshot         `json:"lineage,omitempty"`
	Metadata        map[string]string              `json:"metadata,omitempty"`
}

type RuntimeTurnContextOptions struct {
	Model         modelrouter.ModelCapabilities
	ContextBudget RuntimeContextBudgetSnapshot
	ToolPolicyHash string
	Lineage        RuntimeLineageSnapshot
}

func BuildRuntimeTurnContext(req TurnRequest, session *SessionState, opts RuntimeTurnContextOptions) (RuntimeTurnContext, error) {
	if err := req.Validate(); err != nil {
		return RuntimeTurnContext{}, err
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return RuntimeTurnContext{}, fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(req.TurnID) == "" {
		return RuntimeTurnContext{}, fmt.Errorf("turn id is required")
	}
	metadata := copyRuntimeMetadata(req.Metadata)
	profile := firstMetadataValue(metadata, "profile", "toolProfile", "agentProfile")
	if profile == "" {
		profile = RuntimePromptProfileAdvisor
	}
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" && session != nil {
		hostID = strings.TrimSpace(session.HostID)
	}
	route := strings.TrimSpace(metadata["runtimeRoute"])
	if route == "" {
		route = string(req.SessionType)
	}
	return RuntimeTurnContext{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Route:           RuntimeRouteSnapshot{Route: route, HostID: hostID, Profile: profile},
		Profile:         profile,
		HostID:          hostID,
		Model:           opts.Model,
		Permission: RuntimePermissionSnapshot{
			ApprovalPolicy: strings.TrimSpace(metadata["approvalPolicy"]),
			PermissionHash: strings.TrimSpace(metadata["permissionHash"]),
		},
		ContextBudget:  opts.ContextBudget,
		ToolPolicyHash: strings.TrimSpace(opts.ToolPolicyHash),
		Lineage:        opts.Lineage,
		Metadata:       metadata,
	}, nil
}

func copyRuntimeMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
```

- [x] **Step 4: Add runtime step context types**

Create `internal/runtimekernel/runtime_step_context.go`:

```go
package runtimekernel

import (
	"fmt"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

type RuntimeStepContext struct {
	Turn            RuntimeTurnContext                  `json:"turn"`
	Iteration       int                                 `json:"iteration"`
	ContextState    ContextPipelineResult               `json:"contextState"`
	Compiled        promptcompiler.CompiledPrompt       `json:"compiled"`
	ModelInput      []promptinput.ModelInputItem        `json:"modelInput"`
	ToolSurface     RuntimeToolRouterSnapshot           `json:"toolSurface"`
	ProviderRequest modelrouter.ProviderRequestSnapshot `json:"providerRequest"`
}

func (s RuntimeStepContext) Validate() error {
	if s.Turn.SessionID == "" {
		return fmt.Errorf("turn session id is required")
	}
	if s.Turn.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if s.Iteration < 0 {
		return fmt.Errorf("iteration must be non-negative")
	}
	for i := range s.ModelInput {
		if err := s.ModelInput[i].Validate(); err != nil {
			return fmt.Errorf("model input[%d]: %w", i, err)
		}
	}
	return nil
}
```

- [x] **Step 5: Add temporary tool router snapshot type for step context**

Create `internal/runtimekernel/tool_router_snapshot.go`:

```go
package runtimekernel

type RuntimeToolRouterSnapshot struct {
	RegisteredTools   []string            `json:"registeredTools,omitempty"`
	ModelVisibleTools []string            `json:"modelVisibleTools,omitempty"`
	DispatchableTools []string            `json:"dispatchableTools,omitempty"`
	HiddenReasons     map[string][]string `json:"hiddenReasons,omitempty"`
	PolicyHash        string              `json:"policyHash,omitempty"`
	Fingerprint       string              `json:"fingerprint,omitempty"`
}
```

- [x] **Step 6: Run tests**

Run:

```bash
go test ./internal/runtimekernel -run 'TestBuildRuntimeTurnContextFreezesRequestMetadata|TestRuntimeStepContextOwnsModelInputProviderRequestAndToolSurface'
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/runtimekernel/runtime_turn_context.go internal/runtimekernel/runtime_step_context.go internal/runtimekernel/tool_router_snapshot.go internal/runtimekernel/runtime_context_test.go
git commit -m "feat(runtime): add runtime context snapshots"
```

---

### Task 2: Add ModelInputItem As The Only Prompt Input Fact Source

**Files:**
- Create: `internal/promptinput/model_input_item.go`
- Create: `internal/promptinput/model_input_validation.go`
- Modify: `internal/promptinput/types.go`
- Modify: `internal/promptinput/builder.go`
- Test: `internal/promptinput/model_input_item_test.go`
- Test: `internal/promptinput/builder_test.go`

- [x] **Step 1: Write failing tests for item validation and builder output**

Add `internal/promptinput/model_input_item_test.go`:

```go
package promptinput

import (
	"encoding/json"
	"testing"
)

func TestModelInputItemValidateRequiresProviderRole(t *testing.T) {
	item := ModelInputItem{ID: "item-1", Content: "hello"}
	if err := item.Validate(); err == nil {
		t.Fatal("Validate() succeeded, want provider role error")
	}
}

func TestModelInputItemValidateRequiresToolCallIDForToolResult(t *testing.T) {
	item := ModelInputItem{
		ID:           "tool-result",
		ProviderRole: ProviderRoleTool,
		ToolResult:   &ModelInputToolResult{Content: "ok"},
	}
	if err := item.Validate(); err == nil {
		t.Fatal("Validate() succeeded, want missing tool call id error")
	}
}

func TestModelInputItemHashIsStableForMetadataOrder(t *testing.T) {
	a := ModelInputItem{
		ID:           "item-1",
		ProviderRole: ProviderRoleUser,
		Content:      "hello",
		Metadata:     map[string]string{"b": "2", "a": "1"},
	}
	b := ModelInputItem{
		ID:           "item-1",
		ProviderRole: ProviderRoleUser,
		Content:      "hello",
		Metadata:     map[string]string{"a": "1", "b": "2"},
	}
	if a.StableHash() != b.StableHash() {
		t.Fatalf("hash differs: %s != %s", a.StableHash(), b.StableHash())
	}
}

func TestModelInputToolCallArgumentsMustBeJSON(t *testing.T) {
	item := ModelInputItem{
		ID:           "assistant-tool-call",
		ProviderRole: ProviderRoleAssistant,
		ToolCalls: []ModelInputToolCall{{
			ID:        "call-1",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"cmd":"date"}`),
		}},
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/promptinput -run 'TestModelInput'
```

Expected: FAIL with undefined `ModelInputItem`, `ProviderRoleUser`, `ModelInputToolCall`, or `StableHash`.

- [x] **Step 3: Add ModelInputItem types**

Create `internal/promptinput/model_input_item.go`:

```go
package promptinput

import "encoding/json"

type ProviderRole string

const (
	ProviderRoleSystem    ProviderRole = "system"
	ProviderRoleDeveloper ProviderRole = "developer"
	ProviderRoleUser      ProviderRole = "user"
	ProviderRoleAssistant ProviderRole = "assistant"
	ProviderRoleTool      ProviderRole = "tool"
)

type ModelInputSource struct {
	Layer     string `json:"layer,omitempty"`
	SectionID string `json:"sectionId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Origin    string `json:"origin,omitempty"`
}

type ModelInputContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ModelInputToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ModelInputToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content,omitempty"`
}

type ModelInputItem struct {
	ID           string                  `json:"id"`
	ProviderRole ProviderRole            `json:"providerRole"`
	SemanticRole string                  `json:"semanticRole,omitempty"`
	Content      string                  `json:"content,omitempty"`
	ContentParts []ModelInputContentPart `json:"contentParts,omitempty"`
	Name         string                  `json:"name,omitempty"`
	ToolCalls    []ModelInputToolCall    `json:"toolCalls,omitempty"`
	ToolCallID   string                  `json:"toolCallId,omitempty"`
	ToolResult   *ModelInputToolResult   `json:"toolResult,omitempty"`
	Source       ModelInputSource        `json:"source,omitempty"`
	Phase        string                  `json:"phase,omitempty"`
	CacheGroup   string                  `json:"cacheGroup,omitempty"`
	Metadata     map[string]string       `json:"metadata,omitempty"`
}
```

- [x] **Step 4: Add validation and hashing**

Create `internal/promptinput/model_input_validation.go`:

```go
package promptinput

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func (r ProviderRole) IsValid() bool {
	switch r {
	case ProviderRoleSystem, ProviderRoleDeveloper, ProviderRoleUser, ProviderRoleAssistant, ProviderRoleTool:
		return true
	default:
		return false
	}
}

func (i ModelInputItem) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if !i.ProviderRole.IsValid() {
		return fmt.Errorf("provider role %q is invalid", i.ProviderRole)
	}
	if i.ProviderRole == ProviderRoleTool {
		if strings.TrimSpace(i.ToolCallID) == "" && (i.ToolResult == nil || strings.TrimSpace(i.ToolResult.ToolCallID) == "") {
			return fmt.Errorf("tool result requires tool call id")
		}
	}
	for idx, call := range i.ToolCalls {
		if strings.TrimSpace(call.ID) == "" {
			return fmt.Errorf("tool call[%d] id is required", idx)
		}
		if strings.TrimSpace(call.Name) == "" {
			return fmt.Errorf("tool call[%d] name is required", idx)
		}
		if len(call.Arguments) > 0 && !json.Valid(call.Arguments) {
			return fmt.Errorf("tool call[%d] arguments must be valid json", idx)
		}
	}
	for idx, part := range i.ContentParts {
		if strings.TrimSpace(part.Type) == "" {
			return fmt.Errorf("content part[%d] type is required", idx)
		}
		if part.Type != "text" {
			return fmt.Errorf("content part[%d] type %q is unsupported", idx, part.Type)
		}
	}
	return nil
}

func (i ModelInputItem) StableHash() string {
	data, _ := json.Marshal(i)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func StableModelInputHash(items []ModelInputItem) string {
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
```

- [x] **Step 5: Change BuildResult to expose items instead of Eino messages**

Modify `internal/promptinput/types.go`:

```go
// BuildResult is the provider-neutral model input plus its explainable trace.
type BuildResult struct {
	Items []ModelInputItem
	Trace PromptInputTrace
}
```

Remove this import from `internal/promptinput/types.go`:

```go
"github.com/cloudwego/eino/schema"
```

- [x] **Step 6: Change builder to create ModelInputItem values**

Modify `internal/promptinput/builder.go` so `Build` returns `Items`:

```go
func (Builder) Build(req BuildRequest) (BuildResult, error) {
	promptItems := compiledPromptModelInputItems(req.Compiled)
	opsContextItems := opsContextModelInputItems(req)
	memoryItems := memoryModelInputItems(req)
	history := MessagesForCurrentTurnModelInput(req.History)
	runtimeItems, err := MessagesToModelInputItems(history)
	if err != nil {
		return BuildResult{}, fmt.Errorf("conversation messages: %w", err)
	}

	resultItems := make([]ModelInputItem, 0, len(promptItems)+len(opsContextItems)+len(memoryItems)+len(runtimeItems))
	resultItems = append(resultItems, promptItems...)
	resultItems = append(resultItems, opsContextItems...)
	resultItems = append(resultItems, memoryItems...)
	resultItems = append(resultItems, runtimeItems...)
	for i := range resultItems {
		if err := resultItems[i].Validate(); err != nil {
			return BuildResult{}, fmt.Errorf("model input item[%d]: %w", i, err)
		}
	}
	return BuildResult{
		Items: resultItems,
		Trace: buildTraceFromModelInputItems(req, resultItems, memoryMessagesFromRequest(req), history),
	}, nil
}
```

- [x] **Step 7: Add promptinput-local conversion helpers**

Add these helpers in `internal/promptinput/builder.go` or split to `internal/promptinput/model_input_builder.go`:

```go
func compiledPromptModelInputItems(compiled promptcompiler.CompiledPrompt) []ModelInputItem {
	sections := compiled.Envelope.Sections
	if len(sections) == 0 {
		sections = fallbackCompiledPromptSections(compiled)
	}
	out := make([]ModelInputItem, 0, len(sections))
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		role := ProviderRoleSystem
		if section.Role == "developer" {
			role = ProviderRoleDeveloper
		}
		out = append(out, ModelInputItem{
			ID:           firstNonBlankPromptInputString(section.ID, section.Layer, section.Source),
			ProviderRole: role,
			SemanticRole: firstNonBlankPromptInputString(section.Source, section.Layer, section.ID),
			Content:      content,
			Source: ModelInputSource{
				Layer:     section.Layer,
				SectionID: section.ID,
				Origin:    section.Source,
			},
			Phase:      "prompt",
			CacheGroup: section.Stability,
			Metadata: map[string]string{
				"prompt_section_id": section.ID,
				"prompt_layer":      section.Layer,
			},
		})
	}
	return out
}

func fallbackCompiledPromptSections(compiled promptcompiler.CompiledPrompt) []promptcompiler.PromptCompiledSection {
	out := make([]promptcompiler.PromptCompiledSection, 0, 5)
	if content := strings.TrimSpace(compiled.System.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "system", Layer: "system", Role: "system", Content: content, Stability: "stable", Source: "system"})
	}
	if content := strings.TrimSpace(compiled.Developer.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "developer", Layer: "developer", Role: "developer", Content: content, Stability: "stable", Source: "developer"})
	}
	if content := strings.TrimSpace(compiled.Tools.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "tool_index", Layer: "tool_index", Role: "system", Content: content, Stability: "stable", Source: "tool"})
	}
	if content := strings.TrimSpace(compiled.Dynamic.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "dynamic_prompt", Layer: "dynamic_prompt", Role: "system", Content: content, Stability: "dynamic", Source: "runtime_context"})
	}
	if content := strings.TrimSpace(compiled.Policy.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "runtime_policy", Layer: "runtime_policy", Role: "system", Content: content, Stability: "dynamic", Source: "context"})
	}
	return out
}

func opsContextModelInputItems(req BuildRequest) []ModelInputItem {
	capsule := strings.TrimSpace(req.OpsContextCapsule)
	if capsule == "" {
		return nil
	}
	return []ModelInputItem{{
		ID:           "ops-context-capsule",
		ProviderRole: ProviderRoleSystem,
		SemanticRole: "ops_context_capsule",
		Content:      "Ops context capsule:\n" + capsule,
		Source:       ModelInputSource{Layer: "ops_context_capsule", Origin: "runtime"},
		Phase:        "context",
		CacheGroup:   "dynamic",
	}}
}

func memoryModelInputItems(req BuildRequest) []ModelInputItem {
	memories := memoryMessagesFromRequest(req)
	out := make([]ModelInputItem, 0, len(memories))
	for _, item := range memories {
		out = append(out, ModelInputItem{
			ID:           "memory-" + item.ID,
			ProviderRole: ProviderRoleSystem,
			SemanticRole: "memory",
			Content:      "Memory: " + item.Text,
			Source:       ModelInputSource{Layer: "memory", MessageID: item.ID, Origin: item.Scope},
			Phase:        "memory",
			CacheGroup:   "dynamic",
			Metadata:     map[string]string{"memory_id": item.ID, "memory_scope": item.Scope},
		})
	}
	return out
}

func MessagesToModelInputItems(history []Message) ([]ModelInputItem, error) {
	out := make([]ModelInputItem, 0, len(history))
	for idx, msg := range history {
		item := ModelInputItem{
			ID:           fmt.Sprintf("history-%d", idx),
			ProviderRole: providerRoleFromConversationRole(msg.Role),
			SemanticRole: msg.Role,
			Content:      msg.Content,
			Source:       ModelInputSource{Layer: "history", Origin: msg.Role},
			Phase:        "history",
			CacheGroup:   "dynamic",
		}
		for _, call := range msg.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, ModelInputToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		if msg.ToolResult != nil {
			item.ProviderRole = ProviderRoleTool
			item.ToolCallID = msg.ToolResult.ToolCallID
			item.ToolResult = &ModelInputToolResult{ToolCallID: msg.ToolResult.ToolCallID, Content: msg.ToolResult.Content}
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func providerRoleFromConversationRole(role string) ProviderRole {
	switch strings.TrimSpace(role) {
	case "system":
		return ProviderRoleSystem
	case "assistant":
		return ProviderRoleAssistant
	case "tool":
		return ProviderRoleTool
	default:
		return ProviderRoleUser
	}
}

func buildTraceFromModelInputItems(req BuildRequest, items []ModelInputItem, memories []MemoryItem, history []Message) PromptInputTrace {
	trace := buildTrace(req, nil, memories, history, nil)
	trace.Items = make([]TraceItem, 0, len(items))
	for _, item := range items {
		trace.Items = append(trace.Items, TraceItem{
			ID:           item.ID,
			ProviderRole: string(item.ProviderRole),
			SemanticRole: item.SemanticRole,
			Content:      item.Content,
			CharCount:    len([]rune(item.Content)),
			PromptLayer:  item.Source.Layer,
		})
	}
	return trace
}

func firstNonBlankPromptInputString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "model-input-item"
}
```

- [x] **Step 8: Run promptinput tests**

Run:

```bash
go test ./internal/promptinput
```

Expected: PASS after updating existing tests to assert `result.Items` instead of `result.Messages`.

- [x] **Step 9: Commit**

```bash
git add internal/promptinput/model_input_item.go internal/promptinput/model_input_validation.go internal/promptinput/types.go internal/promptinput/builder.go internal/promptinput/*_test.go
git commit -m "feat(promptinput): make model input items the fact source"
```

---

### Task 3: Remove CompileForEino From Prompt Compiler

**Files:**
- Modify: `internal/promptcompiler/types.go`
- Modify: `internal/promptcompiler/compiler.go`
- Delete: `internal/promptcompiler/eino_format.go`
- Modify tests: `internal/promptcompiler/*_test.go`, `internal/agentmgr/factory_test.go`, `internal/runtimekernel/*_test.go`

- [x] **Step 1: Write interface guard test**

Add `internal/promptcompiler/compiler_interface_test.go`:

```go
package promptcompiler

import (
	"reflect"
	"testing"
)

func TestCompilerInterfaceExposesOnlyCompile(t *testing.T) {
	typ := reflect.TypeOf((*Compiler)(nil)).Elem()
	if _, ok := typ.MethodByName("Compile"); !ok {
		t.Fatal("Compiler missing Compile")
	}
	if _, ok := typ.MethodByName("CompileForEino"); ok {
		t.Fatal("Compiler must not expose CompileForEino")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/promptcompiler -run TestCompilerInterfaceExposesOnlyCompile
```

Expected: FAIL because `CompileForEino` still exists.

- [x] **Step 3: Remove CompileForEino from compiler interface**

Modify `internal/promptcompiler/types.go`:

```go
// Compiler is the unique prompt truth source. It compiles structured inputs
// into a section-first CompiledPrompt. Provider-specific message conversion
// belongs to modelrouter provider adapters.
type Compiler interface {
	Compile(ctx CompileContext) (CompiledPrompt, error)
}
```

- [x] **Step 4: Delete Eino format implementation**

Delete:

```bash
git rm internal/promptcompiler/eino_format.go
```

- [x] **Step 5: Replace tests that asserted CompileForEino**

For promptcompiler tests that verified section order through Eino messages, change them to assert `CompiledPrompt.Envelope.Sections` order:

```go
compiled, err := compiler.Compile(ctx)
if err != nil {
	t.Fatalf("Compile() error = %v", err)
}
got := make([]string, 0, len(compiled.Envelope.Sections))
for _, section := range compiled.Envelope.Sections {
	got = append(got, section.ID)
}
want := []string{"system", "developer", "tool_index", "runtime_policy"}
if !reflect.DeepEqual(got[:len(want)], want) {
	t.Fatalf("section order = %v, want prefix %v", got, want)
}
```

- [x] **Step 6: Run promptcompiler tests**

Run:

```bash
go test ./internal/promptcompiler
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/promptcompiler internal/agentmgr internal/runtimekernel
git commit -m "refactor(promptcompiler): remove eino compiler output"
```

---

### Task 4: Add ProviderRequestSnapshot And Unique Eino Message Adapter

**Files:**
- Create: `internal/modelrouter/provider_request_snapshot.go`
- Create: `internal/modelrouter/provider_response.go`
- Create: `internal/modelrouter/eino_message_adapter.go`
- Create: `internal/modelrouter/provider_adapter.go`
- Test: `internal/modelrouter/eino_message_adapter_test.go`
- Test: `internal/modelrouter/provider_request_snapshot_test.go`

- [x] **Step 1: Write failing adapter tests**

Add `internal/modelrouter/eino_message_adapter_test.go`:

```go
package modelrouter

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestModelInputItemsToEinoMessagesPreservesToolCalls(t *testing.T) {
	items := []promptinput.ModelInputItem{{
		ID:           "assistant-1",
		ProviderRole: promptinput.ProviderRoleAssistant,
		Content:      "I will inspect disk",
		ToolCalls: []promptinput.ModelInputToolCall{{
			ID:        "call-1",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"cmd":"df -h"}`),
		}},
	}}

	messages, audit, err := ModelInputItemsToEinoMessages(items)
	if err != nil {
		t.Fatalf("ModelInputItemsToEinoMessages() error = %v", err)
	}
	if len(messages) != 1 || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("tool calls not preserved: %#v", messages)
	}
	if audit.ProviderMessagesHash == "" || audit.Items[0].ItemID != "assistant-1" {
		t.Fatalf("audit missing hashes or item id: %#v", audit)
	}
}

func TestModelInputItemsToEinoMessagesRejectsInvalidItem(t *testing.T) {
	_, _, err := ModelInputItemsToEinoMessages([]promptinput.ModelInputItem{{ID: "bad"}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
```

- [x] **Step 2: Write failing snapshot hash tests**

Add `internal/modelrouter/provider_request_snapshot_test.go`:

```go
package modelrouter

import (
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestBuildProviderRequestSnapshotSeparatesStableCacheKeyFromDynamicIds(t *testing.T) {
	items := []promptinput.ModelInputItem{{
		ID:           "user-1",
		ProviderRole: promptinput.ProviderRoleUser,
		Content:      "check nginx",
		CacheGroup:   "turn-user",
	}}
	req := ProviderRequestSnapshot{
		Model: "gpt-4.1",
		Provider: "openai",
		Input: items,
		Tools: []ProviderToolSpec{{Name: "exec_command", Hash: "tool-hash"}},
		ReasoningEffort: "high",
		ClientMetadata: map[string]string{
			"turnId": "turn-1",
			"traceId": "trace-1",
		},
	}
	req.ComputeHashes()
	firstCacheKey := req.PromptCacheKey
	req.ClientMetadata["turnId"] = "turn-2"
	req.ClientMetadata["traceId"] = "trace-2"
	req.ComputeHashes()
	if req.PromptCacheKey != firstCacheKey {
		t.Fatalf("PromptCacheKey changed after dynamic id mutation: %q != %q", req.PromptCacheKey, firstCacheKey)
	}
}
```

- [x] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/modelrouter -run 'TestModelInputItemsToEinoMessages|TestBuildProviderRequestSnapshot'
```

Expected: FAIL with undefined provider snapshot and adapter types.

- [x] **Step 4: Add provider request snapshot types**

Create `internal/modelrouter/provider_request_snapshot.go`:

```go
package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"aiops-v2/internal/promptinput"
)

type ProviderToolSpec struct {
	Name string `json:"name"`
	Hash string `json:"hash,omitempty"`
}

type ProviderRequestSnapshot struct {
	Provider              string                         `json:"provider"`
	Model                 string                         `json:"model"`
	Input                 []promptinput.ModelInputItem   `json:"input"`
	Tools                 []ProviderToolSpec             `json:"tools,omitempty"`
	ReasoningEffort       string                         `json:"reasoningEffort,omitempty"`
	Temperature           float64                        `json:"temperature,omitempty"`
	TopP                  float64                        `json:"topP,omitempty"`
	MaxTokens             int                            `json:"maxTokens,omitempty"`
	ParallelToolCalls     bool                           `json:"parallelToolCalls,omitempty"`
	ClientMetadata        map[string]string              `json:"clientMetadata,omitempty"`
	ModelInputHash        string                         `json:"modelInputHash,omitempty"`
	ProviderMessagesHash  string                         `json:"providerMessagesHash,omitempty"`
	RequestPropertiesHash string                         `json:"requestPropertiesHash,omitempty"`
	PromptCacheKey        string                         `json:"promptCacheKey,omitempty"`
	MessageAudit          *ProviderMessageAudit          `json:"messageAudit,omitempty"`
}

func (r *ProviderRequestSnapshot) ComputeHashes() {
	r.ModelInputHash = stableHash(r.Input)
	r.RequestPropertiesHash = stableHash(map[string]any{
		"provider":          r.Provider,
		"model":             r.Model,
		"tools":             r.Tools,
		"reasoningEffort":   r.ReasoningEffort,
		"temperature":       r.Temperature,
		"topP":              r.TopP,
		"maxTokens":         r.MaxTokens,
		"parallelToolCalls": r.ParallelToolCalls,
	})
	r.PromptCacheKey = stableHash(map[string]any{
		"provider":        r.Provider,
		"model":           r.Model,
		"tools":           r.Tools,
		"reasoningEffort": r.ReasoningEffort,
		"cacheGroups":     cacheGroupsForProviderInput(r.Input),
	})
}

func cacheGroupsForProviderInput(items []promptinput.ModelInputItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.CacheGroup != "" {
			out = append(out, item.CacheGroup)
		}
	}
	return out
}

func stableHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
```

- [x] **Step 5: Add provider response types**

Create `internal/modelrouter/provider_response.go`:

```go
package modelrouter

import "time"

type ProviderUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

type ProviderStreamMetrics struct {
	FirstDeltaMs int `json:"firstDeltaMs,omitempty"`
	StreamMs     int `json:"streamMs,omitempty"`
	DeltaCount   int `json:"deltaCount,omitempty"`
	OutputChars  int `json:"outputChars,omitempty"`
}

type ProviderResponse struct {
	RequestID     string                `json:"requestId,omitempty"`
	Output        string                `json:"output,omitempty"`
	ToolCalls     []promptinputToolCall `json:"toolCalls,omitempty"`
	FinishReason  string                `json:"finishReason,omitempty"`
	Usage         ProviderUsage         `json:"usage,omitempty"`
	StreamMetrics ProviderStreamMetrics `json:"streamMetrics,omitempty"`
	StartedAt     time.Time             `json:"startedAt,omitempty"`
	FinishedAt    time.Time             `json:"finishedAt,omitempty"`
}

type promptinputToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}
```

- [x] **Step 6: Add Eino message adapter**

Create `internal/modelrouter/eino_message_adapter.go`:

```go
package modelrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/promptinput"
	"github.com/cloudwego/eino/schema"
)

type ProviderMessageAudit struct {
	ProviderMessagesHash string                     `json:"providerMessagesHash"`
	Items                []ProviderMessageAuditItem `json:"items"`
}

type ProviderMessageAuditItem struct {
	ItemID              string `json:"itemId"`
	ProviderRole        string `json:"providerRole"`
	ToolCallID          string `json:"toolCallId,omitempty"`
	ItemHash            string `json:"itemHash"`
	ProviderMessageHash string `json:"providerMessageHash"`
}

func ModelInputItemsToEinoMessages(items []promptinput.ModelInputItem) ([]*schema.Message, ProviderMessageAudit, error) {
	messages := make([]*schema.Message, 0, len(items))
	auditItems := make([]ProviderMessageAuditItem, 0, len(items))
	for idx, item := range items {
		if err := item.Validate(); err != nil {
			return nil, ProviderMessageAudit{}, fmt.Errorf("item[%d]: %w", idx, err)
		}
		msg := einoMessageFromModelInputItem(item)
		messages = append(messages, msg)
		auditItems = append(auditItems, ProviderMessageAuditItem{
			ItemID:              item.ID,
			ProviderRole:        string(item.ProviderRole),
			ToolCallID:          item.ToolCallID,
			ItemHash:            item.StableHash(),
			ProviderMessageHash: stableProviderHash(msg),
		})
	}
	return messages, ProviderMessageAudit{
		ProviderMessagesHash: stableProviderHash(messages),
		Items:                auditItems,
	}, nil
}

func einoMessageFromModelInputItem(item promptinput.ModelInputItem) *schema.Message {
	msg := &schema.Message{Content: item.Content}
	switch item.ProviderRole {
	case promptinput.ProviderRoleSystem, promptinput.ProviderRoleDeveloper:
		msg.Role = schema.System
	case promptinput.ProviderRoleUser:
		msg.Role = schema.User
	case promptinput.ProviderRoleAssistant:
		msg.Role = schema.Assistant
	case promptinput.ProviderRoleTool:
		msg.Role = schema.Tool
		msg.ToolCallID = firstNonEmptyString(item.ToolCallID, item.ToolResultToolCallID())
	}
	if item.Name != "" {
		msg.Name = item.Name
	}
	if len(item.ToolCalls) > 0 {
		msg.ToolCalls = make([]schema.ToolCall, 0, len(item.ToolCalls))
		for _, call := range item.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
				ID: call.ID,
				Function: schema.FunctionCall{
					Name:      call.Name,
					Arguments: string(call.Arguments),
				},
			})
		}
	}
	msg.Extra = map[string]any{
		"model_input_item_id": item.ID,
		"semantic_role":       item.SemanticRole,
		"source_layer":        item.Source.Layer,
		"source_section_id":   item.Source.SectionID,
		"phase":               item.Phase,
		"cache_group":         item.CacheGroup,
	}
	return msg
}

func stableProviderHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
```

Add this helper to `internal/promptinput/model_input_item.go`:

```go
func (i ModelInputItem) ToolResultToolCallID() string {
	if i.ToolResult == nil {
		return ""
	}
	return i.ToolResult.ToolCallID
}
```

- [x] **Step 7: Add provider adapter interface**

Create `internal/modelrouter/provider_adapter.go`:

```go
package modelrouter

import "context"

type ProviderAdapter interface {
	Call(ctx context.Context, req ProviderRequestSnapshot, onFinalDelta func(string), onReasoning func(ReasoningStreamEvent)) (ProviderResponse, error)
}
```

- [x] **Step 8: Run modelrouter tests**

Run:

```bash
go test ./internal/modelrouter -run 'TestModelInputItemsToEinoMessages|TestBuildProviderRequestSnapshot'
```

Expected: PASS.

- [x] **Step 9: Commit**

```bash
git add internal/modelrouter internal/promptinput/model_input_item.go
git commit -m "feat(modelrouter): add provider request snapshot and eino adapter"
```

---

### Task 5: Move Model Calling Out Of RuntimeKernel

**Files:**
- Modify: `internal/modelrouter/provider_adapter.go`
- Modify: `internal/runtimekernel/eino_kernel.go`
- Move tests: `internal/runtimekernel/model_response_test.go` to `internal/modelrouter/provider_adapter_test.go`

- [x] **Step 1: Write failing provider adapter stream test**

Add `internal/modelrouter/provider_adapter_test.go`:

```go
package modelrouter

import (
	"context"
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestEinoProviderAdapterCallsModelThroughSnapshot(t *testing.T) {
	adapter := NewEinoProviderAdapter(&recordingChatModel{})
	req := ProviderRequestSnapshot{
		Provider: "openai",
		Model: "gpt-4.1",
		Input: []promptinput.ModelInputItem{{
			ID:           "user-1",
			ProviderRole: promptinput.ProviderRoleUser,
			Content:      "hello",
		}},
	}
	resp, err := adapter.Call(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if resp.Output != "ok" {
		t.Fatalf("Output = %q, want ok", resp.Output)
	}
	if resp.RequestID == "" {
		t.Fatal("RequestID is required")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/modelrouter -run TestEinoProviderAdapterCallsModelThroughSnapshot
```

Expected: FAIL with undefined `NewEinoProviderAdapter`.

- [x] **Step 3: Implement EinoProviderAdapter**

Extend `internal/modelrouter/provider_adapter.go` with the existing `generateModelResponse` behavior moved from runtime:

```go
type EinoProviderAdapter struct {
	model ChatModel
}

func NewEinoProviderAdapter(model ChatModel) *EinoProviderAdapter {
	return &EinoProviderAdapter{model: model}
}

func (a *EinoProviderAdapter) Call(ctx context.Context, req ProviderRequestSnapshot, onFinalDelta func(string), onReasoning func(ReasoningStreamEvent)) (ProviderResponse, error) {
	if a == nil || a.model == nil {
		return ProviderResponse{}, fmt.Errorf("provider adapter model is required")
	}
	messages, audit, err := ModelInputItemsToEinoMessages(req.Input)
	if err != nil {
		return ProviderResponse{}, err
	}
	req.ProviderMessagesHash = audit.ProviderMessagesHash
	req.MessageAudit = &audit
	req.ComputeHashes()
	started := time.Now()
	response, err := generateEinoModelResponse(ctx, a.model, messages, req.Tools, onFinalDelta, onReasoning)
	if err != nil {
		return ProviderResponse{}, err
	}
	return ProviderResponse{
		RequestID:    req.ModelInputHash,
		Output:       response.Content,
		FinishReason: modelResponseFinishReason(response),
		Usage:        providerUsageFromEino(response),
		StartedAt:    started,
		FinishedAt:   time.Now(),
	}, nil
}
```

- [x] **Step 4: Delete runtime-owned generateModelResponse**

Move these functions from `internal/runtimekernel/eino_kernel.go` to `internal/modelrouter/provider_adapter.go` with provider-oriented names:

```text
generateModelResponse -> generateEinoModelResponse
modelResponseTimeout -> modelResponseTimeout
readableModelResponseTimeoutError -> readableModelResponseTimeoutError
attachConcatenatedResponseMeta -> attachConcatenatedResponseMeta
generateFallbackResponse -> generateFallbackResponse
isEmptyAssistantResponse -> isEmptyAssistantResponse
fallbackResponseFromToolEvidence -> fallbackResponseFromToolEvidence
```

- [x] **Step 5: Run provider adapter tests**

Run:

```bash
go test ./internal/modelrouter
```

Expected: PASS.

- [x] **Step 6: Run static check that runtime does not call provider directly**

Run:

```bash
rg -n "chatModel\\.Stream|chatModel\\.Generate|generateModelResponse" internal/runtimekernel
```

Expected: no matches.

- [x] **Step 7: Commit**

```bash
git add internal/modelrouter internal/runtimekernel
git commit -m "refactor(modelrouter): own provider model calls"
```

---

### Task 6: Build RuntimeStepContext In The Turn Loop

**Files:**
- Modify: `internal/runtimekernel/eino_kernel.go`
- Create: `internal/runtimekernel/turn_loop.go`
- Modify: `internal/runtimekernel/model_input.go`
- Test: `internal/runtimekernel/react_loop_test.go`
- Test: `internal/runtimekernel/runturn_hook_output_test.go`

- [x] **Step 1: Write failing test that model call receives ProviderRequestSnapshot**

Add to `internal/runtimekernel/react_loop_test.go`:

```go
func TestRunTurnBuildsProviderRequestFromRuntimeStepContext(t *testing.T) {
	kernel, recorder := newProviderRequestRecordingKernel(t)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		SessionID:   "session-step",
		TurnID:      "turn-step",
		Input:       "check nginx errors",
		HostID:      "host-a",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status == "" {
		t.Fatalf("missing status: %#v", result)
	}
	if recorder.lastRequest.ModelInputHash == "" {
		t.Fatal("provider request missing model input hash")
	}
	if len(recorder.lastRequest.Input) == 0 {
		t.Fatal("provider request missing model input items")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/runtimekernel -run TestRunTurnBuildsProviderRequestFromRuntimeStepContext
```

Expected: FAIL because runtime still calls Eino model path directly.

- [x] **Step 3: Add turn loop helper signatures**

Create `internal/runtimekernel/turn_loop.go`:

```go
package runtimekernel

import (
	"context"
	"fmt"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptinput"
)

func (k *RuntimeKernel) buildRuntimeStepContext(
	ctx context.Context,
	turn RuntimeTurnContext,
	session *SessionState,
	snapshot *TurnSnapshot,
	iteration int,
	contextMessages []Message,
	compiled promptcompiler.CompiledPrompt,
	toolSurface RuntimeToolRouterSnapshot,
) (RuntimeStepContext, error) {
	promptBuild, err := buildPromptInputWithContextGovernance(contextMessages, compiled, append([]ContextGovernanceEvent(nil), session.ContextGovernanceEvents...))
	if err != nil {
		return RuntimeStepContext{}, err
	}
	providerReq := modelrouter.ProviderRequestSnapshot{
		Provider: turn.Model.Provider,
		Model: turn.Model.Model,
		Input: promptBuild.Items,
		Tools: providerToolsFromRuntimeToolSurface(toolSurface),
		ReasoningEffort: firstMetadataValue(turn.Metadata, "reasoningEffort", "reasoning_effort"),
		ClientMetadata: map[string]string{
			"sessionId": turn.SessionID,
			"turnId": turn.TurnID,
			"clientTurnId": turn.ClientTurnID,
			"clientMessageId": turn.ClientMessageID,
		},
	}
	providerReq.ComputeHashes()
	step := RuntimeStepContext{
		Turn: turn,
		Iteration: iteration,
		Compiled: compiled,
		ModelInput: promptBuild.Items,
		ToolSurface: toolSurface,
		ProviderRequest: providerReq,
	}
	if err := step.Validate(); err != nil {
		return RuntimeStepContext{}, fmt.Errorf("runtime step context: %w", err)
	}
	return step, nil
}

func providerToolsFromRuntimeToolSurface(surface RuntimeToolRouterSnapshot) []modelrouter.ProviderToolSpec {
	out := make([]modelrouter.ProviderToolSpec, 0, len(surface.ModelVisibleTools))
	for _, name := range surface.ModelVisibleTools {
		out = append(out, modelrouter.ProviderToolSpec{Name: name, Hash: surface.Fingerprint})
	}
	return out
}
```

- [x] **Step 4: Replace model input build and provider call in turn loop**

In the turn loop, replace:

```go
promptBuild, modelErr := buildPromptInputWithContextGovernance(...)
modelInput := promptBuild.Messages
response, genErr := generateModelResponse(...)
```

with:

```go
turnCtx, err := BuildRuntimeTurnContext(req, session, RuntimeTurnContextOptions{
	Model: modelCapabilitiesForRuntime(chatModel, agentKind),
	ContextBudget: RuntimeContextBudgetSnapshot{
		MaxTokens: thresholds.MaxTokens,
		TargetTokens: thresholds.TargetTokens,
	},
	ToolPolicyHash: surfacePolicy.Hash,
})
if err != nil {
	return "", nil, err
}
stepCtx, err := k.buildRuntimeStepContext(ctx, turnCtx, session, snapshot, iteration, contextMessages, compiled, runtimeToolSurfaceSnapshot)
if err != nil {
	return "", nil, err
}
providerResp, genErr := k.providerAdapter.Call(modelCtx, stepCtx.ProviderRequest, onFinalDelta, onReasoning)
```

- [x] **Step 5: Update trace writer call to consume step context**

Change model input trace call from field list to:

```go
tracePath, _ := k.writeRuntimeStepTrace(stepCtx, RuntimeStepTraceOptions{
	PromptInputDiff: promptInputDiff,
	DiagnosticTrace: buildRuntimeDiagnosticTrace(turnID, session, req, compileCtx),
	FinalEvidenceState: &finalEvidenceTrace,
})
```

- [x] **Step 6: Run runtime tests**

Run:

```bash
go test ./internal/runtimekernel -run 'TestRunTurnBuildsProviderRequestFromRuntimeStepContext|TestRunTurn|TestReact'
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/runtimekernel
git commit -m "refactor(runtime): drive model calls from step context"
```

---

### Task 7: Rename EinoKernel To RuntimeKernel Without Alias

**Files:**
- Create: `internal/runtimekernel/runtime_kernel.go`
- Delete: `internal/runtimekernel/eino_kernel.go`
- Modify: all `NewEinoKernel`, `EinoKernel`, `EinoKernelConfig` references
- Test: `internal/runtimekernel/*_test.go`, `internal/appui/*_test.go`, `internal/agentmgr/*_test.go`
- Modify: `internal/eval/mock_agent.go`, `internal/eval/scorer_runner_test.go`, `testdata/eval_cases/*.json` path expectations

- [x] **Step 1: Run reference scan before rename**

Run:

```bash
rg -n "EinoKernel|NewEinoKernel|EinoKernelConfig" internal
```

Expected: shows production and test references to rename.

- [x] **Step 2: Rename public type and constructor**

Create `internal/runtimekernel/runtime_kernel.go`:

```go
package runtimekernel

// RuntimeKernel is the unique aiops-v2 agent runtime owner. Eino is only used
// behind provider and tool adapters.
type RuntimeKernel struct {
	tools            ToolAssemblySource
	compiler         promptcompiler.Compiler
	policy           *policyengine.Engine
	permissions      *permissions.Engine
	hooks            *hooks.Registry
	projector        EventEmitter
	modelRouter      *modelrouter.Router
	providerAdapter  modelrouter.ProviderAdapter
	sessions         *SessionManager
	agentMgr         AgentManagerSource
	spanSource       SpanStreamSource
	observer         Observer
	resourceLockGate ToolResourceLockGate
	compressor       *spanstream.ContextCompressor
	spillRepo        ToolResultSpillRepository
	artifactRepo     ContextArtifactRepository
	skillRegistry    *skills.Registry
	evidenceService  *evidencecore.Service

	turnCancelMu       sync.Mutex
	inFlightTurnCancel map[string]context.CancelFunc
	pendingTurnCancel  map[string]string
}

type RuntimeKernelConfig struct {
	ToolSource       ToolAssemblySource
	Compiler         promptcompiler.Compiler
	Policy           *policyengine.Engine
	Permissions      *permissions.Engine
	Hooks            *hooks.Registry
	Projector        EventEmitter
	ModelRouter      *modelrouter.Router
	ProviderAdapter  modelrouter.ProviderAdapter
	AgentMgr         AgentManagerSource
	Sessions         *SessionManager
	SessionRepo      SessionRepository
	SpanSource       SpanStreamSource
	Observer         Observer
	ResourceLockGate ToolResourceLockGate
	Compressor       *spanstream.ContextCompressor
	SpillRepo        ToolResultSpillRepository
	ArtifactRepo     ContextArtifactRepository
	SkillRegistry    *skills.Registry
	EvidenceService  *evidencecore.Service
}
```

- [x] **Step 3: Rename constructor**

Replace constructor with:

```go
func NewRuntimeKernel(cfg RuntimeKernelConfig) *RuntimeKernel {
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = NewSessionManager(cfg.SessionRepo)
	}
	observer := cfg.Observer
	if observer == nil {
		observer = NoopObserver{}
	}
	return &RuntimeKernel{
		tools:              cfg.ToolSource,
		compiler:           cfg.Compiler,
		policy:             cfg.Policy,
		permissions:        cfg.Permissions,
		hooks:              cfg.Hooks,
		projector:          cfg.Projector,
		modelRouter:        cfg.ModelRouter,
		providerAdapter:    cfg.ProviderAdapter,
		sessions:           sessions,
		agentMgr:           cfg.AgentMgr,
		spanSource:         cfg.SpanSource,
		observer:           observer,
		resourceLockGate:   cfg.ResourceLockGate,
		compressor:         cfg.Compressor,
		spillRepo:          cfg.SpillRepo,
		artifactRepo:       cfg.ArtifactRepo,
		skillRegistry:      cfg.SkillRegistry,
		evidenceService:    cfg.EvidenceService,
		inFlightTurnCancel: make(map[string]context.CancelFunc),
		pendingTurnCancel:  make(map[string]string),
	}
}
```

- [x] **Step 4: Rename all method receivers**

Run non-interactive replacements carefully:

```bash
perl -pi -e 's/\\bEinoKernelConfig\\b/RuntimeKernelConfig/g; s/\\bNewEinoKernel\\b/NewRuntimeKernel/g; s/\\bEinoKernel\\b/RuntimeKernel/g' internal/runtimekernel/*.go internal/appui/*.go internal/agentmgr/*.go
```

Expected: source now uses `RuntimeKernel`.

- [x] **Step 5: Verify no alias exists**

Run:

```bash
rg -n "type EinoKernel|NewEinoKernel|EinoKernelConfig|type RuntimeKernel = EinoKernel" internal
```

Expected: no matches.

- [x] **Step 6: Run impacted tests**

Run:

```bash
go test ./internal/runtimekernel ./internal/appui ./internal/agentmgr
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/runtimekernel internal/appui internal/agentmgr
git commit -m "refactor(runtime): rename eino kernel to runtime kernel"
```

---

### Task 8: Make ToolRouterSnapshot The Shared Visible And Dispatchable Source

**Files:**
- Modify: `internal/tooling/surface_policy.go`
- Modify: `internal/runtimekernel/tool_router_snapshot.go`
- Modify: `internal/runtimekernel/dispatch.go`
- Modify: `internal/runtimekernel/model_input_tool_trace.go`
- Test: `internal/tooling/surface_dispatch_consistency_test.go`
- Test: `internal/runtimekernel/dispatch_test.go`

- [x] **Step 1: Write failing visible/dispatchable consistency test**

Add to `internal/runtimekernel/dispatch_test.go`:

```go
func TestDispatcherRejectsToolNotInRuntimeStepDispatchableTools(t *testing.T) {
	dispatcher := NewToolDispatcher(
		[]ToolDescriptor{{Name: "exec_command"}},
		map[string]ToolExecutor{"exec_command": &captureToolExecutor{}},
		nil,
		nil,
	)
	dispatcher = dispatcher.WithRuntimeToolRouterSnapshot(RuntimeToolRouterSnapshot{
		RegisteredTools:   []string{"exec_command"},
		ModelVisibleTools: []string{"exec_command"},
		DispatchableTools: []string{},
		HiddenReasons: map[string][]string{
			"exec_command": {"profile_denied"},
		},
		PolicyHash: "policy-1",
	})
	result := dispatcher.Dispatch(context.Background(), "session-1", "turn-1", ToolCall{
		ID: "call-1",
		Name: "exec_command",
		Arguments: json.RawMessage(`{"cmd":"date"}`),
	}, SessionTypeHost, ModeChat)
	if !result.Blocked || !strings.Contains(result.Content, "tool_unavailable") {
		t.Fatalf("result = %#v, want blocked unavailable", result)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/runtimekernel -run TestDispatcherRejectsToolNotInRuntimeStepDispatchableTools
```

Expected: FAIL with undefined `WithRuntimeToolRouterSnapshot`.

- [x] **Step 3: Add dispatcher snapshot field and method**

Modify `internal/runtimekernel/dispatch.go`:

```go
func (d *ToolDispatcher) WithRuntimeToolRouterSnapshot(snapshot RuntimeToolRouterSnapshot) *ToolDispatcher {
	cp := RuntimeToolRouterSnapshot{
		RegisteredTools: append([]string(nil), snapshot.RegisteredTools...),
		ModelVisibleTools: append([]string(nil), snapshot.ModelVisibleTools...),
		DispatchableTools: append([]string(nil), snapshot.DispatchableTools...),
		HiddenReasons: copyHiddenReasons(snapshot.HiddenReasons),
		PolicyHash: snapshot.PolicyHash,
		Fingerprint: snapshot.Fingerprint,
	}
	d.runtimeToolSurface = &cp
	return d
}
```

- [x] **Step 4: Enforce dispatchable membership**

In `dispatch`, before executing a tool:

```go
if d.runtimeToolSurface != nil && !runtimeToolSurfaceContains(d.runtimeToolSurface.DispatchableTools, tc.Name) {
	reasons := d.runtimeToolSurface.HiddenReasons[tc.Name]
	reason := "tool_not_dispatchable"
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	return d.hiddenToolUnavailableResult(tc, tooling.ToolHiddenReason{Name: tc.Name, Reason: reason}, nil, meta)
}
```

- [x] **Step 5: Build RuntimeToolRouterSnapshot from tooling output**

Add conversion in `internal/runtimekernel/tool_router_snapshot.go`:

```go
func RuntimeToolRouterSnapshotFromPolicy(registered []string, policy tooling.ToolSurfacePolicySnapshot, visible []string, dispatchable []string, fingerprint string) RuntimeToolRouterSnapshot {
	return RuntimeToolRouterSnapshot{
		RegisteredTools: registered,
		ModelVisibleTools: visible,
		DispatchableTools: dispatchable,
		HiddenReasons: hiddenReasonsFromToolSurfacePolicySnapshot(policy),
		PolicyHash: policy.Hash,
		Fingerprint: fingerprint,
	}
}
```

- [x] **Step 6: Run tool tests**

Run:

```bash
go test ./internal/tooling ./internal/runtimekernel -run 'ToolSurface|Dispatch'
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/tooling internal/runtimekernel
git commit -m "refactor(runtime): unify visible and dispatchable tools"
```

---

### Task 9: Replace Trace v1 With TraceDocumentV2

**Files:**
- Create: `internal/modeltrace/trace_v2.go`
- Create: `internal/modeltrace/raw_payload.go`
- Modify: `internal/runtimekernel/model_input.go`
- Modify: `internal/promptdiag/*`
- Test: `internal/modeltrace/trace_v2_test.go`
- Test: `internal/runtimekernel/model_input_trace_test.go`
- Test: `internal/promptdiag/*_test.go`

- [x] **Step 1: Write failing Trace v2 writer test**

Add `internal/modeltrace/trace_v2_test.go`:

```go
package modeltrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTraceDocumentV2WritesSummaryAndRawRefs(t *testing.T) {
	dir := t.TempDir()
	doc := TraceDocumentV2{
		SchemaVersion: "aiops.trace/v2",
		SessionID: "session-1",
		TurnID: "turn-1",
		Iteration: 0,
		ProviderRequest: ProviderRequestTrace{
			ModelInputHash: "mih",
			PromptCacheKey: "cache",
		},
		RawPayloadRefs: []RawPayloadRef{{
			ID: "raw-request",
			Kind: "provider_request",
			Path: "raw/raw-request.json",
			Sha256: "abc",
		}},
	}
	path, err := WriteTraceDocumentV2(dir, doc)
	if err != nil {
		t.Fatalf("WriteTraceDocumentV2() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got TraceDocumentV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.SchemaVersion != "aiops.trace/v2" {
		t.Fatalf("schema version = %q", got.SchemaVersion)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		t.Fatalf("index.json missing: %v", err)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/modeltrace -run TestWriteTraceDocumentV2WritesSummaryAndRawRefs
```

Expected: FAIL with undefined `TraceDocumentV2`.

- [x] **Step 3: Add Trace v2 types and writer**

Create `internal/modeltrace/trace_v2.go`:

```go
package modeltrace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TraceDocumentV2 struct {
	SchemaVersion string               `json:"schemaVersion"`
	CreatedAt     string               `json:"createdAt"`
	SessionID     string               `json:"sessionId"`
	TurnID        string               `json:"turnId"`
	Iteration     int                  `json:"iteration"`
	TurnContext   any                  `json:"turnContext,omitempty"`
	StepContext   any                  `json:"stepContext,omitempty"`
	ProviderRequest ProviderRequestTrace `json:"providerRequest,omitempty"`
	ToolSurface   any                  `json:"toolSurface,omitempty"`
	RawPayloadRefs []RawPayloadRef      `json:"rawPayloadRefs,omitempty"`
}

type ProviderRequestTrace struct {
	ModelInputHash        string `json:"modelInputHash,omitempty"`
	ProviderMessagesHash string `json:"providerMessagesHash,omitempty"`
	RequestPropertiesHash string `json:"requestPropertiesHash,omitempty"`
	PromptCacheKey       string `json:"promptCacheKey,omitempty"`
}

func WriteTraceDocumentV2(root string, doc TraceDocumentV2) (string, error) {
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = "aiops.trace/v2"
	}
	if doc.CreatedAt == "" {
		doc.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s-iteration-%d.json", doc.SessionID, doc.TurnID, doc.Iteration)
	path := filepath.Join(root, name)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	indexData, _ := json.MarshalIndent([]map[string]any{{
		"path": path,
		"sessionId": doc.SessionID,
		"turnId": doc.TurnID,
		"iteration": doc.Iteration,
		"schemaVersion": doc.SchemaVersion,
	}}, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "index.json"), indexData, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
```

- [x] **Step 4: Add raw payload refs**

Create `internal/modeltrace/raw_payload.go`:

```go
package modeltrace

type RawPayloadRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Sha256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
}
```

- [x] **Step 5: Replace writeModelInputDebugTrace production call**

In runtime trace path, replace `writeModelInputDebugTrace(ModelInputDebugTraceRequest{...})` with:

```go
tracePath, err := modeltrace.WriteTraceDocumentV2(traceRootDir(), modeltrace.TraceDocumentV2{
	SessionID: stepCtx.Turn.SessionID,
	TurnID: stepCtx.Turn.TurnID,
	Iteration: stepCtx.Iteration,
	TurnContext: stepCtx.Turn,
	StepContext: stepCtx,
	ProviderRequest: modeltrace.ProviderRequestTrace{
		ModelInputHash: stepCtx.ProviderRequest.ModelInputHash,
		ProviderMessagesHash: stepCtx.ProviderRequest.ProviderMessagesHash,
		RequestPropertiesHash: stepCtx.ProviderRequest.RequestPropertiesHash,
		PromptCacheKey: stepCtx.ProviderRequest.PromptCacheKey,
	},
	ToolSurface: stepCtx.ToolSurface,
	RawPayloadRefs: rawPayloadRefs,
})
if err != nil {
	return "", nil, err
}
```

- [x] **Step 6: Remove v1 writer from production imports**

Run:

```bash
rg -n "writeModelInputDebugTrace|ModelInputDebugTraceRequest|modeltrace\\.Request" internal/runtimekernel internal/promptdiag internal/eval
```

Expected: no production matches after migration.

- [x] **Step 7: Run trace and promptdiag tests**

Run:

```bash
go test ./internal/modeltrace ./internal/runtimekernel ./internal/promptdiag
```

Expected: PASS.

- [x] **Step 8: Commit**

```bash
git add internal/modeltrace internal/runtimekernel internal/promptdiag
git commit -m "feat(trace): replace model input trace with trace v2"
```

---

### Task 10: Switch PromptTrace UI To Trace v2 Only

**Files:**
- Modify: `web/src/pages/PromptTracePage.tsx`
- Modify/Create: `web/src/utils/promptTraceViewModel.ts`
- Test: `web/src/pages/PromptTracePage.test.tsx`

- [x] **Step 1: Write failing v2 parser test**

Add or modify `web/src/utils/promptTraceViewModel.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { parsePromptTrace } from "./promptTraceViewModel";

describe("parsePromptTrace v2", () => {
  it("reads provider request and raw payload refs from trace v2", () => {
    const vm = parsePromptTrace(JSON.stringify({
      schemaVersion: "aiops.trace/v2",
      sessionId: "session-1",
      turnId: "turn-1",
      iteration: 0,
      providerRequest: {
        modelInputHash: "mih",
        providerMessagesHash: "pmh",
        requestPropertiesHash: "rph",
        promptCacheKey: "cache"
      },
      toolSurface: {
        modelVisibleTools: ["exec_command"],
        dispatchableTools: ["exec_command"],
        hiddenReasons: {}
      },
      rawPayloadRefs: [{ id: "raw-request", kind: "provider_request", path: "raw/raw-request.json" }]
    }));

    expect(vm.summary.schemaVersion).toBe("aiops.trace/v2");
    expect(vm.providerRequest.modelInputHash).toBe("mih");
    expect(vm.rawPayloadRefs).toHaveLength(1);
    expect(vm.toolSurface.summary.visibleCount).toBe(1);
  });
});
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
npm test -- --run web/src/utils/promptTraceViewModel.test.ts
```

Expected: FAIL because parser does not expose v2 fields.

- [x] **Step 3: Update parser to v2 view model**

Implement v2 output shape:

```ts
export function parsePromptTrace(raw: string) {
  const payload = JSON.parse(raw || "{}");
  const providerRequest = payload.providerRequest || {};
  const toolSurface = payload.toolSurface || {};
  const rawPayloadRefs = Array.isArray(payload.rawPayloadRefs) ? payload.rawPayloadRefs : [];
  return {
    summary: {
      schemaVersion: payload.schemaVersion || "",
      sessionId: payload.sessionId || "",
      turnId: payload.turnId || "",
      iteration: Number(payload.iteration) || 0,
      visibleToolCount: (toolSurface.modelVisibleTools || []).length,
      messageCount: (payload.stepContext?.modelInput || []).length,
      promptCharCount: JSON.stringify(payload.stepContext?.modelInput || []).length,
      toolRegistryCharCount: JSON.stringify(toolSurface).length
    },
    providerRequest,
    rawPayloadRefs,
    toolSurface: {
      summary: {
        visibleCount: (toolSurface.modelVisibleTools || []).length,
        dispatchableCount: (toolSurface.dispatchableTools || []).length,
        hiddenCount: Object.keys(toolSurface.hiddenReasons || {}).length
      },
      visible: toolSurface.modelVisibleTools || [],
      dispatchable: toolSurface.dispatchableTools || [],
      hiddenReasons: toolSurface.hiddenReasons || {}
    },
    messages: payload.stepContext?.modelInput || [],
    layers: [],
    tools: {
      visible: toolSurface.modelVisibleTools || [],
      registryText: JSON.stringify(toolSurface, null, 2)
    },
    contextGovernance: payload.stepContext?.contextState?.governanceEvents || null,
    agentUiSources: payload.agentUiSources || null
  };
}
```

- [x] **Step 4: Update PromptTracePage labels and panels**

In `web/src/pages/PromptTracePage.tsx`, add overview badges for:

```tsx
{traceViewModel?.providerRequest?.modelInputHash ? <ToneBadge>输入 {shortHash(traceViewModel.providerRequest.modelInputHash)}</ToneBadge> : null}
{traceViewModel?.providerRequest?.providerMessagesHash ? <ToneBadge>Provider {shortHash(traceViewModel.providerRequest.providerMessagesHash)}</ToneBadge> : null}
{traceViewModel?.providerRequest?.promptCacheKey ? <ToneBadge>Cache {shortHash(traceViewModel.providerRequest.promptCacheKey)}</ToneBadge> : null}
```

Add raw refs panel:

```tsx
<ContextPanel title="Raw Payload Refs">
  <div className="grid gap-2">
    {(traceViewModel?.rawPayloadRefs || []).map((ref) => (
      <div key={ref.id} className="rounded-lg border border-slate-100 bg-white p-2 text-xs">
        <ToneBadge>{ref.kind}</ToneBadge>
        <span className="ml-2 font-mono text-slate-600">{ref.path}</span>
      </div>
    ))}
  </div>
</ContextPanel>
```

- [x] **Step 5: Run UI tests**

Run:

```bash
npm test -- --run web/src/utils/promptTraceViewModel.test.ts web/src/pages/PromptTracePage.test.tsx
```

Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add web/src/utils web/src/pages/PromptTracePage.tsx web/src/pages/PromptTracePage.test.tsx
git commit -m "feat(ui): read prompt trace v2 only"
```

---

### Task 11: Add Static Guards For Single Runtime Logic

**Files:**
- Create: `scripts/verify-agent-runtime-single-path.sh`
- Modify: `package.json` or `Makefile` if this repo already centralizes checks there

- [x] **Step 1: Create static guard script**

Create `scripts/verify-agent-runtime-single-path.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

if rg -n "CompileForEino|BuildResult\\s*\\{[^}]*Messages|ModelInput\\s+\\[\\]\\*schema\\.Message|type\\s+EinoKernel|NewEinoKernel|EinoKernelConfig|type\\s+RuntimeKernel\\s*=\\s*EinoKernel" internal; then
  fail "old runtime, prompt, or trace symbols remain"
fi

if rg -n "chatModel\\.Stream|chatModel\\.Generate|generateModelResponse" internal/runtimekernel; then
  fail "runtimekernel still calls provider model directly"
fi

schema_hits="$(rg -n 'github.com/cloudwego/eino/schema' internal/promptcompiler internal/promptinput internal/modeltrace internal/runtimekernel || true)"
if [ -n "$schema_hits" ]; then
  echo "$schema_hits" >&2
  fail "Eino schema import leaked outside adapter whitelist"
fi

if rg -n "writeModelInputDebugTrace|ModelInputDebugTraceRequest|modeltrace\\.Request" internal/runtimekernel internal/promptdiag internal/eval; then
  fail "v1 trace production path remains"
fi

echo "PASS: single runtime path guards"
```

- [x] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/verify-agent-runtime-single-path.sh
```

Expected: command exits 0.

- [x] **Step 3: Run static guard**

Run:

```bash
./scripts/verify-agent-runtime-single-path.sh
```

Expected: PASS after all previous tasks are complete.

- [x] **Step 4: Commit**

```bash
git add scripts/verify-agent-runtime-single-path.sh
git commit -m "test(runtime): guard single runtime path"
```

---

### Task 12: Final Integration And Regression Verification

**Files:**
- Verify only unless failures require fixes in prior task files.

- [x] **Step 1: Run focused Go packages**

Run:

```bash
go test ./internal/runtimekernel ./internal/promptinput ./internal/promptcompiler ./internal/modelrouter ./internal/modeltrace ./internal/appui ./internal/agentmgr ./internal/promptdiag
```

Expected: PASS.

- [x] **Step 2: Run frontend tests for PromptTrace**

Run:

```bash
npm test -- --run web/src/utils/promptTraceViewModel.test.ts web/src/pages/PromptTracePage.test.tsx
```

Expected: PASS.

- [x] **Step 3: Run static guard**

Run:

```bash
./scripts/verify-agent-runtime-single-path.sh
```

Expected:

```text
PASS: single runtime path guards
```

- [x] **Step 4: Run full Go tests if focused tests pass**

Run:

```bash
go test ./...
```

Expected: PASS.

- [x] **Step 5: Run app smoke path**

Run:

```bash
npm run build
```

Expected: PASS, frontend build completes.

- [x] **Step 6: Manual trace inspection**

Run one chat turn with model trace enabled, then inspect generated Trace v2 JSON. Required observations:

- `schemaVersion` equals `aiops.trace/v2`.
- `providerRequest.modelInputHash` is non-empty.
- `providerRequest.providerMessagesHash` is non-empty.
- `providerRequest.promptCacheKey` is non-empty.
- `stepContext.modelInput` contains structured items, not Eino schema messages.
- `toolSurface.modelVisibleTools` and `toolSurface.dispatchableTools` are both present.
- `rawPayloadRefs` exists when raw provider request/response or tool result is externalized.

- [x] **Step 7: Final commit**

```bash
git add internal web scripts docs/superpowers/plans/2026-06-26-aiops-v2-agent-runtime-layering-implementation-todo.zh.md
git commit -m "feat(runtime): switch to single layered agent runtime"
```

---

## Final Acceptance Checklist

- [x] `RuntimeKernel` is the only runtime kernel type.
- [x] `EinoKernel`, `NewEinoKernel`, and `EinoKernelConfig` do not exist.
- [x] `CompileForEino` does not exist.
- [x] `promptinput.BuildResult.Messages` does not exist.
- [x] `modeltrace.Request.ModelInput []*schema.Message` does not exist.
- [x] `runtimekernel` does not call `chatModel.Stream` or `chatModel.Generate`.
- [x] `promptcompiler`, `promptinput`, `modeltrace`, and `runtimekernel` do not import `github.com/cloudwego/eino/schema`.
- [x] `ModelInputItem` is the fact source for promptinput, provider request, Trace v2, and promptdiag.
- [x] `ProviderRequestSnapshot` records `modelInputHash`, `providerMessagesHash`, `requestPropertiesHash`, and `promptCacheKey`.
- [x] `promptCacheKey` includes model-visible input content and still ignores dynamic client/item ids.
- [x] Production code does not call legacy `modeltrace.Write`; non-runtime diagnostic traces write `aiops.trace/v2`.
- [x] Model response trace entries use provider request identity, not UI item identity, as the primary request id.
- [x] `ProviderMessageAudit` can map every provider message back to a `ModelInputItem`.
- [x] Tool visible and dispatchable lists come from one `RuntimeToolRouterSnapshot`.
- [x] Trace v2 does not reconstruct prompt or provider request from markdown.
- [x] PromptTrace UI reads v2 index/document only.
- [x] `preview` never flows into prompt, provider request, or runtime state.
- [x] `go test ./...` passes.
- [x] `npm run build` passes.
- [x] `./scripts/verify-agent-runtime-single-path.sh` passes.

## Self-Review Notes

- Spec coverage: tasks cover final types, promptinput fact source, provider adapter, turn loop, kernel rename, tool router snapshot, Trace v2, PromptTrace UI, static guards, and regression verification.
- Completed `schema.Message` boundary: production `promptcompiler`/`promptinput`/`runtimekernel`/`modeltrace` no longer import or expose Eino schema DTOs; Eino conversion is confined to `modelrouter`/Eino-facing packages and tests.
- Completed `preview` boundary: persisted runtime state no longer stores `outputPreview`; model-visible resource/artifact snippets use `ContentSnippet`, and UI previews are derived in appui projection or carried only by live UI event payloads.
- Ambiguity resolved: no compatibility mode, no feature flag, no old runtime alias, no v1/v2 trace dual write.
- Highest-risk area covered: `ModelInputItem -> schema.Message` uses strict validation, golden parity, round-trip checks, provider message audit, and hash verification.
- Code-review fixes completed: static guard now scans all production `modeltrace.Write` calls; opsmanual/spanstream trace writes use Trace v2; `promptCacheKey` changes when model-visible content changes; response trace id is tied to `ProviderResponse.RequestID`/`modelInputHash`.
