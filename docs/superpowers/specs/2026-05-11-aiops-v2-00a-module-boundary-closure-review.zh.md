# aiops-v2 模块边界、冲突审查与场景化调整

日期：2026-05-11
状态：企业级平台分层审查
来源总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)

## 1. 审查结论

`aiops-v2` 仍按企业级智能运维平台设计：Case、Evidence、Coroot/Trace、OpsGraph、AI Reasoning、Governed Action、Runner Workflow、Verification、Experience Pack、Audit 都要保留。

需要调整的是产品暴露方式和第一阶段主路径：不要让普通用户在 11 个模块页面之间学习平台。企业能力应该作为后台控制内核和专家治理面存在，普通用户只通过任务入口完成闭环。

本轮审查结论：

- 不删除企业级能力。
- 不把所有能力做成普通用户主入口。
- 用两个高频场景验证产品主路径是否足够简单：浏览器插件慢请求 DebugCase、PG 集群 middleware_repair case。
- 对普通用户无感的复杂功能下沉为后台能力、专家页或后续阶段。
- 不改现有 AI Chat 页面和 Runner 工作流页面/功能；新增能力通过浏览器插件、后端编排、Case API、经验服务和现有 Runner 引擎接入。

## 2. 用户主路径

普通用户只需要理解两条路径。

慢请求：

```text
打开浏览器插件 Debug Mode
  -> 插件为下一次请求强制注入 trace id
  -> 再点慢按钮
  -> 进入 DebugCase
  -> AI 展示慢点、证据、建议和可修复项
  -> 用户确认具体动作
  -> 平台审批、执行、验证
  -> 输出恢复或失败报告
  -> 生成经验候选
```

PG 修复：

```text
AI Chat 输入“帮我修复 xxx 的 PG 集群”
  -> 进入 middleware_repair case
  -> 只读 RCA + 经验匹配
  -> 用户确认复用经验或新 RepairPlan
  -> 平台审批、执行、验证
  -> 输出恢复或失败报告
  -> 生成经验候选
```

页面入口收敛为：

- `AI Chat`：自然语言入口。
- `浏览器插件 Debug Mode`：慢请求调试入口。
- `Case 工作台`：统一承载证据、根因、计划、动作、验证和经验候选。
- `动作确认/审批`：只在生产写动作发生前出现。
- `恢复报告`：告诉用户是否恢复、失败点是什么、下一步是什么。

专家入口保留，但不强迫普通用户使用：

- Coroot / Debug Trace。
- Prompt Trace。
- Workflow Studio。
- Experience Pack 审核台。
- OpsGraph。
- RBAC / Policy / Audit。
- Eval。
- Multi-Agent。

## 3. 功能分层决策

| 能力 | 分层 | 调整决策 |
| --- | --- | --- |
| Case / Evidence / Timeline | 平台控制内核 | 必须保留，普通用户在 Case 工作台中感知 |
| Browser Plugin Debug Trace / Coroot Evidence | 平台控制内核 + 专家钻取 | 普通用户看摘要，专家可看 Trace/Coroot 细节 |
| AI Reasoning | 用户任务层 + 控制内核 | 普通用户看解释和计划，专家看 Prompt Trace |
| Governed Action | 控制内核 | 普通用户只看动作确认，管理员看策略和审计 |
| Runner Workflow / Runbook | 控制内核 + 专家治理 | 复用现有 Runner 引擎和 catalog，不改现有 Runner 页面 |
| Verification | 用户任务层 + 控制内核 | 普通用户看恢复报告，专家看检查明细 |
| Middleware Repair | 用户任务层 + 控制内核 | AI Chat 发起，Case 内承载，PG 先深做 |
| Experience Pack | 专家治理 + AI 推荐来源 | 普通用户只看到“命中某经验”，专家审核和发布 |
| OpsGraph | 控制内核 + 专家钻取 | 普通用户看影响路径摘要，不看大图 |
| Multi-Agent | 专家协作形态 | 后续增强，不作为第一阶段主路径 |
| Eval / Memory / Simple EvoMap | 后台治理能力 | 用于质量和演化，不暴露成普通用户概念 |

## 4. 冲突审查矩阵

| 冲突点 | 风险 | 调整决策 |
| --- | --- | --- |
| Case 状态由 01 和 11 同时维护 | Agent 私自改 case，timeline 不可回放 | 01 是唯一 case 状态机；11 只做协作投影 |
| RepairPlan 由 05 和 08 同时生成 | AI 输出绕过中间件安全基线 | 05 只输出 `repair_plan_draft`；08 拥有 RepairPlan 生命周期 |
| Candidate Store 由 09 和 10 同时实现 | 两套候选、审核和推荐索引 | 09 拥有 Candidate Store / Review Queue / Activation Index；10 是经验包专题模型 |
| ToolDispatcher 和 Governed Action 都做权限 | Workflow 可绕过审批 | 02 做授权和 token；06 执行前校验 token、lease 和 action hash |
| OpsGraph 和 Experience 都能写关系 | 经验审核后绕过图谱审核改边 | 03 拥有图谱补丁审核；09/10 只能生成 patch candidate |
| Observability 和 AI 都能说根因 | Coroot RCA 被当成最终结论 | 04 只产证据和质量；05 产 Hypothesis，必须引用支持证据和反证 |
| Verification 和 Case 都能判定 resolved | 工具成功但业务未恢复仍关闭 | 07 产 VerificationRecord/RecoveryStatus；01 用它作为 resolved/closed 门禁 |
| Runbook/Workflow 与 Experience 都能发布资产 | 候选绕过审核进入正式目录 | 10 只能生成 Runbook/Workflow draft；06 负责 validate/dry-run/publish |
| 前端子方案都要独立页面 | 普通用户被迫跨多个控制台操作 | 普通主路径先在 Chat + Case 工作台承载，独立页面作为专家钻取 |
| PG 之外的中间件也深度修复 | 范围过宽导致每类都不可靠 | PG 先做完整；其他中间件逐步复用对象和安全基线 |

## 5. 每个模块的用户暴露方式

### 01 Incident Control Plane

用户看到：

- Case 标题、状态、影响范围、当前结论、下一步、恢复报告。

后台保留：

- Case 状态机、EvidenceRef、timeline、关闭门禁、postmortem。

### 02 Governed Action & RBAC

用户看到：

- “将执行什么、影响什么、风险是什么、如何回滚、如何验证”。

后台保留：

- RBAC、Policy、Approval、ActionToken、资源锁、Audit。

### 03 OpsGraph & ERP Business Context

用户看到：

- “这个慢请求/PG 异常影响哪些服务和业务能力”。

后台保留：

- 图谱节点/边、根因路径、业务影响查询、图谱补丁审核。

### 04 Observability, Coroot & Debug Trace

用户看到：

- trace id、慢 span、Coroot 摘要、证据质量。

后台保留：

- DebugEvent、TraceContext、EvidenceQuality、Coroot raw ref、trace summary。

### 05 AI Reasoning & Prompt Trace

用户看到：

- 为什么慢/为什么异常、证据、建议、计划、风险、下一步。

后台保留：

- PromptCompiler、Hypothesis、ReasoningOutput、Prompt Trace、Eval hook。

### 06 Execution Fabric, Runbook & Workflow

用户看到：

- 当前执行到哪一步、成功/失败、输出摘要。

后台保留：

- ToolDispatcher、RunbookInstance、WorkflowRun、Runner state、Artifact。

### 07 Verification & Recovery

用户看到：

- 修复是否成功、依据是什么、失败点是什么、下一步是什么。

后台保留：

- VerificationSpec、VerificationRecord、Observation Window、RecoveryStatus。

### 08 Middleware Repair

用户看到：

- PG 根因分析、命中经验、新 RepairPlan、风险、验证结果。

后台保留：

- MiddlewareResource、只读诊断、安全基线、RecoveryAttempt。

### 09 Learning, Memory & Eval

用户看到：

- “本次处理已生成经验候选”或“命中已审核经验”。

后台保留：

- Candidate Store、Review Queue、Activation Index、Memory、EvalCase。

### 10 Experience Pack

用户看到：

- AI 回复中的经验来源、适用条件、禁用条件、成功率、验证项。

专家看到：

- 经验包候选、证据链、EvoMap lineage、技能蒸馏内容、Runner workflow 绑定、审核和发布。

### 11 Multi-Agent Collaboration

用户看到：

- 更清晰的计划和状态，不需要理解有几个 Agent。

后台保留：

- 后续可引入 AgentTask、AgentMessage、冲突处理和协作回放，但不能新增事实源或绕过治理。

## 6. Experience Pack 设计审查

最佳设计采用简单版三件套：

```text
Experience Pack
  = Skills 式经验蒸馏
  + Simple EvoMap 演化账本
  + 现有 Runner Workflow 执行底座
```

### 6.1 Skills 式经验蒸馏

经验包要沉淀“可复用技能”，不是堆日志：

- ProblemSignature：什么问题。
- EvidenceRecipe：需要哪些证据。
- DiagnosisLogic：如何判断根因。
- ActionRecipe：推荐哪些动作。
- Guardrails：哪些情况下禁止使用。
- Rollback：如何回退。
- Verification：如何证明恢复。
- FailureLearning：失败点和反模式。

### 6.2 Simple EvoMap 演化账本

Simple EvoMap 作为后台 append-only 账本和轻量索引，回答：

- 经验来自哪个真实 case。
- 哪些环境成功，哪些环境失败。
- 哪个经验替代哪个经验。
- 哪个经验是某环境的变体。
- 哪些失败导致降权、暂停或新增禁用条件。

### 6.3 现有 Runner Workflow 执行底座

经验包不直接执行生产动作，也不要求修改 Runner 页面，只绑定现有可执行资产：

- 只读诊断 workflow。
- 修复 workflow draft。
- 回滚 workflow。
- verification workflow。
- 人工 runbook 步骤。

执行时仍走：

```text
Experience recommendation
  -> user confirmation
  -> ActionProposal / Approval / ActionToken
  -> existing Runner Workflow / ToolDispatcher
  -> Verification
  -> Outcome back to EvoMap
```

## 7. 验收线

- 普通用户完成浏览器插件慢请求分析和 PG 修复，不需要学习模块名。
- 任意生产写动作都能追溯到 Case、ActionProposal、Approval、ActionToken、ToolResult、Verification 和 Audit。
- 任意 AI 根因结论都能追溯到 EvidenceRef、Coroot/trace 摘要、OpsGraph 路径、Prompt Trace 和支持/反证。
- 任意经验推荐都来自 Activation Index，并能追溯到 reviewer、source case、verification、禁用条件和 Runner artifact。
- 未审核经验不能进入 PromptCompiler、推荐排序、Runbook 正式列表或 Workflow 正式列表。
- DebugCase 没有 trace id 时，系统只能输出证据不足和只读建议。
- PG 修复没有备份、回滚和验证证据时，系统不能进入自动执行。
- Workflow 执行成功但验证失败时，Case 不能进入 resolved。
- 现有 AI Chat 页面和 Runner 工作流页面/功能不作为本方案改动范围。
