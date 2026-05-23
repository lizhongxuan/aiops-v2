# aiops-v2 LLM 候选运维手册与候选 Workflow 生成设计

日期：2026-05-23
状态：正式设计稿
适用范围：AI Chat、OpsManual、Runner Workflow、Run Record、RCA Report、Self-Optimization Lab、Review Queue、Experience / Memory

## 1. 目标

让 aiops-v2 可以利用 LLM 提高运维手册和 Runner Workflow 的生成效率，同时保证生成资产在通过确定性验证、沙箱或真实回归测试、人工审核之前，不能进入生产可执行路径。

第一阶段目标是同时生成两类资产：

- `candidate_ops_manual`
- `candidate_runner_workflow`

两者都只能进入 `pending_review`，不能自动成为 `verified`，也不能被 `search_ops_manuals` 当作默认 `direct_execute` 手册。

## 2. 核心原则

1. LLM 只生成候选，不发布 verified 资产。
2. LLM 不能决定 pass/fail、审批、安全授权、ActionToken 或生产执行状态。
3. 手册和 Workflow 必须绑定同一个候选资产包，避免文档和执行图漂移。
4. 所有候选资产必须有 proof bundle；没有 proof bundle 不能进入人工审核。
5. 所有高风险执行能力仍走现有 RuntimeKernel、ToolDispatcher、Policy、Permission、Approval、ActionToken、Runner 和 Run Record。
6. 任何候选资产、报告、prompt trace、review note 都不能包含 API key、SSH 密码、Authorization header、token、secret ref 明文或原始脚本全文。

## 3. 总体架构

```text
触发信号
  -> Context Builder
  -> LLM Candidate Generator
  -> Deterministic Normalizer
  -> Gate Pipeline
  -> Proof Bundle
  -> Review Queue
  -> verified 发布
  -> Self-Optimization Feedback Loop
```

### 3.1 触发信号

候选生成由明确事件触发：

- `search_ops_manuals` 返回 `no_match`
- `search_ops_manuals` 返回 `adapt`
- `run_ops_manual_preflight` 被阻断
- Workflow dry-run、execute 或 verify 失败
- RCA 闭环成功，Run Record 有可复用价值
- selfopt / prompt-regression 发现手册检索、Workflow 或 RCA 退化
- 人工在 review 页面选择“生成候选手册和 Workflow”

### 3.2 Context Builder

Context Builder 负责收集可给 LLM 的输入，只允许传入脱敏和摘要后的上下文：

- 用户原始请求摘要
- `OperationFrame`
- 只读证据摘要
- 手册检索结果摘要
- preflight 结果摘要
- RCA report 摘要
- Run Record 摘要
- 失败 case 摘要
- 现有相似 verified 手册和 Workflow 的结构摘要

不得传入：

- 原始密钥、密码、token
- 原始长脚本全文
- 完整生产日志
- 未脱敏 tool output
- 可直接复用的生产私有连接串

## 4. Candidate Asset Bundle

候选资产以 bundle 为最小管理单元。一个 bundle 同时包含候选手册、候选 Workflow、proof bundle 和 review 状态。

```json
{
  "bundle_id": "cab-20260523-redis-rca-001",
  "status": "pending_review",
  "source": {
    "type": "run_record|rca_report|manual_no_match|manual_adapt|failed_case|human_request",
    "refs": ["run_record:...", "rca_report:..."]
  },
  "operation_frame": {},
  "candidate_ops_manual": {},
  "candidate_runner_workflow": {},
  "proof_bundle": {},
  "review": {
    "status": "pending_review",
    "reviewer": "",
    "notes": []
  },
  "created_at": "2026-05-23T00:00:00Z",
  "updated_at": "2026-05-23T00:00:00Z"
}
```

状态流：

```text
draft -> generated -> validated -> sandbox_passed -> pending_review
pending_review -> verified
pending_review -> needs_changes
pending_review -> rejected
verified -> deprecated
```

LLM 最多只能创建或修改 `draft` / `generated` / `needs_changes` 候选。`verified` 必须由人工审核动作产生。

## 5. 候选运维手册结构

候选手册复用现有 `internal/opsmanual.OpsManual` 结构，但增加候选元数据。

必须字段：

- `id`
- `title`
- `status = draft|pending_review`
- `operation.target_type`
- `operation.action`
- `operation.risk_level`
- `applicability`
- `required_context.required_inputs`
- `required_context.required_evidence`
- `cannot_use_when`
- `preconditions`
- `preflight_probe`
- `risk_policy`
- `workflow_ref`
- `validation`
- `fallback_guide`
- `verification`
- `document_markdown`

候选手册生成规则：

- `status` 固定为 `pending_review` 或更低。
- `workflow_ref.workflow_digest` 必须来自候选 Workflow 的确定性 digest，不能由 LLM 编造。
- `required_inputs` 必须和 Workflow 参数 schema 对齐。
- `cannot_use_when` 不能为空。
- 高风险操作必须声明审批和 ActionToken 要求。
- `document_markdown` 可以由 LLM 起草，但结构化字段必须被 normalizer 和 gate 校验。

## 6. 候选 Runner Workflow 结构

候选 Workflow 必须使用 Runner graph 结构，不允许只保存自然语言步骤。

必须包含阶段：

- `preflight`：只读检查
- `execute`：实际变更动作
- `verify`：独立验证
- `rollback`：回滚或降级路径

Workflow 生成规则：

- 参数必须 schema 化，不能写死 host、namespace、service、pod、token、路径等环境细节。
- preflight 节点必须只读。
- execute 节点必须声明风险等级。
- 高风险 execute 节点必须要求 approval 和 ActionToken。
- verify 节点不能只复用 execute 输出，必须有独立证据。
- rollback 节点必须存在；如果不可回滚，必须写明人工降级步骤和阻断理由。
- 所有 shell/script 节点必须经过 allowlist、denylist 和 secret scan。
- workflow digest 由系统计算。

## 7. LLM 生成方式

LLM 只输出结构化 candidate draft：

```json
{
  "ops_manual_patch": {},
  "runner_workflow_graph": {},
  "generation_notes": [],
  "assumptions": [],
  "missing_information": [],
  "proposed_eval_cases": []
}
```

生成后必须经过 Deterministic Normalizer：

- 补齐系统字段
- 清除 LLM 生成的 `verified`、`approved`、`published`
- 计算 workflow digest
- 将脚本拆成 Runner 节点
- 生成参数 schema
- 标记风险等级
- 绑定 source refs
- 运行 secret scan

LLM 不允许输出：

- 已执行、已验证、已审批
- 原始密钥
- 自动发布指令
- 绕过 Runner 的执行命令
- 未参数化的生产目标

## 8. Gate Pipeline

候选 bundle 必须通过 gate pipeline。

### 8.1 Schema Gate

- 手册 JSON schema 合法
- Workflow graph schema 合法
- 参数 schema 可被 UI 渲染
- source refs 存在

### 8.2 Manual Gate

- `required_inputs` 完整
- `required_evidence` 完整
- `cannot_use_when` 非空
- `risk_policy` 与 operation risk 匹配
- `workflow_ref` 存在且 digest 匹配
- 候选状态不是 verified

### 8.3 Workflow Gate

- Runner graph validate 通过
- preflight 只读
- execute 和 verify 分离
- rollback 存在或明确不可回滚
- 高风险节点必须 approval + ActionToken
- 节点输出可进入 Run Record

### 8.4 Safety Gate

P0 阻断项：

- 未审批写操作
- 缺 ActionToken 的高风险节点
- final text 或前端状态伪造执行完成
- candidate 被当作 verified
- workflow digest 不匹配
- secret 泄漏

### 8.5 Retrieval Gate

候选手册进入 review 前要用 eval case 验证：

- 应该命中的请求能命中该手册
- 不适用请求不能命中该手册
- 相似但不同对象不能误召回
- 缺少关键参数时必须 `need_info`，不能 `direct_execute`

### 8.6 Dry-Run / Sandbox Gate

默认先跑 dry-run。具备沙箱时跑 sandbox：

- 参数解析能通过
- preflight 能执行且只读
- execute 在沙箱中可执行
- verify 能证明结果
- rollback 能执行或给出明确人工路径

### 8.7 Selfopt Gate

生成候选后自动加入 selfopt：

- 生成 failed/new eval cases
- 跑 prompt-regression
- 跑 retrieval regression
- 跑 workflow validation
- 跑 dashboard smoke

P0 case 退化时 bundle 状态改为 `needs_changes`。

## 9. Proof Bundle

候选资产进入 review 前必须生成 proof bundle：

```json
{
  "schema_validation": {},
  "manual_validation": {},
  "workflow_validation": {},
  "retrieval_tests": {},
  "dry_run": {},
  "sandbox_run": {},
  "safety_scan": {},
  "secret_scan": {},
  "selfopt_scorecard": {},
  "known_limitations": []
}
```

Review 页面展示 proof bundle，而不是只展示 LLM 生成正文。

## 10. Review Queue

Review Queue 是候选资产进入 verified 的唯一入口。

审核动作：

- `approve`：转 verified，写 audit event
- `needs_changes`：保留 candidate，生成修改要求
- `reject`：不参与检索
- `deprecate`：下线 verified 资产

审核时必须看到：

- 手册 diff
- Workflow graph
- 参数 schema
- gate 结果
- proof bundle
- selfopt scorecard
- 适用和不适用 case
- 风险和回滚说明

## 11. 持续优化闭环

每次候选使用、失败或审核都产生优化数据：

```text
candidate generated
-> gate failed
-> LLM patch proposal
-> gate re-run
-> review feedback
-> candidate version bump
```

优化只生成 patch，不直接修改 verified 资产。

优化输入：

- failed gate
- failed eval case
- reviewer note
- Run Record
- verification failure
- rollback failure
- retrieval false positive / false negative

优化输出：

- manual patch
- workflow patch
- new eval case draft
- new selfopt regression case
- experience hint

## 12. 评分体系

每个 bundle 生成 scorecard：

- `manual_completeness_score`
- `workflow_validity_score`
- `retrieval_precision_score`
- `retrieval_recall_score`
- `preflight_readonly_score`
- `approval_safety_score`
- `action_token_score`
- `verification_score`
- `rollback_score`
- `secret_safety_score`
- `review_readiness_score`

P0 权重最高。P0 失败时总分不重要，直接 block。

## 13. 第一阶段落地范围

第一阶段只实现候选和验证，不实现自动发布：

- 新增 Candidate Asset Bundle 数据结构
- 新增 LLM generator service
- 新增 deterministic normalizer
- 新增 gate pipeline
- 新增 proof bundle 输出
- 新增 review queue API 最小版本
- 将候选资产接入 selfopt
- 生成 `candidate_ops_manual` 和 `candidate_runner_workflow`，状态固定 `pending_review`

不做：

- 自动 verified
- 自动生产执行
- 自动替换已有 verified 手册
- 默认连接远程主机
- 默认调用真实 LLM

## 14. 验收标准

1. LLM 能从 no_match、adapt、Run Record 或 RCA report 生成一对候选手册和候选 Workflow。
2. 候选资产状态始终是 `pending_review` 或更低。
3. Workflow graph validate 能通过。
4. 手册和 Workflow 参数 schema 一致。
5. 高风险节点必须审批和 ActionToken。
6. proof bundle 完整生成。
7. selfopt 能读取候选资产并运行相关 case。
8. review queue 能 approve / needs_changes / reject。
9. approve 前 candidate 不参与默认 direct execute。
10. 敏感信息扫描通过。

## 15. 推荐下一步

下一步写实施 todo，按以下顺序：

1. Candidate Asset Bundle schema 与 store
2. LLM generator 和 normalizer
3. Manual / Workflow gate pipeline
4. Proof bundle writer
5. Review queue API
6. selfopt 接入候选资产评分
7. K8s install 和 Coroot RCA 两个首批生成场景
