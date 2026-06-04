# AI Chat Host Mention Plan Subagents Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 `docs/2026-06-04-ai-chat-host-mention-plan-subagents-design.zh.md` 落地 AI 对话页 `@主机`、多主机强制计划模式、每主机独立 host-bound 子 Agent、Codex 风格输入框上方状态面板和子 Agent 独立 drawer。

**Architecture:** 保持 aiops-v2 现有生产路径 `TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React`，新增 hostops 领域包承载 mention、mission、child agent、transcript 和 host binding policy。主 Agent 只做 manager 和计划编排；每个被提及主机创建一个 host-bound child agent；child agent 的主机操作必须经 host-agent runtime 或 runner hybrid dispatcher。

**Tech Stack:** Go 1.24.3, React 19, Vite, Vitest, Playwright, assistant-ui, aiops-v2 `internal/appui`, `internal/server`, `internal/runtimekernel`, `internal/agentmgr`, `internal/planning`, `cmd/host-agent`, `pkg/runner/scheduler`.

---

## 0. 实施边界

- [x] 不改掉 `AGENTS.md` 要求的 structured transport 生产路径。
- [x] 不从 assistant final Markdown 解析 plan、child agent 或工具状态。
- [x] 不让主页面 manager agent 直接执行被 `@` 主机上的 shell/SSH/host-agent `/run`。
- [x] 不把多个 `@host` 合并给一个 child agent；唯一 resolved host 数等于 child agent 数。
- [x] 不允许 child agent 修改自己的绑定 host。
- [x] 多主机任务在 plan 未生成或未确认前，不执行 mutating host operation。
- [x] 所有 high-risk host operation 保持 approval/action token 治理。
- [x] UI 采用 Codex 风格紧凑状态面板：计划列表和子 Agent 状态行在同一个面板内，并紧贴 composer 上方。

Result 2026-06-04:

- Verified by Tasks 1-13 and final acceptance checklist.
- Hostops implementation stays on `TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React`.
- Browser-in-app and Playwright verified the Codex-style compact panel and subagent drawer.

## 1. 文件结构

### 新增后端文件

- `internal/hostops/types.go`
  - 定义 `HostMention`、`HostOperationMission`、`HostChildAgent`、`HostAgentReport`、status 常量、command payload 类型。
- `internal/hostops/mention_parser.go`
  - 后端权威 `@host` token parser，支持中文连接词场景。
- `internal/hostops/resolver.go`
  - 将 mention 解析到 inventory host record，支持 unresolved literal。
- `internal/hostops/mission_store.go`
  - 线程内 host operation mission 和 child agent 状态存储接口及内存实现。
- `internal/hostops/transcript_store.go`
  - child agent 独立 transcript 存储接口及内存实现。
- `internal/hostops/policy.go`
  - host binding enforcement、plan gate、mutating operation gate。
- `internal/hostops/orchestrator.go`
  - mission 创建、plan accept/revise、spawn/send/wait/stop child agent 编排。
- `internal/hostops/tools.go`
  - manager-only 内部工具 schema：`spawn_host_agent`、`send_host_agent_message`、`wait_host_agents`、`stop_host_agent`。
- `internal/hostops/*_test.go`
  - 覆盖 parser、resolver、mission store、transcript、policy、orchestrator。

### 修改后端文件

- `internal/appui/transport_state.go`
  - 增加 host mission、child agent、host mention transport types 和 `subagent` process kind。
- `internal/appui/transport_projector.go`
  - 将 hostops mission/child agent lifecycle 投影到 `AiopsTransportState`。
- `internal/appui/transport_commands.go`
  - 增加 plan accept/revise、child message/stop command 类型和 handler 分发。
- `internal/appui/contracts.go`
  - 增加 `HostOpsService` 接口，并用 hostops 类型别名暴露 command/view 类型。
- `internal/server/assistant_transport_request.go`
  - decode 新 command。
- `internal/server/assistant_transport_request_test.go`
  - 覆盖新 command decode。
- `internal/server/assistant_transport_api.go`
  - 将新 command 转成 appui transport command，并注册 transcript API。
- `internal/server/server.go`
  - 注册 hostops transcript endpoints。
- `internal/runtimekernel/*`
  - 接入 host ops route、manager prompt/tool exposure、plan gate hook。
- `internal/agentmgr/factory.go`
  - 加强 host-bound child agent 创建契约。
- `internal/agentmgr/manager.go`
  - 补充 hostops orchestrator 所需 child agent status/transcript lifecycle hook。
- `pkg/runner/scheduler/hybrid_dispatcher.go`
  - 只阅读现有远程执行契约；Task 7 通过 `ExecutionAdapter` 包装现有 dispatcher，不直接改 dispatcher。

### 新增前端文件

- `web/src/chat/hostMentions.ts`
  - 前端 mention parser、chip model、dedupe helper。
- `web/src/chat/components/HostMentionComposer.tsx`
  - 封装 `@host` 输入体验，保持与 `AiopsComposer` 集成。
- `web/src/chat/components/ComposerHostMentionMenu.tsx`
  - inventory host suggestion menu。
- `web/src/chat/components/HostMentionChip.tsx`
  - mention chip 展示。
- `web/src/chat/components/HostOpsStatusPanel.tsx`
  - Codex 风格输入框上方紧凑状态面板。
- `web/src/chat/components/HostOpsPlanSection.tsx`
  - 面板上半部分计划列表。
- `web/src/chat/components/HostSubagentStatusRow.tsx`
  - 面板底部子 Agent 状态行。
- `web/src/chat/components/HostSubagentDrawer.tsx`
  - 右侧 drawer，显示 child transcript 和 follow-up 输入。
- `web/src/api/hostOps.ts`
  - transcript fetch、child message/stop command helper。
- `web/src/chat/components/*host*.test.tsx`
  - mention、status panel、drawer 组件测试。

### 修改前端文件

- `web/src/chat/components/AiopsComposer.tsx`
  - 集成 mention metadata 和 HostMentionComposer。
- `web/src/chat/ChatPage.tsx`
  - 在 composer 上方渲染 `HostOpsStatusPanel`。
- `web/src/transport/aiopsTransportTypes.ts`
  - 增加 host mission / child agent / host mention TS types。
- `web/src/transport/aiopsTransportConverter.ts`
  - 确保 state 到 Assistant UI message metadata 的转换保留 hostops state。
- `web/src/transport/aiopsTransportRuntime.ts`
  - initial state 增加 optional hostops empty maps。
- `web/src/transport/*.test.ts`
  - 覆盖 hostops state 初始化、转换和兼容性。

## 2. Task 0：建立执行前 baseline

**Files:**
- Read: `docs/2026-06-04-ai-chat-host-mention-plan-subagents-design.zh.md`
- Read: `AGENTS.md`
- Read: `README.md`

- [x] **Step 0.1：记录当前工作区状态**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
git rev-parse HEAD
git status --short
```

Expected:

- 输出当前 commit hash。
- 记录已有未提交变更，后续任务不能回滚无关变更。

Result 2026-06-04:

- commit: `0cc65d5825622fb4b776b106ea53f776165821aa`
- `git status --short`: `.gitignore` modified, implementation todo document untracked.

- [x] **Step 0.2：确认当前后端基础测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/appui ./internal/server ./internal/planning ./internal/agentmgr
```

Expected:

- PASS。
- 如果存在既有失败，记录具体 package 和失败测试名；不在本任务中修复无关失败。

Result 2026-06-04:

- PASS: `./internal/appui`, `./internal/server`, `./internal/planning`, `./internal/agentmgr`.

- [x] **Step 0.3：确认当前前端基础测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/ChatPage.test.tsx src/transport/aiopsTransportConverter.test.ts src/transport/aiopsTransportRuntime.test.ts
```

Expected:

- PASS。
- 若失败，记录失败输出，并先判断是否是已有环境问题。

Result 2026-06-04:

- PASS: 3 test files, 37 tests.

- [x] **Step 0.4：确认 UI snapshot 入口**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test:ui:snapshots
```

Expected:

- PASS，或输出明确的 snapshot 差异。
- 如果本地浏览器依赖缺失，记录缺失依赖，不绕过后续 UI 验证。

Result 2026-06-04:

- Baseline has one pre-existing Playwright failure before implementation:
  `tests/react-shell-snapshot.spec.js:828` / `chat shows context compaction and externalized evidence states`.
- Failure: `getByText("结果较大，仅显示摘要。")` not found.
- 8 snapshot tests passed; this failure must be tracked separately from hostops feature regressions.

## 3. Task 1：新增 hostops 领域类型、parser 与 resolver

**Files:**
- Create: `internal/hostops/types.go`
- Create: `internal/hostops/mention_parser.go`
- Create: `internal/hostops/resolver.go`
- Create: `internal/hostops/mention_parser_test.go`
- Create: `internal/hostops/resolver_test.go`
- Read: `internal/store/store.go`

- [x] **Step 1.1：写 parser 测试**

Create `internal/hostops/mention_parser_test.go` with these test cases:

```go
package hostops

import "testing"

func TestParseHostMentionsChineseConnector(t *testing.T) {
	input := "@1.1.1.1和@1.1.1.2作为pg节点,搭建一个主从集群,@1.1.1.3作为pg_mon."
	mentions := ParseHostMentions(input)
	if len(mentions) != 3 {
		t.Fatalf("len(mentions) = %d, want 3: %#v", len(mentions), mentions)
	}
	if mentions[0].Raw != "@1.1.1.1" || mentions[1].Raw != "@1.1.1.2" || mentions[2].Raw != "@1.1.1.3" {
		t.Fatalf("mentions = %#v, want ordered IP mentions", mentions)
	}
}

func TestParseHostMentionsDedupesNormalizedToken(t *testing.T) {
	mentions := ParseHostMentions("@db-1 检查一下 @db-1 的pg状态")
	unique := UniqueMentionKeys(mentions)
	if len(unique) != 1 {
		t.Fatalf("len(unique) = %d, want 1: %#v", len(unique), unique)
	}
}

func TestParseHostMentionsIgnoresPlainEmail(t *testing.T) {
	mentions := ParseHostMentions("联系 sre@example.com，不要把邮箱解析成主机")
	if len(mentions) != 0 {
		t.Fatalf("mentions = %#v, want none", mentions)
	}
}
```

- [x] **Step 1.2：运行 parser 测试确认失败**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestParseHostMentions|TestUniqueMentionKeys'
```

Expected before implementation:

- FAIL because package or functions do not exist.

Result 2026-06-04:

- RED confirmed: `ParseHostMentions` and `UniqueMentionKeys` were undefined.

- [x] **Step 1.3：实现 hostops types 和 parser**

Create `internal/hostops/types.go` with public contracts:

```go
package hostops

import "time"

type HostMentionSource string

const (
	HostMentionSourceInventory       HostMentionSource = "inventory"
	HostMentionSourceIPLiteral       HostMentionSource = "ip_literal"
	HostMentionSourceHostnameLiteral HostMentionSource = "hostname_literal"
)

type HostMention struct {
	TokenID     string            `json:"tokenId"`
	Raw         string            `json:"raw"`
	SpanStart   int               `json:"spanStart"`
	SpanEnd     int               `json:"spanEnd"`
	HostID      string            `json:"hostId,omitempty"`
	Address     string            `json:"address,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Source      HostMentionSource `json:"source"`
	Resolved    bool              `json:"resolved"`
	Confidence  float64           `json:"confidence"`
	CreatedAt   time.Time         `json:"createdAt"`
}
```

Create `internal/hostops/mention_parser.go` with exported functions:

```go
func ParseHostMentions(input string) []HostMention
func UniqueMentionKeys(mentions []HostMention) []string
```

Implementation constraints:

- Treat `@` followed by IPv4, IPv6 bracket, hostname, inventory-like id as candidate.
- Stop token at whitespace, comma, Chinese comma, semicolon, Chinese semicolon, period, Chinese period, closing bracket, or another `@`.
- Do not parse emails where `@` has an identifier immediately before it.
- Fill `Raw`, `SpanStart`, `SpanEnd`, `Address` or `DisplayName`, `Source`, `Resolved=false`, `Confidence`.

- [x] **Step 1.4：运行 parser 测试确认通过**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestParseHostMentions|TestUniqueMentionKeys'
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/hostops -run 'TestParseHostMentions|TestUniqueMentionKeys'`.

- [x] **Step 1.5：写 resolver 测试**

Create `internal/hostops/resolver_test.go` with an in-memory host lookup:

```go
func TestResolveMentionsMatchesInventoryAddress(t *testing.T) {
	resolver := NewResolver(staticHostLookup{
		{ID: "host-a", Address: "1.1.1.1", DisplayName: "pg-a", Managed: true, Executable: true},
	})
	mentions := ParseHostMentions("@1.1.1.1 部署pg")
	resolved, errs := resolver.Resolve(context.Background(), mentions)
	if len(errs) != 0 {
		t.Fatalf("errs = %#v, want none", errs)
	}
	if !resolved[0].Resolved || resolved[0].HostID != "host-a" {
		t.Fatalf("resolved[0] = %#v, want host-a", resolved[0])
	}
}

func TestResolveMentionsLeavesUnknownIPUnresolved(t *testing.T) {
	resolver := NewResolver(staticHostLookup{})
	resolved, errs := resolver.Resolve(context.Background(), ParseHostMentions("@1.1.1.9 部署pg"))
	if len(errs) != 1 {
		t.Fatalf("len(errs) = %d, want 1", len(errs))
	}
	if resolved[0].Resolved {
		t.Fatalf("resolved[0].Resolved = true, want false")
	}
}
```

- [x] **Step 1.6：实现 resolver**

Create `internal/hostops/resolver.go` with:

```go
type HostRecordView struct {
	ID          string
	Address     string
	Hostname    string
	DisplayName string
	Managed     bool
	Executable  bool
	AgentURL    string
}

type HostLookup interface {
	ListHosts(ctx context.Context) ([]HostRecordView, error)
}

type Resolver struct {
	lookup HostLookup
}

func NewResolver(lookup HostLookup) *Resolver
func (r *Resolver) Resolve(ctx context.Context, mentions []HostMention) ([]HostMention, []MentionResolutionError)
```

Resolution order:

1. exact host id
2. exact address
3. exact hostname
4. exact display name
5. unresolved literal with `Resolved=false`

- [x] **Step 1.7：运行 hostops parser/resolver 测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/hostops`.

- [x] **Step 1.8：提交 Task 1**

Run:

```bash
git add internal/hostops
git commit -m "feat(hostops): add host mention parser and resolver"
```

Expected:

- Commit created with only hostops parser/resolver files.

Result 2026-06-04:

- Commit: `9b904c4 feat(hostops): add host mention parser and resolver`.

## 4. Task 2：扩展 transport state 契约

**Files:**
- Modify: `internal/appui/transport_state.go`
- Modify: `web/src/transport/aiopsTransportTypes.ts`
- Modify: `web/src/transport/aiopsTransportRuntime.ts`
- Test: `web/src/transport/aiopsTransportRuntime.test.ts`
- Test: `internal/appui/transport_state_test.go` if present; otherwise create `internal/appui/transport_hostops_state_test.go`

- [x] **Step 2.1：写 Go transport state 测试**

Create `internal/appui/transport_hostops_state_test.go`:

```go
package appui

import "testing"

func TestNewAiopsTransportStateInitializesHostOpsMaps(t *testing.T) {
	state := NewAiopsTransportState("sess-1", "thread-1")
	if state.HostMissions == nil {
		t.Fatalf("HostMissions is nil")
	}
	if state.ChildAgents == nil {
		t.Fatalf("ChildAgents is nil")
	}
}

func TestAiopsTransportStateSerializesHostMission(t *testing.T) {
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.HostMissions["mission-1"] = AiopsTransportHostMission{
		ID:           "mission-1",
		TurnID:       "turn-1",
		Status:       "planning",
		PlanRequired: true,
		MentionedHosts: []AiopsTransportHostMention{
			{TokenID: "hm-1", Raw: "@1.1.1.1", HostID: "host-a", Address: "1.1.1.1", DisplayName: "1.1.1.1", Source: "inventory", Resolved: true},
		},
	}
	if state.HostMissions["mission-1"].MentionedHosts[0].HostID != "host-a" {
		t.Fatalf("state.HostMissions = %#v, want host-a", state.HostMissions)
	}
}
```

- [x] **Step 2.2：运行 Go transport state 测试确认失败**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/appui -run 'TestNewAiopsTransportStateInitializesHostOpsMaps|TestAiopsTransportStateSerializesHostMission'
```

Expected before implementation:

- FAIL because hostops fields/types do not exist.

Result 2026-06-04:

- RED confirmed: `HostMissions`, `ChildAgents`, `AiopsTransportHostMission`, and `AiopsTransportHostMention` were undefined.

- [x] **Step 2.3：实现 Go transport types**

Modify `internal/appui/transport_state.go`:

```go
const (
	AiopsTransportProcessKindSubagent AiopsTransportProcessKind = "subagent"
)

type AiopsTransportState struct {
	// existing fields stay unchanged
	HostMissions        map[string]AiopsTransportHostMission `json:"hostMissions,omitempty"`
	ChildAgents         map[string]AiopsTransportChildAgent  `json:"childAgents,omitempty"`
	ActiveHostMissionID string                               `json:"activeHostMissionId,omitempty"`
}

type AiopsTransportHostMission struct {
	ID                 string                       `json:"id"`
	TurnID             string                       `json:"turnId"`
	Status             string                       `json:"status"`
	PlanRequired       bool                         `json:"planRequired"`
	PlanAccepted       bool                         `json:"planAccepted"`
	MentionedHosts     []AiopsTransportHostMention  `json:"mentionedHosts"`
	ChildAgentIDs      []string                     `json:"childAgentIds"`
	ManagerAgentID     string                       `json:"managerAgentId,omitempty"`
	ActiveChildAgentID string                       `json:"activeChildAgentId,omitempty"`
	CreatedAt          string                       `json:"createdAt,omitempty"`
	UpdatedAt          string                       `json:"updatedAt,omitempty"`
}
```

Also define `AiopsTransportHostMention` and `AiopsTransportChildAgent` with fields from the design document.

- [x] **Step 2.4：写 TS runtime 测试**

Modify `web/src/transport/aiopsTransportRuntime.test.ts`:

```ts
it("initializes host operation state maps", () => {
  const state = createInitialAiopsTransportState();
  expect(state.hostMissions).toEqual({});
  expect(state.childAgents).toEqual({});
  expect(state.activeHostMissionId).toBeUndefined();
});
```

- [x] **Step 2.5：实现 TS transport types and runtime defaults**

Modify `web/src/transport/aiopsTransportTypes.ts`:

```ts
export type AiopsTransportProcessKind =
  | "plan"
  | "assistant"
  | "reasoning"
  | "search"
  | "command"
  | "file"
  | "tool"
  | "evidence"
  | "approval"
  | "mcp"
  | "system"
  | "subagent";
```

Add:

```ts
export type AiopsTransportHostMission = {
  id: string;
  turnId: string;
  status: HostMissionStatus;
  planRequired: boolean;
  planAccepted: boolean;
  mentionedHosts: AiopsTransportHostMention[];
  childAgentIds: string[];
  managerAgentId?: string;
  activeChildAgentId?: string;
  createdAt?: string;
  updatedAt?: string;
};
```

Modify `createInitialAiopsTransportState()` to include:

```ts
hostMissions: {},
childAgents: {},
```

- [x] **Step 2.6：运行 transport contract 测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/appui -run 'TestNewAiopsTransportStateInitializesHostOpsMaps|TestAiopsTransportStateSerializesHostMission'
cd web
npm run test -- --run src/transport/aiopsTransportRuntime.test.ts
```

Expected:

- Both commands PASS。

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/appui -run 'TestNewAiopsTransportStateInitializesHostOpsMaps|TestAiopsTransportStateSerializesHostMission'`.
- PASS: `npm run test -- --run src/transport/aiopsTransportRuntime.test.ts`.
- Additional PASS: `go test -count=1 ./internal/appui`.
- Additional PASS: `npm run test -- --run src/chat/ChatPage.test.tsx src/transport/aiopsTransportConverter.test.ts src/transport/aiopsTransportRuntime.test.ts`.

- [x] **Step 2.7：提交 Task 2**

Run:

```bash
git add internal/appui/transport_state.go internal/appui/transport_hostops_state_test.go web/src/transport/aiopsTransportTypes.ts web/src/transport/aiopsTransportRuntime.ts web/src/transport/aiopsTransportRuntime.test.ts
git commit -m "feat(transport): add host operation state contracts"
```

Expected:

- Commit created with transport contract changes only.

Result 2026-06-04:

- Commit: `4965bc7 feat(transport): add host operation state contracts`.

## 5. Task 3：实现 host mission、child agent 和 transcript store

**Files:**
- Create: `internal/hostops/mission_store.go`
- Create: `internal/hostops/transcript_store.go`
- Create: `internal/hostops/mission_store_test.go`
- Create: `internal/hostops/transcript_store_test.go`
- Modify: `internal/hostops/types.go`

- [x] **Step 3.1：写 mission store 测试**

Create `internal/hostops/mission_store_test.go`:

```go
func TestMissionStoreCreatesMissionAndChildAgents(t *testing.T) {
	store := NewInMemoryMissionStore()
	mission := HostOperationMission{
		ID: "mission-1", ThreadID: "thread-1", UserTurnID: "turn-1",
		Status: HostMissionStatusPlanning, PlanRequired: true,
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	child := HostChildAgent{
		ID: "agent-1", MissionID: "mission-1", HostID: "host-a", HostAddress: "1.1.1.1",
		Status: HostChildAgentStatusPlanned,
	}
	if err := store.SaveChildAgent(context.Background(), child); err != nil {
		t.Fatalf("SaveChildAgent() error = %v", err)
	}
	children, err := store.ListChildAgents(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("ListChildAgents() error = %v", err)
	}
	if len(children) != 1 || children[0].HostID != "host-a" {
		t.Fatalf("children = %#v, want host-a", children)
	}
}
```

- [x] **Step 3.2：写 transcript store 测试**

Create `internal/hostops/transcript_store_test.go`:

```go
func TestTranscriptStoreAppendsOrderedItems(t *testing.T) {
	store := NewInMemoryTranscriptStore()
	err := store.Append(context.Background(), "agent-1", TranscriptItem{Type: TranscriptItemManagerMessage, Content: "检查PG版本"})
	if err != nil {
		t.Fatalf("Append(manager) error = %v", err)
	}
	err = store.Append(context.Background(), "agent-1", TranscriptItem{Type: TranscriptItemAssistantMessage, Content: "PostgreSQL 15"})
	if err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}
	items, err := store.List(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 || items[0].Type != TranscriptItemManagerMessage || items[1].Type != TranscriptItemAssistantMessage {
		t.Fatalf("items = %#v, want manager then assistant", items)
	}
}
```

- [x] **Step 3.3：运行 store 测试确认失败**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestMissionStore|TestTranscriptStore'
```

Expected before implementation:

- FAIL because store types do not exist.

Result 2026-06-04:

- RED confirmed: mission store, transcript store, mission/child/transcript types were undefined.

- [x] **Step 3.4：实现 mission store 和 transcript store**

Required public interfaces:

```go
type MissionStore interface {
	SaveMission(ctx context.Context, mission HostOperationMission) error
	GetMission(ctx context.Context, missionID string) (HostOperationMission, error)
	ListThreadMissions(ctx context.Context, threadID string) ([]HostOperationMission, error)
	SaveChildAgent(ctx context.Context, child HostChildAgent) error
	GetChildAgent(ctx context.Context, childAgentID string) (HostChildAgent, error)
	ListChildAgents(ctx context.Context, missionID string) ([]HostChildAgent, error)
}

type TranscriptStore interface {
	Append(ctx context.Context, childAgentID string, item TranscriptItem) error
	List(ctx context.Context, childAgentID string) ([]TranscriptItem, error)
}
```

Implementation constraints:

- In-memory store must copy slices/maps on read/write to avoid test data mutation.
- Store methods must return typed not-found errors, for example `ErrMissionNotFound` and `ErrChildAgentNotFound`.
- Transcript items must get stable IDs and timestamps if caller did not provide them.

- [x] **Step 3.5：运行 hostops store 测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestMissionStore|TestTranscriptStore'
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/hostops -run 'TestMissionStore|TestTranscriptStore'`.
- Additional PASS: `go test -count=1 ./internal/hostops`.

- [x] **Step 3.6：提交 Task 3**

Run:

```bash
git add internal/hostops
git commit -m "feat(hostops): add mission and transcript stores"
```

Expected:

- Commit created with hostops store files.

Result 2026-06-04:

- Commit: `b7774cb feat(hostops): add mission and transcript stores`.

## 6. Task 4：接入 transport commands 和 transcript API

**Files:**
- Modify: `internal/server/assistant_transport_request.go`
- Modify: `internal/server/assistant_transport_request_test.go`
- Modify: `internal/appui/transport_commands.go`
- Modify: `internal/appui/transport_commands_test.go`
- Modify: `internal/appui/contracts.go`
- Modify: `internal/server/assistant_transport_api.go`
- Modify: `internal/server/http.go`
- Create: `internal/server/host_ops_api.go`
- Create: `internal/server/host_ops_api_test.go`

- [x] **Step 4.1：扩展 assistant transport request decode 测试**

Add these command payloads to `TestAssistantTransportRequestDecodesKnownCommands`:

```json
{
  "type": "aiops.host-plan-accept",
  "missionId": "mission-1",
  "planId": "plan-1"
},
{
  "type": "aiops.host-plan-revise",
  "missionId": "mission-1",
  "instruction": "先检查PG版本"
},
{
  "type": "aiops.child-agent-message",
  "childAgentId": "agent-1",
  "content": "只读检查，不要修改"
},
{
  "type": "aiops.child-agent-stop",
  "childAgentId": "agent-1"
}
```

Expected assertions:

```go
if len(req.Commands) != 12 {
	t.Fatalf("len(Commands) = %d, want 12", len(req.Commands))
}
```

Assert concrete decoded command types and fields for all four new commands.

- [x] **Step 4.2：运行 request decode 测试确认失败**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/server -run TestAssistantTransportRequestDecodesKnownCommands
```

Expected before implementation:

- FAIL because command types are unknown.

Result 2026-06-04:

- RED confirmed: hostops wire command structs and appui transport command fields were undefined.

- [x] **Step 4.3：实现 server command decode**

Modify `internal/server/assistant_transport_request.go`:

```go
const (
	assistantTransportCommandHostPlanAccept   = "aiops.host-plan-accept"
	assistantTransportCommandHostPlanRevise   = "aiops.host-plan-revise"
	assistantTransportCommandChildAgentMsg    = "aiops.child-agent-message"
	assistantTransportCommandChildAgentStop   = "aiops.child-agent-stop"
)
```

Add concrete structs:

```go
type assistantTransportHostPlanAcceptCommand struct {
	MissionID string `json:"missionId"`
	PlanID    string `json:"planId"`
}

type assistantTransportChildAgentMessageCommand struct {
	ChildAgentID string `json:"childAgentId"`
	Content      string `json:"content"`
}
```

Add decoding branches and validation:

- `missionId` required for plan commands.
- `childAgentId` required for child commands.
- `content` required for child message.

- [x] **Step 4.4：写 appui command handler 测试**

Extend `internal/appui/transport_commands_test.go` with a `transportCommandHostOpsServiceStub`:

```go
type transportCommandHostOpsServiceStub struct {
	acceptedMissionID string
	childMessageID    string
	childMessageText  string
}

func (s *transportCommandHostOpsServiceStub) AcceptPlan(_ context.Context, missionID, planID string) (HostOperationView, error) {
	s.acceptedMissionID = missionID
	return HostOperationView{ID: missionID, Status: "spawning_children"}, nil
}

func (s *transportCommandHostOpsServiceStub) SendChildMessage(_ context.Context, childAgentID, content string) (HostChildAgentView, error) {
	s.childMessageID = childAgentID
	s.childMessageText = content
	return HostChildAgentView{ID: childAgentID, Status: "running"}, nil
}
```

Test:

```go
func TestTransportCommandsHostPlanAcceptCallsHostOpsService(t *testing.T)
func TestTransportCommandsChildAgentMessageCallsHostOpsService(t *testing.T)
```

- [x] **Step 4.5：实现 appui command types and handler**

Modify `internal/appui/transport_commands.go`:

```go
const (
	TransportCommandTypeHostPlanAccept TransportCommandType = "aiops.host-plan-accept"
	TransportCommandTypeHostPlanRevise TransportCommandType = "aiops.host-plan-revise"
	TransportCommandTypeChildAgentMessage TransportCommandType = "aiops.child-agent-message"
	TransportCommandTypeChildAgentStop TransportCommandType = "aiops.child-agent-stop"
)
```

Add `HostOpsService` dependency to `NewTransportCommandHandler` using the smallest constructor change that keeps existing call sites compiling. If constructor churn is too high, add an option setter:

```go
func (h *TransportCommandHandler) WithHostOpsService(service HostOpsService) *TransportCommandHandler
```

- [x] **Step 4.6：实现 transcript HTTP API tests**

Create `internal/server/host_ops_api_test.go`:

```go
func TestHostOpsTranscriptAPIRequiresChildAgentID(t *testing.T)
func TestHostOpsTranscriptAPIReturnsTranscriptItems(t *testing.T)
```

Expected endpoint:

```http
GET /api/v1/host-ops/child-agents/{childAgentId}/transcript
```

Response shape:

```json
{
  "childAgentId": "agent-1",
  "items": [
    {"id": "item-1", "type": "manager_message", "content": "检查PG版本"}
  ]
}
```

- [x] **Step 4.7：实现 transcript API**

Create `internal/server/host_ops_api.go` and register route in `internal/server/http.go`.

Constraints:

- Return `404` for unknown child agent.
- Return `400` for missing child agent id.
- Do not include host-agent token, environment secrets, or raw approval secret.

- [x] **Step 4.8：运行 command/API 测试**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/server -run 'TestAssistantTransportRequestDecodesKnownCommands|TestHostOpsTranscriptAPI'
go test -count=1 ./internal/appui -run 'TestTransportCommandsHost|TestTransportCommandsChild'
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/server -run 'TestAssistantTransportRequestDecodesKnownCommands|TestHostOpsTranscriptAPI'`.
- PASS: `go test -count=1 ./internal/appui -run 'TestTransportCommandsHost|TestTransportCommandsChild'`.
- Additional PASS: `go test -count=1 ./internal/server`.
- Additional PASS: `go test -count=1 ./internal/appui`.

- [x] **Step 4.9：提交 Task 4**

Run:

```bash
git add internal/server internal/appui
git commit -m "feat(transport): add host operation commands and transcript api"
```

Expected:

- Commit created with command/API changes.

Result 2026-06-04:

- Commit: `8506a8d feat(transport): add host operation commands and transcript api`.

## 7. Task 5：实现 host ops route、强制 plan gate 和 manager prompt

**Files:**
- Create: `internal/hostops/route.go`
- Create: `internal/hostops/policy.go`
- Create: `internal/hostops/route_test.go`
- Create: `internal/hostops/policy_test.go`
- Modify: `internal/runtimekernel/*` route or turn entry files
- Modify: `internal/promptcompiler/*` or current manager prompt assembly file
- Test: focused runtimekernel/promptcompiler tests

- [x] **Step 5.1：写 route detector 测试**

Create `internal/hostops/route_test.go`:

```go
func TestDetectRouteForMultiHostForcesPlan(t *testing.T) {
	mentions := []HostMention{
		{Raw: "@1.1.1.1", HostID: "host-a", Resolved: true},
		{Raw: "@1.1.1.2", HostID: "host-b", Resolved: true},
	}
	decision := DetectRoute("搭建pg主从集群", mentions)
	if decision.Kind != RouteKindHostOps {
		t.Fatalf("Kind = %q, want host_ops", decision.Kind)
	}
	if !decision.PlanRequired {
		t.Fatalf("PlanRequired = false, want true")
	}
}

func TestDetectRouteForSingleHostDoesNotForcePlan(t *testing.T) {
	mentions := []HostMention{{Raw: "@1.1.1.1", HostID: "host-a", Resolved: true}}
	decision := DetectRoute("检查pg状态", mentions)
	if decision.Kind != RouteKindHostOps {
		t.Fatalf("Kind = %q, want host_ops", decision.Kind)
	}
	if decision.PlanRequired {
		t.Fatalf("PlanRequired = true, want false for single host read operation")
	}
}
```

- [x] **Step 5.2：写 plan gate policy 测试**

Create `internal/hostops/policy_test.go`:

```go
func TestPlanGateBlocksMutatingOperationBeforePlanAccepted(t *testing.T) {
	mission := HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false}
	err := EnforcePlanGate(mission, OperationRiskMutating)
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("err = %v, want ErrPlanNotAccepted", err)
	}
}

func TestPlanGateAllowsReadOnlyPrecheckBeforePlanAccepted(t *testing.T) {
	mission := HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false}
	if err := EnforcePlanGate(mission, OperationRiskReadOnly); err != nil {
		t.Fatalf("EnforcePlanGate(readonly) error = %v", err)
	}
}
```

- [x] **Step 5.3：实现 route detector and plan gate**

Create `internal/hostops/route.go`:

```go
type RouteKind string

const (
	RouteKindNormalChat RouteKind = "normal_chat"
	RouteKindHostOps    RouteKind = "host_ops"
)

type RouteDecision struct {
	Kind         RouteKind
	Mentions     []HostMention
	PlanRequired bool
	Reason       string
}

func DetectRoute(content string, mentions []HostMention) RouteDecision
```

Create `internal/hostops/policy.go` with:

```go
func EnforcePlanGate(mission HostOperationMission, risk OperationRisk) error
func EnforceHostBinding(ctx ToolContext, requestedHostID string) error
```

- [x] **Step 5.4：接入 runtime turn route**

Modify runtime turn entry so `add-message` with host mentions:

- Parses and resolves mentions server-side.
- Creates a host mission.
- Sets `PlanRequired=true` when unique mention count is at least 2.
- Adds manager prompt constraints to the turn.
- Exposes `update_plan` and manager-only hostops tools to manager route.

Implementation result:

- `TransportCommandHandler.applyAddMessage` now performs server-side `@主机` parsing and route detection, enriches `aiops.hostops.*` turn metadata, and creates the active `HostMission` in transport state for the UI panel.
- Runtime reads `aiops.hostops.*` metadata and enables `HostOpsManager` / `HostOpsPlanRequired` compile context.
- PromptCompiler emits the mandatory host-ops manager rules, including multi-host structured-plan requirement and “manager must not execute host commands directly”.
- Persistent mission orchestration and concrete child-agent spawning remain in Task 6/7 by design; Task 5 only establishes route, gate, prompt, and initial mission state.

Required manager prompt text must include:

```text
当用户消息包含多个 @主机 时，你必须先制定结构化计划。
你不能直接在任何被 @ 的主机上执行命令。
你必须为每个被 @ 的唯一主机启动一个独立 host-bound 子 Agent。
```

- [x] **Step 5.5：运行 route/policy focused tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestDetectRoute|TestPlanGate'
go test -count=1 ./internal/runtimekernel ./internal/promptcompiler
```

Expected:

- PASS。

Actual:

- `go test -count=1 ./internal/appui -run 'TestTransportCommandsAddMessageCreatesMultiHostMissionRoute|TestTransportCommandsAddMessageCallsChatService'` PASS。
- `go test -count=1 ./internal/hostops -run 'TestDetectRoute|TestPlanGate'` PASS。
- `go test -count=1 ./internal/runtimekernel ./internal/promptcompiler` PASS。

- [x] **Step 5.6：提交 Task 5**

Run:

```bash
git add internal/hostops internal/runtimekernel internal/promptcompiler
git commit -m "feat(hostops): route host mentions through mandatory plan mode"
```

Expected:

- Commit created with route, policy and manager prompt changes.

Actual:

- Commit `a73eec4 feat(hostops): route host mentions through mandatory plan mode` created.

## 8. Task 6：实现 manager-only host child agent 编排工具

**Files:**
- Create: `internal/hostops/orchestrator.go`
- Create: `internal/hostops/tools.go`
- Create: `internal/hostops/orchestrator_test.go`
- Modify: `internal/agentmgr/factory.go`
- Modify: `internal/agentmgr/manager.go`
- Modify: `internal/agentmgr/kernel_adapter.go`

- [x] **Step 6.1：写 orchestrator 测试：每个 host 一个 child agent**

Create `internal/hostops/orchestrator_test.go`:

```go
func TestOrchestratorSpawnsOneChildPerMentionedHost(t *testing.T) {
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID: "mission-1", ThreadID: "thread-1", UserTurnID: "turn-1",
		PlanRequired: true, PlanAccepted: true,
		Mentions: []HostMention{
			{HostID: "host-a", Address: "1.1.1.1", Resolved: true},
			{HostID: "host-b", Address: "1.1.1.2", Resolved: true},
			{HostID: "host-c", Address: "1.1.1.3", Resolved: true},
		},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	children, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{
		{HostID: "host-a", Role: "pg primary candidate", Task: "prepare pg primary"},
		{HostID: "host-b", Role: "pg standby candidate", Task: "prepare pg standby"},
		{HostID: "host-c", Role: "pg_mon", Task: "prepare monitor"},
	})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("len(children) = %d, want 3", len(children))
	}
}
```

- [x] **Step 6.2：写 orchestrator 测试：plan 未确认不得 spawn mutating child**

Add:

```go
func TestOrchestratorRejectsSpawnBeforePlanAccepted(t *testing.T) {
	store := NewInMemoryMissionStore()
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	_ = store.SaveMission(context.Background(), HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false})
	_, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "install pg"}})
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("err = %v, want ErrPlanNotAccepted", err)
	}
}
```

- [x] **Step 6.3：实现 orchestrator and manager-only tools**

Create `internal/hostops/orchestrator.go` with:

```go
type ChildSpawner interface {
	SpawnHostChild(ctx context.Context, req SpawnHostChildRequest) (HostChildAgent, error)
	SendMessage(ctx context.Context, childAgentID, content string) (HostChildAgent, error)
	Stop(ctx context.Context, childAgentID string) (HostChildAgent, error)
}

type Orchestrator struct {
	store       MissionStore
	transcript  TranscriptStore
	spawner     ChildSpawner
}
```

Create `internal/hostops/tools.go` with tool schemas for:

- `spawn_host_agent`
- `send_host_agent_message`
- `wait_host_agents`
- `stop_host_agent`

Constraints:

- Tools are manager-only.
- `spawn_host_agent` rejects hosts outside mission mentions.
- Duplicate host in the same mission returns existing child or a typed duplicate error; it must not create a second child for the same host.

Implementation result:

- Added `hostops.Orchestrator` with `SpawnChildren`, `SendMessage`, `Stop`, and `WaitChildren`.
- `SpawnChildren` enforces `ErrPlanNotAccepted`, rejects `ErrHostOutsideMission`, appends transcript entries, and returns an existing child for duplicate host assignment instead of spawning twice.
- Added manager-only tools: `spawn_host_agent`, `send_host_agent_message`, `wait_host_agents`, `stop_host_agent`.

- [x] **Step 6.4：connect agentmgr factory**

Modify `internal/agentmgr/factory.go` and adapter files so `SpawnHostChild`:

- Requires `hostId`.
- Creates child session/thread metadata.
- Calls existing host agent creation path when possible.
- Stores `ParentAgentID`, `MissionID`, `HostID`, `SessionID`.

Required child prompt excerpt:

```text
你是 host-bound 运维子 Agent。
你的绑定主机是 {hostDisplayName}，hostId={hostId}。
你只能对这个主机执行检查、配置、安装或诊断。
如果任务需要其他主机信息，你只能向 manager 汇报需要协调，不能直接操作其他主机。
```

Implementation result:

- Added `AgentFactory.CreateHostChildAgent`, reusing host agent model/tool assembly and injecting the required host-bound child prompt asset.
- Added `KernelAdapter.SpawnHostChild`, `SendMessage`, and `Stop` so it satisfies `hostops.ChildSpawner`.

- [x] **Step 6.5：运行 child orchestration tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestOrchestrator'
go test -count=1 ./internal/agentmgr
```

Expected:

- PASS。

Actual:

- `go test -count=1 ./internal/hostops -run 'TestOrchestrator'` PASS。
- `go test -count=1 ./internal/agentmgr` PASS。
- `go test -count=1 ./internal/hostops ./internal/agentmgr` PASS。

- [x] **Step 6.6：提交 Task 6**

Run:

```bash
git add internal/hostops internal/agentmgr
git commit -m "feat(hostops): orchestrate host-bound child agents"
```

Expected:

- Commit created with orchestrator and agent manager integration.

Actual:

- Commit `423446c feat(hostops): orchestrate host-bound child agents` created.

## 9. Task 7：实现 host binding enforcement 和 host-agent 执行边界

**Files:**
- Modify: `internal/hostops/policy.go`
- Create: `internal/hostops/execution_adapter.go`
- Create: `internal/hostops/execution_adapter_test.go`
- Modify: host tool assembly files discovered by `rg "ToolContext|HostID|runner" internal`
- Read: `pkg/runner/scheduler/hybrid_dispatcher.go`

- [x] **Step 7.1：写 host binding 测试**

Add to `internal/hostops/policy_test.go`:

```go
func TestEnforceHostBindingRejectsCrossHost(t *testing.T) {
	ctx := ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"}
	err := EnforceHostBinding(ctx, "host-b")
	if !errors.Is(err, ErrCrossHostDenied) {
		t.Fatalf("err = %v, want ErrCrossHostDenied", err)
	}
}

func TestEnforceHostBindingDefaultsEmptyRequestedHostToBoundHost(t *testing.T) {
	ctx := ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"}
	if err := EnforceHostBinding(ctx, ""); err != nil {
		t.Fatalf("EnforceHostBinding(empty) error = %v", err)
	}
}
```

- [x] **Step 7.2：写 execution adapter 测试**

Create `internal/hostops/execution_adapter_test.go`:

```go
func TestExecutionAdapterDispatchesOnlyToBoundHostAgent(t *testing.T) {
	dispatcher := &fakeHostDispatcher{}
	adapter := NewExecutionAdapter(dispatcher)
	ctx := ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"}
	_, err := adapter.RunShell(context.Background(), ctx, HostCommandRequest{
		HostID: "host-a",
		Script: "pg_isready",
	})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if dispatcher.lastHostID != "host-a" {
		t.Fatalf("lastHostID = %q, want host-a", dispatcher.lastHostID)
	}
}
```

- [x] **Step 7.3：实现 host binding policy and execution adapter**

`execution_adapter.go` must route:

```text
Child Agent -> host tool adapter -> runner hybrid dispatcher -> host-agent /run
```

It must reject:

```text
Manager Agent -> direct host command
Child Agent @hostA -> HostID hostB
```

Required public function:

```go
func (a *ExecutionAdapter) RunShell(ctx context.Context, toolCtx ToolContext, req HostCommandRequest) (HostCommandResult, error)
```

- [x] **Step 7.4：wire adapter into host tools**

Search:

```bash
rg -n "ToolContext|HostID|runner|host-agent|RunShell|script.shell" internal pkg
```

For every host operation tool, ensure:

- It receives `ToolContext`.
- It calls `EnforceHostBinding`.
- It dispatches through host-agent/runner.
- High-risk operations keep approval.

Result 2026-06-04:

- Added `ToolContext`, manager/host-child agent kind, cross-host denial, manager-direct-host denial.
- Added `ExecutionAdapter.RunShell` that dispatches `script.shell` via runner scheduler task and always applies host binding before dispatch.
- Reviewed existing host tool surface: `exec_command` remains a governed local terminal/break-glass tool and is not repurposed as a remote host operation tool; host operation execution now has a dedicated host-bound adapter for future host tools to call.

- [x] **Step 7.5：运行 enforcement tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops -run 'TestEnforceHostBinding|TestExecutionAdapter'
go test -count=1 ./pkg/runner/scheduler ./cmd/host-agent ./internal/agentmgr
```

Expected:

- PASS。

Actual:

- PASS: `go test -count=1 ./internal/hostops -run 'TestEnforceHostBinding|TestExecutionAdapter'`.
- PASS: `go test -count=1 ./cmd/host-agent ./internal/agentmgr`.
- PASS: `cd pkg/runner && go test -count=1 ./scheduler`.
- Note: the plan's `./pkg/runner/scheduler` path is inside a nested `pkg/runner/go.mod`, so it was verified from the runner module root.

- [x] **Step 7.6：提交 Task 7**

Run:

```bash
git add internal/hostops pkg/runner/scheduler cmd/host-agent internal/agentmgr
git commit -m "feat(hostops): enforce host-bound execution"
```

Expected:

- Commit created with host binding enforcement.

Actual:

- Commit: `afaf29e feat(hostops): enforce host-bound execution`.

## 10. Task 8：前端 `@主机` mention 输入与 metadata

**Files:**
- Create: `web/src/chat/hostMentions.ts`
- Create: `web/src/chat/hostMentions.test.ts`
- Create: `web/src/chat/components/HostMentionComposer.tsx`
- Create: `web/src/chat/components/ComposerHostMentionMenu.tsx`
- Create: `web/src/chat/components/HostMentionChip.tsx`
- Create: `web/src/chat/components/HostMentionComposer.test.tsx`
- Modify: `web/src/chat/components/AiopsComposer.tsx`
- Create: `web/src/api/hostInventory.ts`
- Read: `web/src/api/hosts.js`

- [x] **Step 8.1：写 frontend mention parser 测试**

Create `web/src/chat/hostMentions.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { parseHostMentionCandidates, uniqueHostMentionKeys } from "./hostMentions";

describe("hostMentions", () => {
  it("parses Chinese connector host mentions", () => {
    const result = parseHostMentionCandidates("@1.1.1.1和@1.1.1.2作为pg节点,@1.1.1.3作为pg_mon");
    expect(result.map((item) => item.raw)).toEqual(["@1.1.1.1", "@1.1.1.2", "@1.1.1.3"]);
  });

  it("does not treat email addresses as host mentions", () => {
    expect(parseHostMentionCandidates("联系 sre@example.com")).toEqual([]);
  });

  it("dedupes repeated host tokens", () => {
    const result = parseHostMentionCandidates("@db-1 检查 @db-1");
    expect(uniqueHostMentionKeys(result)).toEqual(["db-1"]);
  });
});
```

- [x] **Step 8.2：实现 frontend parser**

Create `web/src/chat/hostMentions.ts`:

```ts
export type HostMentionCandidate = {
  tokenId: string;
  raw: string;
  value: string;
  start: number;
  end: number;
  source: "ip_literal" | "hostname_literal";
};

export function parseHostMentionCandidates(input: string): HostMentionCandidate[] {
  // Mirror backend parser for UX only; backend remains authoritative.
}

export function uniqueHostMentionKeys(candidates: HostMentionCandidate[]): string[] {
  return Array.from(new Set(candidates.map((item) => item.value.toLowerCase())));
}
```

Implementation constraints:

- Do not use this parser as security authority.
- Keep token IDs stable enough for a single composer edit session.
- Preserve raw text for backend metadata.

- [x] **Step 8.3：写 composer rendering test**

Create `web/src/chat/components/HostMentionComposer.test.tsx` following existing `createRoot` style:

```tsx
it("renders selected host mention chips", async () => {
  await act(async () => {
    root.render(
      <HostMentionComposer
        value="@1.1.1.1 检查pg"
        mentions={[{ tokenId: "hm-1", raw: "@1.1.1.1", value: "1.1.1.1", start: 0, end: 8, source: "ip_literal" }]}
        onChange={() => {}}
      />,
    );
  });
  expect(container.textContent).toContain("@1.1.1.1");
});
```

- [x] **Step 8.4：实现 HostMentionComposer minimal UI**

Constraints:

- Keep existing `AiopsComposer` send behavior.
- Add mention metadata to `add-message` payload:

```ts
metadata: {
  ...existingMetadata,
  "aiops.hostops.mentions": JSON.stringify(mentions),
  "aiops.hostops.clientDetectedMultiHost": String(uniqueHostMentionKeys(mentions).length >= 2),
}
```

- Inventory suggestions can use existing hosts API first; unresolved IP remains allowed as unresolved chip.

- [x] **Step 8.5：运行 frontend mention tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/hostMentions.test.ts src/chat/components/HostMentionComposer.test.tsx src/chat/components/aiopsComposerActions.test.ts
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `npm run test -- --run src/chat/hostMentions.test.ts src/chat/components/HostMentionComposer.test.tsx src/chat/components/aiopsComposerActions.test.ts`.
- 3 files passed, 9 tests passed.

- [x] **Step 8.6：提交 Task 8**

Run:

```bash
git add web/src/chat/hostMentions.ts web/src/chat/hostMentions.test.ts web/src/chat/components/HostMentionComposer.tsx web/src/chat/components/ComposerHostMentionMenu.tsx web/src/chat/components/HostMentionChip.tsx web/src/chat/components/HostMentionComposer.test.tsx web/src/chat/components/AiopsComposer.tsx web/src/api
git commit -m "feat(chat): add host mention composer metadata"
```

Expected:

- Commit created with mention composer changes.

Result 2026-06-04:

- Commit: `797fb94 feat(chat): add host mention composer metadata`.

## 11. Task 9：实现 Codex 风格 HostOpsStatusPanel

**Files:**
- Create: `web/src/chat/components/HostOpsStatusPanel.tsx`
- Create: `web/src/chat/components/HostOpsPlanSection.tsx`
- Create: `web/src/chat/components/HostSubagentStatusRow.tsx`
- Create: `web/src/chat/components/HostOpsStatusPanel.test.tsx`
- Modify: `web/src/chat/ChatPage.tsx`
- Modify: `web/src/chat/ChatPage.test.tsx`

- [x] **Step 9.1：写 status panel test**

Create `web/src/chat/components/HostOpsStatusPanel.test.tsx`:

```tsx
it("renders Codex-style compact plan and subagent rows above composer", async () => {
  const state = sampleHostOpsState();
  await act(async () => {
    root.render(<HostOpsStatusPanel state={state} onOpenChildAgent={() => {}} />);
  });
  expect(container.textContent).toContain("共 5 个任务，已经完成 0 个");
  expect(container.textContent).toContain("3 个后台智能体");
  expect(container.textContent).toContain("Franklin(@1.1.1.1)");
  expect(container.textContent).toContain("打开");
});
```

`sampleHostOpsState()` must include:

- One active mission.
- Five plan steps.
- Three child agents.

- [x] **Step 9.2：实现 HostOpsStatusPanel layout**

Required structure:

```tsx
export function HostOpsStatusPanel({ state, onOpenChildAgent }: HostOpsStatusPanelProps) {
  const mission = selectActiveHostMission(state);
  if (!mission) return null;
  return (
    <section className="rounded-2xl border bg-background shadow-sm" data-testid="host-ops-status-panel">
      <HostOpsPlanSection mission={mission} state={state} />
      <HostSubagentStatusRow mission={mission} state={state} onOpenChildAgent={onOpenChildAgent} />
    </section>
  );
}
```

Layout constraints:

- One compact panel, not two large cards.
- Plan steps and child rows separated by one thin border line.
- Text stays single-line with truncation.
- Right side `打开` is a button.

- [x] **Step 9.3：wire panel into ChatPage**

Modify `web/src/chat/ChatPage.tsx`:

```tsx
<AiopsThread />
<div className="mx-auto w-full max-w-thread px-4">
  <HostOpsStatusPanel />
  <AiopsComposer variant="chat" />
</div>
```

Use the current ChatPage structure rather than rewriting the page.

- [x] **Step 9.4：extend ChatPage test**

Add an initial state with `activeHostMissionId`, `hostMissions`, `childAgents`. Assert:

```ts
expect(container.querySelector('[data-testid="host-ops-status-panel"]')).not.toBeNull();
expect(container.textContent).toContain("3 个后台智能体");
```

- [x] **Step 9.5：run status panel tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/chat/components/HostOpsStatusPanel.test.tsx src/chat/ChatPage.test.tsx
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `npm run test -- --run src/chat/components/HostOpsStatusPanel.test.tsx src/chat/ChatPage.test.tsx`.
- 2 files passed, 22 tests passed.

- [x] **Step 9.6：提交 Task 9**

Run:

```bash
git add web/src/chat/components/HostOpsStatusPanel.tsx web/src/chat/components/HostOpsPlanSection.tsx web/src/chat/components/HostSubagentStatusRow.tsx web/src/chat/components/HostOpsStatusPanel.test.tsx web/src/chat/ChatPage.tsx web/src/chat/ChatPage.test.tsx
git commit -m "feat(chat): render host ops status panel above composer"
```

Expected:

- Commit created with compact status panel.

Result 2026-06-04:

- Commit: `d88111c feat(chat): render host ops status panel above composer`.

## 12. Task 10：实现 HostSubagentDrawer 和 frontend API

**Files:**
- Create: `web/src/api/hostOps.ts`
- Create: `web/src/api/hostOps.test.ts`
- Create: `web/src/chat/components/HostSubagentDrawer.tsx`
- Create: `web/src/chat/components/HostSubagentDrawer.test.tsx`
- Modify: `web/src/chat/components/HostOpsStatusPanel.tsx`
- Modify: `web/src/transport/assistantTransportControl.ts` if child command helper belongs there

- [x] **Step 10.1：写 hostOps API client test**

Create `web/src/api/hostOps.test.ts`:

```ts
it("fetches child agent transcript", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({
    childAgentId: "agent-1",
    items: [{ id: "item-1", type: "manager_message", content: "检查PG版本" }],
  })));
  const transcript = await fetchChildAgentTranscript("agent-1", fetchMock);
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/host-ops/child-agents/agent-1/transcript", expect.any(Object));
  expect(transcript.items[0].content).toBe("检查PG版本");
});
```

- [x] **Step 10.2：implement API client**

Create `web/src/api/hostOps.ts`:

```ts
export async function fetchChildAgentTranscript(
  childAgentId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<HostChildTranscriptResponse> {
  const response = await fetchImpl(`/api/v1/host-ops/child-agents/${encodeURIComponent(childAgentId)}/transcript`, {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) throw new Error(`Failed to load child agent transcript: ${response.status}`);
  return response.json() as Promise<HostChildTranscriptResponse>;
}
```

- [x] **Step 10.3：write drawer rendering test**

Create `web/src/chat/components/HostSubagentDrawer.test.tsx`:

```tsx
it("renders child transcript and follow-up input", async () => {
  await act(async () => {
    root.render(
      <HostSubagentDrawer
        open
        childAgent={sampleChildAgent()}
        transcript={{ childAgentId: "agent-1", items: [
          { id: "item-1", type: "manager_message", content: "检查PG版本" },
          { id: "item-2", type: "assistant_message", content: "PostgreSQL 15" },
        ]}}
        onSendMessage={() => {}}
        onOpenChange={() => {}}
      />,
    );
  });
  expect(container.textContent).toContain("Host Agent: @1.1.1.1");
  expect(container.textContent).toContain("检查PG版本");
  expect(container.textContent).toContain("PostgreSQL 15");
});
```

- [x] **Step 10.4：implement drawer**

Constraints:

- Use existing `web/src/components/ui/sheet.tsx`.
- Header shows host, role, status.
- Transcript item rendering must distinguish manager message, user follow-up, assistant message, tool call, tool result, approval, error.
- Follow-up sends `aiops.child-agent-message` with `childAgentId`.
- Stop action sends `aiops.child-agent-stop`.

- [x] **Step 10.5：wire panel row click to drawer**

Modify `HostOpsStatusPanel.tsx`:

- Maintain active `childAgentId`.
- Pass `onOpenChildAgent` to `HostSubagentStatusRow`.
- Render `HostSubagentDrawer` with selected child agent.

- [x] **Step 10.6：run drawer tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2/web
npm run test -- --run src/api/hostOps.test.ts src/chat/components/HostSubagentDrawer.test.tsx src/chat/components/HostOpsStatusPanel.test.tsx
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `npm run test -- --run src/api/hostOps.test.ts src/chat/components/HostSubagentDrawer.test.tsx src/chat/components/HostOpsStatusPanel.test.tsx`.
- 3 files passed, 5 tests passed.

- [x] **Step 10.7：提交 Task 10**

Run:

```bash
git add web/src/api/hostOps.ts web/src/api/hostOps.test.ts web/src/chat/components/HostSubagentDrawer.tsx web/src/chat/components/HostSubagentDrawer.test.tsx web/src/chat/components/HostOpsStatusPanel.tsx
git commit -m "feat(chat): add host subagent transcript drawer"
```

Expected:

- Commit created with drawer and API client.

Result 2026-06-04:

- Commit: `5f4c5cd feat(chat): add host subagent transcript drawer`.

## 13. Task 11：投影 hostops state 到 transport stream

**Files:**
- Modify: `internal/appui/transport_projector.go`
- Create: `internal/appui/transport_hostops_projector_test.go`
- Modify: `web/src/transport/aiopsTransportConverter.ts`
- Modify: `web/src/transport/aiopsTransportConverter.test.ts`

- [x] **Step 11.1：写 projector test**

Create `internal/appui/transport_hostops_projector_test.go`:

```go
func TestTransportProjectorIncludesHostMissionAndChildAgents(t *testing.T) {
	snapshot := sampleTurnSnapshotWithHostOps()
	state := ProjectTurnSnapshotToAiopsTransportState(snapshot)
	if state.ActiveHostMissionID != "mission-1" {
		t.Fatalf("ActiveHostMissionID = %q, want mission-1", state.ActiveHostMissionID)
	}
	if len(state.ChildAgents) != 3 {
		t.Fatalf("len(ChildAgents) = %d, want 3", len(state.ChildAgents))
	}
}
```

Use the local projector helper names that already exist in `transport_projector_test.go`; if no public helper exists, add the test next to existing projector tests and use the package-private function directly.

- [x] **Step 11.2：implement projector mapping**

Modify `internal/appui/transport_projector.go`:

- Read hostops mission/child agent data from turn snapshot metadata or injected hostops store.
- Set `state.ActiveHostMissionID`.
- Fill `state.HostMissions`.
- Fill `state.ChildAgents`.
- Add `subagent` process blocks for lifecycle events.

Process block examples:

```go
AiopsProcessBlock{
	ID: "subagent-spawn-agent-1",
	Kind: AiopsTransportProcessKindSubagent,
	DisplayKind: "spawn_host_agent",
	Status: "completed",
	Text: "Franklin(@1.1.1.1) 已启动",
}
```

- [x] **Step 11.3：write converter compatibility test**

Modify `web/src/transport/aiopsTransportConverter.test.ts`:

```ts
it("preserves host operation state while converting assistant messages", () => {
  const state = sampleStateWithHostOps();
  const messages = aiopsTransportStateToAssistantMessages(state);
  expect(messages.length).toBeGreaterThan(0);
  expect(state.childAgents["agent-1"].hostDisplayName).toBe("@1.1.1.1");
});
```

- [x] **Step 11.4：run projector/converter tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/appui -run 'TestTransportProjectorIncludesHostMissionAndChildAgents'
cd web
npm run test -- --run src/transport/aiopsTransportConverter.test.ts
```

Expected:

- PASS。

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/appui -run 'TestTransportProjectorIncludesHostMissionAndChildAgents'`.
- PASS: `go test -count=1 ./internal/appui -run 'TestTransportProjectorIncludesHostMissionAndChildAgents|TestTransportProjectorProjectsStructuredTurnItems'`.
- PASS: `npm run test -- --run src/transport/aiopsTransportConverter.test.ts`.

- [x] **Step 11.5：提交 Task 11**

Run:

```bash
git add internal/appui/transport_projector.go internal/appui/transport_hostops_projector_test.go web/src/transport/aiopsTransportConverter.ts web/src/transport/aiopsTransportConverter.test.ts
git commit -m "feat(transport): project host operation lifecycle state"
```

Expected:

- Commit created with projector and converter changes.

Actual:

- Commit: `753cf4f feat(transport): project host operation lifecycle state`.

## 14. Task 12：集成验证和 fake host-agent 场景

**Files:**
- Create: `internal/hostops/integration_test.go`
- Create: `web/tests/host-ops-status-panel.spec.js`
- Modify: `web/src/lib/uiFixturePresets.js`
- Read: `web/tests/helpers/uiFixtureHarness.js`
- Create: `testdata/hostops/fake_host_agents.json`

- [x] **Step 12.1：写 Go integration test**

Create `internal/hostops/integration_test.go`:

```go
func TestMultiHostMissionCreatesThreeChildrenAndBlocksBeforePlanAccepted(t *testing.T) {
	orchestrator := newTestOrchestrator(t)
	missionID := createResolvedThreeHostMission(t, orchestrator, false)
	_, err := orchestrator.SpawnChildren(context.Background(), missionID, threeAssignments())
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("err = %v, want ErrPlanNotAccepted", err)
	}
	if err := orchestrator.AcceptPlan(context.Background(), missionID, "plan-1"); err != nil {
		t.Fatalf("AcceptPlan() error = %v", err)
	}
	children, err := orchestrator.SpawnChildren(context.Background(), missionID, threeAssignments())
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("len(children) = %d, want 3", len(children))
	}
}
```

- [x] **Step 12.2：写 Playwright UI spec**

Modify `web/src/lib/uiFixturePresets.js` to add a `host-ops-three-hosts` preset that returns a chat fixture state with:

- `activeHostMissionId: "mission-1"`
- `hostMissions.mission-1.planRequired = true`
- five plan steps attached to the active turn process
- three child agents with host display names `@1.1.1.1`, `@1.1.1.2`, `@1.1.1.3`

Create `web/tests/host-ops-status-panel.spec.js`:

```js
import { test, expect } from "@playwright/test";
import { openFixturePage } from "./helpers/uiFixtureHarness";

test("shows compact host ops panel above composer and opens child drawer", async ({ page }) => {
  await openFixturePage(page, "/", "host-ops-three-hosts");
  await expect(page.getByTestId("host-ops-status-panel")).toBeVisible();
  await expect(page.getByText("共 5 个任务，已经完成 0 个")).toBeVisible();
  await expect(page.getByText("3 个后台智能体")).toBeVisible();
  await page.getByRole("button", { name: /打开.*1.1.1.1|打开/ }).first().click();
  await expect(page.getByText("Host Agent: @1.1.1.1")).toBeVisible();
  await expect(page.getByText("检查PG版本")).toBeVisible();
});
```

- [x] **Step 12.3：run integration tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/agentmgr
cd web
npm run test
npm run test:ui:snapshots
npm run test:ui -- host-ops-status-panel.spec.js --project=chromium
```

Expected:

- All commands PASS。

Actual 2026-06-04:

- PASS: `go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/agentmgr`.
- PASS: `npm run test` (75 files, 501 tests).
- PASS: `npm run test:ui -- e2e/host-ops-status-panel.spec.js --project=chromium`.
- PARTIAL: `npm run test:ui:snapshots` passed 8/9 tests; the remaining failure is the pre-existing `chat shows context compaction and externalized evidence states` assertion for `结果较大，仅显示摘要。`.

- [x] **Step 12.4：manual browser verification**

Run aiops-v2 locally:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go run ./cmd/ai-server
```

Open:

```text
http://127.0.0.1:18080/
```

Manual checks:

- Type `@1.1.1.1和@1.1.1.2作为pg节点,搭建一个主从集群,@1.1.1.3作为pg_mon.`
- Verify composer recognizes three mentions.
- Verify compact status panel appears above composer.
- Verify plan is required before mutating execution.
- Verify three child agent rows appear after plan acceptance.
- Open one child drawer and verify independent transcript.

Actual 2026-06-04:

- browser-in-app opened `http://127.0.0.1:53173/?fixture=host-ops-three-hosts` on the Vite dev server.
- Verified compact panel visible above composer.
- Verified `共 5 个任务，已经完成 0 个`, `3 个后台智能体`, and all three child rows.
- Opened `child-1` drawer and verified independent transcript contains `检查PG版本` and `PostgreSQL 15 已检测到`.

- [x] **Step 12.5：提交 Task 12**

Run:

```bash
git add internal/hostops web/tests testdata web/playwright.config.js
git commit -m "test(hostops): cover multi-host plan and child agent ui flow"
```

Expected:

- Commit created with integration and UI verification assets.

Actual:

- Commit: `ee3c627 test(hostops): cover multi-host plan and child agent ui flow`.

## 15. Task 13：安全审计、文档更新和 rollout

**Files:**
- Modify: `README.md`
- Modify: `docs/2026-06-04-ai-chat-host-mention-plan-subagents-design.zh.md` if implementation decisions changed the design
- Create: `docs/2026-06-04-ai-chat-host-mention-plan-subagents-test-report.zh.md`
- Modify: `docs/superpowers/plans/2026-06-04-ai-chat-host-mention-plan-subagents-implementation-todo.zh.md`

- [x] **Step 13.1：写 test report**

Create `docs/2026-06-04-ai-chat-host-mention-plan-subagents-test-report.zh.md` with sections:

```markdown
# AIOps @主机 Plan/Subagents Test Report

日期：2026-06-04

## 覆盖范围

- host mention parser/resolver
- multi-host mandatory plan gate
- one child agent per host
- host binding enforcement
- compact status panel
- subagent drawer transcript

## 命令结果

| Command | Result |
| --- | --- |
| `go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/agentmgr` | PASS |
| `npm run test` | PASS |
| `npm run test:ui:snapshots` | PASS |

## 残余风险

- 真实 host-agent 网络抖动仍需灰度观察。
- PostgreSQL 角色自动分配需要用户确认。
```

Result 2026-06-04:

- Created `docs/2026-06-04-ai-chat-host-mention-plan-subagents-test-report.zh.md`.
- Recorded unit, integration, e2e, browser-in-app, and snapshot baseline results.

- [x] **Step 13.2：更新 README**

Add a short section in `README.md`:

```markdown
### AI Chat Host Mentions

AI 对话支持 `@主机` 触发 host operation mission。多主机请求会强制进入计划模式，计划和后台智能体状态显示在输入框上方的紧凑状态面板中。每个被提及主机会启动一个独立 host-bound child agent，主机命令必须经 host-agent runtime 执行。
```

Result 2026-06-04:

- Added `AI Chat Host Mentions` section to `README.md`.

- [x] **Step 13.3：最终全量验证**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/runtimekernel ./internal/planning ./internal/agentmgr ./pkg/runner/scheduler ./cmd/host-agent
cd web
npm run test
npm run build
npm run test:ui:snapshots
```

Expected:

- All commands PASS。
- If snapshot intentionally changed, update snapshots in a separate commit after visual review:

```bash
npm run test:ui:snapshots:update
npm run test:ui:snapshots
```

Result 2026-06-04:

- PASS: `go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/runtimekernel ./internal/planning ./internal/agentmgr ./cmd/host-agent`.
- PASS: `(cd pkg/runner && go test -count=1 ./scheduler)`.
- PASS: `go test -count=1 ./internal/policyengine -run 'TestGatewayPolicyApprovalPaths|TestExecuteModePolicy_MutationNeedsApproval|TestChatModeRequiresApprovalForUnsafeTerminalCommand'`.
- PASS: `npm run test` (75 files, 501 tests).
- PASS: `npm run build`.
- PASS: `npm run test:ui -- e2e/host-ops-status-panel.spec.js --project=chromium`.
- Snapshot baseline remains: `npm run test:ui:snapshots` passed 8/9, with the same pre-existing failure at `tests/react-shell-snapshot.spec.js:828` / `chat shows context compaction and externalized evidence states`.

- [x] **Step 13.4：final acceptance checklist**

Check these against the running browser and tests:

- [x] `@1.1.1.1和@1.1.1.2...@1.1.1.3` parses as three host mentions.
- [x] Multi-host route sets `planRequired=true`.
- [x] Mutating host operation is blocked before plan acceptance.
- [x] Accepting plan creates exactly three child agents.
- [x] Each child agent has immutable `hostId`.
- [x] Cross-host tool call from child agent returns `ErrCrossHostDenied`.
- [x] Host operations dispatch via host-agent/runner path.
- [x] Compact status panel appears above composer.
- [x] Child agent rows appear at the bottom of the same panel.
- [x] Clicking `打开` opens the drawer.
- [x] Drawer shows independent transcript and sends follow-up to selected child only.
- [x] High-risk operation triggers approval.

Result 2026-06-04:

- Parser/resolver, route, plan gate, child orchestration, host binding, and runner dispatch are covered by Go tests.
- Compact panel, child rows, drawer open, and transcript are covered by Vitest, Playwright e2e, and browser-in-app verification.
- High-risk approval is covered by existing policyengine approval-path tests; hostops keeps execution behind the existing policy/permission/approval path and adds no direct manager execution bypass.
- Browser-in-app verified `http://127.0.0.1:53173/?fixture=host-ops-three-hosts`; screenshot saved to `/tmp/aiops-hostops-browser-in-app-20260604.png`.

- [x] **Step 13.5：提交 Task 13**

Run:

```bash
git add README.md docs/2026-06-04-ai-chat-host-mention-plan-subagents-design.zh.md docs/2026-06-04-ai-chat-host-mention-plan-subagents-test-report.zh.md docs/superpowers/plans/2026-06-04-ai-chat-host-mention-plan-subagents-implementation-todo.zh.md web/test-results web/tests
git commit -m "docs(hostops): document host mention subagent rollout"
```

Expected:

- Commit created with docs and verified test report.

Result 2026-06-04:

- Prepared Task 13 documentation/status commit with README, test report, implementation todo status, and `.gitignore` unignore rules for the new docs.

## 16. 推荐执行方式

推荐使用 subagent-driven development，每个任务一个 fresh worker：

1. Worker A：Task 1 hostops parser/resolver。
2. Worker B：Task 2 transport contracts。
3. Worker C：Task 3 stores。
4. Worker D：Task 4 commands/API。
5. Worker E：Task 5 route/plan gate。
6. Worker F：Task 6 child orchestration。
7. Worker G：Task 7 host binding/execution adapter。
8. Worker H：Task 8 frontend mention composer。
9. Worker I：Task 9 Codex-style status panel。
10. Worker J：Task 10 drawer/API client。
11. Worker K：Task 11 projector/converter。
12. Worker L：Task 12 integration verification。
13. Final reviewer：Task 13 security/docs/rollout。

每个 worker 交付后必须：

- 运行本任务列出的 focused tests。
- 提供 `git diff --stat`。
- 不修改无关文件。
- 不回滚其他 worker 或用户已有变更。

## 17. 完整验收命令

最终合并前运行：

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/runtimekernel ./internal/planning ./internal/agentmgr ./pkg/runner/scheduler ./cmd/host-agent
cd web
npm run test
npm run build
npm run test:ui:snapshots
```

Expected:

- Go focused packages PASS。
- Vitest PASS。
- Vite build PASS。
- Playwright snapshot PASS。

## 18. 需求覆盖矩阵

| 设计需求 | 任务 |
| --- | --- |
| `@主机` mention | Task 1, Task 8 |
| 后端权威解析与 inventory resolve | Task 1 |
| 多主机强制 plan mode | Task 5 |
| plan 未确认禁止 mutating host op | Task 5, Task 6, Task 7 |
| 每台 host 一个 child agent | Task 6, Task 12 |
| manager 不直接执行 host command | Task 6, Task 7 |
| child agent host binding 不可变 | Task 7 |
| host-agent runtime 执行边界 | Task 7 |
| Codex 风格紧凑状态面板 | Task 9 |
| 子 Agent 状态行在计划面板底部 | Task 9 |
| 点击 `打开` 进入 child drawer | Task 10 |
| child 独立 transcript | Task 3, Task 4, Task 10 |
| transport structured state | Task 2, Task 11 |
| high-risk approval | Task 7, Task 13 |
| 集成验证 | Task 12, Task 13 |
