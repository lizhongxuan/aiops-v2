# aiops-v2 Agent Harness 可控性实施验证报告

> 设计基线：`2026-07-10-aiops-v2-agent-harness-controllability-design.zh.md`  
> 实施清单：`2026-07-10-aiops-v2-agent-harness-controllability-implementation-todo.zh.md`  
> 验证日期：2026-07-14（CST）  
> 分支：`feat/agent-harness-controllability`  
> Runtime 代码验证基线：`b846322`（后续文档提交不改变运行时行为）

## 1. 最终结论

Task -1 到 Task 20 已实施并验证。最终版本把 Agent 的控制权收敛到 RuntimeKernel：生产 Turn 只有一个生命周期 writer、一个 provider/tool loop、一个事实来源和一个 canonical transport 协议；AppUI、Server、React 与 Eval 均只消费 Runtime 已提交的结果。

本轮最终通过：

- 全量 Go、Web、边界自检、hardening gate、race 定向测试与 Web production build。
- 14 个设计 P0 story 和扩展 golden story。
- canonical rollout 的 contract/provider-fixture/full-story replay。
- Playwright 13/13，以及 browser-in-app 的真实点击、断言、console 检查和截图。
- 当前 `b846322` 构建的本地与远端 GLM-5.1 no-tool 真实 provider 样本，均为 51/51、tool calls 0。

真实 provider 验证过程中发现的 UI 展示回归、legacy eval fixture 和 `FinalContract.status=unknown` 终态误判均已先复现、再修复并加入回归测试；不是以规避样本或放宽断言的方式得到通过。

## 2. 当前真实控制链

```text
AppUI TransportCommand
  -> admission adapter（仅在边界把 legacy metadata 转为 typed AdmissionFacts）
  -> RuntimeKernel.RunTurn（唯一生产 lifecycle / provider-tool loop owner）
  -> immutable TurnAssembly + assembly hash
  -> RuntimeStepContext / StepRevision + step hash
  -> PromptEnvelopeV2（L0 -> L6）
  -> frozen ProviderRequest + PromptFingerprint
  -> provider response
  -> StepToolRouter（provider 可见工具面与 dispatcher 可执行工具面的同一事实源）
  -> ToolDispatcher / ActionToken / approval / evidence / post-check
  -> FinalRuntimeFacts
  -> FinalContract
  -> Runtime transport projection（canonical blockOrder + blocksById）
  -> Server data-aiops_transport_state
  -> React AssistantTransport
  -> canonical transcript block presenter
  -> UI

确定性 SystemTurn 分支：
RuntimeKernel.CommitSystemTurn
  -> canonical user/assistant/checkpoint/final facts/transport/owner trace
  -> 与普通 Turn 相同的 rollout 与 transport 投影出口

每个关键阶段同时 append 到 CanonicalRollout：
admission -> assembly -> prompt -> provider -> tool -> approval -> checkpoint
-> final facts -> transport projection source
```

### 2.1 控制边界

| 边界 | 最终约束 |
|---|---|
| 生命周期 ownership | `RuntimeKernel` 是唯一 writer；AppUI 不再拥有迁移通知、repair plan、workflow generation 的第二套 Turn loop |
| SystemTurn | 只能提交 Runtime 允许的确定性状态，不能伪造 verified/completed 等业务完成事实；其自身 Turn lifecycle 可由 Runtime 正常收口 |
| Prompt | 固定内容在前，动态内容在后：L0-L3 稳定层、L4 history、L5 RAG/MCP/skill/tool result、L6 当前输入/continuation 且最后 |
| Prompt shadow | 只读 display/diagnostic trace，不参与控制，不进入生产 Prompt hash，也不能改变 provider input |
| Tool surface | Provider 和 dispatcher 绑定同一 `StepToolRouter` 指纹；mutation tool 缺 rollback declaration 时在 provider 前拒绝 |
| Final | final text/markdown 只展示；控制状态来自 typed `FinalRuntimeFacts` / `FinalContract` |
| Transport wire | 只传 `blockOrder` 与 `blocksById`；Go `Process`/`Final` 为 `json:"-"`，React 不读取 `metadata.unstable_state` |
| Eval | 只接受 canonical blocks；旧 `process`/`final` wire 直接失败，不再静默兼容 |
| Legacy transcript | 仅在 persisted legacy input converter 边界读取，转换后立即进入 canonical block 模型 |

`FinalContract.status=unknown` 不是无条件成功：只有 Turn/transport 已完成、无 pending approval/evidence/running tool、liveness 正常且没有失败事实时，才投影为 completed block；否则继续 fail closed。

## 3. Prompt 组装与缓存可解释性

生产 Prompt 顺序已经固定：

```text
L0 absolute.system
L1 runtime.contract
L2 role.profile
L3 turn.stable_facts
L4 history/reasoning/checkpoint
L5 dynamic_context（RAG、MCP、skill、tool result）
L6 current_user_input / continuation_instruction
```

Provider adapter 验证 L0/L1 first、L2/L3 在 history 前、L5 在 history 后、L6 last，并保持 tool call/result 配对。cache trace 会显示具体 miss 原因，例如：

- `hash_changed`
- `section_added`
- `section_removed`
- `order_changed`
- `cache_scope_changed`

Prompt shadow parity 仍可供排障展示，但已经从生产 hash、控制判断和 provider request 中彻底隔离。

## 4. 永久防回退门禁

门禁入口：

- `scripts/test-aiops-harness-contract-boundaries.sh`
- `scripts/check-aiops-harness-contract-boundaries.sh`
- `scripts/check-aiops-final-text-control.py`
- `scripts/aichat-harness-hardening-gate.sh`
- `.github/workflows/aiops-v2-hardening.yml`

自证 fixture 覆盖：

- eval 重新访问 `/api/v1/state` 或重新接受 legacy `process`/`final` wire。
- final text/markdown 经 direct、nested transform、template interpolation、closure capture 或多 hop alias 推导控制状态。
- provider adapter、Runtime Step 和 dispatcher 使用不同 tool surface。
- TurnAssembly-before-prompt marker 缺失。
- L0/L1 first 或 L5/L6 last validator 缺失。
- AppUI 重新引入 lifecycle writer 或第二个 provider/tool loop。
- mutation tool 没有 rollback declaration 仍被暴露给 provider。

生产 Turn 还会比较：

```text
RuntimeStepContext.ProviderRequest.Tools[*].Hash
  == DispatchResult.DecisionTrace.ToolSurfaceFingerprint
```

deliberate divergence 会被拒绝。

## 5. 最终代码验证

| 验证 | 最终结果 |
|---|---|
| `go test ./internal/runtimecontract ./internal/agentassembly ./internal/promptcompiler ./internal/promptinput ./internal/modelrouter ./internal/modeltrace -count=1` | PASS |
| `go test ./internal/runtimekernel -count=1` | PASS |
| `go test ./internal/appui ./internal/server ./internal/eval -count=1` | PASS |
| `go test ./...` | PASS |
| `go test -race ./internal/appui ./internal/eval ./internal/server -run 'UnknownFinal\|AssistantTransportStories\|RuntimeLifecycleHasUniqueWriter\|RolloutReplayReferenceFixtures' -count=1` | PASS；appui 2.436s、eval 6.152s、server 14.308s |
| `npm --prefix web test -- --run` | PASS；124 test files、907 tests、15.63s |
| `npm --prefix web run typecheck` | PASS |
| `npm --prefix web run build` | PASS；2831 modules、2.03s；只有既有的 >500 kB chunk warning |
| `bash scripts/test-aiops-harness-contract-boundaries.sh` | PASS |
| `bash scripts/check-aiops-harness-contract-boundaries.sh` | PASS |
| `bash scripts/aichat-harness-hardening-gate.sh` | PASS；含全仓 Go 与 Web focused 118+19 |
| `git diff --check` | PASS |

Web 测试中的 jsdom `HTMLCanvasElement.getContext` 提示是既有非失败 warning；没有新增 test failure。

### 5.1 P0 story

| Story | Deterministic full-story |
|---|---|
| `basic_no_tool` | PASS |
| `single_readonly_tool` | PASS |
| `approval_resume` | PASS |
| `approval_denied` | PASS |
| `mutation_missing_target` | PASS，provider/tool 0 次 |
| `mutation_missing_rollback` | PASS，provider/tool 0 次，并返回 canonical system failed block |
| `partial_mutation_postcheck_failed` | PASS |
| `tool_not_found` | PASS |
| `invalid_arguments` | PASS |
| `cancelled_running_tool` | PASS |
| `context_compaction_resume` | PASS |
| `same_session_host_carryover` | PASS |
| `multi_host_manager` | PASS |
| `evidence_rca_no_exec` | PASS |

扩展 golden story：`host_bound_readonly`、`raw_dsml_markup_sanitized`、`host_agent_unavailable_fallback_denied` 同样通过。

### 5.2 Replay 固定证据

Replay reference 只能通过显式 `AIOPS_UPDATE_ROLLOUT_REPLAY=1` 更新；正常测试不允许偷偷刷新 golden。

| Fixture | Source | Rollout | Transport |
|---|---|---|---|
| `approval_resume` | `sha256:1e295d4f22eccaf3388b4f0c04aab71f4707315f58580584a50158592541ce99` | `sha256:7e0b58c2d150317b29ff5ec626451d244fbce7b98975bfe7251a30fec2a01b49` | `sha256:d5ab006c672d21c77d4255ff4bbbb163b806c28883deb73981555043f075213a` |
| `tool_not_found` | `sha256:3d673cacdb09b353f4e54a901d65e149ee0fc0a10a982b1354513ce53c7b1a86` | `sha256:5a5531c4ea96295e4d406e6aaecd47ea78d39b4b7d2ff4b8177dfdf3c6242748` | `sha256:e7c5f570d235a2f3c11e3248c9a7a5247070d27001f8baeee26805e8cb0a540b` |

三类 replay 均存在：contract replay、provider fixture replay、full-story replay。

## 6. Playwright 与 browser-in-app

### 6.1 Playwright

最终命令：

```bash
npx playwright test \
  tests/react-shell-snapshot.spec.js \
  tests/agentHarnessPromptTrace.snapshot.spec.js \
  --project=chromium
```

结果：13/13 PASS，13.7s。

### 6.2 应用内浏览器真实操作

browser-in-app 打开 `/debug/prompts`，点击 `iteration-001.json`，再打开“控制链”。最终验证：

- 控制链 panel 唯一出现。
- owner 为 `approval`。
- 显示 9 个 hash row，包含 L0、L5、L6。
- cache miss 明确显示 `absolute.system`、`hash_changed`、`section_added`。
- permission、checkpoint、visible-only binding、rollout event/hash 均可见。
- 页面没有显示 credential 或其他敏感信息。
- browser console error 0、warning 0。

截图：`output/playwright/agent-harness-prompt-trace/browser-in-app-control-chain.png`。

## 7. 当前代码的真实 GLM-5.1 验证

Provider 使用用户授权的 Z.AI OpenAI-compatible endpoint，模型为 `glm-5.1`。报告、代码、fixture 与提交中没有保存 API key、SSH 密码或等价敏感信息。

### 7.1 本地最终样本

- Runtime 二进制：`output/task20/ai-server-final`
- SHA-256：`8fd8063bb22664b8cc0a0937ccee6c0411d3ad4dc9144a7ab64543635eea66e3`
- Eval run：`task20-glm51-final-head-smoke-b846322-pass`
- 结果：PASS 1/1，score 1.00，51/51，tool calls 0。
- Turn：`turn-1784002595000293000`，lifecycle completed，1 iteration，4 items。
- canonical head seq 7：
  - event `event:59c6f31c021e7f67188b739cecab092cf156cffd761884fb9605e7e4b303f29f`
  - hash `sha256:897fd8a7930128af8d08b1735ab2a26f6fdec03adfa5bc6aae05ef62fedb3ee6`

### 7.2 远端隔离服务最终样本

- Linux amd64 二进制：`output/task20/ai-server-linux-amd64-final`
- SHA-256：`4c21f0ef5963290cde1ae84cd680052af4617d7454802dc0c703db9a63311c28`
- Eval run：`task20-glm51-remote-final-head-b846322`
- 结果：PASS 1/1，score 1.00，51/51，tool calls 0。
- Turn：`turn-1784003476338768359`。
- canonical head seq 7：
  - event `event:aa0e899bb8b0704842045b4834f017a1a0559095b11fc45f4419577994a2c02e`
  - hash `sha256:75f9aadca8912bfa9597fe6afe7a43d91d5bfa796d7357d4ca10ac91c003ae59`

远端使用临时 29180/29190 服务和 SSH tunnel。验证结束后进程、tunnel 与临时目录均已清理，29180/29190 已确认关闭；既有远端服务未修改。

### 7.3 真实 provider 暴露并修复的问题

最终通过前，真实样本发现 `FinalContract.status=unknown` 在“无工具、回答完成”的合法情况下仍被 AppUI 映射为 running，Eval 也不把它视作终态。修复流程：

1. 增加 RED 回归：`TestCanonicalTransportUnknownFinalContractIsCompletedBlock`。
2. 增加 Server RED 回归：`TestServerAgentAssistantTransportAcceptsCompletedUnknownFinalContract`。
3. 只在完整终态条件满足时把 unknown 映射为 completed；其他 unknown 继续 fail closed。
4. 使用当前 `b846322` 重新构建本地与远端二进制，两个真实样本均达到 51/51。

另一个 built-in semantic no-tool 诊断样本为 50/51，仅因期望 literal “直接回答”未原样出现，Runtime 无错误；最终 release 证据使用独立 exact-output smoke，避免把文字风格差异误判成 harness 故障。

## 8. 修复前历史诊断样本（不作为当前发布通过证据）

修复前还跑过真实 read-only proposal、approval、RCA 与 multi-host advisory。这些样本只用于发现边界问题：

- terminal `pwd` 仍需要服务端 ActionToken/evidence binding，普通 UI approval 不会绕过 policy，executor 保持 0。
- 长 RCA/多主机输出曾触发 Eval canonical item 大小上限；Runtime 已完成但 Eval fail closed。
- multi-host 样本只验证 manager advisory，没有连接多个真实 Host-Agent worker。

以上结果没有被计为当前版本的真实工具执行通过；真正的 mutation、approval resume 与 multi-host 执行仍以 deterministic full-story 作为本轮完成证据。

## 9. 本轮校正提交

| 提交 | 校正内容 |
|---|---|
| `2c4ecbd` | 保留 canonical transcript 的正确 UI presentation；修复 pending/running 标头、连续 Ops Manual artifact 合并与普通工具输出误判 transport error |
| `0a24eee` | 把 browser 发现的 UI 回归记录到 `fixbug.md` |
| `48f6fac` | 把 CLI Eval legacy `process/final` fixture 迁移为 canonical blocks |
| `b846322` | 收敛 unknown final contract 的终态判定，并加入 AppUI/Server 回归 |

`README.md` 中用户已有的当前控制链记录未被本分支覆盖或改写。

## 10. 已知限制与下一阶段

1. terminal read-only command 的 approval/evidence 产品语义仍应单独设计；不能由 UI 绕过 Runtime policy。
2. 超长 typed artifact 需要 ref hydration，而不是放宽 canonical transport 上限或静默截断。
3. 真正的多 Host-Agent worker E2E 需要至少两个可控 worker target；本轮 deterministic manager story 已完成，真实 worker 网络未纳入发布证据。
4. Host-Agent 的 Skill/MCP capability policy 仍需要成为显式、可审计的能力来源。
5. persisted legacy transcript converter 暂时保留在输入边界；内部与输出协议已经 canonical-only。
6. Python final-text checker 是有限静态分析，不是完整语言证明；它与 runtime parity、typed projector 测试和坏 fixture 自检共同构成多层门禁。

## 11. 发布判断

Agent Harness 的确定性 cutover、canonical transport、Prompt 顺序、mutation rollback、replay 与 CI 防回退条件已满足，可进入代码评审与合并选择。

当前可发布范围：no-tool、canonical transcript、Prompt Trace、replay、确定性审批/变更 full-story。真实 terminal mutation、多 Host-Agent worker 和超长 artifact hydration 应继续保持 fail closed，并作为后续独立任务推进。
