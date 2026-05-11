# aiops-v2 Middleware Repair 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-08a-middleware-repair-frontend-design.zh.md](2026-05-11-aiops-v2-08a-middleware-repair-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-08-middleware-repair-module-design.zh.md](2026-05-11-aiops-v2-08-middleware-repair-module-design.zh.md)

## 1. 目标

把 Middleware Repair 从普通 case 的描述升级为中间件异常修复前端控制台：支持中间件资源目录、资源详情、middleware_repair case、只读诊断、RCA、经验匹配、RepairPlan Designer、确认和审批门禁、RecoveryAttempt、Verification 嵌入、Rollback、Experience candidate，并严格保证“先 RCA，再修复；默认只读；高风险动作受治理执行”。

## 2. 实施顺序

```text
Middleware Repair API view-model
  -> Middleware Overview
  -> Resource Inventory / Detail
  -> Repair Case Workspace
  -> Diagnostics and kind-specific baseline
  -> Experience Match
  -> RepairPlan Designer
  -> Confirmation / Approval gates
  -> RecoveryAttempt Detail
  -> Verification / Rollback embedding
  -> Experience Candidate
  -> Cross-module integration
  -> Tests and visual checks
```

先建立 `middlewareRepair.ts` 和 view-model，再做页面。不要在 AI Chat、Incident、Coroot、Runner 页面各自拼接中间件修复状态；现有 AI Chat 页面不改，只消费后端创建/绑定 case 的结果。

## 3. 文件地图

新增：

- `web/src/api/middlewareRepair.ts`：MiddlewareResource、RepairCase、RepairPlan、RecoveryAttempt、ExperienceMatch API client 和类型。
- `web/src/api/middlewareRepair.test.ts`：normalize、fallback、风险阻断、敏感字段裁剪测试。
- `web/src/pages/MiddlewareOverviewPage.tsx`：Middleware Repair 总览。
- `web/src/pages/MiddlewareResourceListPage.tsx`：中间件资源目录。
- `web/src/pages/MiddlewareResourceDetailPage.tsx`：中间件资源详情。
- `web/src/pages/MiddlewareRepairCasePage.tsx`：修复 case 工作台。
- `web/src/pages/RepairPlanPage.tsx`：RepairPlan 详情和审核。
- `web/src/pages/RecoveryAttemptPage.tsx`：RecoveryAttempt 详情。
- `web/src/components/middleware/MiddlewareSafetyStrip.tsx`：Readonly Probe、Backup、Replication、RoleMap、HostLease、Approval、Verification。
- `web/src/components/middleware/MiddlewareSummaryStrip.tsx`：活跃 case、证据不足、待确认、运行中、验证失败、经验候选。
- `web/src/components/middleware/MiddlewareResourceTable.tsx`：资源目录表。
- `web/src/components/middleware/MiddlewareResourceDrawer.tsx`：资源行详情。
- `web/src/components/middleware/MiddlewareTopologyPanel.tsx`：中间件拓扑固定层级视图。
- `web/src/components/middleware/MiddlewareSafetyProfilePanel.tsx`：安全画像。
- `web/src/components/middleware/MiddlewareRepairQueueTable.tsx`：修复 case 队列。
- `web/src/components/middleware/MiddlewareRepairStatusRail.tsx`：repair phase 状态流。
- `web/src/components/middleware/MiddlewareRepairHeader.tsx`：case header。
- `web/src/components/middleware/DiagnosticCoveragePanel.tsx`：只读诊断覆盖率。
- `web/src/components/middleware/RootCauseHypothesisTable.tsx`：RCA 假设列表。
- `web/src/components/middleware/MiddlewareEvidenceDrawer.tsx`：证据详情和敏感裁剪。
- `web/src/components/middleware/PostgresDiagnosticPanel.tsx`：PG 专用诊断。
- `web/src/components/middleware/RedisDiagnosticPanel.tsx`：Redis 专用诊断。
- `web/src/components/middleware/MqKafkaDiagnosticPanel.tsx`：MQ / Kafka 诊断。
- `web/src/components/middleware/ElasticsearchDiagnosticPanel.tsx`：Elasticsearch 诊断。
- `web/src/components/middleware/MysqlDiagnosticPanel.tsx`：MySQL 诊断。
- `web/src/components/middleware/ExperienceMatchPanel.tsx`：经验匹配。
- `web/src/components/middleware/ExperienceMatchDrawer.tsx`：经验详情。
- `web/src/components/middleware/RepairPlanDesigner.tsx`：RepairPlan 编辑和审核。
- `web/src/components/middleware/RepairPlanStepTable.tsx`：计划步骤表。
- `web/src/components/middleware/RepairPlanRiskReview.tsx`：风险和禁用条件。
- `web/src/components/middleware/RepairPlanConfirmDialog.tsx`：确认对话框。
- `web/src/components/middleware/RecoveryAttemptSummary.tsx`：执行摘要。
- `web/src/components/middleware/RecoveryAttemptStepResults.tsx`：步骤结果。
- `web/src/components/middleware/MiddlewareVerificationEmbed.tsx`：07 模块验证嵌入。
- `web/src/components/middleware/MiddlewareRollbackPanel.tsx`：回滚视图。
- `web/src/components/middleware/MiddlewareExperienceCandidatePanel.tsx`：经验候选生成入口。
- `web/src/components/middleware/middlewareRepairViewModels.ts`：状态解释、风险阻断、诊断覆盖、经验匹配、计划确认、敏感裁剪。
- `web/src/components/middleware/middlewareRepairViewModels.test.ts`：view-model 单测。
- `web/src/components/middleware/middlewareComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/IncidentWorkbenchPage.tsx`：`middleware_repair` case 嵌入中间件修复摘要和跳转。
- `web/src/pages/AIReasoningPage.tsx` 或 Prompt Trace 相关页面：`repair_plan_draft` 输出跳转到 Middleware Repair。
- `web/src/pages/OpsGraphPage.tsx`：MiddlewareResource entity 跳转资源详情。
- `web/src/pages/CorootOverviewPage.tsx` 或 Observability 页面：中间件 probe / RCA 证据跳转 repair case。
- `web/src/pages/ApprovalManagementPage.tsx`：中间件高风险动作展示 RepairPlan、backup、rollback、verification blocker。
- `web/src/pages/WorkflowRunDetailPage.tsx` 或对应 Execution Run Detail 组件：source=repair_plan 的 run 展示 RepairPlan 链接；不要求修改现有 Runner 页面。
- `web/src/pages/ExperiencePacksPage.tsx`：展示 middleware_repair source 和 RepairPlan candidate。
- `web/src/pages/PostmortemPage.tsx`：引用 RepairPlan、RecoveryAttempt、failedPoint 和 Verification outcome。
- `web/src/app/navigation.ts`：增加 Middleware Repair 导航。
- `web/src/router.tsx`：注册 `/middleware` 相关路由。
- `web/src/pages/complexPages.test.tsx`：补充 Middleware Repair 路由测试。

## 4. Task 1：建立 Middleware Repair API 与类型

- [ ] 新增 `web/src/api/middlewareRepair.ts`。
- [ ] 定义 `MiddlewareKind`：`postgres`、`redis`、`mq`、`elasticsearch`、`kafka`、`mysql`、`other`。
- [ ] 定义 `RepairPhase`：`created`、`identifying_resource`、`collecting_evidence`、`diagnosing`、`matching_experience`、`planning`、`awaiting_confirmation`、`awaiting_approval`、`executing`、`verifying`、`succeeded`、`failed`、`rolled_back`、`experience_candidate_ready`。
- [ ] 定义 `RepairPlanStatus`：`draft`、`awaiting_confirmation`、`approved`、`running`、`succeeded`、`failed`。
- [ ] 定义 `MiddlewareResourceView`、`MiddlewareTopologyView`、`MiddlewareRoleView`、`MiddlewareSafetyProfileView`、`MiddlewareRepairCaseView`、`DiagnosticCoverageView`、`RootCauseHypothesisView`、`ExperienceMatchView`、`RepairPlanView`、`RepairStepView`、`RecoveryAttemptView`、`RepairBlockerView`。
- [ ] 实现 `listMiddlewareResources(params)` 和 `getMiddlewareResource(resourceId)`。
- [ ] 实现 `diagnoseMiddlewareResource(resourceId, payload)`。
- [ ] 实现 `listMiddlewareRepairCases(params)`、`createMiddlewareRepairCase(payload)`、`getMiddlewareRepairCase(caseId)`。
- [ ] 实现 `matchMiddlewareExperience(caseId, payload)`。
- [ ] 实现 `createRepairPlan(caseId, payload)`、`getRepairPlan(planId)`、`updateRepairPlan(planId, payload)`。
- [ ] 实现 `confirmRepairPlan(planId, payload)`、`dryRunRepairPlan(planId, payload)`、`executeRepairPlan(planId, payload)`、`verifyRepairPlan(planId, payload)`。
- [ ] 实现 `getRecoveryAttempt(attemptId)`、`rollbackRecoveryAttempt(attemptId, payload)`。
- [ ] 实现 `createMiddlewareExperienceCandidate(caseId, payload)`。
- [ ] normalize 时补齐 topology、roleMap、safetyProfile、evidenceCoverage、hypotheses、experienceMatches、repairPlans、recoveryAttempts、verificationRefs、blockers 默认值。
- [ ] normalize 时裁剪 connection string、SQL 参数、cache key 参数、MQ payload、SecretRef、token 和 password。
- [ ] 新增 `middlewareRepair.test.ts` 覆盖 canonical 字段、fallback 字段、PG baseline 默认值、敏感字段裁剪和 destructive blocker。
- [ ] 运行 `npm --prefix web test -- middlewareRepair.test.ts`。

## 5. Task 2：实现 Middleware Repair view-model

- [ ] 新增 `web/src/components/middleware/middlewareRepairViewModels.ts`。
- [ ] 实现 `getMiddlewareKindLabel(kind)` 和 `getRepairPhaseLabel(phase)`。
- [ ] 实现 `summarizeSafetyProfile(resource)`，输出 Readonly Probe、Backup/PITR、Replication、RoleMap、HostLease、Approval、Verification 状态。
- [ ] 实现 `summarizeDiagnosticCoverage(case)`，输出 required baseline 完成率和缺失项。
- [ ] 实现 `deriveRepairBlockers(caseOrPlan)`，识别 missing evidence、backup unknown、replication lag high、disabled condition、approval missing、HostLease conflict、verification missing。
- [ ] 实现 `canConfirmRepairPlan(plan, case)`，仅在 required evidence 完成、禁用条件未触发、rollback/verification/approval 满足时返回 true。
- [ ] 实现 `canExecuteRepairStep(step)`，高风险和破坏性步骤必须绑定 approval、HostLease、ActionToken 和 Verification。
- [ ] 实现 `sortExperienceMatches(matches)`，按 environment match、disabled condition、success rate、recent failure 排序。
- [ ] 实现 `redactMiddlewareEvidence(evidence)`，隐藏连接串、SQL 参数、cache key 参数、MQ payload 和 secret。
- [ ] 新增 `middlewareRepairViewModels.test.ts` 覆盖 confirmation gate、experience sorting、PG blocker、敏感裁剪和 workflow 成功不等于 repair succeeded。
- [ ] 运行 `npm --prefix web test -- middlewareRepairViewModels.test.ts`。

## 6. Task 3：实现 Middleware Overview

- [ ] 新增 `web/src/pages/MiddlewareOverviewPage.tsx`。
- [ ] 新增 `MiddlewareSafetyStrip.tsx`。
- [ ] 新增 `MiddlewareSummaryStrip.tsx`。
- [ ] 新增 `MiddlewareRepairQueueTable.tsx`。
- [ ] Header 展示环境、时间窗口、用户角色、探针健康、刷新、新建修复 case、从 AI Draft 打开。
- [ ] Safety Strip 展示 Readonly Probe、Backup/PITR、Replication、RoleMap、HostLease、Approval、Verification。
- [ ] Backup/PITR unavailable 或 unknown 时，destructive 修复入口禁用。
- [ ] Summary Strip 展示活跃 case、证据不足、待确认 RepairPlan、经验禁用条件触发、运行中 RecoveryAttempt、验证失败、回滚失败、待生成经验候选。
- [ ] Repair Queue 列展示 Case、Resource、Phase、Risk、Evidence Coverage、Experience、Plan、Execution、Verification、Next。
- [ ] 支持按 kind、phase、risk、environment、owner、blockedReason、hasExperience、requiresApproval 筛选。
- [ ] 注册 `/middleware` 路由和导航入口。
- [ ] 单测覆盖 destructive blocker、证据不足、待确认计划、运行中 attempt。

## 7. Task 4：实现 Resource Inventory

- [ ] 新增 `MiddlewareResourceListPage.tsx`。
- [ ] 新增 `MiddlewareResourceTable.tsx`。
- [ ] 新增 `MiddlewareResourceDrawer.tsx`。
- [ ] 支持 kind、environment、clusterName、criticality、safetyProfile、role completeness、backup status、replication status、business capability、owner、has active repair、has approved experience 筛选。
- [ ] 表格列包括 Resource、Topology、Safety、Business、Current Signal、Experience、Active Work、Action。
- [ ] 行动作支持打开详情、创建 repair case、运行只读诊断、匹配经验。
- [ ] Drawer 展示 MiddlewareResource 摘要、OpsGraph 关联实体、业务影响、安全画像、最近告警、最近修复 case、已审核经验。
- [ ] 注册 `/middleware/resources` 路由。
- [ ] 单测覆盖资源筛选、只读诊断入口和 active repair 标识。

## 8. Task 5：实现 Resource Detail

- [ ] 新增 `MiddlewareResourceDetailPage.tsx`。
- [ ] 新增 `MiddlewareTopologyPanel.tsx`。
- [ ] 新增 `MiddlewareSafetyProfilePanel.tsx`。
- [ ] Header 展示 kind、clusterName、environment、criticality、owner、safetyProfile、current health、active repair case、approved experience count。
- [ ] Tabs 包含 Topology、Safety Profile、Diagnostics、Business Impact、Experience、History。
- [ ] Topology 使用固定层级 BusinessCapability / ERPJob -> Service / APIRoute -> MiddlewareResource -> ClusterRole -> Host / Pod / Volume。
- [ ] 节点展示 role、health、replication/lag、read/write routing、storage pressure、owner、last evidence time。
- [ ] Safety Profile 展示 backup、PITR、replication、disk/WAL、权限、HostLease。
- [ ] History 展示历史 repair case、RecoveryAttempt 和验证结果。
- [ ] 注册 `/middleware/resources/:resourceId` 路由。
- [ ] 单测覆盖 PG role map、业务影响路径和安全画像。

## 9. Task 6：实现 Repair Case Workspace

- [ ] 新增 `MiddlewareRepairCasePage.tsx`。
- [ ] 新增 `MiddlewareRepairHeader.tsx`。
- [ ] 新增 `MiddlewareRepairStatusRail.tsx`。
- [ ] Header 展示 caseId、resource、kind、environment、case status、repair phase、risk level、primary hypothesis、evidence coverage、selected experience、active RepairPlan、RecoveryStatus。
- [ ] Status Rail 展示 created、identifying_resource、collecting_evidence、diagnosing、matching_experience、planning、awaiting_confirmation、awaiting_approval、executing、verifying、succeeded、failed、rolled_back、experience_candidate_ready。
- [ ] Tabs 包含 Overview、Resource、Diagnostics、Experience Match、RepairPlan、Execution、Verification、Rollback、Experience Candidate、Timeline / Audit。
- [ ] 从现有 AI Chat 后端创建/绑定的修复 case 初始停在 collecting_evidence 或 diagnosing，不进入 executing。
- [ ] Overview 展示 repair phase、阻塞项、下一步、风险和证据覆盖。
- [ ] 注册 `/middleware/repairs/:caseId` 路由。
- [ ] 单测覆盖自然语言修复请求不会直接显示执行按钮。

## 10. Task 7：实现 Diagnostics 与 RCA

- [ ] 新增 `DiagnosticCoveragePanel.tsx`。
- [ ] 新增 `RootCauseHypothesisTable.tsx`。
- [ ] 新增 `MiddlewareEvidenceDrawer.tsx`。
- [ ] Coverage 展示 Topology、Replication、Locks、Connections、Storage、Performance、Backup、Business Impact、Trace Link。
- [ ] required baseline 缺失时，只显示继续只读诊断或证据不足计划入口，禁用高风险修复确认。
- [ ] RCA 表格列包括 Hypothesis、Confidence、Supporting Evidence、Contradicting Evidence、Blast Radius、Safety Risk、Next Diagnostic。
- [ ] 假设状态支持 candidate、supported、contradicted、needs_more_evidence、confirmed_by_human。
- [ ] Evidence Drawer 展示 EvidenceRef id、source、timeWindow、entityRefs、quality、redactionStatus、rawRef、下游使用资格。
- [ ] SQL、cache、MQ operation 只显示 signature，不显示参数值。
- [ ] 单测覆盖 evidence missing、needs_more_evidence、contradicting evidence 和敏感裁剪。

## 11. Task 8：实现 PG 专用诊断面板

- [ ] 新增 `PostgresDiagnosticPanel.tsx`。
- [ ] 展示 primary、standby、replica、proxy。
- [ ] 展示 replication lag、slot、WAL、sender、receiver。
- [ ] 展示 blocking query、blocked query、lock type、duration。
- [ ] 展示 active、idle、idle in transaction、max connections。
- [ ] 展示 disk usage、WAL usage、tablespace。
- [ ] 展示 slow query fingerprint、CPU、IO、buffer、checkpoint。
- [ ] 展示 latest backup、backup validity、PITR capability。
- [ ] 展示 failover data loss window、application connection、read/write route。
- [ ] 阻断条件包括无最近备份、PITR 未知、replication lag 超限、failover data loss window 不可接受、blocker 是 migration process、blocker identity 不明确、业务路径不明确、DBA approval 不存在。
- [ ] 常见修复类型以计划步骤展示，不做裸按钮。
- [ ] 单测覆盖 PG lock wait、备份未知、migration blocker、failover lag blocker。

## 12. Task 9：实现 Redis、MQ/Kafka、Elasticsearch、MySQL 面板

- [ ] 新增 `RedisDiagnosticPanel.tsx`，展示 role、memory、evictions、latency、slowlog signature、replication lag、clients、blocked clients、keyspace summary、RDB/AOF、failover state。
- [ ] Redis 阻断 maxmemory policy unknown、persistence unknown、failover quorum insufficient、replica lag high、业务 key pattern 无法脱敏。
- [ ] 新增 `MqKafkaDiagnosticPanel.tsx`，展示 broker state、topic/queue health、consumer group lag、partition imbalance、ISR/replica、disk、producer/consumer error、throughput。
- [ ] MQ/Kafka 阻断 retention policy unknown、consumer ownership unclear、partition movement risk high、broker disk pressure no rollback。
- [ ] 新增 `ElasticsearchDiagnosticPanel.tsx`，展示 cluster health、shard allocation、unassigned shards、disk watermark、JVM heap、indexing/search latency、slow query signature、snapshot status。
- [ ] Elasticsearch 阻断 snapshot unavailable、shard movement risk high、disk watermark worsening、index operation scope unclear。
- [ ] 新增 `MysqlDiagnosticPanel.tsx`，展示 primary/replica、replication lag、binlog、slow query fingerprint、lock wait、connection saturation、disk、backup/PITR。
- [ ] MySQL 阻断 binlog/backup unknown、replication topology incomplete、DDL/migration lock unidentified。
- [ ] 单测覆盖每类中间件的主要 blocker 和脱敏展示。

## 13. Task 10：实现 Experience Match

- [ ] 新增 `ExperienceMatchPanel.tsx`。
- [ ] 新增 `ExperienceMatchDrawer.tsx`。
- [ ] 匹配来源只包括 approved ExperiencePack、approved Capsule、approved RepairPlan、published Runbook、published Workflow、historical case。
- [ ] 未审核 candidate 不进入匹配结果。
- [ ] 列表列包括 Experience、Applicability、Fitness、Risk、Disabled Conditions、Evidence、Action。
- [ ] 命中但 disabled condition 触发时，只允许复用只读诊断步骤，禁用 remediationSteps。
- [ ] Drawer 展示来源 case、适用环境、成功率、最近失败、禁用条件、所需权限、风险、审批、回滚、验证、lineage、reviewer。
- [ ] 单测覆盖 approved-only、environment mismatch、disabled condition、只读诊断可复用。

## 14. Task 11：实现 RepairPlan Designer

- [ ] 新增 `RepairPlanPage.tsx`。
- [ ] 新增 `RepairPlanDesigner.tsx`。
- [ ] 新增 `RepairPlanStepTable.tsx`。
- [ ] 新增 `RepairPlanRiskReview.tsx`。
- [ ] Plan Header 展示 plan id、caseId、middlewareResourceId、source、status、riskLevel、approvalPolicy、selected experience、confirmation state。
- [ ] Sections 包含 Assumptions、Diagnostic Steps、Remediation Steps、Risk Review、Approval Policy、Rollback Plan、Verification Spec、Disabled Conditions、Diff。
- [ ] Step 表格列包括 Step、Target、Risk、Evidence、Governance、Inputs、Output、Blockers。
- [ ] 用户确认前检查 required diagnostic evidence、unresolved blockers、disabledConditions、rollbackPlan、verificationSpec、backup/PITR、approvalPolicy、ActionProposal/Workflow mapping。
- [ ] 确认后进入 awaiting_approval 或 approved，不直接执行高风险动作。
- [ ] 注册 `/middleware/repair-plans/:planId` 路由。
- [ ] 单测覆盖 confirmation gate、missing rollback、missing verification、approval missing、disabled condition。

## 15. Task 12：实现 Confirmation / Approval gates

- [ ] 新增 `RepairPlanConfirmDialog.tsx`。
- [ ] Dialog 展示 assumptions、required evidence、disabled conditions、risk summary、backup/PITR、approvalPolicy、rollbackPlan、verificationSpec。
- [ ] 用户必须确认当前假设、风险接受范围、是否需要 DBA/resource owner/service owner。
- [ ] destructive plan 必须显示“默认禁止自动化，除非 break-glass 和强审计”的门禁说明。
- [ ] 触发 approval 时调用统一 approval API 或 Execution Fabric API，不在页面创建私有审批流。
- [ ] ApprovalManagementPage 展示 RepairPlan、backup、rollback、verification blocker。
- [ ] 单测覆盖 destructive confirm blocked、approval request、manual follow-up。

## 16. Task 13：实现 RecoveryAttempt Detail

- [ ] 新增 `RecoveryAttemptPage.tsx`。
- [ ] 新增 `RecoveryAttemptSummary.tsx`。
- [ ] 新增 `RecoveryAttemptStepResults.tsx`。
- [ ] Summary 展示 RepairPlan、RecoveryAttempt、WorkflowRun、ActionProposal refs、HostLease、Approval、current step、failedPoint、rollbackResult、verificationRefs。
- [ ] Step Result 展示 step id、step type、target、status、startedAt、finishedAt、actor、toolName、normalizedInputHash、output summary、evidenceRefs、error summary、next recommendation。
- [ ] 长输出、敏感输出、SecretRef 和连接信息只展示摘要和引用。
- [ ] 失败时展示 failure category：evidence_missing、approval_denied、host_lock_conflict、tool_failed、verification_failed、rollback_failed、source_unavailable。
- [ ] 注册 `/middleware/recovery-attempts/:attemptId` 路由。
- [ ] 单测覆盖 failedPoint、rollbackResult、normalizedInputHash、敏感输出裁剪。

## 17. Task 14：嵌入 Verification 与 Rollback

- [ ] 新增 `MiddlewareVerificationEmbed.tsx`，复用 07 模块 VerificationSpec、VerificationRecord、Evidence Compare、Observation Window、RecoveryStatus。
- [ ] 新增 `MiddlewareRollbackPanel.tsx`。
- [ ] PG 默认验证展示 replication lag、lock wait、connection saturation、disk/WAL、业务 p95 latency、ERP order/job 指标。
- [ ] Rollback 展示 rollback criteria、rollback plan、rollback ActionProposal/WorkflowRun、rollback result、rollback VerificationRecord、data consistency check、business not-worse check、HostLease/approval closure。
- [ ] 回滚失败时 case 不能进入 succeeded 或 recovered，必须进入 failed 或 requires_manual_followup。
- [ ] 单测覆盖 verification failed、rollback failed、DBA manual confirmation、business recovery pending。

## 18. Task 15：实现 Experience Candidate

- [ ] 新增 `MiddlewareExperienceCandidatePanel.tsx`。
- [ ] 生成候选类型支持 Gene candidate、Capsule candidate、RepairPlan candidate、Runbook draft、Workflow graph draft、Eval case、Memory candidate、OpsGraph patch。
- [ ] 候选必须展示 MiddlewareResource、environment profile、problem signature、RCA hypothesis、diagnostic evidence、RepairPlan、RecoveryAttempt、failedPoint、rollbackResult、Verification outcome、disabledConditions、reviewer requirements。
- [ ] “生成经验候选”只创建 candidate，不发布。
- [ ] candidate 不进入 AI Chat、RepairPlan matcher 或 Activation Index。
- [ ] ExperiencePacksPage 支持 `source=middleware_repair` 和 RepairPlan candidate detail。
- [ ] 单测覆盖成功候选、失败候选、未审核不进入推荐。

## 19. Task 16：跨模块集成

- [ ] IncidentWorkbenchPage 对 `middleware_repair` case 展示 Middleware Repair 摘要、active RepairPlan、RecoveryAttempt 和跳转。
- [ ] AI Reasoning / Prompt Trace 页面把 `repair_plan_draft` 输出路由到 `/middleware/repairs/:caseId` 或 `/middleware/repair-plans/:planId`。
- [ ] OpsGraphPage 对 MiddlewareResource entity 提供打开 `/middleware/resources/:resourceId`。
- [ ] Observability / Coroot 页面把 middleware_probe、DB/cache/MQ span、Coroot RCA 跳转到 repair case。
- [ ] Runner / WorkflowRun Detail 对 source=repair_plan 的 run 展示 RepairPlan、RecoveryAttempt 和 Verification 链接。
- [ ] ApprovalManagementPage 对中间件高风险动作展示 backup、rollback、verification、DBA approval 和 disabled condition。
- [ ] Verification 页面能按 source=repair_plan 过滤并回链 RepairPlan。
- [ ] PostmortemPage 引用 RepairPlan、RecoveryAttempt、failedPoint、Verification outcome。
- [ ] 单测覆盖 Chat、Case、OpsGraph、Observability、Execution、Approval、Verification、Experience 的跳转。

## 20. Task 17：路由、导航和权限

- [ ] 修改 `web/src/app/navigation.ts`，增加 Middleware Repair 导航项。
- [ ] 修改 `web/src/router.tsx`，注册 `/middleware`、`/middleware/resources`、`/middleware/resources/:resourceId`、`/middleware/repairs`、`/middleware/repairs/:caseId`、`/middleware/repair-plans/:planId`、`/middleware/recovery-attempts/:attemptId`。
- [ ] viewer 只能查看脱敏摘要和只读诊断结果。
- [ ] operator 可以创建修复 case、运行只读诊断、生成 RepairPlan draft。
- [ ] DBA/resource owner 可以确认高风险中间件计划。
- [ ] approver 可以审批 ActionProposal / Workflow。
- [ ] admin 可以配置中间件探针、break-glass 策略和经验激活策略。
- [ ] 无权限时隐藏连接串、SQL 参数、业务 payload、SecretRef、raw command。
- [ ] 单测覆盖角色权限、按钮禁用和敏感字段隐藏。

## 21. Task 18：事件、轮询和一致性

- [ ] 在 `middlewareRepair.ts` 中实现 summary polling helper。
- [ ] 诊断和执行运行中只刷新 case summary、coverage、plan status、attempt status、verification summary。
- [ ] 大证据正文和原始输出只在用户打开 Drawer 时加载。
- [ ] 接入统一事件时支持 middleware.resource.identified、middleware.diagnostic.completed、repair.plan.generated、repair.plan.confirmed、recovery.attempt.updated、repair.verification.completed、repair.experience_candidate.created。
- [ ] 页面刷新或重进后能从 API 恢复当前修复阶段。
- [ ] run diagnose、confirm、execute、rollback 操作要显示 running guard，避免重复提交。
- [ ] 单测覆盖刷新恢复、重复点击被禁用、polling 错误保留旧数据。

## 22. Task 19：测试与视觉检查

- [ ] 新增 `middlewareRepair.test.ts`。
- [ ] 新增 `middlewareRepairViewModels.test.ts`。
- [ ] 新增 `middlewareComponents.test.tsx`。
- [ ] 扩展 `web/src/pages/complexPages.test.tsx` 覆盖 `/middleware`、`/middleware/resources`、`/middleware/resources/:resourceId`、`/middleware/repairs/:caseId`、`/middleware/repair-plans/:planId`、`/middleware/recovery-attempts/:attemptId`。
- [ ] 新增或扩展 Playwright 用例，覆盖桌面和移动宽度下的 overview、PG resource detail、repair case、RepairPlan confirm dialog。
- [ ] 运行 `npm --prefix web test -- middlewareRepair.test.ts middlewareRepairViewModels.test.ts middlewareComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含视觉实现，启动 dev server 并用浏览器截图检查 `/middleware`、`/middleware/resources/pg-main`、`/middleware/repairs/case-pg-lock`。

## 23. 交付检查

- [ ] `/middleware` 能展示总览、Safety Strip、活跃修复、待确认 RepairPlan、运行中 RecoveryAttempt、验证失败和经验候选。
- [ ] `/middleware/resources` 能展示 PG、Redis、MQ、Kafka、Elasticsearch、MySQL 等资源和安全画像。
- [ ] AI Chat 的“帮我修复 xxx 的 PG 集群”只能由后端创建/绑定 middleware_repair case 并进入诊断或计划阶段，不能直接执行，也不要求改 AI Chat 页面。
- [ ] PG 页面展示拓扑、角色、复制、锁、连接、磁盘/WAL、备份、PITR 和 failover 风险。
- [ ] required diagnostic evidence 缺失时，高风险修复确认被禁用。
- [ ] 命中已审核经验时展示来源、环境、成功率、最近失败、禁用条件、权限、风险、回滚和验证项。
- [ ] 环境不匹配或 disabled condition 触发时，只能复用只读诊断，不能复用修复动作。
- [ ] RepairPlan Designer 展示 assumptions、diagnosticSteps、remediationSteps、risk、approvalPolicy、rollbackPlan、verificationSpec 和 disabledConditions。
- [ ] 高风险和破坏性步骤必须展示审批、HostLease、ActionToken、回滚和验证状态。
- [ ] RecoveryAttempt 展示每步结果、failedPoint、rollbackResult 和 verificationRefs。
- [ ] 修复完成后嵌入 Verification & Recovery，验证中间件健康和业务恢复。
- [ ] 修复成功或失败都能生成经验候选，candidate 不进入 Activation Index。
- [ ] 所有请求走 `web/src/api/middlewareRepair.ts` 或统一 API client，不在页面内裸 `fetch`。
- [ ] 页面不创建私有 WebSocket/SSE。
- [ ] SQL、cache、MQ operation、连接串、SecretRef 和敏感输出不在正文、tooltip、title、aria-label 中泄露。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
