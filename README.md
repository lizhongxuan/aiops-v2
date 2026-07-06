# AIOps V2

`aiops-v2` 是一个面向生产运维场景的 AI 运维工作台。它不是单纯的聊天后端，而是把 AI Chat、受治理工具调用、审批、Runner Workflow、运维手册、Run Record、Prompt Trace、Eval 回归和 React 前端页面收敛在同一个仓库里。

核心目标：让 AI 能在明确边界内完成“理解问题 -> 调用只读证据 -> 匹配或生成运维手册 -> 预检 -> 确认/审批 -> 执行 -> 验证 -> 沉淀经验”的闭环，同时保证高风险动作不能被模型正文或前端状态绕过。

## AIOps 最终目标

AIOps 的最终目标不是“能聊天”或“多一个监控面板”，而是把真实生产问题变成可解释、可处置、可验证、可沉淀的运维闭环。

### 看见真实问题

统一接入 Coroot、Alertmanager、K8s、日志、指标、链路、变更，过滤噪音，聚合成 Case。

### 解释为什么出问题

自动采集证据，输出可信 RCA：根因候选、支持证据、反证、缺失证据、证据边界。

### 知道该怎么处理

匹配运维手册、Runner Workflow、历史 Case、Run Record，给出可执行计划。

### 安全地执行和验证

预检、审批、ActionToken、执行、回滚、验证恢复，全部审计。

### 越用越有经验

成功 Case 沉淀为手册，失败 Run Record 反哺禁用条件，SLO、MTTR、告警压降率证明价值。

## 功能介绍

### AI Chat 与运行时

- 基于 Eino Agent / ADK 组装模型调用、tool call、checkpoint、approval 和 resume。
- `runtimekernel` 是唯一 turn 运行时内核；所有 host/workspace 运行都必须进入同一条 `RunTurn` / `ResumeTurn` 主链。
- React Chat v2 使用 `Interleaved Transcript Blocks`：正文、命令、搜索、文件操作、MCP 工具、审批和最终回答按真实到达顺序交错展示。
- Assistant 输出必须统一使用 `assistant_message + phase + streamState`：`phase=commentary` 保留真实时间线顺序，例如 `tool -> 对话 -> tool -> tool -> 对话`；对话结束时 `phase=final_answer` 只进入最终回答区，其它过程内容默认折叠。
- Chat 过程可以对同类网页搜索和同类命令做 Codex-like 折叠汇总，避免刷屏；普通工具发现、MCP 调用、审批、artifact 等不能全部压成一个“已调用 N 个工具”。
- 单机会话默认是可执行运维模式，不是只读闲聊；非只读或高风险动作必须经 Policy / Permission / Approval。

### AI Chat Host Mentions

AI 对话支持 `@主机` 作为显式主机作用域。只是在某个主机上执行一条明确的 `exec` 命令时，可以由 AI Chat 当前 turn 直接走受治理 `exec_command`，不必启动主机 Agent。只有针对该主机的复杂运维任务才启动 host-bound child agent，并把运维子任务发送给它执行。一个主机最多对应一个主机 Agent；主机 Agent 的命令仍需回到 AI Chat 审批，审批卡片必须显示主机和命令。多主机复杂任务可先进入 Codex-like plan 机制做规划，再把子任务分派给对应主机 Agent。

### Runner Workflow

- `pkg/runner` 提供独立 Runner module，支持 YAML workflow、graph workflow、builtin probes、shell/script/http action、run state、run record、workflow store 和 Runner Studio。
- `web/src/pages/RunnerStudioPage.tsx` 提供可视化编排、节点配置、graph validate、发布前检查、publish review 和运行状态查看。
- Runner Workflow 是生产操作的执行载体；Runbook 只做规划和提案，不直接执行工具。

### 运维手册与闭环资产

AIOps 的核心资产是“运维手册 + Runner Workflow + Run Record”：

- 运维手册说明适用对象、操作类型、环境、参数、前置检查、验证方式、风险、不能使用条件和降级处理。
- Runner Workflow 承载经过验证的步骤、参数定义、预检、执行和验证节点；发布或变更 Workflow 时必须通过发布前检查。
- Run Record 记录真实执行环境、参数摘要、预检、审批、执行结果和验证结果，用于判断工作流是否可靠。
- AI Chat 通过 `search_ops_manuals` 做结构化检索，不能只靠语义相似度决定是否执行。
- Runner Studio 可以从真实 Workflow 反向生成 `workflow_reverse_generated` 运维手册候选，候选必须进入审核页，不能自动发布。

### Web 产品面

`web/` 是当前 React 产品前端，不再依赖旧项目目录。主要页面包括：

- AI 对话：单机运维 Chat、工具过程、审批、Agent-to-UI artifact。
- Runner Workflow：Workflow 库、可视化编排、运行、发布前检查、发布审核。
- 运维手册：verified 手册库、候选审核、run record、workflow 只读预览。
- Prompt Trace：查看本地模型输入 trace、prompt 分层、messages、tools、diff 和 raw。
- LLM 配置、主机与租约、MCP 服务、Coroot 观测、Agent UI、Incident 工作台、OpsGraph 等运维辅助页面。

### OpsGraph 手工构建

OpsGraph 是用户手工维护的最小运维上下文图谱。新环境默认没有外部数据源，也不会从 Coroot、Kubernetes、CMDB 或主机上报自动生成关系。

从 0 创建一张可用图谱：

1. 打开 OpsGraph，进入默认图谱。
2. 把“服务”拖到画布，命名为 `order-api`。
3. 把“中间件集群”拖到画布，命名为 `order-postgres`，子类型选择 `postgres`。
4. 把“主机”或“K8s”拖到画布，作为部署位置。
5. 把中间件实例拖入主机或 K8s 容器，形成部署关系。
6. 从 `order-api` 拖线到 `order-postgres`，关系选择“依赖”。
7. 保存后，Case 和 AI Chat 的 OpsGraph 只读工具会读取这份用户图谱。

### 调试、观测与 Eval

- `modeltrace` 负责本地模型输入 trace 和 prompt fingerprint。
- `observability` 可接 OpenTelemetry / Phoenix，本地看 agent turn、model call、tool call。
- `cmd/agent-eval` 跑 mock/server eval case；`cmd/prompt-diagnose` 对失败 report 做归因。
- `scripts/prompt-regression.sh` 串起 eval、baseline 对比和诊断输出。

## 架构概览

```text
cmd/ai-server/main.go          # 启动入口，组装 ai-server、web/dist、API 和 runtime
cmd/host-agent/                # host agent 入口
cmd/agent-eval/                # Agent eval runner
cmd/prompt-diagnose/           # Prompt / tool / policy 诊断工具
web/                           # React 前端产品面
pkg/runner/                    # Runner Workflow 独立 module
internal/
├── runtimekernel/             # 唯一 turn 运行时内核
├── tooling/                   # Tool 抽象、registry、assembly 真源
├── promptcompiler/            # 四层结构化 prompt 编译
├── policyengine/              # 显式策略硬约束
├── permissions/               # tool 执行权限治理
├── appui/                     # Web/API view-model 与业务服务层
├── server/                    # HTTP/WebSocket/gRPC transport 层
├── opsmanual/                 # 运维手册、检索、候选、workflow 反向生成、run record
├── runnerembed/               # ai-server 内嵌/桥接 Runner Studio
├── modelrouter/               # LLM provider 路由与 fallback
├── modeltrace/                # 本地模型输入 trace
├── observability/             # OpenTelemetry / Phoenix trace
├── eval/                      # eval case / runner / scorer
├── mcp/                       # MCP server registry
├── skills/                    # Skill catalog
├── agents/                    # Agent definition registry
├── hooks/                     # tool / turn lifecycle hooks
├── settings/                  # settings precedence / governance
└── store/                     # 内存、JSON、PostgreSQL 持久化
```

## 开发硬规则

项目级开发硬规则已迁移到 [AGENTS.md](AGENTS.md)。README 只保留产品说明、运行方式、调试入口和扩展文档索引；修改代码前必须先阅读 `AGENTS.md`。

## 典型场景需求：Redis 内存异常排查到手册沉淀

### 用户需求

SRE 在 AI Chat 中输入：

```text
Redis used_memory_rss 持续升高，业务 p95 也升高。请先只读排查，确认风险后再决定是否进入修复流程。
```

系统需要完成的闭环：

1. Chat 抽取 Operation Frame：目标对象为 `redis`，操作为 `rca_or_repair`，执行面可能是 `ssh` / `docker` / `kubernetes`。
2. AI 优先调用只读证据工具，例如 Coroot 指标、容器状态、Redis INFO、慢查询、事件记录。
3. 调用 `search_ops_manuals` 检索 verified 运维手册。
4. 如果目标实例、执行入口或关键指标不足，返回 `need_info`，但不自动伪造固定底部表单；继续通过受控参数解析或普通 Chat 补齐。
5. 如果命中 Redis 手册且 required inputs 完整，先运行只读 preflight；preflight 通过后必须等待用户确认或人工审批。
6. 预检、审批、真实执行和验证结果必须写入 Run Record。
7. 如果本次处理形成稳定闭环，但还没有手册，可以从成功闭环沉淀候选；如果已有 Runner Workflow，可以从 Workflow 反向生成运维手册候选。
8. 候选进入运维手册审核页，审核人看到系统理解、缺口检查、workflow digest、风险策略、发布影响；审核通过后变为 verified，并参与后续 `search_ops_manuals` 检索。

### 验收标准

- 页面不能显示“已执行”除非后端 runtime 确实完成对应 tool / workflow。
- Chat 中不能出现不匹配对象的可执行手册，例如 MySQL 备份不能暴露 PostgreSQL Workflow 的执行入口。
- 高风险修复不能跳过 preflight、审批和验证；发布或变更 Workflow 时不能跳过发布前计划检查。
- 生成的手册正文不能包含 secret ref、token、Authorization header 或原始 shell 脚本全文。
- 发布后的手册在同对象同操作下能被检索命中；跨对象场景只能 reference 或 no_match。

## 运行开关与本地存储

运行期行为通过“设置 -> 运行时配置”页面或 `/api/v1/runtime-settings` 写入数据库，保存后下次请求生效。包括：

- Ops Manual 自动检索。
- Runner Workflow 引用保护模式。
- IntentFrame 路由、只读工具重试、Workflow 校验、Debug trace 和公网搜索开关。

启动级存储配置仍使用环境变量：

- `AIOPS_STORE_DRIVER=postgres` + `AIOPS_POSTGRES_DSN`：使用 PostgreSQL/Gorm 持久化；默认可使用本地内存/JSON 存储。

本地 PostgreSQL 示例：

```bash
docker run -d --name aiops-postgres \
  -e POSTGRES_USER=aiops \
  -e POSTGRES_PASSWORD=aiops \
  -e POSTGRES_DB=aiops \
  -p 127.0.0.1:55432:5432 \
  pgvector/pgvector:pg16

export AIOPS_STORE_DRIVER=postgres
export AIOPS_POSTGRES_DSN='postgres://aiops:aiops@127.0.0.1:55432/aiops?sslmode=disable'
./scripts/start.sh
```

## 快速开始

推荐启动方式：

```bash
cd aiops-v2
./scripts/start.sh              # 构建 web/dist + ai-server，并由 ai-server 托管前端
```

手动执行：

```bash
go test ./...                   # 根模块测试
go build ./cmd/ai-server        # 编译 ai-server
AIOPS_DATA_DIR=.data ./ai-server # 启动，需配置 LLM Provider
```

Runner 子模块测试：

```bash
cd pkg/runner
go test ./...
```

常用启动覆盖：

```bash
AIOPS_HTTP_ADDR=:19080 ./scripts/start.sh
SKIP_WEB_BUILD=1 SKIP_GO_BUILD=1 ./scripts/start.sh
./scripts/start.sh --dry-run
```

部署和统一 env 文件说明见 `docs/deploy-aiops-v2-unified-env.zh.md`。

---

## 本地 Agent 调试与 Eval 闭环

当前本地调试链路固定为：Phoenix 看时间线，`/debug/prompts` 或 `.data/model-input-traces` 看完整模型输入，`cmd/agent-eval` 做回归。不要再为同一目标新增第二套 trace 存储、第二套 eval runner 或第二套 prompt 调试格式。

### 1. Prompt Trace（本地）

完整 prompt 只写入本地 model input trace 文件。默认路径为当前 `AIOPS_DATA_DIR` 下的 `model-input-traces`。可在“设置 -> 运行时配置 -> Debug”开启或关闭 trace。

```bash
AIOPS_HTTP_ADDR=:18080 ./scripts/start.sh
```

调试入口：

- `/debug/prompts` 查看请求详情
- `.data/model-input-traces` 查看完整模型输入和相邻 diff

### 2. 本地模型输入 Trace

开启 Debug / Model Input Trace 后，每次模型调用会在 `.data/model-input-traces/<session>/<turn>/` 下生成 iteration trace。用于排查 prompt 优化时，先从 Phoenix 的 `trace.file` 或 `/debug/prompts` 定位到本轮输入，再看：

- 当前 iteration 的完整模型输入
- `promptFingerprint` / `prompt.stable_hash`
- 非首轮或 resume 后生成的 `input.diff.md`
- visible tools、developer rules、runtime policy 是否符合预期

Phoenix 负责定位“哪一轮慢、哪一步错、哪个 tool 失败”；本地 trace 文件负责看“模型到底看到了什么 prompt 和 tool surface”。

本地排查 AI Chat 模型慢时可以使用 `./scripts/start-ai-chat-trace-dev.sh`。脚本会启动 ai-server，并通过 Runtime Settings API 打开 Debug trace。启动后每次真实 LLM 调用都应在 `.data/model-input-traces/<session>/<turn>/` 下生成 trace；如果 Chat 页面变慢但 Prompt Trace 没有最新文件，优先检查运行时配置页的 Debug 开关和 `AIOPS_DATA_DIR`。

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
AIOPS_LAB_LLM_BASE_URL=http://127.0.0.1:8317/v1 \
AIOPS_LAB_LLM_API_KEY=... \
AIOPS_LAB_LLM_MODEL=gpt-5.4 \
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

1. 跑一次真实 turn，打开 `/debug/prompts` 或 `.data/model-input-traces`。
2. 检查 prompt、developer rules、tool surface、runtime policy。
3. 如果不是首轮，打开相邻输入 diff，确认 resume/approval 后上下文变化。
4. 把失败固化成 eval case，再修改 prompt/tool/policy/model。
5. 先跑 `-agent mock -priority P0`，再跑 `-agent server -priority P0`。
6. 跑 `cmd/prompt-diagnose` 或 `scripts/prompt-regression.sh`，确认目标 case 变好、其他 P0 没退化，并看诊断建议该改 prompt/tool/context/policy/completion_gate 哪一层。

提交前建议：

```bash
./scripts/check-agent-tuning.sh HEAD
./scripts/check-agent-tuning.sh --run HEAD
go test ./internal/eval ./cmd/agent-eval ./internal/promptdiag ./cmd/prompt-diagnose ./internal/observability ./internal/runtimekernel -count=1
```

---

## 📚 扩展文档

- [注册规则升级设计方案](docs/registration-upgrade-design.md) — 基于 claude code 源码分析的注册机制增强计划
- [Skills / MCP 可插拔集成改造设计方案](docs/2026-05-21-aiops-v2-skills-mcp-pluggable-integration-design.zh.md) — 外部系统、OpsManual、Runner、Agent-to-UI 和 profile 的插件化封装边界
- [Agent Trace / Eval / Prompt 优化主指南](docs/agent-trace-eval-guide.zh.md) — 本地 trace、Prompt Trace、eval、诊断和 prompt 优化闭环
