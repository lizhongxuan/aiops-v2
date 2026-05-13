# AIOps Codex V2 — Eino Agent 后端

基于 [Eino Agent Framework](https://github.com/cloudwego/eino)（含 ADK）重写的 AIOps Codex AI Server 后端。

前端页面仍在 `aiops-codex/` 目录，本项目只包含后端 AI Server。前后端通过 HTTP/WebSocket/gRPC API 通信，所有端点路径、方法和 JSON 结构保持向后兼容。

## 架构概览

```
cmd/ai-server/main.go          # 启动入口，组装所有组件
internal/
├── runtimekernel/              # RuntimeKernel — 唯一的 turn 运行时内核
├── tooling/                    # 统一 Tool 抽象与 tool assembly 真源
├── commands/                   # Prompt/slash/local command registry
├── skills/                     # Skill catalog（通过 command surface 暴露）
├── agents/                     # Agent definition registry
├── mcp/                        # MCP server registry
├── integrations/               # 内建 integrations（direct registration）
├── plugins/                    # Plugin spec/loader/registrar 装配层
├── promptcompiler/             # PromptCompiler — 四层结构化 Prompt 编译
├── policyengine/               # PolicyEngine — 显式策略硬约束
├── projection/                 # Projection — 生命周期事件投影
├── modelrouter/                # ModelRouter — LLM Provider 路由与 Fallback
├── agentmgr/                   # AgentManager — Multi-Agent 编排（ADK）
├── spanstream/                 # SpanTree + MultiplexedStream 多路复用流
├── modeltrace/                 # 本地模型输入 trace 文件与 prompt fingerprint
├── observability/              # OpenTelemetry observer（本地 Phoenix trace UI）
├── eval/                       # 本地 agent eval case / runner / scorer
├── server/                     # HTTP/WebSocket/gRPC API 兼容层
├── store/                      # 数据持久化（内存 + JSON 异步写盘）
├── settings/                   # settings precedence / governance 聚合
├── hooks/                      # tool / turn lifecycle hooks
├── permissions/                # tool 执行权限治理
└── integration/                # 集成测试
```

## 当前注册模型（2026-04）

- `Tool` 是唯一模型可调用对象；`PromptCompiler`、`RuntimeKernel`、`AgentFactory` 共享同一份 `AssembledTools`。
- `skills` 不直接进入 tool pool，而是先进入 `SkillRegistry`，再通过 `CommandRegistry.ListSkillLikePromptCommands()` 暴露给 `SkillTool`。
- `AgentDefinition`、`MCPServerConfig`、`settings / hooks / permissions` 都不是 tool；它们分别进入独立 registry 或治理层。
- 旧 `capability` 兼容层已经删除；主运行链只认 `tooling.Tool`、`tooling.Registry`、`mcp.Registry`、`commands/skills/agents/...` 各自独立 registry。

## React Chat v2 终态规则（2026-05）

React Chat 的终态生产规则固定为 Codex 式 `Interleaved Transcript Blocks`。本次改造是从旧版本 `process[] + final + metadata.unstable_state` 一步迁到 React v2，不保留过渡规则，不保留 v1/v2 双生产路径。

assistant-ui 只负责 transport、runtime 和 UI primitives；AIOps 自己的 runtime、tool registry、skills、MCP、agent registry、policy、approval、artifact 语义仍由 AIOps 后端和 appui 管理。不要把 tool / skills / MCP / agent registry 搬进 assistant-ui，也不要用 assistant-ui 的通用 tool bubble 替代 AIOps 的过程时间线。

生产链路固定为：

```text
TurnItem / runtime event
-> AiopsTransportState(v2).turns[turnId].blockOrder + blocksById
-> AssistantTransport data stream state ops
-> React Chat 按 blockOrder 原序渲染
```

所有 assistant 可见输出都必须进入同一条有序时间线：正文、命令、搜索、文件操作、MCP 工具、审批、变更摘要引用和最终回答按真实事件到达顺序交错展示。前端不得再把内容分成“AI 正文区、工具区、最终答案区”；后端不得从 assistant final Markdown/text 解析过程 UI。

固定交互规则：

- 单个命令或工具在 `queued/running` 时自动展开，并实时追加 stdout/stderr 或工具输出。
- 单个命令或工具 `completed` 后自动折叠成一行灰色摘要。
- 单个命令或工具 `failed/blocked/rejected` 后保持展开，直接暴露错误、审批或阻断原因。
- 连续成功的短工具动作只在原时间线位置聚合成低噪音摘要，例如 `已探索 14 个文件,1 次搜索,2 个列表`；聚合不得跨越正文、审批或失败工具。
- 最终回答是普通 `TextBlock`，不是特殊 `final.text` 区域。
- `正在思考` 是时间线末尾的轻量状态 block，不和正在展开的工具同时竞争主视觉。

终态实现边界：

- Assistant message metadata 使用稳定自定义命名空间 `metadata.custom.aiops`，不使用 `metadata.unstable_state` 承载生产 transcript。
- 长命令 stdout/stderr、长 tool output、多工具并发必须通过 `blocksById` 路径的 `append-text` 追加，不靠整 turn `set` 刷新。
- `/api/v1/assistant/transport` 不能长期依赖固定高频轮询；终态必须使用事件驱动订阅，或至少使用动态 backoff 并在有输出时立即恢复短间隔。
- `mcpSurfaces` / `artifacts` 使用 AIOps typed schema 表达 Agent-to-UI 卡片、artifact preview、iframe/app surface、command binding 和 lifecycle state；transcript block 只引用这些对象，不内联大体积产物。
- Approval 的 UI 文案、transport decision 和 runtime decision 必须通过一套映射收敛，避免 `accept/reject/approved/denied/rejected` 多套表达扩散。

设计和实施文档：

- `docs/superpowers/specs/2026-05-08-aiops-v2-codex-chat-ui-design.md`
- `docs/superpowers/specs/2026-05-08-aiops-v2-codex-chat-ui-todo.md`

## 快速开始

```bash
cd aiops-v2
./scripts/start.sh              # 推荐：构建 web/dist + ai-server，并由 ai-server 托管前端
```

如果只想手动执行各步骤：

```bash
go test ./...                   # 运行全部测试
go build ./cmd/ai-server        # 编译
AIOPS_DATA_DIR=.data ./ai-server # 启动（需配置 LLM Provider）
```

常用启动覆盖：

```bash
AIOPS_HTTP_ADDR=:19080 ./scripts/start.sh
AIOPS_GRPC_AUTO_PORT=0 ./scripts/start.sh
SKIP_WEB_BUILD=1 SKIP_GO_BUILD=1 ./scripts/start.sh
./scripts/start.sh --dry-run
```

---

## 本地 Agent 调试与 Eval 闭环

当前本地调试链路固定为：Phoenix 看时间线，`/debug/prompts` 或 `.data/model-input-traces` 看完整模型输入，`cmd/agent-eval` 做回归。不要再为同一目标新增第二套 trace 存储、第二套 eval runner 或第二套 prompt 调试格式。

### 1. Phoenix Trace UI（可选、本地）

Phoenix 只作为本地 OpenTelemetry trace UI，不是生产依赖，也不保存完整 prompt。完整 prompt 仍只写入本地 model input trace 文件。

```bash
uvx --from arize-phoenix phoenix serve
```

启动 aiops server 时开启 trace：

```bash
AIOPS_HTTP_ADDR=:18080 \
AIOPS_OTEL_ENABLED=1 \
AIOPS_OTEL_ENDPOINT=http://localhost:6006/v1/traces \
AIOPS_OTEL_SERVICE_NAME=aiops-v2-agent \
AIOPS_DEBUG_MODEL_INPUT_TRACE=1 \
AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR=.data/model-input-traces \
./scripts/start.sh
```

Phoenix 中应能看到：

- `agent.turn` root span
- `model_call` child span
- 有工具调用时的 `tool_call.<tool_name>` child span
- `model_call` 上的 `prompt.stable_hash`、`trace.file`，以及存在相邻输入 diff 时的 `trace.diff`

默认禁止把完整 prompt 写入 span attribute。`AIOPS_OTEL_INCLUDE_PROMPT` 即使存在，也只能本地临时排查使用，不能写入源码、fixture、README 或报告。

### 2. 本地模型输入 Trace

开启 `AIOPS_DEBUG_MODEL_INPUT_TRACE=1` 后，每次模型调用会在 `.data/model-input-traces/<session>/<turn>/` 下生成 iteration trace。用于排查 prompt 优化时，先从 Phoenix 的 `trace.file` 或 `/debug/prompts` 定位到本轮输入，再看：

- 当前 iteration 的完整模型输入
- `promptFingerprint` / `prompt.stable_hash`
- 非首轮或 resume 后生成的 `input.diff.md`
- visible tools、developer rules、runtime policy 是否符合预期

Phoenix 负责定位“哪一轮慢、哪一步错、哪个 tool 失败”；本地 trace 文件负责看“模型到底看到了什么 prompt 和 tool surface”。

也可以直接打开 Web 页面：

```text
http://127.0.0.1:18080/debug/prompts
```

这个页面只读 `.data/model-input-traces`，会按时间列出最近的模型输入 trace。右侧默认是 Cards-first 视图，支持 `概览`、`Prompt 层`、`Messages`、`Tools`、`Diff`、`Raw`：先用卡片看完整 prompt 分层、message 顺序、visible tools 和 prompt hash，最后再用 `Raw` 查看原始 Markdown/JSON 兜底。它不创建新 trace、不修改 prompt、不替代 Phoenix；只是把原来需要手动找文件和翻 JSON 的步骤收敛成一个本地调试界面。

详细用法见 `docs/agent-trace-eval-guide.zh.md`。

真实 LLM smoke 验证方式：

```bash
# 前提：已在 设置 -> LLM 配置 中配置本地 provider，或通过 /api/v1/llm-config 写入。
# API key 只能来自本地环境变量或 UI 输入，不写入源码/README/fixture。
curl -sS -X POST http://127.0.0.1:18080/api/v1/chat/message \
  -H 'Content-Type: application/json' \
  -d '{"message":"请只回复这一行：PROMPT_TRACE_REAL_LLM_SMOKE"}'

curl -sS 'http://127.0.0.1:18080/api/v1/debug/model-input-traces?limit=1'
```

验收点：

- 最新 trace 的 `markdownPath` 指向刚才 turn 的 `iteration-000-*.md`。
- 打开 `/debug/prompts` 后，左侧最新项是刚才的 session/turn。
- 右侧默认 `概览` 能看到本次 LLM 输入、message/tool 数、prompt size、user message、prompt fingerprint、visible tools 和异常提示。
- `Prompt 层` 能按 system/developer/tool/runtime/conversation 分层查看完整 prompt。
- `Messages` 能按 provider 真实顺序查看发给 LLM 的 message。
- `Tools` 能确认 visible tools、工具描述和 tool registry prompt。
- `Raw` 仍能查看原始 Markdown/JSON；如果一轮内有第二次模型调用，`Diff` 能看到相邻输入变化。

### 3. Agent Eval

快速 mock 回归：

```bash
go run ./cmd/agent-eval \
  -agent mock \
  -priority P0 \
  -cases testdata/eval_cases \
  -out .data/eval_runs/prompt-p0-mock
```

真实本地 server E2E：

```bash
go run ./cmd/agent-eval \
  -agent server \
  -server-url http://127.0.0.1:18080 \
  -priority P0 \
  -cases testdata/eval_cases \
  -out .data/eval_runs/prompt-p0-server \
  -poll-timeout 2m \
  -poll-interval 1s
```

`cmd/agent-eval` 会生成：

- `report.json`：机器可读分数、checks、baseline comparison、prompt fingerprints
- `report.md`：人工可读失败原因、缺失项、异常工具调用、prompt fingerprint 摘要
- 每个 case 的 `answer.txt`、`events.json`、`tool_calls.json`、`turn_items.json`

常用参数：

- `-priority P0|P1|P2`：只跑指定优先级 case
- `-baseline <report.json>`：和旧报告对比，标出 better/worse/same/new/missing
- `-save-baseline <path>`：把当前报告保存成后续 baseline
- `-agent server`：通过真实 `/api/v1/chat/message` 和 `/api/v1/state` 跑本地 aiops server

Server eval 会把 `eval.caseId`、`eval.rootCauseCategory`、`eval.priority` 写入 chat metadata，并把真实 `AgentEvent` 还原成现有 `eval.RunOutput`。失败的 tool result 会保留为 `tool_result(status=failed)`，不能误归类成新的 `tool_call`。如果真实模型触发审批阻断或长请求，`-poll-timeout` 会作为端到端上限让 case 有界失败，避免无人值守回归一直挂住。

### 4. Prompt Diagnose 自动归因

改 prompt 后不要只看单次输出。推荐职责边界是：Phoenix trace 负责看时间线，Prompt Trace 负责看完整模型输入，`cmd/agent-eval` 负责打分，`cmd/prompt-diagnose` 负责把 report、artifacts 和 trace 汇总成“为什么失败、该改哪一层、有没有回归”。

只诊断已有 eval report：

```bash
go run ./cmd/prompt-diagnose \
  -report .data/eval_runs/prompt-p0-server/report.json \
  -cases testdata/eval_cases \
  -trace-dir .data/model-input-traces \
  -out .data/prompt_optimization/prompt-p0-server
```

带 baseline/current 对比：

```bash
go run ./cmd/prompt-diagnose \
  -report .data/eval_runs/prompt-p0-server-current/report.json \
  -baseline .data/eval_runs/prompt-p0-server-baseline/report.json \
  -cases testdata/eval_cases \
  -trace-dir .data/model-input-traces \
  -out .data/prompt_optimization/prompt-p0-server-current \
  -fail-on-regression
```

一键跑 eval + 诊断：

```bash
./scripts/prompt-regression.sh \
  --agent server \
  --server-url http://127.0.0.1:18080 \
  --priority P0 \
  --cases testdata/eval_cases \
  --trace-dir .data/model-input-traces \
  --baseline .data/eval_runs/prompt-p0-server-baseline/report.json \
  --out .data/prompt_optimization/server-current \
  --fail-on-regression
```

定向重跑：

```bash
# 只重跑一个 case
./scripts/prompt-regression.sh \
  --agent server \
  --server-url http://127.0.0.1:18080 \
  --case-id tool-calling \
  --cases testdata/eval_cases \
  --trace-dir .data/model-input-traces \
  --out .data/prompt_optimization/tool-calling-current

# 从旧报告中抽失败 case 重跑
./scripts/prompt-regression.sh \
  --agent server \
  --server-url http://127.0.0.1:18080 \
  --failed-from .data/eval_runs/prompt-p0-server/report.json \
  --cases testdata/eval_cases \
  --trace-dir .data/model-input-traces \
  --out .data/prompt_optimization/failed-rerun
```

为失败或退化 case 生成草稿：

```bash
go run ./cmd/prompt-diagnose \
  -report .data/eval_runs/prompt-p0-server-current/report.json \
  -baseline .data/eval_runs/prompt-p0-server-baseline/report.json \
  -cases testdata/eval_cases \
  -trace-dir .data/model-input-traces \
  -out .data/prompt_optimization/prompt-p0-server-current \
  -draft-cases-out .data/prompt_optimization/prompt-p0-server-current/draft-cases
```

输出文件：

- `diagnosis.json`：机器可读诊断摘要。
- `diagnosis.zh.md`：中文失败归因和证据摘要。
- `compare.zh.md`：baseline/current 的 better/worse/same/new/missing。
- `trace-links.md`：case 到 Prompt Trace 文件的映射，包含可打开 `/debug/prompts?trace=...&caseId=...` 的本地深链。
- `suggestions.zh.md`：人工修改建议。
- `failed-cases.json`：失败或退化 case 子集。
- `run-metadata.json` / `trend.zh.md`：本次 run 摘要和最近历史趋势；默认追加到 `.data/prompt_optimization/history.json`。

`prompt-diagnose` 是只读诊断层：不修改 prompt、不启动新 runner、不新增 trace 存储、不把完整 prompt 正文写入报告。
`scripts/prompt-regression.sh` 会把最近一次输出目录写到 `.data/prompt_optimization/latest_run.txt`，方便回看。

可选 LLM 辅助建议默认关闭，只发送 diagnosis 摘要，不发送完整 prompt：

```bash
AIOPS_LLM_BASE_URL=http://127.0.0.1:8317/v1 \
AIOPS_LLM_API_KEY=... \
AIOPS_LLM_MODEL=gpt-5.4 \
./scripts/prompt-regression.sh \
  --agent server \
  --server-url http://127.0.0.1:18080 \
  --priority P0 \
  --cases testdata/eval_cases \
  --trace-dir .data/model-input-traces \
  --out .data/prompt_optimization/server-current \
  --llm-suggestions
```

### 5. 从失败结果固化 Eval Case

当一次真实运行暴露 prompt/tool/policy/model 问题时，用 eval artifacts 草拟 case：

```bash
go run ./cmd/agent-eval-case \
  -id my-regression-case \
  -category prompt \
  -input "原始用户输入" \
  -answer-file .data/eval_runs/<run>/<case>/answer.txt \
  -tool-calls-file .data/eval_runs/<run>/<case>/tool_calls.json \
  -turn-items-file .data/eval_runs/<run>/<case>/turn_items.json \
  -out testdata/eval_cases/my-regression-case.json
```

生成的 `.draft.md` 只作为人工审核辅助。正式 case 必须人工收敛到稳定断言，避免把一次模型措辞直接固化成脆弱测试。

### 6. 调试顺序

1. 跑一次真实 turn，在 Phoenix 找最新 `agent.turn`。
2. 看 `model_call` 是否耗时异常、是否有错误状态、是否缺少工具调用。
3. 打开 `/debug/prompts` 或 `trace.file`，检查 prompt、developer rules、tool surface、runtime policy。
4. 如果不是首轮，打开 `trace.diff`，确认 resume/approval 后上下文变化。
5. 把失败固化成 eval case，再修改 prompt/tool/policy/model。
6. 先跑 `-agent mock -priority P0`，再跑 `-agent server -priority P0`。
7. 跑 `cmd/prompt-diagnose` 或 `scripts/prompt-regression.sh`，确认目标 case 变好、其他 P0 没退化，并看诊断建议该改 prompt/tool/context/policy/completion_gate 哪一层。

提交前建议：

```bash
./scripts/check-agent-tuning.sh HEAD
./scripts/check-agent-tuning.sh --run HEAD
./scripts/phoenix-smoke.sh
go test ./internal/eval ./cmd/agent-eval ./internal/promptdiag ./cmd/prompt-diagnose ./internal/observability ./internal/runtimekernel -count=1
```

---

## 长时间运行 Runtime Guardrails

这部分约束的是 `runtimekernel` 主运行链。新增 tool、prompt、checkpoint、session 恢复、streaming、compaction 时，必须先满足这里，再谈局部功能。

### 1. 主运行链只有一条

- 长时间运行 turn 只能由 `runtimekernel.EinoKernel` 驱动
- host 路径的标准链路是：
  `model -> assistant/tool_use checkpoint -> ToolDispatcher -> progress/tool_result checkpoint -> next iteration`
- 不允许再造第二套 runtime kernel、第二套 turn loop、第二套 tool execution 旁路
- 不允许在 feature 代码里直接调用 tool executor 绕开 `ToolDispatcher`

### 2. Checkpoint / Suspend / Resume 是硬约束

- assistant response 和 parsed `tool_use` 必须先落 checkpoint，再进入工具执行
- `progress`、`partial result`、`tool_result` 必须支持增量 checkpoint，不能只在整批 tool 结束后一次性落盘
- `NeedApproval` / `NeedEvidence` 必须进入显式 `suspended` 或 `resumable` 状态
- 不允许出现“逻辑上 blocked，但 runtime 没有 checkpoint/pending state”的隐式中断

### 3. Context / Result 只有一套治理路径

- trim、compaction、spill、summary 必须在统一 context pipeline 中决策
- tool result budget 只有一套标准语义：
  `MaxInlineResultBytes`、`ResultSpillPolicy`、`SummarizeLargeResult`
- runtime 结果承载只允许三段策略：
  小结果内联；中结果摘要/预览内联且全文外溢；大结果只回灌摘要与引用
- `ToolResult` 可以带 `blob/card/file` 引用，但这些引用仍要进入 runtime message / checkpoint / session external references 主路径
- 不允许额外引入“只给 UI 看”的隐藏结果存储，或者另一套私有 compaction 状态

### 4. Prompt / Tool 刷新必须按 iteration 收敛

- stable prompt 与 dynamic prompt 的分层只能在 `promptcompiler` 中完成
- tool surface 只能来自 iteration 级 assemble/refresh，不允许手写额外 tool prompt side channel
- permission mode、hooks、MCP topology 造成的 tool 可见性变化，必须通过下一 iteration 的 tool refresh 生效

### 5. 目前仍未完成的边界

- `WorkspaceTask` 的创建/消费主路径仍主要属于后续 multi-agent 生命周期层；当前已具备 store-backed 持久化与 restart reconcile，但不是日常 host turn 主链的一部分

### 6. 未来新增功能的落地规则

- `docs/longtime_design_0422.md` 与 `docs/longtime_todo_0422.md` 的实施顺序固定按 `P0 -> P4` 表达；后续增强项只能追加在 `P4` 之后，不能重排主线
- workspace/host 都必须继续走共享的 `RunTurn -> runHostIterationLoop` 主链；禁止重新引入 legacy workspace runtime path 或其他临时旁路
- 如果新增功能需要 prompt、tool、checkpoint、session 恢复、streaming、compaction 变化，先改主链和设计文档，再落具体 feature

以后新增功能，如果会碰到这些边界，先改设计和主链，不要在边缘逻辑里补兼容分支。

---

## ERP SRE Runtime Guardrails

本节约束 ERP 生产事故处理主链。ERP SRE 不是“会 SSH 的聊天页面”，它必须收敛到同一条受治理的 runtime/tool/approval/checkpoint 路径。

### 1. 主链不变量

- Runbook 不执行工具，只匹配、规划、推进状态，并生成 `ActionProposal`。
- fallback 不执行命令，只在无合适 Runbook 时生成受治理的 `ActionProposal`。
- 真实执行只能通过模型 `tool_use -> ToolDispatcher -> Policy / Approval -> checkpoint`。
- 非只读生产 `exec_command` 必须先有有效 `ActionToken`，再进入 approval。
- 任何 feature 代码都不能直接调用 tool executor、K8s client、Coroot tool 或 shell executor 绕过 `ToolDispatcher`。
- Coroot、ERP 图谱、Runbook、Incident、K8s、exec 都只能作为同一 tool surface 下的能力，不允许新增第二套 runtime loop、tool registry、approval 入口或 capability pool。

### 2. 前端运行态约束

- 事故页面不能新增私有 WebSocket、SSE、EventSource 或 polling 运行态。
- ERP SRE 页面 HTTP 只能经 `web/src/api/*`；Chat / Protocol 运行过程只能经 `AssistantTransport` 写入 `AiopsTransportState`。
- 事故工作台只能消费 `AiopsTransportState` 或对应 API view-model；runbook、proposal、tool、approval、verification 和 postmortem 过程态不能再从旧 projection 推导。
- 页面不能从 assistant final text、Markdown、`snapshot.toolInvocations` 或局部 running flag 推断真实执行状态。

### 3. 统一命名

| 名称 | 语义 |
| --- | --- |
| `ActionProposal` | Runbook、fallback、manual 或 break-glass 生成的动作提案，只描述将要调用的真实 tool、输入、风险、证据、预期效果和验证方式 |
| `ActionToken` | 绑定 session/turn/incident/toolName/normalized input/risk/expiry 的执行授权，非只读生产动作必须携带 |
| `IncidentCase` | ERP 事故主对象，聚合来源、严重级别、业务影响、证据、动作、审批和复盘 |
| `RunbookInstance` | 某个事故中一次 Runbook 执行状态，只推进步骤和生成下一步 proposal |
| `FallbackPlan` | 无合适 Runbook 时的受治理临时处置计划，输出一个或多个 `ActionProposal` |

新增 ERP SRE 功能前，先确认它能落入这些对象和主链；如果需要新对象，先更新设计文档和本节。

---

## Web Product Surface Guardrails

本节约束 `web/` 与 `ai-server` 的正式产品面。新增页面、接口、实时事件、登录、terminal、settings、hosts、MCP、agent profile 时必须遵守这里，不允许重新把旧项目协议当成永久兼容层。

### 1. 前端只有一套数据入口

- 页面和组件不允许直接 `fetch(...)` 调业务 API，必须经由 `web/src/pages/*Api.ts`、`web/src/lib/*` 或专用 API client。
- Chat / Protocol 不允许直接 `new WebSocket(...)`、SSE、polling 或 page-local stream；生产流式入口只有 `AssistantTransport`。
- 旧 `store.js`、Vue entry、Vue router 与 `web/src/realtime/appSocket.js` 已删除；不能作为兼容层重新引入。
- 非 Chat 的专用实时能力必须有明确域边界，例如 terminal 只能使用 `/api/v1/terminal/ws`。

### 2. 后端只有一套 Web API 应用层

- `internal/server` 只做 transport：路由、decode、status code、cookie、ws framing
- 业务投影和命令翻译只能放在 `internal/appui`
- `internal/server` 不允许直接拼 runtime/session/store/mcp/auth/terminal 的业务对象给前端
- 不允许新增 `webcompat`、legacy pusher、私有 snapshot builder 或页面专用 handler

### 3. Chat / Protocol 只能走 RuntimeKernel 主链

- 新消息只能进入 `runtimekernel.RunTurn -> runHostIterationLoop`
- approval decision、choice answer、evidence follow-up 都必须回到同一条 `ResumeTurn` path
- stop/cancel 只能进入 `runtimekernel.CancelTurn`
- 不允许为 protocol workspace、chat 页面或某张 UI card 新造第二套 orchestrator

### 4. Snapshot / WS 只有一套真相源

- Chat 生产状态只能来自 `/api/v1/assistant/transport` 与 `/api/v1/assistant/resume`，并落入 `AiopsTransportState`。
- `/api/v1/state` 只能作为非 Chat 页面 API view-model 或历史兼容输入，不能驱动 React Chat 过程 UI。
- terminal 必须使用独立 `/api/v1/terminal/ws`，不能混进 AssistantTransport。
- 已删除的 legacy `WebSocketPusher`、`LegacyMessage`、Vue router/store、旧 realtime app socket 不能重新引入。

### 5. AssistantTransport 是 React Chat 唯一生产运行态

- React Chat、Protocol、host-agent、tool、MCP、approval、diff、browser 验证的生产过程 UI 只能进入 `AiopsTransportState`。
- 后端 `TurnItem` / `AgentEvent` 可以作为 runtime typed fact 或兼容内部模型保留，但不能作为 React Chat 生产 truth source 暴露给页面。
- 旧 `turn_event`、`agent_event` WebSocket reducer、`AgentEventProjection` selector、`codexProcessTranscript`、`ChatProcessFold` 不能作为 React Chat 生产路径重新引入。
- `AiopsTransportState.schemaVersion` 固定使用 `aiops.transport.v2`；旧 `process[] + final` schema 只能作为被删除对象，不能作为生产兼容路径保留。
- `internal/server` 不允许拼页面私有运行态；AssistantTransport command decode、state ops、resume/cancel 必须经 `internal/appui` / runtime 主链。
- 前端只能有一套 Chat 运行态 reducer：assistant-ui transport runtime；本地 optimistic、send pending、stop、failed、approval blocked、tool progress 都必须表示成 transport state ops。
- Busy/Working 必须由 `AiopsTransportState.status` 与 `runtimeLiveness` 推导：active turns、active agents、active command streams、pending approvals、pending user inputs。
- 主 UI 不显示 UUID/call-id/raw id，只显示 agent handle/name、阶段摘要、diff stats、approval/artifact 入口。

新增或修改 Chat / Protocol / 工作台 UI 时必须同时满足：

- 后端新增运行过程：只扩展 v2 `AiopsTransportState` 相关 DTO、projector/state op 和 AssistantTransport stream writer；不得回写旧 `process[] + final`。
- 前端新增运行过程：先扩展 `web/src/transport/aiopsTransportTypes.ts`、runtime/converter 和对应 UI part；页面只消费 `useAssistantTransportState()`，assistant message 自定义数据只放在 `metadata.custom.aiops`。
- 本地 optimistic、send pending、stop、failed、approval blocked、tool progress 都必须写成 transport command 或 state op，不能只改页面局部变量或 legacy runtime flags。
- 单机会话和协作工作台必须共用 `AiopsTransportState`；允许布局不同，不允许状态模型不同。
- `snapshot.cards` 只能作为持久会话内容 / card artifact 输入，不能作为 running/busy/approval/tool progress 的真相源
- `snapshot.toolInvocations` 只能作为历史证据/详情兼容输入，不能驱动主 Chat 工作流里的实时 process fold
- 单机会话默认运行模式必须是 `ModeExecute`；前端发送消息通常只带 `sessionId`，后端 `SendMessage` 必须从 session 回填 `type/mode/host`，不能让单机会话回落到 `chat` 只读 prompt
- 单机会话过程 UI 必须收敛到一套 assistant-ui transport projection；不能并行用 `LiveStatusCard`、`statusCard`、legacy process cards 或 page-level activity label 生成同一轮运行过程
- 命令、搜索、文件、MCP 等过程行必须使用 typed fields（例如 `displayKind`、`inputSummary`、`command`、`outputPreview`）渲染；不能靠 UI 文案正则重新分类
- 过程行去重必须使用 typed semantic key（例如 `displayKind + command/inputSummary/queries/results`），不能只按可见文案去重，否则会把多次同名搜索或命令错误折叠掉
- `已记录 X 条过程细项`、`明细已折叠`、`处理失败 X 条明细`、`准备上下文`、`编译提示词`、`准备工具`、`调用模型` 这类不面向用户的过程文案不得从上游生成；不得用组件条件或 CSS 隐藏作为修复
- 如果新增字段需要“正在执行/等待审批/已停止/失败/Working”文案，必须能从 `AiopsTransportState.status`、turn status 或 `runtimeLiveness` 推导，不能靠 card title/status 文案正则
- Codex 原生过程 UI 终态以 `docs/superpowers/specs/2026-05-08-aiops-v2-codex-chat-ui-design.md` 和 `docs/superpowers/specs/2026-05-08-aiops-v2-codex-chat-ui-todo.md` 为准；修改单机会话过程 UI 时必须同步更新对应验收项
- 真实 LLM Playwright 回归只能通过 `AIOPS_TEST_LLM_BASE_URL`、`AIOPS_TEST_LLM_API_KEY`、`AIOPS_TEST_LLM_MODEL` 注入配置；API key 不允许写入源码、fixture、README、验收报告或截图命名
- 真实 LLM Playwright 回归必须用临时 `AIOPS_DATA_DIR` 启动服务，并在测试结束后清理；不能把测试 API key 写入项目默认 `.data` 或可提交配置
- 代码评审前必须运行：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "snapshot\\.toolInvocations|store\\.runtime\\.turn|processItemsByTurnId|phaseFoldsByTurnId" web/src
```

以上命令在生产 `web/src` 中必须无旧 Chat truth source 命中；如有测试/fixture/debug 命中，必须明确不参与 React Chat 生产路径，否则先迁移到 `AiopsTransportState`。

### 5.1 Codex-style 结构化流式输出规则

`aiops-v2` 的结构化流式输出主路径固定为：

```text
model/tool/runtime -> TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React
```

这条路径覆盖 plan、tool/search/command、evidence、approval 和 final answer。以后新增运行过程 UI 时，只能扩展 `AiopsTransportState` typed fields、AssistantTransport command/state op、converter 和对应 React part，不能新增并行结构化输出协议。

硬约束：

- 不允许新增 `StructuredResponsePatch`、`emit_response_events`、`StructuredResponsePanel` 作为主路径。
- 不允许页面私有 `new WebSocket(...)`、SSE 或 polling 通道绕过 AssistantTransport；专用终端 WebSocket 只能用于 terminal 域。
- 不允许从 assistant final text 解析 `summary/steps/actions`、command、approval、completed、failed 或 plan 状态。
- `update_plan` 只能落为结构化 `TextBlock` 或 `ToolBlock` 元数据，再经 `AiopsTransportState.turns[*].blockOrder + blocksById` 进入 React Chat。
- command/search/evidence/approval 必须使用 typed fields，例如 `displayKind`、`command`、`queries`、`results`、`source`、`confidence`、`rawRef`。
- final answer 只能作为普通 `TextBlock` 按原始到达顺序流式展示，不能复制到特殊 final 区或过程区。
- 高风险动作只能经 tool/policy/approval path 展示和执行，不能由模型正文伪造“已执行”状态。

提交或评审结构化流式输出相关改动前必须运行：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "JSON\\.parse\\(|markdown heading|summary.*steps.*actions" web/src
```

第一条在生产主路径中必须无命中。第二条允许 settings、fixture、realtime envelope 等普通 JSON 解析，但不能出现从 assistant final text 解析结构化 UI 的实现。

### 6. Approval UX 单入口规则

这次出现“已经有底部 `codex-approval-inline`，但仍显示对话审核 UI 卡片”的根因不是缺组件，而是审批展示路径没有归口：

- `ChatStream` 内有 `ApprovalDock`，会根据 `approvalDock` 渲染对话流里的审核卡片。
- `ChatPage` 底部 composer slot 内有 `codex-approval-inline`，设计目标是替换输入框。
- 旧 fallback `auth-overlay-dock` 仍保留，用于部分历史 card/MCP 审核场景。
- projection approval 和 snapshot approval 曾经分别走不同 computed，导致单机会话里同一个 pending approval 可以同时进入 `ChatStream` 和 composer。
- 早期 approval payload 只有 `title/reason`，真实命令被混在 reason 或缺失，UI 只能显示 `exec_command` 这类工具名。

以后任何审批 UX 改动必须遵守：

- 单机会话的用户审核入口只能是底部 `codex-approval-inline`，并替换输入框；不能在对话流中再渲染 `ApprovalDock` 或其他审核卡片。
- 协作工作台可以保留右侧/流程区审批视图，但必须显式由 workspace layout 消费，不能复用到 single-host 主聊天流。
- `ChatStream.approvalDock` 在 single-host 场景必须为空；如果需要展示等待审批状态，只能通过 composer approval 或轻量状态文案表达。
- `auth-overlay-dock` 只能作为 workspace 或特殊 MCP fallback；新增 single-host approval 不允许接入它。
- 审批展示必须使用真实业务对象：命令审批显示 `command`，文件审批显示 file/path/diff 摘要；禁止把 `exec_command`、`shell_command`、`code_mode` 等工具名当成用户可审内容。
- 后端 approval contract 必须分离 `command` 和 `reason`：`command` 给用户审核，`reason` 给策略解释；不能把策略原因塞进 command，也不能把 command 塞进 reason。
- transport state、snapshot 兼容输入、local optimistic approval 必须归一到同一个 composer approval selector；页面模板里不能再并行判断多套 pending approval 来源。
- 同一个 approval id 在页面上只能有一个可点击决策入口；如果同时出现 `codex-approval-inline` 和 `approval-dock`，视为 P0 UI 回归。
- 审批按钮提交必须继续走统一 decision API，不允许组件私有化决策逻辑或直接改 store 状态。
- `ResumeTurn` 处理批准决策时必须更新 approval resolved state op，再继续工具执行；不能只清 runtime pending state，否则旧 pending approval 会把同一命令重新推回底部审批栏。
- `RunTurn` / `ResumeTurn` 返回 `blocked` 或 `pending_approval` 时，appui async runner 只能保持 transport `blocked` 与 pending approval；不能补 failed state，否则 pending approval 会被失败态清空并丢失可点击审核入口。

修改审批相关代码时必须补或更新以下验证：

- 前端单元测试要断言 single-host pending approval 时：`[data-testid="approval-dock"]` 不存在，`[data-testid="codex-approval-inline"]` 存在，`.omnibar-wrapper` 不存在。
- 前端单元测试要断言显示真实 `command`，且不显示 `exec_command` 这类工具名。
- transport projector/runtime 测试要覆盖 approval `command` 从后端 typed fact 进入 `AiopsTransportState.pendingApprovals`。
- runtime 测试要覆盖批准路径会发 `approval.decided(status=approved)`，拒绝路径会发 `approval.decided(status=denied)`，两者都不能让 pending approval 留在 projection 中。
- 后端 snapshot/projector 测试要覆盖 `PendingApproval.Command` 与 `Reason` 分离。
- Playwright 验证要至少检查一次真实或 fixture pending approval：approval dock 数量为 0、inline 数量为 1、输入框被替换、点击同意会发送 `/api/v1/approvals/:id/decision`。

### 7. 正式域边界

- auth 真相源只能来自 `internal/auth`
- terminal 真相源只能来自 `internal/terminal`
- hosts/settings/agent profiles/skills/agent mcps 只能通过 `Store` 和对应 `appui` service 写入
- MCP runtime 状态只能通过 `internal/mcp.Registry` 和 `appui.MCPService` 投影，不允许页面直接改 runtime registry

新增 Web 功能的最小接入顺序：

1. 先在 `web/src/pages/*Api.ts`、`web/src/lib/*` 或 `AssistantTransport` 定义唯一前端入口
2. 再在 `internal/appui` 定义 service/DTO/command translation
3. 最后在 `internal/server` 补 transport handler
4. 如果涉及 chat/protocol 中断恢复，必须补 runtimekernel resume/cancel 测试
5. 如果涉及运行过程展示，必须先扩展 `AiopsTransportState` schema 与 AssistantTransport state op，再实现 projector/runtime/converter；不要用组件条件或 CSS 隐藏旧模块来替代上游数据清理

---

## ⚠️ Registration Upgrade Guardrails

本节是当前仓库的硬约束，不是建议项。它来自两部分依据：

- [`docs/registration-upgrade-todo.md`](./docs/registration-upgrade-todo.md) 的阶段目标与统一验收清单
- 本地 `../claude code/` 源码的对应实现面，重点参考：
  - `Tool.ts`
  - `tools.ts`
  - `commands.ts`
  - `skills/loadSkillsDir.ts`
  - `types/plugin.ts`
  - `utils/settings/constants.ts`

以后新增功能，如果会碰到注册、装配、治理、prompt 或 agent 编排，先看这节。看完还觉得需要新增第二套模型、第二条主路径或新的 runtime interface，先停下来确认方案。

### 1. 不可破坏的架构不变量

1. `Tool` 是唯一模型可调用对象。
   - `PromptCompiler`、`RuntimeKernel`、`AgentFactory` 只能消费 `AssembledTools`
   - `SkillDefinition`、`AgentDefinition`、`MCPServerConfig`、settings、hooks、permissions 都不是 tool

2. tool assembly 只有一个真源。
   - 必须通过统一装配路径生成 tool pool
   - 内置 tools、编排 tools、MCP dynamic tools 必须在同一套 registry 语义下组装
   - 同名冲突时，内置 tool 优先于 dynamic MCP tool

3. `ToolMetadata` 必须走 traits-first，而不是 kind-first。
   - 主字段是 `Name / Aliases / SearchHint / ShouldDefer / AlwaysLoad / IsMCP / IsLSP / MCPInfo`
   - `Origin` 只是兼容/展示字段，不是新的主分流轴
   - 不允许新增围绕 `Origin=builtin|mcp|meta` 的运行时分支

4. skills 只能通过 command surface 暴露给模型。
   - `skills.Registry` 存定义
   - `commands.CommandRegistry` 发布 skill-like prompt commands
   - `SkillTool` 消费 command surface，不直接消费孤立 `SkillDefinition`

5. agents、MCP servers、plugins 都是独立 registry，不是平行 tool 模型。
   - `AgentDefinition` 进 `internal/agents`
   - `AgentTool` 才是编排型 tool 视图
   - `MCPServerConfig` 进 `internal/mcp`
   - MCP 在 runtime 上表达为 tool source：`IsMCP + MCPInfo`

6. governance 只治理 tool，不定义 tool 类型。
   - `PermissionEngine`、`HookRegistry`、`FeatureFlags`、`settings.Governance` 只能影响暴露、审批、阻断、目录范围、MCP allowlist
   - 它们不能创建新的 capability 类别，也不能偷偷拼出第二套 tool pool

7. plugin / extension 只是装配层，不是 runtime 核心接口。
   - 可分发的组件面是 `commands / skills / agents / hooks / mcp / lsp / output styles / settings`
   - `RuntimeKernel`、`PromptCompiler`、`AgentFactory` 不应直接感知 plugin/extension

8. 旧兼容层已经删除，不允许重建。
   - 不允许重新创建 `internal/capability`
   - 不允许新增 `ExtensionManager` / `MCPServerManager`
   - 不允许新增 `LegacyToolRuntime` / `NewLegacyToolAdapter`
   - 不允许重新发明 `VisibleCapabilities` / `MCPPromptAssets` 一类旁路

### 2. Source Precedence And Governance

- settings/customization 的低到高覆盖顺序是：
  `userSettings -> projectSettings -> localSettings -> flagSettings -> policySettings`
- `policySettings` 的内部优先级只允许在 `internal/settings` 内部定义和合并，业务代码不得重新实现
- agent definition source precedence 是：
  `built-in -> plugin -> userSettings -> projectSettings -> flagSettings -> policySettings`
- `strictPluginOnlyCustomization` 生效时，只有 admin-trusted sources 还能继续往受控 surface 写内容
- 当前 admin-trusted sources 是：
  `plugin`、`built-in`/`builtin`、`bundled`、`policySettings`
- `allowedMcpServers` 与 `additionalDirectories` 属于治理层，不允许散落到单个 feature 的私有配置里

### 3. 新增功能时的接入规则

**新增 Tool**

- 优先实现 `internal/tooling.Tool`，简单场景直接用 `tooling.StaticTool`
- builtin/static tool 只允许 `tooling.Registry.Register(...)`
- dynamic MCP tool 只允许 `mcp.Registry.OnServerConnected(...)`
- tool 的可见性、defer/load 行为、MCP/LSP 来源、审批/权限都必须通过 metadata + assembly / execution pipeline 表达
- 不能在 `RuntimeKernel`、`PromptCompiler`、`PolicyEngine` 里给某个新 tool 写硬编码旁路

**新增 Skill**

- skill definition 放进 `internal/skills`
- skill-like command 放进 `internal/commands`
- 让 `SkillTool` 通过 command surface 发现它
- 不要再把 skill 当成另一种 tool kind 或 capability 树节点

**新增 Agent**

- 定义进 `internal/agents`
- 执行和调度留在 `internal/agentmgr`
- 如果模型需要编排它，暴露 `AgentTool` 风格视图，而不是把 `AgentDefinition` 本身塞进 tool pool
- agent scope 过滤必须围绕 assembled tools 与 metadata traits，不允许退回旧 `Kind*` 主轴

**新增 MCP 能力**

- 注册/管理放在 `internal/mcp`
- 运行时表达必须是 dynamic tool + `IsMCP + MCPInfo`
- 不允许新增 “第二种 MCP tool 模型” 或 “专供 MCP 的 prompt 旁路”

**新增 Plugin / Extension 能力**

- 只允许贡献到已有 registry surface：`commands / skills / agents / hooks / mcp / lsp / output styles / settings`
- builtin integration 只允许通过 `cmd/ai-server/registerBuiltinIntegrations(...)` 直连目标 registry
- plugin 只允许通过 `plugins.ManifestLoader + Registrar`
- 不允许让 plugin/extension 反向控制 `RuntimeKernel`、`PromptCompiler`、`AgentFactory`
- 不允许因为 plugin 需要而新增第二套注册模型或运行时接口

**新增 Prompt / Policy / Governance 规则**

- tool prompt 只能来自 `AssembledTools`
- 非 tool 的补充上下文只能走 `SkillPromptAssets` 或 `ExtraSections`
- prompt 文案不能替代硬策略；真正的 allow / deny / ask 必须进 `PolicyEngine` / `PermissionEngine`
- 新 mode / policy / governance source 若改变行为，必须同步补装配与回归测试

**新增 LLM Provider**

- 必须实现 Eino `model.ChatModel`
- 必须通过 `ModelRouter` 接入
- 不能在 `RuntimeKernel` 里直接调 provider SDK

### 4. 明确禁止的做法

- 禁止创建平行 tool 池、局部 tool 列表或 “仅 prompt 可见 / runtime 不可执行” 的旁路
- 禁止在主运行链路新增任何 capability-kind 风格分类轴
- 禁止重新引入 `VisibleCapabilities -> PromptCompiler` 这类二次筛选模型
- 禁止重新引入 legacy `MCPPromptAssets` 之类的 MCP prompt side channel
- 禁止把 `SkillDefinition`、`AgentDefinition`、`MCPServerConfig`、hooks、permissions、settings 直接塞进 tool pool
- 禁止让 plugin/extension 直接修改 `RuntimeKernel`、`PromptCompiler`、`AgentFactory` 的主逻辑
- 禁止在 prompt helper、loop nudge 或局部 command 里散写系统级规则
- 禁止为了新功能重建 `capability`、legacy adapter、compat manager

### 4.1 以后不允许重新引入的旧入口

- 不允许重新创建 `internal/capability/*`
- 不允许新增 `ExtensionManager` / `MCPServerManager`
- 不允许新增 `LegacyToolRuntime` / `NewLegacyToolAdapter`
- 不允许让 `SkillPromptAssets` 绕过 `CommandRegistry.ListSkillLikePromptCommands()`
- 不允许在 `internal/agents` / `internal/agentmgr` 定义层重新引入 `CapabilityKinds`、`Hosts`、`HostScope`
- 不允许新增 `policyengine.CheckCapability(...)` 或等价 wrapper

### 5. 需要改设计而不是直接写代码的情况

出现下面任一情况，先更新设计文档并确认，再继续编码：

- 你觉得现有 `ToolMetadata` traits 不够表达需求
- 你想新增新的 capability kind / source kind / runtime interface
- 你想让某类对象既不是 tool、又要被模型直接调用
- 你想让 plugin/extension 直接参与 runtime 决策
- 你想绕过统一 source precedence 或 governance merge 逻辑

### 6. 提交前自检清单

- 新增的模型可调用对象是否真的是 `Tool`，而不是别的定义类型
- `PromptCompiler`、`RuntimeKernel`、`AgentFactory` 是否复用了同一份装配结果
- 是否使用了 `ToolMetadata` traits，而不是新增 kind 分支
- skills 是否通过 command surface 暴露
- MCP 是否通过 `IsMCP + MCPInfo` 表达，而不是通过并列 kind 表达
- plugin/extension 是否只做分发，不做 runtime 主逻辑
- governance / precedence 是否复用了现有聚合逻辑，而不是局部重写
- 是否补了单元测试；跨层不变量是否补了 property tests
- 是否运行了 `go test ./...`
- 是否运行了 `go build ./cmd/ai-server`

## 通用规则

1. 注册制优先：所有模块通过统一接口注册，不允许平行能力池或硬编码旁路
2. 接口隔离：各层通过接口通信，禁止跨层直接引用实现
3. 特殊情况必须确认：如果现有接口无法满足需求，必须先讨论方案，再修改接口或添加代码
4. 测试覆盖：新增模块必须包含单元测试；涉及跨层正确性不变量的必须补 `pgregory.net/rapid` property tests
5. 不修改 `aiops-codex/`：所有后端代码变更限定在 `aiops-v2/`
6. 任何影响四层 prompt 语义、tool lifecycle 真源、workspace/host 隔离、source precedence 的变更，都必须同步更新设计/README

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
- 通过 `tooling.Visibility` 或 `Tool.IsEnabled(...)` 控制可见性
- 通过 `Tool.IsReadOnly()` / `IsDestructive()` / `IsConcurrencySafe()` 声明属性
- 通过 `ModePolicy.CheckTool()` 和 `tooling.ToolMetadata` 定义边界
- 通过参数/metadata/source 规则过滤

### 禁止在 Prompt 中补偿代码缺陷

不要因为 PolicyEngine 没有正确拦截某个操作，就在 prompt 中加一句"你不能执行 xxx"来补偿。Prompt 是建议，PolicyEngine 是硬约束。

正确做法：在 `ModePolicy` 或 `PermissionEvaluator` 中添加对应的检查规则。

### 禁止绕过注册机制的"快速修复"

不要因为赶时间，直接在 `eino_kernel.go` 的 `RunTurn` 中插入特殊处理逻辑。所有能力必须通过 Registry 注册，所有策略必须通过 PolicyEngine 执行。

如果现有接口确实无法满足需求，**停下来，跟用户确认方案**，而不是绕过架构。

---

## 📚 扩展文档

- [注册规则升级设计方案](docs/registration-upgrade-design.md) — 基于 claude code 源码分析的注册机制增强计划
- [Agent Trace / Eval / Prompt 优化主指南](docs/agent-trace-eval-guide.zh.md) — 本地 trace、Prompt Trace、eval、诊断和 prompt 优化闭环

## 测试

```bash
go test ./...                           # 全量测试
go test -run TestProperty ./...         # 只跑属性测试
go test -count=1 ./...                  # 清缓存跑
```
