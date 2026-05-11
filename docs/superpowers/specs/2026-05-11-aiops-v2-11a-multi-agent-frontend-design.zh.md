# aiops-v2 Multi-Agent Collaboration 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-11-multi-agent-module-design.zh.md](2026-05-11-aiops-v2-11-multi-agent-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-11b-multi-agent-frontend-todo.zh.md](2026-05-11-aiops-v2-11b-multi-agent-frontend-todo.zh.md)

## 1. 页面目标

Multi-Agent Collaboration 的前端不是“展示一堆 Agent 正在聊天”的可视化效果，也不是让用户手工调度每个内部 Agent。它是 Case 内的协作治理界面，负责把多个 Agent 的分工、证据、假设、计划、动作、验证、学习候选和冲突收敛到同一个 `SharedCaseContext`，让用户能看清楚：当前是谁在做什么、依赖哪些证据、哪些结论存在冲突、哪些事项需要人工决策、哪些执行被治理边界阻止、以及每个 Agent 输出如何追溯到 Prompt Trace、case timeline 和 audit。

页面要让用户完成：

- 在 Case 工作台内查看当前参与的 Agent、角色、状态、任务、最新输出和阻塞原因。
- 在专家模式下查看 Agent 任务图、依赖、并行分支、输出协议和协作事件流。
- 查看 `SharedCaseContext` 中的 eventLog、evidenceRefs、hypotheses、actionProposals、workflowRuns、verificationRecords、hostLeases、promptTraceRefs 和 auditRefs。
- 区分普通用户摘要、case owner 可操作项、reviewer 可审查项、admin 可配置项。
- 对根因假设冲突、计划冲突、资源锁冲突、经验匹配冲突、验证结果冲突进行可解释处理。
- 在人工决策队列中选择计划、要求补证、批准进入 ActionProposal / Workflow / RepairPlan 流程，或拒绝高风险路径。
- 证明 Execution Agent 没有绕过 `ActionToken`、RBAC、Policy、HostLease、Workflow governance 和审批。
- 从任意 Agent 输出点进 Prompt Trace，查看该 Agent 当时看到的证据、工具、经验、隐藏工具和治理裁剪。
- 回放某个 case 的多 Agent 协作过程，证明所有结论都来自共享上下文而不是私有事实。
- 从 Learning Agent 输出进入 Experience Pack 候选审核，而不是直接发布生产资产。

设计原则：

- 多 Agent 是协作形态，不是第二套事实源。所有页面都围绕 case 展示，不创建脱离 case 的“Agent 私有事故”。
- 普通用户默认看到收敛后的状态、下一步和阻塞项；专家用户可以展开 Agent 细节。
- 第一屏展示共享上下文健康度、当前阶段、阻塞项、冲突和需要人工决策的事项，不展示内部 prompt 原文。
- Agent 输出必须类型化展示，不能把自然语言消息当作唯一事实。
- Execution Agent 的执行按钮必须绑定 `ActionToken`、审批状态和 HostLease；没有 token 时只显示阻断原因。
- Agent 失败必须可见，不能被折叠成“AI 已处理”。
- 敏感 evidence、系统策略、raw prompt、stdout/stderr、连接串、token、cookie、SecretRef 和未经授权实体只展示脱敏摘要或引用。

## 2. 与相邻模块的边界

Multi-Agent 前端是跨模块协作层，不替代已有业务页面。

| 模块 | Multi-Agent 前端如何使用 | 不做什么 |
| --- | --- | --- |
| 01 Incident Control Plane | 嵌入 Case 工作台，展示协作面板、任务、冲突、决策和回放 | 不重新定义 case 状态机 |
| 02 Governed Action & RBAC | 展示权限裁剪、ActionToken、审批、HostLease 和审计引用 | 不在 Agent 页面直接批准生产动作 |
| 03 OpsGraph | 展示 Observer / Diagnosis 使用的实体、边、业务影响和根因路径引用 | 不维护私有图谱 |
| 04 Observability | 展示 Coroot、Debug Trace、日志、指标和 trace evidence 的采集任务 | 不替代可观测页面的深度分析 |
| 05 AI Reasoning & Prompt Trace | 为每个 AgentMessage 提供 Prompt Trace drilldown | 不展示未授权 raw prompt |
| 06 Execution Fabric | 展示 Runbook Agent、Execution Agent、Workflow run 和 target fanout 状态 | 不绕过 Runner governance |
| 07 Verification & Recovery | 展示 Verification Agent 的验证记录和恢复判定 | 不让单个 Agent 私自关闭 case |
| 08 Middleware Repair | 展示中间件修复任务中的 RCA、RepairPlan、执行与验证协作 | 不把自然语言修复请求直接映射为动作 |
| 09/10 Learning & Experience Pack | 展示 Learning Agent 生成的候选和来源 case | 不让 Learning Agent 发布生产资产 |

因此 11a 的页面应该优先嵌入 `/incidents/:caseId`，再提供专家级 `/multi-agent` 控制台。若产品阶段只允许一个入口，MVP 应先在 Case 工作台增加 Multi-Agent Panel。

## 3. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/IncidentWorkbenchPage.tsx`：已有 Case 详情初版，包含 hypothesis、evidence、pending approvals、OpsGraph、Runbook 匹配和执行实例。
- `web/src/pages/ProtocolWorkspacePage.tsx`：已有复杂运维 AI Chat，右侧展示 Main Agent Process、Approval Rail、MCP Surfaces、Artifacts / Evidence 和 Workspace Contract。
- `web/src/pages/PromptTracePage.tsx`：已有 Prompt Trace 浏览能力，可承接 AgentMessage drilldown。
- `web/src/pages/ApprovalManagementPage.tsx`、`web/src/pages/HostsPage.tsx`：已有审批和主机治理入口，可承接 ActionToken、HostLease 和 RBAC 链接。
- `web/src/pages/RunbookCatalogPage.tsx`、`web/src/pages/RunnerStudioPage.tsx`：可承接 Runbook Agent 和 Execution Agent 产生的 Runbook / Workflow 关联。
- `web/src/pages/ExperiencePacksPage.tsx`：可承接 Learning Agent 生成的 Experience candidate。
- `web/src/transport/aiopsTransportTypes.ts`：已有 `AiopsRuntimeLiveness.activeAgents`、process block、approval、artifact 等状态结构。
- `internal/agentui/agent_event.go`、`internal/agentui/agent_event_projection.go`：已有 AgentEvent、AgentProjection、TimelineEntry、ApprovalProjection、ArtifactProjection 和 RuntimeLiveness。

主要不足：

- Case 工作台没有多 Agent 协作面板，用户无法看到 Observer、Diagnosis、Runbook、Execution、Verification、Learning 的分工和阻塞。
- Protocol Workspace 只展示 Main Agent Process，缺少多 Agent roster、任务图、AgentMessage 类型、冲突和共享上下文健康度。
- 前端没有 `AgentTask`、`AgentMessage`、`AgentConflict`、`AgentDecision` 和 `AgentCoordination` 的领域 view-model。
- Prompt Trace 还没有从 AgentMessage、AgentTask、task dependency 和 conflict 进入的稳定链接。
- 没有展示“Agent 不能维护私有事实”的共享上下文引用视图。
- 没有资源锁冲突、计划冲突、验证冲突和经验匹配冲突的专用处理页面。
- 没有在 UI 层证明 Execution Agent 只接受已批准 ActionToken。
- 没有多 Agent 协作回放视图，无法审计某个结论从哪个 Agent 输出、哪些证据和 Prompt Trace 演化而来。

## 4. 路由与信息架构

嵌入入口：

```text
/incidents/:caseId?tab=agents
/incidents/:caseId?drawer=agent-task&taskId=:taskId
/incidents/:caseId?drawer=agent-message&messageId=:messageId
/incidents/:caseId?drawer=agent-conflict&conflictId=:conflictId
```

专家入口：

```text
/multi-agent
/multi-agent/cases/:caseId
/multi-agent/tasks/:taskId
/multi-agent/conflicts
/multi-agent/conflicts/:conflictId
/multi-agent/replay/:caseId
/multi-agent/supervision
```

路由语义：

- `/incidents/:caseId?tab=agents`：Case 内嵌多 Agent 协作面板，是主要用户入口。
- `/multi-agent`：多 Agent 协作控制台，按 case、环境、状态、冲突、角色查看。
- `/multi-agent/cases/:caseId`：单 case 的专家协作视图，展示任务图、共享上下文、输出 ledger、冲突和回放。
- `/multi-agent/tasks/:taskId`：AgentTask 详情，展示输入、输出、依赖、权限、Prompt Trace 和 audit。
- `/multi-agent/conflicts`：跨 case 冲突队列，主要给 case owner、reviewer 和平台管理员使用。
- `/multi-agent/conflicts/:conflictId`：冲突处理工作台。
- `/multi-agent/replay/:caseId`：协作回放，按事件序列重放 Agent 输出和共享上下文变化。
- `/multi-agent/supervision`：Agent 角色、能力、健康度、降级策略和默认协作策略，仅管理员可见。

MVP 可以只做 `/incidents/:caseId?tab=agents` 和 `/multi-agent/cases/:caseId`，其余路由先由 drawer 或 tab 承载。

主导航建议：

| 页面 | 目标 |
| --- | --- |
| Case Agent Panel | Case 内查看当前多 Agent 协作状态 |
| Coordination Console | 专家级单 case 多 Agent 任务图和共享上下文 |
| Agent Task Board | 查看任务、依赖、状态、输入和输出 |
| Agent Output Ledger | 查看 AgentMessage、Prompt Trace、timeline 和 audit |
| Conflict Workbench | 处理假设、计划、资源、经验和验证冲突 |
| Human Decision Queue | 汇总需要用户、owner、reviewer 决策的事项 |
| Execution Guardrails | 证明 Execution Agent 被 ActionToken / RBAC / HostLease 约束 |
| Collaboration Replay | 回放协作过程和共享上下文变化 |
| Agent Supervision | 管理角色、能力、健康度和降级策略 |

## 5. Case Agent Panel

Case Agent Panel 是普通用户和 case owner 的主入口，嵌入 Case 工作台。

### 5.1 布局

```text
┌────────────────────────────────────────────────────────────┐
│ Case Agent Header: Coordination / Mode / Phase / Owner / Health │
├────────────────────────────────────────────────────────────┤
│ Shared Context Strip: Evidence / Hypotheses / Actions / Leases / Trace │
├──────────────────────────────┬─────────────────────────────┤
│ Agent Roster + Task Board      │ Decision & Conflict Rail     │
│ Observer / Diagnosis / Runbook │ Human decisions / conflicts  │
│ Execution / Verification /     │ / blocked execution / trace   │
│ Learning                       │                               │
├──────────────────────────────┴─────────────────────────────┤
│ Latest Agent Outputs / Prompt Trace / Timeline links          │
└────────────────────────────────────────────────────────────┘
```

桌面端使用左右布局。移动端将 Decision & Conflict Rail 收到 Agent Roster 下方，Agent 输出以时间线展示。

### 5.2 Header

显示：

- case id、case title、环境、SEV、当前 case 状态。
- coordination mode：single_agent、assisted_multi_agent、parallel_evidence、parallel_diagnosis、execution_locked、verification_only。
- commander：人工 owner 或 Incident Commander Agent。
- active agents count、running tasks count、blocked tasks count、conflict count、decision count。
- shared context health：fresh、stale、partial、degraded、unavailable。

动作：

- 刷新。
- 展开专家视图。
- 打开协作回放。
- 打开 Prompt Trace 过滤视图。
- 请求补证。
- 暂停新 Agent 任务。
- 重新调度失败的只读任务。

暂停新 Agent 任务只影响新任务分派，不停止已签发的执行动作。停止执行必须走 Execution Fabric 和 Action governance。

### 5.3 Shared Context Strip

Shared Context Strip 用引用计数和状态表达共享事实是否完整：

| 区域 | 显示内容 | 异常表达 |
| --- | --- | --- |
| Event Log | timeline event count、last seq、last update | event lag、projection stale |
| Evidence | evidenceRefs count、restricted count、redaction failed count | 证据不足、未授权证据 |
| Hypotheses | active、conflicting、confidence range | 假设冲突 |
| Action Proposals | drafts、awaiting approval、approved、rejected | 写动作未绑定 proposal |
| Workflow Runs | queued、running、failed、verified | run 和 case 未关联 |
| Verification | records、latest outcome、observation window | 验证冲突、恢复未证明 |
| Host Leases | active、exclusive、conflicted、expired | 资源锁冲突 |
| Prompt Trace | traceRefs count、missing count、restricted count | trace 缺失 |
| Audit | auditRefs count、policy denials | 审计缺失 |

点击任意区域打开对应详情 drawer，并保持当前 case 上下文。

## 6. Agent Roster

Agent Roster 回答“谁参与了这个 case，各自职责是什么，是否越权或失败”。

### 6.1 角色卡

每个 Agent 卡片显示：

- role：Incident Commander、Observer、Diagnosis、Runbook、Execution、Verification、Learning。
- agent id、display name、model、reasoning effort、startedAt、lastSeenAt。
- status：idle、queued、running、waiting、blocked、completed、failed、canceled。
- current task、last output summary。
- permissions summary：read scope、tool scope、action scope。
- output count、trace count、failure count。

### 6.2 角色语义

| Agent | UI 重点 | 禁止表达 |
| --- | --- | --- |
| Incident Commander | 任务分派、汇总、决策请求、case 收敛 | 不显示为“最高真相源” |
| Observer | 证据来源、采集状态、EvidenceRef | 不输出最终根因定论 |
| Diagnosis | 假设、支持证据、反证、置信度 | 不直接生成执行动作 |
| Runbook | Runbook / Capsule / Experience 匹配、step 进度 | 不绕过 ActionProposal |
| Execution | ActionToken、Workflow node、执行状态 | 无 token 时不显示执行入口 |
| Verification | VerificationSpec、恢复判定、失败点 | 验证冲突时不允许关闭 |
| Learning | Experience candidate、Eval case、Postmortem follow-up | 不发布生产资产 |

### 6.3 降级状态

当某个 Agent 不可用时，显示：

- unavailable：角色未启动或后端不支持。
- degraded：最近任务失败，但 case 仍可通过人工或单 Agent 推进。
- clipped：由于 RBAC、resource scope 或 evidence 权限裁剪，只能看到部分上下文。
- paused：管理员或 case owner 暂停新任务。

降级状态必须解释影响范围，例如“Observer 不可用，证据采集需要人工触发；Diagnosis 仍可基于已有证据运行”。

## 7. Agent Task Board

Agent Task Board 回答“每个 Agent 的任务如何并行或串行推进”。

### 7.1 任务列

任务按状态展示：

```text
Queued -> Running -> Waiting -> Blocked -> Completed
                    -> Failed
                    -> Canceled
```

也可以按角色泳道展示：

```text
Commander | Observer | Diagnosis | Runbook | Execution | Verification | Learning
```

### 7.2 任务字段

| 字段 | 说明 |
| --- | --- |
| taskId | AgentTask id |
| caseId | 所属 case |
| agentRole | 负责角色 |
| taskType | collect_evidence、update_hypothesis、match_runbook、draft_action、execute_token、verify、create_candidate |
| inputRefs | evidence、hypothesis、workflowRun、ActionToken、HostLease、Experience |
| outputRefs | AgentMessage、EvidenceRef、Hypothesis、ActionProposal draft、VerificationRecord、ExperienceCandidate |
| dependencyTaskIds | 前置任务 |
| status | queued、running、waiting、blocked、completed、failed、canceled |
| blocker | approval_required、host_lease_conflict、evidence_denied、policy_denied、trace_missing、tool_failed |
| promptTraceRef | 对应 Prompt Trace |
| auditRef | 对应 audit |

### 7.3 任务详情 Drawer

任务详情展示：

- 任务目标和输入引用。
- 依赖任务和依赖状态。
- Agent 实际可见的证据、工具、经验和策略摘要。
- 输出类型和落点：case timeline、Prompt Trace、audit、ActionProposal、Workflow run、Experience candidate。
- 是否需要人工决策。
- 失败点和 retry 条件。

任务详情不展示完整 raw prompt，除非用户具备 Prompt Trace reviewer 权限。

## 8. Agent Output Ledger

Agent Output Ledger 回答“Agent 输出了什么，这些输出如何进入共享上下文”。

### 8.1 AgentMessage 类型

支持模块定义中的消息类型：

| messageType | 展示方式 | 落点 |
| --- | --- | --- |
| evidence_summary | 证据摘要、来源、EvidenceRef、脱敏状态 | Evidence tab、timeline、Prompt Trace |
| hypothesis_update | 假设、支持证据、反证、置信度、冲突状态 | Hypothesis tab、timeline |
| plan_proposal | 计划、步骤、风险、需要决策事项 | Actions / Planning tab |
| action_proposal_draft | 动作草案、风险、目标、审批要求 | Governed Action |
| execution_status | token、workflow node、target、结果、失败点 | Execution Fabric |
| verification_result | VerificationSpec、观察窗口、结果、恢复判定 | Verification tab |
| learning_candidate | 候选类型、来源、审核状态、隔离状态 | Experience Pack |

### 8.2 Ledger 列

| 列 | 内容 |
| --- | --- |
| Time | createdAt、seq |
| Agent | role、name、status |
| Message | messageType、summary |
| Confidence | confidence、evidence coverage |
| Refs | evidence、hypothesis、action、workflow、verification、trace、audit |
| Decision | requiresHumanDecision、decision status |
| Impact | case state impact、downstream target |
| Action | 打开详情、打开 Prompt Trace、打开 Case Timeline |

### 8.3 普通用户视图

普通用户默认只看到：

- 已收敛摘要。
- 当前下一步。
- 阻塞项。
- 冲突存在但不展开敏感证据。
- 可点击的 Prompt Trace 摘要。

专家用户可以展开 AgentMessage 原始结构、refs、governance decisions 和 prompt layer 摘要。

## 9. Conflict Workbench

Conflict Workbench 回答“多个 Agent 意见不一致时如何收敛”。

### 9.1 冲突类型

| 类型 | 例子 | 处理方式 |
| --- | --- | --- |
| 根因假设冲突 | Diagnosis 认为 DB 慢，Observer 发现网络丢包 | 展示支持证据和反证，要求补证或选择当前假设 |
| 计划冲突 | Runbook 建议 failover，Diagnosis 建议只读诊断 | Commander 汇总后请求用户或审批人选择 |
| 资源锁冲突 | 两个执行任务请求同一 exclusive host lease | HostLease 硬拒绝，显示持有者和释放条件 |
| 经验匹配冲突 | 两个 Experience Pack 都命中但环境条件互斥 | 展示 matcher reason、disabled condition、fitness 和审核记录 |
| 验证结果冲突 | 指标恢复但业务 trace 仍慢 | Case 不能 resolved，要求继续验证或回滚 |

### 9.2 工作台布局

```text
┌────────────────────────────────────────────────────────────┐
│ Conflict Header: type / severity / status / case / owner      │
├────────────────────────────────────────────────────────────┤
│ Evidence Matrix: claim / supporting / contradicting / missing  │
├──────────────────────────────┬─────────────────────────────┤
│ Candidate Resolutions          │ Audit & Prompt Trace          │
│ choose / request evidence /    │ agent messages / trace / logs │
│ split plan / block execution   │                               │
└──────────────────────────────┴─────────────────────────────┘
```

### 9.3 决策动作

支持动作：

- accept_hypothesis。
- request_more_evidence。
- select_plan。
- split_plan。
- block_execution。
- wait_for_host_lease。
- mark_experience_incompatible。
- require_verification_window。
- reopen_case。
- escalate_to_human_owner。

每个动作必须写入 audit，并回写 case timeline。涉及生产动作的选择只产生 ActionProposal 或 Workflow draft，不直接执行。

## 10. Human Decision Queue

Human Decision Queue 回答“现在到底需要谁做什么决定”。

### 10.1 决策来源

- Incident Commander 要求选择计划。
- Diagnosis 要求补充证据。
- Runbook Agent 请求确认使用哪个经验或 Runbook。
- Execution Agent 等待 ActionToken 或审批完成。
- Verification Agent 要求决定是否延长观察窗口、回滚或继续修复。
- Learning Agent 请求是否生成经验候选或进入审核队列。

### 10.2 决策卡字段

| 字段 | 说明 |
| --- | --- |
| decisionId | 决策 id |
| caseId | 所属 case |
| requestedBy | Agent role |
| decisionType | choose_plan、request_evidence、approve_route、select_experience、verification_decision |
| summary | 决策摘要 |
| options | 可选项和风险 |
| requiredRole | owner、reviewer、admin、oncall |
| dueAt | 超时 |
| downstreamEffect | 选择后影响哪些任务 |
| auditRef | 审计引用 |

### 10.3 决策体验

- 默认按 case 严重级别、阻塞时间、风险排序。
- 每个选项展示证据支持、反证、风险、回滚和验证要求。
- 没有权限的用户只能查看摘要和责任人，不显示决策按钮。
- 决策提交前二次确认高风险影响。
- 决策后更新 AgentTask 和 case timeline，不直接修改底层生产资源。

## 11. Execution Guardrails

Execution Guardrails 回答“多 Agent 是否仍然被生产治理边界控制”。

### 11.1 Guardrail Strip

展示：

- User scope：发起用户、团队、case resource scope。
- Agent scope：各 Agent 继承后的有效权限。
- Tool visibility：visible tools、hidden tools denied、denied reason。
- ActionToken：none、draft、awaiting_approval、approved、expired、revoked。
- HostLease：not_required、pending、acquired、conflicted、expired、released。
- Workflow node：validate、dry-run、approval、running、failed、verified。
- Policy result：allowed、requires_approval、denied、break_glass_required。

### 11.2 执行阻断表达

Execution Agent 在以下情况下只显示阻断原因：

- 没有 ActionToken。
- ActionToken 未批准、过期或被撤销。
- RBAC 不允许该用户或 case scope。
- HostLease 冲突。
- Workflow validate 或 dry-run 失败。
- 风险策略要求额外审批。
- Prompt Trace 缺失或 Agent 输出未进入 audit。

阻断文案必须清楚说明“无法执行”的具体原因和下一步，例如“需要 SRE reviewer 批准 ActionProposal ap-123 后才能继续”。

## 12. Prompt Trace Drilldown

Prompt Trace Drilldown 回答“某个 Agent 为什么这么判断”。

从以下位置可以打开：

- Agent Roster 的最新输出。
- AgentTask 详情。
- AgentMessage 行。
- Conflict Workbench 的证据矩阵。
- Human Decision Queue 的选项。
- Execution Guardrails 的 denied tool。
- Collaboration Replay 的任一事件。

Drilldown 内容：

- Agent role、taskId、messageId、traceId。
- Prompt layers 摘要。
- visible tools 和 hidden tools denied。
- evidenceRefs 和 restricted evidence count。
- activatedExperienceRefs。
- model output summary。
- governance decisions。
- output routing。
- raw trace 权限状态。

Prompt Trace 页面应支持 query 参数：

```text
/ai-reasoning/prompt-traces/:traceId?agentRole=diagnosis&taskId=agtask-1&messageId=msg-1
```

若还未落地 `/ai-reasoning`，兼容跳转到 `/debug/prompts?traceId=...`。

## 13. Collaboration Replay

Collaboration Replay 回答“整个多 Agent 协作过程是否可回放、可审计、可解释”。

### 13.1 回放视图

```text
┌────────────────────────────────────────────────────────────┐
│ Replay Header: case / time range / seq / filters / export     │
├──────────────────────────────┬─────────────────────────────┤
│ Timeline                      │ Context Snapshot             │
│ Agent events / messages /     │ evidence / hypotheses /      │
│ approvals / workflow / verify │ actions / leases / traces    │
├──────────────────────────────┴─────────────────────────────┤
│ Agent Graph at selected seq                                  │
└────────────────────────────────────────────────────────────┘
```

### 13.2 过滤

- agentRole。
- messageType。
- event kind。
- status。
- evidenceRef。
- hypothesisId。
- actionProposalId。
- workflowRunId。
- hostLeaseId。
- traceId。
- auditRef。

### 13.3 快照

选择任意 seq 时，右侧显示当时的共享上下文快照：

- 已知证据。
- 活跃假设。
- 当前计划。
- 待审批动作。
- 活跃 HostLease。
- 已完成验证。
- 已生成候选经验。

若某个结论没有可回放来源，显示治理告警，并进入 audit review。

## 14. Agent Supervision

Agent Supervision 是管理员页面，不是普通运维入口。

### 14.1 页面目标

- 查看 Agent 角色是否启用。
- 查看默认模型、权限、工具、MCP、并发限制和降级策略。
- 查看最近失败、超时、权限裁剪和数据源不可用。
- 配置哪些 case 类型允许并行 Observer、并行 Diagnosis 或 Learning Agent。
- 配置 Execution Agent 必须绑定的 ActionToken、Workflow node 和 HostLease 约束说明。

### 14.2 配置项

| 配置 | 说明 |
| --- | --- |
| roleEnabled | 是否启用角色 |
| maxParallelTasks | 每 case 最大并发任务 |
| evidenceScope | 可读 evidence 类型 |
| toolScope | 可见工具和 MCP |
| actionScope | 是否允许生成动作草案或执行已批准 token |
| spawnPolicy | 自动、手动、按 case type |
| fallbackPolicy | 失败后转人工、单 Agent、只读模式 |
| promptTraceRequired | 是否强制生成 Prompt Trace |
| outputProtocolRequired | 是否强制 AgentMessage 类型化输出 |

Agent Supervision 的保存动作必须写入配置审计，并提示可能影响正在运行的 case。

## 15. 数据模型

前端 view-model 建议：

```text
MultiAgentCaseView
  caseId
  coordinationMode
  phase
  health
  sharedContext
  agents[]
  tasks[]
  messages[]
  conflicts[]
  decisions[]
  guardrails
  replayCursor

AgentRoleView
  role
  agentId
  name
  status
  currentTaskId
  lastMessageId
  permissions
  stats
  failures

AgentTaskView
  taskId
  agentRole
  taskType
  status
  inputRefs
  outputRefs
  dependencyTaskIds
  blocker
  promptTraceRef
  auditRef

AgentMessageView
  messageId
  messageType
  agentRole
  summary
  refs
  confidence
  requiresHumanDecision
  downstreamTarget
  createdAt

AgentConflictView
  conflictId
  type
  severity
  status
  claims
  supportingEvidenceRefs
  contradictingEvidenceRefs
  candidateResolutions
  decisionRef

AgentDecisionView
  decisionId
  decisionType
  requestedBy
  requiredRole
  options
  downstreamEffect
  status
```

页面内不得把这些 view-model 作为新事实源。它们只是后端 case、agent task、timeline、prompt trace 和 audit 的投影视图。

## 16. API 与事件

基于模块 API 草案扩展前端需要的读接口：

```text
GET    /api/v1/cases/{case_id}/agent-coordination
GET    /api/v1/cases/{case_id}/agent-messages
GET    /api/v1/cases/{case_id}/agent-tasks
GET    /api/v1/cases/{case_id}/agent-conflicts
GET    /api/v1/cases/{case_id}/agent-decisions
GET    /api/v1/cases/{case_id}/agent-replay?after_seq=...
POST   /api/v1/agents/tasks
GET    /api/v1/agents/tasks/{id}
POST   /api/v1/agents/tasks/{id}/messages
POST   /api/v1/cases/{case_id}/agent-coordination
POST   /api/v1/agent-decisions/{decision_id}/decision
```

实时事件建议复用统一 transport / projection：

```text
agent.task.created
agent.task.updated
agent.message.created
agent.conflict.created
agent.conflict.updated
agent.decision.requested
agent.decision.resolved
agent.guardrail.denied
agent.replay.seq
```

前端订阅只更新 view-model，不直接触发生产动作。用户决策和执行仍通过后端治理 API。

## 17. 权限与可见性

| 用户 | 可见内容 | 可操作内容 |
| --- | --- | --- |
| 普通业务用户 | 收敛摘要、当前状态、阻塞项、非敏感 Agent 输出 | 提供反馈、补充现象 |
| Case Owner | Agent 状态、任务、冲突、决策、Prompt Trace 摘要 | 请求补证、选择计划、暂停新任务 |
| Oncall / SRE | 证据、假设、计划、治理阻断、执行和验证详情 | 提交决策、创建动作草案、请求审批 |
| Reviewer | Prompt Trace 裁剪视图、audit、风险和发布影响 | 审查决策、审批治理路径 |
| Admin | Agent Supervision、角色策略、并发和降级策略 | 修改配置、暂停角色、导出审计 |

敏感信息裁剪规则：

- 未授权 evidence 只显示 `restricted evidence` 和 ref id。
- raw prompt 默认不展示。
- Tool input/output 默认只展示摘要、hash 和 rawRef。
- SecretRef、token、cookie、连接串、SQL 参数、用户敏感输入不得直接展示。
- Agent scratchpad 不作为事实展示，除非其中结论已经落到 AgentMessage 并绑定 refs。

## 18. 空态、异常与降级

空态：

- 当前 case 未启用多 Agent：显示单 Agent 模式和启用说明。
- 后端不支持 agent coordination API：显示只读降级，不影响 Case 工作台其他功能。
- 暂无 AgentMessage：显示等待 Observer 或 Commander 输出。
- 暂无 Prompt Trace：显示 trace 缺失告警和影响范围。

异常：

- Agent task failed：显示失败点、输入 refs、可 retry 条件。
- Projection stale：显示 lastSeq 和 lastUpdated，提供刷新。
- Conflict unresolved：在 case header 和 status rail 提示，禁止关闭 case。
- HostLease conflict：展示持有者、资源、过期时间和等待策略。
- Candidate leakage：若 Learning candidate 进入推荐路径，展示高优先级治理告警。

降级：

- Observer 不可用：只能基于已有证据诊断。
- Diagnosis 不可用：Commander 只能展示证据和建议补证。
- Runbook Agent 不可用：不能自动匹配 Runbook / Experience。
- Execution Agent 不可用：已批准动作仍可由 Runner 页面承接，但要显示交接状态。
- Verification Agent 不可用：case 不能自动 resolved。
- Learning Agent 不可用：不生成经验候选，不影响修复闭环。

## 19. 验收标准

- Case 工作台能展示当前参与 Agent、任务、最新输出、冲突和人工决策。
- 所有 Agent 输出都能追溯到 case timeline 或 Prompt Trace。
- `SharedCaseContext` 在页面上以引用和状态展示，不出现页面私有事实源。
- Execution Agent 无 ActionToken 时没有执行入口，并显示明确阻断原因。
- HostLease 冲突能在任务、冲突和执行治理视图中同时可见。
- 根因假设冲突能展示支持证据、反证、缺失证据和建议决策。
- 验证冲突存在时，关闭 case 或 resolved 动作被阻断。
- Learning Agent 输出只能进入 Experience candidate / review queue，不能显示为已发布经验。
- 普通用户视图不泄露 raw prompt、敏感 evidence、系统策略或 secret。
- Collaboration Replay 可以按 seq 回放 AgentMessage、任务状态和共享上下文变化。
