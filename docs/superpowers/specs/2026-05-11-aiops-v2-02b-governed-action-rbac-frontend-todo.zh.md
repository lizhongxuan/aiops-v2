# aiops-v2 Governed Action & RBAC 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-02a-governed-action-rbac-frontend-design.zh.md](2026-05-11-aiops-v2-02a-governed-action-rbac-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-02-governed-action-rbac-module-design.zh.md](2026-05-11-aiops-v2-02-governed-action-rbac-module-design.zh.md)

## 1. 目标

把现有 `ApprovalManagementPage.tsx` 从“审批流水 + 授权记录”升级为动作治理中心，并抽出可复用的 ActionProposal、Approval、PolicyDecision、HostLease、RBAC、Audit 和 Break-glass 前端组件，供 Chat、Case 工作台和 Workflow 页面共享。

## 2. 实施顺序

```text
governance API view-model
  -> 动作治理中心页面骨架
  -> ActionProposal 详情和审批
  -> HostLease 面板
  -> Audit / Grants 兼容迁移
  -> RBAC / Policy 只读页
  -> Break-glass 受控入口
  -> Chat / Case / Workflow 嵌入复用
  -> 测试和视觉检查
```

先做只读治理中心和审批详情，再做 Break-glass 和跨页面复用。

## 3. 文件地图

新增：

- `web/src/api/governance.ts`：ActionProposal、PolicyDecision、HostLease、RBAC、Audit、Break-glass API client 和 normalize。
- `web/src/api/governance.test.ts`：兼容旧 approval audits/grants 和新 action proposals 的 normalize 测试。
- `web/src/components/governance/GovernanceSummaryStrip.tsx`：治理指标条。
- `web/src/components/governance/ActionProposalTable.tsx`：动作提案表格。
- `web/src/components/governance/ActionProposalDetailDrawer.tsx`：动作详情 drawer。
- `web/src/components/governance/ApprovalDecisionDialog.tsx`：批准/拒绝确认。
- `web/src/components/governance/PolicyDecisionPanel.tsx`：策略判断详情。
- `web/src/components/governance/HostLeaseTable.tsx`：主机锁列表。
- `web/src/components/governance/GrantTable.tsx`：授权记录表。
- `web/src/components/governance/AuditTable.tsx`：审计流水表。
- `web/src/components/governance/RbacMePanel.tsx`：当前用户权限视图。
- `web/src/components/governance/BreakGlassPanel.tsx`：break-glass 请求表单。
- `web/src/components/governance/governanceViewModels.ts`：风险文案、状态排序、脱敏、分组。
- `web/src/components/governance/governanceViewModels.test.ts`：view-model 单测。
- `web/src/components/governance/governanceComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/ApprovalManagementPage.tsx`：重构为动作治理中心，使用 governance 组件。
- `web/src/api/approvalManagement.js`：保留兼容；新代码不继续扩展它。
- `web/src/api/approvals.js`：保留 approval decision fallback。
- `web/src/pages/complexPagesApi.ts`：停止新增治理 API；逐步迁移到 `governance.ts`。
- `web/src/chat/components/ApprovalBlockPart.tsx`：复用 ActionProposal 摘要/详情入口。
- `web/src/pages/IncidentWorkbenchPage.tsx` 或后续 incidents 组件：复用 governance 组件展示 actions。
- `web/src/pages/ExecutionRunDetailPage.tsx` 或 runner 节点详情组件：展示节点 ActionProposal / Approval / HostLease；不要求修改现有 Runner 页面。
- `web/src/app/navigation.ts`：将 `/approval-management` 文案调整为“动作治理”或“Action Governance”。
- `web/src/pages/complexPages.test.tsx`：更新审批管理页面测试。

## 4. Task 1：建立 Governance API 与类型

- [ ] 新增 `web/src/api/governance.ts`。
- [ ] 定义 `ActionRisk`、`ResourceRefView`、`ActionProposalView`、`PolicyDecisionView`、`ApprovalView`、`ActionTokenView`、`HostLeaseView`、`AuditRecordView`、`GrantView`、`RbacMeView`、`BreakGlassRequest`。
- [ ] 实现 `listActionProposals(params)`、`getActionProposal(id)`、`policyCheckActionProposal(id)`。
- [ ] 实现 `approveActionProposal(id, payload)`、`rejectActionProposal(id, payload)`、`requestActionEvidence(id, payload)`。
- [ ] 实现 `listHostLeases(params)`、`releaseHostLease(id, payload)`。
- [ ] 实现 `listActionAudits(params)`、`listApprovalGrants(params)`、`updateApprovalGrant(id, action)`。
- [ ] 实现 `getRbacMe()`、`getActionPolicies()`、`requestBreakGlass(payload)`。
- [ ] 兼容旧接口：`/api/v1/approval-audits` 和 `/api/v1/approval-grants` normalize 到新 view-model。
- [ ] 新增 `web/src/api/governance.test.ts`，覆盖新旧 payload normalize。
- [ ] 运行 `npm --prefix web test -- governance.test.ts`。

## 5. Task 2：重构动作治理中心页面骨架

- [ ] 新增 `GovernanceSummaryStrip.tsx`。
- [ ] 修改 `ApprovalManagementPage.tsx`，标题改为“动作治理中心”。
- [ ] tabs 调整为：待审批、动作提案、主机锁、授权记录、审计流水、策略/权限、Break-glass。
- [ ] 页面数据从 `governance.ts` 加载，不直接依赖 `complexPagesApi.ts`。
- [ ] loading、error、empty state 统一使用现有 `LoadingState`、`StatusAlert`、`EmptyPanel`。
- [ ] 单测覆盖 tabs 和 summary 指标。

## 6. Task 3：ActionProposal 表格与详情

- [ ] 新增 `ActionProposalTable.tsx`。
- [ ] 表格列包括动作、Case、风险、目标资源、申请人、阻塞原因、到期时间、操作。
- [ ] 新增 `ActionProposalDetailDrawer.tsx`。
- [ ] Drawer 展示 source、caseId、sessionId、turnId、toolName、normalizedInputHash、toolInput、targetResources、risk、expectedEffect、rollback、verificationSpec、evidenceRefs、policyDecision、approvals、hostLeases、audit trail。
- [ ] toolInput 默认折叠，敏感字段脱敏。
- [ ] 高风险动作只能从详情 drawer 进入 approval dialog，不能从表格行直接批准。
- [ ] 单测覆盖高风险动作表格无直接批准按钮、详情显示 hash 和回滚。

## 7. Task 4：审批确认 Dialog

- [ ] 新增 `ApprovalDecisionDialog.tsx`。
- [ ] 批准确认展示 risk、targetResources、rollback、verificationSpec、policy reasons。
- [ ] 拒绝确认要求输入拒绝原因。
- [ ] destructive 风险动作要求二次确认短语。
- [ ] 调用 `approveActionProposal` / `rejectActionProposal`，若后端尚未支持则 fallback 到 `/api/v1/approvals/{id}/decision`。
- [ ] 成功后刷新 proposal list 和当前 drawer。
- [ ] 失败后保留 drawer 状态并显示错误。
- [ ] 单测覆盖 approve/reject payload 和 destructive 二次确认。

## 8. Task 5：PolicyDecision 面板

- [ ] 新增 `PolicyDecisionPanel.tsx`。
- [ ] 展示 allow、reasons、requiredApprovals、requiredLeases、requiredPrechecks、deniedFields、auditLevel。
- [ ] 策略拒绝时用明确文案展示原因，例如证据不足、变更窗口外、权限不足、锁冲突。
- [ ] `deniedFields` 不显示敏感原始值。
- [ ] 单测覆盖 allow 和 deny 两种状态。

## 9. Task 6：HostLease 面板

- [ ] 新增 `HostLeaseTable.tsx`。
- [ ] 展示 hostId、mode、status、userId、sessionId、caseId、reason、heartbeatAt、expiresAt、blocked proposals。
- [ ] expired lease 显示可清理状态。
- [ ] active lease 仅 admin 或 break-glass 用户显示释放入口。
- [ ] 释放锁必须要求原因，并写入 audit payload。
- [ ] 单测覆盖锁冲突展示和非 admin 不显示释放按钮。

## 10. Task 7：Grants 与 Audit 迁移

- [ ] 新增 `GrantTable.tsx`，迁移现有授权记录表。
- [ ] 支持 enable、disable、revoke。
- [ ] 新增 `AuditTable.tsx`，迁移现有审批流水并扩展 action audit 类型。
- [ ] 审计筛选支持 caseId、actor、action、resource、risk、decision、时间窗口。
- [ ] 被拒绝、过期、权限不足、锁冲突、策略拒绝都能展示。
- [ ] 单测覆盖旧 approval audit payload 仍能渲染。

## 11. Task 8：RBAC / Policy 只读视图

- [ ] 新增 `RbacMePanel.tsx`。
- [ ] 展示 userId、teams、roles、permissions、resourceScopes、canBreakGlass。
- [ ] 展示当前可发起动作等级、可审批动作等级和可执行资源范围。
- [ ] 新增策略只读区，展示 action governance policy version 和核心规则。
- [ ] 普通用户不能编辑策略。
- [ ] 单测覆盖有/无 break-glass 权限两种状态。

## 12. Task 9：Break-glass 受控入口

- [ ] 新增 `BreakGlassPanel.tsx`。
- [ ] 表单字段包括 caseId、resourceRefs、actionType、risk、reason、expectedEffect、rollback、verification。
- [ ] reason 必填，长度少于 20 字时不能提交。
- [ ] 提交前显示二次确认，说明会强审计并创建 postmortem follow-up。
- [ ] 调用 `requestBreakGlass(payload)`。
- [ ] 无 break-glass 权限时只显示说明和申请权限提示，不显示提交表单。
- [ ] 单测覆盖权限不足、原因不足、成功提交。

## 13. Task 10：跨页面复用

- [ ] 更新 `web/src/chat/components/ApprovalBlockPart.tsx`，使用 governance view-model 展示 action proposal 摘要和详情入口。
- [ ] 更新 Incident Control Plane 的 Actions panel 计划，明确复用 `ActionProposalTable` 和 `ActionProposalDetailDrawer`。
- [ ] 更新 Execution Run Detail 或 runner 节点详情计划，展示节点绑定的 ActionProposal、Approval、HostLease；不要求修改现有 Runner 页面。
- [ ] 确保三个入口的 approve/reject 最终调用同一个 governance API 函数。
- [ ] 单测覆盖 Chat approval block 不出现绕过审批的“直接执行”按钮。

## 14. Task 11：测试与视觉检查

- [ ] 扩展 `web/src/pages/complexPages.test.tsx`，覆盖 `/approval-management` 的 tabs、proposal detail、approval decision、host lease、rbac、break-glass。
- [ ] 新增 `governanceComponents.test.tsx`，覆盖组件级安全渲染和操作。
- [ ] 新增或扩展 Playwright 用例，覆盖动作治理中心桌面和移动布局。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果包含视觉实现，启动 dev server 并用浏览器检查 `/approval-management`。

## 15. 交付检查

- [ ] 高风险动作不能在表格行直接批准。
- [ ] 审批确认必须显示 risk、targetResources、rollback、verificationSpec。
- [ ] toolInput 中的 secret、token、password、cookie 字段被脱敏。
- [ ] HostLease 冲突能显示持有人、caseId、expiresAt 和冲突资源。
- [ ] RBAC 只读页能展示当前用户角色、资源范围和 break-glass 权限。
- [ ] Break-glass 必须填写原因并二次确认。
- [ ] Chat、Case、Workflow 不再各自维护独立审批 UI。
- [ ] 页面不直接执行生产工具，只调用治理 API。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
