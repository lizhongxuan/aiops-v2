# aiops-v2 Agent Harness 可控性实施验证报告

> 设计基线：`2026-07-10-aiops-v2-agent-harness-controllability-design.zh.md`  
> 实施清单：`2026-07-10-aiops-v2-agent-harness-controllability-implementation-todo.zh.md`  
> 验证日期：2026-07-14（CST）  
> 分支：`feat/agent-harness-controllability`  
> 代码验证基线：`c3c0159`（报告提交不改变运行时行为）

## 1. 结论

本轮已完成 Task -1 到 Task 20 的实现和验证。确定性控制链、三类 replay、Prompt L0-L6 顺序、provider/dispatcher 同一 `StepToolRouter`、facts-only Final/Transport 投影、append-only canonical rollout、Prompt Trace 页面和永久 CI 门禁均已落地。

确定性发布门禁全部通过；真实 GLM-5.1 的 no-tool 本地和远端样本通过。真实工具/审批/RCA/多主机抽样暴露了三个受限点，并按 fail-closed 处理，没有伪造为通过：

1. 终端 `pwd` 在当前策略中仍按需要审批/证据的执行动作处理，普通 UI approval 不能替代服务端 ActionToken/evidence binding，executor 保持 0。
2. RCA 和多主机长输出在 eval 的 canonical agent-items 严格边界触发截断错误；Runtime 已完成，但 eval 判定不通过。
3. 真实 multi-host 样本只验证了 manager advisory 规划，没有连接真实多个 Host-Agent worker；真实 manager worker 生命周期由确定性 P0 full-story 覆盖。

## 2. 最终真实控制链

```text
AppUI TransportCommand
  -> admission adapter（仅一次 legacy metadata -> typed AdmissionFacts）
  -> IntentFrame + resource/role/session bindings + policy
  -> immutable TurnAssembly + hash
  -> RuntimeStepContext / StepRevision + hash
  -> PromptEnvelopeV2（L0 -> L6）
  -> frozen ProviderRequest + PromptFingerprint
  -> StepToolRouter（provider 可见面与 dispatcher 可执行面的同一事实源）
  -> ToolDispatcher / ActionToken / approval / evidence / post-check
  -> FinalRuntimeFacts -> FinalContract
  -> AiopsTransportState（facts-only projection）
  -> UI

每个关键变化同时 append 到 CanonicalRollout：
admission -> assembly -> prompt -> provider -> tool -> approval -> checkpoint
-> final facts -> transport projection source
```

控制权边界：

- System Prompt、绝对角色和稳定 runtime contract 位于 L0-L2；Turn 稳定事实位于 L3。
- history 位于 L4，RAG/MCP/skill/tool result 等动态内容位于 L5。
- 当前用户输入或 continuation instruction 位于 L6 且是最后逻辑输入。
- final text/markdown 只用于展示，不能推导 approval、verified、completed、failed 等控制状态。
- `/api/v1/state` 可继续服务非 agent 页面，但 `internal/eval` 和 agent harness 禁止消费。

## 3. Cutover 与兼容面审计

| 检查项 | 结果 | 证据/结论 |
|---|---|---|
| legacy provider input builder | 已删除 | Task 12 已将生产 ProviderRequest 切到 validated EnvelopeV2；旧 builder 分支不再存在 |
| shadow 双写控制分支 | 已退出控制链 | Prompt shadow parity 仅作 hash/trace 对照，ProviderRequest.Input 是冻结权威输入 |
| legacy assembly trace | 只读兼容 | `LegacyAgentAssemblySnapshot` 已标记 `Deprecated`，不得作为 runtime control input |
| route/target/role/profile metadata 散读 | 已迁移 | admission 边界一次性适配，runtime 使用 `AdmissionFacts` / `TurnAssembly` |
| `/api/v1/state` | 非 agent 兼容 | boundary gate 禁止 `internal/eval` 和 `cmd/agent-eval` 访问 |
| final text 控制推导 | 已禁止 | Python stdlib 有限污点分析支持嵌套 transform、16-hop alias、Go/TS、if/ternary |

## 4. 永久门禁与绕过自证

门禁入口：

- `scripts/test-aiops-harness-contract-boundaries.sh`
- `scripts/check-aiops-harness-contract-boundaries.sh`
- `scripts/check-aiops-final-text-control.py`
- `scripts/aichat-harness-hardening-gate.sh`
- `.github/workflows/aiops-v2-hardening.yml`

坏 fixture 覆盖：

- eval 重新访问 `/api/v1/state`。
- final text/markdown 直接、单层、嵌套、未知 transform 和 2-hop alias 推导控制状态。
- Go `strings.ToLower(strings.TrimSpace(finalText))`、TS `markdown.trim().toLocaleLowerCase()`。
- provider adapter 错接另一 router、Runtime Step 调用点传入另一 tool surface、dispatcher 仅用注释伪造 binding。
- TurnAssembly-before-prompt marker 缺失。
- L0/L1 first 或 L5/L6 last validator 缺失。
- 合法注释、test/dist 文件和纯 display/sanitize 使用不会误报。

独立审查曾三次成功绕过旧规则；每次先留下 RED fixture，再修到 GREEN。最终规则不再枚举 trim/lower 函数，而是在函数作用域内追踪 final display text 到条件表达式的污染传播。

真实生产 Turn 测试还会比较：

```text
RuntimeStepContext.ProviderRequest.Tools[*].Hash
  == DispatchResult.DecisionTrace.ToolSurfaceFingerprint
```

测试中的 deliberate divergence 会被拒绝。

CI backend job 在门禁前执行 `setup-node` 和 `npm ci`；boundary self-test 是第一个验证 step。新 Python checker 也在 workflow path trigger 中。

## 5. 完整 deterministic 验证

| 日期 | 命令 | Exit | 结果/重跑说明 |
|---|---|---:|---|
| 2026-07-14 | `go test ./internal/runtimecontract ./internal/agentassembly ./internal/promptcompiler ./internal/promptinput ./internal/modelrouter ./internal/modeltrace -count=1` | 0 | 通过，约 1.95s；为取得稳定耗时只读复跑一次，同样通过 |
| 2026-07-14 | `go test ./internal/runtimekernel -count=1` | 0 | 通过，约 11.42s |
| 2026-07-14 | `go test ./internal/appui ./internal/server ./internal/eval -count=1` | 0 | 通过，约 5.55s |
| 2026-07-14 | `npm --prefix web test -- --run` | 1 -> 0 | 首次因 Vitest 误收集 Playwright spec 退出 1；`f7f1185` 隔离 runner 后 123 files / 899 tests 全通过 |
| 2026-07-14 | `npm --prefix web run build` | 0 | 2831 modules；仅有既有大 chunk warning |
| 2026-07-14 | `bash scripts/test-aiops-harness-contract-boundaries.sh` | 0 | 坏 fixture 自检通过 |
| 2026-07-14 | `bash scripts/check-aiops-harness-contract-boundaries.sh` | 0 | 真实仓库扫描通过 |
| 2026-07-14 | `bash scripts/aichat-harness-hardening-gate.sh` | 0 | 最新 `c3c0159` 后由主 agent 再跑；Go、Web 117+18、全仓测试通过 |
| 2026-07-14 | `go test ./...` | 0 | 全仓通过，约 7.31s；hardening 中再次通过 |
| 2026-07-14 | `git diff --check` | 0 | 通过 |
| 2026-07-14 | `npx playwright test tests/agentHarnessPromptTrace.snapshot.spec.js --project=chromium` | 0 | 1 passed，3.5s |

### 5.1 设计中的 14 个 P0 story

| Story | Deterministic full-story |
|---|---|
| `basic_no_tool` | PASS |
| `single_readonly_tool` | PASS |
| `approval_resume` | PASS |
| `approval_denied` | PASS |
| `mutation_missing_target` | PASS，provider/tool 0 次 |
| `mutation_missing_rollback` | PASS，provider/tool 0 次 |
| `partial_mutation_postcheck_failed` | PASS |
| `tool_not_found` | PASS |
| `invalid_arguments` | PASS |
| `cancelled_running_tool` | PASS |
| `context_compaction_resume` | PASS |
| `same_session_host_carryover` | PASS |
| `multi_host_manager` | PASS |
| `evidence_rca_no_exec` | PASS |

当前 golden corpus 实际为 17 个；除上表外还包括 `host_bound_readonly`、`raw_dsml_markup_sanitized`、`host_agent_unavailable_fallback_denied`，均在 `TestAIChatHarnessGoldenCases` 中通过。

### 5.2 三种 Replay

`approval_resume` 和 `tool_not_found` 均通过：

- Contract replay：重建 TurnAssembly/Step/Final hash，不调用 provider/tool。
- Provider Fixture replay：使用冻结 provider response 重跑 adapter、policy 和 final facts。
- Full-story replay：复用真实 `TransportCommandHandler -> RuntimeKernel -> TransportProjector`。

固定 hash：

| Fixture | Source | Rollout | Transport |
|---|---|---|---|
| `approval_resume` | `sha256:7d0d2733d69d72aa438126ae3bacfef0778fcc59eac8ef1bc27ab69f92fd7c07` | `sha256:ba034d8f7a3a56de31492980c2351d0b0ce79a78870a85743df50aeb271623b1` | `sha256:2ef952f306ffcb7f19a86ff8a86b23c9ad5c702ccb357f811bf3130f4e0bcebe` |
| `tool_not_found` | `sha256:3d673cacdb09b353f4e54a901d65e149ee0fc0a10a982b1354513ce53c7b1a86` | `sha256:683d26218cf720b16ff6515788b71a878d05da1f1a79c1bcbfb215d71df14b6a` | `sha256:1ddc26cb779b5948d3e776adddeebd5a47c74309e3305517680f41cf49c6a8a9` |

### 5.3 Prompt hash 稳定性

| 变化 | 已验证变化 | 已验证保持不变 |
|---|---|---|
| 用户输入 | `current_user_input_hash`、`model_input_hash` | stable/turn prefix、history、dynamic |
| RAG | `dynamic_context_hash`、`model_input_hash` | stable/turn prefix、current user |
| role | `role_profile_hash`、`stable_prefix_hash`、model | absolute system |
| host/Turn facts | turn stable/prefix、model | stable prefix |
| history/reasoning | history、model | stable/turn prefix、dynamic、current user |
| approval/continuation | checkpoint/history/continuation/model | TurnAssembly、L0-L3 |

Provider adapter 验证 L0/L1 first、L2/L3 before history、L5 after history、L6 last，并保持 tool call/result 配对；provider-specific cache 标记不改变逻辑顺序。

## 6. Playwright 与应用内浏览器

### 6.1 Prompt Trace

- Playwright snapshot：`output/playwright/agent-harness-prompt-trace/`
- 截图：`output/playwright/agent-harness-prompt-trace/.playwright-cli/page-2026-07-13T22-38-01-379Z.png`
- fixture trace：`output/playwright/agent-harness-prompt-trace/prompt-traces/iteration-001.json`
- 页面显示 TurnAssembly/Step、9 个 Prompt hash、router diff、approval/checkpoint/rollout refs 和 first divergence owner。
- Playwright 和 browser-in-app 均通过，应用 console error 为 0。

### 6.2 真实审批 UI 与远端 UI

- 本地审批截图：`output/task20/playwright/.playwright-cli/page-2026-07-13T23-33-49-667Z.png`
- 远端 UI 截图：`output/task20/playwright/.playwright-cli/page-2026-07-13T23-41-27-873Z.png`
- 应用内浏览器实际点击 Submit；UI 进入 runtime reapproval/pending evidence，未绕过 ActionToken/evidence 执行。
- 远端隔离 tunnel 页面显示 `glm-5.1`，Playwright/browser-in-app console error 均为 0。

## 7. 真实 GLM-5.1 抽样

Provider：Zhipu OpenAI-compatible endpoint（`api.z.ai`）；Model：`glm-5.1`。报告和仓库未保存 API key 或主机密码。

| Story | Turn / rollout | Runtime/评测结果 | 人工结论 |
|---|---|---|---|
| no-tool（本地） | `turn-1783984870930765000`; seq 10 `sha256:6842bb83b82422eced32ffd87300408b39346a7fcf7447d7059e8dfccd291120` | PASS 1/1，score 1.00，51/51；completed，2 iterations | 真实 provider 基础链通过 |
| read-only/tool proposal | `turn-1783985138184650000`; 最终 seq 23 `sha256:3e965853d1dddf01aef4d6e22dfeb7b56ab2b1e4e04c662edf86e01b2af7b4ba` | 模型提出一次 `exec_command pwd`；executor 0；最终 `pending_evidence` | targetless 请求先 fail closed；绑定目标后仍不能把 terminal command 当作无条件只读执行 |
| approval resume | 同上 | 应用内浏览器提交 approval 后重发 runtime approval，随后保持 pending evidence；executor 0 | UI 决策不能伪造服务端 ActionToken/evidence，安全但该样本未完成执行 |
| RCA evidence | `turn-1783985237221184000`; seq 19 `sha256:c41a980a04cb8453e50016a302e1eaa98f3342243b9304cf94f5d147394bc273` | Runtime completed，2 iterations，回答 3384 chars；eval FAIL，score 0.8515（50/61） | RCA 内容正确分析 timeline divergence；失败源是 11 items / 40852 bytes 的严格 canonical 截断，不是 Runtime 未完成 |
| multi-host | `turn-1783985652484311000`; seq 19 `sha256:796a8f4b8a4753fcf71bb53d19ba2938e5e1d171fda67f7bd40b96a57cb3c7fb` | Runtime completed，2 iterations，回答 5365 chars；eval adapter 严格截断 | 只验证 manager 对 A/B/C 的 advisory 拆分；未声称真实多 Host-Agent worker 执行通过 |
| no-tool（远端隔离服务） | `turn-1783985980732101515`; seq 10 `sha256:5544bde7a11259e56eff6234bfddacc0d033be59df1e59c3a119d7a7fc4194d7` | PASS 1/1，score 1.00，51/51；completed，2 iterations | 同一发布二进制在远端环境通过 |

真实样本产物：`output/task20/real-eval/`、`output/task20/real-read-only-local-2.json`、`output/task20/real-multi-host-advisory.json`。

远端验证使用隔离的 29180/29190 临时服务和 SSH tunnel；发布二进制 SHA-256 为 `e2b7db959648b34edf9cb613f69e8cc81af46977762ae2bcbaa8a6d56d06c9ab`。既有 19180/19190/7072 服务未修改，临时远端目录、tunnel 和进程已清理。

## 8. 已知限制与后续项

1. `exec_command pwd` 的风险分类和 evidence policy 当前使真实 approval resume 无法完成到 executor；若产品期望“明确只读 terminal 命令”可直接执行，应另开 policy 设计任务，不能在 UI 绕过 ActionToken。
2. 长真实模型输出会触发 AssistantTransport canonical agent-items 截断。建议把大型 typed artifacts 外置为 ref，并让 eval adapter 读取 ref，而不是放宽 canonical 上限或静默丢弃。
3. 真实 multi-host worker E2E 需要至少两个可控 Host-Agent target；本轮只完成 deterministic manager full-story 与真实模型 advisory 抽样。
4. Python 污点 checker 是 16-hop、函数内、Go/TS 的有限分析，不是完整语言证明；它与 production RunTurn parity、typed projector 对抗测试、坏 fixture self-test 共同构成多层门禁。

## 9. 发布判断

确定性 harness cutover 和 CI 防回退条件满足，可进入代码评审/合并流程。真实工具执行相关能力建议分阶段发布：

- no-tool、read-only facts、Prompt Trace、replay：可发布。
- mutation/terminal approval：保持 fail-closed，先修正或明确 policy 产品语义。
- 长 RCA/multi-host eval：先修 canonical ref hydration，再作为真实模型 release gate。

本报告不把受限真实样本计为通过；发布判断以“确定性控制链已锁定、真实不确定路径安全失败”为依据。
