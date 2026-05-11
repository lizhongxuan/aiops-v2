# aiops-v2 Verification & Recovery 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)

## 1. 模块定位

Verification & Recovery 模块负责回答“动作执行后是否真的恢复”。它不能只看命令退出码，而要结合 Coroot SLO、ERP 业务指标、用户侧 trace、K8s 状态、主机检查、中间件健康、Workflow outputs 和人工确认，形成可审计的恢复结论。

## 2. 设计目标

- 每个高风险动作必须有验证计划。
- case 进入 `resolved` 前必须有成功验证记录。
- 支持自动验证、人工确认、业务验证、回滚验证和 observation window。
- 修复失败时能定位失败点、失败原因、已回滚状态和下一步建议。
- 验证结果进入经验包、Memory、Eval 和复盘。

## 3. 验证对象

```text
VerificationRecord
  id
  caseId
  actionProposalId
  workflowRunId
  type: slo | business | trace | middleware | host | k8s | manual | rollback
  status: pending | running | passed | failed | inconclusive | skipped
  timeWindow
  checks
  evidenceRefs
  resultSummary
  failedPoint
  nextRecommendation
  createdAt
```

```text
VerificationSpec
  id
  source: runbook | workflow | ai | repair_plan | manual
  requiredChecks
  optionalChecks
  observationWindow
  successCriteria
  failureCriteria
  rollbackCriteria
```

## 4. 验证来源

Coroot：

- latency p50/p95/p99。
- error rate。
- throughput。
- resource saturation。
- SLO burn。
- topology health。

ERP：

- 业务任务恢复。
- 订单状态推进。
- 租户 SLA 恢复。
- 业务错误码下降。

Debug Trace：

- 同一用户动作或重放动作耗时恢复。
- 慢 span 消失或降到阈值内。
- 前端 TTFB 和接口耗时恢复。

中间件：

- PG replication lag、lock wait、connection saturation、disk/WAL。
- Redis memory、evictions、latency、replication。
- MQ lag、consumer health、broker state。

主机/K8s：

- systemd service status。
- pod readiness。
- rollout status。
- events/logs。
- CPU、memory、disk、network。

人工确认：

- 用户确认功能恢复。
- SRE 确认风险可接受。
- DBA 确认数据一致性。

## 5. Observation Window

动作完成后进入观察窗口：

```text
action completed
  -> immediate smoke check
  -> wait observation window
  -> metric trend check
  -> business validation
  -> close or rollback decision
```

不同场景默认窗口：

- 配置 reload：1-3 分钟。
- 服务重启：3-5 分钟。
- 扩容：5-15 分钟。
- PG failover：10-30 分钟。
- 数据修复：按业务一致性检查定义。

## 6. 恢复状态

```text
RecoveryStatus
  not_started
  mitigation_in_progress
  mitigated
  verifying
  recovered
  partially_recovered
  failed
  rolled_back
  requires_manual_followup
```

`mitigated` 表示技术风险下降，但业务未完全验证。`recovered` 表示技术和业务验证都通过。

## 7. 回滚验证

回滚不是“执行 rollback 命令”就结束，必须验证：

- 原动作是否撤销。
- 服务是否回到可用状态。
- 数据是否一致。
- 业务是否没有进一步恶化。
- 主机锁和审批状态是否释放或关闭。

回滚失败也必须生成 Evidence 和经验候选。

## 8. API 草案

```text
POST   /api/v1/verifications
GET    /api/v1/verifications/{id}
POST   /api/v1/verifications/{id}/run
POST   /api/v1/verifications/{id}/manual-confirm
POST   /api/v1/verifications/{id}/mark-failed
GET    /api/v1/cases/{case_id}/recovery-status
POST   /api/v1/cases/{case_id}/recovery-decision
```

## 9. UI 设计

验证页面展示：

- 本次动作和验证目标。
- 检查项列表和状态。
- 指标趋势图。
- 业务验证结果。
- 用户侧 trace 对比。
- 中间件健康对比。
- 失败点和下一步建议。

Case header 显示恢复状态，而不是只显示 workflow 成败。

## 10. 验收标准

- 高风险 ActionProposal 缺少 VerificationSpec 时不能审批通过。
- Workflow 执行成功但验证失败时，case 不能进入 resolved。
- DebugCase 修复后能用新 trace 对比验证慢点是否消失。
- PG 修复后能检查复制、锁、连接、磁盘和业务指标。
- 回滚动作有独立 VerificationRecord。
- 验证失败会生成经验包候选中的失败原因和禁用条件。
