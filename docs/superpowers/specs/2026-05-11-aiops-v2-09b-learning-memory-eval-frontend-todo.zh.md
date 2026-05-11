# aiops-v2 Learning, Experience Pack, Memory & Eval 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-09a-learning-memory-eval-frontend-design.zh.md](2026-05-11-aiops-v2-09a-learning-memory-eval-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md](2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md)
专题文档：[2026-05-11-aiops-v2-10-experience-packs-design.zh.md](2026-05-11-aiops-v2-10-experience-packs-design.zh.md)

## 1. 目标

把当前静态 ExperiencePacksPage 升级为 Learning 资产控制台：支持 Experience Pack、Candidate Store、Review Queue、Activation Index、Memory Explorer、Eval Workbench、Evolution Map、Lineage、Environment Profile 和跨模块资产沉淀闭环。所有自动生成内容只能进入 candidate 或 draft，审核通过后才能影响 AI Chat、DebugCase、中间件修复、Runbook 或 Workflow。

## 2. 实施顺序

```text
Learning API view-model
  -> Learning Overview
  -> Experience Pack Library / Detail
  -> Review Queue / Candidate Review
  -> Activation Index Explorer
  -> Memory Explorer
  -> Eval Workbench / Report
  -> Evolution Map / Lineage
  -> Environment Profiles / Variants
  -> Cross-module embedding
  -> Tests and visual checks
```

先建立 `learningAssets.ts` 和 view-model，再迁移 `ExperiencePacksPage.tsx`。不要让 Chat、Case、Middleware、Runner 页面各自读取 Candidate Store 或拼接推荐状态。

## 3. 文件地图

新增：

- `web/src/api/learningAssets.ts`：ExperiencePack、CandidateArtifact、Review、ActivationIndex、Memory、Eval、EvolutionMap、EnvironmentProfile API client 和类型。
- `web/src/api/learningAssets.test.ts`：normalize、fallback、候选隔离、权限裁剪、redaction 测试。
- `web/src/pages/LearningOverviewPage.tsx`：Learning 总览。
- `web/src/pages/ExperiencePackLibraryPage.tsx`：经验包库。
- `web/src/pages/ExperiencePackDetailPage.tsx`：经验包详情。
- `web/src/pages/ExperienceReviewQueuePage.tsx`：审核队列。
- `web/src/pages/ExperienceCandidateReviewPage.tsx`：候选审核详情。
- `web/src/pages/ActivationIndexPage.tsx`：Activation Index。
- `web/src/pages/MemoryExplorerPage.tsx`：Memory Explorer。
- `web/src/pages/MemoryDetailPage.tsx`：Memory 详情。
- `web/src/pages/EvalWorkbenchPage.tsx`：Eval Workbench。
- `web/src/pages/EvalReportPage.tsx`：Eval Report。
- `web/src/pages/EvolutionMapPage.tsx`：Evolution Map。
- `web/src/pages/AssetLineagePage.tsx`：资产 lineage。
- `web/src/pages/EnvironmentProfileListPage.tsx`：环境画像列表。
- `web/src/pages/EnvironmentProfileDetailPage.tsx`：环境画像详情。
- `web/src/components/learning/LearningGovernanceStrip.tsx`：Candidate Isolation、Review SLA、Activation、Memory、Eval、Asset Drift。
- `web/src/components/learning/LearningSummaryStrip.tsx`：候选、审核、激活、Memory、Eval 风险统计。
- `web/src/components/learning/ExperiencePackTable.tsx`：经验包列表。
- `web/src/components/learning/ExperiencePackDetailTabs.tsx`：详情 tabs。
- `web/src/components/learning/EvidenceChainPanel.tsx`：证据链。
- `web/src/components/learning/CandidateArtifactPanel.tsx`：候选 artifact 详情。
- `web/src/components/learning/ReviewQueueTable.tsx`：审核队列表。
- `web/src/components/learning/ReviewGateResultPanel.tsx`：Gate Result。
- `web/src/components/learning/ReviewDecisionPanel.tsx`：审核决策。
- `web/src/components/learning/ArtifactDiffPanel.tsx`：Gene/Capsule/Runbook/Workflow/RepairPlan/Memory/OpsGraph patch diff。
- `web/src/components/learning/ActivationIndexTable.tsx`：Activation Index 列表。
- `web/src/components/learning/ActivationExplainDrawer.tsx`：匹配解释。
- `web/src/components/learning/MemoryRecordTable.tsx`：Memory 表格。
- `web/src/components/learning/MemoryImpactPanel.tsx`：Memory prompt impact。
- `web/src/components/learning/EvalCaseTable.tsx`：Eval case 列表。
- `web/src/components/learning/EvalRunForm.tsx`：Eval run 表单。
- `web/src/components/learning/EvalReportSummary.tsx`：Eval report 摘要。
- `web/src/components/learning/EvalRegressionPanel.tsx`：回归诊断。
- `web/src/components/learning/EvolutionEventTable.tsx`：演化事件。
- `web/src/components/learning/AssetLineageGraph.tsx`：固定布局 lineage 图。
- `web/src/components/learning/FitnessPanel.tsx`：fitness 和失败聚类。
- `web/src/components/learning/EnvironmentProfileTable.tsx`：环境画像表。
- `web/src/components/learning/EnvironmentVariantPanel.tsx`：环境变体。
- `web/src/components/learning/learningViewModels.ts`：状态解释、gate、candidate isolation、review action、eval blocker、redaction、lineage summary。
- `web/src/components/learning/learningViewModels.test.ts`：view-model 单测。
- `web/src/components/learning/learningComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/ExperiencePacksPage.tsx`：改为 API 驱动或跳转到 `ExperiencePackLibraryPage`，保留兼容路由。
- `web/src/pages/IncidentWorkbenchPage.tsx`：Assets tab 嵌入 Learning candidates。
- `web/src/pages/PromptTracePage.tsx` 或 AI Reasoning 页面：展示 activatedExperienceRefs、memoryRefs、evalLabels、candidate leakage warning。
- `web/src/pages/CorootOverviewPage.tsx` / Observability 页面：Debug Trace 生成 Debug RCA candidate。
- `web/src/pages/MiddlewareRepairCasePage.tsx` 或后续 08 页面：Repair case 生成 RepairPlan/Capsule candidate。
- `web/src/pages/RunnerStudioPage.tsx`：Workflow draft 来源为 experience-pack 时展示 source、review、environment、evidence。
- `web/src/pages/RunbookCatalogPage.tsx`：只展示 approved/published Runbook，candidate 不进入正式目录。
- `web/src/pages/OpsGraphPage.tsx`：展示 Experience / Memory / Eval asset，并接入 OpsGraph patch review。
- `web/src/pages/PostmortemPage.tsx`：引用 Experience candidate、Eval case、Memory candidate。
- `web/src/app/navigation.ts`：增加 Learning 导航。
- `web/src/router.tsx`：注册 `/learning` 相关路由，并让 `/experience-packs` 兼容。
- `web/src/pages/complexPages.test.tsx`：补充 Learning 路由测试。

## 4. Task 1：建立 Learning API 与类型

- [ ] 新增 `web/src/api/learningAssets.ts`。
- [ ] 定义 `ExperiencePackStatus`：`candidate`、`review_pending`、`approved`、`rejected`、`published`、`deprecated`。
- [ ] 定义 `LearningAssetType`：`gene`、`capsule`、`debug_rca`、`repair_plan`、`runbook`、`workflow`、`memory`、`opsgraph_patch`、`eval_case`。
- [ ] 定义 `ExperiencePackView`、`CandidateArtifactView`、`ReviewRecordView`、`ActivationIndexEntryView`、`MemoryRecordView`、`EvalCaseView`、`EvalReportView`、`EvolutionEventView`、`LineageView`、`EnvironmentProfileView`、`FitnessView`。
- [ ] 实现 `getLearningOverview(params)`，请求 `GET /api/v1/learning/overview`，后端未提供时从 experience-packs、review-queue、activation-index、memory、eval 聚合。
- [ ] 实现 `listExperiencePacks(params)`、`getExperiencePack(packId)`、`matchExperiencePacks(payload)`。
- [ ] 实现 `listReviewQueue(params)`、`reviewExperiencePack(packId, payload)`。
- [ ] 实现 `generateRunbookDraft(packId, payload)`、`generateWorkflowDraft(packId, payload)`、`createExperienceVariant(packId, payload)`。
- [ ] 实现 `searchActivationIndex(params)`、`explainActivationIndex(payload)`、`pauseActivationEntry(entryId, payload)`、`deprecateActivationEntry(entryId, payload)`。
- [ ] 实现 `searchMemory(params)`、`getMemory(memoryId)`、`markMemoryStale(memoryId, payload)`、`requestMemoryPromotion(memoryId, payload)`。
- [ ] 实现 `createEvalCase(payload)`、`runEval(payload)`、`getEvalReport(reportId)`。
- [ ] 实现 `listEvolutionEvents(params)`、`queryEvolutionMap(payload)`、`getAssetLineage(assetId)`。
- [ ] 实现 `listEnvironmentProfiles(params)`、`getEnvironmentProfile(profileId)`、`probeEnvironmentProfile(payload)`。
- [ ] normalize 时补齐 evidenceRefs、artifacts、review、activation、lineage、fitness、disabledConditions、redactionStatus 默认值。
- [ ] normalize 时受限证据、raw prompt、SecretRef、token、连接串、用户敏感输入只保留摘要和 restriction reason。
- [ ] 新增 `learningAssets.test.ts` 覆盖 candidate isolation、activation status、memory scopes、eval report、redaction。
- [ ] 运行 `npm --prefix web test -- learningAssets.test.ts`。

## 5. Task 2：实现 Learning view-model

- [ ] 新增 `web/src/components/learning/learningViewModels.ts`。
- [ ] 实现 `getExperiencePackStatusLabel(status)` 和 `getLearningAssetTypeLabel(type)`。
- [ ] 实现 `summarizeLearningGovernance(data)`，输出 Candidate Isolation、Review SLA、Activation Index、Memory Hygiene、Eval Gate、Asset Drift。
- [ ] 实现 `deriveCandidateGate(candidate)`，检查 evidence、environment、risk、rollback、verification、dry-run、eval、review roles。
- [ ] 实现 `canApproveCandidate(candidate, gate)`，高风险缺 rollback/verification、Eval failed、redaction failed 时返回 false。
- [ ] 实现 `candidateCanEnterActivation(candidate)`，仅 gene/capsule/debug_rca/repair_plan/runbook/workflow 且 approved。
- [ ] 实现 `summarizeActivationExplanation(entry, input)`，展示 why matched、why candidates excluded、disabledConditions、prompt impact。
- [ ] 实现 `summarizeMemoryHygiene(records)`，输出 stale、expired、redaction failed、unreviewed org memory。
- [ ] 实现 `deriveEvalPublicationBlocker(report)`，识别 forbidden behavior、candidate leakage、permission bypass、unsafe action。
- [ ] 实现 `buildLineageNodes(lineage)`，生成固定布局节点和边。
- [ ] 新增 `learningViewModels.test.ts` 覆盖候选隔离、审核门禁、Activation 解释、Memory stale、Eval blocker 和 lineage。
- [ ] 运行 `npm --prefix web test -- learningViewModels.test.ts`。

## 6. Task 3：实现 Learning Overview

- [ ] 新增 `LearningOverviewPage.tsx`。
- [ ] 新增 `LearningGovernanceStrip.tsx`。
- [ ] 新增 `LearningSummaryStrip.tsx`。
- [ ] Header 展示环境、时间窗口、用户角色、Candidate Store 状态、Activation Index 版本、Eval runner 状态。
- [ ] Governance Strip 展示 Candidate Isolation、Review SLA、Activation Index、Memory Hygiene、Eval Gate、Asset Drift。
- [ ] Summary Strip 展示新增候选、待审核、审核超时、已激活、pause/deprecated、Memory stale、Eval regression、variant needed。
- [ ] Tabs 包含 Candidates、Activation、Memory、Eval、Evolution。
- [ ] Candidate Isolation violation 时展示阻断告警和 audit 链接。
- [ ] 注册 `/learning` 路由和导航入口。
- [ ] 单测覆盖治理状态、候选隔离 violation、Eval failed 和 Memory stale。

## 7. Task 4：升级 Experience Pack Library

- [ ] 新增 `ExperiencePackLibraryPage.tsx`。
- [ ] 新增 `ExperiencePackTable.tsx`。
- [ ] 修改 `ExperiencePacksPage.tsx`，从静态 `experiencePacks` 切到 `learningAssets.ts`，或跳转到 `ExperiencePackLibraryPage`。
- [ ] 支持 status、assetType、source、environment profile、risk、reviewer、activation eligible、fitness、disabled condition、updatedAt 筛选。
- [ ] 表格列包括 Pack、Scenario、Artifacts、Evidence、Environment、Review、Activation、Fitness、Action。
- [ ] Tabs 展示 candidate、review_pending、approved、rejected、published、deprecated。
- [ ] `/experience-packs` 兼容路由展示同一页面。
- [ ] 单测覆盖 candidate 不显示为 published、静态 fallback、筛选和空态。

## 8. Task 5：实现 Experience Pack Detail

- [ ] 新增 `ExperiencePackDetailPage.tsx`。
- [ ] 新增 `ExperiencePackDetailTabs.tsx`。
- [ ] 新增 `EvidenceChainPanel.tsx`。
- [ ] 新增 `CandidateArtifactPanel.tsx`。
- [ ] Tabs 包含 Overview、Evidence Chain、Artifacts、Review、Activation、Lineage、Fitness、Eval。
- [ ] Evidence Chain 展示 case、Coroot、Debug Trace、Middleware Repair、Workflow、Approval、Verification、Postmortem。
- [ ] Artifacts 展示 Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft、Memory、OpsGraph patch、Eval case。
- [ ] Review tab 展示审核记录、Gate Result、评论、决策和 audit。
- [ ] Activation tab 展示 Activation Index 条目、推荐场景、禁用条件、pause。
- [ ] Lineage tab 展示来源、版本、变体、supersedes、incompatible_with。
- [ ] Fitness tab 展示成功率、失败聚类、recent failures、降权原因。
- [ ] 注册 `/learning/experience-packs/:packId`。
- [ ] 单测覆盖 evidence chain、candidate artifact、activation disabled 和 lineage。

## 9. Task 6：实现 Review Queue

- [ ] 新增 `ExperienceReviewQueuePage.tsx`。
- [ ] 新增 `ReviewQueueTable.tsx`。
- [ ] 新增 `ExperienceCandidateReviewPage.tsx`。
- [ ] 新增 `ReviewGateResultPanel.tsx`。
- [ ] 新增 `ReviewDecisionPanel.tsx`。
- [ ] 新增 `ArtifactDiffPanel.tsx`。
- [ ] 队列列包括 Candidate、Scope、Evidence、Risk、Impact、Gate、SLA、Action。
- [ ] 默认排序为 Gate violation、高风险候选、Review SLA overdue、Eval regression blocker、新增候选。
- [ ] 审核详情展示 Evidence、AI Chat Impact、Artifact Diff、Debug / Repair Diff、Gate Result、Review History。
- [ ] 支持 reject、request_changes、approve_gene、approve_capsule、approve_debug_rca、approve_repair_plan、approve_as_memory、approve_opsgraph_patch、generate_runbook_draft、generate_workflow_draft、create_environment_variant、pause_asset、deprecate_asset。
- [ ] 高风险候选缺 rollback 或 verification 时禁用 approve。
- [ ] Eval failed 时禁用会影响推荐的 activation。
- [ ] 注册 `/learning/review-queue` 和 `/learning/review-queue/:candidateId`。
- [ ] 单测覆盖 Gate blocker、Eval blocker、review decision payload、request changes。

## 10. Task 7：实现 Candidate Artifact 专用展示

- [ ] Gene Candidate 展示 rule summary、scenario signature、evidence refs、expected recommendation、forbidden recommendation、activation scope、fitness seed。
- [ ] Capsule Candidate 展示 applicability、diagnostic steps、action steps、verification、rollback、disabled conditions、environment compatibility。
- [ ] Debug RCA Candidate 展示 frontend page、user action、trace id、slow span、service path、Coroot RCA、suggested repair、redaction status、before/after verification。
- [ ] RepairPlan Candidate 展示 MiddlewareResource、RCA hypothesis、diagnostic steps、remediation steps、risk、rollback、verification、disabled conditions、failedPoint、RecoveryAttempt。
- [ ] Runbook / Workflow Draft 展示 source experience、semantic diff、risk summary、validation result、dry-run result、rollback and verification coverage、target environment。
- [ ] Memory Candidate 展示 scope、subjectRef、content summary、sourceRefs、confidence、ttl、redactionStatus、stale risk。
- [ ] OpsGraph Patch 展示 nodes、edges、operations、source evidence、before/after、risk、reviewer。
- [ ] Eval Case 展示 scenarioType、inputRefs、expectedBehavior、forbiddenBehavior、scoringRubric、linkedPromptTraceIds。
- [ ] 单测覆盖各 assetType 的关键字段和不可执行提示。

## 11. Task 8：实现 Activation Index

- [ ] 新增 `ActivationIndexPage.tsx`。
- [ ] 新增 `ActivationIndexTable.tsx`。
- [ ] 新增 `ActivationExplainDrawer.tsx`。
- [ ] 列表列包括 Entry、Scenario、Environment、Conditions、Confidence、Review、Usage、Action。
- [ ] 支持按 assetType、scenario、environment、status、disabled condition、fitness、reviewer 筛选。
- [ ] 匹配解释展示输入 scenario、匹配 asset、environment match、disabledConditions、confidence/fitness、why ranked higher、why candidates excluded、prompt layer impact、downstream matcher。
- [ ] 候选经验存在但未审核时，只显示数量和审核入口，不展示具体可执行建议。
- [ ] pause 操作需要 reviewer comment，立即从推荐中排除。
- [ ] deprecated 不删除资产，展示 supersededBy 或 deprecated reason。
- [ ] 注册 `/learning/activation-index`。
- [ ] 单测覆盖匹配解释、候选排除、pause、deprecated。

## 12. Task 9：实现 Memory Explorer

- [ ] 新增 `MemoryExplorerPage.tsx`。
- [ ] 新增 `MemoryDetailPage.tsx`。
- [ ] 新增 `MemoryRecordTable.tsx`。
- [ ] 新增 `MemoryImpactPanel.tsx`。
- [ ] 支持 scope：user、session、service、host、incident、org。
- [ ] 表格列包括 Memory、Content、Source、Confidence、TTL、Redaction、Impact、Action。
- [ ] Memory 详情展示 content summary、sourceRefs、redaction report、last used trace、prompt impact、linked Eval failures。
- [ ] 支持打开、标记 stale、延长 TTL、提升审核、删除候选。
- [ ] session memory 自动过期，不允许直接提升到 org。
- [ ] org memory 只能来自审核通过的证据。
- [ ] stale memory 不进入 PromptCompiler。
- [ ] redaction failed memory 不能进入任何 prompt。
- [ ] 注册 `/learning/memory` 和 `/learning/memory/:memoryId`。
- [ ] 单测覆盖 scope、TTL、stale、org promotion gate、redaction failed blocker。

## 13. Task 10：实现 Eval Workbench

- [ ] 新增 `EvalWorkbenchPage.tsx`。
- [ ] 新增 `EvalReportPage.tsx`。
- [ ] 新增 `EvalCaseTable.tsx`。
- [ ] 新增 `EvalRunForm.tsx`。
- [ ] 新增 `EvalReportSummary.tsx`。
- [ ] 新增 `EvalRegressionPanel.tsx`。
- [ ] EvalCase 支持 sourceCaseId、scenarioType、inputRefs、expectedBehavior、forbiddenBehavior、scoringRubric、linkedPromptTraceIds、linkedExperienceRefs、linkedMemoryRefs。
- [ ] 创建来源支持 Case close、Debug Trace、Middleware Repair、Workflow failure、Approval denied、Prompt Trace regression、Experience review。
- [ ] Eval Run 表单支持 eval suite、case category、model、prompt version、tool schema version、experience index version、memory snapshot、policy version、baseline report。
- [ ] 运行前展示覆盖场景、高风险 forbidden behavior、Activation Index、Candidate Store 隔离。
- [ ] Report 列表展示 Run、Change、Summary、Risk、Artifacts、Action。
- [ ] Report 详情展示 case scores、failed checks、forbidden behavior、regression vs baseline、Prompt Trace links、activated experience links、memory refs、diagnosis、recommended fixes。
- [ ] Eval failed 时禁用相关 publish / activation 按钮，除非有授权 override。
- [ ] 注册 `/learning/evals` 和 `/learning/evals/:reportId`，并兼容 `/ai-reasoning/evals`。
- [ ] 单测覆盖 forbidden behavior、candidate leakage、baseline regression、publish blocker。

## 14. Task 11：实现 Evolution Map 与 Lineage

- [ ] 新增 `EvolutionMapPage.tsx`。
- [ ] 新增 `AssetLineagePage.tsx`。
- [ ] 新增 `EvolutionEventTable.tsx`。
- [ ] 新增 `AssetLineageGraph.tsx`。
- [ ] 新增 `FitnessPanel.tsx`。
- [ ] 事件流展示 candidate.created、review.decided、activation.indexed、activation.paused、asset.deprecated、runbook.draft.generated、workflow.draft.generated、eval.completed、memory.promoted、opsgraph.patch.created、variant.created、incompatible.edge.created、fitness.updated。
- [ ] 事件可按 asset、case、environment、reviewer、source、time window 过滤。
- [ ] Lineage 使用固定布局 Source -> Candidate -> Approved Artifact -> Draft/Published Asset -> Activation/Outcome/Variant。
- [ ] 节点展示 status、review decision、semantic hash、environment profile、last outcome、fitness。
- [ ] Fitness 展示 success rate、activation count、recent failures、failure clusters、environment-specific failures、downgrade/pause reason、suggested variant。
- [ ] 注册 `/learning/evolution-map` 和 `/learning/lineage/:assetId`。
- [ ] 单测覆盖 lineage 节点、variant、incompatible edge、fitness downgrade。

## 15. Task 12：实现 Environment Profiles

- [ ] 新增 `EnvironmentProfileListPage.tsx`。
- [ ] 新增 `EnvironmentProfileDetailPage.tsx`。
- [ ] 新增 `EnvironmentProfileTable.tsx`。
- [ ] 新增 `EnvironmentVariantPanel.tsx`。
- [ ] 环境画像来源展示 runner agent、Coroot、只读命令探测、Workflow run、Debug Trace、MiddlewareResource、OpsGraph entity。
- [ ] 表格列包括 Profile、Runtime、Capabilities、Business、Middleware、Compatibility、Action。
- [ ] 支持打开详情、probe、创建变体、运行 Eval。
- [ ] 不兼容资产只能创建 variant candidate，不能修改原资产。
- [ ] 变体候选展示 parent asset、diff、review、validate、dry-run、Eval 要求。
- [ ] 注册 `/learning/environment-profiles` 和 `/learning/environment-profiles/:profileId`。
- [ ] 单测覆盖 environment mismatch、variant candidate、probe result、compatibility filter。

## 16. Task 13：跨模块集成

- [ ] IncidentWorkbenchPage 的 Assets tab 嵌入 Experience candidate、Runbook draft、Workflow draft、Memory candidate、OpsGraph patch、Eval case。
- [ ] DebugEvent / Observability 页面提供从 Debug Trace 生成 Debug RCA candidate 和 Eval case。
- [ ] Middleware Repair 页面提供生成 RepairPlan candidate、Capsule candidate、Eval case 和查看派生经验包。
- [ ] Workflow Run Detail 提供从成功 run 生成 Workflow draft / Capsule candidate，从失败 run 生成 anti-pattern / Eval case。
- [ ] RunnerStudioPage 对 source=experience-pack 的 workflow 展示 experience_pack_id、variant_key、review_record_id、evidence chain、environment scope。
- [ ] PromptTracePage 展示 activatedExperienceRefs、memoryRefs、evalLabels、candidate leakage warning、experience activation diff。
- [ ] Chat 回复只展示已激活经验来源，不展示 candidate 的具体推荐内容。
- [ ] OpsGraphPage 的 Asset Map 展示 approved runbooks、workflows、experience packs、memory snippets、Eval cases。
- [ ] PostmortemPage 引用 Experience candidate、Eval case、Memory candidate。
- [ ] 单测覆盖 Case、Debug、Repair、Runner、PromptTrace、OpsGraph、Postmortem 的跳转和候选隔离。

## 17. Task 14：路由、导航和权限

- [ ] 修改 `web/src/app/navigation.ts`，增加 Learning 导航项。
- [ ] 修改 `web/src/router.tsx`，注册 `/learning`、`/learning/experience-packs`、`/learning/experience-packs/:packId`、`/learning/review-queue`、`/learning/review-queue/:candidateId`、`/learning/activation-index`、`/learning/memory`、`/learning/memory/:memoryId`、`/learning/evals`、`/learning/evals/:reportId`、`/learning/evolution-map`、`/learning/lineage/:assetId`、`/learning/environment-profiles`、`/learning/environment-profiles/:profileId`。
- [ ] `/experience-packs` 继续可访问，内部复用新经验包库。
- [ ] viewer 只能查看脱敏摘要。
- [ ] reviewer 可以审核候选、request changes、reject。
- [ ] SRE / service owner / DBA 按 asset risk 和 scope 批准对应资产。
- [ ] admin 可以 pause、deprecate、配置 Activation Index 和 Eval gate。
- [ ] 无权限时隐藏 raw prompt、敏感 evidence、SecretRef、连接串、token、用户敏感输入。
- [ ] 单测覆盖权限不足、按钮禁用和敏感字段隐藏。

## 18. Task 15：事件、轮询和一致性

- [ ] 在 `learningAssets.ts` 中实现 review queue 和 eval run summary polling helper。
- [ ] Review queue、Eval run、Activation Index diff 只刷新 summary。
- [ ] 大证据链、raw diff、Prompt Trace 只在打开详情时加载。
- [ ] 接入统一事件时支持 candidate.created、review.decided、activation.updated、memory.promoted、eval.completed、lineage.updated。
- [ ] 页面刷新或重进后能从 API 恢复当前审核和 eval 状态。
- [ ] review、activate、pause、deprecate、run eval 操作要显示 running guard，避免重复提交。
- [ ] 单测覆盖刷新恢复、重复点击被禁用、polling 错误保留旧数据。

## 19. Task 16：测试与视觉检查

- [ ] 新增 `learningAssets.test.ts`。
- [ ] 新增 `learningViewModels.test.ts`。
- [ ] 新增 `learningComponents.test.tsx`。
- [ ] 扩展 `web/src/pages/complexPages.test.tsx` 覆盖 `/learning`、`/learning/experience-packs`、`/learning/review-queue`、`/learning/activation-index`、`/learning/memory`、`/learning/evals`、`/learning/evolution-map`。
- [ ] 新增或扩展 Playwright 用例，覆盖桌面和移动宽度下的 Learning Overview、Review Queue、Activation Explain、Eval Report。
- [ ] 运行 `npm --prefix web test -- learningAssets.test.ts learningViewModels.test.ts learningComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含视觉实现，启动 dev server 并用浏览器截图检查 `/learning`、`/learning/review-queue`、`/learning/activation-index`、`/learning/evals`。

## 20. 交付检查

- [ ] `/learning` 展示候选隔离、审核、Activation Index、Memory、Eval 和 drift 总览。
- [ ] `/learning/experience-packs` 按 candidate、review_pending、approved、rejected、published、deprecated 分组展示。
- [ ] `/learning/review-queue` 能审核 Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft、Memory candidate、OpsGraph patch、Eval case。
- [ ] 未审核候选不显示为可推荐、可执行或已发布资产。
- [ ] 审核详情展示 Evidence、AI Chat Impact、Artifact Diff、Debug / Repair Diff、Gate Result 和 Audit。
- [ ] 高风险候选缺少 rollback、verification、dry-run 或 Eval 时不能 approve。
- [ ] Activation Index 能解释命中、候选排除、disabled condition 和 prompt impact。
- [ ] Memory Explorer 展示 scope、sourceRefs、TTL、stale、redactionStatus 和 prompt impact。
- [ ] `org` memory 只能由审核通过的证据生成。
- [ ] Eval Workbench 能创建 EvalCase、运行 Eval、查看报告、定位 Prompt Trace 和阻断发布。
- [ ] Evolution Map 展示候选来源、激活、lineage、fitness、variant 和 incompatible edge。
- [ ] Experience Pack 审核通过后才能进入 AI Chat、DebugCase 或 Middleware Repair 推荐。
- [ ] 从经验包生成的 Runbook / Workflow draft 进入对应审核和 validate/dry-run gate。
- [ ] 所有请求走 `web/src/api/learningAssets.ts` 或统一 API client，不在页面内裸 `fetch`。
- [ ] 页面不创建私有 WebSocket/SSE。
- [ ] 受限证据、raw prompt、SecretRef、连接串、token、用户敏感输入不在正文、tooltip、title、aria-label 中泄露。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
