# aiops-v2 模块边界、冲突审查与闭环收敛

日期：2026-05-11
状态：总纲一致性审查
来源总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)

## 1. 审查结论

总纲和 01-11 主模块设计整体方向一致：系统以 Case 为事实根，以 Evidence/OpsGraph/Prompt Trace/Audit 为可追溯证据，以 Governed Action 控制生产写动作，以 Execution Fabric 执行，以 Verification 证明恢复，以 Learning/Experience Pack 沉淀资产，以 Multi-Agent 提升协作效率。

当前没有必须推翻的架构冲突。需要收敛的是实现边界：不要因为每个模块都有页面、API 和对象，就各自建立事实源、执行链路或审核队列。最终产品要保持“普通用户简单、专家可钻取、审计可回放”。

## 2. 简化后的用户主路径

普通用户只需要理解一条路径：

```text
发起问题或告警
  -> 进入 Case
  -> AI 基于证据解释原因和下一步
  -> 用户确认需要的动作
  -> 平台审批、上锁、执行
  -> 平台验证是否恢复
  -> 关闭 Case 并沉淀候选经验
```

页面入口收敛为：

- `AI Chat`：自然语言入口，承接操作、调试和修复请求。
- `Case 工作台`：统一看证据、假设、动作、审批、执行、验证、复盘和经验候选。
- `审批/动作确认`：只在需要生产写动作时出现。
- `验证结果`：告诉用户是否恢复、失败点是什么、下一步是什么。
- 专家钻取页：Coroot/Debug Trace、Prompt Trace、Workflow Studio、Experience Pack、OpsGraph、Multi-Agent。

不要让普通用户在 11 个模块页面之间完成一次事故处理。

## 3. 冲突审查矩阵

| 冲突点 | 文档来源 | 风险 | 裁剪决策 |
| --- | --- | --- | --- |
| Case 状态由 01 和 11 同时维护 | 01、11 | Agent 私自改 case，timeline 不可回放 | 01 拥有 case 状态机；11 只能生成 AgentMessage 和状态转换建议 |
| RepairPlan 由 05 和 08 同时生成 | 05、08 | AI 输出绕过中间件安全基线 | 05 输出 `repair_plan_draft`；08 校验、持久化并拥有 RepairPlan 生命周期 |
| Candidate Store 由 09 和 10 同时实现 | 09、10 | 两套候选、审核和推荐索引 | 09 是 Learning 总控制面；10 是 Experience Pack 详细模型；实现一套 Candidate Store/Activation Index |
| ToolDispatcher 和 Governed Action 都做权限 | 02、06 | 权限判断分裂，Workflow 可绕过审批 | 02 做授权和 token；06 执行前只校验 token、lease 和 hash |
| OpsGraph 和 Experience 都能写关系 | 03、09、10 | 经验审核后绕过图谱审核改边 | 03 拥有 OpsGraphPatch review/apply；09/10 只能生成 patch candidate |
| Observability 和 AI 都能说根因 | 04、05 | Coroot RCA 被当成最终结论 | 04 只产证据和质量；05 产 Hypothesis，必须引用支持证据和反证 |
| Verification 和 Case 都能判定 resolved | 01、07 | 工作流成功但业务未恢复仍关闭 | 07 产 VerificationRecord/RecoveryStatus；01 用它作为 resolved/closed 门禁 |
| Runbook/Workflow 与 Experience 都能发布资产 | 06、09、10 | 候选绕过审核进入正式目录 | 10 只能生成 Runbook/Workflow draft；06 负责 validate/dry-run/publish 生命周期 |

## 4. 每个模块的改动范围

### 01 Incident Control Plane

改动范围：

- Case 模型、状态机、合并关联、timeline、EvidenceRef 引用、关闭门禁。
- Case 工作台聚合展示其他模块输出。
- 负责“这件事是什么、是否能关闭”。

不做：

- 不做根因推理。
- 不执行工具。
- 不审核经验资产。

### 02 Governed Action & RBAC

改动范围：

- RBAC、Policy、ActionProposal、Approval、ActionToken、HostLease、Audit。
- 生产写动作的唯一授权边界。
- 多用户会话隔离和主机锁。

不做：

- 不执行工具。
- 不定义 Workflow DAG。
- 不判断业务恢复。

### 03 OpsGraph & ERP Business Context

改动范围：

- 业务能力、服务、API、中间件、主机、Runbook、Workflow、Experience 和 Case 的图谱关系。
- 业务影响、根因路径、经验匹配上下文。
- OpsGraphPatch 的审核和应用。

不做：

- 不当静态 CMDB。
- 不因一次 AI 推理直接修改图谱。
- 不执行修复。

### 04 Observability, Coroot & Debug Trace

改动范围：

- Coroot、trace、日志、前端 Debug Mode 和只读探针归一化为 EvidenceRef。
- TraceContext、DebugEvent、EvidenceQuality。
- 用户侧慢请求的 trace id 贯通和证据质量。

不做：

- 不给最终根因。
- 不生成修复计划。
- 不保存请求体、cookie、token、密码或敏感输入。

### 05 AI Reasoning & Prompt Trace

改动范围：

- PromptCompiler、ReasoningOutput、Hypothesis update、计划草稿、PromptTrace、Eval 运行入口。
- 将 AI 输出路由到目标模块。
- 解释证据、反证、置信度、风险、审批和验证要求。

不做：

- 不直接执行工具。
- 不拥有 RepairPlan、Workflow、Experience Pack 生命周期。
- 不用 prompt 代替 RBAC 或审批。

### 06 Execution Fabric, Runbook & Workflow

改动范围：

- Tool schema、ToolDispatcher、Runbook、RunbookInstance、Workflow、WorkflowRun、Runner state、fanout 和回调。
- 执行已授权动作并记录结果。
- AI 或经验生成的 Workflow 只能先进入 draft。

不做：

- 不签发 ActionToken。
- 不绕过 HostLease。
- 不让 Runbook 直接执行工具。

### 07 Verification & Recovery

改动范围：

- VerificationSpec、VerificationRecord、RecoveryStatus、Observation Window、回滚验证。
- 判断技术指标和业务指标是否真的恢复。
- 为 case close、经验候选和 Eval 提供验证证据。

不做：

- 不执行修复。
- 不关闭 case。
- 不只看命令退出码。

### 08 Middleware Repair

改动范围：

- MiddlewareResource、RepairPlan、RecoveryAttempt。
- PG/Redis/MQ/Kafka/MySQL 等中间件 RCA、安全基线、修复计划、风险、回滚和验证。
- 将自然语言“修复集群”变成可审计的 middleware_repair case。

不做：

- 不直接重启、failover、删数据或写配置。
- 不绕过 DBA/service owner 审批。
- 不维护独立于 Case 的事故状态。

### 09 Learning, Memory & Eval

改动范围：

- Learning 总控制面、Memory、EvalCase、Activation Index 契约、候选审核总流程。
- 从真实 case、验证、Prompt Trace、Audit、Postmortem 中生成可审核资产。
- 防止候选进入推荐。

不做：

- 不自由修改生产资产。
- 不维护第二套 Experience Pack 实现。
- 不把未审核经验写入 org memory。

### 10 Experience Pack 自我演化

改动范围：

- ExperiencePack、Gene、Capsule、Debug RCA、RepairPlan candidate、环境变体、fitness、lineage。
- Candidate 到 Activation / Runbook draft / Workflow draft 的详细审核门禁。
- 经验资产的不可变发布和变体演化。

不做：

- 不新建执行系统。
- 不原地修改已发布 Runbook/Workflow。
- 不让 candidate 进入 AI Chat、DebugCase 或 Middleware Repair 推荐。

### 11 Multi-Agent Collaboration

改动范围：

- AgentTask、AgentMessage、AgentConflict、SharedCaseContext 投影、协作回放。
- 多 Agent 分工收集证据、诊断、推进 Runbook、执行已授权动作、验证和学习。
- 冲突展示和人工决策提示。

不做：

- 不新增事实源。
- 不替代 Case timeline、Prompt Trace、Policy、HostLease 或 Audit。
- 不让 Execution Agent 自行发明动作。

## 5. 闭环完整性检查

| 闭环环节 | 负责人 | 必须产物 |
| --- | --- | --- |
| 输入归一化 | 01、04、08 | Case、EvidenceRef、TraceContext、MiddlewareResource |
| 上下文关联 | 03 | BusinessImpact、RootCausePath、entity refs |
| 推理与计划 | 05、11 | Hypothesis、ReasoningOutput、AgentMessage、PromptTrace |
| 动作治理 | 02 | ActionProposal、PolicyDecision、Approval、ActionToken、HostLease、AuditRecord |
| 执行编排 | 06 | ToolResult、WorkflowRun、RunbookInstance、Artifact |
| 恢复验证 | 07 | VerificationRecord、RecoveryStatus、failedPoint |
| 关闭复盘 | 01、07 | case close、postmortem draft、remaining risk |
| 经验沉淀 | 09、10 | Experience candidate、Memory candidate、EvalCase、OpsGraphPatch candidate |
| 审核激活 | 09、10、03、06 | ActivationIndexEntry、Runbook draft/publish、Workflow draft/publish、OpsGraphPatch applied |

如果一个功能不能放入这条闭环，就先不做，避免平台变成松散工具集合。

## 6. 实现时的验收线

- 任意生产写动作都能从 Case 追到 ActionProposal、Approval、ActionToken、HostLease、ToolResult、Verification 和 Audit。
- 任意 AI 结论都能从 Case 追到 EvidenceRef、OpsGraph 查询、Prompt Trace 和支持/反证。
- 任意经验推荐都来自 Activation Index，并能追到 reviewer、source case、verification 和 fitness。
- 任意自动生成内容在审核前只能是 candidate 或 draft。
- 任意多 Agent 输出都落到 SharedCaseContext、case timeline 或 Prompt Trace。
- 普通用户完成一次事故处理不需要进入超过 4 个主入口。
