# aiops-v2 Incident Control Plane 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-01-incident-control-plane-module-design.zh.md](2026-05-11-aiops-v2-01-incident-control-plane-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-01b-incident-control-plane-frontend-todo.zh.md](2026-05-11-aiops-v2-01b-incident-control-plane-frontend-todo.zh.md)

## 1. 页面目标

Incident Control Plane 的前端不是“事故列表 + 聊天页”的组合，而是生产事实工作台。页面应让用户在一个 case 中看到：

- 当前事故或操作目标是什么。
- 影响哪些业务能力、服务、主机和中间件。
- 当前证据、根因假设、动作、审批、执行、验证和复盘状态。
- 哪些信息来自 Coroot、ERP、DebugEvent、用户、Workflow、Runbook 或人工输入。
- 下一步可以做什么，以及哪些动作需要审批和主机锁。

设计原则：

- 第一屏显示可扫描的生产状态，不做营销式说明。
- 页面只消费统一 API view-model 和统一 realtime/transport 状态，不私有维护执行状态。
- 证据、动作、验证都围绕 case 展示，避免用户在 Coroot、Runbook、Approval、Workflow 页面之间来回跳。
- UI 密度偏运维控制台：信息清晰、紧凑、可比较，不做大面积装饰卡片。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/IncidentListPage.tsx`：已有事故列表初版。
- `web/src/pages/IncidentWorkbenchPage.tsx`：已有事故详情初版，包含 hypothesis、evidence、pending approvals、OpsGraph、Runbook 匹配。
- `web/src/pages/complexPagesApi.ts`：已有 `listIncidents`、`getIncident`、`matchRunbooks`、`listRunbookInstances` 等临时聚合 API client。
- `web/src/app/navigation.ts` 与 `web/src/router.tsx`：已有 `/incidents` 和 `/incidents/:incidentId` 路由。
- `web/src/components/ui/*`：已有 Button、Card、Badge、Dialog、Sheet、Tabs、Tooltip 等基础组件。
- `web/src/transport/*` 与 `web/src/chat/components/*`：已有 AI Chat / transport 运行态基础。

主要不足：

- `IncidentListPage` 缺少状态、严重级别、环境、来源、业务能力等筛选和批量扫描视图。
- `IncidentWorkbenchPage` 仍是多个临时面板堆叠，没有形成 case header、状态轨道、上下文栏、证据时间线和动作区的稳定布局。
- `complexPagesApi.ts` 过于聚合，后续应拆出 `web/src/api/cases.ts` 和页面 view-model。
- 详情页目前用 `pendingApprovals` 直接调用 approval API，但还没有 `ActionProposal`、`VerificationRecord`、`TimelineEvent` 的完整表达。
- 缺少创建 case、合并/关联 case、关闭 case、生成复盘/经验候选的前端入口。

## 3. 路由与信息架构

保持现有路由：

```text
/incidents
/incidents/:caseId
```

页面语义改成：

- `/incidents`：Case 队列。展示 `incident`、`operation`、`debug`、`middleware_repair` 等活跃 case，而不只是 Coroot incident。
- `/incidents/:caseId`：Case 工作台。所有证据、动作、验证和资产沉淀都从这里进入。

后续可选路由：

```text
/incidents/:caseId/timeline
/incidents/:caseId/postmortem
```

MVP 不新增这些子路由，先在工作台内用 tabs 和 drawer 承载。

## 4. 页面一：Case 队列

### 4.1 布局

```text
Header: Cases / 环境 / 新建 Case / 刷新
Filter Bar: 状态 / 类型 / SEV / 来源 / 环境 / 业务能力 / 时间窗口
Summary Strip: 活跃数 / SEV1 / 待审批 / 执行中 / 验证中
Main Table: case 列表
Right Rail: 当前最高风险 case 摘要
```

桌面端使用左右布局；窄屏时 Right Rail 收到列表下方。

### 4.2 表格列

| 列 | 内容 |
| --- | --- |
| Case | 标题、摘要、case id |
| 类型 | incident / operation / debug / middleware_repair |
| 状态 | new / diagnosing / awaiting_approval / executing / verifying / resolved |
| SEV | sev0-sev3 / info |
| 来源 | coroot / erp / user / frontend_debug / middleware / ticket / manual |
| 业务影响 | capability、tenant count、SLO |
| 技术影响 | service、host、middleware |
| 阻塞项 | 待证据、待审批、锁冲突、验证失败 |
| 更新时间 | latest timeline event |

### 4.3 交互

- 点击行进入 `/incidents/:caseId`。
- 筛选条件进入 URL query，便于分享。
- “新建 Case”打开右侧 Sheet，不跳离列表。
- “只看我的”按 ownerUserId / ownerTeamId 筛选。
- “待我审批”按 pending approval 筛选。

### 4.4 新建 Case Sheet

字段：

- 类型：incident / operation / debug / middleware_repair / maintenance / drill。
- 标题。
- 环境。
- 严重级别。
- 业务能力。
- 影响服务。
- 影响中间件。
- 初始摘要。

提交后：

```text
POST /api/v1/cases
  -> 返回 case
  -> 跳转 /incidents/:caseId
```

## 5. 页面二：Case 工作台

### 5.1 总体布局

```text
┌────────────────────────────────────────────────────────────┐
│ Case Header: 标题 / 状态 / SEV / 类型 / 环境 / Owner / Actions │
├────────────────────────────────────────────────────────────┤
│ Status Rail: triaging -> diagnosing -> planning -> executing │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区                       │ 右侧上下文栏                  │
│ Tabs: Overview / Evidence /    │ Impact / OpsGraph / Coroot / │
│ Actions / Verification /       │ Runbook / HostLease / Trace  │
│ Timeline / Assets              │                              │
└──────────────────────────────┴─────────────────────────────┘
```

主工作区宽度优先，右侧栏固定在 360-420px。移动端右侧栏变成 tabs。

### 5.2 Case Header

显示：

- 标题、摘要。
- 状态、类型、严重级别。
- 环境、Owner、团队。
- 影响业务能力、服务、中间件数量。
- 最新更新时间。

动作：

- 刷新。
- 继续对话。
- 关联 Case。
- 合并 Case。
- 关闭 Case。
- 生成复盘。

关闭 Case 必须打开确认 Dialog，展示验证状态、未关闭动作、未处理审批和经验沉淀状态。

### 5.3 Status Rail

状态轨道展示：

```text
new -> triaging -> diagnosing -> planning -> awaiting_approval -> executing -> verifying -> mitigated -> resolved -> closed
```

要求：

- 当前状态高亮。
- 已失败分支显示失败点。
- `awaiting_approval` 显示待审批数量。
- `executing` 显示 active workflow / tool count。
- `verifying` 显示 observation window。

### 5.4 Overview Tab

Overview 是第一屏核心：

- `Current Summary`：当前事故摘要和影响范围。
- `Top Hypotheses`：根因假设排名、置信度、支持证据、反证数量。
- `Recommended Next Actions`：只读诊断、Runbook step、Workflow、ActionProposal。
- `Blocking Items`：缺失证据、待审批、锁冲突、验证失败。
- `Latest Timeline`：最近 5 条 timeline event。

### 5.5 Evidence Tab

证据按来源分组：

- Coroot。
- ERP。
- Browser Plugin Debug。
- Trace。
- Tool。
- Workflow。
- Middleware。
- Change。
- User。

每条证据显示：

- 来源 icon。
- 摘要。
- 关联实体。
- 置信度。
- 脱敏状态。
- observedAt。
- rawRef 链接或展开摘要。

敏感或权限不足证据只显示“受限证据”和引用 id，不显示正文。

### 5.6 Actions Tab

动作区展示：

- ActionProposal 列表。
- Pending Approval。
- ActionToken 状态。
- HostLease 状态。
- Workflow run 节点状态。
- 已完成和失败动作。

操作规则：

- 只读动作可以显示“运行诊断”。
- 中高风险动作只能显示“提交审批”或“等待审批”。
- 审批按钮复用统一 approval API，不在页面里创建私有审批流程。
- ActionProposal 展开后显示工具名、标准化输入、风险、预期效果、回滚和验证。

### 5.7 Verification Tab

展示：

- VerificationSpec。
- VerificationRecord。
- Coroot SLO 恢复。
- ERP 业务恢复。
- Debug Trace 对比。
- 中间件健康。
- 人工确认。

状态：

- pending。
- running。
- passed。
- failed。
- inconclusive。
- skipped。

当验证失败时，页面必须显示 failedPoint 和 nextRecommendation。

### 5.8 Timeline Tab

Timeline 是 append-only event log：

- case.created。
- evidence.added。
- hypothesis.updated。
- action.proposed。
- approval.changed。
- workflow.event。
- verification.completed。
- case.closed。

用户可以按类型、来源、actor 过滤。Timeline 只读，不提供编辑。

### 5.9 Assets Tab

展示可沉淀资产：

- Postmortem draft。
- Experience candidate。
- Runbook draft。
- Workflow draft。
- Memory candidate。
- OpsGraph patch candidate。

未审核资产必须标记 candidate/draft，不进入正式推荐。

## 6. 右侧上下文栏

### 6.1 Impact

- ERP business capability。
- tenant impact。
- SLO。
- 业务任务或订单影响摘要。

### 6.2 OpsGraph

展示 1-2 层邻域：

```text
BusinessCapability -> Service -> Middleware -> Host/Pod
```

不在工作台内做复杂全图编辑，只提供“打开 OpsGraph 详情”。

### 6.3 Coroot

- SLO status。
- RCA summary。
- latency / error / saturation 摘要。
- topology ref。

### 6.4 Trace / Debug

当 case 类型为 `debug` 或有 traceContextRefs 时显示：

- trace id。
- frontend route。
- user action。
- slow span。
- evidence quality。

### 6.5 HostLease

- 当前锁定资源。
- 持有人。
- TTL。
- 冲突说明。

## 7. 前端数据模型

建议新增 `web/src/api/cases.ts`，不要继续把 case API 挤进 `complexPagesApi.ts`。

```ts
export type CaseStatus =
  | "new"
  | "triaging"
  | "diagnosing"
  | "planning"
  | "awaiting_approval"
  | "executing"
  | "verifying"
  | "mitigated"
  | "resolved"
  | "failed"
  | "closed";

export type CaseType = "incident" | "operation" | "debug" | "middleware_repair" | "maintenance" | "drill";

export type CaseView = {
  id: string;
  type: CaseType;
  status: CaseStatus;
  severity: string;
  source: string;
  title: string;
  summary?: string;
  environment?: string;
  ownerUserId?: string;
  ownerTeamId?: string;
  affectedBusinessCapabilities: CaseEntityRef[];
  affectedServices: CaseEntityRef[];
  affectedHosts: CaseEntityRef[];
  affectedMiddleware: CaseEntityRef[];
  evidence: EvidenceView[];
  hypotheses: HypothesisView[];
  actions: ActionProposalView[];
  approvals: ApprovalView[];
  verifications: VerificationView[];
  timeline: TimelineEventView[];
  assets: CaseAssetView[];
  updatedAt?: string;
};
```

## 8. API 契约

页面优先使用 canonical API：

```text
GET    /api/v1/cases
POST   /api/v1/cases
GET    /api/v1/cases/{case_id}
PATCH  /api/v1/cases/{case_id}
POST   /api/v1/cases/{case_id}/evidence
POST   /api/v1/cases/{case_id}/hypotheses
POST   /api/v1/cases/{case_id}/actions
POST   /api/v1/cases/{case_id}/verifications
POST   /api/v1/cases/{case_id}/close
POST   /api/v1/cases/{case_id}/merge
POST   /api/v1/cases/{case_id}/relate
GET    /api/v1/cases/{case_id}/timeline
```

兼容期可以让 `web/src/api/cases.ts` fallback 到现有 `/api/v1/incidents`，但页面组件只能依赖 `cases.ts` 的 view-model，不直接感知兼容路径。

## 9. 状态与数据流

```text
Route
  -> cases API
  -> normalizeCaseView
  -> page state
  -> tabs and panels
```

实时更新策略：

- MVP 使用刷新按钮和定时轻量 polling。
- 后续接入统一 transport/realtime event，不新增页面私有 WebSocket。
- 任何 workflow、approval、tool、verification 运行态都来自统一事件投影或 case timeline。

## 10. 错误、空态与加载态

- 列表加载：显示 skeleton 行，不改变表格高度。
- 详情加载：保留 header skeleton + 内容 skeleton。
- API 失败：顶部 StatusAlert，保留上一次成功数据。
- 空证据：只显示“暂无证据”。
- 权限不足：显示“无权查看此证据/动作”，不暴露资源名和原始内容。
- Case 不存在：显示 404 空态和返回列表按钮。

## 11. 响应式与可访问性

- 桌面端：主区 + 右侧上下文栏。
- 平板端：上下文栏降到主区下方。
- 手机端：tabs 使用横向滚动，表格改为列表行。
- 所有 icon button 必须有 aria-label 或 tooltip。
- 状态不能只靠颜色表达，必须有文本。

## 12. 验收标准

- `/incidents` 能展示 case 队列，并支持状态、类型、严重级别、来源、环境筛选。
- `/incidents/:caseId` 能展示 Case Header、Status Rail、Overview、Evidence、Actions、Verification、Timeline、Assets 和右侧上下文栏。
- DebugCase 不展示请求体、cookie、token 或敏感输入。
- 待审批动作能从工作台触发统一 approval decision API。
- 验证失败能显示 failedPoint 和 nextRecommendation。
- Case close 前能提示未完成验证、未处理审批和未沉淀资产。
- 页面不直接调用裸 `fetch`，所有请求走 `web/src/api/cases.ts` 或既有统一 API client。
- 页面不创建私有 WebSocket/SSE。
