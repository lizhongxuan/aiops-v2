# aiops-v2 Codex-like AI Chat Runtime V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 `docs/superpowers/specs/2026-06-23-aiops-v2-codex-like-ai-chat-runtime-v2-design.zh.md`，把 AI Chat 调整为默认无主机绑定、显式 `@host/@local/@ip` 才执行、能基于用户证据和 WebLearn 做复杂运维分析的 V2 Runtime 闭环。

**Architecture:** 不重写现有 agent runtime 主循环。新增 appui 前置路由、mention 解析、用户证据抽取、per-turn 工具面 metadata、审批拒绝 fallback、WebLearn 策略，并复用现有 HostOps、多主机 Agent、Coroot MCP、OpsManual/Workflow、transport projector 和 Agent UI artifact。

**Tech Stack:** Go appui/runtimekernel/tooling/hostops, React + TypeScript Chat UI, Vitest/Testing Library, Go test, Playwright/browser-in-app, `cmd/agent-eval`。

---

## 1. 实施边界

### 1.1 必须实现

- [x] AI Chat 默认无主机绑定；用户没有显式 `@host/@local/@ip` 时，不把请求绑定到 `server-local`。
- [x] 每一轮用户输入都经过轻量 Intent Router，输出 `chat_advisory`、`evidence_rca`、`host_bound_ops`、`multi_host_ops`。
- [x] Agent profile 可以按轮切换，但对话证据、历史和 opsRun 连续保留。
- [x] 无 host mention 时隐藏或禁用 `exec_command` 等主机执行工具。
- [x] 用户贴出命令输出、日志、监控内容时，优先进入 evidence RCA，而不是要求执行本机命令。
- [x] 用户显式 `@local` 时才绑定 `server-local`。
- [x] 用户显式 `@Coroot` 且 Coroot MCP 正常时才进入 Coroot RCA；Coroot 异常时不阻塞 Chat。
- [x] OpsManual/Workflow 只有高置信命中时才推荐；用户可跳过。
- [x] 用户拒绝或跳过审批后，系统继续基于已有证据做受限分析，不空结束。
- [x] WebLearn 优先使用官方来源；用于解释陌生中间件、版本差异、工具语义。
- [x] 通过 Codex PG timeline 样例 case、host mention case、approval fallback case 做自动化和浏览器验收；PG 只作为验收样例，不进入 runtime 专项逻辑。

### 1.2 明确不做

- [x] 不新建 Case 作为 Chat 起点。
- [x] 不做自愈 Operator 工作台。
- [x] 不把 PG timeline 逻辑写死成专项能力。
- [x] 不新增另一套多主机 Agent；继续复用现有 HostOps/child agent。
- [x] 不把 Coroot 做成所有 RCA 的必经路径。
- [x] 不让 OpsManual/Workflow 低置信“差不多就推荐”。
- [x] 不在 Chat 顶部新增复杂步骤状态条。
- [x] 不默认运行预检。
- [x] 不自动执行 Workflow、手册或 OpsGraph patch。
- [x] 不把用户拒绝审批解释成任务失败后直接结束。

### 1.3 Runtime core 保护线

默认不要修改这些文件：

- [x] `internal/runtimekernel/eino_kernel.go`
- [x] `internal/runtimekernel/dispatch.go`
- [x] `internal/runtimekernel/reconcile.go`
- [x] `internal/runtimekernel/agent_config_runner.go`
- [x] `internal/runtimekernel/agent_final_gate.go`
- [x] `internal/runtimekernel/verification_completion_gate.go`

允许的例外：

- [x] 如果 per-turn 工具面 metadata 无法通过 `internal/tooling/turn_metadata_filter.go` 达到目标，可以小改 `internal/tooling/*`，但不改 runtime loop。
- [x] 如果审批拒绝 fallback 无法在 appui 层实现，可以先写 failing test 证明原因，再单独评审是否最小修改 `ResumeTurn` 的 denied 分支。

## 2. 文件边界

### 2.1 后端新增文件

- [x] Create: `internal/appui/chat_runtime_route.go`  
  负责 Intent Router、route mode、metadata keys、route -> runtime request 的映射。
- [x] Create: `internal/appui/chat_runtime_route_test.go`  
  覆盖无 mention 咨询、用户证据、`@local`、单 host、多 host、显式禁止执行、`@Coroot`。
- [x] Create: `internal/appui/user_evidence_extractor.go`  
  从用户输入中提取命令输出、日志片段、SQL 输出、配置片段、中间件恢复/复制历史相关 evidence hints。
- [x] Create: `internal/appui/user_evidence_extractor_test.go`  
  覆盖数据库恢复样例 fixture、普通日志、空输入、用户明确“只基于输出分析”。
- [x] Create: `internal/appui/chat_tool_surface.go`  
  负责把 route 转成 per-turn metadata：`aiops.route.mode`、`aiops.route.allowsExecCommand`、`toolProfile`、`enableToolPack`。
- [x] Create: `internal/appui/chat_tool_surface_test.go`  
  验证无 host 不允许 exec，`@local` 允许 exec，多 host 进入 hostops tool pack，advisor/evidence 允许 public_web。
- [x] Create: `internal/appui/approval_fallback_controller.go`  
  在审批拒绝/跳过后创建受限分析 follow-up turn，或向 runtime resume 注入 fallback metadata。
- [x] Create: `internal/appui/approval_fallback_controller_test.go`  
  验证拒绝后不会空结束，不再重复同一类主机命令。

### 2.2 后端修改文件

- [x] Modify: `internal/appui/chat_service.go`  
  在构造 `runtimekernel.TurnRequest` 后立即计算 route；移除无显式 host 时的 `server-local` fallback；写入 route/evidence/tool metadata；保留 workflow generation、generic ops repair、hostops route 现有能力。
- [x] Modify: `internal/appui/approval_service.go`  
  接入 `ApprovalFallbackController`；拒绝审批时继续受限分析。
- [x] Modify: `internal/appui/contracts.go`  
  如需在 `TurnResponse` 或 `ChatRunTraceView` 暴露 route summary，只加可选字段。已通过 `ChatRunTraceView` / transport state 增加可选 `routeMode`、`toolSurfaceSummary`，无需修改 core runtime。
- [x] Modify: `internal/hostops/mention_parser.go`  
  支持 `@local`，并确保 `@Coroot` 不被误当 host ops mention。
- [x] Modify: `internal/hostops/resolver.go`  
  将 `@local` 解析为 `server-local`；IP/hostname 多候选时必须返回 ambiguity，不自动猜。
- [x] Modify: `internal/hostops/route.go`  
  继续保持“有有效 host mention 才 host_ops”；不要基于关键词把无 mention 文本升级为 host_ops。已确认当前 `DetectRoute` 只有有效 mention 才进入 `host_ops`。
- [x] Modify: `internal/tooling/turn_metadata_filter.go`  
  支持 per-turn 隐藏 `exec_command`、host mutation、Coroot RCA-only 工具。
- [x] Modify: `internal/integrations/localtools/register.go`  
  保持 `web_search`/`browse_url` 可被 advisor/evidence 使用；`exec_command` 只在 host-bound route 可见。已确认 base registry 保持全集注册，turn metadata filter 按 route 控制可见性。
- [x] Modify: `internal/integrations/coroot/tools.go`  
  若有 RCA 专用工具或 artifact 输出，必须检查 `aiops.coroot.explicitRCA=true`。
- [x] Modify: `internal/integrations/opsmanuals/tools.go`  
  高置信推荐才返回可操作 action；低置信返回 `reference_only` 或 `no_match`。

### 2.3 前端修改文件

- [x] Modify: `web/src/chat/components/AiopsThread.tsx` / `web/src/chat/ChatPage.tsx`  
  Chat 默认不以“单机会话/当前主机”为产品心智；保留 HostOps workspace 展示，但不暗示已绑定 `server-local`。
- [x] Modify: `web/src/chat/components/SessionContextBar.tsx`  
  去掉默认 `host:server-local` 作为 Chat 输入的隐式目标；保留会话切换能力。
- [x] Modify: `web/src/chat/components/AiopsComposer.tsx`  
  发送消息时只把显式 mention 写入 metadata/hostId；无 mention 不带 `hostId`。
- [x] Modify: `web/src/chat/hostMentions.ts`  
  支持 `@local`、`@127.0.0.1`、`@hostAlias` metadata；确保 `@Coroot` 走 Coroot metadata，不走 host mention。
- [x] Modify: `web/src/chat/hostMentionSearch.ts`  
  mention 搜索第一项支持 `@local`；IP/hostname 匹配使用主机清单。
- [x] Modify: `web/src/chat/components/HostMentionSuggestionPopover.tsx`  
  展示 `@local`、主机名、IP、连接状态和歧义提示。
- [x] Modify: `web/src/chat/components/OpsRunSummaryCard.tsx`  
  增加 route mode/target summary 展示，但保持轻量，不新增复杂步骤条。
- [x] Modify: `web/src/transport/aiopsTransportTypes.ts`  
  可选增加 `routeMode`、`targetSummary`、`toolSurfaceSummary`。

### 2.4 测试与验收新增文件

- [x] Create: `testdata/eval_cases/codex_pg_timeline_v2.json`  
  Codex rollout 对照 case：PG timeline 高于主库但无法同步。
- [x] Create: `testdata/eval_cases/codex_pg_timeline_with_evidence_v2.json`  
  带 `pg_controldata`、`pg_is_in_recovery()`、`standby.signal`、日志证据的 RCA case。
- [x] Create: `web/src/chat/ChatPage.runtimeV2.test.tsx`  
  前端默认无主机绑定、mention chip、`@local` 发送 metadata 验收。
- [x] Create: `scripts/verify-aiops-chat-runtime-v2.mjs`  
  Playwright 脚本：打开 `http://127.0.0.1:18083/`，模拟真实用户输入并截屏。

## 3. Phase 0：基线和开关

### Task 0.1：确认当前基线

**Files:**
- Read: `internal/appui/chat_service.go`
- Read: `internal/hostops/route.go`
- Read: `internal/tooling/turn_metadata_filter.go`
- Read: `web/src/chat/ChatPage.tsx`
- Read: `web/src/chat/components/AiopsComposer.tsx`

- [x] Run backend baseline:

```bash
go test ./internal/appui ./internal/hostops ./internal/tooling ./internal/integrations/localtools ./internal/integrations/coroot ./internal/integrations/opsmanuals ./internal/server -count=1
```

Expected: existing tests pass before V2 changes.

- [x] Run frontend baseline:

```bash
cd web && npm test -- ChatPage.test.tsx AiopsComposer.test.tsx HostMentionComposer.test.tsx HostMentionSuggestionPopover.test.tsx
```

Expected: existing tests pass before V2 changes.

- [x] Record current failed browser behavior manually:

```text
Input: pgBackRest 恢复主机 A 后，B 加入 pg_auto_failover 为什么 timeline 比 A 高？
Current bad behavior: requests server-local exec_command such as cat /etc/hosts or pg_controldata.
```

Expected: this behavior is documented as baseline defect, not accepted V2 behavior.

### Task 0.2：增加 V2 feature flags

**Files:**
- Modify: `internal/featureflag/flags.go`
- Test: `internal/featureflag/flags_test.go`

- [x] Add fields to `featureflag.Flags`:

```go
ChatRuntimeV2Enabled       bool
ChatDefaultHostBindingOff  bool
ChatHostMentionEnabled     bool
ChatWebLearnEnabled        bool
```

- [x] Add env constants:

```go
envChatRuntimeV2              = "AIOPS_CHAT_RUNTIME_V2"
envChatDefaultHostBindingOff  = "AIOPS_CHAT_DEFAULT_HOST_BINDING_OFF"
envChatHostMention            = "AIOPS_CHAT_HOST_MENTION"
envChatWebLearn               = "AIOPS_WEB_LEARN"
```

- [x] Parse env values in `FromEnv`.

- [x] Add tests:

```go
func TestFromEnvParsesChatRuntimeV2Flags(t *testing.T) {
  flags := FromEnv(func(key string) string {
    switch key {
    case "AIOPS_CHAT_RUNTIME_V2", "AIOPS_CHAT_DEFAULT_HOST_BINDING_OFF", "AIOPS_CHAT_HOST_MENTION", "AIOPS_WEB_LEARN":
      return "1"
    default:
      return ""
    }
  })
  if !flags.ChatRuntimeV2Enabled || !flags.ChatDefaultHostBindingOff || !flags.ChatHostMentionEnabled || !flags.ChatWebLearnEnabled {
    t.Fatalf("chat runtime v2 flags not parsed: %+v", flags)
  }
}
```

- [x] Run:

```bash
go test ./internal/featureflag -count=1
```

Expected: pass.

## 4. Phase 1：Intent Router 和 route metadata

### Task 1.1：实现 ChatRuntimeRoute

**Files:**
- Create: `internal/appui/chat_runtime_route.go`
- Create: `internal/appui/chat_runtime_route_test.go`

- [x] Define route modes:

```go
type ChatRuntimeRouteMode string

const (
  ChatRouteAdvisory     ChatRuntimeRouteMode = "chat_advisory"
  ChatRouteEvidenceRCA  ChatRuntimeRouteMode = "evidence_rca"
  ChatRouteHostBoundOps ChatRuntimeRouteMode = "host_bound_ops"
  ChatRouteMultiHostOps ChatRuntimeRouteMode = "multi_host_ops"
)
```

- [x] Define route model:

```go
type ChatRuntimeRoute struct {
  Mode                   ChatRuntimeRouteMode
  Reasons                []string
  UserProhibitedHostExec bool
  RequiresHostBinding    bool
  AllowsExecCommand      bool
  AllowsWebLearn         bool
  AllowsCorootRCA        bool
  Confidence             string
}
```

- [x] Implement `BuildChatRuntimeRoute(input string, mentions []hostops.HostMention, evidence UserEvidenceExtraction) ChatRuntimeRoute`.

Route rules:

- [x] No host mention + no user evidence -> `chat_advisory`, `AllowsExecCommand=false`, `AllowsWebLearn=true`.
- [x] User pasted logs/outputs/config -> `evidence_rca`, `AllowsExecCommand=false`, `AllowsWebLearn=true`.
- [x] User says “不要执行命令/只基于输出/不要采集本机” -> force `AllowsExecCommand=false`.
- [x] One resolved host mention -> `host_bound_ops`, `AllowsExecCommand=true`.
- [x] Two or more resolved host mentions -> `multi_host_ops`, `AllowsExecCommand=false` for manager, host child agents execute through HostOps.
- [x] `@Coroot` only sets `AllowsCorootRCA=true` when explicit Coroot mention is present; it does not become host ops.

- [x] Add tests:

```go
func TestBuildChatRuntimeRoutePlainQuestionUsesAdvisory(t *testing.T)
func TestBuildChatRuntimeRoutePastedEvidenceUsesEvidenceRCA(t *testing.T)
func TestBuildChatRuntimeRouteUserProhibitsHostExec(t *testing.T)
func TestBuildChatRuntimeRouteLocalMentionUsesHostBoundOps(t *testing.T)
func TestBuildChatRuntimeRouteMultipleHostsUsesMultiHostOps(t *testing.T)
func TestBuildChatRuntimeRouteCorootMentionDoesNotBecomeHostOps(t *testing.T)
```

- [x] Run:

```bash
go test ./internal/appui -run 'TestBuildChatRuntimeRoute' -count=1
```

Expected: all route tests pass.

### Task 1.2：把 route 写入 request metadata

**Files:**
- Modify: `internal/appui/chat_runtime_route.go`
- Test: `internal/appui/chat_runtime_route_test.go`

- [x] Implement:

```go
func applyChatRuntimeRouteMetadata(req *runtimekernel.TurnRequest, route ChatRuntimeRoute)
```

Metadata contract:

```text
aiops.route.mode
aiops.route.reasons
aiops.route.confidence
aiops.route.allowsExecCommand
aiops.route.allowsWebLearn
aiops.route.allowsCorootRCA
aiops.route.requiresHostBinding
aiops.route.userProhibitedHostExec
```

- [x] Add test:

```go
func TestApplyChatRuntimeRouteMetadata(t *testing.T)
```

Expected metadata example for advisory:

```text
aiops.route.mode=chat_advisory
aiops.route.allowsExecCommand=false
aiops.route.allowsWebLearn=true
```

## 5. Phase 2：默认无主机绑定

### Task 2.1：ChatService 先 route，再决定 HostID

**Files:**
- Modify: `internal/appui/chat_service.go`
- Test: `internal/appui/chat_service_test.go`

Current risky behavior:

```go
if strings.TrimSpace(req.HostID) == "" && req.SessionType == runtimekernel.SessionTypeHost {
  req.HostID = serverLocalHostID
}
```

V2 behavior:

- [x] 先解析 mentions 和 user evidence。
- [x] 构造 route。
- [x] route 为 advisory/evidence 时清空 `req.HostID`。
- [x] route 为 advisory/evidence 且原本是 host session 时，将 runtime request 调整为 workspace execute 或 chat mode，避免 kernel/session 自动继承 host。
- [x] route 为 `host_bound_ops` 时，只使用显式 mention 解析出的 hostId。
- [x] route 为 `multi_host_ops` 时 manager request 不绑定单机 hostId。
- [x] 老功能在 feature flag 未开时保持不变。

Recommended structure:

```go
evidence := ExtractUserEvidence(content)
mentions := s.hostOpsMentionsForCommand(ctx, cmd, content)
route := BuildChatRuntimeRoute(content, mentions, evidence)
applyChatRuntimeRouteMetadata(&req, route)
applyChatRuntimeToolSurfaceMetadata(&req, route)
applyUserEvidenceMetadata(&req, evidence)
applyRouteHostBinding(&req, route, mentions)
```

- [x] Add tests:

```go
func TestChatServiceV2PlainQuestionDoesNotBindServerLocal(t *testing.T)
func TestChatServiceV2EvidenceQuestionDoesNotBindServerLocal(t *testing.T)
func TestChatServiceV2LocalMentionBindsServerLocal(t *testing.T)
func TestChatServiceV2MultipleHostMentionsCreateHostOpsMission(t *testing.T)
func TestChatServiceV2FeatureFlagOffPreservesLegacyServerLocalFallback(t *testing.T)
```

- [x] Run:

```bash
go test ./internal/appui -run 'TestChatServiceV2' -count=1
```

Expected:

- Plain PG timeline question calls runtime with empty `HostID`.
- Metadata contains `aiops.route.mode=chat_advisory`.
- No `aiops.host.id=server-local` metadata.

### Task 2.2：避免 session 旧 HostID 污染新 turn

**Files:**
- Modify: `internal/appui/chat_service.go`
- Test: `internal/appui/chat_service_test.go`

- [x] If active session has `HostID=server-local` from old turns, advisory/evidence route must still clear current turn host binding.
- [x] Add metadata:

```text
aiops.target.binding=none
aiops.target.summary=未绑定主机
```

- [x] Add test:

```go
func TestChatServiceV2IgnoresLegacySessionHostForAdvisory(t *testing.T)
```

Expected: runtime request has empty `HostID` even if session previously selected `server-local`.

## 6. Phase 3：Mention Resolver

### Task 3.1：后端支持 `@local`

**Files:**
- Modify: `internal/hostops/mention_parser.go`
- Modify: `internal/hostops/resolver.go`
- Test: `internal/hostops/mention_parser_test.go`
- Test: `internal/hostops/resolver_test.go`

- [x] `@local` should parse as a host mention with source `local_alias`.
- [x] Resolver maps `@local` to `server-local`.
- [x] `@Coroot` is not returned as host ops mention.
- [x] Emails such as `a@b.com` still not parsed as host mention.

- [x] Tests:

```go
func TestParseHostMentionsIncludesLocalAlias(t *testing.T)
func TestParseHostMentionsSkipsCorootObservabilityMention(t *testing.T)
func TestResolverMapsLocalAliasToServerLocal(t *testing.T)
func TestResolverDoesNotGuessAmbiguousHostname(t *testing.T)
```

- [x] Run:

```bash
go test ./internal/hostops -run 'TestParseHostMentions|TestResolver' -count=1
```

Expected: pass.

### Task 3.2：前端 mention 只提交显式目标

**Files:**
- Modify: `web/src/chat/hostMentions.ts`
- Modify: `web/src/chat/hostMentionSearch.ts`
- Modify: `web/src/chat/components/AiopsComposer.tsx`
- Test: `web/src/chat/hostMentions.test.ts`
- Test: `web/src/chat/hostMentionSearch.test.ts`
- Test: `web/src/chat/components/AiopsComposer.test.tsx`

- [x] `parseHostMentionCandidates` recognizes `@local`.
- [x] `buildHostMentionMetadata` serializes `@local` as host `server-local`.
- [x] `@Coroot` uses `buildCorootMentionMetadata` and is not included in `aiops.hostops.mentions`.
- [x] No mention means no `hostId` in outgoing message payload.

- [x] Test cases:

```ts
it("does not attach hostId when the user sends a plain advisory question")
it("attaches server-local only when the user explicitly mentions @local")
it("keeps @Coroot out of hostops mentions")
```

- [x] Run:

```bash
cd web && npm test -- hostMentions.test.ts hostMentionSearch.test.ts AiopsComposer.test.tsx
```

Expected: pass.

### Task 3.3：Chat 页面文案去掉“当前单机”误导

**Files:**
- Modify: `web/src/chat/ChatPage.tsx`
- Modify: `web/src/chat/components/SessionContextBar.tsx`
- Test: `web/src/chat/ChatPage.test.tsx`
- Test: `web/src/chat/components/SessionContextBar.test.ts`

- [x] Chat first viewport 不再显示“选择单台主机进行 AI Chat”作为默认路径。
- [x] 输入框附近提示保持轻量，例如 placeholder 可表达：“输入问题，使用 @local 或 @主机名 选择执行目标”。
- [x] 不新增复杂顶部状态。
- [x] 保留 HostOpsStatusPanel 在有 mission 时展示。

- [x] Run:

```bash
cd web && npm test -- ChatPage.test.tsx SessionContextBar.test.ts
```

Expected: pass.

## 7. Phase 4：Tool Surface Planner

### Task 4.1：基于 route 设置 per-turn 工具面

**Files:**
- Create: `internal/appui/chat_tool_surface.go`
- Create: `internal/appui/chat_tool_surface_test.go`
- Modify: `internal/appui/chat_service.go`

- [x] Implement:

```go
func applyChatRuntimeToolSurfaceMetadata(req *runtimekernel.TurnRequest, route ChatRuntimeRoute)
```

Metadata contract:

```text
aiops.tool.execCommandAllowed=false
aiops.tool.hostMutationAllowed=false
aiops.tool.corootRCAAllowed=false|true
toolProfile=chat_advisory|evidence_rca|host_bound_ops|multi_host_ops
enableToolPack=public_web
```

Rules:

- [x] `chat_advisory`: no `exec_command`; allow `web_search`.
- [x] `chat_advisory`: allow OpsManual search only as high-confidence recommendation path.
- [x] `evidence_rca`: no `exec_command`; allow `web_search`, `browse_url`, evidence read tools.
- [x] `host_bound_ops`: allow `exec_command` scoped to resolved host.
- [x] `multi_host_ops`: manager cannot direct `exec_command`; manager can use HostOps tool pack and child host agents.

- [x] Run:

```bash
go test ./internal/appui -run 'TestChatRuntimeToolSurface' -count=1
```

Expected: pass.

### Task 4.2：tooling metadata filter 隐藏 exec_command

**Files:**
- Modify: `internal/tooling/turn_metadata_filter.go`
- Test: `internal/tooling/turn_metadata_filter_test.go`

- [x] Extend `IsToolVisibleForTurnMetadata`:

```go
case meta.Name == "exec_command":
  return metadataBool(metadata, "aiops.tool.execCommandAllowed")
```

- [x] Ensure existing opsManual conditions still work.
- [x] Add tests:

```go
func TestTurnMetadataHidesExecCommandWhenRouteDisallowsHostExec(t *testing.T)
func TestTurnMetadataAllowsExecCommandWhenHostBound(t *testing.T)
func TestTurnMetadataKeepsWebSearchVisibleForAdvisor(t *testing.T)
```

- [x] Run:

```bash
go test ./internal/tooling -run 'TestTurnMetadata' -count=1
```

Expected: pass.

### Task 4.3：model input trace 验证工具面

**Files:**
- Test: `internal/runtimekernel/model_input_trace_test.go`
- Test: `internal/runtimekernel/react_loop_test.go`

- [x] Add or update test to run a turn with metadata:

```text
aiops.route.mode=chat_advisory
aiops.tool.execCommandAllowed=false
```

- [x] Assert prompt/tool surface does not contain `exec_command`.
- [x] Assert `web_search` remains visible or discoverable.

- [x] Run:

```bash
go test ./internal/runtimekernel -run 'Test.*ToolSurface.*ExecCommand|Test.*ModelInput.*ToolSurface' -count=1
```

Expected: pass.

## 8. Phase 5：用户证据抽取

### Task 5.1：实现 UserEvidenceExtractor

**Files:**
- Create: `internal/appui/user_evidence_extractor.go`
- Create: `internal/appui/user_evidence_extractor_test.go`

- [x] Implement extraction result:

```go
type UserEvidenceExtraction struct {
  HasEvidence      bool
  UserProhibitsExec bool
  EvidenceKinds    []string
  Commands         []string
  Signals          []string
  RawExcerpt       string
}
```

Generic evidence kinds:

- [x] `command_output`
- [x] `log`
- [x] `sql_result`
- [x] `config`
- [x] `monitoring`
- [x] `stack_trace`

中间件恢复/复制历史 hints are allowed as generic signals, not as a PostgreSQL hardcoded solution:

- [x] `database_recovery_inactive`
- [x] `replica_marker_missing`
- [x] `history_branch_id`
- [x] `archive_recovery_completed`
- [x] `restore_command_configured`
- [x] `recovery_target_history_branch_configured`

- [x] Add tests:

```go
func TestExtractUserEvidenceDetectsPGTimelineEvidence(t *testing.T)
func TestExtractUserEvidenceDetectsNoExecInstruction(t *testing.T)
func TestExtractUserEvidenceDetectsPlainLogBlock(t *testing.T)
func TestExtractUserEvidenceReturnsEmptyForShortQuestion(t *testing.T)
```

- [x] Run:

```bash
go test ./internal/appui -run 'TestExtractUserEvidence' -count=1
```

Expected: pass.

### Task 5.2：把用户证据注入 route 和 prompt metadata

**Files:**
- Modify: `internal/appui/chat_service.go`
- Modify: `internal/appui/chat_runtime_route.go`
- Test: `internal/appui/chat_service_test.go`

- [x] For evidence route, set metadata:

```text
aiops.userEvidence.present=true
aiops.userEvidence.kinds=command_output,sql_result
aiops.userEvidence.signals=database_recovery_inactive,history_branch_id
aiops.userEvidence.rawExcerpt=<truncated excerpt>
```

- [x] Add test:

```go
func TestChatServiceV2PastedEvidenceSetsEvidenceMetadata(t *testing.T)
```

Expected:

- route mode is `evidence_rca`.
- `HostID` is empty.
- `aiops.userEvidence.present=true`.
- `execCommandAllowed=false`.

## 9. Phase 6：WebLearn 官方来源策略

### Task 6.1：Advisor/Evidence 自动允许 public_web

**Files:**
- Modify: `internal/appui/chat_tool_surface.go`
- Test: `internal/appui/chat_tool_surface_test.go`

- [x] For `chat_advisory` and `evidence_rca`, append `public_web` to `enableToolPack` when `AIOPS_WEB_LEARN=1`.
- [x] Do not require web search for every question; only expose capability and add prompt guidance.
- [x] Do not expose host execution tools just because WebLearn is enabled.

Expected metadata:

```text
enableToolPack=public_web
aiops.weblearn.enabled=true
aiops.weblearn.sourcePolicy=official_first
```

### Task 6.2：补 WebLearn prompt/context 指令

**Files:**
- Modify: `internal/appui/chat_tool_surface.go`
- Modify: `internal/promptcompiler/developer_rules.go` only if appui metadata cannot provide enough guidance.
- Test: `internal/runtimekernel/model_input_trace_test.go`

- [x] Prompt rule must say:

```text
遇到陌生中间件、工具版本、系统命令语义、网络或操作系统差异时，优先使用 web_search/browse_url 查官方文档或项目文档。
不要用 exec_command/bash/python 代替 WebLearn。
引用资料时说明适用版本和来源 URL。
```

- [x] Add trace assertion that advisor/evidence prompt contains `official` or `官方来源` guidance when WebLearn is enabled.

Run:

```bash
go test ./internal/runtimekernel -run 'Test.*ModelInput.*WebLearn|Test.*Prompt.*WebLearn' -count=1
```

Expected: pass.

## 10. Phase 7：审批拒绝 fallback

### Task 7.1：设计 appui 层 fallback，不先改 runtime denied branch

**Files:**
- Create: `internal/appui/approval_fallback_controller.go`
- Create: `internal/appui/approval_fallback_controller_test.go`
- Modify: `internal/appui/approval_service.go`

- [x] When user rejects approval, first let existing approval decision path mark the pending approval as denied.
- [x] Then create a new follow-up turn in the same session with input:

```text
用户拒绝了上一步主机命令或高风险操作。不要再次请求同类命令；请基于已有用户证据、已完成工具结果和公开资料继续做受限分析，明确说明哪些结论受限。
```

- [x] Follow-up turn metadata:

```text
aiops.approvalFallback=true
aiops.approvalFallback.decision=denied
aiops.approvalFallback.originalTurnId=<turn>
aiops.tool.execCommandAllowed=false
aiops.route.mode=evidence_rca
resume.input=<fallback text>
```

- [x] If existing runtime can resume denied approvals with fallback metadata after a minimal safe change, write failing test first and keep change scoped.

- [x] Tests:

```go
func TestApprovalFallbackCreatesEvidenceOnlyFollowupAfterDeniedExec(t *testing.T)
func TestApprovalFallbackDisablesExecCommandForFollowup(t *testing.T)
func TestApprovalFallbackDoesNotRunForApprovedDecision(t *testing.T)
```

- [x] Run:

```bash
go test ./internal/appui -run 'TestApprovalFallback' -count=1
```

Expected: pass.

### Task 7.2：前端拒绝后不显示硬失败

**Files:**
- Modify: `web/src/chat/components/AiopsComposer.tsx`
- Modify: `web/src/chat/components/ProcessTranscript.tsx`
- Test: `web/src/chat/ChatPage.test.tsx`
- Test: `web/src/chat/components/ProcessTranscript.test.tsx`

- [x] 拒绝审批后，UI 可以显示“已拒绝，将基于已有证据继续分析”。
- [x] 不把 `approval denied` 作为红色全局错误打断对话。
- [x] 保留 rejected process block 作为审计。

Run:

```bash
cd web && npm test -- ChatPage.test.tsx ProcessTranscript.test.tsx AiopsComposer.test.tsx
```

Expected: pass.

## 11. Phase 8：OpsManual/Workflow 高置信推荐

### Task 8.1：确保低置信不推荐执行

**Files:**
- Modify: `internal/opsmanual/scoring.go`
- Modify: `internal/opsmanual/candidate_filter.go`
- Modify: `internal/integrations/opsmanuals/tools.go`
- Test: `internal/opsmanual/scoring_test.go`
- Test: `internal/integrations/opsmanuals/tools_test.go`

- [x] High confidence requires:

```text
resource kind match
symptom/failure type match
environment/version compatible or explicitly unknown-safe
required evidence present
target scope unambiguous
usage stats available when recommending execution
```

- [x] Return decisions:

```text
direct_execute      only when all high-confidence conditions pass
adapt               high confidence but needs user parameter confirmation
reference_only      useful but not safe to recommend as direct path
need_info           missing important target/evidence
no_match            no trustworthy candidate
```

- [x] Tests must assert PG timeline generic question does not receive unrelated Redis/Linux manual.

Run:

```bash
go test ./internal/opsmanual ./internal/integrations/opsmanuals -run 'Test.*Scoring|Test.*OpsManual' -count=1
```

Expected: pass.

### Task 8.2：前端推荐卡只展示明确边界

**Files:**
- Modify: `web/src/chat/components/OpsManualChatArtifacts.tsx`
- Test: `web/src/chat/components/OpsManualChatArtifacts.test.tsx`

- [x] 推荐卡必须展示：

```text
为什么匹配
适用边界
不适用边界
使用次数
成功次数
成功率
用户可选：使用 / 仅参考 / 不使用
```

- [x] `reference_only` 不展示“使用该手册/Workflow”主按钮。
- [x] 不展示“运行预检”为第一版默认步骤。

Run:

```bash
cd web && npm test -- OpsManualChatArtifacts.test.tsx
```

Expected: pass.

## 12. Phase 9：Coroot RCA gate

### Task 9.1：确认 `@Coroot` 才进入 RCA

**Files:**
- Modify: `internal/appui/chat_run_trace.go`
- Modify: `internal/appui/chat_runtime_route.go`
- Modify: `internal/integrations/coroot/tools.go`
- Test: `internal/appui/chat_runtime_route_test.go`
- Test: `internal/integrations/coroot/tools_test.go`

- [x] `@Coroot` sets:

```text
aiops.coroot.explicitRCA=true
aiops.route.allowsCorootRCA=true
```

- [x] Coroot MCP unavailable returns skip reason:

```text
coroot_mcp_unavailable  # done
target_not_matched      # done
time_window_not_matched # done
empty_data              # done
```

- [x] Without `@Coroot`, Coroot data may be evidence but not RCA artifact.

Run:

```bash
go test ./internal/appui ./internal/integrations/coroot -run 'Test.*Coroot|Test.*RCA' -count=1
```

Expected: pass.

## 13. Phase 10：Eval 和浏览器验收

### Task 10.1：新增 Codex PG timeline eval case

**Files:**
- Create: `testdata/eval_cases/codex_pg_timeline_v2.json`
- Create: `testdata/eval_cases/codex_pg_timeline_with_evidence_v2.json`
- Modify: `cmd/agent-eval/main_test.go` only if case discovery needs update.

- [x] Case 1 input: 用户第一轮只问 PG timeline 为什么更高。
- [x] Expected:

```text
不能调用 exec_command
不能绑定 server-local
必须解释 timeline 高不是数据更新，而是 WAL 历史分支
必须给出候选原因：B 非空/被提升/pgBackRest latest/auto.conf 恢复残留/旧 stanza 混写
必须建议查官方资料或实际版本文档
```

- [x] Case 2 input: 用户贴 A/B/C 证据。
- [x] Expected:

```text
判断 B 仍在恢复/standby，但处在与 A 不兼容的 timeline 分叉上
依据 pg_is_in_recovery=true、standby.signal 存在、B timeline 高于 A、日志 not a child
说明 B 的 timeline 不是 A 当前 timeline 的子历史
给出安全重建 B 流程
给出加入后的验收命令
```

- [x] Run:

```bash
go test ./cmd/agent-eval -count=1
```

Expected: pass.

### Task 10.2：服务端集成验收

**Files:**
- Test: `internal/server/assistant_transport_api_test.go`
- Test: `internal/server/chat_api_test.go`

- [x] Add test: plain PG question submitted through assistant transport does not include `hostId=server-local`.
- [x] Add test: `@local` submitted through assistant transport includes `hostId=server-local`.
- [x] Add test: approval reject generates fallback follow-up or fallback metadata.

Run:

```bash
go test ./internal/server -run 'Test.*ChatRuntimeV2|Test.*AssistantTransport.*RuntimeV2' -count=1
```

Expected: pass.

### Task 10.3：Playwright/browser-in-app 真实用户流

**Files:**
- Create: `scripts/verify-aiops-chat-runtime-v2.mjs`
- Test manually with Browser plugin at `http://127.0.0.1:18083/`

- [x] Scenario A: plain advisory

```text
Input: 我用 pgbackrest 恢复主机A后，从节点执行 pg_autoctl create postgres 后 timeline 比主机A高，为什么？
Expected UI: no approval prompt, no server-local command block, answer continues with analysis.
```

- [x] Scenario B: evidence RCA

```text
Input: 不要执行命令，只基于下面输出分析：pg_is_in_recovery=false ... no standby.signal ... timeline=11
Expected UI: no approval prompt, answer identifies B is not standby.
```

- [x] Scenario C: explicit local

```text
Input: @local 帮我只读检查 uname 和当前目录
Expected UI: host-bound route, approval behavior follows existing policy, command target is server-local.
```

- [x] Scenario D: multi-host

```text
Input: @hostA @hostB 对比 PG 状态
Expected UI: HostOps mission appears; manager/child agent path is used.
```

- [x] Scenario E: approval reject

```text
Action: reject a requested command.
Expected UI: rejected block remains, conversation continues with limited evidence analysis.
```

Run:

```bash
node scripts/verify-aiops-chat-runtime-v2.mjs
```

Expected: script exits 0 and writes screenshots to `.data/browser-verify-chat-runtime-v2/`.

## 14. Phase 11：部署验证

### Task 11.1：完整测试

- [x] Run Go targeted suites:

```bash
go test ./internal/appui ./internal/hostops ./internal/tooling ./internal/runtimekernel ./internal/integrations/localtools ./internal/integrations/coroot ./internal/integrations/opsmanuals ./internal/opsmanual ./internal/server -count=1
```

- [x] Run frontend targeted suites:

```bash
cd web && npm test -- ChatPage.test.tsx ChatPage.runtimeV2.test.tsx AiopsComposer.test.tsx HostMentionComposer.test.tsx HostMentionSuggestionPopover.test.tsx OpsManualChatArtifacts.test.tsx ProcessTranscript.test.tsx
```

- [x] Build frontend:

```bash
cd web && npm run build
```

- [x] Start aiops-v2 locally:

```bash
./scripts/start.sh
```

Expected: service reachable at `http://127.0.0.1:18083/`.

### Task 11.2：上线开关默认值

- [x] Local/dev default:

```text
AIOPS_CHAT_RUNTIME_V2=1
AIOPS_CHAT_DEFAULT_HOST_BINDING_OFF=1
AIOPS_CHAT_HOST_MENTION=1
AIOPS_WEB_LEARN=1
```

- [x] Production rollout:

```text
Phase A: AIOPS_CHAT_RUNTIME_V2=1, default host binding off only for new sessions
Phase B: enable WebLearn
Phase C: enable approval fallback follow-up
Phase D: remove legacy default server-local binding from Chat UI
```

## 15. 状态跟踪

| Phase | 状态 | 完成标准 |
| --- | --- | --- |
| Phase 0 基线和开关 | 已完成 | flags 可用，baseline 已记录 |
| Phase 1 Intent Router | 已完成 | route unit tests pass |
| Phase 2 默认无主机绑定 | 已完成 | plain/evidence turn 不绑定 server-local |
| Phase 3 Mention Resolver | 已完成 | `@local` 可用，`@Coroot` 不误判 host |
| Phase 4 Tool Surface | 已完成 | 无 host 时 prompt/tool surface 无 `exec_command` |
| Phase 5 用户证据 | 已完成 | 中间件恢复历史/log/monitoring/stack_trace evidence route 生效 |
| Phase 6 WebLearn | 已完成 | advisor/evidence 已暴露 public_web、official_first metadata 与 prompt guidance |
| Phase 7 审批 fallback | 已完成 | 拒绝后继续受限分析 |
| Phase 8 OpsManual/Workflow | 已完成 | PG timeline 不误推 Redis；reference_only 不展示执行/预检主按钮；完整成功率/边界展示已补 |
| Phase 9 Coroot RCA gate | 已完成 | 只有 `@Coroot` 才 RCA；`coroot_mcp_unavailable`、`target_not_matched`、`time_window_not_matched`、`empty_data` skip reason 已补 |
| Phase 10 Eval/Browser | 已完成 | eval case、脚本、Playwright 与 browser-in-app 已跑 |
| Phase 11 部署验证 | 已完成 | 本地 `18083` 已用 V2 flags 重新部署；生产 rollout 计划已记录 |

## 16. 建议并行分工

- [x] Agent A 后端路由边界：`internal/appui/chat_runtime_route.go`、`chat_service.go`、`chat_tool_surface.go`、相关 tests。
- [x] Agent B Mention/UI：`internal/hostops/*mention*`、`web/src/chat/hostMentions.ts`、`AiopsComposer.tsx`、`SessionContextBar.tsx`。
- [x] Agent C Evidence/WebLearn/OpsManual：`user_evidence_extractor.go`、`internal/tooling/turn_metadata_filter.go`、`internal/integrations/opsmanuals/*`。
- [x] Agent D 验收：`testdata/eval_cases/*`、`scripts/verify-aiops-chat-runtime-v2.mjs`、Browser/Playwright 测试。

冲突规则：

- [x] 同一时间只能一个 agent 修改 `internal/appui/chat_service.go`。
- [x] 同一时间只能一个 agent 修改 `web/src/chat/components/AiopsComposer.tsx`。
- [x] Runtime core 文件需要主 agent 审批后才能改。

## 17. 最终验收标准

- [x] 用户直接问中间件恢复/复制历史类原理或故障原因，系统不再请求执行 `cat /etc/hosts`、`pg_controldata /var/lib/pgsql/data` 这类本机命令。
- [x] 用户贴证据并说明“不执行命令”时，系统进入 evidence RCA，能给出根因候选和受限结论。
- [x] 用户输入 `@local` 时，才进入本机执行上下文。
- [x] 用户输入多个主机 mention 时，继续复用现有多主机 Agent。
- [x] 用户 `@Coroot` 且 MCP 正常时，才进入 Coroot RCA。
- [x] 用户拒绝审批后，系统不会空结束，也不会重复要求同类命令。
- [x] OpsManual/Workflow 推荐必须有清晰边界、使用次数、成功率和用户确认按钮。
- [x] 浏览器验收通过，截图保留在 `.data/browser-verify-chat-runtime-v2/`。
