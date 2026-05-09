# 本地 Coroot MCP 接入 aiops-v2 设计

日期：2026-05-10

## 背景

目标是在当前工作站完成三个闭环：

1. `coroot/` 部署起来，但 Coroot 主进程不使用 Docker。
2. `coroot/coroot-mcp` 构建并可通过 stdio MCP 协议工作。
3. `aiops-v2` 接入 Coroot 能力，让运行时可见 `coroot.*` 观测工具。

当前环境约束：

- 根目录 `/Users/lizhongxuan/Desktop/aiops` 不是 git 仓库，`coroot` 与 `aiops-v2` 是独立项目。
- Docker 可用，当前没有运行容器。
- `kubectl` 没有 current context，因此本轮不做 Kubernetes 部署。
- `coroot` 已有 `coroot-local` 二进制、`start-coroot.sh` 和 `coroot-mcp` Go 项目。
- `aiops-v2` 已有 `internal/integrations/coroot`，可在配置 Coroot endpoint 后向 MCP registry 注册 7 个只读 `coroot.*` 工具。

## 推荐方案

采用“本地 Coroot 二进制 + Docker 依赖 + aiops-v2 内建 Coroot 工具接入”。

Coroot 主进程直接在本机启动，依赖中间件使用 Docker：

- Prometheus：`127.0.0.1:19190 -> 9090`
- ClickHouse native：`127.0.0.1:19000 -> 9000`
- Coroot HTTP：`127.0.0.1:18180`

Coroot MCP 作为独立 stdio MCP server 构建和 smoke test。aiops-v2 接入优先使用现有内建 Coroot integration，而不是新增第二套 MCP client runtime。

## 架构

```text
Docker Prometheus 127.0.0.1:19190
Docker ClickHouse 127.0.0.1:19000
        |
        v
local coroot-local :18180
        |
        +--> coroot-mcp-server stdio smoke test
        |
        +--> aiops-v2 AIOPS_COROOT_BASE_URL=http://127.0.0.1:18180
                 |
                 v
             internal/integrations/coroot
                 |
                 v
             MCP registry dynamic tools: coroot.*
```

## 组件

### Coroot 依赖

Prometheus 与 ClickHouse 使用固定容器名，便于重复启动：

- `aiops-coroot-prometheus`
- `aiops-coroot-clickhouse`

启动命令必须是幂等的：如果容器存在则 `docker start`，否则 `docker run` 创建。Prometheus 需要启用 remote write receiver，ClickHouse 使用 native 端口。

### Coroot 主进程

`coroot/coroot-local` 直接启动，参数为：

- `--listen=:18180`
- `--bootstrap-prometheus-url=http://127.0.0.1:19190`
- `--bootstrap-prometheus-remote-write-url=http://127.0.0.1:19190/api/v1/write`
- `--bootstrap-clickhouse-address=127.0.0.1:19000`
- `--auth-anonymous-role=Admin`

日志写入 `coroot/coroot.log`，PID 写入 `coroot/coroot.pid`。现有 `start-coroot.sh` 可作为基础，但需要避免交互式确认阻塞自动部署。

### Coroot MCP

`coroot/coroot-mcp` 构建成本机二进制：

```bash
go build -o coroot-mcp-server .
```

运行时通过环境变量连接 Coroot：

- `COROOT_URL=http://127.0.0.1:18180`
- 可选 `COROOT_TIMEOUT=30s`

`coroot-mcp/run.sh` 当前硬编码到 `/Users/lizhongxuan/Desktop/coroot/...`，需要修正为当前 workspace 下的绝对路径，或改成基于脚本目录解析二进制。

MCP smoke test 使用 JSON-RPC：

- `initialize`
- `notifications/initialized`
- `tools/list`

验收是 `tools/list` 返回 Coroot MCP 工具列表，且进程无配置错误。

### aiops-v2 接入

aiops-v2 使用现有 `internal/integrations/coroot.RegisterBuiltins`，通过环境变量接入：

- `AIOPS_COROOT_BASE_URL=http://127.0.0.1:18180`
- `AIOPS_COROOT_PROJECT=default`
- `AIOPS_HTTP_ADDR=:18080`

如果启动后 MCP registry 未出现 `coroot` server，需要修复 `cmd/ai-server/main.go` 的 endpoint 读取方式，让 `AIOPS_COROOT_BASE_URL` 与现有文档、`ClientConfigFromEnv` 保持一致。修复应最小化，只补配置兼容，不新增另一条 Coroot 工具注册路径。

## 数据流

1. Prometheus 和 ClickHouse 容器启动并通过健康检查。
2. `coroot-local` 读取 bootstrap 参数，连接 Prometheus 和 ClickHouse。
3. Coroot UI/API 在 `http://127.0.0.1:18180` 可访问。
4. `coroot-mcp-server` 通过 `COROOT_URL` 调 Coroot HTTP API，并对 MCP client 暴露工具。
5. `aiops-v2` 启动时读取 Coroot endpoint，注册 MCP server `coroot` 和只读工具：
   - `coroot.list_services`
   - `coroot.service_metrics`
   - `coroot.rca_report`
   - `coroot.service_topology`
   - `coroot.alert_rules`
   - `coroot.incident_timeline`
   - `coroot.slo_status`
6. Runtime tool assembly 将这些动态 MCP tools 暴露给 inspect/plan/execute 模式。

## 错误处理

- Docker 未启动：部署命令直接失败并报告 `docker` 错误，不尝试静默降级。
- Prometheus 或 ClickHouse 健康检查失败：停止进入 Coroot 启动步骤，输出最近容器状态。
- Coroot 端口被占用：报告占用 PID，避免自动杀掉未知进程。
- Coroot health 未通过：保留日志和 PID 状态，输出 `tail -50 coroot.log` 的关键错误。
- Coroot MCP 配置缺失：`COROOT_URL` 必须存在；smoke test 捕获 stderr。
- aiops-v2 未注册 Coroot：先检查环境变量和 `/api/v1/mcp/servers`，再决定是否补配置兼容代码。

## 测试与验收

最低验收命令：

```bash
curl -fsS http://127.0.0.1:18180/health
```

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp
go test ./...
COROOT_URL=http://127.0.0.1:18180 ./coroot-mcp-server
```

MCP smoke test 应验证 `tools/list` 非空。

aiops-v2 验收：

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test ./internal/integrations/coroot ./internal/mcp ./cmd/ai-server
AIOPS_HTTP_ADDR=:18080 AIOPS_COROOT_BASE_URL=http://127.0.0.1:18180 ./scripts/start.sh
curl -fsS http://127.0.0.1:18080/api/v1/mcp/servers
```

`/api/v1/mcp/servers` 应显示 `coroot`，状态为 connected，工具数量为 7。

## 范围外

- 不把 Coroot 主服务放入 Docker。
- 不配置 Kubernetes。
- 不引入新的 MCP transport 或 HTTP/SSE wrapper。
- 不重构 aiops-v2 runtime、tool dispatcher、MCP registry 或前端 MCP UI。
- 不写入 LLM API key 或 Coroot token 到源码、文档 fixture 或 git。

## 自检

- 无未完成项。
- 方案与用户约束一致：Coroot 主进程本地运行，依赖中间件可使用 Docker。
- aiops-v2 接入沿用现有 Coroot integration，只在必要时补环境变量兼容。
- 验收标准覆盖 Coroot health、MCP tools/list、aiops-v2 MCP registry 可见性。
