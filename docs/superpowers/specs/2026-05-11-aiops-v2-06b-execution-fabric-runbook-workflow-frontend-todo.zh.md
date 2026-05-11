# aiops-v2 Execution Fabric, Runbook & Workflow 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-06a-execution-fabric-runbook-workflow-frontend-design.zh.md](2026-05-11-aiops-v2-06a-execution-fabric-runbook-workflow-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-06-execution-fabric-runbook-workflow-module-design.zh.md](2026-05-11-aiops-v2-06-execution-fabric-runbook-workflow-module-design.zh.md)

## 1. 目标

把现有 Runbook 页面和 Runner Studio 升级为 Execution Fabric 前端控制台：支持 Tool Catalog、Runbook lifecycle、Runbook Instance、Workflow lifecycle、Start/Fanout/End 企业级配置、Workflow Run Detail、ActionToken / HostLease / EvidenceRef 治理展示，以及 Case、AI Reasoning、Governed Action、Verification、Experience 的跨页面集成。

## 2. 实施顺序

```text
Execution Fabric API view-model
  -> Tool Catalog
  -> Runbook Catalog / Detail / Instance
  -> Runner Studio API 迁移
  -> Workflow lifecycle gates
  -> Start target selector / fanout / lock policy
  -> Node governance / ActionToken / HostLease
  -> End outputs / callbacks / verification
  -> Workflow Run Detail
  -> AI draft 和跨页面集成
  -> 测试和视觉校验
```

先做统一 API 和治理 view-model，再做画布细节增强。不要先扩展裸 `fetch` 或把 Runbook 做成直接执行工具的入口。

## 3. 文件地图

新增：

- `web/src/api/executionFabric.ts`：ToolDefinition、Runbook、RunbookInstance、Workflow、WorkflowRun、ActionToken、HostLease、EvidenceRef API client 和类型。
- `web/src/api/executionFabric.test.ts`：Execution Fabric normalize 和兼容字段测试。
- `web/src/api/runbooks.ts`：迁移 `web/src/api/runbooks.js`，补充 create/review/publish/instance。
- `web/src/pages/ExecutionOverviewPage.tsx`：Execution Fabric 总览。
- `web/src/pages/ToolCatalogPage.tsx`：Tool Catalog。
- `web/src/pages/RunbookInstancePage.tsx`：Runbook instance 工作台。
- `web/src/pages/WorkflowRunDetailPage.tsx`：Workflow run 独立详情页。
- `web/src/components/execution/ExecutionGovernanceStrip.tsx`：ActionToken、HostLease、Approval、Tool health 状态。
- `web/src/components/execution/ExecutionSummaryStrip.tsx`：运行中、失败、待审批、回调重试指标。
- `web/src/components/execution/ToolCatalogTable.tsx`：工具目录表。
- `web/src/components/execution/ToolDefinitionDrawer.tsx`：工具详情。
- `web/src/components/execution/RunbookFilterBar.tsx`：Runbook 筛选。
- `web/src/components/execution/RunbookTable.tsx`：Runbook 表格。
- `web/src/components/execution/RunbookHeader.tsx`：Runbook 详情 header。
- `web/src/components/execution/RunbookStepEditor.tsx`：Runbook step 编辑。
- `web/src/components/execution/RunbookMatchPanel.tsx`：匹配测试。
- `web/src/components/execution/RunbookActionTemplatesPanel.tsx`：动作提案模板。
- `web/src/components/execution/RunbookInstanceRail.tsx`：Runbook instance step rail。
- `web/src/components/execution/RunbookInstanceCurrentStep.tsx`：当前步骤。
- `web/src/components/execution/WorkflowStartPanel.tsx`：Start targetSelector、variables、lockPolicy、fanout。
- `web/src/components/execution/WorkflowNodeGovernancePanel.tsx`：节点风险、权限、ActionToken、HostLease。
- `web/src/components/execution/WorkflowEndPanel.tsx`：outputs、callbacks、verification。
- `web/src/components/execution/WorkflowLifecycleGatePanel.tsx`：validate、dry-run、risk review、publish gate。
- `web/src/components/execution/WorkflowRunHeader.tsx`：run header。
- `web/src/components/execution/WorkflowRunGraphPanel.tsx`：运行态 graph。
- `web/src/components/execution/WorkflowRunEventsPanel.tsx`：RunEvent 时间线。
- `web/src/components/execution/WorkflowRunHostsPanel.tsx`：每台主机结果。
- `web/src/components/execution/WorkflowRunVariablesPanel.tsx`：inputs、outputs、exports、nodeResults。
- `web/src/components/execution/WorkflowRunApprovalsPanel.tsx`：workflow approval、ActionProposal、ActionToken。
- `web/src/components/execution/WorkflowRunCallbacksPanel.tsx`：回调状态和 retry。
- `web/src/components/execution/WorkflowRunAuditPanel.tsx`：operator、policy、normalizedInputHash、PromptTrace。
- `web/src/components/execution/executionViewModels.ts`：排序、生命周期门禁、风险摘要、目标解析摘要、运行状态汇总。
- `web/src/components/execution/executionViewModels.test.ts`：view-model 单测。
- `web/src/components/execution/executionComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/RunbookCatalogPage.tsx`：从 `complexPagesApi.ts` 迁移到 `runbooks.ts`，拆出表格和筛选组件。
- `web/src/pages/RunbookDetailPage.tsx`：补齐 lifecycle、steps、match、action templates、verification、rollback、instances、lineage。
- `web/src/pages/RunnerStudioPage.tsx`：去掉页面内 `requestJson`，使用 `runnerStudioClient` 或 `executionFabric.ts`；接入 Start/Node/End 治理面板。
- `web/src/api/runnerStudioClient.js`：必要时补充 canonical wrapper 或保持作为兼容 client。
- `web/src/components/runner/RunnerCanvas.tsx`：展示 ActionToken、HostLease、fanout、callback 状态。
- `web/src/components/runner/runStateReducer.js`：补齐 ActionToken、HostLease、callback_retry、agent_unavailable 事件聚合。
- `web/src/components/runner/runnerVariables.js`：补齐 End outputs 和 callback output 的变量展示。
- `web/src/pages/IncidentWorkbenchPage.tsx`：嵌入 matched Runbooks、Runbook instances、Workflow runs。
- `web/src/pages/ApprovalManagementPage.tsx`：展示 source Runbook step / Workflow node / run id。
- `web/src/pages/AIReasoningPage.tsx`：ReasoningOutput 的 runbook_draft / workflow_draft 跳转到执行面。
- `web/src/app/navigation.ts`：增加 `/execution`、`/execution/tools`、`/execution/runs`、`/execution/runs/:runId`、`/runbook-instances/:instanceId`。
- `web/src/router.tsx`：注册新页面。
- `web/src/pages/complexPages.test.tsx`：补充 Execution、Tool Catalog、Runbook Instance、Workflow Run 路由测试。

## 4. Task 1：建立 Execution Fabric API 与类型

- [ ] 新增 `web/src/api/executionFabric.ts`。
- [ ] 定义 `ToolDefinitionView`、`RunbookView`、`RunbookStepView`、`RunbookInstanceView`、`WorkflowView`、`WorkflowStartView`、`WorkflowEndView`、`WorkflowRunView`、`TargetSelectorView`、`TargetResolutionView`、`ActionTokenRefView`、`HostLeaseRefView`、`CallbackResultView`。
- [ ] 实现 `listTools(params)`，请求 `GET /api/v1/tools`，兼容 `/api/runner-studio/actions`。
- [ ] 实现 `listRunbooks(params)`、`getRunbook(id)`、`createRunbook(payload)`、`reviewRunbook(id, payload)`、`publishRunbook(id, payload)`。
- [ ] 实现 `createRunbookInstance(payload)`、`getRunbookInstance(id)`、`advanceRunbookInstance(id, payload)`。
- [ ] 实现 `listWorkflows(params)`、`getWorkflow(id)`、`createWorkflow(payload)`。
- [ ] 实现 `validateWorkflow(id, payload)`、`dryRunWorkflow(id, payload)`、`publishWorkflow(id, payload)`、`createWorkflowRun(id, payload)`。
- [ ] 实现 `getWorkflowRun(runId)`、`getWorkflowRunEvents(runId)`、`retryWorkflowRunCallback(runId, callbackId)`、`cancelWorkflowRun(runId)`。
- [ ] normalize 时补齐 status、risk、lineage、targetSummary、evidenceRefs、actionTokens、hostLeases、callbackResults 默认值。
- [ ] 新增 `executionFabric.test.ts`，覆盖新 API 字段和当前 runner-studio 兼容字段。
- [ ] 运行 `npm --prefix web test -- executionFabric.test.ts`。

## 5. Task 2：实现 Tool Catalog

- [ ] 新增 `ToolCatalogPage.tsx`。
- [ ] 新增 `ToolCatalogTable.tsx`。
- [ ] 新增 `ToolDefinitionDrawer.tsx`。
- [ ] 表格列包括 Tool、Risk、Permissions、Schema、Runtime、Governance、Usage、Action。
- [ ] 详情展示 inputSchema、outputSchema、risk、requiredPermissions、requiredCapabilities、idempotency、timeout、redactionRules、allowlist、最近调用和失败。
- [ ] 高风险和破坏性工具显示不能直接运行的原因和 ActionProposal 入口。
- [ ] 修改 `navigation.ts` 和 `router.tsx` 注册 `/execution/tools`。
- [ ] 单测覆盖 readonly、high_risk、destructive、not allowlisted 展示。

## 6. Task 3：实现 Execution Overview

- [ ] 新增 `ExecutionOverviewPage.tsx`。
- [ ] 新增 `ExecutionGovernanceStrip.tsx`。
- [ ] 新增 `ExecutionSummaryStrip.tsx`。
- [ ] 页面展示 running、failed、awaiting approval、callback retry、lock conflict、agent unavailable。
- [ ] 列表按风险和阻塞排序，列包括 Item、Source、Status、Governance、Target、Evidence、Next。
- [ ] 支持跳转 Workflow Run、Runbook Instance、Approval、Case、Evidence。
- [ ] 注册 `/execution` 和 `/execution/runs`。
- [ ] 单测覆盖失败 run、待审批、锁冲突和 callback retry。

## 7. Task 4：升级 Runbook Catalog

- [ ] 新增 `web/src/api/runbooks.ts`，迁移 `web/src/api/runbooks.js`。
- [ ] 新增 `RunbookFilterBar.tsx`。
- [ ] 新增 `RunbookTable.tsx`。
- [ ] 修改 `RunbookCatalogPage.tsx`，从 `complexPagesApi.ts` 切到 `runbooks.ts`。
- [ ] 支持 status、risk、environment、entityTypes、problemSignatures、owner、capability、disabledConditions、updatedAt 筛选。
- [ ] 表格列包括 Runbook、Status、Match、Risk、Verification、Instances、Lineage、Action。
- [ ] 增加手动创建、从 Case 提炼、从 AI draft 创建、从 Experience Pack 创建入口。
- [ ] 单测覆盖筛选、创建入口、AI draft 显示为 draft。

## 8. Task 5：升级 Runbook Detail

- [ ] 新增 `RunbookHeader.tsx`。
- [ ] 新增 `RunbookStepEditor.tsx`。
- [ ] 新增 `RunbookMatchPanel.tsx`。
- [ ] 新增 `RunbookActionTemplatesPanel.tsx`。
- [ ] 修改 `RunbookDetailPage.tsx`，Tabs 包含 Overview、Steps、Match、Action Templates、Verification、Rollback、Instances、Lineage。
- [ ] Steps 支持 observe、decide、propose_action、verify、rollback。
- [ ] `observe` 只允许只读证据请求。
- [ ] `propose_action` 只生成 ActionProposal，不直接执行工具。
- [ ] `verify` 必须引用 VerificationSpec 或 EvidenceRequirement。
- [ ] `rollback` 必须说明触发条件和回滚验证。
- [ ] 单测覆盖 step 类型、动作提案模板、禁止直接执行。

## 9. Task 6：实现 Runbook Instance 工作台

- [ ] 新增 `RunbookInstancePage.tsx`。
- [ ] 新增 `RunbookInstanceRail.tsx`。
- [ ] 新增 `RunbookInstanceCurrentStep.tsx`。
- [ ] Header 展示 instance、case、runbook、current step、status。
- [ ] Step Rail 展示 observe -> decide -> propose_action -> verify -> rollback。
- [ ] 当前步骤展示 instructions、evidence requirements、observations、decision options、generated ActionProposal refs、verification refs、blocking items。
- [ ] 支持记录观察、请求更多证据、生成 ActionProposal、打开审批、执行下一步、标记验证通过或失败。
- [ ] 禁止直接执行写工具、跳过 required evidence、绕过审批和 HostLease。
- [ ] 注册 `/runbook-instances/:instanceId`。
- [ ] 单测覆盖步骤推进、ActionProposal 生成入口、阻断直接执行。

## 10. Task 7：迁移 Runner Studio API 调用

- [ ] 修改 `RunnerStudioPage.tsx`，移除页面内 `requestJson`。
- [ ] 使用 `runnerStudioClient` 或 `executionFabric.ts` 的 wrapper 调用 workflows、graph、validate、dry-run、publish、runs、run history、actions、AI draft。
- [ ] 保留当前本地 draft fallback 行为。
- [ ] 服务器不可用时禁用 validate、dry-run、publish、run，但允许本地编辑和保存草稿。
- [ ] 扩展 `runnerStudioClient.test.js` 覆盖新增 wrapper。
- [ ] 单测覆盖 Runner API 不可用时不提交生产运行。

## 11. Task 8：实现 Workflow lifecycle gates

- [ ] 新增 `WorkflowLifecycleGatePanel.tsx`。
- [ ] 在 Runner Studio 显示 validate、dry-run、risk review、publish gate。
- [ ] validate 问题定位到节点、边和字段。
- [ ] dry-run 展示 target resolution、host lease precheck、ActionToken requirement、variable resolution、graph path、fanout batches、callbacks preview。
- [ ] risk review 展示 high risk/destructive nodes、mutating tools、missing rollback、missing verification、disabled conditions、impacted services、required approvals。
- [ ] publish modal 必须展示 validate status、dry-run status、risk summary、semantic diff、AI generated draft、reviewer note、publish blocker。
- [ ] 未通过 validate、dry-run 或 risk review 时禁用 publish。
- [ ] 单测覆盖 publish blocker。

## 12. Task 9：实现 Start target selector / fanout / lock policy

- [ ] 新增 `WorkflowStartPanel.tsx`。
- [ ] Start 支持 targetSelector.labels、explicitHosts、variables、lockPolicy.mode、lockPolicy.acquire。
- [ ] Fanout 支持 sequential、parallel、batch、batchSize、failureThreshold。
- [ ] Dry-run 后展示解析主机、标签来源、权限检查、HostLease 预检查、fanout 批次、失败阈值策略。
- [ ] 与现有 StartHostGroupsEditor 兼容，逐步迁移 `ui.host_groups` 到 `start.targetSelector` view-model。
- [ ] 单测覆盖 label selector、explicit hosts、batch fanout、failure threshold。

## 13. Task 10：实现节点治理面板

- [ ] 新增 `WorkflowNodeGovernancePanel.tsx`。
- [ ] 所有执行节点展示 action、target labels、explicit hosts、input variables、output schema、risk、requiredPermissions、requiredCapabilities、timeout、retry safety、idempotency。
- [ ] 展示 approval requirement、ActionProposal、ActionToken、HostLease 状态。
- [ ] 高风险工具节点不能直接运行，必须显示待创建或已绑定 ActionProposal。
- [ ] Condition 节点展示 expression、引用变量、IF/ELSE 输出、当前运行匹配分支、变量缺失警告。
- [ ] Approval 节点展示 approvers、timeout、risk reason、approved/rejected 分支、approval id、reviewer、decision、comment、evidence refs。
- [ ] 单测覆盖高风险节点阻断、变量缺失、approval 分支。

## 14. Task 11：实现 End outputs / callbacks / verification

- [ ] 新增 `WorkflowEndPanel.tsx`。
- [ ] End 支持 outputs：key、source、type、redaction。
- [ ] End 支持 callbacks：incident.update、erp.update、webhook、experience.candidate。
- [ ] End 支持 verification 和 callback retry policy。
- [ ] 运行完成后展示输出变量、验证结果、回调成功/retry/failed、失败回调下一步。
- [ ] 与现有 EndCallbacksEditor 兼容，逐步迁移 `ui.callbacks` 到 `end.callbacks` view-model。
- [ ] 单测覆盖 output source、callback retry、verification 失败。

## 15. Task 12：实现 Workflow Run Detail

- [ ] 新增 `WorkflowRunDetailPage.tsx`。
- [ ] 新增 `WorkflowRunHeader.tsx`。
- [ ] 新增 `WorkflowRunGraphPanel.tsx`。
- [ ] 新增 `WorkflowRunEventsPanel.tsx`。
- [ ] 新增 `WorkflowRunHostsPanel.tsx`。
- [ ] 新增 `WorkflowRunVariablesPanel.tsx`。
- [ ] 新增 `WorkflowRunApprovalsPanel.tsx`。
- [ ] 新增 `WorkflowRunCallbacksPanel.tsx`。
- [ ] 新增 `WorkflowRunAuditPanel.tsx`。
- [ ] Tabs 包含 Graph、Events、Hosts、Variables、Approvals、Tool Results、Verification、Callbacks、Audit。
- [ ] Header 展示 run id、workflow name/version、status、source、triggeredBy、caseId、startedAt、finishedAt、risk、target summary。
- [ ] 支持 cancel、查看 case、查看 EvidenceRef、重试失败回调、生成复盘/经验候选。
- [ ] 注册 `/execution/runs/:runId` 和 `/workflows/:workflowName/runs/:runId`。
- [ ] 单测覆盖 agent_unavailable、callback_retry、host result、variables、audit。

## 16. Task 13：AI draft 和跨页面集成

- [ ] Runner AI draft 展示来源 case、PromptTrace、Observability evidence、OpsGraph path、ExperiencePack、user instruction。
- [ ] AI draft 结果展示 summary、graph diff 或 runbook step diff、evidence refs、tool refs、risk summary、disabled conditions、required validation、PromptTrace。
- [ ] AI draft 只能应用为 draft，不能直接 publish 或 run。
- [ ] `IncidentWorkbenchPage.tsx` 嵌入 matched Runbooks、active Runbook instances、recommended Workflow drafts、active Workflow runs。
- [ ] `ApprovalManagementPage.tsx` 展示 source Runbook step、source Workflow node、run id、normalizedInputHash。
- [ ] `AIReasoningPage.tsx` 的 runbook_draft / workflow_draft 跳转到执行面。
- [ ] Verification 页面后续复用 Workflow End verification 和 Runbook verification。
- [ ] Experience Pack 审核页后续支持从成功 run 生成经验候选。
- [ ] 单测覆盖 Case、Approval、AI Reasoning 跳转。

## 17. Task 14：运行状态和事件聚合增强

- [ ] 扩展 `runStateReducer.js` 支持 ActionToken、HostLease、callback_retry、agent_unavailable。
- [ ] 扩展 `runEventHistory.js` 映射 node、host、callback、approval、token、lease 事件。
- [ ] 扩展 `runnerRunVisualState.js` 在画布节点展示 blocked、awaiting_approval、callback_retry、agent_unavailable。
- [ ] 扩展 `runnerVariables.js` 收集 End outputs、callback outputs、verification outputs。
- [ ] 单测覆盖新事件和变量来源。

## 18. Task 15：测试与视觉检查

- [ ] 新增 `executionComponents.test.tsx`，覆盖 ExecutionOverview、ToolCatalog、RunbookDetail、RunbookInstance、WorkflowRunDetail、WorkflowLifecycleGatePanel。
- [ ] 扩展 `complexPages.test.tsx`，覆盖 `/execution`、`/execution/tools`、`/runbook-instances/:id`、`/execution/runs/:runId`。
- [ ] 扩展 `runner-studio.spec.js`，覆盖 Start fanout、End callbacks、publish gate、run detail。
- [ ] 运行 `npm --prefix web test -- executionFabric.test.ts executionViewModels.test.ts executionComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含页面视觉变更，启动 dev server 并用浏览器截图检查 `/execution`、`/execution/tools`、`/runbooks`、`/runner`、`/execution/runs/:runId` 的桌面和移动宽度。
- [ ] 检查 `/runner/:workflowName`、`/execution/runs/:runId`、`/runbook-instances/:instanceId` 能正确落点。

## 19. 交付检查

- [ ] `RunnerStudioPage.tsx` 不直接调用裸 `fetch`。
- [ ] `RunbookCatalogPage.tsx` 和 `RunbookDetailPage.tsx` 不再依赖 `complexPagesApi.ts` 扩展新能力。
- [ ] Runbook step 只能生成 ActionProposal，不能直接执行生产写动作。
- [ ] Tool Catalog 能展示 risk、permissions、schema、idempotency、timeout、redactionRules 和 allowlist。
- [ ] Workflow Start 能定义 targetSelector、labels、explicitHosts、variables、lockPolicy、fanout、batchSize、failureThreshold。
- [ ] Workflow 节点能展示 target、risk、permissions、approval、ActionToken、HostLease。
- [ ] Workflow End 能展示 outputs、callbacks、verification、callback retry。
- [ ] Validate、dry-run、risk review、publish gate 可见且能阻断发布。
- [ ] Workflow Run Detail 能展示 graph、events、hosts、variables、approvals、tool results、verification、callbacks、audit。
- [ ] 多主机 fanout 每个目标都有独立结果。
- [ ] AI generated Runbook/Workflow 只能进入 draft，不能直接发布或运行。
- [ ] SecretRef、token、password、cookie、authorization header、私钥、完整连接串不会出现在 DOM。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
