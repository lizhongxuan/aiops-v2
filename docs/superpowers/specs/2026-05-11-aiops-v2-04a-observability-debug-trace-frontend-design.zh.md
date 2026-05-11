# aiops-v2 Observability, Coroot & Debug Trace 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-04-observability-debug-trace-module-design.zh.md](2026-05-11-aiops-v2-04-observability-debug-trace-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-04b-observability-debug-trace-frontend-todo.zh.md](2026-05-11-aiops-v2-04b-observability-debug-trace-frontend-todo.zh.md)

## 1. 页面目标

Observability, Coroot & Debug Trace 的前端不是监控大屏，也不是 Coroot iframe 的包装层，而是生产证据工作台。页面要让用户完成：

- 查看 Coroot、trace backend、日志、前端 Debug Mode、中间件只读探针生成的 `EvidenceRef`。
- 判断证据是否足够支持下一步 RCA、计划生成、自动化修复或验证。
- 从 Coroot service、SLO、RCA、topology 跳到 case、OpsGraph、Debug Trace 和 Runbook。
- 用户反馈“业务页面按钮慢”后，进入 Debug Mode，捕获下一次用户动作到后端、中间件和数据库的 trace。
- 在 DebugCase 中查看 waterfall、慢 span、Coroot RCA、证据质量、AI 假设、反证、修复建议和修复后对比。
- 明确区分“证据”“假设”“已确认根因”“可自动化动作”，避免把 Coroot RCA 或 AI 推理直接展示为事实。

设计原则：

- 第一屏展示数据源健康、证据数量、质量缺口和活跃 DebugEvent，不做纯图表展示。
- Coroot iframe 可以保留，但不能成为唯一交互方式。关键排障信息必须进入结构化 view-model。
- Debug Trace 只采集性能和路由上下文，不采集请求体、cookie、token、password、用户输入原文。
- 证据质量是核心 UI 元素。trace 缺失、span 覆盖不完整、时间窗口不匹配、脱敏失败都必须显式展示。
- 页面只消费统一 Observability API client，不在页面内直接拼接 Coroot proxy、trace backend 和 case API。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/CorootOverviewPage.tsx`：已有 Coroot 服务列表、Dashboard iframe、Topology iframe 和 AI monitor context。
- `web/src/api/coroot.js`：已有 Coroot config、service list 和通用 Coroot JSON 请求。
- `web/src/lib/corootCardAdapter.js`：已有 Coroot service、metrics、alerts、host、stats 的卡片适配。
- `web/src/lib/corootTopologyAdapter.js`：已有 Coroot topology 和 service dependencies 的节点边适配。
- `web/src/pages/PromptTracePage.tsx`：已有本地模型输入 Prompt Trace 查看器。
- `web/src/pages/IncidentWorkbenchPage.tsx`：已有 case 详情中展示 evidence、hypothesis、OpsGraph 和 Runbook 的初版上下文栏。
- `internal/incidents/evidence.go`：后端已有 `EvidenceRef` 基础结构。
- `internal/server/resource_coroot.go`：已有只读 Coroot proxy 和 config API。
- `internal/server/coroot_webhook_api.go`：已有 Coroot webhook 入口。
- `web/src/components/ui/*` 与 `web/src/pages/settingsComponents.tsx`：已有按钮、卡片、表格、Badge、Alert、Loading 等基础组件。

主要不足：

- `CorootOverviewPage.tsx` 使用页面内 `fetch` 和页面内类型定义，应迁移到统一 API client。
- Coroot 页面仍偏 iframe 和列表，缺少 EvidenceRef、EvidenceQuality、Case、OpsGraph 的统一串联。
- 用户侧 Debug Mode / DebugEvent / DebugCase 工作台尚未落地。
- Operational Trace 和 Prompt Trace 没有明确区分。`/debug/prompts` 应继续属于 AI Reasoning / Prompt Trace，不应承载用户侧慢请求调试。
- 缺少 trace waterfall、span 覆盖、证据质量和脱敏状态的复用组件。
- 缺少 Coroot 不可用、trace backend 不可用、header 验证失败、用户关闭页面等异常状态的前端表达。

## 3. 路由与信息架构

新增统一入口：

```text
/observability
/observability/debug-events/:debugEventId
```

保留现有入口：

```text
/coroot
/debug/prompts
```

路由语义：

- `/observability`：可观测证据工作台。整合 Coroot、Evidence Feed、Debug Events、Trace Summary、Source Health。
- `/observability/debug-events/:debugEventId`：DebugCase 详情。展示用户动作、trace waterfall、证据质量、RCA 和验证对比。
- `/coroot`：兼容入口，后续可以跳转到 `/observability?tab=coroot`，或保留为 Coroot 专用视图但复用同一组件。
- `/debug/prompts`：LLM Prompt Trace 查看器，继续归 05 模块，不混入用户侧业务请求 trace。

MVP 可以先不新增 `:debugEventId` 子页面，使用 `/observability?debugEventId=...&tab=debug-detail` 作为落点；但设计上保留详情页能力。

主 tabs：

| Tab | 目标 |
| --- | --- |
| Evidence Feed | 浏览和筛选 ObservationEvidence / EvidenceRef |
| Coroot Services | 服务健康、SLO、RCA、metrics、topology 和原始 Coroot 引用 |
| Debug Events | 发起、跟踪、分析用户侧 DebugEvent |
| Trace Waterfall | 展示 frontend、gateway、backend、middleware、db、mq 的耗时分布和慢点 |
| Evidence Quality | 展示 trace completeness、span coverage、time window、redaction、confidence |
| Source Health | 展示 Coroot、trace backend、webhook、gateway debug header、ingestion backlog |

## 4. 页面一：Observability 工作台

### 4.1 总体布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: Observability / 环境 / 时间窗口 / 刷新 / 创建 DebugEvent │
├────────────────────────────────────────────────────────────┤
│ Source Health Strip: Coroot / Trace Backend / Webhook / Gateway │
├────────────────────────────────────────────────────────────┤
│ Context Bar: Case / Service / Trace / DebugEvent / Route / Action │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: Evidence / Quality Gaps / Active Debug / RCA / SLO │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Evidence Detail Drawer   │
│ Evidence / Coroot / Debug /   │ Evidence / Span / RCA /       │
│ Waterfall / Quality / Health  │ Coroot Raw Ref / Case Links   │
└──────────────────────────────┴─────────────────────────────┘
```

桌面端右侧 Drawer 宽度 360-420px。窄屏时 Drawer 变成底部 Sheet。

### 4.2 Header

显示：

- 当前环境。
- 当前时间窗口。
- 数据刷新时间。
- 当前用户是否具备 Debug Mode、查看受限证据、创建 case、创建 ActionProposal 的权限。

动作：

- 刷新。
- 创建 DebugEvent。
- 打开 Coroot 原始页面。
- 打开关联 case。
- 复制当前证据查询链接。

### 4.3 Source Health Strip

展示：

| 数据源 | 状态 | 展示内容 |
| --- | --- | --- |
| Coroot | connected / degraded / unavailable / not_configured | base URL、最后成功拉取、错误摘要 |
| Trace Backend | connected / degraded / unavailable | trace 查询延迟、采样率、最近错误 |
| Webhook | receiving / idle / failed | 最近 webhook 时间、失败次数 |
| Gateway Debug Header | enabled / disabled / invalid | header 验证状态、签名 TTL 配置 |
| Evidence Ingestion | healthy / backlog / failed | 队列积压、失败事件数 |

Source Health 不只是状态灯。点击每一项必须打开 detail，说明受影响能力，例如“Coroot 不可用，RCA 和 SLO 证据缺失，但 trace waterfall 仍可查看”。

### 4.4 Context Bar

支持上下文：

| 上下文 | 输入 | 页面行为 |
| --- | --- | --- |
| Case | case id | 展示该 case 的证据、trace、Coroot RCA、质量缺口 |
| Service | service id / name | 展示服务指标、SLO、topology、关联 case |
| Trace | trace id | 展示 trace summary 和 waterfall |
| DebugEvent | debug event id | 展示 DebugCase 详情入口 |
| Route / Action | routeName、actionName | 检索用户动作和 API 路径证据 |
| Middleware | db / cache / mq id | 展示中间件相关 span、Coroot topology 和只读探针证据 |

字段：

- Query。
- Type。
- Environment。
- Time window。
- Source。
- Quality filter：全部、可用于 RCA、缺失 trace、脱敏失败、低置信。
- Redaction filter：visible、redacted、restricted。

## 5. Evidence Feed Tab

Evidence Feed 回答“目前有哪些证据，它们是否可信，能用于什么”。

### 5.1 表格列

| 列 | 内容 |
| --- | --- |
| Evidence | id、summary、observationType |
| Source | coroot、trace_backend、frontend_debug、gateway、log、middleware_probe |
| Entity | service、api route、middleware、host、case |
| Time Window | start、end、是否匹配当前查询 |
| Quality | completeness、coverage、freshness、confidence |
| Redaction | visible、redacted、restricted、failed |
| Usage | RCA allowed、Prompt allowed、Automation blocked、Verification allowed |
| Action | 查看详情、加入 case、打开 OpsGraph、打开 Coroot raw ref |

### 5.2 证据详情

右侧 Drawer 展示：

- EvidenceRef id。
- source 和 rawRef。
- observationType。
- summary。
- timeWindow。
- entityRefs。
- quality 细项。
- redactionStatus。
- 关联 case、trace、Coroot service、OpsGraph entity。

受限证据只显示 id、source、observedAt、restriction reason，不能在 tooltip、title、aria-label 中泄露正文。

### 5.3 使用资格

每条证据必须显示它可以用于哪些下游：

- PromptCompiler：只有脱敏通过且权限允许的摘要可以进入。
- RCA：需要 traceCompleteness 非 missing，且 timeWindowMatch 非 mismatch。
- Automation：不能只依赖低置信证据或 Coroot RCA。
- Verification：必须有明确 timeWindow 和目标实体。

## 6. Coroot Services Tab

Coroot Services 回答“Coroot 看到了哪些服务异常，它们和 AIOps 证据如何对应”。

### 6.1 布局

```text
Left: Service Table
Center: Metrics / SLO / RCA / Topology
Right: Evidence Detail / Case Links / OpsGraph Links
```

### 6.2 Service Table

| 列 | 内容 |
| --- | --- |
| Service | name、id、project、application |
| Status | healthy、warning、critical、unknown |
| SLO | availability、latency、burn rate |
| Metrics | p95 latency、error rate、throughput、resource saturation |
| Evidence | EvidenceRef count、RCA count、topology edge candidate count |
| Links | Case、OpsGraph、Runbook、Coroot raw |

### 6.3 Service Detail

服务详情区展示：

- service overview。
- latency、error rate、throughput、CPU、memory、network、disk。
- SLO 和 burn。
- Coroot RCA 摘要。
- timeline。
- topology dependencies。
- 生成的 EvidenceRef 列表。
- OpsGraph 映射状态。

Coroot RCA 展示为 `HypothesisCandidate` 或证据来源，不能显示为已确认根因。

### 6.4 Coroot iframe

保留 iframe 作为人工深挖入口：

- iframe 放在 Coroot Raw 子区，不作为第一屏核心。
- iframe 不承载审核、证据质量、case 创建和动作建议。
- 若 Coroot 未配置，显示配置缺口和 Settings/MCP 入口。

## 7. Debug Events Tab

Debug Events 回答“用户侧慢请求调试是否已捕获、是否能分析、下一步做什么”。

### 7.1 Debug Mode Launcher

创建 DebugEvent 的表单字段：

| 字段 | 说明 |
| --- | --- |
| Case | 可选。没有 case 时可以创建 DebugCase |
| Target App | 业务系统或前端应用标识 |
| Route Name | 业务页面 route |
| Action Name | 用户要再次点击的按钮或动作 |
| Environment | prod、staging、dev |
| TTL | Debug header 有效期，默认 5 分钟 |
| Capture Mode | capture_next_action、capture_until_stop |
| Expected Slow Threshold | 用户可感知慢阈值 |

创建后展示：

- DebugEvent id。
- signed `x-aiops-debug` 状态。
- traceparent 注入状态。
- SDK / gateway / browser extension 是否已接入。
- 用户操作提示：只提示“请在目标业务页面复现一次动作”，不展示敏感采集内容。

### 7.2 Debug Events 表格

| 列 | 内容 |
| --- | --- |
| DebugEvent | id、routeName、actionName |
| Status | captured、analyzing、remediating、verified、failed、captured_without_trace_backend |
| Trace | trace id、root span、trace completeness |
| Frontend | click_to_request、TTFB、render commit |
| Backend | total、slow span count、error span count |
| Quality | span coverage、time window、redaction |
| Action | 查看 DebugCase、分析、打开 case、打开 OpsGraph |

### 7.3 DebugEvent 状态

```text
created -> armed -> captured -> analyzing -> diagnosis_ready
  -> awaiting_approval -> remediating -> verifying -> verified
  -> failed
```

异常状态：

- `captured_without_trace_backend`：DebugEvent 已创建，但 trace backend 不可用。
- `header_rejected`：debug header 验证失败，只显示安全审计摘要。
- `partial_trace`：缺少 backend、middleware、db 或 mq span。
- `redaction_failed`：证据不能进入 PromptCompiler。
- `expired`：TTL 过期，没有捕获到用户动作。

## 8. DebugCase 详情页

DebugCase 是用户侧慢请求调试的核心页面。

### 8.1 Header

显示：

- DebugEvent id。
- case id。
- routeName。
- actionName。
- trace id。
- status。
- evidence quality summary。
- createdAt、capturedAt、analyzedAt。

动作：

- 重新分析。
- 关联或创建 case。
- 打开 OpsGraph 根因路径。
- 生成 ActionProposal。
- 发起修复后验证对比。

`traceCompleteness=missing` 或 `redactionStatus=failed` 时，生成 ActionProposal 按钮必须禁用，并显示原因。

### 8.2 第一屏布局

```text
┌────────────────────────────────────────────────────────────┐
│ Debug Header + Quality Gate                                │
├──────────────────────────────┬─────────────────────────────┤
│ Trace Waterfall               │ Root Cause & Evidence       │
│ frontend/gateway/backend/...  │ Coroot RCA / AI Hypothesis  │
├──────────────────────────────┴─────────────────────────────┤
│ Verification Compare / Suggested Actions / Timeline         │
└────────────────────────────────────────────────────────────┘
```

### 8.3 Quality Gate

Quality Gate 汇总：

- traceCompleteness：full、partial、missing。
- spanCoverage：frontend、gateway、backend、middleware、db、mq。
- timeWindowMatch：exact、overlapping、weak、mismatch。
- redactionStatus：passed、partial、failed。
- sourceFreshness。
- confidence。

展示下游影响：

- 可否进入 AI Prompt。
- 可否给出候选根因。
- 可否生成自动化修复建议。
- 可否作为验证基线。

## 9. Trace Waterfall Tab

Trace Waterfall 回答“慢在哪里”。

### 9.1 分层

```text
Frontend
Gateway
Backend Service
Middleware
Database
Queue
Cache
Render / User Perceived Complete
```

### 9.2 展示字段

| 字段 | 说明 |
| --- | --- |
| Segment | frontend、gateway、backend、db、mq、cache |
| Service / Resource | service name、db、queue、cache |
| Operation | HTTP route、SQL signature、lock wait、cache command、mq publish/consume |
| Duration | span duration |
| Start Offset | 相对点击时间 |
| Status | ok、slow、error、missing |
| Evidence | span id、EvidenceRef、Coroot metric |

SQL 或 cache operation 只显示 signature，不展示参数和用户输入。

### 9.3 慢点解释

慢点解释区展示：

- slow span 排名。
- error span 排名。
- Coroot RCA 摘要。
- SLO burn。
- 反证，例如 frontend render 很快、backend total 较低、DB span 缺失。
- 推荐下一步，例如补采 trace、查看 lock wait、打开 OpsGraph、生成 Runbook 匹配。

## 10. Evidence Quality Tab

Evidence Quality 面向排障可靠性，而不是展示漂亮分数。

### 10.1 质量矩阵

| 维度 | full / good | partial / weak | missing / failed |
| --- | --- | --- | --- |
| Trace Completeness | traceparent 贯通前后端 | 部分 span 缺失 | trace id 缺失 |
| Span Coverage | frontend/gateway/backend/middleware/db/mq 完整 | 缺少部分资源 | 只能看到 frontend 或 Coroot |
| Time Window Match | 证据窗口精确覆盖用户动作 | 窗口重叠但不精确 | 时间窗口不匹配 |
| Redaction | 脱敏通过 | 部分字段受限 | 脱敏失败 |
| Source Freshness | 当前窗口内 | 延迟或缓存 | 过期 |
| Confidence | 多来源互相印证 | 单来源或弱证据 | 低置信 |

### 10.2 决策门禁

显示清楚：

- 不能自动修复的原因。
- 不能确认根因的原因。
- 需要补充的证据。
- 可以安全继续的只读诊断动作。

例如：

```text
trace id missing -> 禁止自动修复，只允许生成补充观测计划
middleware span missing -> 不能断言 DB/MQ 根因
redaction failed -> 不能进入 PromptCompiler，只保留 EvidenceRef
time window mismatch -> 只能作为背景证据
```

## 11. Source Health Tab

Source Health 展示接入和采集链路状态。

### 11.1 Coroot

显示：

- configured。
- baseUrl / iframeUrl。
- last successful request。
- service API 状态。
- topology API 状态。
- SLO API 状态。
- webhook last received。
- 最近错误。

操作：

- 测试连接。
- 打开 Coroot。
- 打开 MCP Catalog。
- 查看 webhook payload 摘要。

### 11.2 Trace Backend

显示：

- query API 状态。
- trace lookup latency。
- sampling summary。
- last trace seen。
- missing span rate。
- storage retention。

### 11.3 Gateway Debug Header

显示：

- signed marker 是否启用。
- TTL 配置。
- 最近拒绝数。
- 拒绝原因分布。
- `x-aiops-session` hash 状态。

安全规则：

- 不显示签名密钥。
- 不显示原始 session id。
- 不显示 cookie 或 authorization header。

## 12. 嵌入式入口

### 12.1 Case 工作台

Case 工作台复用：

- Evidence Quality Summary。
- Coroot Evidence Summary。
- Debug Trace Summary。
- Trace Waterfall 摘要。
- Source Health warning。

Case 页面只显示摘要和跳转入口，不展示完整 Coroot iframe。

### 12.2 OpsGraph

OpsGraph 使用 Observability 证据：

- Coroot topology 生成 edge candidate。
- Trace span signature 生成 FrontendPage -> UserAction -> APIRoute -> Service -> MiddlewareResource 路径。
- EvidenceRef 作为边证据。

OpsGraph 页面只引用 evidence summary，证据详情回到 Observability。

### 12.3 AI Reasoning

AI Reasoning 只能读取结构化摘要：

- debug event summary。
- trace waterfall summary。
- Coroot RCA summary。
- quality gate。
- evidenceRefs。

不能读取原始 trace payload、请求体、cookie、token、password 或用户输入原文。

### 12.4 Verification

修复后验证复用：

- 同一 action 的新 DebugEvent。
- baseline trace 与 post-fix trace 对比。
- SLO 和 Coroot 指标恢复情况。
- ERP 或业务接口验证结果。

## 13. 前端数据模型

新增 `web/src/api/observability.ts`：

```ts
export interface TraceContextView {
  id: string;
  caseId?: string;
  traceId?: string;
  rootSpanId?: string;
  source: "frontend_debug" | "coroot" | "trace_backend" | "gateway";
  frontendRoute?: string;
  userAction?: string;
  apiRoute?: string;
  servicePath: string[];
  slowSpanIds: string[];
  errorSpanIds: string[];
  baggageSummary?: string;
  redactionStatus: "visible" | "redacted" | "restricted" | "failed";
  createdAt?: string;
}

export interface DebugEventView {
  id: string;
  caseId?: string;
  userId?: string;
  sessionIdHash?: string;
  pageUrlHash?: string;
  routeName?: string;
  actionName?: string;
  traceContext?: TraceContextView;
  frontendTimings: FrontendTimingView;
  backendTimings: BackendTimingView;
  corootEvidenceRefs: EvidenceRefView[];
  quality: EvidenceQualityView;
  status:
    | "created"
    | "armed"
    | "captured"
    | "captured_without_trace_backend"
    | "analyzing"
    | "diagnosis_ready"
    | "awaiting_approval"
    | "remediating"
    | "verifying"
    | "verified"
    | "failed"
    | "expired"
    | "header_rejected";
  createdAt?: string;
  capturedAt?: string;
  analyzedAt?: string;
}

export interface ObservationEvidenceView {
  id: string;
  caseId?: string;
  source: "coroot" | "trace_backend" | "frontend_debug" | "gateway" | "log" | "middleware_probe";
  sourceRef?: string;
  observationType: "metric" | "rca" | "topology" | "trace" | "log" | "event" | "middleware_probe";
  timeWindow: { start?: string; end?: string };
  entityRefs: string[];
  summary: string;
  quality: EvidenceQualityView;
  redactionStatus: "visible" | "redacted" | "restricted" | "failed";
  usage: EvidenceUsageView;
}

export interface EvidenceQualityView {
  traceCompleteness: "full" | "partial" | "missing";
  spanCoverage: Array<"frontend" | "gateway" | "backend" | "middleware" | "db" | "mq" | "cache">;
  timeWindowMatch: "exact" | "overlapping" | "weak" | "mismatch";
  redactionStatus: "passed" | "partial" | "failed";
  sourceFreshness: "fresh" | "delayed" | "stale" | "unknown";
  confidence: number;
  blockingReasons: string[];
}

export interface TraceWaterfallSegmentView {
  id: string;
  segment: "frontend" | "gateway" | "backend" | "middleware" | "db" | "mq" | "cache" | "render";
  service?: string;
  resource?: string;
  operation?: string;
  durationMs: number;
  startOffsetMs: number;
  status: "ok" | "slow" | "error" | "missing";
  evidenceRefs: EvidenceRefView[];
}

export interface SourceHealthView {
  source: "coroot" | "trace_backend" | "webhook" | "gateway_debug_header" | "evidence_ingestion";
  status: "connected" | "healthy" | "degraded" | "unavailable" | "not_configured" | "failed";
  lastSeenAt?: string;
  message?: string;
  affectedCapabilities: string[];
}
```

## 14. API 契约

使用模块设计中的 API：

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

兼容当前已存在 API：

```text
GET    /api/v1/coroot/config
GET    /api/v1/coroot/api/v1/services
GET    /api/v1/coroot/api/v1/topology
POST   /api/v1/coroot/webhook
GET    /api/v1/debug/model-input-traces
GET    /api/v1/debug/model-input-traces/file
```

注意：`/api/v1/debug/model-input-traces` 是 LLM Prompt Trace 数据，前端可以保留现有页面，但不能把它作为用户业务 DebugEvent 的 trace backend。

建议新增：

```text
GET    /api/v1/observability/evidence
GET    /api/v1/observability/source-health
GET    /api/v1/debug-events
GET    /api/v1/debug-events/{id}/waterfall
GET    /api/v1/debug-events/{id}/verification-compare
```

## 15. 状态与数据流

```text
User query / route query / debug launcher
  -> observability API client
  -> normalize Observability view-model
  -> page state
  -> Evidence / Coroot / Debug / Waterfall / Quality / Health tabs
  -> Detail Drawer
```

Debug Mode 数据流：

```text
Create DebugEvent
  -> receive signed debug marker
  -> user reproduces target action
  -> frontend/gateway attaches traceparent and baggage
  -> trace backend and Coroot collect data
  -> Observability creates EvidenceRef and quality gate
  -> DebugCase shows waterfall and candidate RCA
  -> Case / OpsGraph / ActionProposal / Verification consume evidence summary
```

要求：

- 页面不直接调用裸 `fetch`。
- 页面不私自保存敏感 trace payload。
- Source Health、Evidence Feed、DebugEvent、Trace Waterfall 的 loading/error 状态相互隔离。
- URL query 是当前上下文、tab、时间窗口、质量过滤和来源过滤的事实来源。
- 同一个 evidence id、trace id、debugEvent id 在页面内复用缓存，避免同屏不一致。

## 16. 错误、空态与加载态

错误态：

- Coroot 未配置：显示配置缺口，不影响 Debug Trace。
- Coroot 不可用：保留 trace 证据，RCA 区显示缺少 Coroot 指标。
- Trace backend 不可用：DebugEvent 可创建，但状态为 `captured_without_trace_backend`。
- Debug header 验证失败：显示安全审计摘要，不展示业务 trace。
- 用户关闭页面：DebugEvent 保留，显示后台拉取 trace 的 TTL。
- redaction failed：证据不能进入 PromptCompiler，只显示受限引用。

空态：

- 无 Evidence：提示调整时间窗口、来源或 case/service 过滤。
- 无 DebugEvent：展示创建 DebugEvent 入口。
- 无 trace waterfall：显示 trace 缺失原因和补采建议。
- 无 Coroot service：显示 Coroot 未配置、无权限或当前环境无数据。
- 无 verification compare：提示需要完成修复后复现同一动作。

加载态：

- Header、Source Health Strip、Context Bar 不随 tab 切换闪烁。
- Waterfall 使用稳定行高和 skeleton，避免 span 加载时布局跳动。
- Coroot iframe 单独 lazy load，不阻塞结构化 Coroot 数据展示。

## 17. 安全与治理显示规则

- 前端禁止渲染请求体、cookie、token、password、authorization header、用户输入原文。
- SQL、cache、MQ operation 只显示 signature，不显示参数值。
- `x-aiops-debug` 只显示签名状态和 TTL，不显示签名密钥或完整 token。
- `x-aiops-session` 只显示 redacted session hash。
- `redactionStatus=failed` 的证据不能进入 PromptCompiler，UI 必须显示阻断原因。
- `traceCompleteness=missing` 时不能生成自动化修复建议，只能生成补充观测计划。
- Coroot RCA 只能展示为证据或假设来源，不能展示为已确认根因。
- 自动修复入口必须进入 Governed Action Plane，不能从 Observability 页面直接执行。
- Debug Mode 必须有可见 TTL 和停止入口，避免长期采集。

## 18. 验收标准

- `/observability` 能展示 Source Health、Evidence Feed、Coroot Services、Debug Events、Trace Waterfall、Evidence Quality。
- `/coroot` 保持可用，并复用结构化 Coroot service、SLO、RCA、topology 展示组件或跳转到 Observability。
- 用户可以创建 DebugEvent，并看到 signed debug marker、trace capture 状态、TTL 和捕获结果。
- DebugCase 能展示 routeName、actionName、trace id、waterfall、slow span、Coroot RCA、证据质量和修复后对比。
- trace 缺失、backend span 缺失、middleware span 缺失、redaction failed、time window mismatch 都能在 UI 中明确阻断对应下游能力。
- Evidence Feed 中每条证据都能查看 source、rawRef、summary、quality、redactionStatus、usage 和关联实体。
- Coroot RCA 不会被展示成已确认根因。
- 页面不渲染请求体、cookie、token、password、authorization header 或用户输入原文。
- 所有请求经过 `web/src/api/observability.ts` 或迁移后的 `web/src/api/coroot.ts`，页面不保留裸 `fetch`。
- Case、OpsGraph、AI Reasoning、Verification 能消费 Observability 的 evidence summary 和 quality gate。
