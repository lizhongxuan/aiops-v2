# aiops-v2 Observability, Coroot & Debug Trace 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-04a-observability-debug-trace-frontend-design.zh.md](2026-05-11-aiops-v2-04a-observability-debug-trace-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-04-observability-debug-trace-module-design.zh.md](2026-05-11-aiops-v2-04-observability-debug-trace-module-design.zh.md)

## 1. 目标

把现有 `CorootOverviewPage.tsx` 和零散 trace 页面升级为 Observability 前端工作台：支持可观测证据流、Coroot 结构化视图、用户侧 DebugEvent、Trace Waterfall、Evidence Quality、Source Health，以及 Case、OpsGraph、AI Reasoning、Verification 的嵌入式复用。

## 2. 实施顺序

```text
Observability API view-model
  -> Evidence Quality 和脱敏规则
  -> Observability 工作台骨架
  -> Coroot 结构化面板
  -> Evidence Feed
  -> DebugEvent 创建和列表
  -> DebugCase 和 Trace Waterfall
  -> Source Health
  -> Case / OpsGraph / Verification 集成
  -> 测试和视觉校验
```

先做结构化证据和质量门禁，再做 DebugEvent 修复建议。不要先把 Coroot iframe 放大成新页面，也不要让前端自行判断最终根因。

## 3. 文件地图

新增：

- `web/src/api/observability.ts`：Observability API client、类型、DebugEvent、Trace Summary、Evidence Feed、Source Health。
- `web/src/api/observability.test.ts`：Observability normalize、质量门禁和敏感字段裁剪测试。
- `web/src/api/coroot.ts`：迁移 `web/src/api/coroot.js`，保留兼容导出。
- `web/src/pages/ObservabilityPage.tsx`：统一 Observability 工作台。
- `web/src/pages/DebugEventPage.tsx`：DebugCase 详情页，MVP 可由 `ObservabilityPage` 的 detail tab 承载。
- `web/src/components/observability/SourceHealthStrip.tsx`：数据源健康条。
- `web/src/components/observability/ObservabilityContextBar.tsx`：case、service、trace、debugEvent、route、action 查询栏。
- `web/src/components/observability/ObservabilitySummaryStrip.tsx`：证据数、质量缺口、活跃 DebugEvent、RCA、SLO 指标。
- `web/src/components/observability/EvidenceFeedTable.tsx`：ObservationEvidence 表格。
- `web/src/components/observability/EvidenceDetailDrawer.tsx`：证据、span、RCA、Coroot raw ref 详情。
- `web/src/components/observability/EvidenceQualityBadge.tsx`：质量状态和门禁原因。
- `web/src/components/observability/EvidenceUsageMatrix.tsx`：Prompt、RCA、Automation、Verification 使用资格。
- `web/src/components/observability/CorootServiceTable.tsx`：Coroot 服务表。
- `web/src/components/observability/CorootServiceDetailPanel.tsx`：metrics、SLO、RCA、timeline、EvidenceRef。
- `web/src/components/observability/CorootTopologyPanel.tsx`：结构化 topology。
- `web/src/components/observability/CorootRawFrame.tsx`：延迟加载 Coroot iframe。
- `web/src/components/observability/DebugModeLauncher.tsx`：创建 DebugEvent 的表单。
- `web/src/components/observability/DebugEventTable.tsx`：DebugEvent 列表。
- `web/src/components/observability/DebugTraceWaterfall.tsx`：frontend、gateway、backend、middleware、db、mq、cache waterfall。
- `web/src/components/observability/DebugQualityGate.tsx`：DebugCase 第一屏质量门禁。
- `web/src/components/observability/DebugRcaPanel.tsx`：Coroot RCA、AI hypothesis、反证和下一步。
- `web/src/components/observability/DebugVerificationCompare.tsx`：修复前后同一 action trace 对比。
- `web/src/components/observability/observabilityViewModels.ts`：排序、分组、质量门禁、敏感字段裁剪和状态文案。
- `web/src/components/observability/observabilityViewModels.test.ts`：view-model 单测。
- `web/src/components/observability/observabilityComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/CorootOverviewPage.tsx`：迁移到 `web/src/api/coroot.ts`，复用 observability 组件，去掉页面内裸 `fetch`。
- `web/src/pages/IncidentWorkbenchPage.tsx`：复用 Evidence Quality Summary、Debug Trace Summary、Coroot Evidence Summary。
- `web/src/pages/OpsGraphPage.tsx`：增加 EvidenceRef 详情跳转到 Observability。
- `web/src/pages/ERPHealthPage.tsx`：业务慢请求调试入口跳到 DebugEvent 创建。
- `web/src/pages/PromptTracePage.tsx`：保持 Prompt Trace 定位，但迁移裸 `fetch` 到专用 API client 时避免和 Observability DebugEvent 混用。
- `web/src/app/navigation.ts`：增加 `/observability` 和 `/observability/debug-events/:debugEventId` 导航/路由元数据。
- `web/src/router.tsx`：注册 Observability 和 DebugEvent 页面。
- `web/src/pages/complexPages.test.tsx`：补充 `/observability`、`/coroot`、DebugCase 和敏感字段裁剪测试。

## 4. Task 1：建立 Observability API 与类型

- [ ] 新增 `web/src/api/observability.ts`。
- [ ] 定义 `TraceContextView`、`DebugEventView`、`ObservationEvidenceView`、`EvidenceQualityView`、`TraceWaterfallSegmentView`、`SourceHealthView`、`EvidenceUsageView`、`CorootServiceView`、`CorootRcaView`。
- [ ] 实现 `listObservationEvidence(params)`，请求 `GET /api/v1/observability/evidence`。
- [ ] 实现 `listSourceHealth(params)`，请求 `GET /api/v1/observability/source-health`。
- [ ] 实现 `createDebugEvent(payload)`，请求 `POST /api/v1/debug-events`。
- [ ] 实现 `listDebugEvents(params)`，请求 `GET /api/v1/debug-events`。
- [ ] 实现 `getDebugEvent(debugEventId)`，请求 `GET /api/v1/debug-events/{id}`。
- [ ] 实现 `attachDebugTrace(debugEventId, payload)`，请求 `POST /api/v1/debug-events/{id}/attach-trace`。
- [ ] 实现 `analyzeDebugEvent(debugEventId)`，请求 `POST /api/v1/debug-events/{id}/analyze`。
- [ ] 实现 `getTraceSummary(traceId)`，请求 `GET /api/v1/traces/{trace_id}/summary`。
- [ ] 实现 `getDebugWaterfall(debugEventId)` 和 `getDebugVerificationCompare(debugEventId)`。
- [ ] normalize 时补齐 `quality.blockingReasons`、`usage`、`redactionStatus`、`spanCoverage` 默认值。
- [ ] 新增 `web/src/api/observability.test.ts`，覆盖正常、缺 trace、脱敏失败、旧字段兼容。
- [ ] 运行 `npm --prefix web test -- observability.test.ts`。

## 5. Task 2：迁移 Coroot API

- [ ] 新增 `web/src/api/coroot.ts`，迁移 `fetchCorootConfig`、`fetchCorootServices`、`fetchCorootJson`。
- [ ] 新增 `fetchCorootServiceMetrics(serviceId)`、`fetchCorootServiceRca(serviceId)`、`fetchCorootTopology(params)`。
- [ ] 保留 `web/src/api/coroot.js` 兼容导出，或统一修改调用点后删除。
- [ ] API client 使用 `httpClient`，页面不直接调用裸 `fetch`。
- [ ] 为 Coroot 未配置、上游 403、上游 502、非 JSON iframe 响应分别 normalize 错误。
- [ ] 新增或扩展测试，覆盖 Coroot config 和 service list normalize。

## 6. Task 3：实现 Evidence Quality 和脱敏 view-model

- [ ] 新增 `observabilityViewModels.ts`。
- [ ] 实现 `computeQualityGate(quality)`，返回 `canUseInPrompt`、`canUseForRca`、`canSuggestAutomation`、`canUseForVerification` 和阻断原因。
- [ ] 实现 `redactEvidenceSummary(evidence)`，过滤 request body、cookie、token、password、authorization header、用户输入原文。
- [ ] 实现 `rankSlowSpans(segments)`，按 slow/error、duration、segment 排序。
- [ ] 实现 `groupEvidenceBySource(evidence)` 和 `groupSegmentsByLayer(segments)`。
- [ ] 新增 `observabilityViewModels.test.ts`，覆盖 trace missing、middleware span missing、redaction failed、time window mismatch、敏感字段裁剪。
- [ ] 运行 `npm --prefix web test -- observabilityViewModels.test.ts`。

## 7. Task 4：新增 Observability 工作台骨架

- [ ] 新增 `ObservabilityPage.tsx`。
- [ ] 新增 `SourceHealthStrip.tsx`、`ObservabilityContextBar.tsx`、`ObservabilitySummaryStrip.tsx`、`EvidenceDetailDrawer.tsx`。
- [ ] 页面布局包含 Header、Source Health Strip、Context Bar、Summary Strip、Tabs、Detail Drawer。
- [ ] Tabs 包含 Evidence Feed、Coroot Services、Debug Events、Trace Waterfall、Evidence Quality、Source Health。
- [ ] URL query 支持 `caseId`、`serviceId`、`traceId`、`debugEventId`、`routeName`、`actionName`、`tab`、`environment`、`timeWindow`、`source`、`quality`、`redaction`。
- [ ] 每个 tab 的 loading 和 error 状态独立。
- [ ] 修改 `navigation.ts` 和 `router.tsx`，注册 `/observability`。
- [ ] 单测覆盖 `/observability?traceId=...&tab=waterfall` 能正确落点。

## 8. Task 5：实现 Evidence Feed

- [ ] 新增 `EvidenceFeedTable.tsx`。
- [ ] 表格列包括 Evidence、Source、Entity、Time Window、Quality、Redaction、Usage、Action。
- [ ] 新增 `EvidenceQualityBadge.tsx`。
- [ ] 新增 `EvidenceUsageMatrix.tsx`。
- [ ] 点击证据打开 `EvidenceDetailDrawer`，展示 source、rawRef、observationType、summary、timeWindow、entityRefs、quality、redactionStatus。
- [ ] `restricted` 和 `failed` 证据不能渲染正文。
- [ ] Action 支持加入 case、打开 OpsGraph、打开 Coroot raw ref。
- [ ] 单测覆盖受限证据、使用资格、rawRef 展示。

## 9. Task 6：实现 Coroot Services 结构化面板

- [ ] 新增 `CorootServiceTable.tsx`。
- [ ] 新增 `CorootServiceDetailPanel.tsx`。
- [ ] 新增 `CorootTopologyPanel.tsx`。
- [ ] 新增 `CorootRawFrame.tsx`。
- [ ] `CorootOverviewPage.tsx` 改为复用这些组件，保留 `/coroot` 入口。
- [ ] 服务表列包括 Service、Status、SLO、Metrics、Evidence、Links。
- [ ] 服务详情展示 overview、latency、error rate、throughput、CPU、memory、network、disk、SLO、Coroot RCA、timeline、topology dependencies、EvidenceRef。
- [ ] Coroot RCA 显示为证据或 hypothesis，不显示为已确认根因。
- [ ] iframe 放在 Coroot Raw 子区，并延迟加载。
- [ ] 单测覆盖 Coroot 未配置、服务详情、RCA 文案和 iframe lazy load。

## 10. Task 7：实现 DebugEvent 创建和列表

- [ ] 新增 `DebugModeLauncher.tsx`。
- [ ] 表单字段包括 Case、Target App、Route Name、Action Name、Environment、TTL、Capture Mode、Expected Slow Threshold。
- [ ] 创建成功后展示 DebugEvent id、signed debug marker 状态、traceparent 注入状态、TTL 和 capture 状态。
- [ ] 新增 `DebugEventTable.tsx`。
- [ ] 表格列包括 DebugEvent、Status、Trace、Frontend、Backend、Quality、Action。
- [ ] 支持 `created`、`armed`、`captured`、`captured_without_trace_backend`、`analyzing`、`diagnosis_ready`、`awaiting_approval`、`remediating`、`verifying`、`verified`、`failed`、`expired`、`header_rejected`。
- [ ] `header_rejected` 只显示安全审计摘要。
- [ ] 单测覆盖创建成功、TTL 显示、header rejected 裁剪。

## 11. Task 8：实现 DebugCase 和 Trace Waterfall

- [ ] 新增 `DebugEventPage.tsx`，或在 `ObservabilityPage.tsx` 中实现 `debug-detail` tab。
- [ ] 新增 `DebugQualityGate.tsx`。
- [ ] 新增 `DebugTraceWaterfall.tsx`。
- [ ] 新增 `DebugRcaPanel.tsx`。
- [ ] 新增 `DebugVerificationCompare.tsx`。
- [ ] DebugCase Header 展示 DebugEvent id、case id、routeName、actionName、trace id、status、quality summary、createdAt、capturedAt、analyzedAt。
- [ ] Waterfall 分层展示 frontend、gateway、backend、middleware、db、mq、cache、render。
- [ ] Segment 字段包括 Segment、Service/Resource、Operation、Duration、Start Offset、Status、Evidence。
- [ ] SQL、cache、MQ operation 只显示 signature，不显示参数值。
- [ ] `traceCompleteness=missing` 或 `redactionStatus=failed` 时禁用生成 ActionProposal，并显示原因。
- [ ] 单测覆盖慢 span 排名、缺 trace 阻断、敏感字段不出现在 DOM。

## 12. Task 9：实现 Source Health

- [ ] 在 `SourceHealthStrip.tsx` 中展示 Coroot、Trace Backend、Webhook、Gateway Debug Header、Evidence Ingestion。
- [ ] 增加 Source Health tab 明细视图。
- [ ] Coroot 明细展示 configured、baseUrl、iframeUrl、last successful request、service API、topology API、SLO API、webhook last received、recent errors。
- [ ] Trace Backend 明细展示 query API、lookup latency、sampling summary、last trace seen、missing span rate、retention。
- [ ] Gateway Debug Header 明细展示 signed marker、TTL、最近拒绝数、拒绝原因分布、session hash 状态。
- [ ] 不显示签名密钥、原始 session id、cookie 或 authorization header。
- [ ] 单测覆盖 not_configured、degraded、failed 三类状态。

## 13. Task 10：跨页面集成

- [ ] `IncidentWorkbenchPage.tsx` 复用 Evidence Quality Summary、Debug Trace Summary、Coroot Evidence Summary。
- [ ] Case 页面只显示摘要和跳转入口，不嵌入完整 Coroot iframe。
- [ ] `OpsGraphPage.tsx` 的 EvidenceRef 点击跳转 `/observability?evidenceId=...` 或 `/observability?traceId=...`。
- [ ] `ERPHealthPage.tsx` 增加“创建 DebugEvent”入口，带上 routeName、business capability 和 environment。
- [ ] Verification 页面或模块后续复用 `DebugVerificationCompare` 展示修复前后同一 action trace 对比。
- [ ] `PromptTracePage.tsx` 继续保留 `/debug/prompts`，页面文案明确这是 LLM Prompt Trace，不是用户业务 Debug Trace。
- [ ] 单测覆盖 Case、OpsGraph、ERP 的跳转 URL。

## 14. Task 11：测试与视觉检查

- [ ] 扩展 `web/src/pages/complexPages.test.tsx`，覆盖 `/observability`、`/coroot`、`/observability?tab=debug-events`、DebugCase。
- [ ] 新增 `observabilityComponents.test.tsx`，覆盖 SourceHealthStrip、EvidenceFeedTable、CorootServiceTable、DebugEventTable、DebugTraceWaterfall、DebugQualityGate。
- [ ] 运行 `npm --prefix web test -- observability.test.ts observabilityViewModels.test.ts observabilityComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含页面视觉变更，启动 dev server 并用浏览器截图检查 `/observability`、`/coroot`、DebugCase 的桌面和移动宽度。
- [ ] 检查 `/observability?traceId=...&tab=waterfall`、`/observability?debugEventId=...&tab=debug-detail`、`/observability?serviceId=...&tab=coroot` 能正确落点。

## 15. 交付检查

- [ ] Observability 页面不直接调用裸 `fetch`。
- [ ] Coroot 页面不直接调用裸 `fetch`。
- [ ] Coroot iframe 不是第一屏核心，结构化 service、SLO、RCA、topology 可独立展示。
- [ ] Evidence Feed 每条证据都展示 source、rawRef、summary、quality、redactionStatus、usage、entityRefs。
- [ ] Coroot RCA 不显示为已确认根因。
- [ ] `traceCompleteness=missing` 时禁止自动化修复建议。
- [ ] middleware span 缺失时不能断言 DB/MQ 根因。
- [ ] `redactionStatus=failed` 时证据不能进入 PromptCompiler。
- [ ] 页面不渲染请求体、cookie、token、password、authorization header、用户输入原文。
- [ ] Debug Mode 显示 TTL 和停止/过期状态。
- [ ] Case、OpsGraph、ERP、Verification 能复用 Observability 摘要组件或跳转到 `/observability`。
- [ ] `/debug/prompts` 保持 Prompt Trace 语义，不和业务 Debug Trace 混用。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
