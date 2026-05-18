# aiops-v2 后续四项能力设计方案

日期：2026-05-18（Asia/Shanghai）
状态：设计方案
范围：浏览器插件一键根因分析、GitHub 代码拉取与一键部署、Kubernetes 资源管理、故障告警分析。

## 1. 背景和现状

aiops-v2 当前已经具备 AIOps 主链路的基础能力：`RuntimeKernel`、`ToolDispatcher`、`PolicyEngine`、`ActionToken`、`IncidentCase`、`EvidenceRef`、`OpsGraph`、Coroot 工具、Agent-to-UI artifact、React Chat 结构化传输和 Runner/Workflow 基础设施。已有设计明确要求：

- 所有工具调用必须经过 `ToolDispatcher`，不能绕开统一策略、审批和审计。
- Chat 生产过程 UI 必须进入 `AiopsTransportState`，不能新增页面私有 SSE/WebSocket 或 final Markdown parser。
- Coroot、K8s、变更、执行、审批都只能作为同一 tool surface 下的能力。
- Mock 数据不能作为生产证据；无法接真实数据时必须显式标记 `mock: true`。

当前差距：

- 浏览器插件代码不在当前仓库，aiops-v2 需要先固定后端 bridge、事件协议、Case/Evidence 绑定和 RCA 入口。
- `internal/integrations/k8s` 已有工具名和审批模型，但读写结果仍是 mock，需要接入真实 Kubernetes API。
- `internal/integrations/changes` 已有 recent deployments/config changes 工具，但仍是 mock，需要接真实 GitHub/CI/CD/审计源。
- Coroot webhook 当前只创建 Case 和 raw evidence，没有自动完成告警归并、证据补全和 RCA 分析。

## 2. 目标

1. 浏览器插件打通链路调用链：用户在真实业务页面一键反馈“这个功能慢/报错”，aiops-v2 自动绑定 trace、URL、业务动作、后端服务、Coroot/K8s/变更证据，并生成结构化 RCA 报告。
2. GitHub 代码拉取与一键部署：用户选择服务、仓库、分支/tag/commit 后，系统生成部署计划、构建或触发 CI、生成镜像 digest、执行 Kubernetes 部署、验证 rollout，并保留回滚方案。
3. Kubernetes 资源管理：支持真实集群资源查询、日志/事件/rollout 采集、资源变更 dry-run、审批后执行、回滚和审计。
4. 故障告警分析：接入 Coroot/Alertmanager/webhook 告警，完成去重聚合、Case 关联、证据补全、根因假设排序、影响面分析和下一步建议。
5. 所有关键验收必须使用真实数据：真实浏览器页面、真实 trace/Coroot 或 Prometheus 数据、真实 GitHub 仓库/Actions、真实 Kubernetes API、真实 webhook payload。

## 3. 非目标

- 不把 aiops-v2 变成 coding agent：不提供通用代码文件编辑、补丁生成或任意 repo 文件搜索工具。
- 不在浏览器插件里保存 LLM API Key，不让插件直接调用 Coroot、K8s、GitHub 或部署工具。
- 不自动执行高风险生产动作；一键部署/修复必须先生成计划，经过审批和 ActionToken。
- 不把 Coroot Enterprise 私有 RCA 当作主依赖；aiops-v2 自己基于证据生成 RCA。
- 不以 mock、截图或静态 fixture 作为最终验收依据。fixture 只用于单元测试和可重复回归，真实联调测试必须单独通过。

## 4. 设计选择

### 4.1 推荐方案：Case/Evidence 中心化编排

四项能力都汇入统一 Case：

```text
Browser issue / Alert / Deploy / K8s event
  -> IncidentCase
  -> EvidenceRef
  -> ToolDispatcher read-only collectors
  -> RCA / DeployPlan / K8sActionProposal
  -> Agent-to-UI artifact
  -> Approval / Runner / Verification
```

优点：复用现有 Incident、Evidence、Transport、Approval 和 Runner；真实数据可追溯；浏览器、告警、部署和 K8s 之间可以互相关联。

代价：需要先补齐统一数据模型和 connector registry，第一阶段会比独立页面开发慢。

推荐使用该方案。

### 4.2 备选方案：四个独立功能页

浏览器插件、GitHub 部署、K8s 管理、告警分析分别落页面和接口。

优点：单点功能启动快。

问题：证据、审批、审计和 Agent-to-UI 会重复实现，后续 RCA 很难把浏览器 trace、告警、部署和 K8s 事件串起来。

不推荐。

### 4.3 备选方案：全部收敛为 Runner Workflow

所有能力都以 Runner Workflow 表达，UI 只展示 workflow run。

优点：执行面统一。

问题：告警归并、浏览器 trace 关联、K8s 资源浏览和 RCA artifact 不适合全部塞进执行流程；读证据和写动作边界会变模糊。

只用于部署、修复、验证这类执行阶段，不作为总体架构。

## 5. 总体架构

```text
External sources
  - Browser Extension
  - GitHub / GitHub Actions
  - Kubernetes API
  - Coroot / Alertmanager / Prometheus
        |
        v
Connector Registry
        |
        v
ToolDispatcher + PolicyEngine
        |
        v
Case/Evidence Hub
        |
        +--> RCA Analyzer
        +--> Deployment Planner
        +--> K8s Resource Manager
        +--> Alert Correlator
        |
        v
AiopsTransportState -> React Chat / Incident Workbench / Plugin Side Panel
        |
        v
Approval -> Runner -> Verification -> Experience / Postmortem
```

关键边界：

- 外部系统只做数据源或动作执行端，不做 LLM 推理。
- 模型只能调用注册过的工具；工具必须声明 `readOnly`、`riskLevel`、`mock`、输入输出 schema。
- 所有 evidence 带 `source`、`rawRef`、`timeRange`、`caseId`、`digest`。
- 所有变更动作先生成计划和 preview，再由审批链路签发 ActionToken。

## 6. 数据模型

### 6.1 BrowserIssueEvent

```json
{
  "schemaVersion": "aiops.browser_issue/v1",
  "eventId": "browser-issue-20260518-0001",
  "sessionId": "sess-1",
  "tabId": "chrome-tab-42",
  "url": "https://erp.example.com/checkout",
  "pageTitle": "提交订单",
  "businessAction": "submit_order",
  "symptom": "提交订单变慢",
  "traceId": "trace-abc-001",
  "aiopsRunId": "run-browser-001",
  "observedAt": "2026-05-18T10:00:00+08:00",
  "performance": {
    "inpMs": 780,
    "lcpMs": 2100,
    "slowRequests": [
      {"urlPattern": "/api/orders/submit", "durationMs": 2850, "status": 200}
    ]
  }
}
```

### 6.2 AlertEvent

```json
{
  "schemaVersion": "aiops.alert/v1",
  "fingerprint": "alert-checkout-latency-prod",
  "source": "coroot",
  "severity": "critical",
  "service": "checkout-api",
  "environment": "prod",
  "startsAt": "2026-05-18T10:02:00+08:00",
  "labels": {"slo": "latency", "namespace": "prod"},
  "annotations": {"summary": "checkout-api p95 latency is above SLO"}
}
```

### 6.3 DeploymentPlan

```json
{
  "schemaVersion": "aiops.deployment_plan/v1",
  "planId": "deploy-plan-001",
  "service": "checkout-api",
  "environment": "prod",
  "repo": "github.com/example/checkout-api",
  "revision": "main@sha256:abc123",
  "build": {
    "strategy": "github_actions_workflow_dispatch",
    "workflow": "deploy.yml",
    "expectedImage": "registry.example.com/checkout-api:abc123"
  },
  "kubernetes": {
    "cluster": "prod-cn",
    "namespace": "prod",
    "workload": "deployment/checkout-api",
    "preview": "server-side-dry-run diff"
  },
  "risk": "medium",
  "expectedEffect": "checkout-api rollout to abc123 image digest",
  "rollback": "kubectl -n prod rollout undo deployment/checkout-api"
}
```

### 6.4 K8sResourceSnapshot

```json
{
  "schemaVersion": "aiops.k8s_resource/v1",
  "cluster": "prod-cn",
  "namespace": "prod",
  "kind": "Deployment",
  "name": "checkout-api",
  "uid": "2c26b46b",
  "resourceVersion": "129921",
  "status": "available",
  "replicas": {"desired": 3, "ready": 3, "updated": 3},
  "images": ["registry.example.com/checkout-api@sha256:abc123"],
  "capturedAt": "2026-05-18T10:03:00+08:00"
}
```

## 7. 功能设计

### 7.1 浏览器插件一键根因分析

用户路径：

```text
用户在 ERP 页面点击插件按钮“一键分析”
  -> 插件上报 BrowserIssueEvent
  -> aiops-v2 创建或关联 IncidentCase
  -> 绑定 traceId、URL、业务动作、性能指标和网络请求
  -> OpsGraph 将 URL/API/trace 映射到服务和业务能力
  -> 调用 Coroot、K8s、changes、GitHub metadata 只读工具采证
  -> aiops.rca_analyze 生成 RCAReport artifact
  -> 插件 side panel 和 aiops Chat 同步展示结果
```

后端落点：

```text
internal/browserissue/
  model.go
  service.go
  correlator.go
  redaction.go

internal/appui/
  browser_issue_service.go

internal/server/
  browser_issue_api.go
```

工具和接口：

- `POST /api/v1/browser/issues`：插件上报问题。
- `POST /api/v1/browser/issues/{eventId}/analyze`：触发 RCA。
- `browser.issue_context`：只读工具，读取当前 Case 绑定的浏览器证据。
- `aiops.rca_analyze`：统一 RCA 编排入口，优先使用已有 Coroot RCA 设计。

安全约束：

- 插件上报前必须本地脱敏 DOM、请求参数、错误栈和页面文本。
- 插件只持有短期 session token，不持有 LLM/GitHub/K8s/Coroot 凭证。
- 页面动作执行仍走 `page_agent.*` 工具和 `ToolDispatcher`，不能由插件自循环决策。
- 一键 RCA 只读，不执行修复动作。

### 7.2 GitHub 代码拉取与一键部署

本功能的“拉取代码”用于构建/部署，不用于让模型通用读写代码。

推荐 P0 路径：优先触发已有 GitHub Actions workflow。

```text
选择服务和 revision
  -> github.resolve_revision
  -> deploy.create_plan
  -> 生成 DeploymentPlan artifact
  -> 用户审批
  -> github.dispatch_workflow 或 build service
  -> 获取 image digest / artifact provenance
  -> k8s.server_side_diff
  -> 用户确认 rollout
  -> k8s.apply_manifest 或 k8s.set_image
  -> rollout_status + Coroot SLO 验证
```

后端落点：

```text
internal/integrations/github/
  client.go
  register.go
  tools.go
  models.go

internal/deployment/
  plan.go
  planner.go
  executor.go
  verifier.go
  store.go
```

工具：

- `github.list_repositories`：按服务或组织查仓库。
- `github.resolve_revision`：解析 branch/tag/PR/commit，返回 commit SHA、作者、时间、状态检查。
- `github.dispatch_workflow`：触发受允许列表限制的 workflow_dispatch。
- `deploy.create_plan`：生成部署计划、风险、预期影响、回滚方案。
- `deploy.execute_plan`：审批后执行部署计划。
- `changes.recent_deployments`：从 GitHub Actions、Release、Argo CD 或 Kubernetes rollout history 返回真实变更。

本地构建只作为 P1 fallback：

- 使用隔离 workspace 和短期 GitHub token。
- 只允许仓库声明的 `Dockerfile`、`buildpacks`、`ko`、`helm`、`kustomize` 入口。
- 构建产物必须输出 image digest，不接受 mutable tag 作为部署依据。
- 不允许模型修改仓库代码。

### 7.3 Kubernetes 资源管理

现有 `internal/integrations/k8s` 工具名保留，但实现从 mock 替换为真实 client-go/dynamic client。

后端落点：

```text
internal/integrations/k8s/
  client.go
  dynamic_client.go
  resource_mapper.go
  tools.go
  diff.go
  rollout.go
  policy.go
```

读能力：

- `k8s.list_resources`
- `k8s.get_workload`
- `k8s.get_events`
- `k8s.get_logs`
- `k8s.rollout_status`
- `k8s.describe_resource`
- `k8s.top_pods`（有 metrics-server 时启用）

写能力：

- `k8s.server_side_diff`
- `k8s.apply_manifest`
- `k8s.restart_workload`
- `k8s.scale_workload`
- `k8s.rollout_undo`

治理要求：

- 读操作必须带 cluster、namespace、kind/name 或 labelSelector。
- 写操作必须先执行 dry-run/diff，生成 `ActionProposal`。
- 写操作必须携带 ActionToken，并经过用户审批。
- 高风险命名空间、集群范围资源、PVC 删除、CRD 修改默认拒绝。
- 每次写操作必须写入 Case/Evidence/DeploymentRun 审计记录。
- 执行后必须跑 rollout 和 Coroot/K8s 验证。

### 7.4 故障告警分析

告警入口：

- Coroot webhook。
- Alertmanager webhook。
- Prometheus rule webhook。
- 手工导入告警 payload。

后端落点：

```text
internal/alerts/
  model.go
  normalizer.go
  deduper.go
  correlator.go
  analyzer.go
  store.go

internal/appui/
  alert_service.go

internal/server/
  alert_webhook_api.go
```

流程：

```text
Webhook payload
  -> AlertEvent normalizer
  -> fingerprint 去重和窗口聚合
  -> 创建或关联 IncidentCase
  -> 写 raw alert evidence
  -> enrich: Coroot metrics/topology/SLO + K8s events/logs + deployments + OpsGraph impact
  -> AlertAnalysis artifact
  -> 可选触发 aiops.rca_analyze
```

告警分析输出：

- 告警摘要和影响范围。
- 同窗口相关告警聚类。
- 相关服务、依赖、K8s workload、最近部署、配置变更。
- 根因假设排序和置信度。
- 缺失证据。
- 建议下一步：只读采证、生成部署回滚计划、执行 K8s 修复前置检查。

## 8. 前端和 Agent-to-UI

新增或扩展 artifact：

- `browser_issue_context`：展示页面、动作、trace、性能、慢请求。
- `rca_report`：复用 Coroot RCA 设计中的 RCA 主报告。
- `deployment_plan`：展示 revision、构建策略、diff、风险、回滚和审批状态。
- `k8s_resource_table`：展示命名空间、workload、pod、事件、日志入口。
- `alert_analysis`：展示告警聚合、影响面、证据和根因假设。

页面落点：

- Incident Detail：作为主工作台，串起浏览器问题、告警、部署、K8s 证据和 RCA。
- Chat：展示 artifact 和自然语言解释。
- Settings/Integrations：配置 GitHub、Kubernetes、Coroot、Alertmanager connector。
- K8s Resources：资源浏览和受控动作入口。
- Deployments：部署计划、执行记录、验证和回滚入口。

所有 Chat 可见过程仍走：

```text
TurnItem -> AiopsTransportState -> AssistantTransport -> React Chat
```

## 9. 真实数据测试策略

### 9.1 测试环境

必须准备一套真实联调环境：

- Kubernetes：本地 kind/minikube 或 staging 集群，真实 API Server、真实 Pod、真实 Event、真实 rollout。
- GitHub：专用测试仓库 `aiops-v2-deploy-fixture`，包含 Dockerfile、Helm/Kustomize manifest、GitHub Actions workflow。
- Registry：可推送和拉取镜像的测试 registry。
- Coroot/Prometheus：可读取真实服务指标、trace 或至少 Prometheus 指标的测试项目。
- Alertmanager 或 Coroot webhook：可向 aiops-v2 发送真实 webhook payload。
- 浏览器插件：使用真实构建产物或外部插件仓库产物，连接真实 aiops-v2 后端和测试业务页面。

### 9.2 单元测试

Go：

- `internal/browserissue`：脱敏、事件校验、traceId/URL/API/service 关联。
- `internal/alerts`：Alertmanager/Coroot payload normalizer、fingerprint、去重窗口、Case 关联。
- `internal/integrations/github`：revision 解析、workflow dispatch payload、权限错误、rate limit。
- `internal/integrations/k8s`：resource mapper、dry-run diff、ActionToken 校验、禁止高风险资源。
- `internal/deployment`：部署计划生成、风险分类、rollback 生成、verification 状态机。

前端：

- `web/src/components/chat/*Artifact*.test.tsx` 覆盖新增 artifact 渲染。
- `web/src/api/*` 覆盖 GitHub、K8s、alerts、browser issue API client。
- `web/src/transport/*` 确认 artifact 只通过 `AiopsTransportState` 投影。

### 9.3 真实集成测试

真实测试使用环境变量显式开启，默认开发机没有凭证时跳过，但预发布流水线必须开启并通过。

```bash
AIOPS_REAL_K8S=1 go test ./internal/integrations/k8s ./internal/deployment -run Real -count=1
AIOPS_REAL_GITHUB=1 go test ./internal/integrations/github ./internal/deployment -run Real -count=1
AIOPS_REAL_ALERTS=1 go test ./internal/alerts ./internal/appui -run Real -count=1
AIOPS_REAL_COROOT=1 go test ./internal/integrations/coroot ./internal/integrations/rca -run Real -count=1
```

真实测试验收条件：

- K8s 返回 `source: "kubernetes"`，不得返回 `source: "mock"`。
- GitHub 返回真实 commit SHA、workflow run id、check status。
- 部署输出真实 image digest 和 rollout revision。
- Coroot/Prometheus evidence 带 rawRef、时间窗和 digest。
- Alert webhook 创建或关联真实 Case，并写入 raw alert evidence。

### 9.4 浏览器端到端测试

使用 Playwright 加载真实插件或插件测试构建：

```bash
cd web
npm run test:ui -- browser-plugin-rca.spec.js
npm run test:ui -- deployment-k8s-real.spec.js
npm run test:ui:snapshots
```

场景 1：浏览器插件一键 RCA

1. 启动测试业务页面 `/checkout`，后端生成真实 traceId。
2. 插件点击“一键分析”。
3. aiops-v2 创建 Case。
4. Case evidence 包含 BrowserIssueEvent、traceId、Coroot/Prometheus evidence、K8s workload snapshot。
5. Chat/插件展示 `rca_report` artifact。
6. 断言报告包含根因假设、证据、反证、缺失证据和下一步建议。

场景 2：GitHub 到 K8s 一键部署

1. 选择 `aiops-v2-deploy-fixture` 的测试分支。
2. 解析真实 commit SHA。
3. 触发 GitHub Actions workflow。
4. 获取真实 image digest。
5. 生成 Kubernetes dry-run diff。
6. 审批后部署到 kind/staging namespace。
7. rollout 成功，服务镜像 digest 与计划一致。
8. 验证 `changes.recent_deployments` 能查到该部署。

场景 3：K8s 资源管理

1. 从真实集群列出 namespace、Deployment、Pod、Service。
2. 读取真实 Pod logs 和 Events。
3. 对测试 Deployment 执行 scale dry-run。
4. 审批后 scale 到 2，再 scale 回 1。
5. 审计记录包含 ActionToken、审批人、diff、执行结果和 rollback。

场景 4：故障告警分析

1. 通过 Alertmanager/Coroot 发送 checkout-api latency 告警。
2. 系统按 fingerprint 去重。
3. 创建或关联 Case。
4. 自动补充 Coroot/K8s/changes/OpsGraph 证据。
5. 生成 `alert_analysis` artifact。
6. 对同一告警重复发送，只更新聚合窗口，不创建重复 Case。

### 9.5 回归守卫

每次涉及 Chat、tool、审批、MCP、K8s、部署、浏览器插件链路的变更都必须运行：

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "JSON\\.parse\\(|markdown heading|summary.*steps.*actions" web/src
```

生产代码中不得出现：

- 直接绕过 `ToolDispatcher` 调用 K8s/GitHub/Coroot 写动作。
- 对特定 tool name 做硬编码 UI 解析。
- 使用 final Markdown 派生过程 UI。
- 真实验收路径返回 `mock: true`。

## 10. 分阶段交付

### P0：真实数据基础设施

- 增加 connector registry：GitHub、Kubernetes、Alertmanager、Coroot 配置统一管理。
- 将 `k8s.get_workload/get_events/get_logs/rollout_status` 接真实 Kubernetes API。
- 将 `changes.recent_deployments` 接 GitHub Actions 或 Kubernetes rollout history。
- 增加 `alerts` normalizer/deduper/store。
- 增加 `browser issues` 后端 API 和 Case/Evidence 绑定。
- 建立 kind + GitHub fixture + webhook + Coroot/Prometheus 的真实测试脚本。

验收：四类真实数据源都能写入 EvidenceRef，且不返回 mock。

### P1：一键分析和部署计划

- 浏览器插件 issue 触发 `aiops.rca_analyze`。
- 告警触发 `alert_analysis` 和可选 RCA。
- GitHub revision 解析和 `deploy.create_plan` artifact。
- K8s resource table 和 Incident Detail 集成。

验收：浏览器问题和告警都能形成 Case + RCA artifact；部署能生成完整计划和 dry-run diff。

### P2：受控执行

- `github.dispatch_workflow` 接真实 GitHub Actions。
- `deploy.execute_plan` 编排构建、镜像 digest、K8s rollout。
- `k8s.scale_workload/restart_workload/rollout_undo/apply_manifest` 接真实集群。
- 所有写动作纳入 ActionToken、审批、审计和恢复验证。

验收：测试服务可从 GitHub commit 部署到 Kubernetes，并可完成 rollout 验证和回滚。

### P3：闭环优化

- RCA 与部署/告警/K8s 事件自动关联。
- Proof of Recovery 沉淀为 Experience candidate。
- 通过真实历史 Case 做评测集，覆盖慢请求、CrashLoop、错误率升高、部署回滚四类场景。

验收：系统能从真实 Case 自动生成可审核经验，并在相似问题中召回。

## 11. 验收标准

浏览器插件链路：

- 真实页面一键触发 Case。
- 真实 traceId 能关联到服务或明确报告缺失原因。
- RCA artifact 包含证据、假设、反证、置信度、缺失证据和下一步。
- 不执行任何修复动作。

GitHub 部署链路：

- 能从真实 GitHub repo 解析 commit SHA。
- 能触发真实 workflow 或受控 build。
- 能获取真实 image digest。
- 能对真实 K8s workload 生成 dry-run diff。
- 审批后 rollout 成功，失败时保留 rollback。

K8s 管理：

- 读操作来自真实 API Server。
- 写操作必须有 dry-run、ActionToken、审批和审计。
- 禁止未授权 namespace、cluster-scoped 高危资源和删除 PVC。
- 执行后必须验证 rollout 或资源状态。

故障告警分析：

- Coroot/Alertmanager webhook 能创建或关联 Case。
- 同 fingerprint 告警按时间窗口聚合。
- 告警分析能补充 Coroot、K8s、变更和 OpsGraph 证据。
- 证据不足时明确标注缺失，不伪造根因。

## 12. 风险和缓解

| 风险 | 缓解 |
| --- | --- |
| 真实凭证泄露 | connector 凭证只存后端，Secret 加密或外部 Secret Manager，前端和插件只拿短期 session token |
| 一键部署误操作 | plan-first、dry-run、ActionToken、审批、namespace allowlist、rollback 必填 |
| 真实测试不稳定 | 单元/契约测试保持快速，真实测试放预发布和 nightly；失败保留 evidence artifact |
| 浏览器数据敏感 | 插件本地脱敏，后端二次校验，EvidenceRef 只保存摘要和 rawRef digest |
| K8s 权限过大 | 按 cluster/environment 配置只读和写权限 service account，生产写动作默认需要人工审批 |
| GitHub workflow 不统一 | P0 只支持 allowlist workflow；本地构建作为 P1 fallback，不阻塞主链路 |

## 13. 实施顺序建议

1. 先做真实 K8s read-only 和 alerts normalizer，因为它们是 RCA 和部署验证的共同依赖。
2. 再做 GitHub revision/workflow connector，把 `changes.recent_deployments` 从 mock 替换为真实变更源。
3. 接浏览器 issue API 和 Case/Evidence 绑定，让插件可以一键创建 RCA 输入。
4. 最后打开受控部署和 K8s 写动作，确保每个动作已有 dry-run、审批、回滚和验证。

这个顺序能先把证据面做真实，再逐步放开执行面，避免在 mock 数据上构建“一键部署”和“一键修复”体验。
