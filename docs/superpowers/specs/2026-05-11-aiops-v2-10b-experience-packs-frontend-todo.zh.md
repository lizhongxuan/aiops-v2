# aiops-v2 Experience Pack 自我演化前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-10a-experience-packs-frontend-design.zh.md](2026-05-11-aiops-v2-10a-experience-packs-frontend-design.zh.md)
来源专题：[2026-05-11-aiops-v2-10-experience-packs-design.zh.md](2026-05-11-aiops-v2-10-experience-packs-design.zh.md)
关联模块：[2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md](2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md)

## 1. 目标

把当前静态 `ExperiencePacksPage.tsx` 升级为 Experience Pack 自我演化专题页面：支持 API 驱动的经验包库、Candidate Store、Review Workbench、Artifact Studio、Variant Resolver、Activation Impact、Fitness & Drift、Evolution Map，以及从 Case、Debug Trace、Middleware Repair、Workflow Run 派生候选并通过审核进入 Activation Index、Runbook 或 Workflow。

## 2. 实施顺序

```text
Experience Pack API view-model
  -> Experience Overview
  -> Pack Library / Pack Detail
  -> Candidate Store / Candidate Detail
  -> Artifact Studio
  -> Review Workbench
  -> Variant Resolver
  -> Activation Impact
  -> Fitness & Drift
  -> Cross-module generation entries
  -> Tests and visual checks
```

优先复用 09 的 `learningAssets.ts` 和 `learning` 组件。如果 Experience Pack 专用逻辑过多，再增加 `web/src/api/experiencePacks.ts` 作为 thin wrapper，不新增第二套事实源。

## 3. 文件地图

新增：

- `web/src/api/experiencePacks.ts`：可选 thin wrapper，封装 Experience Pack 专题 API，内部复用 `learningAssets.ts` 类型和 request helper。
- `web/src/api/experiencePacks.test.ts`：Experience Pack 专用 normalize、gate、variant、fitness、redaction 测试。
- `web/src/pages/ExperiencePackOverviewPage.tsx`：Experience Pack 总览。
- `web/src/pages/ExperiencePackCandidateStorePage.tsx`：Candidate Store。
- `web/src/pages/ExperiencePackCandidateDetailPage.tsx`：候选详情。
- `web/src/pages/ExperiencePackReviewPage.tsx`：Review Workbench。
- `web/src/pages/ExperiencePackActivationPage.tsx`：Activation Impact。
- `web/src/pages/ExperiencePackVariantsPage.tsx`：Variant Resolver。
- `web/src/pages/ExperiencePackFitnessPage.tsx`：Fitness & Drift。
- `web/src/components/experience/ExperienceIsolationStrip.tsx`：Candidate Store、Activation Index、Runbook、Workflow、PromptCompiler、Eval Gate。
- `web/src/components/experience/ExperienceSummaryStrip.tsx`：候选、审核、变体、Fitness、Eval 统计。
- `web/src/components/experience/ExperiencePackLifecycleTabs.tsx`：All / Candidate / Review / Approved / Published / Rejected / Deprecated / Variants / Paused。
- `web/src/components/experience/ExperiencePackTable.tsx`：经验包表格。
- `web/src/components/experience/ExperiencePackHeader.tsx`：详情 Header。
- `web/src/components/experience/ExperienceEvidenceChain.tsx`：证据链。
- `web/src/components/experience/ExperienceCasePanel.tsx`：ExperienceCase。
- `web/src/components/experience/ExperienceCandidateStoreTable.tsx`：候选库表。
- `web/src/components/experience/ExperienceCandidateDetail.tsx`：候选详情。
- `web/src/components/experience/ExperienceArtifactStudio.tsx`：Artifact Studio 容器。
- `web/src/components/experience/GeneCandidatePanel.tsx`：Gene 展示。
- `web/src/components/experience/CapsuleCandidatePanel.tsx`：Capsule 展示。
- `web/src/components/experience/DebugRcaCandidatePanel.tsx`：Debug RCA 展示。
- `web/src/components/experience/RepairPlanCandidatePanel.tsx`：RepairPlan 展示。
- `web/src/components/experience/RunbookDraftCandidatePanel.tsx`：Runbook draft 展示。
- `web/src/components/experience/WorkflowDraftCandidatePanel.tsx`：Workflow graph draft 展示。
- `web/src/components/experience/ExperienceReviewGatePanel.tsx`：Gate Result。
- `web/src/components/experience/ExperienceReviewDecisionPanel.tsx`：审核动作。
- `web/src/components/experience/ExperiencePublishImpactPanel.tsx`：发布影响说明。
- `web/src/components/experience/VariantTreePanel.tsx`：变体树。
- `web/src/components/experience/VariantDiffPanel.tsx`：适配 diff。
- `web/src/components/experience/VariantMatchExplanation.tsx`：匹配顺序解释。
- `web/src/components/experience/ActivationImpactPanel.tsx`：Activation 影响。
- `web/src/components/experience/FitnessDriftPanel.tsx`：Fitness 与退化。
- `web/src/components/experience/ExperienceAuditPanel.tsx`：审核、发布、pause、deprecated 审计。
- `web/src/components/experience/experiencePackViewModels.ts`：Experience Pack 专用 view-model。
- `web/src/components/experience/experiencePackViewModels.test.ts`：view-model 单测。
- `web/src/components/experience/experienceComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/ExperiencePacksPage.tsx`：从静态数据迁移到 API 驱动，并复用 Pack Library / Detail 组件。
- `web/src/pages/ExperiencePackLibraryPage.tsx`：如果 09 已落地，补充 10 专题 tabs 和详情入口。
- `web/src/pages/ExperiencePackDetailPage.tsx`：补充 Evidence Chain、Experience Case、Artifacts、Variants、Fitness。
- `web/src/pages/ExperienceReviewQueuePage.tsx`：补充 10 专题审核动作和发布影响说明。
- `web/src/pages/RunnerStudioPage.tsx`：展示 source=experience-pack、experience_pack_id、variant_key、review_record_id、evidence chain、environment scope。
- `web/src/pages/RunbookCatalogPage.tsx`：只加载 approved/published，candidate 不进入正式目录。
- `web/src/pages/IncidentWorkbenchPage.tsx`：Case Assets 提供“生成经验候选”入口。
- `web/src/pages/PromptTracePage.tsx`：Chat / Prompt Trace 展示已激活经验来源和候选排除提示。
- `web/src/pages/CorootOverviewPage.tsx` 或 Observability 页面：Debug Trace 生成候选入口。
- `web/src/pages/PostmortemPage.tsx`：复盘页展示派生候选和审核状态。
- `web/src/router.tsx`：注册 `/experience-packs` 相关专题路由。
- `web/src/pages/complexPages.test.tsx`：补充 Experience Pack 专题路由测试。

## 4. Task 1：补充 Experience Pack API wrapper 与类型

- [ ] 新增或扩展 `web/src/api/experiencePacks.ts`。
- [ ] 复用 `learningAssets.ts` 的基础类型，补充 `ExperiencePackLifecycle`、`CandidateArtifactKind`、`ExperiencePackDetailView`、`ExperienceCandidateReviewView`、`ExperienceVariantView`、`FitnessDriftView`、`ActivationImpactView`。
- [ ] 实现 `listExperiencePacks(params)`，请求 `GET /api/v1/experience-packs`。
- [ ] 实现 `getExperiencePack(packId)`，请求 `GET /api/v1/experience-packs/{id}`。
- [ ] 实现 `listExperienceCandidates(params)`，通过 `GET /api/v1/experience-packs?status=candidate` 或 review queue 聚合。
- [ ] 实现 `getExperienceGate(packId)`，请求 `GET /api/v1/experience-packs/{id}/gate`。
- [ ] 实现 `reviewExperiencePack(packId, payload)`，请求 `POST /api/v1/experience-packs/{id}/review`。
- [ ] 实现 `generateRunbookDraft(packId, payload)`、`generateWorkflowDraft(packId, payload)`、`createVariant(packId, payload)`。
- [ ] 实现 `getActivationImpact(packId)`、`getFitness(packId)`、`pauseExperiencePack(packId, payload)`、`deprecateExperiencePack(packId, payload)`。
- [ ] 实现 `createFromDebugTrace(traceId, payload)`、`createFromRepairCase(caseId, payload)`。
- [ ] normalize 时补齐 evidenceChain、experienceCase、artifacts、variants、lineage、fitness、activationEntries、reviewRecords 默认值。
- [ ] normalize 时裁剪 raw prompt、stdout/stderr、connection string、token、cookie、SecretRef、SQL 参数和用户敏感输入。
- [ ] 新增 `experiencePacks.test.ts` 覆盖 candidate isolation、gate、variant、fitness、sensitive redaction。
- [ ] 运行 `npm --prefix web test -- experiencePacks.test.ts`。

## 5. Task 2：实现 Experience Pack view-model

- [ ] 新增 `web/src/components/experience/experiencePackViewModels.ts`。
- [ ] 实现 `getExperienceLifecycleLabel(lifecycle)`。
- [ ] 实现 `summarizeIsolationState(payload)`，输出 Candidate Store、Activation Index、Runbook Catalog、Workflow Library、PromptCompiler、Eval Gate。
- [ ] 实现 `deriveExperienceGate(candidate)`，检查 evidence、redaction、environment、risk、rollback、verification、dry-run、Eval、candidate isolation。
- [ ] 实现 `canSubmitReview(candidate, gate)` 和 `canApproveReview(candidate, gate)`。
- [ ] 实现 `summarizeEvidenceChain(evidenceChain)`，按 source 和 used_for 聚合。
- [ ] 实现 `buildVariantTree(pack)`，生成 base pack、variants、supersedes、incompatible_with。
- [ ] 实现 `explainVariantMatch(input, variants)`，输出精确变体、兼容变体、只读诊断、无匹配候选。
- [ ] 实现 `summarizeFitnessDrift(fitness)`，输出下降原因、失败聚类、建议动作。
- [ ] 实现 `buildActivationImpact(pack)`，输出 prompt layer、matcher、Chat/Debug/Repair 影响。
- [ ] 新增 `experiencePackViewModels.test.ts` 覆盖 candidate leak、gate blocker、variant match、fitness downgrade、activation impact。
- [ ] 运行 `npm --prefix web test -- experiencePackViewModels.test.ts`。

## 6. Task 3：实现 Experience Pack Overview

- [ ] 新增 `ExperiencePackOverviewPage.tsx`。
- [ ] 新增 `ExperienceIsolationStrip.tsx`。
- [ ] 新增 `ExperienceSummaryStrip.tsx`。
- [ ] Header 展示环境、时间窗口、新建候选、运行 Eval、刷新、导出审计。
- [ ] Isolation Strip 展示 Candidate Store、Activation Index、Runbook Catalog、Workflow Library、PromptCompiler、Eval Gate。
- [ ] Candidate leak 时显示高优先级告警，并链接 audit。
- [ ] Summary Strip 展示新候选、待审核、高风险候选、disabled condition、Runbook/Workflow draft、需要变体、Fitness 下降、Eval regression、pause/deprecated。
- [ ] Tabs 包含 Library、Candidates、Review、Activation、Variants、Fitness。
- [ ] 注册 `/experience-packs` 或在旧页面中嵌入 Overview。
- [ ] 单测覆盖 isolation strip、candidate leak、Eval regression、Fitness 下降。

## 7. Task 4：升级 Pack Library

- [ ] 新增 `ExperiencePackLifecycleTabs.tsx`。
- [ ] 新增 `ExperiencePackTable.tsx`。
- [ ] 修改 `ExperiencePacksPage.tsx`，从 `web/src/data/opsWorkspace.js` 静态数据迁移到 API 数据。
- [ ] 保留静态 fallback，仅用于 API unavailable 的只读演示状态，并明确标记为 degraded。
- [ ] Tabs 包含 All、Candidate、Review Pending、Approved、Published、Rejected、Deprecated、Variants、Paused。
- [ ] 支持 source、assetType、environment、scenario signature、risk、review decision、activation state、fitness、disabled condition、variant key、reviewer 筛选。
- [ ] 表格列包括 Pack、Scenario、Environment、Evidence、Artifacts、Review、Activation、Fitness、Action。
- [ ] 单测覆盖 candidate 不进入 published tab、fallback degraded、筛选、空态。

## 8. Task 5：实现 Pack Detail

- [ ] 新增或扩展 `ExperiencePackHeader.tsx`。
- [ ] 新增 `ExperienceEvidenceChain.tsx`。
- [ ] 新增 `ExperienceCasePanel.tsx`。
- [ ] 新增 `ExperienceAuditPanel.tsx`。
- [ ] Header 展示 pack id、title、status、version、source、scenario signature、environment profile、confidence、review state、activation state、fitness、last outcome。
- [ ] Header 动作包括打开审核、生成 Runbook Draft、生成 Workflow Draft、创建环境变体、运行 Eval、pause、deprecate、导出 evidence refs。
- [ ] Tabs 包含 Overview、Evidence Chain、Experience Case、Artifacts、Review & Gate、Activation、Variants、Fitness、Lineage。
- [ ] Evidence Chain 展示 Coroot、Frontend Debug、Trace backend、Middleware Repair、Chat、Workflow run、Runbook instance、Approval、Incident close、Outcome verification。
- [ ] Experience Case 展示 problem_signature、trace_signature、middleware_signature、actions_taken、outcome、verification、operator_decisions、environment_profile_id、activated_assets。
- [ ] 单测覆盖 evidence used_for、semantic hash、activation state、variant list、audit refs。

## 9. Task 6：实现 Candidate Store 和 Candidate Detail

- [ ] 新增 `ExperiencePackCandidateStorePage.tsx`。
- [ ] 新增 `ExperiencePackCandidateDetailPage.tsx`。
- [ ] 新增 `ExperienceCandidateStoreTable.tsx`。
- [ ] 新增 `ExperienceCandidateDetail.tsx`。
- [ ] Candidate 类型支持 Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow graph draft、Anti-pattern、Incompatibility edge、Environment variant、Verification improvement。
- [ ] 列表列包括 Candidate、Pack、Evidence、Gate、Impact、Risk、Status、Action。
- [ ] 候选详情顶部显示“当前候选未审核，不能进入 AI Chat 推荐、DebugCase 推荐、中间件修复推荐、Runbook 正式目录或 Workflow 正式目录”。
- [ ] 候选详情展示生成来源、synthesis reason、evidence coverage、redaction status、environment compatibility、disabled conditions、Gate Result、Review actions。
- [ ] 单测覆盖候选隔离文案、review submit、request evidence、rejected、redaction failed blocker。

## 10. Task 7：实现 Artifact Studio

- [ ] 新增 `ExperienceArtifactStudio.tsx`。
- [ ] 新增 `GeneCandidatePanel.tsx`，展示 trigger condition、recommendation、evidence refs、forbidden condition、activation scope、expected prompt impact。
- [ ] 新增 `CapsuleCandidatePanel.tsx`，展示 applicability、diagnostic steps、action steps、verification、rollback、failure boundary、environment compatibility。
- [ ] 新增 `DebugRcaCandidatePanel.tsx`，展示 page、user action、trace signature、slow span、service path、Coroot RCA、suggested remediation boundary、redaction status、after-trace verification。
- [ ] 新增 `RepairPlanCandidatePanel.tsx`，展示 MiddlewareResource、middleware signature、RCA、diagnostic steps、remediation steps、risk、approval、rollback、verification、disabled conditions、failedPoint。
- [ ] 新增 `RunbookDraftCandidatePanel.tsx`，展示 observe / decide / propose_action / verify / rollback steps、risk、evidence requirement、review comments、diff。
- [ ] 新增 `WorkflowDraftCandidatePanel.tsx`，展示 graph nodes/edges、variables、target selector、approval nodes、risk、rollback path、verification、validate/dry-run result、graph hash。
- [ ] 所有 artifact panel 都显示“候选不可直接执行”的约束。
- [ ] 单测覆盖每种 artifact 的关键字段和执行约束。

## 11. Task 8：实现 Review Workbench

- [ ] 新增 `ExperiencePackReviewPage.tsx`。
- [ ] 新增 `ExperienceReviewGatePanel.tsx`。
- [ ] 新增 `ExperienceReviewDecisionPanel.tsx`。
- [ ] 新增 `ExperiencePublishImpactPanel.tsx`。
- [ ] Gate Result 展示 Evidence completeness、Redaction passed、Environment compatibility、Disabled conditions、Risk policy、Rollback coverage、Verification coverage、Approval roles、Workflow validate、Workflow dry-run、Eval result、Candidate isolation。
- [ ] 任一 required gate failed 时 approve 按钮禁用，并显示阻断原因。
- [ ] 支持 approve_gene、approve_capsule、approve_debug_rca、approve_repair_plan、approve_as_runbook、approve_as_workflow_draft、approve_both、request_changes、reject、create_variant、pause_asset、deprecate_asset。
- [ ] 审核前显示发布影响：进入哪个索引或目录、影响哪些 AI Chat / Debug / Repair matcher、是否生成 Runbook / Workflow、是否需要 Eval、所需 reviewer。
- [ ] 所有审核动作写入 audit。
- [ ] 单测覆盖 gate failed、approve payload、request changes、publish impact。

## 12. Task 9：实现 Variant Resolver

- [ ] 新增 `ExperiencePackVariantsPage.tsx`。
- [ ] 新增 `VariantTreePanel.tsx`。
- [ ] 新增 `VariantDiffPanel.tsx`。
- [ ] 新增 `VariantMatchExplanation.tsx`。
- [ ] 变体树展示 base pack、linux/ubuntu/systemd/k8s、linux/centos/systemd/vm、macos/launchd/arm64、k8s-only/service-account 等 variant key。
- [ ] 每个变体展示 compatibility、forbidden actions、required actions、changed steps、added verification、review state、activation state、fitness。
- [ ] Diff 分类展示参数适配、动作适配、编排适配。
- [ ] 动作适配必须展示 action catalog、capability precheck、dry-run、reviewer。
- [ ] 匹配解释按精确环境变体、同 OS family 兼容变体、基础经验包只读诊断步骤、无匹配候选展示。
- [ ] 只可使用只读诊断时，隐藏 remediation recommendation。
- [ ] 单测覆盖 macOS launchd 变体、只读降级、action adaptation、dry-run blocker。

## 13. Task 10：实现 Activation Impact

- [ ] 新增 `ExperiencePackActivationPage.tsx`。
- [ ] 新增 `ActivationImpactPanel.tsx`。
- [ ] 展示 ActivationIndexEntry、scenario signature、environment profile、disabled conditions、confidence、fitness、review record。
- [ ] 展示 prompt layer impact 和 matcher impact：PromptCompiler、Runbook matcher、Workflow recommender、Debug RCA matcher、RepairPlan matcher。
- [ ] 展示 candidate excluded count、last activated case、last outcome。
- [ ] Chat 集成文案只展示已审核经验来源、环境和成功率。
- [ ] 存在候选但未审核时，只显示数量和审核入口，不展示具体建议。
- [ ] disabled condition 生效时显示经验不适用原因。
- [ ] 单测覆盖 activation impact、candidate excluded、disabled condition、Chat 文案。

## 14. Task 11：实现 Fitness & Drift

- [ ] 新增 `ExperiencePackFitnessPage.tsx`。
- [ ] 新增 `FitnessDriftPanel.tsx`。
- [ ] 展示 activation_count、success_rate、mttr_delta、rollback_rate、rejection_rate、environment_mismatch_rate、cost、freshness、recent failures、failure clusters。
- [ ] 支持手动 pause、创建修复候选、创建环境变体、创建 Eval case、deprecate、打开 lineage。
- [ ] 降权只更新 Activation Index 排序或状态，不修改资产本身。
- [ ] pause 和 deprecate 必须填写 reviewer comment，并写 audit。
- [ ] 单测覆盖 fitness downgrade、pause、deprecated、variant candidate、Eval case creation。

## 15. Task 12：跨模块生成入口

- [ ] IncidentWorkbenchPage / Case Assets 增加从 case close 生成经验候选入口。
- [ ] Observability / DebugEvent 页面增加从 Debug Trace 生成 Debug RCA candidate 入口。
- [ ] Middleware Repair 页面增加从 repair case 生成 RepairPlan / Capsule candidate 入口。
- [ ] Workflow Run Detail 增加从成功 run 生成 Workflow draft / Capsule candidate，从失败 run 生成 Anti-pattern / Eval case。
- [ ] Runbook Instance 增加从实例结果生成 Runbook draft variant 或 disabled condition candidate。
- [ ] PostmortemPage 展示派生候选和审核状态。
- [ ] 所有入口只调用后端生成 API，不在前端拼接候选正文。
- [ ] 单测覆盖 case、debug trace、repair case、workflow run、postmortem 入口。

## 16. Task 13：Runner / Runbook / Chat 集成

- [ ] RunnerStudioPage 对经验包来源 draft 展示 source=experience-pack、experience_pack_id、variant_key、review_record_id。
- [ ] Runner publish modal 展示审核人、证据链、环境适用范围、graph hash、dry-run 状态。
- [ ] RunbookCatalogPage 只展示 approved/published，不展示 candidate。
- [ ] RunbookDetailPage 展示 source experience、review record、lineage 和 environment scope。
- [ ] PromptTracePage 展示 activatedExperienceRefs、candidate leakage warning 和 experience activation diff。
- [ ] Chat 回复只显示已激活经验，不显示候选具体推荐内容。
- [ ] 单测覆盖 candidate 不进入 Runbook/Workflow 正式列表，Chat 不泄露 candidate 建议。

## 17. Task 14：路由、导航和权限

- [ ] 修改 `web/src/router.tsx`，注册 `/experience-packs`、`/experience-packs/:packId`、`/experience-packs/candidates`、`/experience-packs/candidates/:candidateId`、`/experience-packs/review`、`/experience-packs/review/:reviewId`、`/experience-packs/activation`、`/experience-packs/variants`、`/experience-packs/evolution-map`、`/experience-packs/lineage/:assetId`、`/experience-packs/fitness/:assetId`。
- [ ] 保持 `/learning/experience-packs` 与 `/experience-packs` 共享组件。
- [ ] viewer 只能查看脱敏摘要。
- [ ] reviewer 可以 submit review、request changes、reject。
- [ ] SRE/service owner/DBA 按风险和 scope approve。
- [ ] admin 可以 pause、deprecate、处理 candidate leak。
- [ ] 无权限时隐藏 raw prompt、敏感 evidence、SecretRef、连接串、token、用户敏感输入。
- [ ] 单测覆盖权限不足、按钮禁用、敏感字段隐藏。

## 18. Task 15：事件、轮询和一致性

- [ ] 在 `experiencePacks.ts` 或 `learningAssets.ts` 中实现 candidate、review、fitness、eval summary polling helper。
- [ ] Candidate、review、fitness、eval 状态 10-30 秒刷新 summary。
- [ ] Evidence raw ref、Prompt Trace、Workflow graph diff 只在打开详情时加载。
- [ ] 接入统一事件时支持 experience.candidate.created、experience.review.decided、activation.indexed、variant.created、fitness.updated、asset.paused、asset.deprecated。
- [ ] 页面刷新或重进后能从 API 恢复当前候选、审核和激活状态。
- [ ] review、generate draft、create variant、pause、deprecate 操作要显示 running guard，避免重复提交。
- [ ] 单测覆盖刷新恢复、重复点击禁用、polling 错误保留旧数据。

## 19. Task 16：测试与视觉检查

- [ ] 新增 `experiencePacks.test.ts`。
- [ ] 新增 `experiencePackViewModels.test.ts`。
- [ ] 新增 `experienceComponents.test.tsx`。
- [ ] 扩展 `web/src/pages/complexPages.test.tsx` 覆盖 `/experience-packs`、`/experience-packs/candidates`、`/experience-packs/review`、`/experience-packs/activation`、`/experience-packs/variants`、`/experience-packs/fitness/asset-1`。
- [ ] 新增或扩展 Playwright 用例，覆盖桌面和移动宽度下的 Pack Library、Candidate Detail、Review Gate、Variant Resolver、Fitness 页面。
- [ ] 运行 `npm --prefix web test -- experiencePacks.test.ts experiencePackViewModels.test.ts experienceComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含视觉实现，启动 dev server 并用浏览器截图检查 `/experience-packs`、`/experience-packs/candidates`、`/experience-packs/review`。

## 20. 交付检查

- [ ] `/experience-packs` API 驱动，支持 candidate、review_pending、published、rejected、deprecated tabs。
- [ ] Candidate Store 和 Activation Index 强隔离，candidate 不显示为可推荐、可执行或已发布。
- [ ] 候选详情展示 synthesis reason、evidence coverage、redaction status、environment compatibility、disabled conditions 和 Gate Result。
- [ ] Review Workbench 展示 Evidence、AI Chat Impact、Artifact Diff、Debug / Repair Diff、Gate Result 和 Audit。
- [ ] Gene/Capsule/Debug RCA/RepairPlan 只有审核通过后才能进入 Activation Index。
- [ ] Runbook draft 和 Workflow graph draft 只能进入对应发布流程，不能直接变成生产执行资产。
- [ ] 环境不匹配时只能创建 variant candidate，不能修改原发布资产。
- [ ] Variant Resolver 能解释匹配顺序和只读降级原因。
- [ ] Fitness 下降能触发 pause、deprecate、variant candidate 或 Eval case，但不自动修改发布资产。
- [ ] Chat 只展示已激活经验来源，候选只显示数量和审核入口。
- [ ] Debug Trace 和 Middleware Repair 能生成经验候选，并保留脱敏、验证和失败点。
- [ ] Runner Studio 展示 experience_pack_id、variant_key、review_record_id、evidence chain 和 environment scope。
- [ ] 所有审核、发布、拒绝、变体生成、pause、deprecated 都有 audit record。
- [ ] 所有请求走 `web/src/api/learningAssets.ts`、`web/src/api/experiencePacks.ts` wrapper 或统一 API client，不在页面内裸 `fetch`。
- [ ] 页面不创建私有 WebSocket/SSE。
- [ ] 敏感证据、raw prompt、SecretRef、连接串、token、用户敏感输入不在正文、tooltip、title、aria-label 中泄露。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
