<p align="center">
  <img src="docs/logo.svg" alt="AIOps Codex" width="80" />
</p>

<h1 align="center">AIOps Codex</h1>

<p align="center">
  AI-native 智能运维平台 — 将 LLM 能力深度集成到服务器运维全链路
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#核心特性">核心特性</a> ·
  <a href="#架构概览">架构</a> ·
  <a href="#规则模块-rule-module">规则模块</a> ·
  <a href="#部署指南">部署</a> ·
  <a href="#开发指南">开发</a>
</p>

---

## 简介

AIOps Codex 是一个面向生产环境的 AI 运维平台，通过 Chat / Workspace / Runner 三种交互模式，让运维工程师以自然语言驱动服务器巡检、故障诊断、变更执行和监控分析。

平台默认通过 **Bifrost LLM Gateway** 连接模型提供方，支持 **OpenAI、Anthropic、Ollama**，再通过 **Host Agent** 安全接入目标主机，所有变更操作经过统一审批审计，确保 AI 辅助运维的可控性和可追溯性。

## 核心特性

### 🤖 AI 驱动的运维交互

- **Chat 模式** — 自然语言对话，AI 自动选择工具执行运维操作
- **Workspace 协作** — 多主机编排，支持 Planner → Worker 任务分解
- **Runner 脚本** — 可复用的自动化脚本，带参数 schema 和 dry-run 预览

### 🔐 治理与审批

- **统一审批流** — 所有变更命令经审批后执行，支持手动审批和自动放行
- **审核流水** — 结构化审计日志，支持按时间/主机/操作人/决策多维筛选
- **主机级授权白名单** — 已授权命令自动通过，支持撤销、禁用、TTL 过期
- **高风险命令拦截** — `rm -rf /`、`sudo su` 等危险命令强制要求 TTL

### 🛡️ 三层能力网关

| 层级 | 说明 | 审批策略 |
|------|------|----------|
| Structured Read | 14 个标准化只读接口 (`host.summary`, `host.process.top` 等) | 免审批 |
| Controlled Mutation | 5 个受控变更接口 (`service.restart`, `config.apply` 等) | 强制审批 |
| Raw Shell | 原始命令执行兜底 | 按策略审批 |

### 📊 Coroot 监控集成

- **服务健康总览** — 嵌入 Coroot 服务视图，实时展示健康/告警/异常状态
- **7 个 MCP 动态工具** — `coroot.list_services`、`coroot.service_metrics`、`coroot.rca_report` 等
- **监控内嵌 AI** — 在监控页面直接调用 AI 分析，支持面板解释、异常归因、修复建议
- **卡片映射** — Coroot 查询结果自动映射为结构化 UI 卡片

### 🧩 能力中心

- **Skills 管理** — 内置 + 自定义技能，支持分类、版本、启用/禁用
- **MCP Servers** — 统一管理 MCP 工具服务，支持探活和权限控制
- **能力绑定** — Skill/MCP 与 Agent Profile、Workspace Preset、UI Card 的绑定关系

### 🎴 UI 卡片系统

- **9 种内置卡片** — 摘要卡、KPI 条、时序图、状态表、控制面板、操作表单、监控聚合、修复聚合
- **卡片管理后台** — 元数据编辑、触发调试器、实时预览
- **Bundle 机制** — Monitor Bundle 和 Remediation Bundle 聚合多卡片

### ⚗️ 沙盒演练

- **Lab 环境** — 创建隔离的沙盒环境，模拟多节点拓扑
- **场景模板** — 预置双层架构、三层微服务、缓存层等拓扑模板
- **故障注入** — 对 mock 节点注入故障，验证告警响应和修复流程
- **v1 Mock 模式** — 前端可正常渲染 bundle 和 control panel，命令返回模拟结果

### 🏭 Generator Workshop

- **自动生成** — 从 MCP Tool → Skill 草稿、Script Config → UI Card 草稿、Coroot → Bundle Preset 草稿
- **4 步流程** — Generate → Lint → Preview → Publish Draft
- **草稿隔离** — 所有生成物默认 draft 状态，不自动上线

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                        Web Dashboard                         │
│   Vue 3 + Pinia + Vue Router + Lucide Icons                  │
│   Chat │ Workspace │ Terminal │ 监控 │ 管理后台               │
└────────────────────────┬────────────────────────────────────┘
                         │ HTTP / WebSocket
┌────────────────────────▼────────────────────────────────────┐
│                     AI Server (Go)                           │
│                                                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐  │
│  │ Session  │ │ Approval │ │ Dynamic  │ │  Orchestrator │  │
│  │ Manager  │ │ & Audit  │ │  Tools   │ │  (Workspace)  │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐  │
│  │  Agent   │ │  Coroot  │ │ UI Card  │ │   Generator   │  │
│  │ Profile  │ │  Client  │ │  Store   │ │   Service     │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────────┘  │
│                         │                                    │
│              Bifrost provider routing                        │
│                         ▼                                    │
│               Bifrost LLM Gateway (Go)                        │
│                         │                                    │
│        OpenAI / Anthropic / Ollama / OpenAI-compatible       │
├──────────────────────────────────────────────────────────────┤
│  HTTP :8080    gRPC :18090    Coroot Proxy :8080/coroot      │
└────────────┬─────────────────────────┬───────────────────────┘
             │ gRPC 双向流              │ HTTP Reverse Proxy
┌────────────▼──────────┐    ┌─────────▼──────────────────────┐
│    Host Agent (Go)    │    │     Coroot Server (外部)        │
│  每台目标主机一个实例   │    │   服务监控 / 拓扑 / RCA         │
│  终端 / 执行 / 文件    │    └────────────────────────────────┘
└───────────────────────┘
```

### 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | Vue 3, Pinia, Vue Router, Lucide Icons, Monaco Editor, xterm.js |
| 后端 | Go 1.26, gRPC, net/http |
| AI 引擎 | Bifrost LLM Gateway, 支持 OpenAI / Anthropic / Ollama |
| 监控集成 | Coroot (HTTP Client + Reverse Proxy) |
| 持久化 | JSON 文件 (内存 + 异步写盘, sync.RWMutex) |
| 测试 | Vitest, Playwright, Go testing |
| 部署 | Docker, Docker Compose, systemd |

## 项目结构

```
aiops-codex/
├── cmd/
│   ├── ai-server/          # AI Server 主入口
│   └── host-agent/         # Host Agent 主入口
├── internal/
│   ├── bifrost/            # 多 provider LLM Gateway
│   ├── agentloop/          # Bifrost ReAct loop / context / workspace runtime
│   ├── server/             # HTTP/gRPC 服务、API 路由、审批流程
│   ├── store/              # 内存存储 + JSON 持久化
│   ├── model/              # 数据模型定义
│   ├── config/             # 配置管理
│   ├── coroot/             # Coroot HTTP 客户端
│   ├── generator/          # Skill/Card/Bundle 自动生成服务
│   ├── orchestrator/       # Workspace 编排引擎
│   └── agentrpc/           # gRPC 协议定义
├── web/
│   ├── src/
│   │   ├── pages/          # 16 个 Vue 页面
│   │   ├── components/     # 可复用组件 (Coroot Embed, Monitor AI 等)
│   │   ├── lib/            # 工具库 (MCP Bundle Resolver 等)
│   │   └── store.js        # Pinia 全局状态
│   └── tests/
│       ├── *.spec.js       # Vitest 组件测试 (24 个文件, 151 测试)
│       └── e2e/            # Playwright E2E 测试 (6 个文件, 55 测试)
├── deploy/docker/          # Docker 构建和编排
├── proto/                  # Protobuf 定义
├── docs/                   # 架构文档
└── scripts/                # 运维脚本
```

## 快速开始

### 前置条件

- Go 1.26+
- Node.js 22+
- Docker & Docker Compose (部署用)
- LLM provider 凭证或本地模型服务

### 本地开发

```bash
# 克隆仓库
git clone https://github.com/lizhongxuan/aiops-codex.git
cd aiops-codex

# 启动后端
export LLM_PROVIDER=openai
export LLM_API_KEY=sk-xxx
go run ./cmd/ai-server

# 启动前端 (另一个终端)
cd web
npm install
npm run dev

# 访问 http://localhost:5173
```

### Docker 部署

```bash
cd deploy/docker
cp .env.example .env
# 编辑 .env 填入 LLM_PROVIDER / LLM_API_KEY / LLM_BASE_URL 和 HOST_AGENT_BOOTSTRAP_TOKEN

# OpenAI-compatible 服务或 Ollama 可通过 LLM_BASE_URL 指定自定义地址

docker compose build
docker compose up -d

# 访问 http://localhost:18080
```

详细部署文档见 [deploy/docker/README.md](deploy/docker/README.md)。

## 规则模块 (Rule Module)

这一节是 **V2 架构规则入口**。从本次重构开始，后续需求默认必须遵守这套规则，不允许继续向旧真源加逻辑。

设计与实施文档见：

- [docs/aiops-codex-architecture-refactor-20260419.md](docs/aiops-codex-architecture-refactor-20260419.md)
- [docs/aiops-codex-architecture-refactor-todo-20260419.md](docs/aiops-codex-architecture-refactor-todo-20260419.md)
- [docs/aiops-codex-business-logic-baseline-20260419.md](docs/aiops-codex-business-logic-baseline-20260419.md)

### 1. 核心原则

V2 架构只保留五个核心骨架：

- `runtimekernel`
- `capability registry`
- `prompt compiler`
- `policy engine`
- `projection/display model`

后续新增：

- 新 tool
- 新 prompt
- 新 skill
- 新 MCP server
- 新 UI 卡片
- 新工作流

都必须落到这五层中的某一层，不允许横向补胶水。

同时必须遵守一条额外硬规则：

> 新架构可以替换旧实现，但不能无记录地改变 [docs/aiops-codex-business-logic-baseline-20260419.md](docs/aiops-codex-business-logic-baseline-20260419.md) 冻结的当前业务逻辑。

### 2. 核心会话域

核心会话域只有两个：

- `host`
- `workspace`

说明：

- `host`：单主机对话与执行面
- `workspace`：协作工作台对话，协调多个 host-agent

不属于核心架构的域：

- `coroot`
- `lab`
- `generator`
- 其他垂类页面

这些能力只能作为 extension / capability 挂载，不允许反向主导核心架构设计。

### 3. Runtime Kernel 规则

- 全局只能有一个 `runtimekernel` 作为 turn 运行时内核。
- 不允许再出现第二套主 loop。
- `host` 和 `workspace` 共享同一个 loop。
- mode 切换依赖：
  - prompt
  - capability visibility
  - policy engine

从 V2 开始，以下位置不得再承载新的主流程逻辑：

- `internal/agentloop/`
- `internal/server/bifrost_runtime.go`
- `internal/server/orchestrator_integration.go`
- `internal/server/session_runtime.go`

### 3.1 业务逻辑基线规则

V2 不只是“统一架构”，还必须“守住现有产品语义”。

后续任何重构或需求实现，都必须先对照：

- [docs/aiops-codex-business-logic-baseline-20260419.md](docs/aiops-codex-business-logic-baseline-20260419.md)

硬规则：

- 不允许无记录地改变 host / workspace 的用户可见行为
- 不允许无记录地改变 approval / evidence / process / final 的边界
- 不允许把搜索次数、浏览次数、tool 行数等真实业务语义重新做成 UI 猜测
- 不允许因为新架构切换而丢失 stop / offline / reconcile / budget / queue 这些运维语义
- 任何会改变业务语义的改动，必须同步更新基线文档和对应测试

### 4. Capability Registry 规则

采用接近 Claude Code 的细粒度 capability registry。

核心 capability 类型固定为：

- `tool:*`
- `skill:*`
- `mcp_tool:*`
- `ui_surface:*`
- `mode_rule:*`
- `workspace:*`

规则：

- 所有新能力必须注册到 capability registry
- 不允许再创建一套平行的“主能力池”
- `workspace:*` 是唯一额外能力域
- 不使用重型 bundle 作为核心抽象

### 5. Tool 规则

Tool 继续以 `UnifiedTool` 为 canonical contract。

规则：

- 一个 tool 只表达一种明确能力
- tool 必须通过 registry 可见
- tool prompt 只写：
  - capability
  - constraints
  - result shape
  - approval note
- tool 必须显式提供：
  - `CheckPermissions()`
  - `IsReadOnly()`
  - `IsDestructive()`
  - `IsConcurrencySafe()`
- tool 的结构化 UI 输出统一走 `ToolDisplayPayload`

### 6. PromptCompiler 规则

`PromptCompiler` 是唯一 effective prompt 真源。

从 V2 开始：

- 不允许把新规则继续写到：
  - `BuildSystemPrompt()`
  - `renderMainAgentDeveloperInstructions()`
  - 页面或 runtime 内的 ad-hoc prompt prose
  - dynamic schema 的长 description prose
- 不允许再通过局部 helper 直接 append 一段业务 prose

prompt 固定为四层：

1. `System Prompt`
2. `Developer Instructions`
3. `Tool-owned Prompt`
4. `Runtime Policy Prompt`

硬规则：

- tool prompt 不得写 answer style
- runtime gating 只能写在 policy 层
- developer instructions 只写环境和运行约束
- system prompt 不写垂类 heuristics

### 7. Policy Engine 规则

本项目使用 Claude Code 风格的“同一 loop + prompt/mode 切换”，但必须额外保留显式 `policy engine`。

原因：

- 运维执行不能只靠 prompt 自觉
- `readonly / plan / execute / approval / evidence / completion` 都需要硬边界

核心 mode 固定为四个：

- `chat`
- `inspect`
- `plan`
- `execute`

不允许新增第五个核心 mode，除非同步更新 policy engine、prompt compiler 和测试基线。

### 8. Projection / Display 规则

所有运行时结果最终统一投影为：

- `toolInvocations`
- `runtime.activity`
- `cards`
- `approvals`
- `evidence`
- `snapshot`

规则：

- lifecycle event 是活动真源
- projection 只做投影，不做业务推理
- 前端优先消费：
  - `toolInvocations`
  - `runtime.activity`
  - `detail.display`
- 不允许从 assistant 文本解析工具状态

### 9. UI 规则

页面壳层只保留两个：

- `HostConversationPage`
- `WorkspaceConversationPage`

内容 renderer 只保留三个核心层：

- `MessageRenderer`
- `ActivityRenderer`
- `StructuredDisplayRenderer`

规则：

- 统一 renderer/model，不统一 page shell
- 不允许继续在页面里新增大段 tool-name heuristic
- 不允许让 chat 和 workspace 分别维护两套主 activity 语义

### 10. Extension 规则

`coroot / lab / generator` 等非核心能力只能作为 extension 挂载。

规则：

- 不允许它们继续参与核心 runtime / prompt / policy 架构设计
- 它们只能通过 capability registry 和 projection/display 模型接入

### 11. 后续需求必须遵守

后续任何新需求，在动手之前都必须先判断它属于哪一层：

1. `runtimekernel`
2. `capability registry`
3. `prompt compiler`
4. `policy engine`
5. `projection/display`
6. `extension`

如果一个需求无法落到这六层中的某一层，说明需求描述还不够清楚，或者实现方式正在回到旧架构。

一句话总原则：

> 后续需求必须沿 V2 骨架扩展，不允许重新发明第二套 runtime、第二套 prompt 真源、第二套 capability 主链或第二套页面级 heuristic。

## API 概览

### 核心 API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/healthz` | GET | 健康检查 |
| `/api/v1/state` | GET | 全局状态快照 |
| `/api/v1/chat/message` | POST | 发送聊天消息 |
| `/api/v1/approvals/{id}` | POST | 审批决策 |

### 审计与授权

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/approval-audits` | GET | 审核流水列表 (支持筛选/分页) |
| `/api/v1/approval-audits/{id}` | GET | 审核流水详情 |
| `/api/v1/approval-grants` | GET/POST | 授权白名单管理 |
| `/api/v1/approval-grants/{id}/revoke` | POST | 撤销授权 |
| `/api/v1/approval-grants/{id}/disable` | POST | 禁用授权 |
| `/api/v1/approval-grants/{id}/enable` | POST | 启用授权 |

### 资源管理

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/capability-bindings` | CRUD | 能力绑定管理 |
| `/api/v1/ui-cards` | CRUD | UI 卡片定义管理 |
| `/api/v1/ui-cards/{id}/preview` | POST | 卡片预览 |
| `/api/v1/script-configs` | CRUD | 脚本配置管理 |
| `/api/v1/script-configs/{id}/dry-run` | POST | 脚本 Dry-Run |
| `/api/v1/lab-environments` | CRUD | 沙盒环境管理 |
| `/api/v1/lab-environments/{id}/start` | POST | 启动沙盒 |
| `/api/v1/lab-environments/{id}/inject` | POST | 故障注入 |
| `/api/v1/lab-environments/{id}/reset` | POST | 重置沙盒 |

### 监控与生成

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/coroot/*` | GET | Coroot 反向代理 (只读) |
| `/api/v1/generator/generate` | POST | 生成 Skill/Card 草稿 |
| `/api/v1/generator/lint` | POST | 校验草稿 |
| `/api/v1/generator/preview` | POST | 预览草稿 |
| `/api/v1/generator/publish-draft` | POST | 发布草稿 |

## 测试

```bash
# Go 后端测试
go test ./internal/... -count=1

# 前端组件测试 (Vitest)
cd web && npm test

# 前端 E2E 测试 (Playwright)
cd web && npx playwright test
```

### 测试覆盖

| 类别 | 文件数 | 测试数 |
|------|--------|--------|
| Go 单元测试 | 20+ | 80+ |
| Vitest 组件测试 | 24 | 151 |
| Playwright E2E | 6 | 55 |

## Agent Profile 策略

Agent Profile 控制 AI 的权限边界，分为两级：

- **main-agent** — 全局策略，控制所有会话的基线权限
- **host-agent-default** — 远程主机默认策略，与 main-agent 取交集

策略维度：

```yaml
capabilityPermissions:
  commandExecution: enabled | approval_required | disabled
  fileRead: enabled
  fileChange: approval_required
  terminal: enabled

commandPermissions:
  defaultMode: allow | approval_required | readonly_only | deny
  allowSudo: false
  categoryPolicies:
    service_mutation: approval_required
    package_mutation: deny
```

## 安全设计

- 所有变更命令经统一审批流程
- Host Agent 通过 Bootstrap Token + 可选 mTLS 认证
- 结构化接口参数校验，拒绝 shell 注入 (`;`, `&&`, `` ` ``, `$(` 等)
- 高风险命令 (`rm -rf /`, `sudo su`, `iptables -F`) 禁止创建无 TTL 授权
- Coroot 代理仅允许只读路径，GET 方法
- 沙盒环境与生产完全隔离，mock host Kind="lab"
- 所有生成物默认 draft 状态，不自动加载执行

## License

MIT
