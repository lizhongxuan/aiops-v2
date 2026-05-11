# aiops-v2 Incident Control Plane 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-01a-incident-control-plane-frontend-design.zh.md](2026-05-11-aiops-v2-01a-incident-control-plane-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-01-incident-control-plane-module-design.zh.md](2026-05-11-aiops-v2-01-incident-control-plane-module-design.zh.md)

## 1. 目标

把现有 `IncidentListPage.tsx` 和 `IncidentWorkbenchPage.tsx` 从临时事故面板升级为 Incident Control Plane 前端工作台：支持 case 队列、case 详情、证据、假设、动作、审批、验证、时间线、资产沉淀和安全裁剪。

## 2. 实施顺序

```text
API view-model
  -> 列表页
  -> 详情页结构
  -> 证据和时间线
  -> 动作和审批
  -> 验证和关闭
  -> 资产沉淀入口
  -> 测试和视觉校验
```

先做只读 case 工作台，再接入审批、关闭和资产沉淀。不要先做复杂图谱可视化或多 Agent 面板。

## 3. 文件地图

新增：

- `web/src/api/cases.ts`：Case API client、类型、兼容 `/api/v1/incidents` 的 view-model adapter。
- `web/src/api/cases.test.ts`：Case API 和 normalize 测试。
- `web/src/components/incidents/CaseStatusRail.tsx`：状态轨道。
- `web/src/components/incidents/CaseHeader.tsx`：Case header 和主操作按钮。
- `web/src/components/incidents/CaseSummaryStrip.tsx`：列表页指标条。
- `web/src/components/incidents/CaseFilterBar.tsx`：列表筛选。
- `web/src/components/incidents/CaseTable.tsx`：Case 队列表格。
- `web/src/components/incidents/CaseCreateSheet.tsx`：新建 Case 表单。
- `web/src/components/incidents/CaseOverviewPanel.tsx`：Overview tab。
- `web/src/components/incidents/CaseEvidencePanel.tsx`：Evidence tab。
- `web/src/components/incidents/CaseActionsPanel.tsx`：Actions tab。
- `web/src/components/incidents/CaseVerificationPanel.tsx`：Verification tab。
- `web/src/components/incidents/CaseTimelinePanel.tsx`：Timeline tab。
- `web/src/components/incidents/CaseAssetsPanel.tsx`：Assets tab。
- `web/src/components/incidents/CaseContextRail.tsx`：右侧上下文栏。
- `web/src/components/incidents/CaseCloseDialog.tsx`：关闭确认。
- `web/src/components/incidents/incidentViewModels.ts`：前端派生状态、排序、分组、文案。
- `web/src/components/incidents/incidentViewModels.test.ts`：view-model 单测。
- `web/src/components/incidents/incidentComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/IncidentListPage.tsx`：改为消费 `cases.ts` 和拆分组件。
- `web/src/pages/IncidentWorkbenchPage.tsx`：改为稳定工作台布局和 tabs。
- `web/src/pages/complexPagesApi.ts`：移除或保留兼容导出，避免新页面继续扩展这个聚合文件。
- `web/src/pages/complexPages.test.tsx`：补充 case 列表、详情、审批、关闭、权限裁剪测试。
- `web/src/app/navigation.ts`：确认 `/incidents` 文案从 Incidents 调整为 Cases 或 事故工作台。
- `web/src/router.tsx`：保持路由不变，必要时补 404 case 状态。

## 4. Task 1：建立 Case API 与类型

- [ ] 新增 `web/src/api/cases.ts`。
- [ ] 定义 `CaseType`、`CaseStatus`、`CaseView`、`EvidenceView`、`HypothesisView`、`ActionProposalView`、`VerificationView`、`TimelineEventView`、`CaseAssetView`。
- [ ] 实现 `listCases(params)`，优先请求 `/api/v1/cases`。
- [ ] 实现 `getCase(caseId)`，优先请求 `/api/v1/cases/{caseId}`。
- [ ] 实现 `createCase(payload)`、`closeCase(caseId, payload)`、`mergeCase(caseId, payload)`、`relateCase(caseId, payload)`。
- [ ] 实现兼容 adapter：当后端仍返回旧 `IncidentRecord` 字段时，统一转成 `CaseView`。
- [ ] 新增 `web/src/api/cases.test.ts`，覆盖旧字段和新字段都能 normalize。
- [ ] 运行 `npm --prefix web test -- cases.test.ts`。

## 5. Task 2：实现列表页筛选和队列

- [ ] 新增 `CaseSummaryStrip.tsx`，展示活跃数、SEV1、待审批、执行中、验证中。
- [ ] 新增 `CaseFilterBar.tsx`，支持状态、类型、SEV、来源、环境、业务能力筛选。
- [ ] 新增 `CaseTable.tsx`，表格列包括 Case、类型、状态、SEV、来源、业务影响、技术影响、阻塞项、更新时间。
- [ ] 修改 `IncidentListPage.tsx`，从 `listIncidents` 切换到 `listCases`。
- [ ] 筛选条件写入 URL query，刷新后保持。
- [ ] 空态只显示短文案和“新建 Case”按钮。
- [ ] 单测覆盖筛选参数传递、行点击链接、空态。
- [ ] 运行 `npm --prefix web test -- complexPages.test.tsx incidentComponents.test.tsx`。

## 6. Task 3：实现新建 Case Sheet

- [ ] 新增 `CaseCreateSheet.tsx`。
- [ ] 字段包括 type、title、environment、severity、businessCapability、affectedService、affectedMiddleware、summary。
- [ ] 表单提交调用 `createCase`。
- [ ] 创建成功后跳转 `/incidents/:caseId`。
- [ ] 创建失败时在 Sheet 内展示错误，不清空用户输入。
- [ ] 单测覆盖成功跳转和失败保留输入。

## 7. Task 4：重构工作台骨架

- [ ] 新增 `CaseHeader.tsx`，显示标题、状态、类型、SEV、环境、owner、更新时间和主操作。
- [ ] 新增 `CaseStatusRail.tsx`，展示 case 状态轨道和失败分支。
- [ ] 修改 `IncidentWorkbenchPage.tsx`，布局改成 header、status rail、主 tabs、右侧 context rail。
- [ ] 使用现有 `Tabs` 组件实现 Overview、Evidence、Actions、Verification、Timeline、Assets。
- [ ] 详情页 loading 时保持 header skeleton 和内容 skeleton。
- [ ] Case 不存在时显示 404 空态。
- [ ] 单测覆盖每个 tab 标签和关键状态渲染。

## 8. Task 5：Overview 与右侧上下文栏

- [ ] 新增 `CaseOverviewPanel.tsx`。
- [ ] Overview 展示当前摘要、Top Hypotheses、Recommended Next Actions、Blocking Items、Latest Timeline。
- [ ] 新增 `CaseContextRail.tsx`。
- [ ] Context rail 展示 Impact、OpsGraph、Coroot、Trace/Debug、HostLease。
- [ ] DebugCase 只展示 trace id、route、action、slow span、evidence quality，不展示敏感原文。
- [ ] 单测覆盖 DebugCase 敏感字段不会出现在 DOM。

## 9. Task 6：Evidence 与 Timeline

- [ ] 新增 `CaseEvidencePanel.tsx`。
- [ ] 按来源分组展示 Coroot、ERP、Browser Plugin Debug、Trace、Tool、Workflow、Middleware、Change、User。
- [ ] 显示 confidence、redactionStatus、observedAt、rawRef。
- [ ] 权限不足或脱敏失败的证据只展示受限状态和 evidence id。
- [ ] 新增 `CaseTimelinePanel.tsx`。
- [ ] Timeline 支持按事件类型、actor、来源过滤。
- [ ] Timeline 只读，不提供编辑入口。
- [ ] 单测覆盖 evidence 分组、受限证据裁剪和 timeline 排序。

## 10. Task 7：Actions 与 Approval

- [ ] 新增 `CaseActionsPanel.tsx`。
- [ ] 展示 ActionProposal、pending approval、ActionToken、HostLease、Workflow run、已完成和失败动作。
- [ ] ActionProposal 展开后显示 toolName、normalizedInputHash、risk、expectedEffect、rollback、verificationSpec。
- [ ] 待审批动作复用 `submitApprovalDecision` 或迁移到统一 approval API client。
- [ ] 审批成功后刷新 case。
- [ ] 审批失败时保留当前 case 数据并显示错误。
- [ ] 单测覆盖 approve/reject 调用 `/api/v1/approvals/{id}/decision`。

## 11. Task 8：Verification 与关闭 Case

- [ ] 新增 `CaseVerificationPanel.tsx`。
- [ ] 展示 VerificationSpec、VerificationRecord、Coroot SLO、ERP 业务恢复、Debug Trace 对比、中间件健康、人工确认。
- [ ] 验证失败时展示 failedPoint 和 nextRecommendation。
- [ ] 新增 `CaseCloseDialog.tsx`。
- [ ] 关闭前检查未完成 verification、pending approval、executing workflow、未沉淀 assets。
- [ ] 调用 `closeCase(caseId, payload)`。
- [ ] 关闭成功后刷新详情或跳回列表。
- [ ] 单测覆盖验证失败展示和关闭前阻塞提示。

## 12. Task 9：Assets 资产沉淀入口

- [ ] 新增 `CaseAssetsPanel.tsx`。
- [ ] 展示 Postmortem draft、Experience candidate、Runbook draft、Workflow draft、Memory candidate、OpsGraph patch candidate。
- [ ] candidate/draft 必须有状态标识，不能显示为 published。
- [ ] 增加“生成复盘草稿”和“生成经验候选”入口。
- [ ] 入口只调用后端 action，不在前端拼接经验内容。
- [ ] 单测覆盖 candidate 状态标识。

## 13. Task 10：页面集成测试和视觉检查

- [ ] 扩展 `web/src/pages/complexPages.test.tsx`，覆盖 `/incidents`、`/incidents/:caseId`、审批、关闭、DebugCase。
- [ ] 新增或扩展 Playwright 用例，覆盖桌面和移动宽度下的 case 列表、详情页、右侧栏折叠。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含视觉变更，启动 dev server 并用浏览器截图检查 `/incidents` 和 `/incidents/incident-1`。

## 14. 交付检查

- [ ] 页面不直接调用裸 `fetch`，所有请求走 API client。
- [ ] 页面不创建私有 WebSocket 或 SSE。
- [ ] DebugCase 不渲染 request body、cookie、token、password、用户输入原文。
- [ ] ActionProposal、Approval、WorkflowRun、Verification 都能从 case 页面追溯。
- [ ] Case close 前能提示未完成验证、未处理审批和未沉淀资产。
- [ ] 新增组件文件职责单一，避免继续扩大 `IncidentWorkbenchPage.tsx`。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
