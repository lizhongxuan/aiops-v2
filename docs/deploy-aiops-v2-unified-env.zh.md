# aiops-v2 统一 env 部署说明

本文档说明如何用一个统一 env 文件部署和运行 `aiops-v2`，并接入 Coroot、选择数据存储、完成基础验证。

适用范围：

- 本机或单机部署：通过 `./scripts/start.sh` 启动。
- 团队或生产部署：使用 PostgreSQL 作为主存储，配置由 ConfigMap/Secret/PVC 管理。
- Coroot 接入：启动后在 `Coroot 观测` 页面配置 Base URL、Project ID 和 API Key；配置会持久化到数据存储。

## 1. 部署结构

推荐部署形态：

```text
aiops-v2
├── scripts/start.sh             # 统一启动入口
├── web/dist                     # 前端构建产物，由 ai-server 托管
├── .data/aiops.env              # 统一本地 env 文件，不提交 git
├── .data/bin/ai-server          # 本地构建出的后端二进制
└── .data                        # 本地运行数据、trace、secrets
```

`scripts/start.sh` 会默认加载 `.data/aiops.env`。如果设置了 `AIOPS_ENV_FILE`，则只加载指定文件。显式环境变量优先级最高，env 文件不会覆盖已经存在的环境变量。

旧的 `.env.local` 和 `.data/coroot.env` 不再作为默认启动入口使用。

## 2. 准备依赖

本机部署需要：

- Go，版本以仓库 `go.mod` 为准。
- Node.js 和 npm，用于构建 `web/dist`。
- 可选：PostgreSQL 或 MySQL。
- 可选：`jq`，用于验证接口返回。

进入项目目录：

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
```

## 3. 创建统一 env 文件

创建 `.data/aiops.env`：

```bash
mkdir -p .data
chmod 700 .data
touch .data/aiops.env
chmod 600 .data/aiops.env
```

本机开发或单机演示推荐先使用 JSON 文件存储：

```bash
cat > .data/aiops.env <<'EOF'
# listen
AIOPS_HTTP_ADDR=:18080
AIOPS_GRPC_AUTO_PORT=1

# local paths
AIOPS_DATA_DIR=.data
AIOPS_WEB_DIST_DIR=web/dist

# store: json is simplest for local/single-node use
AIOPS_STORE_DRIVER=json

# LLM
AIOPS_LLM_PROVIDER=openai
AIOPS_LLM_MODEL=gpt-5.4
AIOPS_LLM_COMPACT_MODEL=gpt-5.4-mini
AIOPS_LLM_API_KEY=<replace-with-llm-api-key>
# AIOPS_LLM_BASE_URL=https://api.openai.com/v1
EOF
chmod 600 .data/aiops.env
```

注意：

- 不要把 `.data/aiops.env` 提交到 git。
- LLM API Key、数据库密码都只放在本地 env 文件或 Secret 中。Coroot API Key 在启动后通过 `Coroot 观测` 页面保存。
- 如果 `AIOPS_GRPC_ADDR` 不写死，`AIOPS_GRPC_AUTO_PORT=1` 可以在默认 `:18090` 被占用时自动选择 `:18190+`。

## 4. 数据存储选择

`aiops-v2` 当前支持三种主存储：

| 场景 | 配置 | 说明 |
| --- | --- | --- |
| 本机调试、单机演示 | `AIOPS_STORE_DRIVER=json` | 数据写入 `AIOPS_DATA_DIR`，最简单。 |
| 团队或生产 | `AIOPS_STORE_DRIVER=postgres` | 推荐生产使用，连接串来自 `AIOPS_POSTGRES_DSN` 或 `DATABASE_URL`。 |
| 已有 MySQL 标准 | `AIOPS_STORE_DRIVER=mysql` | 连接串来自 `AIOPS_MYSQL_DSN`。 |

PostgreSQL 示例：

```bash
docker run -d --name aiops-postgres \
  -e POSTGRES_USER=aiops \
  -e POSTGRES_PASSWORD=aiops \
  -e POSTGRES_DB=aiops \
  -p 127.0.0.1:55432:5432 \
  postgres:16
```

`.data/aiops.env` 中改为：

```bash
AIOPS_STORE_DRIVER=postgres
AIOPS_POSTGRES_DSN=postgres://aiops:aiops@127.0.0.1:55432/aiops?sslmode=disable
```

## 5. 启动

首次启动会构建前端和后端：

```bash
./scripts/start.sh
```

只看配置和将要执行的动作：

```bash
./scripts/start.sh --dry-run
```

如果前端已经构建过，只重建后端并启动：

```bash
SKIP_WEB_BUILD=1 ./scripts/start.sh
```

如果前后端都已经构建过：

```bash
SKIP_WEB_BUILD=1 SKIP_GO_BUILD=1 ./scripts/start.sh
```

访问地址：

```text
http://127.0.0.1:18080
```

## 6. Coroot 接入验证

确认 aiops-v2 读取到了 Coroot 配置：

```bash
curl --noproxy '*' -sS http://127.0.0.1:18080/api/v1/coroot/config | jq
```

测试 Coroot 连接：

```bash
curl --noproxy '*' -sS \
  -X POST http://127.0.0.1:18080/api/v1/coroot/test-connection | jq
```

期望结果：

- `ok` 为 `true`。
- `project` 等于 `Coroot 观测` 页面保存的 Project ID。
- `applicationCount` 大于 0。

确认 MCP 工具注册：

```bash
curl --noproxy '*' -sS http://127.0.0.1:18080/api/v1/mcp/servers | jq
```

期望 Coroot server 处于 connected 状态，并包含 Coroot 工具。

## 7. UI 功能验证

打开：

```text
http://127.0.0.1:18080/coroot
```

验证项：

1. Coroot 页面能显示已配置的 base URL 和 project。
2. 点击连接测试后返回成功。
3. 进入聊天或 agent-to-ui 流程，询问某个 Coroot 服务的 CPU 和内存情况，例如：

   ```text
   查看 Coroot 中服务 <service-id> 的 CPU 和内存趋势，并给出根因分析建议。
   ```

4. UI 中应出现 Coroot 服务 CPU/内存时间序列图表。
5. 如果 Coroot 自身 RCA 接口返回 AI disabled，agent 仍应基于 Coroot 指标、拓扑和事件证据给出分析，并明确说明 Coroot RCA 摘要不可用。

## 8. 生产部署建议

生产部署推荐：

- `AIOPS_STORE_DRIVER=postgres`。
- `AIOPS_DATA_DIR` 挂载持久化卷，用于 runner state、trace、secrets 等文件。
- env 配置拆成 ConfigMap 和 Secret。
- `replicas: 1`，除非后续补齐共享会话和运行态协调。

Kubernetes 配置映射建议：

ConfigMap 放非敏感项：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: aiops-v2-config
data:
  AIOPS_HTTP_ADDR: ":8080"
  AIOPS_GRPC_ADDR: ":18090"
  AIOPS_DATA_DIR: "/var/lib/aiops"
  AIOPS_WEB_DIST_DIR: "/app/web/dist"
  AIOPS_STORE_DRIVER: "postgres"
  AIOPS_LLM_PROVIDER: "openai"
  AIOPS_LLM_MODEL: "gpt-5.4"
  AIOPS_LLM_COMPACT_MODEL: "gpt-5.4-mini"
```

Secret 放敏感项：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aiops-v2-secret
type: Opaque
stringData:
  AIOPS_POSTGRES_DSN: "postgres://aiops:<password>@postgres.aiops.svc.cluster.local:5432/aiops?sslmode=disable"
  AIOPS_LLM_API_KEY: "<replace-with-llm-api-key>"
```

Deployment 中引用：

```yaml
envFrom:
  - configMapRef:
      name: aiops-v2-config
  - secretRef:
      name: aiops-v2-secret
```

持久化卷挂载：

```yaml
volumeMounts:
  - name: aiops-data
    mountPath: /var/lib/aiops
```

已有 Kubernetes 示例见：

- `deploy/k8s/aiops-v2.yaml`
- `deploy/k8s/aiops-v2-ingress.example.yaml`

## 9. 常见问题

### gRPC 端口被占用

现象：

```text
grpc listen: listen tcp :18090: bind: address already in use
```

处理：

- 本机部署优先不要在 `.data/aiops.env` 写死 `AIOPS_GRPC_ADDR`。
- 保留 `AIOPS_GRPC_AUTO_PORT=1`，让脚本自动选择可用端口。
- 或者手动停止旧进程：

  ```bash
  lsof -nP -iTCP:18090 -sTCP:LISTEN
  ```

### HTTP 端口被占用

处理：

```bash
lsof -nP -iTCP:18080 -sTCP:LISTEN
```

如果不能停止旧服务，可以临时覆盖：

```bash
AIOPS_HTTP_ADDR=:19080 ./scripts/start.sh
```

### Coroot 连接失败

检查：

```bash
curl --noproxy '*' -sS http://172.18.13.11:8000/
curl --noproxy '*' -sS -X POST http://127.0.0.1:18080/api/v1/coroot/test-connection | jq
```

常见原因：

- `Coroot 观测` 页面保存的 Base URL 不能从 aiops-v2 所在机器访问。
- `Coroot 观测` 页面保存的 Project ID 写成了展示名而不是 URL 中的 project id。
- Token 无效或没有访问项目权限。

### 启动时提示 PostgreSQL 不可达

如果本机只是调试，先改回：

```bash
AIOPS_STORE_DRIVER=json
```

如果是生产部署，确认 DSN、网络、Service、Secret 注入都正确。

## 10. 升级和回滚

升级前：

```bash
cp .data/aiops.env .data/aiops.env.bak.$(date +%Y%m%d%H%M%S)
```

JSON 存储部署需要同时备份 `.data`。PostgreSQL 部署需要按数据库标准流程做备份。

回滚时：

1. 停止当前 aiops-v2。
2. 恢复旧镜像或旧二进制。
3. 恢复 `.data/aiops.env` 或 Kubernetes ConfigMap/Secret。
4. 启动后重新执行 Coroot 连接测试和 UI 图表验证。
