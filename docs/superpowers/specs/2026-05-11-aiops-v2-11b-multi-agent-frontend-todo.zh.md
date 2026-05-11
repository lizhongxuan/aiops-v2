# aiops-v2 Multi-Agent Collaboration 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-11a-multi-agent-frontend-design.zh.md](2026-05-11-aiops-v2-11a-multi-agent-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-11-multi-agent-module-design.zh.md](2026-05-11-aiops-v2-11-multi-agent-module-design.zh.md)

## 1. 目标

把 Multi-Agent Collaboration 落到 Case 工作台和专家协作控制台：用户能在同一个 case 中查看 Agent 角色、任务、输出、共享上下文、冲突、人工决策、执行治理、Prompt Trace 和协作回放；同时保证多 Agent 不产生第二套事实源，不绕过 RBAC、ActionToken、HostLease、Workflow governance、Prompt Trace 和 audit。

## 2. 实施顺序

```text
Multi-Agent API wrapper and types
  -> view-model and governance derivation
  -> Case Agent Panel
  -> Agent Roster and Task Board
  -> Agent Output Ledger and Prompt Trace drilldown
  -> Conflict Workbench
  -> Human Decision Queue
  -> Execution Guardrails
  -> Collaboration Replay
  -> Protocol Workspace and Prompt Trace integration
  -> Supervision page
  -> Tests and visual checks
```

MVP 先完成 `/incidents/:caseId?tab=agents` 内嵌面板，再拆出 `/multi-agent/cases/:caseId` 专家视图。不要先做独立 Agent 管理页，否则容易偏离 case 事实源。

## 3. 文件地图

新增：

- `web/src/api/multiAgents.ts`：Multi-Agent 协作 API wrapper。
- `web/src/api/multiAgents.test.ts`：API normalize、redaction、guardrail 和 conflict 测试。
- `web/src/pages/MultiAgentCasePage.tsx`：专家级单 case 协作控制台。
- `web/src/pages/MultiAgentConflictsPage.tsx`：跨 case 冲突队列。
- `web/src/pages/MultiAgentConflictDetailPage.tsx`：冲突处理工作台。
- `web/src/pages/MultiAgentReplayPage.tsx`：协作回放页。
- `web/src/pages/MultiAgentSupervisionPage.tsx`：管理员监督页。
- `web/src/components/agents/CaseAgentPanel.tsx`：Case 工作台内嵌多 Agent 面板。
- `web/src/components/agents/AgentCoordinationHeader.tsx`：协作 header。
- `web/src/components/agents/SharedContextStrip.tsx`：共享上下文引用条。
- `web/src/components/agents/AgentRoster.tsx`：Agent 角色列表。
- `web/src/components/agents/AgentTaskBoard.tsx`：任务看板。
- `web/src/components/agents/AgentTaskDrawer.tsx`：任务详情 drawer。
- `web/src/components/agents/AgentOutputLedger.tsx`：AgentMessage ledger。
- `web/src/components/agents/AgentMessageDrawer.tsx`：消息详情 drawer。
- `web/src/components/agents/AgentConflictRail.tsx`：Case 内冲突侧栏。
- `web/src/components/agents/AgentConflictWorkbench.tsx`：冲突处理面板。
- `web/src/components/agents/HumanDecisionQueue.tsx`：人工决策队列。
- `web/src/components/agents/AgentExecutionGuardrails.tsx`：执行治理边界。
- `web/src/components/agents/AgentPromptTraceLink.tsx`：Prompt Trace 跳转和权限摘要。
- `web/src/components/agents/CollaborationReplayTimeline.tsx`：回放时间线。
- `web/src/components/agents/CollaborationContextSnapshot.tsx`：回放上下文快照。
- `web/src/components/agents/AgentSupervisionTable.tsx`：角色监督表。
- `web/src/components/agents/multiAgentViewModels.ts`：Multi-Agent view-model 和派生逻辑。
- `web/src/components/agents/multiAgentViewModels.test.ts`：view-model 单测。
- `web/src/components/agents/agentComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/IncidentWorkbenchPage.tsx`：增加 Agents tab 或右侧协作面板入口。
- `web/src/pages/ProtocolWorkspacePage.tsx`：从 Main Agent Process 升级为可显示 active agents、task board、shared context summary。
- `web/src/pages/PromptTracePage.tsx`：支持从 agentRole、taskId、messageId、conflictId 进入并过滤。
- `web/src/pages/ApprovalManagementPage.tsx`：补充来源 AgentTask、decision 和 case coordination 链接。
- `web/src/pages/HostsPage.tsx`：HostLease 冲突入口跳转到 Agent Conflict。
- `web/src/pages/ExperiencePacksPage.tsx`：Learning Agent candidate 来源跳转到 case agent message。
- `web/src/router.tsx`：注册 `/multi-agent` 相关路由。
- `web/src/app/navigation.ts`：按产品阶段决定是否展示 Multi-Agent 专家入口。
- `web/src/pages/complexPagesApi.ts`：MVP 可临时接入，后续迁移到 `web/src/api/multiAgents.ts`。
- `web/src/pages/complexPages.test.tsx`：补充路由和页面渲染测试。

## 4. Task 1：补充 Multi-Agent API wrapper 与类型

- [ ] 新增 `web/src/api/multiAgents.ts`。
- [ ] 定义 `AgentRole`：`incident_commander`、`observer`、`diagnosis`、`runbook`、`execution`、`verification`、`learning`。
- [ ] 定义 `AgentTaskStatus`：`queued`、`running`、`waiting`、`blocked`、`completed`、`failed`、`canceled`。
- [ ] 定义 `AgentMessageType`：`evidence_summary`、`hypothesis_update`、`plan_proposal`、`action_proposal_draft`、`execution_status`、`verification_result`、`learning_candidate`。
- [ ] 定义 `AgentConflictType`：`hypothesis_conflict`、`plan_conflict`、`host_lease_conflict`、`experience_match_conflict`、`verification_conflict`。
- [ ] 定义 `MultiAgentCaseView`、`AgentRoleView`、`AgentTaskView`、`AgentMessageView`、`AgentConflictView`、`AgentDecisionView`、`AgentGuardrailView`、`CollaborationReplayEvent`。
- [ ] 实现 `getAgentCoordination(caseId)`，请求 `GET /api/v1/cases/{case_id}/agent-coordination`。
- [ ] 实现 `listAgentMessages(caseId)`，请求 `GET /api/v1/cases/{case_id}/agent-messages`。
- [ ] 实现 `listAgentTasks(caseId)`，请求 `GET /api/v1/cases/{case_id}/agent-tasks`。
- [ ] 实现 `listAgentConflicts(caseId)`，请求 `GET /api/v1/cases/{case_id}/agent-conflicts`。
- [ ] 实现 `listAgentDecisions(caseId)`，请求 `GET /api/v1/cases/{case_id}/agent-decisions`。
- [ ] 实现 `listAgentReplay(caseId, params)`，请求 `GET /api/v1/cases/{case_id}/agent-replay`。
- [ ] 实现 `createAgentTask(payload)`，请求 `POST /api/v1/agents/tasks`，只允许只读或治理后的任务类型。
- [ ] 实现 `submitAgentDecision(decisionId, payload)`，请求 `POST /api/v1/agent-decisions/{decision_id}/decision`。
- [ ] normalize 时对缺失字段补齐空数组、空对象和 `unknown` 状态，避免页面私自推断事实。
- [ ] normalize 时裁剪 raw prompt、SecretRef、token、cookie、连接串、SQL 参数、stdout/stderr 和未授权 evidence。
- [ ] 新增 `multiAgents.test.ts` 覆盖角色类型、消息类型、冲突类型、redaction、guardrail blocked 和 replay normalize。
- [ ] 运行 `npm --prefix web test -- multiAgents.test.ts`。

## 5. Task 2：实现 Multi-Agent view-model

- [ ] 新增 `web/src/components/agents/multiAgentViewModels.ts`。
- [ ] 实现 `buildMultiAgentCaseView(payload)`，将 coordination、tasks、messages、conflicts、decisions、guardrails 合并为只读投影。
- [ ] 实现 `summarizeSharedContext(sharedContext)`，输出 eventLog、evidence、hypotheses、actions、workflowRuns、verification、hostLeases、promptTrace、audit 的状态。
- [ ] 实现 `deriveCoordinationHealth(view)`，输出 fresh、stale、partial、degraded、unavailable。
- [ ] 实现 `groupTasksByRole(tasks)` 和 `groupTasksByStatus(tasks)`。
- [ ] 实现 `summarizeAgentRole(agent, tasks, messages)`，输出当前任务、最后输出、失败数、权限裁剪。
- [ ] 实现 `deriveConflictSeverity(conflict)`，按 verification_conflict、host_lease_conflict、plan_conflict、hypothesis_conflict、experience_match_conflict 排序。
- [ ] 实现 `buildDecisionOptions(decision)`，为每个选项附加证据、反证、风险、回滚和下游影响。
- [ ] 实现 `deriveExecutionGuardrail(task, guardrails)`，判断 no_token、approval_pending、rbac_denied、lease_conflict、policy_denied、workflow_dry_run_failed、trace_missing。
- [ ] 实现 `buildPromptTraceHref(ref)`，优先生成 `/ai-reasoning/prompt-traces/:traceId?...`，未启用时兼容 `/debug/prompts?traceId=...`。
- [ ] 实现 `buildReplaySnapshot(events, seq)`，按 seq 生成共享上下文快照摘要。
- [ ] 新增 `multiAgentViewModels.test.ts` 覆盖 shared context stale、task grouping、conflict severity、guardrail blocking、trace href、replay snapshot。
- [ ] 运行 `npm --prefix web test -- multiAgentViewModels.test.ts`。

## 6. Task 3：实现 Case Agent Panel

- [ ] 新增 `CaseAgentPanel.tsx`。
- [ ] 新增 `AgentCoordinationHeader.tsx`。
- [ ] 新增 `SharedContextStrip.tsx`。
- [ ] 在 `IncidentWorkbenchPage.tsx` 增加 Agents tab 或将 `CaseAgentPanel` 放入右侧上下文栏。
- [ ] Header 显示 coordination mode、phase、owner、active agents、running tasks、blocked tasks、conflicts、decisions、health。
- [ ] Shared Context Strip 显示 eventLog、evidenceRefs、hypotheses、actionProposals、workflowRuns、verificationRecords、hostLeases、promptTraceRefs、auditRefs。
- [ ] 任一 shared context 项点击后打开 drawer，并保留 caseId query。
- [ ] 未启用多 Agent 时显示单 Agent 模式，解释当前 case 仍使用同一 case timeline。
- [ ] 后端 API 不可用时显示 degraded read-only，不影响 Case 工作台其他 tab。
- [ ] 单测覆盖 enabled、disabled、degraded、stale projection、restricted evidence。
- [ ] 运行 `npm --prefix web test -- agentComponents.test.tsx`。

## 7. Task 4：实现 Agent Roster 和 Task Board

- [ ] 新增 `AgentRoster.tsx`。
- [ ] 新增 `AgentTaskBoard.tsx`。
- [ ] 新增 `AgentTaskDrawer.tsx`。
- [ ] Roster 展示 Incident Commander、Observer、Diagnosis、Runbook、Execution、Verification、Learning。
- [ ] 每个角色显示 status、current task、last output、permissions、trace count、failure count。
- [ ] Task Board 支持按状态列和角色泳道切换。
- [ ] 任务卡展示 taskType、inputRefs、outputRefs、dependencies、blocker、promptTraceRef、auditRef。
- [ ] 任务详情展示目标、输入引用、依赖状态、可见证据、可见工具、输出落点、失败点和 retry 条件。
- [ ] Execution task 无 ActionToken 时显示阻断原因，不显示执行按钮。
- [ ] Observer task 输出不能标记为最终根因。
- [ ] Learning task 输出只能标记为 candidate。
- [ ] 单测覆盖角色语义、任务状态、dependency、blocker、Execution token gate、Learning candidate gate。

## 8. Task 5：实现 Agent Output Ledger 与 Prompt Trace Drilldown

- [ ] 新增 `AgentOutputLedger.tsx`。
- [ ] 新增 `AgentMessageDrawer.tsx`。
- [ ] 新增 `AgentPromptTraceLink.tsx`。
- [ ] Ledger 支持 `evidence_summary`、`hypothesis_update`、`plan_proposal`、`action_proposal_draft`、`execution_status`、`verification_result`、`learning_candidate`。
- [ ] 列表展示 Time、Agent、Message、Confidence、Refs、Decision、Impact、Action。
- [ ] Drawer 展示 refs、downstream target、requiresHumanDecision、Prompt Trace、audit。
- [ ] Prompt Trace link 传递 agentRole、taskId、messageId、caseId。
- [ ] 普通用户只显示脱敏摘要和引用数。
- [ ] reviewer 可以展开治理详情和 prompt layer 摘要。
- [ ] raw trace 权限不足时显示 restricted 状态，不显示原文。
- [ ] 单测覆盖 message type rendering、restricted prompt trace、downstream routing、learning candidate isolation。

## 9. Task 6：实现 Conflict Workbench

- [ ] 新增 `AgentConflictRail.tsx`。
- [ ] 新增 `AgentConflictWorkbench.tsx`。
- [ ] 新增 `MultiAgentConflictsPage.tsx`。
- [ ] 新增 `MultiAgentConflictDetailPage.tsx`。
- [ ] Case 内 Rail 展示 unresolved conflicts、highest severity、blocked close reason。
- [ ] Workbench 支持 hypothesis、plan、host lease、experience match、verification 五类冲突。
- [ ] Evidence Matrix 展示 claim、supportingEvidenceRefs、contradictingEvidenceRefs、missingEvidenceRefs。
- [ ] Candidate Resolutions 支持 accept_hypothesis、request_more_evidence、select_plan、split_plan、block_execution、wait_for_host_lease、mark_experience_incompatible、require_verification_window、reopen_case、escalate_to_human_owner。
- [ ] 验证冲突存在时禁用 case resolved / close 入口，并说明原因。
- [ ] HostLease 冲突显示持有者、资源、过期时间和等待策略。
- [ ] 经验匹配冲突显示 matcher reason、disabled condition、fitness 和审核记录。
- [ ] 决策提交后刷新 case timeline、conflict status 和 AgentTask 状态。
- [ ] 单测覆盖五类冲突、决策 payload、case close blocker、HostLease hard reject。

## 10. Task 7：实现 Human Decision Queue

- [ ] 新增 `HumanDecisionQueue.tsx`。
- [ ] 在 Case Agent Panel 和 `/multi-agent/cases/:caseId` 中同时展示。
- [ ] 决策按 case severity、阻塞时长、风险、requiredRole 排序。
- [ ] 决策类型支持 choose_plan、request_evidence、approve_route、select_experience、verification_decision。
- [ ] 每个选项展示证据、反证、风险、回滚、验证要求和下游影响。
- [ ] 无权限用户只显示责任人和只读摘要。
- [ ] 高风险决策提交前弹出确认 Dialog。
- [ ] 决策提交使用 `submitAgentDecision(decisionId, payload)`。
- [ ] 决策完成后更新 AgentTask、AgentMessage、case timeline 和 audit refs。
- [ ] 单测覆盖权限隐藏、排序、高风险确认、decision API payload、刷新行为。

## 11. Task 8：实现 Execution Guardrails

- [ ] 新增 `AgentExecutionGuardrails.tsx`。
- [ ] 展示 user scope、agent scope、tool visibility、ActionToken、HostLease、Workflow node、Policy result。
- [ ] 对每个 Execution Agent task 计算 guardrail state。
- [ ] no_token、approval_pending、rbac_denied、lease_conflict、policy_denied、workflow_dry_run_failed、trace_missing 时隐藏执行入口。
- [ ] 每个阻断原因提供下一步链接：Approval、HostLease、Runner、Prompt Trace、Policy audit。
- [ ] Guardrails 组件可嵌入 ActionProposal、Workflow Run、Task Drawer 和 Conflict Workbench。
- [ ] 单测覆盖所有阻断原因、approved token、expired token、revoked token、lease conflict、dry-run failed。

## 12. Task 9：实现 Collaboration Replay

- [ ] 新增 `MultiAgentReplayPage.tsx`。
- [ ] 新增 `CollaborationReplayTimeline.tsx`。
- [ ] 新增 `CollaborationContextSnapshot.tsx`。
- [ ] Replay Header 支持 case、time range、seq、agentRole、messageType、event kind、status、traceId、auditRef 过滤。
- [ ] Timeline 展示 AgentTask、AgentMessage、approval、workflow、verification、conflict、decision、learning candidate。
- [ ] 选择任意 seq 时显示共享上下文快照。
- [ ] 快照展示 evidence、hypotheses、actions、leases、verification、prompt trace 和 learning candidates。
- [ ] 若某个结论缺少可回放来源，显示治理告警。
- [ ] 支持导出 replay refs，不导出敏感原文。
- [ ] 单测覆盖 seq snapshot、filter、missing source warning、restricted export。

## 13. Task 10：实现 Protocol Workspace 和 Chat 集成

- [ ] 修改 `ProtocolWorkspacePage.tsx`，将 Main Agent Process 扩展为 Agent Roster + Task Board 的轻量版。
- [ ] 在 Chat turn 的 process block 中识别 agentId、agentRole、taskId、messageId、promptTraceRef。
- [ ] Chat 中的 Agent 输出只显示摘要，详情跳到 Case Agent Panel 或 Prompt Trace。
- [ ] Pending Approval Rail 增加来源 AgentTask 和 case coordination 链接。
- [ ] Artifacts / Evidence 增加 AgentMessage 来源。
- [ ] RuntimeLiveness.activeAgents 显示在工作台 summary strip。
- [ ] 如果 Chat session 没有关联 case，只展示 session 级 active agents，不显示 SharedCaseContext。
- [ ] 单测覆盖 activeAgents、task link、prompt trace link、case missing fallback。

## 14. Task 11：实现 Agent Supervision

- [ ] 新增 `MultiAgentSupervisionPage.tsx`。
- [ ] 新增 `AgentSupervisionTable.tsx`。
- [ ] 展示 roleEnabled、maxParallelTasks、evidenceScope、toolScope、actionScope、spawnPolicy、fallbackPolicy、promptTraceRequired、outputProtocolRequired。
- [ ] 展示最近失败、超时、权限裁剪和数据源不可用。
- [ ] 保存配置前展示影响范围，说明会影响新任务分派和后续 case。
- [ ] 保存动作写入配置审计。
- [ ] 仅 admin 可见；非 admin 显示权限不足。
- [ ] 单测覆盖权限、配置编辑、保存 payload、审计提示、运行中 case warning。

## 15. Task 12：路由、导航与跨模块链接

- [ ] 在 `web/src/router.tsx` 注册 `/multi-agent`、`/multi-agent/cases/:caseId`、`/multi-agent/conflicts`、`/multi-agent/conflicts/:conflictId`、`/multi-agent/replay/:caseId`、`/multi-agent/supervision`。
- [ ] 在 `web/src/app/navigation.ts` 增加 Multi-Agent 专家入口，默认可先不设为主 nav。
- [ ] 在 `IncidentWorkbenchPage.tsx` 增加 Agents tab query 支持。
- [ ] 在 `PromptTracePage.tsx` 支持 agentRole、taskId、messageId、conflictId query 参数。
- [ ] 在 `ApprovalManagementPage.tsx` 增加来源 AgentTask 链接。
- [ ] 在 `HostsPage.tsx` 或 HostLease 页面增加冲突跳转。
- [ ] 在 `ExperiencePacksPage.tsx` 增加 Learning Agent candidate 来源链接。
- [ ] 路由测试覆盖每个新增路径和无权限状态。

## 16. Task 13：实时刷新与一致性

- [ ] 定义前端处理的事件名：agent.task.created、agent.task.updated、agent.message.created、agent.conflict.created、agent.conflict.updated、agent.decision.requested、agent.decision.resolved、agent.guardrail.denied、agent.replay.seq。
- [ ] 优先复用统一 AssistantTransport / AgentEventProjection；没有实时通道时使用轮询。
- [ ] 轮询间隔按 case 状态调整：active case 3-5 秒，resolved case 30 秒。
- [ ] 使用 lastSeq 避免重复渲染。
- [ ] projection stale 时显示 lastSeq、lastUpdated 和刷新按钮。
- [ ] 事件更新只刷新 view-model，不在前端直接触发生产动作。
- [ ] 单测覆盖 event merge、duplicate seq、stale projection、polling fallback。

## 17. Task 14：测试与视觉验收

- [ ] 单测覆盖 API normalize、view-model、Case Agent Panel、Task Board、Ledger、Conflict、Decision、Guardrails、Replay。
- [ ] 路由测试覆盖 `/incidents/:caseId?tab=agents` 和 `/multi-agent/*`。
- [ ] 权限测试覆盖普通用户、case owner、reviewer、admin。
- [ ] 敏感信息测试覆盖 raw prompt、SecretRef、token、cookie、连接串、stdout/stderr、restricted evidence。
- [ ] 视觉检查桌面宽度：1440px、1280px。
- [ ] 视觉检查移动宽度：390px、430px。
- [ ] 检查长 case title、长 Agent message、长 blocker reason 不溢出。
- [ ] 检查任务状态、冲突 badge、决策按钮、Prompt Trace 链接在深色和浅色状态下可读。
- [ ] 运行 `npm --prefix web test -- multiAgents.test.ts multiAgentViewModels.test.ts agentComponents.test.tsx complexPages.test.tsx`。
- [ ] 如果页面已接入真实路由，使用浏览器打开 `/incidents/:caseId?tab=agents` 和 `/multi-agent/cases/:caseId` 做截图验收。

## 18. 交付门禁

- [ ] 多 Agent 页面只读取 case coordination、agent tasks、agent messages、prompt trace、audit 和 shared context 投影。
- [ ] 没有任何页面创建独立于 case 的私有事实源。
- [ ] Execution Agent 无 ActionToken 时没有执行入口。
- [ ] HostLease 冲突在 Task Board、Conflict Workbench 和 Guardrails 中一致展示。
- [ ] Verification conflict 存在时 case close / resolved 被阻断。
- [ ] Learning Agent 输出只能进入 candidate / review queue。
- [ ] Prompt Trace drilldown 对普通用户裁剪 raw prompt 和敏感 evidence。
- [ ] Agent 失败、降级和不可用状态可见。
- [ ] 所有人工决策写入 audit，并回写 case timeline。
- [ ] 新增路由、组件、API wrapper 和 view-model 均有测试覆盖。
