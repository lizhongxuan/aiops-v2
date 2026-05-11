# aiops-v2 Governed Action & RBAC 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-02-governed-action-rbac-module-design.zh.md](2026-05-11-aiops-v2-02-governed-action-rbac-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-02b-governed-action-rbac-frontend-todo.zh.md](2026-05-11-aiops-v2-02b-governed-action-rbac-frontend-todo.zh.md)

## 1. 页面目标

Governed Action & RBAC 前端的目标是把“生产写动作的硬边界”变成用户可理解、可操作、可审计的产品界面。用户在 AI Chat、Case 工作台、Runbook、Workflow 或人工页面发起动作时，前端必须让用户清楚看到：

- 动作具体是什么，不是模糊的“同意 AI 修复”。
- 动作来自哪里：AI、Runbook、Workflow、人工、break-glass。
- 动作要操作哪些资源：环境、服务、主机、中间件、namespace。
- 平台为什么允许、拒绝或要求审批。
- 谁需要审批，当前卡在哪个环节。
- 是否持有 HostLease，是否与其他会话冲突。
- 执行后怎么验证，失败怎么回滚。
- 完整审计记录在哪里。

前端不能新增绕过治理的快捷按钮。任何生产写动作都必须以 `ActionProposal` 为中心。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/ApprovalManagementPage.tsx`：已有审批流水和授权记录初版。
- `web/src/api/approvalManagement.js`：已有 approval audit、grant lifecycle API client。
- `web/src/api/approvals.js`：已有 approval decision API。
- `web/src/pages/complexPagesApi.ts`：已有临时 `fetchApprovalAudits`、`fetchApprovalGrants`、`submitApprovalDecision`。
- `web/src/chat/components/ApprovalBlockPart.tsx`：已有 Chat 内审批块。
- `web/src/pages/IncidentWorkbenchPage.tsx`：已有 pending approval 展示初版。
- `web/src/pages/RunnerStudioPage.tsx`：Workflow 节点和运行状态已有基础。
- `web/src/app/navigation.ts`：已有 `/approval-management` 路由。

主要不足：

- 审批页仍以“审批流水/授权记录”为中心，没有 ActionProposal、PolicyDecision、HostLease、ActionToken 的完整概念。
- 审批详情缺少证据、风险、回滚、验证、资源范围和 normalized input hash。
- 缺少 HostLease 冲突面板，用户不知道为什么动作被拒绝或等待。
- 缺少 RBAC 当前权限视图，用户不知道自己能看、能提议、能审批、能执行什么。
- 缺少 break-glass 的受控入口和强审计提示。
- Chat、Case、Workflow 的审批显示还不是同一个 view-model。

## 3. 信息架构

保留现有 `/approval-management` 作为 MVP 入口，页面语义升级为“动作治理中心”。

建议后续别名：

```text
/governance/actions       -> redirect /approval-management
/governance/rbac          -> Settings 内 RBAC 页面
/governance/audit         -> 审计专页或 approval-management tab
```

MVP 页面 tabs：

```text
待审批
动作提案
主机锁
授权记录
审计流水
策略/权限
Break-glass
```

其中“策略/权限”MVP 先只读展示，避免第一阶段引入危险配置写入口。

## 4. 页面一：动作治理中心

### 4.1 布局

```text
Header: 动作治理中心 / 环境 / 刷新 / 我的审批 / Break-glass
Summary Strip: 待审批 / 高风险 / 锁冲突 / 今日拒绝 / Break-glass
Tabs:
  - Pending Approvals
  - Action Proposals
  - Host Leases
  - Grants
  - Audit
  - Policy & RBAC
  - Break-glass
Right Drawer:
  - ActionProposal detail
  - PolicyDecision detail
  - Evidence / rollback / verification
```

### 4.2 待审批 Tab

列表列：

| 列 | 内容 |
| --- | --- |
| 动作 | toolName、动作标题、来源 |
| Case | caseId、case title、SEV |
| 风险 | readonly / low / medium / high / destructive |
| 目标资源 | service、host、middleware、namespace |
| 申请人 | createdBy、sessionId、turnId |
| 阻塞原因 | requiredApprovals、requiredPrechecks、hostLeaseRequired |
| 到期时间 | expiresAt |
| 操作 | 查看详情、批准、拒绝 |

审批按钮不能直接出现在表格裸行上执行。点击批准或拒绝时必须先打开详情 drawer 或确认 dialog，展示证据、风险、回滚和验证。

### 4.3 ActionProposal 详情 Drawer

必须展示：

- 动作来源：AI / Runbook / Workflow / manual / break-glass。
- 关联 case、session、turn。
- toolName。
- normalizedInputHash。
- 结构化 toolInput。
- targetResources。
- risk。
- expectedEffect。
- rollback。
- verificationSpec。
- supporting evidence refs。
- policy decision。
- required approvals。
- host lease 状态。
- audit trail。

详情页操作：

- 批准。
- 拒绝。
- 要求补充证据。
- 打开 Case。
- 打开 Prompt Trace。
- 打开 Workflow run。

`destructive` 风险动作默认不显示“批准并执行”，只允许“批准提案，等待执行链路继续校验”。

### 4.4 Action Proposals Tab

展示所有动作提案，不只待审批：

- draft。
- pending_policy。
- pending_approval。
- approved。
- rejected。
- expired。
- executed。
- failed。

用途：

- SRE 查看某个 case 的动作历史。
- 审计人员查看被拒绝、过期、策略拒绝的动作。
- 调试 ActionToken 或 HostLease 绑定问题。

### 4.5 Host Leases Tab

HostLease 面板展示：

| 列 | 内容 |
| --- | --- |
| 主机/资源 | hostId、resource label |
| 模式 | exclusive / shared_readonly |
| 状态 | active / released / expired |
| 持有人 | userId、sessionId、caseId |
| 原因 | reason |
| TTL | expiresAt、heartbeatAt |
| 冲突 | blocked proposals |

交互：

- 打开 case。
- 打开持有会话。
- 查看冲突动作。
- 管理员可释放过期锁；不能释放 active 锁，除非具备 break-glass 或 admin 权限并记录原因。

### 4.6 授权记录 Tab

沿用现有 grants，但语义升级：

- grantId。
- resource scope。
- command/toolName。
- grantedAt。
- expiresAt。
- status。
- grantedBy。
- risk。
- usage count。

操作：

- disable。
- enable。
- revoke。

禁用和撤销必须写 audit。

### 4.7 审计流水 Tab

审计流水必须展示成功和失败：

- proposal created。
- policy allow / deny。
- approval approved / rejected / expired。
- token issued / revoked / used。
- host lease acquired / conflict / released / expired。
- tool dispatched / blocked / failed。
- break-glass requested / approved / executed。

筛选：

- caseId。
- actor。
- action。
- resource。
- risk。
- decision。
- 时间窗口。

### 4.8 Policy & RBAC Tab

MVP 先只读展示：

- 当前用户。
- 所属团队。
- 角色。
- 资源范围。
- 可发起动作等级。
- 可审批动作等级。
- 可执行资源范围。
- break-glass 权限。
- 当前策略版本。

后续再提供 admin 编辑页，避免早期把权限配置做成高风险在线编辑入口。

### 4.9 Break-glass Tab

Break-glass 是受控紧急入口，不是普通快捷按钮。

页面必须要求：

- 选择 case。
- 选择资源。
- 选择动作类型。
- 填写紧急原因。
- 展示将绕过或放宽哪些策略。
- 展示强审计和复盘要求。
- 二次确认。

提交后：

```text
POST /api/v1/break-glass/requests
  -> 创建 break-glass action proposal
  -> 通知审计渠道
  -> 强制 postmortem follow-up
```

## 5. 嵌入式治理组件

全局动作治理中心不是唯一入口。以下页面必须复用同一组件和 view-model。

### 5.1 Chat 中的 Approval Block

Chat 中的高风险建议显示为：

- “待审批动作”块。
- 风险和目标资源。
- 打开详情。
- 批准/拒绝入口。

不能显示为普通“执行”按钮。

### 5.2 Case 工作台 Actions Tab

Case 工作台展示当前 case 的：

- ActionProposal。
- Approval。
- ActionToken。
- HostLease。
- Audit。

这部分应复用动作治理中心的表格/详情组件，避免两套审批 UI。

### 5.3 Workflow 节点

Workflow 节点详情展示：

- 节点对应 ActionProposal。
- PolicyDecision。
- Approval。
- HostLease。
- ActionToken。
- ToolResult。

如果节点因为锁冲突或策略拒绝失败，错误信息必须来自治理对象，不由 Workflow 页面自行解释。

## 6. 前端数据模型

建议新增 `web/src/api/governance.ts`。

```ts
export type ActionRisk = "readonly" | "low" | "medium" | "high" | "destructive";

export type ActionProposalView = {
  id: string;
  caseId: string;
  sessionId?: string;
  turnId?: string;
  source: "ai" | "runbook" | "workflow" | "manual" | "break_glass";
  title: string;
  toolName: string;
  toolInput: Record<string, unknown>;
  normalizedInputHash: string;
  targetResources: ResourceRefView[];
  risk: ActionRisk;
  approvalRequired: boolean;
  hostLeaseRequired: boolean;
  expectedEffect?: string;
  rollback?: string;
  verificationSpec?: Record<string, unknown>;
  evidenceRefs: string[];
  policyDecision?: PolicyDecisionView;
  approvals: ApprovalView[];
  hostLeases: HostLeaseView[];
  actionToken?: ActionTokenView;
  status: string;
  createdBy?: string;
  expiresAt?: string;
};
```

```ts
export type PolicyDecisionView = {
  allow: boolean;
  reasons: string[];
  requiredApprovals: string[];
  requiredLeases: string[];
  requiredPrechecks: string[];
  deniedFields: string[];
  auditLevel: string;
};
```

```ts
export type RbacMeView = {
  userId: string;
  teams: string[];
  roles: string[];
  permissions: string[];
  resourceScopes: ResourceScopeView[];
  canBreakGlass: boolean;
};
```

## 7. API 契约

Canonical API：

```text
GET    /api/v1/action-proposals
POST   /api/v1/action-proposals
GET    /api/v1/action-proposals/{id}
POST   /api/v1/action-proposals/{id}/policy-check
POST   /api/v1/action-proposals/{id}/approve
POST   /api/v1/action-proposals/{id}/reject
POST   /api/v1/action-proposals/{id}/request-evidence
POST   /api/v1/action-proposals/{id}/issue-token
GET    /api/v1/host-leases
POST   /api/v1/host-leases/acquire
POST   /api/v1/host-leases/{id}/heartbeat
POST   /api/v1/host-leases/{id}/release
GET    /api/v1/audit/actions
GET    /api/v1/rbac/me
GET    /api/v1/policies/action-governance
POST   /api/v1/break-glass/requests
```

兼容期：

- `/api/v1/approval-audits` 映射为 Audit tab。
- `/api/v1/approval-grants` 映射为 Grants tab。
- `/api/v1/approvals/{id}/decision` 继续作为 Approval decision fallback。

页面组件只能依赖 `governance.ts` view-model，不能在各页面分散兼容逻辑。

## 8. 状态与数据流

```text
ActionProposal source
  -> governance API
  -> normalizeGovernanceView
  -> global governance page / embedded component
  -> user decision
  -> approval API
  -> refresh proposal / case / workflow
```

实时策略：

- MVP 使用刷新和短间隔 polling。
- 后续接入统一 event/transport。
- 不新增页面私有 WebSocket 或 SSE。

## 9. 错误与空态

- 权限不足：显示“你无权审批此动作”，隐藏敏感 toolInput。
- 策略拒绝：显示 PolicyDecision reasons，不显示“系统错误”。
- 锁冲突：显示持有人、caseId、expiresAt 和冲突资源。
- 过期审批：禁用审批按钮，提供刷新。
- Break-glass 提交失败：保留用户输入，并显示失败原因。

## 10. 安全与显示规则

- toolInput 默认折叠；包含 secret、token、password、cookie 的字段必须脱敏。
- normalizedInputHash 永远展示，便于审计对账。
- 审批确认必须显示 risk、targetResources、rollback、verificationSpec。
- `destructive` 风险动作必须二次确认。
- 普通 viewer 只能看裁剪后的详情。
- Break-glass 权限和操作必须显著标记。

## 11. 验收标准

- `/approval-management` 展示待审批、动作提案、主机锁、授权记录、审计流水、策略权限和 Break-glass tabs。
- 审批详情能看到 source、case、turn、toolName、normalizedInputHash、targetResources、risk、expectedEffect、rollback、verificationSpec、evidenceRefs 和 policyDecision。
- 用户不能在没有打开详情或确认 dialog 的情况下批准高风险动作。
- HostLease 冲突能展示冲突资源、持有人、caseId 和 TTL。
- RBAC 当前权限页能展示用户角色、资源范围、可审批等级和 break-glass 权限。
- Break-glass 提交必须要求紧急原因和二次确认。
- Case、Chat、Workflow 复用同一个 ActionProposal / Approval view-model。
- 页面不直接绕过 approval/action API 执行生产工具。
