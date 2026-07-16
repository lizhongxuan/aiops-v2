# AIOps V2

`aiops-v2` 是面向生产运维场景的 AI 运维工作台。它把 AI Chat、受控工具调用、人工审批、Runner Workflow、运维手册、运行记录和可观测调试能力整合在一个系统中。

项目目标不是让模型“自由操作生产环境”，而是让 Agent 沿着可审计的控制链完成：

```text
理解问题 -> 收集证据 -> 形成计划 -> 权限与风险检查 -> 人工审批
         -> 受控执行 -> 结果验证 -> 沉淀运维经验
```

## 核心功能

- **AI 运维对话**：支持流式运行状态与工具过程、`@主机` 作用域和多轮上下文；最终结论在终态门禁后分区展示。
- **可控 Agent Harness**：统一组装 Prompt、工具面和运行时上下文；由 `runtimekernel` 管理模型调用、工具循环、checkpoint、approval 和 resume。
- **安全工具执行**：工具统一受 Tool Surface、Policy 和 Permission 治理；变更或高风险操作还必须经过 ActionToken 和 Approval，不能只凭模型文本直接执行。
- **Runner Workflow**：支持 YAML/Graph Workflow、预检、执行、验证、运行状态、Run Record 和可视化 Runner Studio。
- **运维知识闭环**：管理运维手册、Workflow、历史 Case、证据和 Run Record，并把成功处理过程沉淀为可审核资产。
- **观测与回归**：提供 Prompt Trace、模型输入 diff、OpenTelemetry/Phoenix、Agent Eval 和 Prompt Diagnose，便于定位 Agent 行为变化和回归。

Web 端主要包含 AI 对话、Runner Workflow、运维手册、主机管理、MCP/Skill、Coroot、Incident、OpsGraph、LLM 配置和 Prompt Trace 等页面。

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
├── promptcompiler/            # 分层结构化 prompt 编译
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

## 部署方式

### 环境要求

- Go 1.24+
- Node.js 22+ 与 npm
- Docker（用于快速启动 PostgreSQL 或构建容器镜像，可选）
- 本地启动脚本默认端口：HTTP `18080`、gRPC `18090`（容器内 HTTP 默认为 `8080`）

### 方式一：本地部署（推荐）

`scripts/start.sh` 是本地统一入口：构建 `web/dist`、host-agent artifact 和 `ai-server`，最后由单个 `ai-server` 进程托管 API 与前端。该脚本默认使用 PostgreSQL，并连接 `127.0.0.1:55432`。

```bash
cd aiops-v2

# 首次安装前端依赖
npm --prefix web ci

# 启动默认 PostgreSQL
docker run -d --name aiops-postgres \
  --restart unless-stopped \
  -e POSTGRES_USER=aiops \
  -e POSTGRES_PASSWORD=aiops \
  -e POSTGRES_DB=aiops \
  -p 127.0.0.1:55432:5432 \
  pgvector/pgvector:pg16

# 构建并启动 aiops-v2
./scripts/start.sh
```

启动完成后：

- Web：<http://127.0.0.1:18080/>
- LLM 配置：<http://127.0.0.1:18080/settings/llm>
- Prompt Trace：<http://127.0.0.1:18080/debug/prompts>

在“设置 -> LLM 配置”中填写 Provider、Base URL、Model 和 API Key 后即可使用 AI Chat，也可以调用 `PUT /api/v1/llm-config` 完成配置。不要把 API Key 写入源码、README 或测试 fixture。

如果只做本地功能体验、不需要 PostgreSQL，可使用 JSON 文件存储：

```bash
AIOPS_STORE_DRIVER=json ./scripts/start.sh
```

数据会写入 `.data/`。生产或多人环境建议使用 PostgreSQL。

### 方式二：Docker 部署

下面的命令使用 JSON 存储快速启动单容器版本，并通过 volume 持久化数据：

```bash
docker build -t aiops-v2:local .
docker volume create aiops-v2-data

docker run --rm --name aiops-v2 \
  -p 18080:8080 \
  -p 18090:18090 \
  -e AIOPS_STORE_DRIVER=json \
  -e AIOPS_SKILLS_DIRS=/app/skills \
  -v aiops-v2-data:/var/lib/aiops \
  -v "$(pwd)/skills:/app/skills:ro" \
  aiops-v2:local
```

当前 Dockerfile 不会自动复制仓库内的 `skills/`，所以上例显式只读挂载该目录；如果不需要内置 Skill，可以去掉对应环境变量和挂载。生产部署时把 `AIOPS_STORE_DRIVER` 改为 `postgres`，并从 Secret 管理系统注入 `AIOPS_POSTGRES_DSN`。

### 方式三：Kubernetes 部署

仓库提供 `deploy/k8s/aiops-v2.yaml` 和 Ingress 示例。部署前需要：

1. 构建并推送镜像，把清单中的 `registry.example.com/aiops-v2:latest` 替换为真实镜像。
2. 在 `aiops` namespace 创建 `aiops-v2-secret`。清单中的 `secretRef` 不是 optional，即使使用 JSON 存储也必须存在该 Secret。
3. 根据集群环境调整 PVC、Ingress、内置 Skill 的挂载方式，以及 `kubectl`/`kubeconfig` hostPath 和 gRPC `hostPort: 8002`。

```bash
kubectl create namespace aiops --dry-run=client -o yaml | kubectl apply -f -

kubectl -n aiops create secret generic aiops-v2-secret \
  --from-literal=AIOPS_STORE_DRIVER=postgres \
  --from-literal=AIOPS_POSTGRES_DSN='postgres://USER:PASSWORD@POSTGRES_HOST:5432/aiops?sslmode=require' \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f deploy/k8s/aiops-v2.yaml
kubectl -n aiops rollout status deployment/aiops-v2
kubectl -n aiops port-forward service/aiops-v2 18080:80
```

当前 K8s 清单面向路径匹配的自管集群：节点必须存在 `/usr/local/bin/kubectl` 和 `/etc/kubernetes/admin.conf`。在托管集群中应删除这两个 hostPath，改用 ServiceAccount/RBAC；如需内置 Skill，还要把 `skills/` 打包进自定义镜像或单独挂载，并设置 `AIOPS_SKILLS_DIRS`。

### 常用配置

`scripts/start.sh` 默认读取 `.data/aiops.env`，但不会覆盖命令行已经设置的环境变量。

| 环境变量 | 本地脚本默认值 | 作用 |
| --- | --- | --- |
| `AIOPS_HTTP_ADDR` | `:18080` | Web 与 HTTP API 监听地址 |
| `AIOPS_GRPC_ADDR` | `:18090` | host-agent gRPC 监听地址 |
| `AIOPS_DATA_DIR` | `.data` | 本地状态、trace 和构建产物目录 |
| `AIOPS_STORE_DRIVER` | `postgres` | 存储后端；可选 `postgres` 或 `json` |
| `AIOPS_POSTGRES_DSN` | 本地 `55432` 默认 DSN | PostgreSQL 连接串 |
| `AIOPS_ENV_FILE` | `.data/aiops.env` | 统一环境变量文件路径 |
