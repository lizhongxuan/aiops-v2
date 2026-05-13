# aiops-v2 Verification & Recovery 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-07-verification-recovery-module-design.zh.md](2026-05-11-aiops-v2-07-verification-recovery-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-07b-verification-recovery-frontend-todo.zh.md](2026-05-11-aiops-v2-07b-verification-recovery-frontend-todo.zh.md)

## 1. 页面目标

Verification & Recovery 的前端不是“执行结果页”或“关闭事故前的确认框”，而是生产恢复证明工作台。页面要让用户完成：

- 判断动作执行后系统是否真的恢复，而不是只看 Workflow、工具命令或脚本退出码。
- 为高风险动作、Workflow、Runbook、RepairPlan、DebugCase 修复建立 `VerificationSpec`。
- 在观察窗口内持续查看 smoke check、指标趋势、业务验证、trace 对比、中间件健康、主机/K8s 状态和人工确认。
- 将 case 的恢复状态明确区分为 `mitigated`、`verifying`、`recovered`、`partially_recovered`、`failed`、`rolled_back`、`requires_manual_followup`。
- 在验证失败时定位 failedPoint、失败证据、失败链路、已回滚状态和下一步建议。
- 在回滚后再次验证服务可用、数据一致性、业务未恶化、HostLease 与审批状态已收敛。
- 从 Case、Workflow Run、ActionProposal、Approval、Debug Trace、Middleware Repair、Postmortem 和 Experience Pack 跳入同一份验证记录。
- 让成功和失败的验证结果可沉淀为经验候选、Memory、Eval case 和复盘证据。

设计原则：

- 第一屏显示恢复状态和阻塞项，不把验证藏在执行详情深处。
- Case header 显示 `RecoveryStatus`，不能只显示 `WorkflowRun.status`。
- `resolved` 必须依赖通过的 VerificationRecord 或授权人工覆盖决策。
- 验证来源要可解释：Coroot、ERP、Debug Trace、中间件探针、主机/K8s、Workflow output、人工确认各自独立展示。
- Observation Window 是状态机，不是静态倒计时；每个阶段都要有证据、失败条件和下一步动作。
- 验证失败和回滚失败也属于一等结果，必须进入 timeline、evidence、experience candidate 和 audit。
- 前端只消费统一 Verification / Recovery API 和 case timeline，不新增页面私有执行流。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/IncidentWorkbenchPage.tsx`：已有 case 工作台初版，可承载 Verification tab 和 Recovery header。
- `docs/superpowers/specs/2026-05-11-aiops-v2-01a-incident-control-plane-frontend-design.zh.md`：已定义 Case Verification Tab、failedPoint 和 nextRecommendation 展示要求。
- `web/src/pages/RunbookDetailPage.tsx`：已有 Runbook 验证项展示初版。
- `web/src/pages/RunnerStudioPage.tsx` 与 runner 组件：已有 Workflow 运行、节点状态、变量、审批、回调、运行历史聚合能力。
- `web/src/components/runner/runStateReducer.js`、`runEventHistory.js`、`runnerRunVisualState.js`：可扩展为 Verification event 和 recovery gate 聚合。
- `web/src/pages/CorootOverviewPage.tsx`、`web/src/api/coroot.js`：已有 Coroot 只读数据与 iframe 接入基础。
- `web/src/pages/PostmortemPage.tsx`：已有复盘草稿页面和 verification 区块。
- `web/src/pages/complexPagesApi` 与复杂页面组件：已有基础 Card、Badge、StatusAlert、EmptyPanel、LoadingState 可复用。

主要不足：

- 没有独立 Verification / Recovery 工作台，用户很难从所有 case 中筛出“验证中、验证失败、需要回滚、需要人工确认”的事项。
- Case 页面尚未把 `RecoveryStatus` 提升到 header，执行成功和恢复成功仍容易被混淆。
- 缺少 `VerificationSpec` 创建、编辑、绑定、覆盖和门禁说明页面。
- 缺少 `VerificationRecord` 详情页，无法展示每个 check 的证据、阈值、时间窗口、失败点和下一步建议。
- 缺少 Observation Window 页面模型，无法表达 smoke check、等待、趋势检查、业务验证、关闭或回滚决策。
- Coroot、ERP、Debug Trace、中间件、主机/K8s 的验证证据缺少统一对比组件。
- 回滚验证没有独立 UI，容易把“执行 rollback 命令成功”误认为“恢复完成”。
- 人工确认没有结构化表单、权限裁剪、理由、有效期和证据引用。
- 验证结果还没有稳定回写 Experience Pack、Memory、Eval、Postmortem 的前端入口。

## 3. 路由与信息架构

建议新增统一入口：

```text
/verification
/verification/records/:verificationId
/verification/specs/:specId
/verification/cases/:caseId
/verification/recovery-decisions/:decisionId
```

保留并增强嵌入入口：

```text
/incidents/:caseId?tab=verification
/execution/runs/:runId?tab=verification
/runbook-instances/:instanceId?tab=verification
/observability/debug-events/:debugEventId?tab=after-fix
/middleware/repairs/:repairId?tab=verification
/postmortems/:postmortemId
```

路由语义：

- `/verification`：Verification & Recovery 总览。跨 case 展示验证队列、恢复状态、观察窗口、失败点、回滚和人工确认。
- `/verification/records/:verificationId`：Verification Record 详情。查看 spec、checks、证据、趋势、trace 对比、失败原因、人工确认和审计。
- `/verification/specs/:specId`：Verification Spec 详情或编辑。定义 requiredChecks、optionalChecks、observationWindow、success/failure/rollback criteria。
- `/verification/cases/:caseId`：Case Recovery 工作台。聚合该 case 的所有动作、workflow、repair、verification 和 recovery decision。
- `/verification/recovery-decisions/:decisionId`：恢复决策详情。解释为什么标记 recovered、partially_recovered、failed、rolled_back 或 manual follow-up。

MVP 可以先不新增所有详情路由，把详情通过 `/verification?recordId=...`、`/incidents/:caseId?tab=verification` 和 Drawer 承载；但 API、组件和文档按终态拆分。

主导航分区：

| 页面 | 目标 |
| --- | --- |
| Verification Overview | 跨 case 恢复队列、验证失败、观察窗口、人工确认和回滚阻塞 |
| Case Recovery Workspace | 单个 case 的恢复状态、验证计划、证据矩阵、关闭门禁和恢复决策 |
| Verification Record Detail | 单次验证记录的 checks、证据、趋势、失败点、建议和审计 |
| Verification Spec Designer | 为动作、Workflow、Runbook、RepairPlan 或 AI plan 定义验证标准 |
| Observation Window Console | 展示动作完成后的 smoke、等待、趋势、业务验证和回滚决策 |
| Evidence Compare | 修复前后 Coroot、ERP、Trace、中间件、主机/K8s 指标对比 |
| Rollback Verification | 回滚动作、回滚证据、数据一致性、业务未恶化和锁释放验证 |
| Manual Confirmation | 人工确认请求、权限、理由、证据引用和冲突处理 |

## 4. 页面一：Verification Overview

Verification Overview 回答“当前哪些事故已经执行动作，但恢复还没有被证明”。

### 4.1 布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: Verification & Recovery / 环境 / 时间窗口 / 创建验证计划 / 刷新 │
├────────────────────────────────────────────────────────────┤
│ Recovery Strip: verifying / failed / rollback / manual / recovered │
├────────────────────────────────────────────────────────────┤
│ Source Health: Coroot / ERP / Trace / Middleware Probe / K8s / Host │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Detail Drawer           │
│ Queue / Records / Observation │ Case / Verification /        │
│ / Rollback / Manual / Audit   │ Evidence / Decision          │
└──────────────────────────────┴─────────────────────────────┘
```

桌面端右侧 Drawer 宽度 400-480px。窄屏时 Drawer 变成底部 Sheet。

### 4.2 Header

显示：

- 当前环境。
- 当前时间窗口。
- 当前用户角色：viewer、operator、approver、recovery_reviewer、admin。
- 数据刷新时间。
- 验证源健康摘要。

动作：

- 刷新。
- 创建 VerificationSpec。
- 运行选中 VerificationRecord。
- 打开人工确认队列。
- 打开失败验证导出。
- 打开验证策略说明。

### 4.3 Recovery Strip

Recovery Strip 是总览页的核心。

| 指标 | 含义 |
| --- | --- |
| verifying | 正在观察窗口或 checks 运行中的 case |
| mitigated | 技术风险下降但业务未完全验证的 case |
| recovered | 技术和业务验证均通过的 case |
| partially_recovered | 部分服务、租户、业务能力或指标恢复的 case |
| failed | 验证失败且未回滚或未形成下一步处置的 case |
| rolled_back | 已执行并验证回滚的 case |
| requires_manual_followup | 自动验证不足，需要人工确认或后续跟进的 case |

每个状态卡点击后过滤队列，不能只作为静态计数。

### 4.4 Source Health

Verification Source Health 展示下游验证能力是否可用：

| 来源 | 状态 | 影响说明 |
| --- | --- | --- |
| Coroot | connected / degraded / unavailable | SLO、latency、error、saturation、topology health 是否可用于验证 |
| ERP | connected / degraded / unavailable | 业务任务、订单、租户 SLA 是否可用于验证 |
| Trace Backend | connected / degraded / unavailable | Debug Trace 修复前后对比是否可用 |
| Middleware Probe | connected / degraded / unavailable | PG/Redis/MQ 等健康项是否可读取 |
| K8s | connected / degraded / unavailable | pod readiness、rollout、events 是否可验证 |
| Host Agent | connected / partial / unavailable | systemd、CPU、memory、disk、network 是否可验证 |
| Manual Confirmation | available / restricted / no_approver | 是否有具备权限的人可确认 |

当来源不可用时，关联 VerificationRecord 要显示 `inconclusive` 或 `blocked_by_source_unavailable`，不能静默跳过 required check。

## 5. Recovery Queue Tab

Recovery Queue 回答“我现在该处理哪个验证或恢复阻塞”。

### 5.1 筛选

支持：

- recoveryStatus。
- verificationStatus。
- caseType。
- severity。
- environment。
- source：workflow、runbook、repair_plan、ai、manual。
- verificationType：slo、business、trace、middleware、host、k8s、manual、rollback。
- blockedReason。
- owner。
- observationWindowState。
- failedPoint。
- requiresManualConfirmation。

筛选条件写入 URL query，便于从 Case、Execution、Middleware Repair、Approval 跳入。

### 5.2 表格列

| 列 | 内容 |
| --- | --- |
| Case | caseId、title、type、severity |
| Recovery | RecoveryStatus、上次变化时间、阻塞项 |
| Source Action | ActionProposal、WorkflowRun、RunbookInstance、RepairPlan |
| Verification | record count、required passed、failed、inconclusive |
| Observation | window state、剩余时间、下一次检查 |
| Evidence | Coroot、ERP、Trace、Middleware、Host/K8s、Manual coverage |
| Failed Point | failedPoint、failed check、scope |
| Next | 运行验证、查看失败、请求人工确认、触发回滚、恢复决策 |

排序规则：

1. failed 且无 rollback decision。
2. observation window 超时。
3. required check inconclusive。
4. manual confirmation 超时。
5. partially_recovered。
6. verifying。
7. recovered。

### 5.3 行详情

展开行展示：

- 当前 RecoveryStatus。
- 关联 ActionProposal / WorkflowRun / RepairPlan。
- VerificationSpec 摘要。
- requiredChecks 通过率。
- failed checks。
- EvidenceRef 列表。
- nextRecommendation。
- 已生成的 Experience candidate 或 Postmortem evidence。

## 6. Case Recovery Workspace

Case Recovery Workspace 回答“这个 case 是否可以被认定恢复，为什么”。

### 6.1 Header

Header 必须同时显示 case 状态和恢复状态：

```text
Case: checkout submit latency spike
CaseStatus: verifying
RecoveryStatus: mitigated
Primary Blocker: ERP business validation pending
```

展示字段：

- caseId。
- case type。
- severity。
- affected business capability。
- affected service / middleware / host。
- owner。
- CaseStatus。
- RecoveryStatus。
- active VerificationRecord。
- observation window。
- close gate 状态。

### 6.2 Recovery Status Rail

状态流：

```text
not_started
  -> mitigation_in_progress
  -> mitigated
  -> verifying
  -> recovered
```

分支：

```text
verifying -> partially_recovered
verifying -> failed
failed -> rolled_back
failed -> requires_manual_followup
```

每个状态节点显示：

- 进入时间。
- 触发事件。
- 证据数量。
- 操作者或系统 actor。
- 可回溯 timeline event。

### 6.3 Tabs

| Tab | 目标 |
| --- | --- |
| Recovery Summary | 恢复状态、阻塞项、验证覆盖率和下一步 |
| Verification Plan | VerificationSpec、required/optional checks、criteria |
| Verification Records | 每次验证运行记录、状态和证据 |
| Evidence Compare | 修复前后技术和业务证据对比 |
| Observation Window | 当前观察窗口状态、阶段和倒计时 |
| Rollback | 回滚条件、回滚动作、回滚验证和失败点 |
| Manual Confirmation | 用户、SRE、DBA 等人工确认 |
| Decision & Close Gate | 恢复决策、关闭阻塞、覆盖理由和审计 |

### 6.4 Recovery Summary

展示：

- 当前 RecoveryStatus。
- 成功验证数、失败验证数、inconclusive 数。
- requiredChecks 覆盖率。
- 技术验证状态。
- 业务验证状态。
- trace 对比状态。
- 中间件健康状态。
- 回滚可用性。
- close gate 是否允许。

恢复状态解释要直接面向用户，例如：

```text
技术指标已经恢复，但 ERP 订单推进仍未通过 10 分钟窗口验证，因此只能标记 mitigated，不能 resolved。
```

## 7. Verification Spec Designer

Verification Spec Designer 回答“动作执行后必须如何证明恢复”。

### 7.1 入口

可从以下对象创建或查看：

- ActionProposal。
- Workflow node。
- Workflow End。
- Runbook step。
- RepairPlan。
- AI Reasoning 的 verification_plan 输出。
- DebugCase 修复建议。
- Middleware Repair plan。
- 手工创建。

高风险 ActionProposal 缺少 VerificationSpec 时，审批页和执行页要显示阻断原因，并提供“创建验证计划”入口。

### 7.2 表单区域

| 区域 | 字段 |
| --- | --- |
| Source | source、caseId、actionProposalId、workflowRunId、runbookInstanceId、repairPlanId |
| Scope | service、business capability、tenant、middleware、host、k8s workload、trace route |
| Required Checks | slo、business、trace、middleware、host、k8s、manual、rollback |
| Optional Checks | 可补强但不阻塞恢复判断的 checks |
| Observation Window | duration、sampling interval、start condition、timeout behavior |
| Success Criteria | 每个 required check 的通过条件 |
| Failure Criteria | 失败条件、失败阈值、failedPoint 映射 |
| Rollback Criteria | 何时必须建议或触发回滚 |
| Manual Override | 谁能覆盖、需要什么理由和证据 |
| Evidence Policy | 需要哪些 EvidenceRef、脱敏状态和时间窗口 |

### 7.3 Criteria Builder

Criteria Builder 使用结构化条件，不要求用户写表达式：

- 指标：p95 latency < threshold、error rate < threshold、SLO burn below threshold。
- 业务：订单成功率、任务积压、租户 SLA、业务错误码。
- trace：指定 route 的 slow span 消失、TTFB 降到阈值内、DB span 恢复。
- 中间件：replication lag、lock wait、connections、disk/WAL、evictions、MQ lag。
- K8s：pod ready、rollout complete、events 无严重错误。
- 主机：systemd active、CPU/memory/disk/network 在阈值内。
- 人工：角色、确认对象、确认文本、有效期。

每个 criteria 显示：

- baseline 来源。
- current 来源。
- timeWindow。
- pass/fail 阈值。
- evidence requirement。
- inconclusive 时的处理。

## 8. Verification Record Detail

Verification Record Detail 回答“这一次验证是怎么得出结论的”。

### 8.1 Header

显示：

- VerificationRecord id。
- caseId。
- source。
- type。
- status。
- RecoveryStatus contribution。
- startedAt、finishedAt。
- observation window。
- owner。
- failedPoint。
- nextRecommendation。

动作：

- 运行验证。
- 重跑失败 check。
- 请求人工确认。
- 标记 failed。
- 打开 Recovery Decision。
- 创建 Experience candidate。
- 导出证据引用。

### 8.2 Check List

每个 check 展示：

| 字段 | 内容 |
| --- | --- |
| Check | name、type、required/optional |
| Status | pending、running、passed、failed、inconclusive、skipped |
| Criteria | success/failure/rollback criteria |
| Evidence | EvidenceRef、rawRef、timeWindow |
| Baseline | 修复前或历史基线 |
| Current | 当前观测值 |
| Delta | 变化方向和幅度 |
| Failed Point | 失败位置或不确定原因 |
| Next | 重跑、请求证据、人工确认、回滚建议 |

状态不能只靠颜色表达，必须有文本和说明。

### 8.3 Evidence Panel

右侧 Evidence Panel 展示：

- Coroot SLO metrics。
- ERP business metrics。
- Debug Trace before/after。
- Middleware probe result。
- Host/K8s result。
- Workflow outputs。
- ToolResult refs。
- Manual confirmation refs。

每个证据显示可用性：

- `usable_for_recovery`。
- `usable_for_prompt`。
- `restricted`。
- `redacted`。
- `time_window_mismatch`。
- `source_unavailable`。
- `low_confidence`。

受限证据只展示 id、来源、时间、限制原因，不暴露正文。

## 9. Observation Window Console

Observation Window Console 回答“动作后系统是否在足够长的窗口内保持恢复”。

### 9.1 状态机

```text
action completed
  -> immediate smoke check
  -> wait observation window
  -> metric trend check
  -> business validation
  -> close or rollback decision
```

前端状态：

| 状态 | 说明 |
| --- | --- |
| waiting_for_action | 动作尚未完成，验证窗口不能开始 |
| smoke_check_running | 正在运行即时 smoke checks |
| observation_waiting | 正在等待窗口累计数据 |
| trend_check_running | 正在检查指标趋势 |
| business_validation_running | 正在检查 ERP 或业务接口 |
| decision_required | checks 完成，需要恢复决策 |
| completed | 已形成恢复结论 |
| failed | 窗口内出现失败 |
| expired | 观察窗口超时但未得到足够证据 |

### 9.2 页面展示

显示：

- 窗口开始条件。
- 窗口时长。
- 已经过时间。
- 下一次采样时间。
- 已完成 checks。
- 未完成 checks。
- 窗口内失败事件。
- 当前趋势。
- rollbackCriteria 是否触发。

不同场景默认展示文案：

- 配置 reload：1-3 分钟。
- 服务重启：3-5 分钟。
- 扩容：5-15 分钟。
- PG failover：10-30 分钟。
- 数据修复：按业务一致性检查定义。

## 10. Evidence Compare

Evidence Compare 回答“修复前后到底变好了什么，还有什么没恢复”。

### 10.1 对比维度

| 维度 | 对比内容 |
| --- | --- |
| Coroot SLO | latency p50/p95/p99、error rate、throughput、resource saturation、SLO burn |
| ERP Business | 业务任务恢复、订单状态推进、租户 SLA、业务错误码 |
| Debug Trace | 同一用户动作或重放动作的 frontend、gateway、backend、db、middleware span |
| Middleware | PG、Redis、MQ 等关键健康项 |
| Host/K8s | systemd、pod readiness、rollout、events、CPU、memory、disk、network |
| Workflow Output | 输出变量、验证变量、回调状态 |

### 10.2 展示方式

- Summary row：每个维度显示 passed、failed、inconclusive、not_applicable。
- Trend mini chart：展示修复前、动作时刻、观察窗口、当前值。
- Delta badge：展示改善、恶化、无明显变化、证据不足。
- Evidence links：打开原始 EvidenceRef 或 rawRef。
- Criteria explanation：解释为什么通过或失败。

### 10.3 Debug Trace 对比

DebugCase 修复后必须支持：

- before trace id。
- after trace id。
- user action。
- route。
- TTFB before/after。
- slow span before/after。
- database span before/after。
- middleware span before/after。
- trace completeness。

如果 after trace 缺失，验证状态是 `inconclusive`，不能自动标记 recovered。

## 11. Rollback Verification

Rollback Verification 回答“回滚是否真的把系统带回可接受状态”。

### 11.1 回滚卡片

展示：

- 原动作。
- 回滚动作。
- 触发 rollbackCriteria。
- rollback ActionProposal。
- rollback WorkflowRun。
- rollback VerificationRecord。
- rollback status。
- 数据一致性检查。
- 业务未恶化检查。
- HostLease 和审批状态收敛。

### 11.2 回滚验证项

必须展示：

- 原动作是否撤销。
- 服务是否回到可用状态。
- 数据是否一致。
- 业务是否没有进一步恶化。
- 主机锁是否释放或关闭。
- 审批状态是否关闭。
- 后续手工事项。

回滚失败时：

- RecoveryStatus 进入 `failed` 或 `requires_manual_followup`。
- failedPoint 显示回滚失败位置。
- nextRecommendation 给出下一步。
- 生成 EvidenceRef 和 Experience candidate。

## 12. Manual Confirmation

Manual Confirmation 回答“哪些恢复结论需要人确认，谁确认了什么”。

### 12.1 请求列表

| 列 | 内容 |
| --- | --- |
| Confirmation | id、type、caseId、target |
| Role | user、SRE、DBA、service owner、business owner |
| Reason | 为什么需要人工确认 |
| Evidence | 需要查看的证据引用 |
| Status | pending、confirmed、rejected、expired、revoked |
| Validity | 有效期、过期时间 |
| Action | 打开确认、拒绝、请求更多证据 |

### 12.2 确认表单

表单字段：

- decision：confirmed、rejected、need_more_evidence。
- summary。
- evidenceRefs。
- scope：全部恢复、部分恢复、仅某租户、仅某服务。
- validity。
- riskAccepted。
- comment。

确认时必须展示：

- 当前 RecoveryStatus。
- 自动 checks 结果。
- 未通过或 inconclusive checks。
- 人工确认的作用范围。
- 决策会如何影响 case close gate。

普通用户确认业务页面恢复时，只能确认自己可见的业务范围，不能覆盖 DBA 或 SRE 级别的技术验证。

## 13. Recovery Decision 与关闭门禁

Recovery Decision 回答“为什么可以或不可以把 case 认定恢复”。

### 13.1 Decision Dialog

打开时展示：

- caseId。
- current CaseStatus。
- current RecoveryStatus。
- required checks summary。
- failed checks。
- inconclusive checks。
- manual confirmations。
- rollback status。
- postmortem / experience candidate 状态。

可选决策：

- mark_mitigated。
- mark_recovered。
- mark_partially_recovered。
- mark_failed。
- trigger_rollback。
- mark_rolled_back。
- requires_manual_followup。

`mark_recovered` 必须满足：

- required checks passed。
- 无 required check failed。
- observation window completed。
- 业务验证通过或获得具备权限的 manual confirmation。
- 无未收敛 rollback。

如果允许人工覆盖，必须输入：

- 覆盖原因。
- 风险接受者。
- evidenceRefs。
- 有效期。
- follow-up。

### 13.2 Close Gate

Case close 前展示：

- VerificationRecords 状态。
- pending approval。
- executing workflow。
- active HostLease。
- failed rollback。
- missing postmortem evidence。
- unreviewed experience candidate。

关闭按钮只有在 close gate 通过或授权覆盖后可用。

## 14. 嵌入入口与跨模块集成

### 14.1 Incident Control Plane

Case 工作台嵌入：

- Case Header 的 RecoveryStatus。
- Verification Tab。
- Recovery Decision Dialog。
- Close Gate。
- Timeline events：verification.created、verification.completed、recovery.decision、rollback.verified。

### 14.2 Execution Fabric

Workflow Run Detail 嵌入：

- End outputs。
- verification refs。
- callback status。
- observation window。
- recovery contribution。

Runbook Instance 嵌入：

- 当前 verify step。
- VerificationSpec。
- VerificationRecord。
- failedPoint。
- rollback step。

### 14.3 Governed Action & RBAC

Approval 详情嵌入：

- VerificationSpec。
- expectedEffect。
- rollbackCriteria。
- missing verification blocker。

高风险 ActionProposal 如果缺少 VerificationSpec，审批按钮禁用并显示阻断原因。

### 14.4 Observability & Debug Trace

DebugEvent 详情嵌入：

- 修复前 trace。
- 修复后 trace。
- slow span 对比。
- 用户确认。
- trace completeness。

### 14.5 Middleware Repair

RepairPlan 详情嵌入：

- PG/Redis/MQ 健康验证。
- 数据一致性检查。
- role/replication/lag/lock/disk 验证。
- rollback verification。

### 14.6 Learning, Memory & Eval

验证完成后提供：

- 创建 Experience candidate。
- 创建 Eval case。
- 更新 Memory candidate。
- 复盘中引用 VerificationRecord。

成功和失败都可以生成候选，但必须带上禁用条件、失败点和适用范围。

## 15. 前端数据模型

建议新增 `web/src/api/verificationRecovery.ts`，不要把验证 API 挤进 `complexPagesApi.ts`。

```ts
export type VerificationType =
  | "slo"
  | "business"
  | "trace"
  | "middleware"
  | "host"
  | "k8s"
  | "manual"
  | "rollback";

export type VerificationStatus =
  | "pending"
  | "running"
  | "passed"
  | "failed"
  | "inconclusive"
  | "skipped";

export type RecoveryStatus =
  | "not_started"
  | "mitigation_in_progress"
  | "mitigated"
  | "verifying"
  | "recovered"
  | "partially_recovered"
  | "failed"
  | "rolled_back"
  | "requires_manual_followup";

export type VerificationCheckView = {
  id: string;
  name: string;
  type: VerificationType;
  required: boolean;
  status: VerificationStatus;
  criteriaSummary: string;
  baseline?: VerificationValueView;
  current?: VerificationValueView;
  deltaSummary?: string;
  evidenceRefs: EvidenceRefView[];
  failedPoint?: string;
  nextRecommendation?: string;
  startedAt?: string;
  finishedAt?: string;
};

export type VerificationSpecView = {
  id: string;
  source: "runbook" | "workflow" | "ai" | "repair_plan" | "manual";
  caseId?: string;
  actionProposalId?: string;
  workflowRunId?: string;
  runbookInstanceId?: string;
  repairPlanId?: string;
  requiredChecks: VerificationCheckTemplateView[];
  optionalChecks: VerificationCheckTemplateView[];
  observationWindow: ObservationWindowSpecView;
  successCriteria: CriteriaView[];
  failureCriteria: CriteriaView[];
  rollbackCriteria: CriteriaView[];
  evidencePolicy: EvidencePolicyView;
};

export type VerificationRecordView = {
  id: string;
  caseId: string;
  actionProposalId?: string;
  workflowRunId?: string;
  runbookInstanceId?: string;
  repairPlanId?: string;
  type: VerificationType;
  status: VerificationStatus;
  recoveryStatusContribution?: RecoveryStatus;
  timeWindow: TimeWindowView;
  observationWindow?: ObservationWindowView;
  checks: VerificationCheckView[];
  evidenceRefs: EvidenceRefView[];
  resultSummary?: string;
  failedPoint?: string;
  nextRecommendation?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type ObservationWindowView = {
  id: string;
  state:
    | "waiting_for_action"
    | "smoke_check_running"
    | "observation_waiting"
    | "trend_check_running"
    | "business_validation_running"
    | "decision_required"
    | "completed"
    | "failed"
    | "expired";
  startsAt?: string;
  endsAt?: string;
  durationSeconds: number;
  elapsedSeconds: number;
  nextSampleAt?: string;
  completedChecks: string[];
  pendingChecks: string[];
  triggeredRollbackCriteria: CriteriaView[];
};

export type RecoveryDecisionView = {
  id: string;
  caseId: string;
  decision:
    | "mark_mitigated"
    | "mark_recovered"
    | "mark_partially_recovered"
    | "mark_failed"
    | "trigger_rollback"
    | "mark_rolled_back"
    | "requires_manual_followup";
  fromStatus: RecoveryStatus;
  toStatus: RecoveryStatus;
  reason: string;
  evidenceRefs: EvidenceRefView[];
  override: boolean;
  acceptedRiskBy?: string;
  followUpRefs: string[];
  createdBy: string;
  createdAt: string;
};
```

## 16. API 契约

页面优先使用 canonical API：

```text
GET    /api/v1/verifications
POST   /api/v1/verifications
GET    /api/v1/verifications/{id}
PATCH  /api/v1/verifications/{id}
POST   /api/v1/verifications/{id}/run
POST   /api/v1/verifications/{id}/manual-confirm
POST   /api/v1/verifications/{id}/mark-failed
GET    /api/v1/verification-specs/{id}
POST   /api/v1/verification-specs
PATCH  /api/v1/verification-specs/{id}
GET    /api/v1/cases/{case_id}/recovery-status
GET    /api/v1/cases/{case_id}/verification-summary
POST   /api/v1/cases/{case_id}/recovery-decision
GET    /api/v1/cases/{case_id}/close-gate
POST   /api/v1/cases/{case_id}/close
```

建议补充查询 API：

```text
GET    /api/v1/recovery-queue
GET    /api/v1/verifications/{id}/evidence-compare
GET    /api/v1/verifications/{id}/observation-window
POST   /api/v1/verifications/{id}/checks/{check_id}/rerun
POST   /api/v1/verifications/{id}/manual-confirmations
POST   /api/v1/verifications/{id}/experience-candidate
POST   /api/v1/verifications/{id}/eval-case
```

兼容期可以让 `verificationRecovery.ts` fallback 到 case、runner、coroot、middleware 现有 API，但页面组件只能依赖 `verificationRecovery.ts` 的 view-model。

## 17. 状态与数据流

```text
Route / query
  -> verificationRecovery API
  -> normalizeVerificationRecord / normalizeRecoveryQueueItem
  -> page state
  -> summary strips, tables, drawers, dialogs
```

实时更新策略：

- MVP 使用刷新按钮和轻量 polling。
- 观察窗口运行中可以 5-15 秒轮询 record summary，不轮询大证据正文。
- 后续接入统一 transport/realtime event，事件类型包括 verification.started、verification.check.completed、observation.window.updated、recovery.decision.created、rollback.verified。
- 任何 Workflow、Tool、Approval、HostLease 状态必须来自 Execution Fabric 或 case timeline，不在 Verification 页面重复维护。

## 18. 错误、空态与加载态

- 总览加载：显示固定高度 skeleton，避免表格跳动。
- 记录详情加载：Header skeleton + checks skeleton。
- API 失败：顶部 StatusAlert，保留上一次成功数据。
- 来源不可用：Source Health 显示 degraded/unavailable，相关 check 显示 inconclusive 或 blocked。
- 空队列：显示“当前没有需要处理的验证或恢复阻塞”。
- 无权限：显示“无权查看此验证证据”，不暴露资源名、指标细节或业务数据。
- Observation Window 超时：显示 expired，并给出 nextRecommendation。
- EvidenceRef 丢失：对应 check 显示 inconclusive，不能默认为 passed。
- manual confirmation 过期：状态显示 expired，并要求重新确认。

## 19. 响应式与可访问性

- 桌面端：主区 + 右侧 Drawer。
- 平板端：Drawer 降级为底部 Sheet。
- 手机端：表格改为列表，Recovery Strip 横向滚动，Tabs 横向滚动。
- 所有状态都必须有文本，不只依赖颜色。
- 倒计时必须同时展示绝对时间和剩余时间。
- 趋势图必须有文本 summary。
- icon button 必须有 aria-label 或 tooltip。
- 确认、回滚、恢复决策按钮必须有二次确认。

## 20. 验收标准

- `/verification` 能展示跨 case 的 recovery queue、source health、recovery strip 和阻塞排序。
- Case header 能显示 `RecoveryStatus`，且不会把 workflow 成功误显示为 recovered。
- 高风险 ActionProposal 缺少 VerificationSpec 时，审批或执行入口被阻断并展示原因。
- `/verification/records/:verificationId` 能展示 checks、criteria、evidenceRefs、baseline/current/delta、failedPoint 和 nextRecommendation。
- Observation Window 能展示 smoke、等待、趋势、业务验证、决策和超时状态。
- DebugCase 修复后能用 before/after trace 对比验证慢点是否消失。
- PG 等中间件修复后能展示复制、锁、连接、磁盘/WAL、业务指标和人工 DBA 确认。
- Workflow 执行成功但 VerificationRecord failed 时，case 不能进入 resolved。
- 回滚动作有独立 VerificationRecord，且回滚失败能生成 failedPoint、nextRecommendation 和经验候选入口。
- Case close 前能展示 close gate，阻断未完成验证、pending approval、执行中 workflow、active HostLease 和未收敛回滚。
- 页面不直接调用裸 `fetch`，所有请求走 `web/src/api/verificationRecovery.ts` 或统一 API client。
- 页面不创建私有 WebSocket/SSE。
