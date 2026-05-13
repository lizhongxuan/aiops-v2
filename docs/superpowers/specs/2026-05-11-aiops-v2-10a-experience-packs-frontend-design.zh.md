# aiops-v2 Experience Pack 自我演化前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源专题：[2026-05-11-aiops-v2-10-experience-packs-design.zh.md](2026-05-11-aiops-v2-10-experience-packs-design.zh.md)
关联模块：[2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md](2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-10b-experience-packs-frontend-todo.zh.md](2026-05-11-aiops-v2-10b-experience-packs-frontend-todo.zh.md)

## 1. 页面目标

Experience Pack 自我演化前端不是“经验库 CRUD”，也不是“自动把成功步骤发布为最佳实践”。它是围绕 Simple EvoMap 和 Experience Pack 的自我演化审计界面，负责让用户看清楚：系统从哪些真实运维事件中生成了什么候选、候选为什么能或不能被激活、如何派生 SkillCard / Runbook / Workflow / 变体、以及已激活资产在后续运行中表现如何。

页面要让用户完成：

- 从 Coroot 事故、AI Chat、Debug Trace、中间件修复、Workflow run、Runbook instance、Approval、Postmortem、Verification outcome 中查看自动生成的 Experience candidate。
- 查看 Experience Pack 的 evidence chain、environment profile、problem signature、candidate artifacts、review record、lineage、fitness、variants 和 activation 状态。
- 审核 SkillCard、Debug RCA、RepairPlan、Runbook draft、Workflow graph draft、Anti-pattern、Incompatibility edge 和 Environment variant；Gene/Capsule 只作为后续高级形态。
- 明确候选和已激活资产的隔离边界：candidate 永远不进入 AI Chat 推荐、DebugCase 推荐、中间件修复推荐、正式 Runbook 或正式 Workflow。
- 将审核通过的 SkillCard/Debug RCA/RepairPlan 写入 Activation Index，或将 Runbook/Workflow draft 推入对应发布流程。
- 查看 AI Chat 命中了哪些已审核经验，以及为什么未使用未审核候选。
- 查看 Debug Trace 和 Middleware Repair 如何生成候选，以及失败修复如何沉淀为 disabled condition、anti-pattern 或 incompatibility edge。
- 对已发布资产创建环境变体，但不原地修改已发布 SkillCard / Runbook / Workflow。
- 查看 Fitness、失败聚类、环境不兼容、降权、pause、deprecated 和 supersedes 关系。
- 通过 Eval 和 dry-run 证明经验变更没有引入越权、错误推荐、candidate leakage 或危险动作。

设计原则：

- 第一屏展示候选隔离、审核阻塞、Activation 影响和风险门禁，不展示“自动发布”。
- Experience Pack 是演化容器，SkillCard/Debug RCA/RepairPlan/Runbook/Workflow 是简单版可派生资产；Gene/Capsule 后续可从 SkillCard 再拆分。
- Candidate Store 与 Activation Index 必须在页面上强隔离，用不同状态、文案和动作表达。
- 审核动作必须先看 Gate Result：证据、环境、风险、回滚、验证、dry-run、Eval、权限。
- 已发布资产不可变。任何环境适配、流程优化或失败修复都生成新候选或新版本。
- 失败经验与成功经验同等重要，失败候选必须突出失败点、禁用条件、环境不兼容和下一步验证。
- 页面只展示脱敏摘要、引用和 hash，不展示完整 stdout/stderr、连接串、token、cookie、SecretRef、SQL 参数或用户敏感输入。

## 2. 与 09 Learning 前端的边界

09a 是 Learning 资产控制台总设计，覆盖 Experience、Memory、Eval、Activation Index、Evolution Map 和 Environment Profile 的整体入口。

10a 是 Experience Pack 自我演化的专题细化，重点回答：

- Experience Pack 候选如何从真实运行中生成。
- 候选详情如何展示 evidence、diff、Gate Result 和发布影响。
- SkillCard、Debug RCA、RepairPlan、Runbook draft、Workflow draft 如何审核和派生；Gene/Capsule 仅作为后续高级拆分。
- 环境变体如何创建、比较、审核、验证。
- Fitness 如何影响 Activation Index 排序、降权、pause 和 deprecated。
- Experience Pack 如何与 AI Chat、Debug Trace、Middleware Repair、现有 Runner workflow metadata、Runbook Catalog、Postmortem 和 Eval 互相跳转。

因此 10a 可以复用 09a 的 Learning 导航和 API 基础，但页面和组件应提供更深的 Experience Pack 专用表达。

## 3. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/ExperiencePacksPage.tsx`：已有静态经验包列表、搜索、详情、版本演进和关联资源。
- `web/src/data/opsWorkspace.js`：已有经验包 mock 数据，包括 Redis 只读故障切换等样例。
- `web/src/pages/IncidentWorkbenchPage.tsx`：已有事故详情、证据、假设、Runbook 匹配、执行实例和复盘草稿入口。
- `web/src/pages/RunbookCatalogPage.tsx`、`RunbookDetailPage.tsx`：可承接 Runbook draft 发布和来源 lineage。
- 现有 Runner workflow API / catalog：可承接 Workflow graph draft、validate、dry-run、publish 和 metadata；不要求改 Runner Studio 页面。
- `web/src/pages/PromptTracePage.tsx`：可展示 activatedExperienceRefs、candidate leakage warning 和 Eval trace。
- `web/src/pages/OpsGraphPage.tsx`：可展示 Experience Pack、Memory、EvalCase、OpsGraph patch 与实体关联。
- `web/src/pages/PostmortemPage.tsx`：可引用 Experience candidate、follow-up、verification 和 audit。
- `internal/eval`、`internal/promptdiag`：已有 eval case、report、diagnosis 和 prompt trace 关联能力。

主要不足：

- `ExperiencePacksPage.tsx` 还没有 API 驱动，无法表达 candidate、review_pending、approved、published、deprecated 的完整生命周期。
- 没有 Experience Case、Candidate Synthesizer、Review Queue、Variant Resolver、Fitness、Activation explanation 的前端对象。
- 没有从 Debug Trace 和 Middleware Repair 进入候选详情的稳定页面。
- 没有 Runbook / Workflow draft 的专用差异展示和 publish gate 说明。
- 没有 Variant tree、environment profile compatibility 和 incompatible_with 关系展示。
- 没有 Fitness 降权、pause、deprecated、supersedes 的操作和 audit 页面。
- 没有 Candidate Store 和 Activation Index 的隔离证明页面。

## 4. 路由与信息架构

建议新增或复用以下路由：

```text
/experience-packs
/experience-packs/:packId
/experience-packs/candidates
/experience-packs/candidates/:candidateId
/experience-packs/review
/experience-packs/review/:reviewId
/experience-packs/activation
/experience-packs/activation/:entryId
/experience-packs/variants
/experience-packs/variants/:variantId
/experience-packs/evolution-map
/experience-packs/lineage/:assetId
/experience-packs/fitness/:assetId
```

与 Learning 路由兼容：

```text
/learning/experience-packs
/learning/experience-packs/:packId
/learning/review-queue/:candidateId
/learning/activation-index
/learning/evolution-map
/learning/lineage/:assetId
```

路由语义：

- `/experience-packs`：经验包库，兼容旧入口，展示 packs、candidates、review、published、deprecated。
- `/experience-packs/:packId`：单个经验包详情，展示 evidence chain、artifacts、review、activation、variants、fitness。
- `/experience-packs/candidates`：Candidate Store 视图，只展示未激活候选。
- `/experience-packs/candidates/:candidateId`：候选详情和审核入口。
- `/experience-packs/review`：Review Queue，按 gate violation、风险、SLA、来源排序。
- `/experience-packs/activation`：Activation Index 视图，只展示已审核可推荐资产。
- `/experience-packs/variants`：环境变体列表，展示 base pack、variant key、compatibility、diff、review。
- `/experience-packs/evolution-map`：Evolution Map 专题视图，展示事件、节点、边和运行结果。
- `/experience-packs/fitness/:assetId`：单资产 fitness、降权、失败聚类和 pause/deprecated 操作。

MVP 可以先把 `/experience-packs` 升级为 Experience Pack 专题页，在内部用 tabs 承载 Candidate、Review、Activation、Variants、Fitness；后续再拆详情路由。

主导航分区：

| 页面 | 目标 |
| --- | --- |
| Experience Overview | 候选隔离、审核阻塞、Activation 影响、Fitness 风险 |
| Pack Library | 经验包列表、状态、场景、证据、环境、版本和变体 |
| Candidate Store | 只展示候选，不提供任何推荐或执行入口 |
| Review Workbench | Gate Result、Artifact Diff、审核动作和发布影响 |
| Artifact Studio | Gene、Capsule、Debug RCA、RepairPlan、Runbook、Workflow 详情 |
| Variant Resolver | 环境兼容、变体建议、适配 diff、dry-run 和 Eval |
| Activation Index | 已审核推荐索引、匹配解释、候选排除说明 |
| Fitness & Drift | success rate、失败聚类、降权、pause、deprecated |
| Evolution Map | 来源事件、lineage、outcome、supersedes、incompatible_with |

## 5. Experience Overview

Experience Overview 回答“系统自动沉淀的经验是否可控，是否有候选错误进入推荐，哪些资产正在退化”。

### 5.1 布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: Experience Packs / 环境 / 时间窗口 / 新建候选 / 运行 Eval │
├────────────────────────────────────────────────────────────┤
│ Isolation Strip: Candidate Store / Activation Index / Runbook / Workflow │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: Candidates / Review Pending / Variants / Fitness / Eval │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Detail Drawer           │
│ Library / Candidates / Review │ Evidence / Gate / Impact /   │
│ / Activation / Variants       │ Diff / Lineage               │
└──────────────────────────────┴─────────────────────────────┘
```

### 5.2 Isolation Strip

| 区域 | 状态 | 说明 |
| --- | --- | --- |
| Candidate Store | healthy / violation / unavailable | candidate 是否只在候选库中可见 |
| Activation Index | stable / changed / stale | 已审核资产是否可用于推荐 |
| Runbook Catalog | clean / candidate_leak / degraded | 正式 Runbook 是否只包含 approved/published |
| Workflow Library | clean / draft_pending / candidate_leak | Workflow 是否保留 draft/published 生命周期 |
| PromptCompiler | clean / candidate_leak / unknown | prompt 是否只读取 Activation Index |
| Eval Gate | passed / failed / missing | 最近经验变更是否已回归 |

Candidate leak 必须显示高优先级告警，并链接到 audit 和相关 asset。

### 5.3 Summary Strip

指标：

- 新候选数。
- 待审核数。
- 高风险候选数。
- 触发 disabled condition 的候选数。
- 待生成 Runbook / Workflow draft 数。
- 需要环境变体数。
- Fitness 下降资产数。
- Eval regression 数。
- pause / deprecated 数。

## 6. Pack Library

Pack Library 回答“经验包在生命周期中处于哪个状态，有哪些可审核或可发布产物”。

### 6.1 Tabs

- All。
- Candidate。
- Review Pending。
- Approved。
- Published。
- Rejected。
- Deprecated。
- Variants。
- Paused。

### 6.2 筛选

- source：coroot、chat、frontend_debug、trace、middleware_repair、workflow_run、approval_audit、postmortem、verification。
- assetType：gene、capsule、debug_rca、repair_plan、runbook、workflow。
- environment：os、runtime、namespace、service、agent capability。
- scenario signature。
- risk。
- review decision。
- activation state。
- fitness range。
- disabled condition。
- variant key。
- reviewer。

### 6.3 列表列

| 列 | 内容 |
| --- | --- |
| Pack | title、id、version、status、semantic hash |
| Scenario | symptoms、entities、source case、problem signature |
| Environment | environment profile、variant key、compatibility |
| Evidence | evidence count、redaction、verification outcome |
| Artifacts | Gene、Capsule、Debug RCA、RepairPlan、Runbook、Workflow |
| Review | required roles、decision、reviewer、comment |
| Activation | eligible、indexedAt、paused、deprecated |
| Fitness | activation count、success rate、recent failures |
| Action | 打开、审核、生成 Draft、创建变体、运行 Eval |

## 7. Pack Detail

Pack Detail 回答“这个经验包从哪里来、包含什么、能否被复用、是否已经影响推荐”。

### 7.1 Header

显示：

- pack id。
- title。
- status。
- version。
- source。
- scenario signature。
- environment profile。
- confidence。
- review state。
- activation state。
- fitness。
- last outcome。

动作：

- 打开审核。
- 生成 Runbook Draft。
- 生成 Workflow Draft。
- 创建环境变体。
- 运行 Eval。
- pause。
- deprecate。
- 导出 evidence refs。

### 7.2 Tabs

| Tab | 目标 |
| --- | --- |
| Overview | 状态、场景、环境、风险、下一步 |
| Evidence Chain | Coroot、Chat、Debug Trace、Middleware Repair、Workflow、Approval、Verification、Postmortem |
| Experience Case | problem signature、actions_taken、outcome、operator decisions、activated assets |
| Artifacts | SkillCard、Debug RCA、RepairPlan、Runbook draft、Workflow draft；Gene/Capsule 后续高级形态 |
| Review & Gate | Gate Result、review history、audit、阻断项 |
| Activation | Activation Index entry、Chat/Debug/Repair 推荐影响 |
| Variants | base pack、variant tree、compatibility、diff |
| Fitness | success rate、rollback rate、rejection rate、environment mismatch、cost、freshness |
| Lineage | derived_from、variant_of、supersedes、incompatible_with、verified_by |

### 7.3 Evidence Chain

Evidence Chain 展示每条证据的来源和用途：

- Coroot webhook / tools。
- Browser Plugin Debug / Trace backend。
- Middleware Repair。
- Chat / Runtime event。
- Workflow run。
- Runbook instance。
- Approval / Audit。
- Incident close。
- Outcome verification。

每条证据显示：

- EvidenceRef。
- source。
- time window。
- redaction status。
- summary hash。
- raw ref。
- used_for：synthesis、review、activation、eval、verification。

受限证据只显示 id、source、restriction reason，不在正文、tooltip、title、aria-label 中泄露。

## 8. Candidate Store

Candidate Store 回答“系统自动生成了哪些候选，但还不能影响任何推荐”。

### 8.1 Candidate 类型

- Gene candidate。
- Capsule candidate。
- Debug RCA candidate。
- RepairPlan draft。
- Runbook draft。
- Workflow graph draft。
- Anti-pattern candidate。
- Incompatibility edge candidate。
- Environment variant candidate。
- Verification improvement candidate。

### 8.2 列表列

| 列 | 内容 |
| --- | --- |
| Candidate | id、type、title、source |
| Pack | base pack、variant key、lineage |
| Evidence | sourceRefs、verification、redaction |
| Gate | evidence、environment、risk、rollback、verification、dry-run、eval |
| Impact | 如果批准，会影响哪些推荐或发布 |
| Risk | max risk、destructive、data risk、permission |
| Status | candidate、review_pending、changes_requested、rejected |
| Action | 打开、提交审核、请求证据、拒绝、创建变体 |

### 8.3 候选详情

候选详情顶部必须显示：

```text
当前候选未审核，不能进入 AI Chat 推荐、DebugCase 推荐、中间件修复推荐、Runbook 正式目录或 Workflow 正式目录。
```

并显示：

- 生成来源。
- Synthesis reason。
- Evidence coverage。
- Redaction status。
- Environment compatibility。
- Disabled conditions。
- Gate Result。
- Review actions。

## 9. Artifact Studio

Artifact Studio 回答“候选具体会变成什么资产”。

### 9.1 Gene

展示：

- trigger condition。
- recommendation。
- evidence refs。
- forbidden condition。
- activation scope。
- expected prompt impact。

Gene 只能影响推荐，不允许绑定执行动作。

### 9.2 Capsule

展示：

- applicability。
- diagnostic steps。
- action steps。
- verification。
- rollback。
- failure boundary。
- environment compatibility。
- derivable Runbook / Workflow。

### 9.3 Debug RCA

展示：

- page。
- user action。
- trace signature。
- slow span location。
- service path。
- Coroot RCA。
- suggested remediation boundary。
- redaction status。
- replay / after-trace verification。

### 9.4 RepairPlan

展示：

- MiddlewareResource。
- middleware signature。
- RCA。
- diagnostic steps。
- remediation steps。
- risk。
- approval。
- rollback。
- verification。
- disabled conditions。
- failedPoint。

### 9.5 Runbook Draft

展示：

- steps。
- observe / decide / propose_action / verify / rollback。
- risk。
- evidence requirement。
- review comments。
- diff from previous version。
- publish target。

Runbook draft 不能直接执行工具，只能发布到 Runbook catalog 后按 06 的规则使用。

### 9.6 Workflow Graph Draft

展示：

- graph nodes and edges。
- variables。
- target selector。
- approval nodes。
- action risk。
- rollback path。
- verification。
- validate result。
- dry-run result。
- graph hash。

Workflow draft 必须进入现有 Runner workflow validate / dry-run / publish gate；Experience Pack 页面展示 gate 结果、graph hash 和 metadata，不要求改 Runner 页面。

## 10. Review Workbench

Review Workbench 回答“这个候选能否被固化，固化后会影响什么”。

### 10.1 Gate Result

Gate Result 项：

- Evidence completeness。
- Redaction passed。
- Environment compatibility。
- Disabled conditions。
- Risk policy。
- Rollback coverage。
- Verification coverage。
- Approval roles。
- Workflow validate。
- Workflow dry-run。
- Eval result。
- Candidate isolation。

任一 required gate failed 时，approve 按钮禁用，并显示阻断原因。

### 10.2 审核动作

| 动作 | 结果 |
| --- | --- |
| approve_gene | 写入 Activation Index |
| approve_capsule | 写入 Activation Index |
| approve_debug_rca | 写入 Activation Index |
| approve_repair_plan | 写入 Activation Index |
| approve_as_runbook | 写入 Runbook draft / reviewed / published 流程 |
| approve_as_workflow_draft | 写入 Runner workflow draft |
| approve_both | 同时生成 Runbook 和 Workflow draft |
| request_changes | 保留候选并记录 reviewer 意见 |
| reject | 保留证据但不进入推荐 |
| create_variant | 创建新环境变体候选 |
| pause_asset | 从 Activation Index 排除 |
| deprecate_asset | 保留历史但不再推荐 |

### 10.3 发布影响说明

审核前必须展示：

- 会进入哪个索引或目录。
- 会影响哪些 AI Chat 场景。
- 会影响哪些 DebugCase / RepairCase matcher。
- 是否会生成 Runbook / Workflow draft。
- 是否需要 Eval。
- 是否需要 service owner / DBA / SRE 审核。

## 11. Variant Resolver

Variant Resolver 回答“当前经验是否适合这个环境，不适合时如何派生安全变体”。

### 11.1 变体树

展示：

```text
base experience pack
  -> linux/ubuntu/systemd/k8s
  -> linux/centos/systemd/vm
  -> macos/launchd/arm64
  -> k8s-only/service-account
```

每个变体显示：

- variant key。
- compatibility。
- forbidden actions。
- required actions。
- changed steps。
- added verification。
- review state。
- activation state。
- fitness。

### 11.2 适配 diff

Diff 分类：

- 参数适配：host、namespace、service、threshold、timeout、路径。
- 动作适配：systemctl -> launchctl、apt -> yum/brew、裸机命令 -> kubectl rollout。
- 编排适配：单机顺序流程 -> 多 pod 并行、readiness gate、approval node。

动作适配必须显示 action catalog、capability precheck、dry-run 和 reviewer。

### 11.3 匹配规则解释

Variant Resolver 解释匹配顺序：

1. 精确环境变体。
2. 同 OS family 兼容变体。
3. 基础经验包只读诊断步骤。
4. 无匹配时生成待审核候选。

如果只可使用只读诊断，页面必须隐藏 remediation recommendation。

## 12. Activation Impact

Activation Impact 回答“这个经验被激活后，下一次对话或修复会怎样变化”。

展示：

- ActivationIndexEntry。
- scenario signature。
- environment profile。
- disabled conditions。
- confidence。
- fitness。
- review record。
- prompt layer impact。
- matcher impact：PromptCompiler、Runbook matcher、Workflow recommender、Debug RCA matcher、RepairPlan matcher。
- candidate excluded count。
- last activated case。
- last outcome。

Chat 集成文案：

- 命中已审核 Gene / Capsule / Debug RCA / RepairPlan 时显示来源、环境和成功率。
- 存在候选但未审核时只显示“有待审核候选”，不展示具体建议。
- disabled condition 生效时显示“经验不适用”的原因。

## 13. Fitness & Drift

Fitness & Drift 回答“已激活经验是否仍然可靠”。

### 13.1 指标

- activation_count。
- success_rate。
- mttr_delta。
- rollback_rate。
- rejection_rate。
- environment_mismatch_rate。
- cost。
- freshness。
- recent failures。
- failure clusters。

### 13.2 操作

- 手动 pause。
- 创建修复候选。
- 创建环境变体。
- 创建 Eval case。
- deprecate。
- 打开 lineage。

降权不修改资产本身，只更新 Activation Index 排序或状态，并写 audit。

## 14. 数据模型

建议在 09 的 `web/src/api/learningAssets.ts` 基础上补充 Experience Pack 专用 view-model；如果后续文件过大，可拆出 `web/src/api/experiencePacks.ts` 作为 thin wrapper。

```ts
export type ExperiencePackLifecycle =
  | "candidate"
  | "review_pending"
  | "approved"
  | "rejected"
  | "published"
  | "paused"
  | "deprecated";

export type CandidateArtifactKind =
  | "gene"
  | "capsule"
  | "debug_rca"
  | "repair_plan"
  | "runbook_draft"
  | "workflow_graph_draft"
  | "anti_pattern"
  | "incompatibility_edge"
  | "environment_variant";

export type ExperiencePackDetailView = {
  id: string;
  title: string;
  lifecycle: ExperiencePackLifecycle;
  version: string;
  semanticHash: string;
  scenario: ScenarioSignatureView;
  environmentProfile: EnvironmentProfileView;
  evidenceChain: ExperienceEvidenceView[];
  experienceCase: ExperienceCaseView;
  artifacts: CandidateArtifactView[];
  reviewRecords: ReviewRecordView[];
  activationEntries: ActivationIndexEntryView[];
  variants: ExperienceVariantView[];
  lineage: AssetLineageView;
  fitness: FitnessView;
  disabledConditions: DisabledConditionView[];
};

export type ExperienceCandidateReviewView = {
  id: string;
  packId: string;
  artifactKind: CandidateArtifactKind;
  source: "coroot" | "chat" | "frontend_debug" | "trace" | "middleware_repair" | "workflow_run" | "approval_audit" | "postmortem" | "verification";
  gate: ReviewGateResultView;
  impact: ActivationImpactView;
  diff: ArtifactDiffView;
  auditRefs: string[];
};
```

## 15. API 契约

页面优先使用 10 专题 API：

```text
GET    /api/v1/experience-packs
GET    /api/v1/experience-packs/{id}
POST   /api/v1/experience-packs/match
GET    /api/v1/experience-packs/review-queue
POST   /api/v1/experience-packs/{id}/review
POST   /api/v1/experience-packs/from-debug-trace
POST   /api/v1/experience-packs/from-repair-case
POST   /api/v1/experience-packs/{id}/generate-runbook-draft
POST   /api/v1/experience-packs/{id}/generate-workflow-draft
POST   /api/v1/experience-packs/{id}/variants
GET    /api/v1/debug-traces/{trace_id}/experience-candidates
GET    /api/v1/repair-cases/{case_id}/experience-candidates
GET    /api/v1/evolution-map/entities/{id}
GET    /api/v1/evolution-map/lineage/{asset_id}
GET    /api/v1/evolution-map/events
POST   /api/v1/evolution-map/query
GET    /api/v1/activation-index/search
GET    /api/v1/environment-profiles
GET    /api/v1/environment-profiles/{id}
POST   /api/v1/environment-profiles/probe
```

建议补充：

```text
GET    /api/v1/experience-packs/{id}/gate
GET    /api/v1/experience-packs/{id}/activation-impact
GET    /api/v1/experience-packs/{id}/fitness
POST   /api/v1/experience-packs/{id}/pause
POST   /api/v1/experience-packs/{id}/deprecate
POST   /api/v1/experience-packs/{id}/eval
POST   /api/v1/experience-packs/{id}/request-changes
```

## 16. 状态与数据流

```text
Runtime Events / Debug Trace / Middleware Repair / Workflow Run / Verification
  -> Experience Collector
  -> Experience Case
  -> Candidate Synthesizer
  -> Candidate Store
  -> Experience Pack UI
  -> Review Workbench
  -> Activation Index / Runbook Draft / Workflow Draft / Variant Candidate
  -> Outcome and Fitness
```

实时更新策略：

- MVP 使用刷新按钮和轻量 polling。
- Candidate、review、fitness、eval 状态 10-30 秒刷新 summary。
- Evidence raw ref、Prompt Trace、Workflow graph diff 只在用户打开详情时加载。
- 后续接入统一事件：experience.candidate.created、experience.review.decided、activation.indexed、variant.created、fitness.updated、asset.paused、asset.deprecated。

## 17. 错误、空态与加载态

- 经验包库为空：显示“当前没有经验包”，提供从 case/debug/repair/workflow 生成入口。
- Candidate Store 不可用：禁用生成候选和提交审核。
- Activation Index 不可用：禁用激活动作，已激活资产展示 degraded。
- EvidenceRef 丢失：Gate Result 显示 evidence missing。
- Redaction failed：禁止提交审核和激活。
- Eval runner 不可用：publish gate 显示 eval missing。
- Workflow dry-run 失败：禁止 workflow publish。
- 环境画像缺失：只允许 read-only diagnostic variant。

## 18. 验收标准

- `/experience-packs` 从静态页面升级为 API 驱动，支持 candidate、review_pending、published、rejected、deprecated tabs。
- Candidate Store 和 Activation Index 在页面上强隔离，candidate 不显示为可推荐、可执行或已发布。
- 候选详情展示 synthesis reason、evidence coverage、redaction status、environment compatibility、disabled conditions 和 Gate Result。
- 审核页展示 Evidence、AI Chat Impact、Artifact Diff、Debug / Repair Diff、Gate Result 和 Audit。
- SkillCard/Debug RCA/RepairPlan 只有审核通过后才能进入 Activation Index；Gene/Capsule 后续实现时也必须经过同一审核。
- Runbook draft 和 Workflow graph draft 只能进入对应发布流程，不能直接变成生产执行资产。
- 环境不匹配时只能创建 variant candidate，不能修改原发布资产。
- Variant Resolver 能解释匹配顺序和只读降级原因。
- Fitness 下降能触发 pause、deprecate、variant candidate 或 Eval case，但不自动修改发布资产。
- Chat 集成只展示已激活经验来源，候选只显示数量和审核入口。
- Debug Trace 和 Middleware Repair 能生成经验候选，并保留脱敏、验证和失败点。
- 经验服务能把 experience_pack_id、variant_key、review_record_id、evidence chain 和 environment scope 写入现有 Runner workflow metadata；本方案不要求修改 Runner Studio 页面。
- 所有审核、发布、拒绝、变体生成、pause、deprecated 都有 audit record。
- 页面不直接调用裸 `fetch`，所有请求走 `web/src/api/learningAssets.ts`、后续 `experiencePacks.ts` wrapper 或统一 API client。
- 页面不创建私有 WebSocket/SSE。
- 敏感证据、raw prompt、SecretRef、连接串、token、用户敏感输入不在正文、tooltip、title、aria-label 中泄露。
