# aiops-v2 Learning, Experience Pack, Memory & Eval 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-enterprise-control-plane-design.zh.md)
专题文档：[2026-05-11-aiops-experience-packs-design.zh.md](2026-05-11-aiops-experience-packs-design.zh.md)

## 1. 模块定位

Learning 模块负责把真实运维过程沉淀为可审核、可复用、可评测的资产：Experience Pack、Runbook、Workflow、Memory、OpsGraph patch 和 Eval case。它不是让系统自由修改自己，而是把运行历史变成受治理的候选资产。

## 2. 设计目标

- 自动生成内容只能进入 candidate 或 draft。
- 人工审核通过后，资产才能影响 AI Chat、DebugCase 和中间件修复推荐。
- 成功和失败都要沉淀：成功路径、禁用条件、风险边界、回滚失败、环境不兼容。
- Memory 分层存储，避免把短期会话事实污染长期组织知识。
- Eval 覆盖 AI 推理、工具选择、权限拒绝、经验匹配和复盘质量。

## 3. 资产类型

```text
ExperiencePack
Gene
Capsule
RunbookDraft
WorkflowDraft
DebugRCACandidate
RepairPlanDraft
MemoryRecord
OpsGraphPatch
EvalCase
```

## 4. 经验包来源

- Coroot 事故关闭。
- AI Chat 运维过程。
- Debug Trace 慢请求分析。
- 中间件 RepairPlan 和 RecoveryAttempt。
- Workflow run history。
- Runbook instance。
- Approval/Audit。
- Postmortem。
- Verification outcome。

所有来源都必须经过脱敏、摘要、hash 和引用化。

## 5. Memory 分层

```text
MemoryRecord
  id
  scope: user | session | service | host | incident | org
  subjectRef
  content
  sourceRefs
  confidence
  ttl
  redactionStatus
  lastUsedAt
  stale
```

分层规则：

- `user`：用户偏好和交互习惯。
- `session`：当前会话短期上下文。
- `service`：服务画像、常见故障、恢复方式。
- `host`：主机能力、限制、历史问题。
- `incident`：事故经验、证据、验证结果。
- `org`：组织规则、审批习惯、变更窗口。

未审核经验不能写入 `org` memory。

## 6. Activation Index

Activation Index 是推荐唯一入口：

```text
ActivationIndexEntry
  id
  assetType: gene | capsule | debug_rca | repair_plan | runbook | workflow
  assetId
  scenarioSignature
  environmentProfile
  disabledConditions
  confidence
  fitness
  reviewRecord
```

PromptCompiler、Runbook matcher、Workflow 推荐器、Debug RCA matcher、RepairPlan matcher 只能读取 Activation Index。

Candidate Store 不能进入推荐。

## 7. Candidate Synthesizer

生成策略：

- 稳定判断 -> Gene candidate。
- 完整处置路径 -> Capsule candidate。
- 人工步骤清晰 -> Runbook draft。
- 可自动化步骤稳定 -> Workflow draft。
- 慢请求 trace -> Debug RCA candidate。
- 中间件修复 -> RepairPlan draft。
- 失败或误用 -> Anti-pattern、disabled condition、incompatibility edge。

## 8. Eval case

```text
EvalCase
  id
  sourceCaseId
  scenarioType: incident | debug | middleware_repair | permission | workflow | postmortem
  inputRefs
  expectedBehavior
  forbiddenBehavior
  scoringRubric
  linkedPromptTraceIds
```

评测目标：

- AI 是否引用正确证据。
- AI 是否识别证据不足。
- AI 是否避免越权和高风险直执。
- AI 是否正确匹配经验。
- AI 是否生成可执行、可审批、可验证计划。

## 9. 审核流程

```text
candidate created
  -> reviewer opens evidence
  -> reviewer checks environment, risk, rollback, verification
  -> approve gene/capsule/debug_rca/repair_plan
  -> approve runbook or workflow draft
  -> write Activation Index or publish draft
  -> update lineage and fitness
```

审核动作必须写 audit。

## 10. API 草案

```text
GET    /api/v1/experience-packs
GET    /api/v1/experience-packs/{id}
POST   /api/v1/experience-packs/match
GET    /api/v1/experience-packs/review-queue
POST   /api/v1/experience-packs/{id}/review
POST   /api/v1/experience-packs/from-debug-trace
POST   /api/v1/experience-packs/from-repair-case
GET    /api/v1/activation-index/search
GET    /api/v1/memory/search
POST   /api/v1/evals/cases
POST   /api/v1/evals/run
```

## 11. 验收标准

- Case 关闭后能生成经验包候选。
- Debug Trace 能生成 Debug RCA candidate。
- 中间件修复能生成 RepairPlan 或 Capsule candidate。
- 未审核候选不能进入 PromptCompiler、Activation Index 或推荐排序。
- 审核通过的资产能在下一次相似场景中作为推荐来源展示。
- Eval 能检测 prompt、工具 schema 或经验索引变更后的行为退化。
