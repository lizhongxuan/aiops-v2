# AIOps V2 Docker 和 Kubernetes 部署

本文档说明如何把 `aiops-v2` 的 React 前端和 Go 后端打进同一个 Linux 镜像，并在 Kubernetes 中部署。

## 镜像内容

单镜像包含：

- 后端二进制：`/usr/local/bin/ai-server`
- 前端静态资源：`/app/web/dist`
- 内置 ERP runbook：`/app/runbooks`
- 内置 opsgraph seed：`/app/data`
- 持久化数据目录：`/var/lib/aiops`

容器启动后只运行一个进程：`ai-server`。后端通过 `AIOPS_WEB_DIST_DIR=/app/web/dist` 直接托管前端页面，所以不需要单独的 Nginx 或 Node 服务。

## 构建镜像

Linux amd64：

```bash
docker buildx build \
  --platform linux/amd64 \
  -t registry.example.com/aiops-v2:0.1.0 \
  .
```

Linux arm64：

```bash
docker buildx build \
  --platform linux/arm64 \
  -t registry.example.com/aiops-v2:0.1.0-arm64 \
  .
```

推送镜像：

```bash
docker push registry.example.com/aiops-v2:0.1.0
```

## 本地运行

```bash
docker run --rm \
  -p 8080:8080 \
  -v aiops-v2-data:/var/lib/aiops \
  -e AIOPS_LLM_API_KEY="$AIOPS_LLM_API_KEY" \
  -e AIOPS_LLM_PROVIDER=openai \
  -e AIOPS_LLM_MODEL=gpt-5.4 \
  registry.example.com/aiops-v2:0.1.0
```

访问：

```text
http://127.0.0.1:8080
```

## 镜像配置

核心配置：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AIOPS_HTTP_ADDR` | `:8080` | HTTP 和前端页面监听地址 |
| `AIOPS_GRPC_ADDR` | `:18090` | gRPC 监听地址 |
| `AIOPS_DATA_DIR` | `/var/lib/aiops` | 会话、LLM 配置、MCP 配置等 JSON 持久化目录 |
| `AIOPS_WEB_DIST_DIR` | `/app/web/dist` | 前端构建产物目录 |

如果在 Kubernetes 中修改 `AIOPS_HTTP_ADDR` 的端口，需要同步修改 Deployment 的 `containerPort`、Service 的 `targetPort` 和 readiness/liveness probe。

LLM 首次引导配置：

| 变量 | 说明 |
| --- | --- |
| `AIOPS_LLM_PROVIDER` | Provider，例如 `openai`、`anthropic` |
| `AIOPS_LLM_MODEL` | 主模型，例如 `gpt-5.4` |
| `AIOPS_LLM_COMPACT_MODEL` | 轻量模型，例如 `gpt-5.4-mini` |
| `AIOPS_LLM_BASE_URL` | OpenAI-compatible 网关地址，可为空 |
| `AIOPS_LLM_API_KEY` | API Key，建议从 Secret 注入 |
| `AIOPS_LLM_CONFIG_FILE` | 可选，挂载完整 `llm-config.json` 文件路径 |
| `AIOPS_BOOTSTRAP_LLM_CONFIG` | `if-missing` 或 `overwrite`，默认只在数据目录没有 `llm-config.json` 时写入 |

注意：后端真实读取的是 `${AIOPS_DATA_DIR}/llm-config.json`。容器 entrypoint 只是为了首次部署方便，会把上述 env 或 `AIOPS_LLM_CONFIG_FILE` 转成这个 JSON 文件。之后如果在 UI 里修改了 LLM 配置，配置会写回 PVC。若要强制用新的 Secret 覆盖旧配置，临时把 `AIOPS_BOOTSTRAP_LLM_CONFIG` 改成 `overwrite` 后重启一次。

常用可选配置：

| 变量 | 说明 |
| --- | --- |
| `AIOPS_ACTION_TOKEN_SECRET` | runbook/action token 签名密钥 |
| `AIOPS_COROOT_BASE_URL` / `COROOT_BASE_URL` | Coroot API 地址 |
| `AIOPS_COROOT_TOKEN` / `COROOT_TOKEN` | Coroot Token |
| `AIOPS_COROOT_PROJECT` / `COROOT_PROJECT` | Coroot project，默认 `default` |
| `AIOPS_COROOT_IFRAME_URL` | Coroot iframe 地址 |
| `AIOPS_OTEL_ENABLED` | 是否启用 OpenTelemetry trace |
| `AIOPS_OTEL_ENDPOINT` | OTLP HTTP trace endpoint |
| `AIOPS_DEBUG_MODEL_INPUT_TRACE` | 是否落盘模型输入 trace |
| `AIOPS_SKILLS_DIRS` | 额外 skills 目录，多个目录用系统 path separator 分隔 |
| `AIOPS_PLUGIN_DIRS` | 额外插件目录，多个目录用系统 path separator 分隔 |

## Kubernetes 部署

1. 修改镜像地址：

   编辑 `deploy/k8s/aiops-v2.yaml`，把：

   ```yaml
   image: registry.example.com/aiops-v2:latest
   ```

   改成你实际推送的镜像，例如：

   ```yaml
   image: registry.example.com/aiops-v2:0.1.0
   ```

2. 修改 Secret：

   编辑 `deploy/k8s/aiops-v2.yaml` 中的 `Secret`，替换：

   ```yaml
   AIOPS_LLM_API_KEY: "replace-with-your-api-key"
   AIOPS_ACTION_TOKEN_SECRET: "replace-with-a-random-long-secret"
   ```

   也可以用命令创建 Secret：

   ```bash
   kubectl create namespace aiops --dry-run=client -o yaml | kubectl apply -f -
   kubectl -n aiops create secret generic aiops-v2-secret \
     --from-literal=AIOPS_LLM_API_KEY="$AIOPS_LLM_API_KEY" \
     --from-literal=AIOPS_ACTION_TOKEN_SECRET="$(openssl rand -hex 32)" \
     --dry-run=client -o yaml | kubectl apply -f -
   ```

3. 部署：

   ```bash
   kubectl apply -f deploy/k8s/aiops-v2.yaml
   ```

4. 查看状态：

   ```bash
   kubectl -n aiops get pods
   kubectl -n aiops logs deploy/aiops-v2
   ```

5. 临时访问：

   ```bash
   kubectl -n aiops port-forward svc/aiops-v2 8080:80
   ```

   然后打开：

   ```text
   http://127.0.0.1:8080
   ```

6. 可选 Ingress：

   编辑 `deploy/k8s/aiops-v2-ingress.example.yaml` 的域名和 `ingressClassName`，然后执行：

   ```bash
   kubectl apply -f deploy/k8s/aiops-v2-ingress.example.yaml
   ```

## 生产注意事项

- 当前服务使用本地 JSON 文件持久化，Kubernetes 建议 `replicas: 1` 并挂载 RWO PVC。
- `AIOPS_DATA_DIR` 需要可写；示例 YAML 使用 `fsGroup: 10001` 让非 root 容器可以写 PVC。
- API Key 不要写入 Git；示例 YAML 中的 Secret 值必须替换。
- 如果接入 Ingress，WebSocket 路径 `/ws` 和 `/api/v1/terminal/ws` 需要支持长连接，示例 Nginx Ingress 已设置较长超时。
