# aiops-v2 Learning, Experience Pack, Memory & Eval 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md](2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md)
专题文档：[2026-05-11-aiops-v2-10-experience-packs-design.zh.md](2026-05-11-aiops-v2-10-experience-packs-design.zh.md)
实施清单：[2026-05-11-aiops-v2-09b-learning-memory-eval-frontend-todo.zh.md](2026-05-11-aiops-v2-09b-learning-memory-eval-frontend-todo.zh.md)

## 1. 页面目标

Learning, Experience Pack, Memory & Eval 的前端不是知识库页面，也不是“AI 自动学习”的开关。它是企业运维经验资产控制台，负责把真实 case、Debug Trace、中间件修复、Workflow run、审批、验证和复盘沉淀为候选资产，并在人工审核、环境匹配、风险门禁和 Eval 回归后，才允许进入推荐和生产资产。

页面要让用户完成：

- 查看真实运维过程产生的 ExperiencePack、Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft、Memory、OpsGraph patch 和 Eval case。
- 明确区分 Candidate Store、Draft、Review Queue、Published Asset、Activation Index，避免未审核经验影响 AI Chat 或自动推荐。
- 审核候选资产时查看证据链、环境画像、适用场景、禁用条件、风险、回滚、验证、失败点、lineage 和 dry-run 结果。
- 将已审核 Gene、Capsule、Debug RCA、RepairPlan、Runbook、Workflow 写入 Activation Index 或正式目录。
- 管理 Memory 分层，避免短期会话事实污染长期组织知识。
- 查看 Activation Index 为什么命中某个经验，为什么候选经验不能被使用，哪些资产被 pause、deprecated 或因环境不兼容被排除。
- 从 Prompt / Tool / Experience / Policy / Memory 变更运行 Eval，发现 AI 推理、工具选择、权限拒绝、经验匹配和复盘质量退化。
- 查看 Evolution Map、asset lineage、environment variant、fitness 和 failure clustering，让“自我演化”保持可审计。
- 从 Case、Debug Trace、Middleware Repair、Workflow Run、Postmortem、Prompt Trace 和 OpsGraph 跳入对应候选或审核记录。

设计原则：

- Candidate Store 和 Activation Index 在 UI 上强隔离。候选可以看、审、拒绝、派生，但不能被 Chat 直接推荐。
- 审核页面第一屏展示 Gate Result 和风险，不先展示“批准”按钮。
- 成功和失败都可沉淀：成功路径、禁用条件、风险边界、回滚失败、环境不兼容和误用都要可见。
- Memory 是受治理资产，不是无限追加的聊天历史；`org` memory 必须来自审核通过的证据。
- Eval 是发布门禁，不只是离线报告。经验索引、Prompt、工具 schema、Memory 或政策变更后必须可回归。
- 自动生成内容只能进入 candidate 或 draft；发布、激活、降权、pause、deprecated 都必须写 audit。
- 前端只消费 Learning / Experience / Memory / Eval API view-model，不从页面直接扫描文件路径或拼接本地存储。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/ExperiencePacksPage.tsx`：已有经验包静态列表、详情、版本演进和关联资源初版。
- `web/src/data/opsWorkspace.js`：已有静态经验包样例数据。
- `web/src/pages/IncidentWorkbenchPage.tsx`：已有 case 详情和复盘草稿入口，可生成 Experience candidate。
- `web/src/pages/PromptTracePage.tsx` 与 05a 设计：已有 Prompt Trace / Eval / activated experience 的前端方向。
- `internal/eval`、`internal/promptdiag`：已有 Eval case、report、diagnose、draft case 和 Prompt Trace 关联基础。
- `web/src/pages/RunbookCatalogPage.tsx`、`RunnerStudioPage.tsx`：可承接从候选生成 Runbook draft / Workflow draft。
- `web/src/pages/OpsGraphPage.tsx`：可承接 OpsGraph patch candidate、Asset Map 和 experience match。
- `web/src/pages/PostmortemPage.tsx`：可引用 Verification outcome、Experience candidate 和 follow-up。
- `docs/superpowers/specs/2026-05-11-aiops-v2-10-experience-packs-design.zh.md`：已定义 Evolution Map、Experience Pack、Activation Index、EnvironmentProfile、Candidate Artifact、Review 和存储形态。

主要不足：

- `ExperiencePacksPage.tsx` 仍是静态数据页面，没有 review queue、candidate/detail、Activation Index、Memory、Eval、lineage 和 environment variant。
- 没有 API client 和 view-model 来表达 ExperiencePack、ActivationIndexEntry、MemoryRecord、EvalCase、EvolutionEvent。
- 经验候选没有清晰的审核工作台，无法检查 evidence、AI Chat impact、artifact diff、gate result。
- 未审核候选与已激活资产在 UI 语义上没有强隔离。
- Chat、DebugCase、Middleware Repair、Runbook、Runner、Postmortem 的经验来源和回写入口尚未统一。
- Memory 缺少分层浏览、过期、stale、来源追溯和提升到 org memory 的审核。
- Eval 目前偏后端 / promptdiag 能力，前端缺少 eval case、suite、run、report、regression diagnosis 和失败回写候选入口。
- Evolution Map 和 lineage 不可视，用户无法知道某个经验来自哪次 case、何时被激活、在哪些环境失败。

## 3. 路由与信息架构

建议新增统一入口：

```text
/learning
/learning/experience-packs
/learning/experience-packs/:packId
/learning/review-queue
/learning/review-queue/:candidateId
/learning/activation-index
/learning/memory
/learning/memory/:memoryId
/learning/evals
/learning/evals/:reportId
/learning/evolution-map
/learning/lineage/:assetId
/learning/environment-profiles
/learning/environment-profiles/:profileId
```

保留并增强兼容入口：

```text
/experience-packs
/ai-reasoning/evals
/ai-reasoning/evals/:reportId
/incidents/:caseId?tab=assets
/observability/debug-events/:debugEventId?tab=experience
/middleware/repairs/:caseId?tab=experience
/execution/runs/:runId?tab=experience
/postmortems/:postmortemId
```

路由语义：

- `/learning`：学习资产总览。展示候选、待审核、已激活、Memory、Eval、Activation Index 和 drift。
- `/learning/experience-packs`：经验包库。按 candidate、review_pending、approved、rejected、published、deprecated 分组。
- `/learning/experience-packs/:packId`：经验包详情。展示 evidence、artifacts、review、lineage、fitness、variants 和 activation。
- `/learning/review-queue`：候选审核队列。集中处理 Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft、Memory candidate、OpsGraph patch、Eval case。
- `/learning/review-queue/:candidateId`：候选审核详情。
- `/learning/activation-index`：已审核推荐索引。查看已激活资产、匹配场景、环境兼容性、禁用条件、fitness 和推荐影响。
- `/learning/memory`：Memory Explorer。查看 user、session、service、host、incident、org 分层记忆。
- `/learning/evals`：Eval Workbench。创建 eval case、运行 eval suite、比较报告和诊断退化。
- `/learning/evolution-map`：Evolution Map。查看事件、节点、边、候选来源、激活和结果反馈。
- `/learning/lineage/:assetId`：资产 lineage。查看来源 case、版本、变体、supersedes、incompatible_with、review 和 fitness。
- `/learning/environment-profiles`：环境画像。查看环境兼容性和变体候选。

MVP 可以先把 `/experience-packs` 升级为 `/learning/experience-packs` 的兼容页面，并在同一页面内用 tabs 承载 Review、Activation、Memory、Eval；但 API、组件和文档按终态拆分。

主导航分区：

| 页面 | 目标 |
| --- | --- |
| Learning Overview | 资产沉淀总览、候选隔离、审核阻塞、Activation Index 和 Eval 风险 |
| Experience Pack Library | 经验包、候选、版本、环境变体、证据链和发布状态 |
| Review Queue | 候选审核、Gate Result、Artifact Diff、发布动作和 audit |
| Activation Index | 已审核推荐索引、匹配解释、禁用条件、pause/deprecated |
| Memory Explorer | 分层记忆、来源、TTL、stale、提升审核和裁剪 |
| Eval Workbench | Eval case、suite、run、report、regression diagnosis |
| Evolution Map | 运维经验演化事件、节点、边、lineage 和 fitness |
| Environment Profiles | 环境画像、变体解析、兼容性和不兼容边 |

## 4. 页面一：Learning Overview

Learning Overview 回答“系统学到了什么、哪些还只是候选、哪些已经影响推荐、哪些变化需要评测”。

### 4.1 布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: Learning / 环境 / 时间窗口 / 新建 Eval / 刷新 / 导出审计  │
├────────────────────────────────────────────────────────────┤
│ Governance Strip: Candidate Isolation / Review SLA / Activation / Eval Gate │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: Candidates / Review Pending / Activated / Memory / Eval Risk │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Detail Drawer           │
│ Candidates / Activation /     │ Candidate / Evidence /       │
│ Memory / Eval / Evolution     │ Impact / Gate / Lineage      │
└──────────────────────────────┴─────────────────────────────┘
```

桌面端右侧 Drawer 宽度 420-500px。窄屏时 Drawer 变成底部 Sheet。

### 4.2 Header

显示：

- 当前环境。
- 当前时间窗口。
- 当前用户角色：viewer、reviewer、service owner、SRE、DBA、admin。
- Candidate Store 状态。
- Activation Index 版本。
- Eval runner 状态。

动作：

- 刷新。
- 新建 Eval run。
- 打开 Review Queue。
- 打开 Activation Index diff。
- 导出 audit。

### 4.3 Governance Strip

| 项 | 状态 | 说明 |
| --- | --- | --- |
| Candidate Isolation | healthy / violation / unknown | 候选是否被隔离在 Candidate Store |
| Review SLA | healthy / overdue / blocked | 待审核候选是否超时 |
| Activation Index | stable / changed / stale / degraded | 推荐索引版本和最近变更 |
| Memory Hygiene | healthy / stale / risky | Memory 过期、stale、未审核 org memory |
| Eval Gate | passed / failed / missing / running | 最近变更是否有 Eval 结果 |
| Asset Drift | low / medium / high | 失败聚类、环境不兼容、fitness 下降 |

如果 Candidate Isolation 为 `violation`，页面必须展示阻断告警，并提供打开相关 audit 的入口。

### 4.4 Summary Strip

指标：

- 新增候选。
- 待审核候选。
- 审核超时。
- 已激活资产。
- 被 pause / deprecated 的资产。
- Memory stale。
- Eval failed / regression。
- 需要环境变体的资产。

每个指标可点击过滤对应列表。

## 5. Experience Pack Library

Experience Pack Library 回答“有哪些经验资产，它们处于候选、审核、发布还是弃用状态”。

### 5.1 筛选

- status：candidate、review_pending、approved、rejected、published、deprecated。
- assetType：gene、capsule、debug_rca、repair_plan、runbook、workflow、memory、opsgraph_patch、eval_case。
- source：coroot、chat、frontend_debug、trace、middleware_repair、workflow_run、approval_audit、postmortem、verification。
- environment profile：OS、runtime、namespace、service、agent capability。
- risk。
- reviewer。
- activation eligible。
- fitness。
- disabled condition。
- updatedAt。

筛选条件写入 URL query，便于从 Case、Debug、Middleware、Runner、Prompt Trace 跳入。

### 5.2 表格列

| 列 | 内容 |
| --- | --- |
| Pack | title、id、version、status |
| Scenario | problem signature、entity、source case |
| Artifacts | Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft |
| Evidence | evidence count、verification outcome、redaction status |
| Environment | profile、variant key、compatibility |
| Review | required roles、decision、reviewer、SLA |
| Activation | eligible、indexedAt、disabled、pause/deprecated |
| Fitness | success rate、activation count、recent failures |
| Action | 打开详情、审核、生成 Runbook、生成 Workflow、创建变体 |

### 5.3 详情页

经验包详情 tabs：

| Tab | 目标 |
| --- | --- |
| Overview | 场景、来源、状态、证据、风险和下一步 |
| Evidence Chain | case、Coroot、Debug Trace、中间件、Workflow、Approval、Verification、Postmortem |
| Artifacts | Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft |
| Review | 审核记录、Gate Result、评论、决策和 audit |
| Activation | Activation Index 条目、推荐场景、禁用条件和 pause |
| Lineage | 来源、版本、变体、supersedes、incompatible_with |
| Fitness | 成功率、失败聚类、recent failures、降权原因 |
| Eval | 关联 Eval case、最近 report、regression |

## 6. Review Queue

Review Queue 回答“哪些候选资产可以被审核，是否允许进入推荐或正式资产”。

### 6.1 队列列

| 列 | 内容 |
| --- | --- |
| Candidate | id、assetType、title、source |
| Scope | scenario signature、environment、entity |
| Evidence | sourceRefs、redaction、verification、postmortem |
| Risk | max risk、missing rollback、missing verification、policy blockers |
| Impact | 会影响 Chat、Debug、Middleware、Runbook、Workflow 的哪些推荐 |
| Gate | evidence、environment、risk、dry-run、eval、review roles |
| SLA | createdAt、review due、blocked reason |
| Action | 打开审核、拒绝、请求变更、批准、生成 draft |

默认排序：

1. Gate violation。
2. 高风险候选。
3. Review SLA overdue。
4. Eval regression blocker。
5. 新增候选。

### 6.2 审核详情布局

```text
┌────────────────────────────────────────────────────────────┐
│ Candidate Header: assetType / source / status / risk / gate │
├────────────────────────────────────────────────────────────┤
│ Gate Result: Evidence / Environment / Risk / Rollback / Eval │
├──────────────────────────────┬─────────────────────────────┤
│ Main Tabs                     │ Right Review Panel          │
│ Evidence / Impact / Diff /    │ Decision / Roles / Comment  │
│ Lineage / Eval / Audit        │ / Publish Effects           │
└──────────────────────────────┴─────────────────────────────┘
```

审核详情必须展示：

- Evidence：事故、Coroot、Debug Trace、Middleware Repair、Workflow、Approval、Verification、Postmortem。
- AI Chat Impact：批准后会进入哪些推荐场景、哪些 prompt layers、哪些 matcher。
- Artifact Diff：Gene/Capsule/Runbook/Workflow/RepairPlan/Memory/OpsGraph patch 的 diff。
- Debug / Repair Diff：trace 签名、中间件修复计划、失败点、验证要求。
- Gate Result：环境兼容、风险、回滚、验证、dry-run、Eval、权限。
- Review History：谁看过、谁批准、谁拒绝、为什么。

### 6.3 审核动作

动作：

- reject。
- request_changes。
- approve_gene。
- approve_capsule。
- approve_debug_rca。
- approve_repair_plan。
- approve_as_memory。
- approve_opsgraph_patch。
- generate_runbook_draft。
- generate_workflow_draft。
- approve_runbook_publish。
- approve_workflow_publish。
- create_environment_variant。
- pause_asset。
- deprecate_asset。

动作规则：

- 高风险候选缺少 rollback 或 verification 时不能 approve。
- Workflow draft 必须 validate 和 dry-run 通过后才能 publish。
- Runbook draft 必须通过人工审核后才进入 Runbook catalog。
- Memory candidate 写入 `org` scope 必须有 reviewer 和证据。
- Eval failed 时不能激活影响推荐的资产，除非有 explicit override 和 audit。

## 7. Candidate Artifact 详情

不同候选类型使用同一框架，不同 tabs 展示专用内容。

### 7.1 Gene Candidate

展示：

- rule summary。
- scenario signature。
- evidence refs。
- expected recommendation。
- forbidden recommendation。
- activation scope。
- fitness seed。

Gene 不能执行动作，只影响 AI 推理和推荐。

### 7.2 Capsule Candidate

展示：

- applicability。
- diagnostic steps。
- action steps。
- verification。
- rollback。
- disabled conditions。
- environment compatibility。

Capsule 可以派生 Runbook 或 Workflow，但不能直接执行。

### 7.3 Debug RCA Candidate

展示：

- frontend page。
- user action。
- trace id。
- slow span。
- service path。
- Coroot RCA。
- suggested repair。
- redaction status。
- before / after verification。

如果 trace completeness 不足，只能 request_more_evidence 或 reject，不能 approve。

### 7.4 RepairPlan Candidate

展示：

- MiddlewareResource。
- RCA hypothesis。
- diagnostic steps。
- remediation steps。
- risk。
- rollback。
- verification。
- disabled conditions。
- failedPoint。
- RecoveryAttempt。

环境不匹配或 backup/PITR 不满足时，只能创建 variant 或保留只读诊断。

### 7.5 Runbook / Workflow Draft

展示：

- source experience。
- semantic diff。
- risk summary。
- validation result。
- dry-run result。
- rollback and verification coverage。
- target environment。

Workflow draft 发布必须跳转 Runner Studio 或嵌入 Runner publish gate。

### 7.6 Memory / OpsGraph / Eval Candidate

Memory candidate 展示 scope、subjectRef、content summary、sourceRefs、confidence、ttl、redactionStatus、stale risk。

OpsGraph patch 展示 nodes、edges、operations、source evidence、before / after、risk 和 reviewer。

Eval case 展示 scenarioType、inputRefs、expectedBehavior、forbiddenBehavior、scoringRubric、linkedPromptTraceIds。

## 8. Activation Index

Activation Index 回答“哪些经验真的会影响下一次 AI Chat、DebugCase 或 Middleware Repair 推荐”。

### 8.1 列表列

| 列 | 内容 |
| --- | --- |
| Entry | id、assetType、assetId、status |
| Scenario | scenarioSignature、entity、case type |
| Environment | environmentProfile、variant key、compatibility |
| Conditions | disabledConditions、incompatible_with、pause |
| Confidence | confidence、fitness、success rate、recent failures |
| Review | reviewRecord、reviewer、approvedAt |
| Usage | activation count、lastUsedAt、last outcome |
| Action | 打开资产、解释匹配、pause、deprecate、创建 Eval |

### 8.2 匹配解释

匹配解释 Drawer 展示：

- 输入 scenario。
- 匹配的 asset。
- environment match。
- disabledConditions。
- confidence / fitness。
- why ranked higher。
- why candidates excluded。
- prompt layer impact。
- downstream matcher：PromptCompiler、Runbook matcher、Workflow recommender、Debug RCA matcher、RepairPlan matcher。

候选经验如果存在但未审核，只能显示数量和审核入口，不能显示具体可执行建议。

### 8.3 pause / deprecated

pause 操作：

- 需要 reviewer comment。
- 立即从 Activation Index 推荐中排除。
- 保留 lineage 和历史。
- 触发 Eval 或 review follow-up。

deprecated 操作：

- 不删除资产。
- 不再进入推荐。
- 显示 supersededBy 或 deprecated reason。

## 9. Memory Explorer

Memory Explorer 回答“系统记住了什么，是否应该继续影响推理”。

### 9.1 分层

Memory scopes：

- user：用户偏好和交互习惯。
- session：当前会话短期上下文。
- service：服务画像、常见故障、恢复方式。
- host：主机能力、限制、历史问题。
- incident：事故经验、证据、验证结果。
- org：组织规则、审批习惯、变更窗口。

### 9.2 表格列

| 列 | 内容 |
| --- | --- |
| Memory | id、scope、subjectRef |
| Content | redacted summary、source type |
| Source | sourceRefs、case、trace、repair、verification |
| Confidence | confidence、reviewed、lastUsedAt |
| TTL | ttl、expiresAt、stale |
| Redaction | visible、redacted、restricted、failed |
| Impact | used by prompt、used by matcher、blocked |
| Action | 打开、标记 stale、延长 TTL、提升审核、删除候选 |

### 9.3 提升和裁剪

规则：

- session memory 自动过期，不允许直接提升到 org。
- incident/service/host memory 可以申请提升，但必须有 sourceRefs 和 reviewer。
- org memory 只能来自审核通过的证据。
- stale memory 不进入 PromptCompiler。
- redaction failed memory 不能进入任何 prompt。

Memory 详情展示：

- content summary。
- sourceRefs。
- redaction report。
- last used trace。
- prompt impact。
- linked Eval failures。

## 10. Eval Workbench

Eval Workbench 回答“经验、Prompt、工具、Memory 或 Policy 变更是否导致退化”。

### 10.1 Eval Case

EvalCase 字段：

- sourceCaseId。
- scenarioType：incident、debug、middleware_repair、permission、workflow、postmortem。
- inputRefs。
- expectedBehavior。
- forbiddenBehavior。
- scoringRubric。
- linkedPromptTraceIds。
- linkedExperienceRefs。
- linkedMemoryRefs。

创建来源：

- Case close。
- Debug Trace。
- Middleware Repair。
- Workflow failure。
- Approval denied。
- Prompt Trace regression。
- Experience review。

### 10.2 Eval Run 表单

字段：

- eval suite。
- case category。
- model。
- prompt version。
- tool schema version。
- experience index version。
- memory snapshot。
- policy version。
- baseline report。

运行前显示：

- 会覆盖哪些场景。
- 是否包含高风险 forbidden behavior。
- 是否使用当前 Activation Index。
- 是否需要隔离 candidate store。

### 10.3 Eval Report

列表列：

| 列 | 内容 |
| --- | --- |
| Run | runId、createdAt、actor |
| Change | prompt、tools、experience、memory、policy |
| Summary | pass rate、regression count、forbidden violations |
| Risk | permission bypass、unsafe action、candidate leakage |
| Artifacts | linked Prompt Trace、tool calls、case outputs |
| Action | 查看报告、打开失败 trace、创建候选、阻断发布 |

报告详情展示：

- overall summary。
- case scores。
- failed checks。
- forbidden behavior。
- regression vs baseline。
- Prompt Trace links。
- activated experience links。
- memory refs。
- diagnosis。
- recommended fixes。

Eval failed 时，相关 publish / activation 按钮禁用，除非有授权 override。

## 11. Evolution Map 与 Lineage

Evolution Map 回答“经验资产从哪里来、怎么演化、在哪里成功或失败”。

### 11.1 Evolution Events

事件流：

- experience.candidate.created。
- experience.review.started。
- experience.review.decided。
- activation.indexed。
- activation.paused。
- asset.deprecated。
- runbook.draft.generated。
- workflow.draft.generated。
- eval.case.created。
- eval.run.completed。
- memory.created。
- memory.promoted。
- opsgraph.patch.created。
- variant.created。
- incompatible.edge.created。
- fitness.updated。

事件列表可按 asset、case、environment、reviewer、source、time window 过滤。

### 11.2 Lineage 图

Lineage 图固定布局：

```text
Source Case / DebugTrace / RepairCase / WorkflowRun
  -> Experience Candidate
  -> Gene / Capsule / Debug RCA / RepairPlan
  -> Runbook Draft / Workflow Draft / Memory / OpsGraph Patch / EvalCase
  -> Activation Index / Published Runbook / Published Workflow
  -> Outcomes / Fitness / Variants
```

节点显示：

- status。
- review decision。
- semantic hash。
- environment profile。
- last outcome。
- fitness。

### 11.3 Fitness 和失败聚类

展示：

- success rate。
- activation count。
- recent failures。
- failure clusters。
- environment-specific failures。
- downgrade / pause reason。
- suggested variant。

系统可以建议 variant candidate，但不能自动修改已发布资产。

## 12. Environment Profiles

Environment Profiles 回答“经验能否安全复用于当前环境”。

### 12.1 环境画像来源

- runner agent heartbeat / probe。
- Coroot project、namespace、service、deployment、topology。
- 只读命令探测。
- Workflow run context。
- Debug Trace context。
- MiddlewareResource context。
- OpsGraph entity。

### 12.2 表格列

| 列 | 内容 |
| --- | --- |
| Profile | id、environment、variant key |
| Runtime | OS、distro、init system、container/k8s、arch |
| Capabilities | agent capabilities、tool availability、permissions |
| Business | namespace、service、business capability、tenant |
| Middleware | cluster kind、role、replication、backup state |
| Compatibility | matching assets、incompatible assets、variant candidates |
| Action | 打开详情、probe、创建变体、运行 Eval |

### 12.3 变体创建

当资产不兼容：

- 不修改原资产。
- 创建 variant candidate。
- 保留 parent asset。
- 展示 diff。
- 需要 review、validate、dry-run、Eval。

## 13. 跨模块集成

### 13.1 Incident Control Plane

Case Assets tab 嵌入：

- Postmortem draft。
- Experience candidate。
- Runbook draft。
- Workflow draft。
- Memory candidate。
- OpsGraph patch candidate。
- Eval case。

Case close 后可生成候选，但不能直接发布。

### 13.2 Observability / Debug Trace

DebugEvent 页面提供：

- 从 Debug Trace 生成 Debug RCA candidate。
- 展示 trace signature。
- 关联 Experience review。
- 创建 Eval case。

Trace completeness 不足时，candidate gate 显示 blocked。

### 13.3 Middleware Repair

Repair case 页面提供：

- 生成 RepairPlan candidate。
- 生成 Capsule candidate。
- 生成 Eval case。
- 查看从该 repair 派生的经验包。

失败修复也要生成 disabled condition 和 anti-pattern candidate。

### 13.4 Execution Fabric

Workflow Run Detail 提供：

- 从成功 workflow run 生成 Workflow draft / Capsule candidate。
- 从失败 run 生成 anti-pattern / Eval case。
- 回写 Execution evidence、approval、HostLease、verification。

Runner Studio 展示：

- source=experience-pack。
- experience_pack_id。
- variant_key。
- review_record_id。
- evidence chain。
- environment scope。

### 13.5 AI Reasoning / Prompt Trace

Prompt Trace 展示：

- activatedExperienceRefs。
- memoryRefs。
- evalLabels。
- candidate leakage warning。
- experience activation diff。

Chat 回复中只展示已激活经验来源，不展示 candidate 的具体推荐内容。

### 13.6 OpsGraph

Experience review 可生成 OpsGraphPatch candidate。

OpsGraph Asset Map 展示：

- approved runbooks。
- workflows。
- experience packs。
- memory snippets。
- Eval cases。

Patch 审核通过后才写入图谱。

## 14. 前端数据模型

建议新增 `web/src/api/learningAssets.ts`，不要把经验资产 API 挤进 `complexPagesApi.ts`。

```ts
export type ExperiencePackStatus =
  | "candidate"
  | "review_pending"
  | "approved"
  | "rejected"
  | "published"
  | "deprecated";

export type LearningAssetType =
  | "gene"
  | "capsule"
  | "debug_rca"
  | "repair_plan"
  | "runbook"
  | "workflow"
  | "memory"
  | "opsgraph_patch"
  | "eval_case";

export type ExperiencePackView = {
  id: string;
  title: string;
  status: ExperiencePackStatus;
  version: string;
  scenario: ScenarioSignatureView;
  environmentProfile: EnvironmentProfileView;
  confidence: ConfidenceView;
  artifacts: CandidateArtifactView[];
  activation: ActivationStateView;
  lineage: LineageSummaryView;
  review: ReviewSummaryView;
  evidenceRefs: EvidenceRefView[];
  disabledConditions: DisabledConditionView[];
  fitness: FitnessView;
};

export type ActivationIndexEntryView = {
  id: string;
  assetType: "gene" | "capsule" | "debug_rca" | "repair_plan" | "runbook" | "workflow";
  assetId: string;
  scenarioSignature: ScenarioSignatureView;
  environmentProfile: EnvironmentProfileView;
  disabledConditions: DisabledConditionView[];
  confidence: number;
  fitness: FitnessView;
  reviewRecord: ReviewRecordView;
  status: "active" | "paused" | "deprecated";
};

export type MemoryRecordView = {
  id: string;
  scope: "user" | "session" | "service" | "host" | "incident" | "org";
  subjectRef: string;
  contentSummary: string;
  sourceRefs: EvidenceRefView[];
  confidence: number;
  ttl?: string;
  expiresAt?: string;
  redactionStatus: "visible" | "redacted" | "restricted" | "failed";
  lastUsedAt?: string;
  stale: boolean;
  reviewed: boolean;
};

export type EvalCaseView = {
  id: string;
  sourceCaseId?: string;
  scenarioType: "incident" | "debug" | "middleware_repair" | "permission" | "workflow" | "postmortem";
  inputRefs: EvidenceRefView[];
  expectedBehavior: string[];
  forbiddenBehavior: string[];
  scoringRubric: ScoringRubricView;
  linkedPromptTraceIds: string[];
  linkedExperienceRefs: string[];
  linkedMemoryRefs: string[];
};
```

## 15. API 契约

页面优先使用 canonical API：

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
GET    /api/v1/activation-index/search
GET    /api/v1/memory/search
POST   /api/v1/evals/cases
POST   /api/v1/evals/run
GET    /api/v1/evals/reports/{id}
GET    /api/v1/evolution-map/entities/{id}
GET    /api/v1/evolution-map/lineage/{asset_id}
GET    /api/v1/evolution-map/events
POST   /api/v1/evolution-map/query
GET    /api/v1/environment-profiles
GET    /api/v1/environment-profiles/{id}
POST   /api/v1/environment-profiles/probe
```

建议补充：

```text
POST   /api/v1/activation-index/{entry_id}/pause
POST   /api/v1/activation-index/{entry_id}/deprecate
GET    /api/v1/activation-index/explain
POST   /api/v1/memory/{id}/mark-stale
POST   /api/v1/memory/{id}/promote-review
POST   /api/v1/evals/cases/from-case
POST   /api/v1/evals/cases/from-prompt-trace
GET    /api/v1/learning/overview
GET    /api/v1/learning/governance-summary
```

兼容期可以让 `learningAssets.ts` fallback 到现有 ExperiencePacksPage 静态数据和 eval report API，但页面组件只能依赖 Learning view-model。

## 16. 状态与数据流

```text
Case / DebugTrace / RepairCase / WorkflowRun / Approval / Verification / Postmortem
  -> Experience Collector
  -> Candidate Store
  -> Learning API
  -> Review Queue / Experience Pack Detail
  -> Review Decision
  -> Activation Index / Runbook / Workflow / Memory / OpsGraph / Eval
```

实时更新策略：

- MVP 使用刷新按钮和轻量 polling。
- Review queue 和 Eval run 状态可以 10-30 秒刷新 summary。
- 大证据链、raw diff、Prompt Trace 只在用户打开详情时加载。
- 后续接入统一 event stream，事件包括 candidate.created、review.decided、activation.updated、memory.promoted、eval.completed、lineage.updated。
- 页面刷新后从 API 恢复所有状态，不依赖内存态。

## 17. 错误、空态与加载态

- Learning Overview 加载：固定高度 skeleton。
- 经验包为空：显示“当前没有候选或发布经验”，提供从 case/debug/repair 生成入口。
- Review queue 为空：显示“没有待审核候选”。
- Candidate Store 不可用：禁用生成和审核动作。
- Activation Index 不可用：Chat 仍可运行，但不显示经验推荐，页面展示 degraded。
- Eval runner 不可用：发布 / 激活 gate 显示 eval missing。
- EvidenceRef 丢失：Gate Result 显示 evidence missing。
- Redaction failed：候选不能进入 review approve。
- 权限不足：只显示摘要，不显示 raw prompt、敏感证据、连接串、SecretRef。

## 18. 响应式与可访问性

- 桌面端：主区 + 右侧 Drawer。
- 平板端：Drawer 降级为底部 Sheet。
- 手机端：表格改为列表，Tabs 横向滚动。
- 状态必须有文本，不只靠颜色。
- 评测图表必须有文本 summary。
- icon button 必须有 aria-label 或 tooltip。
- approve、publish、activate、pause、deprecate 必须有二次确认。
- 审核按钮附近必须展示 Gate Result 和阻断原因。

## 19. 验收标准

- `/learning` 能展示候选隔离、审核、Activation Index、Memory、Eval 和 drift 总览。
- `/learning/experience-packs` 能按 candidate、review_pending、approved、rejected、published、deprecated 分组展示经验包。
- `/learning/review-queue` 能审核 Gene、Capsule、Debug RCA、RepairPlan、Runbook draft、Workflow draft、Memory candidate、OpsGraph patch、Eval case。
- 未审核候选不会显示为可推荐、可执行或已发布资产。
- 审核详情能展示 Evidence、AI Chat Impact、Artifact Diff、Debug / Repair Diff、Gate Result 和 Audit。
- 高风险候选缺少 rollback、verification、dry-run 或 Eval 时不能 approve。
- Activation Index 页面能解释为什么命中某资产、为什么候选被排除、哪些 disabled condition 生效。
- Memory Explorer 能展示 scope、sourceRefs、TTL、stale、redactionStatus 和 prompt impact。
- `org` memory 只能由审核通过的证据生成。
- Eval Workbench 能创建 EvalCase、运行 Eval、查看报告、定位 Prompt Trace 和阻断发布。
- Evolution Map 能展示候选来源、激活、lineage、fitness、variant 和 incompatible edge。
- Experience Pack 审核通过后才能进入 AI Chat、DebugCase 或 Middleware Repair 推荐。
- 从经验包生成的 Runbook / Workflow draft 必须进入对应审核和 validate/dry-run gate。
- 页面不直接调用裸 `fetch`，所有请求走 `web/src/api/learningAssets.ts` 或统一 API client。
- 页面不创建私有 WebSocket/SSE。
- 受限证据、raw prompt、SecretRef、连接串、token、用户敏感输入不在正文、tooltip、title、aria-label 中泄露。
