# aiops-v2 企业级智能运维控制平面总体设计

日期：2026-05-11
状态：按浏览器插件调试、PG 修复和经验沉淀场景调整
范围：`aiops-v2` 企业级运维平台的产品分层、核心场景、模块边界、经验包机制、现有 AI Chat/Runner 兼容约束和跨模块不变量。

## 1. 北极星

`aiops-v2` 的目标仍然是企业级智能运维控制平面，不是一个轻量 ChatOps demo。它必须能承载生产治理、审计、审批、证据、自动化执行、恢复验证和经验沉淀。

但企业级不等于把复杂度交给用户。产品应该采用：

```text
简单用户入口
  + 企业级控制内核
  + 专家治理和资产沉淀
```

普通用户只需要会做三件事：

1. 页面慢了，打开浏览器插件的调试开关，再点一次慢按钮。
2. 中间件异常了，在 AI Chat 里说“帮我修复 xxx 的 PG 集群”。
3. 平台要动生产时，看懂风险、动作、回滚和验证，再确认。

平台内部仍然保留企业级能力：

- Case 作为事实根。
- Evidence / Trace / Coroot / OpsGraph 作为证据和上下文。
- AI Reasoning 负责解释、推理、计划和报告。
- Governed Action 负责权限、审批、ActionToken、审计和资源锁。
- Runner Workflow / Runbook 作为运维步骤底层执行引擎。
- Verification 证明恢复，失败时定位 failedPoint。
- Experience Pack 把真实运维过程蒸馏成可审核、可复用、可执行、可演化的运维技能。

硬约束：

- 不改现有 AI Chat 页面和交互功能；PG 修复请求通过后端意图识别、工具编排和 Case/经验服务接入现有 AI Chat。
- 不改现有 Runner 工作流页面和功能；自动化修复只复用现有 Runner workflow 引擎、workflow catalog、运行 API 和事件，不要求改 Runner Studio UI。
- 浏览器侧调试能力通过浏览器插件提供；业务页面无需为每个按钮改代码。

## 2. 产品分层

### 2.1 用户任务层

普通用户主入口只保留：

- `AI Chat`：输入问题、发起修复、查看解释和计划。
- `浏览器插件 Debug Mode`：插件强制为下一次请求注入 trace id，再次点击后自动发起 DebugEvent。
- `Case 工作台`：当前问题的证据、根因、计划、动作、验证和经验候选。
- `动作确认/审批`：只在生产写动作发生前出现。
- `恢复报告`：修复是否成功，失败点是什么，下一步是什么。

用户不需要主动理解 Prompt Trace、OpsGraph、Experience Pack、Workflow Studio、RBAC、Multi-Agent、Eval 等内部概念。

### 2.2 平台控制内核

这层是企业级能力的核心，不因用户入口简化而删除：

```text
Input
  -> Case
  -> Evidence / TraceContext / OpsGraph
  -> AI Reasoning
  -> Governed Action
  -> Runner Workflow / Runbook / ToolDispatcher
  -> Verification
  -> Recovery Report
  -> Experience Candidate
  -> Review
  -> Activation Index
```

### 2.3 专家治理层

这些能力对企业平台重要，但不作为普通用户主路径：

- Coroot / Debug Trace 专家钻取。
- Prompt Trace 和 AI 输出审计。
- Workflow Studio 和 Runbook Catalog。
- Experience Pack 审核台。
- OpsGraph 图谱和补丁审核。
- RBAC / Policy / Audit 管理。
- Eval / 回归评测。
- Multi-Agent 协作视图。

原则是：**专家可钻取，普通用户不必学。**

## 3. 核心场景一：浏览器插件慢请求调试

```text
用户打开浏览器插件
  -> 插件进入 aiops debug 状态
  -> 插件预生成或向 aiops-v2 申请 trace id / debug event id
  -> 再次点击慢按钮
  -> 插件拦截浏览器请求并强制插入 traceparent / baggage / x-aiops-debug
  -> 插件向 aiops-v2 上报 DebugEvent
  -> 网关、后端、DB、缓存、中间件 span 继承同一 trace id
  -> Coroot / trace backend / 日志摘要归一化为 EvidenceRef
  -> 创建或关联 DebugCase
  -> AI 基于 Evidence + OpsGraph 路径 + Coroot RCA 生成 Hypothesis
  -> 用户看到为什么慢、证据是什么、建议是什么、哪些可自动化修复
  -> 用户确认具体修复动作
  -> Governed Action 审批和签发 ActionToken
  -> Runner Workflow / ToolDispatcher 执行
  -> Verification 用新 trace、Coroot SLO 和业务指标验证
  -> 输出恢复报告；失败时说明 failedPoint、失败原因和下一步
  -> 关闭 Case 时生成 Debug RCA / Runbook / Workflow / Experience candidate
```

必须满足：

- trace id 是主关联键；没有 trace id 只能给证据不足和只读建议。
- 插件注入必须是短 TTL、签名、可撤销的 debug marker，避免被普通用户伪造为生产调试流量。
- 插件只能采集 URL hash、route、action name、timing、status、trace id 和脱敏 header 摘要；不得采集请求体、cookie、token、密码或用户输入原文。
- Coroot RCA 是证据，不是最终结论。
- AI 必须展示支持证据、反证、证据质量和置信度。
- 自动化修复必须经过动作治理。
- 修复后必须用新 trace 或相同窗口对比证明是否恢复。

## 4. 核心场景二：PG 集群修复

```text
用户：帮我修复 xxx 的 PG 集群
  -> 现有 AI Chat 后端识别 middleware_repair 意图
  -> 创建 middleware_repair case
  -> 识别 MiddlewareResource、PG cluster 和业务影响
  -> 只读采集拓扑、角色、复制、锁、连接、磁盘、WAL、备份、Coroot 指标
  -> 查询 Activation Index 是否有已审核经验
  -> 命中经验：展示来源、适用环境、成功率、禁用条件、风险、验证项
  -> 未命中经验：生成 RepairPlan draft
  -> 用户确认复用经验或新 RepairPlan
  -> 高风险动作经 DBA / service owner 审批
  -> 复用现有 Runner Workflow / ToolDispatcher 执行受治理步骤
  -> Verification 检查 PG 健康、业务恢复和 Coroot 指标
  -> 成功或失败都生成恢复报告和经验候选
```

必须满足：

- “修复集群”不能直接变成重启、failover、删数据或写配置。
- PG 是第一类深度修复对象；Redis/MQ/Kafka/MySQL 复用模型逐步扩展。
- 数据破坏性动作默认不自动化。
- 已审核经验只能降低诊断和计划成本，不能绕过审批、备份检查、回滚和验证。
- 现有 AI Chat 页面不增加专用入口；响应内容仍通过现有消息、工具过程和审批确认能力呈现。

## 5. 功能保留、下沉和延后

“没必要”的准确含义不是删除，而是不让它们成为普通用户主路径、第一阶段交付阻塞或默认页面入口。

| 能力 | 处理方式 | 原因 |
| --- | --- | --- |
| 11 个模块独立页面 | 下沉为专家页 | 普通用户处理一次事故不应跨十几个控制台 |
| 完整 Multi-Agent 协作面板 | 延后到 P2 | 两个核心场景可由单 AI orchestrator 先闭环 |
| 完整 RBAC/Policy 编辑中心 | 保留内核，管理页延后 | 本阶段需要审批和审计，不需要完整组织治理 UI |
| 全量 OpsGraph / CMDB | 先做最小关系路径 | 慢请求和 PG 修复只需要页面、API、服务、中间件、业务影响 |
| 全量 Workflow Studio | 保留 Runner 底座，复杂编排延后 | 现有 Runner 页面和功能不改，先复用引擎和 catalog |
| 完整 Eval 平台 | 后台保留最小用例，工作台延后 | 需要防回归，但不应挡住主闭环 |
| 完整 Evolution Map UI | 后台建事件和 lineage，图谱 UI 延后 | 演化关系重要，但普通用户不需要看大图 |
| 所有中间件自治修复 | PG 先做深，其他渐进扩展 | 深修复比广覆盖更重要 |
| 自动图谱补丁发布 | 只生成候选，人工审核 | 避免经验直接污染业务图谱 |

## 6. 运维经验积累机制

Experience Pack 是 `aiops-v2` 的长期壁垒。按本阶段需求采用简单版机制：**EvoMap 只做轻量演化账本，Runner workflow 只做底层执行引擎，Skills 只做经验蒸馏格式**。不先做复杂图谱 UI、自主进化发布或新 Runner 页面。

```text
Skills 经验蒸馏
  + 简单版 EvoMap 演化账本
  + 现有 Runner Workflow 执行引擎
```

### 6.1 Skills 经验蒸馏

经验包要把一次真实运维过程提炼成可复用技能，而不是保存原始日志：

- 适用场景和问题签名。
- 必须采集的证据。
- 根因判断逻辑。
- 推荐动作和动作风险。
- 禁用条件。
- 回滚方案。
- 验证方式。
- 失败点和反模式。
- 可绑定的 Runner workflow / runbook。

### 6.2 简单版 EvoMap

EvoMap 不作为普通用户的大图 UI，也不在本阶段做复杂自我进化。它只作为后台 append-only 账本和索引：

- 经验来自哪个 case。
- 在什么环境成功过。
- 在什么环境失败过。
- 哪个版本替代了旧版本。
- 哪些环境不兼容。
- 最近失败是否导致降权、暂停或生成变体候选。

### 6.3 复用现有 Runner Workflow 引擎

经验包本身不直接执行生产动作，也不要求修改 Runner 页面。它只描述“怎么判断、怎么做、怎么验证”，并绑定现有 Runner 能执行的 workflow 或 runbook：

- 只读诊断步骤。
- 人工 Runbook 步骤。
- Runner Workflow 自动化步骤。
- 回滚 Workflow。
- Verification Workflow。

Governed Action 决定能不能做，现有 Runner 引擎决定怎么执行，Experience Pack 决定什么时候应该这么做。

### 6.4 推荐模型

```text
Candidate Store
  -> Human Review
  -> Activation Index
  -> AI Chat / DebugCase / Middleware Repair 推荐
```

硬约束：

- 未审核候选不能进入 PromptCompiler。
- 未审核候选不能进入推荐排序。
- 已审核经验命中时必须展示来源、适用条件、禁用条件、风险和验证项。
- 经验只推荐计划，不跳过用户确认、审批和验证。

## 7. 模块设计文档

核心子方案：

| 模块 | 文档 | 用户可见方式 |
| --- | --- | --- |
| 00a Module Boundary & Closure Review | [2026-05-11-aiops-v2-00a-module-boundary-closure-review.zh.md](2026-05-11-aiops-v2-00a-module-boundary-closure-review.zh.md) | 分层暴露和边界审查 |
| 01 Incident Control Plane | [2026-05-11-aiops-v2-01-incident-control-plane-module-design.zh.md](2026-05-11-aiops-v2-01-incident-control-plane-module-design.zh.md) | Case 工作台 |
| 02 Governed Action & RBAC | [2026-05-11-aiops-v2-02-governed-action-rbac-module-design.zh.md](2026-05-11-aiops-v2-02-governed-action-rbac-module-design.zh.md) | 动作确认和审批 |
| 03 OpsGraph & ERP Business Context | [2026-05-11-aiops-v2-03-opsgraph-business-context-module-design.zh.md](2026-05-11-aiops-v2-03-opsgraph-business-context-module-design.zh.md) | 影响路径摘要，专家钻取 |
| 04 Observability, Coroot & Debug Trace | [2026-05-11-aiops-v2-04-observability-debug-trace-module-design.zh.md](2026-05-11-aiops-v2-04-observability-debug-trace-module-design.zh.md) | Debug 结果和证据质量 |
| 05 AI Reasoning & Prompt Trace | [2026-05-11-aiops-v2-05-ai-reasoning-prompt-trace-module-design.zh.md](2026-05-11-aiops-v2-05-ai-reasoning-prompt-trace-module-design.zh.md) | AI 解释、计划、报告，专家 Prompt Trace |
| 06 Execution Fabric, Runbook & Workflow | [2026-05-11-aiops-v2-06-execution-fabric-runbook-workflow-module-design.zh.md](2026-05-11-aiops-v2-06-execution-fabric-runbook-workflow-module-design.zh.md) | 执行步骤和结果，专家 Workflow Studio |
| 07 Verification & Recovery | [2026-05-11-aiops-v2-07-verification-recovery-module-design.zh.md](2026-05-11-aiops-v2-07-verification-recovery-module-design.zh.md) | 恢复报告和 failedPoint |
| 08 Middleware Repair | [2026-05-11-aiops-v2-08-middleware-repair-module-design.zh.md](2026-05-11-aiops-v2-08-middleware-repair-module-design.zh.md) | AI Chat 发起，Case 内承载 |
| 09 Learning, Memory & Eval | [2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md](2026-05-11-aiops-v2-09-learning-memory-eval-module-design.zh.md) | 后台候选、审核和激活 |
| 10 Experience Pack | [2026-05-11-aiops-v2-10-experience-packs-design.zh.md](2026-05-11-aiops-v2-10-experience-packs-design.zh.md) | 专家审核，AI 推荐来源 |
| 11 Multi-Agent Collaboration | [2026-05-11-aiops-v2-11-multi-agent-module-design.zh.md](2026-05-11-aiops-v2-11-multi-agent-module-design.zh.md) | P2 专家协作形态 |

前端子方案 `01a-11b` 是终态设计和专家页参考。第一阶段落地时，普通用户主路径必须优先收敛到 **AI Chat / 浏览器插件 Debug Mode / Case 工作台 / 动作确认 / 恢复报告**。

已有专题文档继续作为局部来源：

- [2026-05-11-runner-embedded-runtime-design.md](2026-05-11-runner-embedded-runtime-design.md)：Runner Embedded Runtime 设计。
- [2026-05-10-local-coroot-mcp-aiops-v2-design.zh.md](2026-05-10-local-coroot-mcp-aiops-v2-design.zh.md)：Coroot MCP 接入设计。
- [2026-05-03-erp-sre-runtime-design.zh.md](2026-05-03-erp-sre-runtime-design.zh.md)：ERP SRE runtime 设计。

## 8. Ownership

| 模块 | 拥有 | 不直接做 |
| --- | --- | --- |
| 01 | Case、EvidenceRef 引用、timeline、状态机、关闭门禁 | 不诊断、不执行、不生成经验内容 |
| 02 | ActionProposal、PolicyDecision、Approval、ActionToken、Audit、必要资源锁 | 不执行工具、不编排 DAG、不判断恢复 |
| 03 | OpsGraph 查询、业务影响、环境上下文、图谱补丁审核 | 不做静态 CMDB，不因 AI 推理直接改图 |
| 04 | DebugEvent、TraceContext、Coroot/trace/log/probe Evidence、EvidenceQuality | 不给最终根因、不生成修复计划 |
| 05 | PromptCompiler、Hypothesis、ReasoningOutput、计划草稿、PromptTrace | 不直接执行工具，不拥有 RepairPlan/Workflow/Experience 生命周期 |
| 06 | Tool schema、ToolDispatcher、RunbookInstance、WorkflowRun、执行结果回写 | 不签发授权，不绕过 02 |
| 07 | VerificationSpec、VerificationRecord、RecoveryStatus、failedPoint | 不执行修复，不关闭 case |
| 08 | MiddlewareResource、PG RCA、RepairPlan、RecoveryAttempt | 不把自然语言直接变成写动作 |
| 09 | Candidate Store、Review Queue、Activation Index、最小 Eval case | 不维护第二套 Experience Pack 实现 |
| 10 | ExperiencePack、ProblemSignature、EnvironmentProfile、EvoMap lineage、Runner artifact binding | 不新建执行系统，不让候选进入推荐 |
| 11 | 多 Agent 协作投影和专家协作体验 | 不作为事实源，不绕过治理 |

## 9. 跨模块系统不变量

1. 生产写动作不能绕过 `ToolDispatcher`、Runner governance 或 Action governance。
2. 非只读动作必须绑定 `ActionProposal`，除非处于显式 break-glass。
3. 审批只批准具体动作，不批准模糊意图。
4. AI 生成的经验只能进入 candidate 或 draft。
5. Prompt Trace 是可回放治理记录，不是临时日志。
6. Runbook 不直接执行工具，只生成下一步动作提案。
7. Workflow 不替代 RBAC、Policy、ActionToken 和必要资源锁。
8. Coroot、trace、日志和探针输出都是证据；最终根因由 AI Reasoning 基于证据和反证形成。
9. 用户侧慢请求调试必须贯通 trace id；没有 trace id 的调试事件不能直接给出自动修复结论。
10. 中间件修复必须先完成只读根因分析和 `RepairPlan`；不得把自然语言“修复集群”直接映射为高风险动作。
11. Experience Pack 只能作为已审核路径被复用；候选经验不能跳过用户确认、审批和验证。
12. 不采集用户请求体、密码、token、cookie 原文作为调试证据；只保存脱敏摘要、trace id、span、指标和引用。
13. 现有 AI Chat 页面和 Runner 工作流页面/功能不作为本方案改动范围；所有新增能力通过后端编排、浏览器插件、Case API、工具注册、经验服务和现有 Runner 引擎接入。

## 10. 成功判据

用户简单：

- 普通用户可以不理解模块名，也能完成慢请求分析和 PG 修复。
- 一次处理不需要进入超过 4 个主入口。
- 每个 AI 建议都能解释“为什么、证据、风险、下一步”。

企业级可信：

- 任意生产写动作都能从 Case 追到 ActionProposal、Approval、ActionToken、ToolResult、Verification 和 Audit。
- 任意 AI 根因结论都能追到 EvidenceRef、Coroot/trace 摘要、OpsGraph 路径、Prompt Trace 和支持/反证。
- 任意经验推荐都来自 Activation Index，并能追到 reviewer、source case、verification、禁用条件和 Runner artifact。
- Workflow 执行成功但验证失败时，Case 不能进入 resolved。
- 修复成功或失败都能沉淀经验候选。

## 11. 非目标

- 不把普通用户入口设计成 11 个模块页面的大平台。
- 不把 MCP 接入数量当作核心目标。
- 不以全自动无人运维作为第一阶段定位。
- 不让 Runbook、Workflow、AI 或 Experience Pack 绕过动作治理。
- 不让候选经验自动发布到生产资产。
- 不为每个页面新增私有状态机、私有 SSE、私有执行链路。
- 不把 EvoMap、Prompt Trace、Eval、Multi-Agent 暴露成普通用户必须学习的概念。
