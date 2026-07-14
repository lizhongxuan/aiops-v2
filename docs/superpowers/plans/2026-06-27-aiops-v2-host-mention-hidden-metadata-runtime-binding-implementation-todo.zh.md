# aiops-v2 Host Mention Hidden Metadata Runtime Binding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 当用户通过 `@` 主机列表选择主机时，前端随消息携带隐藏的主机身份信息，agent runtime 能稳定识别目标主机并在该主机上执行允许的只读命令。

**Architecture:** 主路径为 `HostInventory suggestion -> selected mention hidden metadata -> aiops.hostops.mentions -> server-side HostRepository resolution -> ChatRuntimeRoute(host_bound_ops) -> TurnRequest.HostID -> prompt host_scope bound -> exec_command visible`。文本中的 `@server-local`、`@host`、`@IP` 只作为可读显示和兜底解析；前端隐藏 metadata 是用户选择证据，不是执行授权事实，runtime 只能以服务端重新解析确认后的 host mention 作为绑定事实。

**Tech Stack:** React/TypeScript chat composer, npm/Vitest/React Testing Library, Go appui hostops routing, Go runtimekernel/tooling tests, Playwright/browser-in-app manual verification.

---

## Scope Rules

- [ ] 只绑定用户显式 `@` 选择或输入的主机；裸文本 `server-local 查看 CPU` 不允许自动绑定。
- [ ] `@` 主机列表选中项必须携带 `hostId`、`address`、`displayName`、`source`、`resolved` 等隐藏信息。
- [ ] `@server-local`、`@local`、`@localhost`、`@127.0.0.1` 都必须稳定映射到 `hostId=server-local`。
- [ ] 单主机绑定进入 `host_bound_ops`，允许 `exec_command` 只读检查；多主机仍进入多主机 plan/manager 路径。
- [ ] 不新增第二套 agent runtime 逻辑；只补齐现有 `aiops.hostops.mentions` 到 `TurnRequest.HostID` 的单一路径。
- [ ] 不放宽 terminal policy；CPU 检查只使用现有允许的只读命令，如 `uptime`、`top`、`lscpu`、`nproc`。

## Risk Boundaries

- [ ] **客户端 metadata 不可信。** `aiops.hostops.mentions` 可以帮助服务端知道用户从哪个 `@` 列表项选择了主机，但服务端必须用 `HostRepository` 重新解析并确认 `hostId/address/displayName`，不能因为客户端传了 `resolved=true` 就直接绑定执行。
- [ ] **伪造 hostId 必须失败关闭。** 如果 metadata 写入不存在的 `hostId`、不可执行主机、非终端主机，route 必须降级为 `chat_advisory` 或明确要求重新选择主机，不能暴露 `exec_command`。
- [ ] **过期选择必须失效。** 用户选择 `@server-local` 后如果继续编辑或删除该 token，composer 发送时必须重新用当前文本交叉校验隐藏 metadata；当前文本不存在的 selected metadata 不能继续绑定主机。
- [ ] **显示文本和隐藏身份不一致时服务端优先校验身份。** 例如 visible raw 是 `@server-local`，metadata 却带 `hostId=host-a`，服务端必须通过 inventory resolution 识别不一致并拒绝或按服务端解析结果处理，不能让隐藏字段偷偷改目标。
- [ ] **本地主机别名只在显式 `@` 下生效。** `@server-local`、`@localhost`、`@127.0.0.1` 可以映射到 `server-local`；裸文本 `server-local 查看 CPU` 仍然不能自动绑定。
- [ ] **多主机不走单主机直连。** 同一输入中出现两个及以上不同 resolved host 时，必须进入 multi-host plan/manager 路径，不能任选一个 host 执行。
- [ ] **特殊 @ 工具不是主机。** `@coroot`、`@ops_graph`、`@ops_manual` 继续走 special mention，不进入 hostops mention metadata。
- [ ] **变更命令仍需要审批。** 本任务只解决只读主机检查可用性，不改变 mutation approval、terminal policy、ActionToken 和高风险命令拦截。

## Implementation Usability Notes

- [ ] 当前仓库前端包在 `web/` 下，锁文件是 `web/package-lock.json`；本文所有前端测试命令使用 `npm --prefix web ...`，不要使用根目录 `npm run build` 或 `pnpm --dir web ...`。
- [ ] 如果新增 Playwright `.spec.js` 文件，必须确认 `web/vitest.config.js` 排除它，避免 `npm --prefix web test` 把 Playwright 测试误当 Vitest 执行。
- [ ] TypeScript source union 要同步扩展：前端 selected hidden metadata 需要支持 `source: "inventory"`，不能只保留 `ip_literal | hostname_literal | local_alias`。
- [ ] Go 侧 `HostMentionSourceInventory` 已存在；实现时复用现有 `hostops.HostMentionSource` 常量，不要发明新的 source 字符串。
- [ ] 浏览器验证必须同时看 UI 结果和 outgoing transport/runtime trace，不能只看消息气泡里出现 `@server-local`。

## File Structure

**Modify**

- `web/src/chat/hostMentions.ts`：定义并序列化从主机列表选中的隐藏 mention 元数据。
- `web/src/chat/hostMentionSearch.ts`：保证 suggestion 包含稳定 `hostId/address/displayName`，本机 suggestion 与 inventory suggestion 形状一致。
- `web/src/chat/components/AiopsComposer.tsx`：保存用户选中的 mention 隐藏信息，并在发送消息时合并到 `aiops.hostops.mentions`。
- `internal/hostops/mention_parser.go`：保证本地主机别名解析与前端一致，用作无 metadata 时的兜底。
- `internal/appui/chat_service.go`：metadata mention 优先于 raw 文本解析，但必须经 HostRepository 重新确认后才能生成 host-bound route。
- `internal/appui/transport_commands.go`：assistant transport fallback 不丢弃显式本地主机 mention。

**Test**

- `web/src/chat/hostMentions.test.ts`
- `web/src/chat/hostMentionSearch.test.ts`
- `web/src/chat/components/AiopsComposer.test.tsx`
- `internal/hostops/mention_parser_test.go`
- `internal/appui/chat_service_test.go`
- `internal/server/assistant_transport_api_test.go`
- `internal/runtimekernel/intent_tool_packs_test.go`

---

### Task 1: Lock the Host Mention Metadata Contract

**Files:**
- Modify: `web/src/chat/hostMentions.ts`
- Test: `web/src/chat/hostMentions.test.ts`

- [x] **Step 1: Add failing tests for hidden selected-host metadata**

Add tests that assert selected host metadata is serialized even when visible text is only `@server-local`:

```ts
it("serializes selected inventory host metadata as resolved host mention", () => {
  const mentions = parseHostMentionCandidates("@server-local 查看 CPU");
  const selected = [{
    raw: "@server-local",
    value: "server-local",
    hostId: "server-local",
    address: "server-local",
    displayName: "server-local",
    source: "inventory",
    resolved: true,
    confidence: 1,
  }];

  expect(JSON.parse(buildHostMentionMetadata(mentions, selected)["aiops.hostops.mentions"])).toEqual([
    expect.objectContaining({
      raw: "@server-local",
      value: "server-local",
      hostId: "server-local",
      address: "server-local",
      displayName: "server-local",
      source: "inventory",
      resolved: true,
    }),
  ]);
});

it("normalizes explicit local aliases to server-local metadata", () => {
  for (const raw of ["@local", "@server-local", "@localhost", "@127.0.0.1"]) {
    const metadata = buildHostMentionMetadata(parseHostMentionCandidates(`${raw} 查看 CPU`));
    const [mention] = JSON.parse(metadata["aiops.hostops.mentions"]);
    expect(mention).toEqual(expect.objectContaining({
      hostId: "server-local",
      value: "server-local",
      source: "local_alias",
    }));
  }
});
```

- [x] **Step 2: Run the focused test and confirm it fails**

Run:

```bash
npm --prefix web run test -- src/chat/hostMentions.test.ts
```

Expected: FAIL because `buildHostMentionMetadata` does not accept selected hidden metadata yet, and `@server-local` is not normalized as local alias.

- [x] **Step 3: Implement a single metadata builder contract**

Update `hostMentions.ts` so `buildHostMentionMetadata(candidates, selectedMetadata?)` merges by normalized mention value:

- define a shared source union: `"ip_literal" | "hostname_literal" | "local_alias" | "inventory"`;
- define a metadata item type that includes `tokenId/raw/value/start/end/hostId/address/displayName/source/resolved/confidence`;
- selected hidden metadata wins when `hostId` is present.
- explicit local aliases normalize to `hostId=server-local`.
- manually typed inventory/IP mentions still serialize as before.
- emitted JSON item fields are stable: `tokenId/raw/value/start/end/hostId/address/displayName/source/resolved/confidence`.
- merge selected metadata only when the current parsed text still contains the same normalized mention token; deleted or edited tokens must not be serialized.

- [x] **Step 4: Run the focused test and confirm it passes**

Run:

```bash
npm --prefix web run test -- src/chat/hostMentions.test.ts
```

Expected: PASS.

---

### Task 2: Preserve Hidden Host Metadata When the User Selects an @ Suggestion

**Files:**
- Modify: `web/src/chat/hostMentionSearch.ts`
- Modify: `web/src/chat/components/AiopsComposer.tsx`
- Test: `web/src/chat/hostMentionSearch.test.ts`
- Test: `web/src/chat/components/AiopsComposer.test.tsx`

- [x] **Step 1: Add failing composer tests for selected host list items**

Add a test that simulates choosing the `server-local` host from the suggestion list, then sending:

```ts
it("sends hidden host metadata when selecting server-local from @ host list", async () => {
  await renderComposer(root);
  const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;

  await typeInComposer(input, "@ser");
  await act(async () => {
    container.querySelector('[data-testid="host-mention-suggestion-item"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await act(async () => {
    container.querySelector('[data-testid="omnibar-primary-action"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });

  const command = mockState.sendCommand.mock.calls.at(-1)?.[0];
  expect(command.message.hostId).toBe("server-local");
  expect(JSON.parse(command.message.metadata["aiops.hostops.mentions"])).toEqual([
    expect.objectContaining({
      hostId: "server-local",
      displayName: expect.any(String),
      resolved: true,
    }),
  ]);
});
```

- [x] **Step 2: Run the focused composer test and confirm it fails**

Run:

```bash
npm --prefix web run test -- src/chat/components/AiopsComposer.test.tsx
```

Expected: FAIL because suggestion selection currently replaces visible text but does not persist a selected-host hidden metadata record.

- [x] **Step 3: Store selected suggestion metadata in the composer**

In `AiopsComposer.tsx`:

- Add state or ref for selected host mention metadata keyed by normalized mention value.
- In `applySuggestion`, after `replaceActiveHostMention`, store `{ raw, value, hostId, address, displayName, source: "inventory", resolved: true, confidence: 1 }`.
- Clear this hidden metadata when the composer clears after submit.
- Derive `selectedHostMentions` from current text plus hidden metadata, so deleted mention tokens do not continue to bind a host.
- If the user manually edits the mention after selection, drop the selected hidden metadata unless the current parsed token still matches the same normalized key.

- [x] **Step 4: Keep local suggestion and inventory suggestion metadata consistent**

In `hostMentionSearch.ts`, ensure both `buildLocalSuggestion` and inventory `buildSuggestion` expose enough data for the composer:

- local suggestion: `hostId=server-local`, `address=server-local`, `label=local`, `displayName=server-local`, `source=local_alias`.
- inventory suggestion: stable `hostId`, display label, address, status, `displayName`, `source=inventory`.

- [x] **Step 5: Run frontend tests**

Run:

```bash
npm --prefix web run test -- src/chat/hostMentions.test.ts src/chat/hostMentionSearch.test.ts src/chat/components/AiopsComposer.test.tsx
```

Expected: PASS.

---

### Task 3: Validate Explicit Metadata Before Host Binding

**Files:**
- Modify: `internal/appui/chat_service.go`
- Modify: `internal/hostops/mention_parser.go`
- Test: `internal/appui/chat_service_test.go`
- Test: `internal/hostops/mention_parser_test.go`

- [x] **Step 1: Add failing backend tests for hidden metadata binding**

Add a `ChatService` test that sends content `@server-local 查看 CPU 情况` with metadata:

```go
Metadata: map[string]string{
	"aiops.hostops.mentions": `[{"raw":"@server-local","value":"server-local","hostId":"server-local","address":"server-local","displayName":"server-local","source":"inventory","resolved":true,"confidence":1}]`,
},
```

Expected assertions:

- `runReq.HostID == "server-local"`
- `runReq.SessionType == runtimekernel.SessionTypeHost`
- `runReq.Metadata["aiops.route.mode"] == "host_bound_ops"`
- `runReq.Metadata["aiops.target.binding"] == "host"`
- `runReq.Metadata["aiops.tool.execCommandAllowed"] == "true"`

- [x] **Step 2: Add failing backend tests for forged metadata**

Add a `ChatService` test that sends visible content `@server-local 查看 CPU 情况` but forged metadata:

```go
Metadata: map[string]string{
	"aiops.hostops.mentions": `[{"raw":"@server-local","value":"server-local","hostId":"does-not-exist","address":"10.255.255.255","displayName":"server-local","source":"inventory","resolved":true,"confidence":1}]`,
},
```

Expected assertions:

- `runReq.HostID == ""`
- `runReq.SessionType == runtimekernel.SessionTypeWorkspace`
- `runReq.Metadata["aiops.target.binding"] == "none"`
- `runReq.Metadata["aiops.tool.execCommandAllowed"] == "false"`

This test must fail before implementation if current code trusts client `HostID` or client `resolved=true`.

- [x] **Step 3: Add failing parser tests for explicit local aliases**

Add table-driven tests:

```go
for _, input := range []string{"@local 查看 CPU", "@server-local 查看 CPU", "@localhost 查看 CPU", "@127.0.0.1 查看 CPU"} {
	mentions := ParseHostMentions(input)
	if len(mentions) != 1 || mentions[0].Source != HostMentionSourceLocalAlias {
		t.Fatalf("%q parsed as %#v, want one local_alias mention", input, mentions)
	}
}
```

- [x] **Step 4: Run focused Go tests and confirm they fail where expected**

Run:

```bash
go test ./internal/hostops ./internal/appui -run 'TestParseHostMentions|TestChatService.*Host.*Metadata|TestChatService.*Forged|TestChatService.*ServerLocal'
```

Expected: FAIL until local alias normalization, metadata-first binding, and forged metadata rejection are complete.

- [x] **Step 5: Implement metadata-first semantics without trusting the client**

Keep the order in `hostOpsMentionsForCommand`:

1. parse `cmd.Metadata["aiops.hostops.mentions"]`;
2. treat parsed metadata as untrusted lookup hints;
3. resolve those mentions through `HostRepository`;
4. keep only server-confirmed mentions for route counting and host binding;
5. only if no metadata mention exists, parse raw content.

Rules:

- metadata with `source=inventory` and `hostId` can be used as a lookup key, but its `Resolved` value must be ignored until resolver confirms a matching host;
- if `HostRepository` is unavailable, drop inventory metadata and allow only explicit local aliases or IP literal fallback already supported by `hostops.ParseHostMentions`;
- if resolver reports no matching host, remove that mention before `filterHostOpsRouteMentions`;
- do not let `filterHostOpsRouteMentions` pass an inventory mention only because client supplied a non-empty `HostID`.

- [x] **Step 6: Preserve raw text fallback without adding a second runtime path**

Raw content parsing remains fallback only:

1. use metadata when metadata contains at least one candidate;
2. if metadata exists but fails server validation, fail closed as no host binding;
3. only if metadata is absent, parse raw content.

Make sure a forged metadata item with `hostId` and `resolved=true` is not rescued by later raw parsing in the same request; the user should reselect a valid host if metadata validation fails.

- [x] **Step 7: Normalize local aliases in backend parser**

Update `isLocalAliasToken` so the explicit `@` forms `local/server-local/localhost/127.0.0.1/::1/[::1]` all produce `HostMentionSourceLocalAlias`.

- [x] **Step 8: Run focused Go tests**

Run:

```bash
go test ./internal/hostops ./internal/appui -run 'TestParseHostMentions|TestChatService.*Host.*Metadata|TestChatService.*Forged|TestChatService.*ServerLocal'
```

Expected: PASS.

---

### Task 4: Fix Assistant Transport Fallback for @server-local

**Files:**
- Modify: `internal/appui/transport_commands.go`
- Test: `internal/server/assistant_transport_api_test.go`

- [x] **Step 1: Add failing assistant transport test**

Add a test mirroring the existing `@local` test, but with `@server-local 查看 CPU 情况`:

```go
payload := assistantTransportAddMessagePayload(t, "", "thread-v2-server-local-mention", "@server-local 查看 CPU 情况")
```

Expected assertions:

- `runReq.HostID == "server-local"`
- `runReq.SessionType == runtimekernel.SessionTypeHost`
- route mode is `host_bound_ops`
- target binding is `host`
- exec command allowed is `true`

- [x] **Step 2: Run focused transport test and confirm it fails if fallback still drops @server-local**

Run:

```bash
go test ./internal/server -run 'TestAssistantTransport.*ServerLocal|TestAssistantTransportAPILocalMentionBindsServerLocal'
```

Expected: FAIL for `@server-local` before transport fallback is fixed.

- [x] **Step 3: Implement fallback by reusing hostops parser normalization**

Do not add a transport-specific host parser. Ensure `buildChatRuntimeTransportRoute` consumes `hostops.ParseHostMentions(messageText)` after parser local alias normalization, so `@server-local` survives `filterHostOpsRouteMentions`.

- [x] **Step 4: Run focused transport tests**

Run:

```bash
go test ./internal/server -run 'TestAssistantTransport.*ServerLocal|TestAssistantTransportAPILocalMentionBindsServerLocal'
```

Expected: PASS.

---

### Task 5: Verify Runtime Tool Surface Exposes exec_command for Bound Host CPU Checks

**Files:**
- Test: `internal/runtimekernel/intent_tool_packs_test.go`
- Test: `internal/tooling/turn_metadata_filter_test.go`

- [x] **Step 1: Add or extend runtime tests for bound host CPU inspection**

Ensure a turn with:

```go
TurnRequest{
	SessionType: runtimekernel.SessionTypeHost,
	Mode: runtimekernel.ModeChat,
	HostID: "server-local",
	Input: "查看 CPU 情况",
	Metadata: map[string]string{
		"aiops.target.binding": "host",
		"aiops.target.hostId": "server-local",
		"aiops.tool.execCommandAllowed": "true",
		"aiops.route.mode": "host_bound_ops",
	},
}
```

has `exec_command` in assembled/model-visible tools.

- [x] **Step 2: Confirm no-host advisory still hides exec_command**

Keep or add a negative test with:

```go
Metadata: map[string]string{
	"aiops.target.binding": "none",
	"aiops.tool.execCommandAllowed": "false",
	"aiops.route.mode": "chat_advisory",
}
```

Expected: `exec_command` is not visible.

- [x] **Step 3: Run runtime/tooling focused tests**

Run:

```bash
go test ./internal/runtimekernel ./internal/tooling -run 'TestRunTurn_EnablesExecForSelectedHostResourceInspection|Test.*NoHost|Test.*HostBound'
```

Expected: PASS.

---

### Task 6: Browser Verification for Real User Flow

**Files:**
- Test manually through browser-in-app or Playwright against `http://127.0.0.1:5173/`

- [x] **Step 1: Start the dev stack**

Use the repo's existing dev command. If a server is already running on `5173`, reuse it.

- [x] **Step 2: Verify selecting a host from @ list**

In the chat composer:

1. type `@ser`;
2. choose the `server-local` host from the suggestion list;
3. continue with `查看 CPU 情况`;
4. send.

Expected UI/runtime behavior:

- message visible text may show `@server-local 查看 CPU 情况` or `@local 查看 CPU 情况`;
- outgoing transport message contains `hostId=server-local`;
- process/runtime state does not show `host_scope:none`;
- model/tool layer can use `exec_command`;
- final answer includes actual CPU/read-only command evidence or a clear execution result.

- [x] **Step 3: Verify direct @local still works**

Send:

```text
@local 查看 CPU 情况
```

Expected: same host-bound behavior as selected `server-local`.

- [x] **Step 4: Verify bare server-local still does not bind**

Send:

```text
server-local 查看 CPU 情况
```

Expected: advisory/no host binding unless the UI explicitly selected a target; this protects against accidental host execution.

- [x] **Step 5: Verify remote host from inventory**

Select a remote host from the `@` list, for example `120.77.239.90` / `host-a`, then ask for CPU.

Expected:

- hidden metadata carries that host's `hostId`;
- route is host-bound to that host;
- `exec_command` runs against the selected host channel, not `server-local`.

Status note: Playwright verified the selected remote host outbound payload carries `hostId=remote-120-77-239-90`, `address=120.77.239.90`, and `source=inventory`. Live remote trace `sess-1783309800183552000/turn-1783309801351679000` bound `aiops.target.hostId=remote-120-77-239-90`, `aiops.route.mode=host_bound_ops`, `aiops.tool.execCommandAllowed=true`, and model-visible `exec_command`. Tool evidence shows `nproc`, `cat /proc/cpuinfo`, and `cat /proc/loadavg` executed with `hostId=remote-120-77-239-90`, `source=host.agent_http_exec`, and `exitCode=0`, so execution used the selected remote host channel instead of `server-local`. The second model pass was cancelled when the Playwright page closed after tool evidence was captured; the host binding and command execution contract was still verified.

---

### Task 7: Full Regression Commands

**Files:**
- Modify: `web/vitest.config.js` only if new Playwright spec files are added.

- [x] **Step 1: Run frontend regression**

Run:

```bash
npm --prefix web run test -- src/chat/hostMentions.test.ts src/chat/hostMentionSearch.test.ts src/chat/components/AiopsComposer.test.tsx
```

Expected: PASS.

- [x] **Step 2: Run frontend build and typecheck**

Run:

```bash
npm --prefix web run typecheck
npm --prefix web run build
```

Expected: PASS. The Vite chunk-size warning is acceptable; TypeScript errors are not.

- [x] **Step 3: Keep Vitest and Playwright separated**

If this implementation adds or renames any `web/tests/*.spec.js` Playwright file, add that file to `web/vitest.config.js` `test.exclude`, then run:

```bash
npm --prefix web test
```

Expected: Vitest does not import Playwright specs. Existing unrelated test failures must be documented before merge.

Status note: `web/vitest.config.js` now excludes the existing Playwright specs `tests/assistant-message-single-path.spec.js`, `tests/chat-runtime-folding-snapshot.spec.js`, and `tests/llm-provider-config-snapshot.spec.js`. After that change `npm --prefix web test` no longer imports Playwright specs, but still exits non-zero on 8 existing unrelated frontend assertions: `tests/hostListViewModel.spec.js` (3), `src/chat/ChatPage.runtimeContractV3.test.tsx` (1), `src/lib/zhLabels.test.ts` (1), `src/pages/complexPages.test.tsx` (1), `src/chat/components/PostRunSuggestions.test.tsx` (1), and `src/pages/opsgraph/OpsGraphListPage.test.tsx` (1).

- [x] **Step 4: Run backend routing regression**

Run:

```bash
go test ./internal/hostops ./internal/appui ./internal/server -run 'HostMention|ServerLocal|LocalMention|HostBound|AssistantTransport'
```

Expected: PASS.

- [x] **Step 5: Run runtime/tool surface regression**

Run:

```bash
go test ./internal/runtimekernel ./internal/tooling -run 'Exec|HostBound|NoHost|ResourceInspection|ToolVisibility'
```

Expected: PASS.

- [x] **Step 6: Run static guard if agent runtime layering work is present**

Run:

```bash
./scripts/verify-agent-runtime-single-path.sh
```

Expected: PASS, or only failures unrelated to this host mention task are reported and documented before merge.

- [x] **Step 7: Run full Go regression**

Run:

```bash
go test ./...
```

Expected: PASS.

---

## Acceptance Criteria

- [x] 通过 `@` 主机列表选择 `server-local` 后，消息 metadata 中稳定携带 `hostId=server-local`。
- [x] 服务端重新确认 `aiops.hostops.mentions`，伪造的 `hostId` 或伪造的 `resolved=true` 不会打开 host-bound 执行。
- [x] `@server-local 查看 CPU 情况` 进入 `host_bound_ops`，prompt runtime state 为 `host_scope: bound`。
- [x] `exec_command` 对 host-bound CPU 只读检查可见且可调度。
- [x] 裸文本 `server-local 查看 CPU 情况` 不自动绑定主机。
- [x] 删除或编辑已选中的 `@host` token 后，旧隐藏 metadata 不会继续绑定主机。
- [x] 多主机 mention 不会任选一个主机直接执行。
- [x] 前端、transport、appui、runtime tool surface 都通过对应测试。
- [x] browser-in-app/Playwright 真实用户流程验证通过。
