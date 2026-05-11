# aiops-v2 Verification & Recovery 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-07a-verification-recovery-frontend-design.zh.md](2026-05-11-aiops-v2-07a-verification-recovery-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-07-verification-recovery-module-design.zh.md](2026-05-11-aiops-v2-07-verification-recovery-module-design.zh.md)

## 1. 目标

把 Verification & Recovery 从 case 页面里的附属信息升级为生产恢复证明前端：支持跨 case recovery queue、VerificationSpec、VerificationRecord、Observation Window、Evidence Compare、Rollback Verification、Manual Confirmation、Recovery Decision、Case Close Gate，并和 Incident、Execution、Observability、Middleware Repair、Postmortem、Experience Pack 形成闭环。

## 2. 实施顺序

```text
Verification API view-model
  -> Recovery Queue / Overview
  -> Case Recovery Header and Workspace
  -> VerificationSpec Designer
  -> VerificationRecord Detail
  -> Observation Window
  -> Evidence Compare
  -> Manual Confirmation
  -> Rollback Verification
  -> Recovery Decision and Close Gate
  -> Cross-module embedding
  -> Tests and visual checks
```

先建立统一 `verificationRecovery.ts` 和 view-model，再做页面。不要在 Incident、Runner、Coroot 页面各自拼接私有验证状态。

## 3. 文件地图

新增：

- `web/src/api/verificationRecovery.ts`：VerificationSpec、VerificationRecord、RecoveryStatus、ObservationWindow、RecoveryDecision、CloseGate API client 和类型。
- `web/src/api/verificationRecovery.test.ts`：normalize、fallback、权限裁剪和默认值测试。
- `web/src/pages/VerificationOverviewPage.tsx`：Verification & Recovery 总览。
- `web/src/pages/VerificationRecordPage.tsx`：VerificationRecord 详情页。
- `web/src/pages/VerificationSpecPage.tsx`：VerificationSpec 详情和编辑页。
- `web/src/pages/CaseRecoveryPage.tsx`：单 case recovery 工作台。
- `web/src/components/verification/RecoveryStrip.tsx`：recovery status 统计和过滤。
- `web/src/components/verification/VerificationSourceHealthStrip.tsx`：Coroot、ERP、Trace、中间件、K8s、Host、Manual source health。
- `web/src/components/verification/RecoveryQueueTable.tsx`：跨 case 恢复队列表。
- `web/src/components/verification/RecoveryStatusRail.tsx`：单 case 恢复状态流。
- `web/src/components/verification/RecoverySummaryPanel.tsx`：恢复摘要和阻塞项。
- `web/src/components/verification/VerificationSpecDesigner.tsx`：验证计划编辑器。
- `web/src/components/verification/CriteriaBuilder.tsx`：结构化 success/failure/rollback criteria builder。
- `web/src/components/verification/VerificationRecordHeader.tsx`：验证记录头部。
- `web/src/components/verification/VerificationCheckList.tsx`：checks、criteria、baseline/current/delta。
- `web/src/components/verification/VerificationEvidencePanel.tsx`：证据引用和可用性。
- `web/src/components/verification/ObservationWindowPanel.tsx`：观察窗口状态机。
- `web/src/components/verification/EvidenceComparePanel.tsx`：修复前后对比容器。
- `web/src/components/verification/CorootVerificationCompare.tsx`：Coroot SLO 和指标对比。
- `web/src/components/verification/BusinessVerificationCompare.tsx`：ERP 业务验证对比。
- `web/src/components/verification/TraceVerificationCompare.tsx`：Debug Trace before/after 对比。
- `web/src/components/verification/MiddlewareVerificationCompare.tsx`：PG/Redis/MQ 健康对比。
- `web/src/components/verification/HostK8sVerificationCompare.tsx`：主机和 K8s 验证对比。
- `web/src/components/verification/RollbackVerificationPanel.tsx`：回滚验证。
- `web/src/components/verification/ManualConfirmationQueue.tsx`：人工确认队列。
- `web/src/components/verification/ManualConfirmationDialog.tsx`：人工确认表单。
- `web/src/components/verification/RecoveryDecisionDialog.tsx`：恢复决策。
- `web/src/components/verification/CloseGatePanel.tsx`：关闭门禁。
- `web/src/components/verification/verificationViewModels.ts`：排序、状态解释、覆盖率、阻塞项、证据可用性、close gate 计算。
- `web/src/components/verification/verificationViewModels.test.ts`：view-model 单测。
- `web/src/components/verification/verificationComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/IncidentWorkbenchPage.tsx`：Case Header 增加 RecoveryStatus；Verification tab 复用 `verification` 组件。
- `web/src/pages/RunbookDetailPage.tsx`：Runbook 验证项链接到 VerificationSpec / VerificationRecord。
- `web/src/pages/WorkflowRunDetailPage.tsx`：Workflow End / Run Detail 嵌入 verification refs、observation window、recovery contribution；不要求修改现有 Runner 页面。
- `web/src/pages/CorootOverviewPage.tsx` 或 Observability 页面：DebugEvent 修复后跳转 Evidence Compare。
- `web/src/pages/PostmortemPage.tsx`：复盘引用 VerificationRecord 和 RecoveryDecision。
- `web/src/pages/ApprovalManagementPage.tsx`：高风险 ActionProposal 展示 missing VerificationSpec blocker。
- `web/src/app/navigation.ts`：增加 `/verification`。
- `web/src/router.tsx`：注册 Verification 页面路由。
- `web/src/pages/complexPages.test.tsx`：补充 Verification 路由和关键状态测试。

## 4. Task 1：建立 Verification Recovery API 与类型

- [ ] 新增 `web/src/api/verificationRecovery.ts`。
- [ ] 定义 `VerificationType`：`slo`、`business`、`trace`、`middleware`、`host`、`k8s`、`manual`、`rollback`。
- [ ] 定义 `VerificationStatus`：`pending`、`running`、`passed`、`failed`、`inconclusive`、`skipped`。
- [ ] 定义 `RecoveryStatus`：`not_started`、`mitigation_in_progress`、`mitigated`、`verifying`、`recovered`、`partially_recovered`、`failed`、`rolled_back`、`requires_manual_followup`。
- [ ] 定义 `VerificationSpecView`、`VerificationRecordView`、`VerificationCheckView`、`ObservationWindowView`、`RecoveryQueueItemView`、`RecoveryDecisionView`、`CloseGateView`、`ManualConfirmationView`、`EvidenceCompareView`。
- [ ] 实现 `listRecoveryQueue(params)`，请求 `GET /api/v1/recovery-queue`，后端未提供时 fallback 到 `GET /api/v1/verifications` 聚合。
- [ ] 实现 `listVerifications(params)`、`createVerification(payload)`、`getVerification(id)`、`updateVerification(id, payload)`。
- [ ] 实现 `runVerification(id)`、`rerunVerificationCheck(id, checkId)`、`markVerificationFailed(id, payload)`。
- [ ] 实现 `getVerificationSpec(id)`、`createVerificationSpec(payload)`、`updateVerificationSpec(id, payload)`。
- [ ] 实现 `getCaseRecoveryStatus(caseId)`、`getCaseVerificationSummary(caseId)`、`getCaseCloseGate(caseId)`。
- [ ] 实现 `createRecoveryDecision(caseId, payload)`。
- [ ] 实现 `getEvidenceCompare(verificationId)`、`getObservationWindow(verificationId)`。
- [ ] 实现 `createManualConfirmation(verificationId, payload)`、`submitManualConfirmation(confirmationId, payload)`。
- [ ] normalize 时补齐 `checks`、`evidenceRefs`、`failedPoint`、`nextRecommendation`、`observationWindow`、`sourceHealth`、`closeGate` 默认值。
- [ ] 受限证据 normalize 后只保留 id、source、observedAt、restrictionReason。
- [ ] 新增 `verificationRecovery.test.ts` 覆盖 canonical 字段、fallback 字段、受限证据裁剪和 source unavailable 状态。
- [ ] 运行 `npm --prefix web test -- verificationRecovery.test.ts`。

## 5. Task 2：实现 Verification view-model

- [ ] 新增 `web/src/components/verification/verificationViewModels.ts`。
- [ ] 实现 `getRecoveryStatusLabel(status)` 和 `getRecoveryStatusDescription(status)`。
- [ ] 实现 `sortRecoveryQueue(items)`，排序优先级为 failed、observation expired、required inconclusive、manual expired、partially_recovered、verifying、recovered。
- [ ] 实现 `summarizeVerificationCoverage(records)`，输出 required passed、failed、inconclusive、optional passed、manual pending。
- [ ] 实现 `derivePrimaryBlocker(queueItem)`，识别 missing VerificationSpec、source unavailable、failed check、manual pending、rollback failed、close gate blocked。
- [ ] 实现 `isRecoveredAllowed(summary)`，仅在 required checks passed、observation completed、业务验证通过或授权人工确认后返回 true。
- [ ] 实现 `summarizeEvidenceAvailability(evidenceRefs)`，输出 usable、restricted、redacted、time_window_mismatch、source_unavailable、low_confidence 计数。
- [ ] 实现 `buildCloseGateReasons(closeGate)`，生成用户可读阻塞原因。
- [ ] 新增 `verificationViewModels.test.ts` 覆盖排序、恢复门禁、证据可用性和 close gate 文案。
- [ ] 运行 `npm --prefix web test -- verificationViewModels.test.ts`。

## 6. Task 3：实现 Verification Overview

- [ ] 新增 `web/src/pages/VerificationOverviewPage.tsx`。
- [ ] 新增 `RecoveryStrip.tsx`。
- [ ] 新增 `VerificationSourceHealthStrip.tsx`。
- [ ] 新增 `RecoveryQueueTable.tsx`。
- [ ] 页面 Header 展示环境、时间窗口、用户角色、刷新、创建验证计划。
- [ ] Recovery Strip 展示 verifying、mitigated、recovered、partially_recovered、failed、rolled_back、requires_manual_followup，并支持点击过滤。
- [ ] Source Health 展示 Coroot、ERP、Trace Backend、Middleware Probe、K8s、Host Agent、Manual Confirmation。
- [ ] Recovery Queue 支持 recoveryStatus、verificationStatus、caseType、severity、environment、source、verificationType、blockedReason、owner、observationWindowState、failedPoint、requiresManualConfirmation 筛选。
- [ ] 表格列包括 Case、Recovery、Source Action、Verification、Observation、Evidence、Failed Point、Next。
- [ ] 行操作支持打开 record、打开 case recovery、运行验证、请求人工确认、打开 recovery decision。
- [ ] 注册 `/verification` 路由和导航入口。
- [ ] 单测覆盖过滤、排序、source unavailable、failed record 的 Next 操作。

## 7. Task 4：实现 Case Recovery Header 与 Workspace

- [ ] 新增 `web/src/pages/CaseRecoveryPage.tsx`。
- [ ] 新增 `RecoveryStatusRail.tsx`。
- [ ] 新增 `RecoverySummaryPanel.tsx`。
- [ ] 修改 `IncidentWorkbenchPage.tsx`，Case Header 同时展示 `CaseStatus` 和 `RecoveryStatus`。
- [ ] Header 展示 caseId、case type、severity、affected business capability、affected service/middleware/host、owner、active VerificationRecord、observation window、close gate。
- [ ] Recovery Status Rail 展示 not_started、mitigation_in_progress、mitigated、verifying、recovered，并支持 failed、rolled_back、requires_manual_followup 分支。
- [ ] Tabs 包含 Recovery Summary、Verification Plan、Verification Records、Evidence Compare、Observation Window、Rollback、Manual Confirmation、Decision & Close Gate。
- [ ] Recovery Summary 展示 requiredChecks 覆盖率、技术验证、业务验证、trace 对比、中间件健康、回滚可用性和 close gate。
- [ ] 路由 `/verification/cases/:caseId` 加载单 case recovery view。
- [ ] 单测覆盖 workflow 成功但 verification failed 时 RecoveryStatus 不显示 recovered。

## 8. Task 5：实现 VerificationSpec Designer

- [ ] 新增 `VerificationSpecPage.tsx`。
- [ ] 新增 `VerificationSpecDesigner.tsx`。
- [ ] 新增 `CriteriaBuilder.tsx`。
- [ ] Source 区域支持 ActionProposal、Workflow node、Workflow End、Runbook step、RepairPlan、AI verification_plan、DebugCase、Middleware Repair、manual。
- [ ] Scope 区域支持 service、business capability、tenant、middleware、host、k8s workload、trace route。
- [ ] Required Checks 支持 slo、business、trace、middleware、host、k8s、manual、rollback。
- [ ] Optional Checks 支持不阻塞恢复判断的补强验证。
- [ ] Observation Window 支持 duration、sampling interval、start condition、timeout behavior。
- [ ] Success Criteria、Failure Criteria、Rollback Criteria 使用结构化 builder，不让用户输入自由表达式作为唯一来源。
- [ ] Evidence Policy 支持 required source、timeWindow、redactionStatus、confidence。
- [ ] 高风险 ActionProposal 缺少 VerificationSpec 时，Approval 页面展示阻断原因和创建入口。
- [ ] 单测覆盖 required check、rollback criteria、manual override 和 high-risk blocker。

## 9. Task 6：实现 VerificationRecord Detail

- [ ] 新增 `web/src/pages/VerificationRecordPage.tsx`。
- [ ] 新增 `VerificationRecordHeader.tsx`。
- [ ] 新增 `VerificationCheckList.tsx`。
- [ ] 新增 `VerificationEvidencePanel.tsx`。
- [ ] Header 展示 record id、caseId、source、type、status、RecoveryStatus contribution、startedAt、finishedAt、observation window、owner、failedPoint、nextRecommendation。
- [ ] Header 动作支持运行验证、重跑失败 check、请求人工确认、标记 failed、打开 Recovery Decision、创建 Experience candidate、导出证据引用。
- [ ] Check List 展示 check、status、criteria、evidence、baseline、current、delta、failedPoint、next。
- [ ] Evidence Panel 展示 Coroot SLO、ERP business、Debug Trace、Middleware probe、Host/K8s、Workflow outputs、ToolResult、Manual confirmation。
- [ ] 受限证据只展示 id、source、observedAt、restriction reason。
- [ ] 注册 `/verification/records/:verificationId`。
- [ ] 单测覆盖 failedPoint、nextRecommendation、restricted evidence、rerun check。

## 10. Task 7：实现 Observation Window

- [ ] 新增 `ObservationWindowPanel.tsx`。
- [ ] 支持状态 waiting_for_action、smoke_check_running、observation_waiting、trend_check_running、business_validation_running、decision_required、completed、failed、expired。
- [ ] 展示窗口开始条件、窗口时长、已过时间、下一次采样时间、完成 checks、未完成 checks、窗口内失败事件、当前趋势、rollbackCriteria 是否触发。
- [ ] 不同场景显示默认窗口说明：配置 reload、服务重启、扩容、PG failover、数据修复。
- [ ] 运行中使用轻量 polling 刷新 summary，不轮询大证据正文。
- [ ] expired 状态给出 nextRecommendation，并阻止自动 recovered。
- [ ] 单测覆盖 expired、rollbackCriteria triggered、business validation pending。

## 11. Task 8：实现 Evidence Compare

- [ ] 新增 `EvidenceComparePanel.tsx`。
- [ ] 新增 `CorootVerificationCompare.tsx`。
- [ ] 新增 `BusinessVerificationCompare.tsx`。
- [ ] 新增 `TraceVerificationCompare.tsx`。
- [ ] 新增 `MiddlewareVerificationCompare.tsx`。
- [ ] 新增 `HostK8sVerificationCompare.tsx`。
- [ ] Coroot 对比展示 latency p50/p95/p99、error rate、throughput、resource saturation、SLO burn、topology health。
- [ ] ERP 对比展示业务任务恢复、订单状态推进、租户 SLA、业务错误码下降。
- [ ] Debug Trace 对比展示 before trace id、after trace id、user action、route、TTFB、slow span、database span、middleware span、trace completeness。
- [ ] Middleware 对比展示 PG replication lag、lock wait、connection saturation、disk/WAL；Redis memory、evictions、latency、replication；MQ lag、consumer health、broker state。
- [ ] Host/K8s 对比展示 systemd、pod readiness、rollout、events、CPU、memory、disk、network。
- [ ] 每个维度展示 passed、failed、inconclusive、not_applicable 和 evidence links。
- [ ] after trace 缺失时状态显示 inconclusive，不能自动标记 recovered。
- [ ] 单测覆盖 trace 缺失、中间件 failed、ERP pending 和 Coroot degraded。

## 12. Task 9：实现 Manual Confirmation

- [ ] 新增 `ManualConfirmationQueue.tsx`。
- [ ] 新增 `ManualConfirmationDialog.tsx`。
- [ ] 列表列包括 Confirmation、Role、Reason、Evidence、Status、Validity、Action。
- [ ] Dialog 展示当前 RecoveryStatus、自动 checks、未通过或 inconclusive checks、人工确认作用范围、对 close gate 的影响。
- [ ] 表单字段包括 decision、summary、evidenceRefs、scope、validity、riskAccepted、comment。
- [ ] 支持 confirmed、rejected、need_more_evidence。
- [ ] 普通用户只能确认自己可见业务范围，不能覆盖 DBA 或 SRE 技术验证。
- [ ] expired confirmation 必须重新确认。
- [ ] 单测覆盖权限裁剪、need_more_evidence、expired、partial scope。

## 13. Task 10：实现 Rollback Verification

- [ ] 新增 `RollbackVerificationPanel.tsx`。
- [ ] 展示原动作、回滚动作、触发 rollbackCriteria、rollback ActionProposal、rollback WorkflowRun、rollback VerificationRecord、rollback status。
- [ ] 展示原动作是否撤销、服务是否可用、数据是否一致、业务是否未恶化、HostLease 是否释放、审批状态是否关闭、后续手工事项。
- [ ] rollback VerificationRecord 失败时显示 failedPoint 和 nextRecommendation。
- [ ] rollback failed 时 RecoveryStatus 进入 failed 或 requires_manual_followup。
- [ ] 生成 EvidenceRef 和 Experience candidate 入口。
- [ ] 单测覆盖 rollback passed、rollback failed、HostLease 未释放、数据一致性 inconclusive。

## 14. Task 11：实现 Recovery Decision 与 Close Gate

- [ ] 新增 `RecoveryDecisionDialog.tsx`。
- [ ] 新增 `CloseGatePanel.tsx`。
- [ ] Dialog 展示 caseId、current CaseStatus、current RecoveryStatus、required checks summary、failed checks、inconclusive checks、manual confirmations、rollback status、postmortem/experience candidate 状态。
- [ ] 决策支持 mark_mitigated、mark_recovered、mark_partially_recovered、mark_failed、trigger_rollback、mark_rolled_back、requires_manual_followup。
- [ ] mark_recovered 必须校验 required checks passed、无 required failed、observation window completed、业务验证通过或授权 manual confirmation、无未收敛 rollback。
- [ ] 人工覆盖必须填写覆盖原因、风险接受者、evidenceRefs、有效期、follow-up。
- [ ] Close Gate 展示 VerificationRecords、pending approval、executing workflow、active HostLease、failed rollback、missing postmortem evidence、unreviewed experience candidate。
- [ ] Case close 按钮只有 close gate 通过或授权覆盖后可用。
- [ ] 单测覆盖 recovered blocker、manual override、close gate blocked、trigger rollback。

## 15. Task 12：跨模块嵌入集成

- [ ] 修改 `IncidentWorkbenchPage.tsx`，Verification tab 复用 Recovery Summary、Verification Records、Evidence Compare、Decision & Close Gate。
- [ ] 修改 `ApprovalManagementPage.tsx`，高风险 ActionProposal 显示 VerificationSpec、expectedEffect、rollbackCriteria 和 missing verification blocker。
- [ ] 修改 `WorkflowRunDetailPage.tsx` 或对应 Execution Run Detail 组件，Workflow End / Run Detail 显示 verification refs、observation window、recovery contribution；不要求修改现有 Runner 页面。
- [ ] 修改 `RunbookDetailPage.tsx`，Runbook 验证项链接到 VerificationSpec / VerificationRecord。
- [ ] 修改 Observability / DebugEvent 页面，修复后展示 before/after trace 对比和 VerificationRecord 链接。
- [ ] 修改 Middleware Repair 页面，嵌入 PG/Redis/MQ 健康验证和 rollback verification。
- [ ] 修改 `PostmortemPage.tsx`，复盘引用 VerificationRecord、RecoveryDecision 和 failedPoint。
- [ ] 修改 Experience Pack 入口，成功和失败验证都能生成候选，但必须带禁用条件和适用范围。
- [ ] 单测覆盖从 Case、Approval、Workflow Run、DebugEvent、Postmortem 打开 verification 链接。

## 16. Task 13：路由、导航和权限

- [ ] 修改 `web/src/app/navigation.ts`，增加 Verification & Recovery 导航项。
- [ ] 修改 `web/src/router.tsx`，注册 `/verification`、`/verification/records/:verificationId`、`/verification/specs/:specId`、`/verification/cases/:caseId`、`/verification/recovery-decisions/:decisionId`。
- [ ] 普通 viewer 只能查看可见证据摘要。
- [ ] operator 可以运行验证和请求人工确认。
- [ ] recovery_reviewer 可以创建 RecoveryDecision。
- [ ] admin 可以配置 VerificationSpec 和覆盖策略。
- [ ] 无权限时不渲染敏感指标、业务数据、rawRef、SecretRef。
- [ ] 单测覆盖权限不足时按钮禁用和敏感字段隐藏。

## 17. Task 14：事件、轮询和一致性

- [ ] 在 `verificationRecovery.ts` 中实现 summary polling helper。
- [ ] Observation Window 运行中只刷新 record summary、window state、check status。
- [ ] 大证据正文只在用户打开 Evidence Panel 时加载。
- [ ] 接入统一事件时支持 verification.started、verification.check.completed、observation.window.updated、recovery.decision.created、rollback.verified。
- [ ] 页面刷新或重进后能从 API 恢复当前状态，不依赖内存态。
- [ ] 同一 VerificationRecord 的 run / rerun 操作要显示 idempotency 或 running guard。
- [ ] 单测覆盖刷新恢复、重复点击 run 被禁用、polling 错误保留旧数据。

## 18. Task 15：测试与视觉检查

- [ ] 新增 `verificationRecovery.test.ts`。
- [ ] 新增 `verificationViewModels.test.ts`。
- [ ] 新增 `verificationComponents.test.tsx`。
- [ ] 扩展 `web/src/pages/complexPages.test.tsx` 覆盖 `/verification`、`/verification/records/:verificationId`、`/verification/cases/:caseId`。
- [ ] 新增或扩展 Playwright 用例，覆盖桌面和移动宽度下的 recovery queue、record detail、decision dialog。
- [ ] 运行 `npm --prefix web test -- verificationRecovery.test.ts verificationViewModels.test.ts verificationComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含视觉实现，启动 dev server 并用浏览器截图检查 `/verification`、`/verification/records/verify-1`、`/verification/cases/case-1`。

## 19. 交付检查

- [ ] `/verification` 能展示 recovery queue、source health、recovery strip 和阻塞排序。
- [ ] Case header 展示 `RecoveryStatus`，workflow 成功不等于 recovered。
- [ ] 高风险 ActionProposal 缺少 VerificationSpec 时审批或执行入口被阻断。
- [ ] VerificationRecord 详情展示 checks、criteria、evidenceRefs、baseline/current/delta、failedPoint 和 nextRecommendation。
- [ ] Observation Window 展示 smoke、等待、趋势、业务验证、决策和超时状态。
- [ ] DebugCase 支持 before/after trace 对比，after trace 缺失时是 inconclusive。
- [ ] PG 等中间件修复支持复制、锁、连接、磁盘/WAL、业务指标和 DBA 人工确认。
- [ ] Rollback 有独立 VerificationRecord，回滚失败能生成 failedPoint 和经验候选入口。
- [ ] Case close 前能展示 close gate，并阻断未完成验证、pending approval、执行中 workflow、active HostLease、未收敛回滚。
- [ ] 所有请求走 `web/src/api/verificationRecovery.ts` 或统一 API client，不在页面内裸 `fetch`。
- [ ] 页面不创建私有 WebSocket/SSE。
- [ ] 受限证据不在正文、tooltip、title、aria-label 中泄露。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
