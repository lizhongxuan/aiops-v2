# aiops-v2 Observability, Coroot & Debug Trace 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)
前端设计：[2026-05-11-aiops-v2-04a-observability-debug-trace-frontend-design.zh.md](2026-05-11-aiops-v2-04a-observability-debug-trace-frontend-design.zh.md)
实施清单：[2026-05-11-aiops-v2-04b-observability-debug-trace-frontend-todo.zh.md](2026-05-11-aiops-v2-04b-observability-debug-trace-frontend-todo.zh.md)

## 1. 模块定位

Observability 模块负责把 Coroot、trace backend、日志、前端 Debug Mode 和中间件只读诊断结果归一化为 `EvidenceRef`。它不直接给出最终根因，也不直接执行修复，而是为 Incident Control Plane、AI Reasoning Plane、OpsGraph 和 Verification 提供可信证据。

用户侧慢请求调试是本模块的一等场景：用户觉得页面某个按钮慢，开启 Debug Mode 后再次点击，系统必须把浏览器请求到后端、中间件和数据库的全链路打上 trace id，并基于 Coroot 和 trace 证据定位慢点。

## 2. 设计目标

- Coroot MCP 和 webhook 输出统一转成 EvidenceRef。
- 前端 DebugEvent 能生成 trace id 并贯通浏览器、网关、服务、中间件和数据库。
- 证据必须结构化、可引用、可脱敏、可裁剪。
- Coroot RCA 只能作为证据和假设来源，不能直接作为最终根因。
- 支持证据质量评估：缺失 trace、缺失 span、采样不足、时间窗口不匹配必须明确标记。

## 3. 关键对象

```text
TraceContext
  id
  caseId
  traceId
  rootSpanId
  source: frontend_debug | coroot | trace_backend | gateway
  frontendRoute
  userAction
  apiRoute
  servicePath
  slowSpanIds
  errorSpanIds
  baggageSummary
  redactionStatus
  createdAt
```

```text
DebugEvent
  id
  caseId
  userId
  sessionId
  pageUrlHash
  routeName
  actionName
  traceContextId
  frontendTimings
  backendTimings
  corootEvidenceRefs
  status: captured | analyzing | remediating | verified | failed
```

```text
ObservationEvidence
  id
  caseId
  source
  sourceRef
  observationType: metric | rca | topology | trace | log | event | middleware_probe
  timeWindow
  entityRefs
  summary
  quality
  redactionStatus
```

## 4. Coroot 接入

Coroot 提供以下证据：

- services、applications、projects。
- service metrics：latency、error rate、throughput、resource saturation。
- topology：service dependency、database、queue、cache。
- SLO 和 burn。
- RCA 和 timeline。
- alert rule 和 webhook。

归一化规则：

- `coroot.service` -> OpsGraph `Service`。
- `coroot.application` -> OpsGraph `RuntimeResource` 或 `Application`。
- `coroot.rca` -> `EvidenceRef` + `HypothesisCandidate`。
- `coroot.topology` -> OpsGraph edge candidate。
- `coroot.webhook` -> create/update `IncidentCase`。

Coroot 证据必须保留 raw ref，避免把第三方工具输出复制到主数据库。

## 5. Debug Mode 全链路

```text
User enables Debug Mode
  -> frontend creates DebugEvent
  -> frontend creates or receives trace id
  -> request carries traceparent, baggage, aiops-debug
  -> gateway validates debug header and records user/session hash
  -> backend spans inherit trace id
  -> DB/cache/MQ spans attach to same trace
  -> Coroot and trace backend collect data
  -> aiops-v2 creates DebugCase
  -> AI performs RCA and remediation planning
```

前端必须采集：

- route name、page url hash、action name。
- click to request。
- request start、TTFB、response end。
- render commit 或用户可感知完成时间。
- HTTP status、api route、trace id。

前端禁止采集：

- 请求体。
- cookie。
- token。
- password。
- 用户输入原文。

## 6. Header 约定

```text
traceparent: W3C trace context
baggage: aiops_case_id, aiops_debug_event_id, environment
x-aiops-debug: signed debug marker
x-aiops-session: redacted session hash
```

`x-aiops-debug` 必须是短 TTL 签名标记，避免普通用户伪造生产调试请求。

## 7. 证据质量评分

```text
EvidenceQuality
  traceCompleteness: full | partial | missing
  spanCoverage: frontend | gateway | backend | middleware | db | mq
  timeWindowMatch: exact | overlapping | weak | mismatch
  redactionStatus: passed | partial | failed
  sourceFreshness
  confidence
```

质量规则：

- trace id 缺失：不能自动给出修复结论。
- backend span 缺失：只能定位到前端或网关。
- middleware span 缺失：不能断言 DB/MQ 是根因。
- redaction failed：证据不能进入 PromptCompiler，只能保留受限引用。

## 8. API 草案

```text
POST   /api/v1/debug-events
GET    /api/v1/debug-events/{id}
POST   /api/v1/debug-events/{id}/attach-trace
POST   /api/v1/debug-events/{id}/analyze
GET    /api/v1/traces/{trace_id}/summary
GET    /api/v1/coroot/services
GET    /api/v1/coroot/services/{id}/metrics
GET    /api/v1/coroot/services/{id}/rca
GET    /api/v1/coroot/topology
POST   /api/v1/coroot/webhooks
```

## 9. 输出给 AI 的摘要

AI 不直接读取全部原始 trace。Prompt 输入使用结构化摘要：

```yaml
debug_event:
  action: "checkout.submit"
  route: "/orders/new"
  trace_id: "4bf92f..."
  frontend:
    click_to_request_ms: 32
    ttfb_ms: 1840
    render_ms: 90
  backend:
    total_ms: 1780
    slow_spans:
      - service: "order-api"
        operation: "POST /api/orders"
        duration_ms: 460
      - service: "postgres"
        operation: "lock wait"
        duration_ms: 1180
  coroot:
    rca_summary: "postgres lock wait increased"
    slo_status: "latency burn"
quality:
  traceCompleteness: "full"
  redactionStatus: "passed"
```

## 10. UI 设计

DebugCase 页面展示：

- 用户动作和页面路径。
- trace id 和证据质量。
- waterfall：frontend、gateway、backend、middleware、db、mq。
- Coroot RCA 和 SLO 状态。
- AI 根因解释和反证。
- 可自动化修复建议。
- 修复后同一动作的验证对比。

## 11. 异常处理

- trace backend 不可用：DebugEvent 创建成功，但状态为 `captured_without_trace_backend`。
- Coroot 不可用：保留 trace 证据，AI 输出必须标记缺少 Coroot 指标。
- 用户关闭页面：DebugEvent 保留，后台继续拉取 trace 直到 TTL。
- 调试 header 验证失败：拒绝 Debug Mode 上报，只保存安全审计。

## 12. 验收标准

- Coroot webhook 能生成 EvidenceRef 并关联 case。
- Debug Mode 产生的请求能携带 traceparent 并贯通后端 span。
- DebugCase 能展示前端动作、慢 span、Coroot RCA 和证据质量。
- trace 缺失时系统不会给出自动修复结论。
- Debug Trace 数据经过脱敏，不保存请求体、cookie、token 或敏感输入。
- 修复后能用新的 trace 对比验证用户动作耗时是否恢复。
