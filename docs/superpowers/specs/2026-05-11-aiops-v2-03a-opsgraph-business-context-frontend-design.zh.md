# aiops-v2 OpsGraph & ERP Business Context 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-03-opsgraph-business-context-module-design.zh.md](2026-05-11-aiops-v2-03-opsgraph-business-context-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-03b-opsgraph-business-context-frontend-todo.zh.md](2026-05-11-aiops-v2-03b-opsgraph-business-context-frontend-todo.zh.md)

## 1. 页面目标

OpsGraph & ERP Business Context 的前端不是单纯的拓扑图页面，而是业务上下文推理工作台。页面要让用户在同一个入口回答：

- 某个技术实体影响哪些 ERP 模块、业务能力、租户、SLO 和活跃 case。
- 某个 ERP 业务异常可能经过哪些服务、API、中间件、主机和 trace span。
- 某次用户侧 Debug Trace 从页面动作到后端慢点、错误点和资源等待点的链路是什么。
- 某个实体有哪些可复用的 Runbook、Workflow、Experience Pack、Memory 和历史 case。
- 每条业务和技术关系来自哪里、置信度是多少、证据引用是什么、是否可用于自动化动作。
- AI 或经验包提出的图谱补丁是否可信，能否进入审核和应用流程。

设计原则：

- 第一屏是可操作的图谱工作台，不做装饰性全屏关系图。
- 图谱可视化必须服务于排障、影响分析、经验匹配和证据解释。
- 所有边都显示 `source`、`confidence`、`updatedAt`、`evidenceRefs`，避免把推断关系伪装成事实。
- 页面消费统一 OpsGraph API view-model，不在页面内拼接 Coroot、ERP、trace 和经验包的私有数据结构。
- ERP 租户、订单、用户等敏感信息默认脱敏，前端只展示业务引用、hash、聚合指标和权限允许的摘要。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/OpsGraphPage.tsx`：已有 OpsGraph 初版页面，支持实体检索、邻域查询和业务影响查询。
- `web/src/api/opsgraph.js`：已有 `lookupOpsGraph`、`getOpsGraphNeighborhood`、`getOpsGraphBusinessImpact`。
- `web/src/api/erp.js`：已有 ERP 健康、指标和租户影响 API client。
- `web/src/pages/ERPHealthPage.tsx`：已有 ERP 健康页面初版。
- `web/src/lib/corootTopologyAdapter.js`：已有 Coroot topology 到前端节点和边的转换逻辑。
- `web/src/lib/corootCardAdapter.js`：已有 Coroot service、SLO、RCA、metrics card 的展示适配。
- `web/src/pages/complexPagesApi.ts`：已有部分复杂页面的临时聚合 API。
- `web/src/components/ui/*`：已有 Button、Card、Badge、Dialog、Sheet、Tabs、Tooltip 等基础组件。

主要不足：

- `OpsGraphPage.tsx` 仍有页面内请求和页面内类型定义，应迁移到统一 API client 和 view-model adapter。
- 现有页面只覆盖搜索、邻域和业务影响，缺少 Impact Map、Root Cause Path、Asset Map、Patch Review 的稳定信息架构。
- 图谱边的来源、置信度、证据、更新时间没有作为核心视觉元素展示。
- ERP 页面、Case 工作台、Debug Trace 页面和 OpsGraph 页面之间还没有形成可复用组件。
- 缺少 OpsGraph patch 审核页，经验包和复盘生成的图谱补丁没有前端审核入口。
- 缺少按权限裁剪证据和敏感业务对象的统一显示规则。

## 3. 路由与信息架构

保持现有入口：

```text
/opsgraph
```

页面语义改成：

- `/opsgraph`：OpsGraph 工作台。支持搜索实体、查看影响、根因路径、资产匹配和图谱补丁。

后续可选路由：

```text
/opsgraph/entities/:entityId
/opsgraph/patches/:patchId
```

MVP 不新增子路由，先把实体 id、查询类型和 tab 写入 URL query，便于从 Case、ERP、Debug、Experience Review 页面跳入：

```text
/opsgraph?entityId=service:checkout-api&tab=impact
/opsgraph?caseId=case-123&tab=root-cause
/opsgraph?traceId=trace-456&tab=root-cause
/opsgraph?patchId=patch-789&tab=patch-review
```

主 tabs：

| Tab | 目标 |
| --- | --- |
| Entity Search | 检索和选择 BusinessCapability、Service、APIRoute、MiddlewareResource、Host、TraceSpanSignature 等实体 |
| Impact Map | 从技术实体或 case 反查业务影响 |
| Root Cause Path | 从 ERP 异常、Debug Trace 或 case 查看候选根因路径 |
| Asset Map | 从实体匹配 Runbook、Workflow、Experience Pack、Memory 和历史 case |
| Patch Review | 审核 AI、复盘或经验包产生的 OpsGraph patch candidate |

## 4. 页面一：OpsGraph 工作台

### 4.1 总体布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: OpsGraph / 环境 / 数据源健康 / 刷新 / 导出证据引用       │
├────────────────────────────────────────────────────────────┤
│ Context Bar: Entity / Case / Trace / ERP Job / 时间窗口 / 权限状态 │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: 业务影响 / 关键路径 / 可用资产 / 待审补丁 / 低置信边 │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Detail Drawer           │
│ Search / Impact / RCA / Asset │ Entity / Edge / Evidence /   │
│ / Patch Review                │ Coroot / ERP / Case / Trace  │
└──────────────────────────────┴─────────────────────────────┘
```

桌面端右侧 Detail Drawer 宽度 360-420px。窄屏时 Detail Drawer 变成底部 Sheet，tabs 保持横向滚动。

### 4.2 Header

显示：

- 当前环境。
- OpsGraph 数据更新时间。
- 数据源健康：ERP、Coroot、Trace、Workflow、Experience Pack。
- 当前用户权限摘要：是否可查看敏感证据、是否可审核 patch、是否可发起动作。

动作：

- 刷新。
- 复制当前图谱查询链接。
- 导出当前证据引用列表。
- 打开 Patch Review。

### 4.3 Context Bar

Context Bar 是跨页面跳转后的落点，支持以下上下文：

| 上下文 | 输入 | 页面行为 |
| --- | --- | --- |
| Entity | entity id、entity type | 加载实体详情、邻域、业务影响和资产匹配 |
| Case | case id | 加载 case 关联实体、影响范围、根因路径和经验匹配 |
| Trace | trace id、frontend action | 加载 Debug Trace 路径和 slow span 关联实体 |
| ERP Job | ERP job id、business ref | 加载业务能力、服务路径和中间件依赖 |
| Manual Query | 用户输入关键词 | 调用实体检索并展示候选结果 |

字段：

- Query：关键词、entity id、case id、trace id 或 ERP job ref。
- Type：BusinessCapability、ERPModule、ERPJob、FrontendPage、UserAction、APIRoute、Service、MiddlewareResource、Host、TraceSpanSignature。
- Environment：prod、staging、dev 或业务自定义环境。
- Time window：最近 15 分钟、1 小时、24 小时、7 天、自定义。
- Source filter：erp、coroot、trace、workflow、experience、manual。
- Confidence：全部、仅高置信、隐藏低置信。

## 5. Entity Search Tab

### 5.1 搜索结果

搜索结果以表格为主，不使用松散卡片瀑布流：

| 列 | 内容 |
| --- | --- |
| Entity | 名称、id、类型 |
| 业务上下文 | ERP module、business capability、tenant impact 摘要 |
| 技术上下文 | service、namespace、middleware、host 摘要 |
| 数据源 | ERP、Coroot、Trace、Experience、Manual |
| 置信度 | 最高边置信度、低置信边数量 |
| 更新时间 | latest observed at |
| 操作 | 查看影响、查看路径、查看资产 |

### 5.2 Entity Summary

选中实体后，在 Summary Strip 和 Detail Drawer 中展示：

- 基础属性：id、type、displayName、environment、owner。
- 业务属性：domain、capability、ERP module、tenant impact。
- 技术属性：service、namespace、deployment、resource、host labels。
- 证据质量：source count、evidence count、low confidence edge count、last updated。
- 关联状态：active cases、matched runbooks、matched workflows、matched experience packs。

实体详情只展示脱敏后的业务引用。权限不足时只显示 entity id、type、hash 和受限原因。

## 6. Impact Map Tab

Impact Map 回答“这个技术实体或 case 影响什么业务”。

### 6.1 布局

```text
Left: Impact Summary Table
Center: Layered Graph
Right: Business Evidence / SLO / Active Cases
```

Layered Graph 分层：

```text
Host/Pod/Middleware
  -> Service
  -> APIRoute
  -> BusinessCapability
  -> ERPModule
  -> Tenant/SLO/ERPJob
```

### 6.2 Impact Summary Table

| 列 | 内容 |
| --- | --- |
| Business Capability | 业务能力名称、所属 ERP module |
| Impact Level | critical、high、medium、low、unknown |
| Evidence | ERP 指标、SLO、case、trace、Coroot 证据数量 |
| Confidence | 影响链路综合置信度 |
| Active Cases | 当前关联 case 数 |
| Suggested Entry | 打开 case、根因路径、资产匹配 |

### 6.3 图谱节点与边

节点要求：

- 不用复杂力导向布局作为主要交互。MVP 使用固定层级布局，确保运维人员能稳定扫描。
- 节点颜色表达实体类型，状态图标表达健康、异常、未知、权限受限。
- 节点点击后打开 Detail Drawer。

边要求：

- 边显示来源和置信度摘要。
- 低置信边使用虚线，不参与高风险自动动作建议。
- 多来源边显示 source count，可展开查看证据。

### 6.4 业务证据栏

右侧显示：

- ERP 指标摘要：任务失败率、订单延迟、业务 SLA、租户影响数量。
- SLO 摘要：availability、latency、error rate、burn rate。
- 活跃 case：case id、状态、severity、owner。
- 证据引用：evidence id、source、observedAt、redactionStatus。

## 7. Root Cause Path Tab

Root Cause Path 回答“这个业务异常或 Debug Trace 可能经过哪些技术路径导致”。

### 7.1 输入来源

支持：

- ERP module / ERP job / business error code。
- Debug Trace id / frontend action。
- Case id。
- BusinessCapability。
- Manual symptom query。

### 7.2 候选路径列表

路径以可比较列表展示，避免用户只能看一张大图：

| 列 | 内容 |
| --- | --- |
| Rank | 候选排序 |
| Path | ERPJob -> BusinessCapability -> Service -> APIRoute -> MiddlewareResource -> TraceSpanSignature |
| Confidence | 综合置信度和主要证据来源 |
| Evidence | slow span、error span、Coroot RCA、ERP 指标、change record |
| Blocking Unknowns | 缺失证据、低置信边、权限受限证据 |
| Suggested Next | 补充观测、打开 case、匹配资产、生成 ActionProposal |

### 7.3 路径详情

点击路径后显示：

- 分段路径图。
- 每条边的 source、confidence、validFrom、validTo、updatedAt、evidenceRefs。
- 支持证据和反证。
- 最近变更、SLO、Coroot RCA、trace slow span。
- 可复用经验和 Runbook 匹配结果。

页面不能把候选路径表述成确定根因。文案使用“候选路径”“支持证据”“反证”“需要补充观测”，只有 case 推进到确认状态后才显示“已确认根因”。

## 8. Asset Map Tab

Asset Map 回答“这个实体或路径上有什么可复用运维资产”。

### 8.1 资产类型

展示：

- RunbookVersion。
- WorkflowVersion。
- ExperiencePack。
- Memory。
- EvalCase。
- Historical Case。
- VerificationSpec。

### 8.2 匹配表格

| 列 | 内容 |
| --- | --- |
| Asset | 名称、版本、类型 |
| Match Reason | 命中的实体、症状、指标、trace、环境 |
| Status | draft、approved、published、deprecated、disabled |
| Risk | low、medium、high、critical |
| Compatibility | 环境、版本、组件、禁用条件 |
| Last Outcome | 最近执行成功率、失败原因、验证结果 |
| Action | 查看详情、打开 Runbook、创建 Workflow run、加入 case |

### 8.3 资产详情

详情 Drawer 展示：

- 适用范围。
- 禁用条件。
- 所需权限和审批等级。
- 输入变量和 host label 需求。
- 最近成功案例和失败案例。
- 关联 OpsGraph 边和证据。
- 可生成的 ActionProposal 或 WorkflowRun。

前端不能直接执行修复动作。所有执行都必须进入 Governed Action Plane，生成 ActionProposal、审批、ActionToken 和审计流水。

## 9. Patch Review Tab

Patch Review 用于审核复盘、经验包和 AI 推理产生的图谱补丁。

### 9.1 补丁列表

| 列 | 内容 |
| --- | --- |
| Patch | patch id、标题、来源 case / experience pack |
| Operations | add_node、add_edge、update_edge_confidence、mark_incompatible |
| Risk | low、medium、high、critical |
| Evidence | 证据数量、受限证据数量、低置信证据数量 |
| Status | candidate、approved、rejected、applied |
| Reviewer | 当前 reviewer、reviewedAt |
| Action | 查看、批准、拒绝 |

### 9.2 补丁详情

详情必须展示：

- 变更摘要。
- 每个 operation 的 before / after。
- 新增或修改的节点和边。
- 证据引用和证据来源。
- 受影响的业务能力、服务、中间件、Runbook、Experience Pack。
- 风险说明和回滚方式。
- 审核意见输入。

### 9.3 审核规则

- `approved` 和 `rejected` 都必须填写审核意见。
- 高风险 patch 必须展示受影响业务能力和低置信边。
- 权限不足用户只能查看，不能审批。
- 前端只提交审核决策，不直接修改图谱。

## 10. Detail Drawer

Detail Drawer 在所有 tabs 中复用。

### 10.1 Entity Detail

显示：

- entity id、type、displayName、environment。
- 标签和来源。
- 业务上下文。
- 技术上下文。
- 关联 case、runbook、workflow、experience pack。
- 最近证据和更新时间。

动作：

- 打开 Impact Map。
- 打开 Root Cause Path。
- 打开 Asset Map。
- 在 Case 中引用。
- 打开 Coroot、ERP、Trace 外部引用。

### 10.2 Edge Detail

显示：

- from、to、type。
- source、confidence、validFrom、validTo、updatedAt。
- evidenceRefs。
- 是否低置信、是否过期、是否权限受限。
- 是否允许参与自动化动作建议。

### 10.3 Evidence Refs

证据列表统一字段：

| 字段 | 说明 |
| --- | --- |
| evidenceId | 证据 id |
| source | erp、coroot、trace、workflow、experience、manual |
| summary | 脱敏摘要 |
| observedAt | 观测时间 |
| redactionStatus | visible、redacted、restricted |
| rawRef | 权限允许时的外部引用 |

受限证据只显示 `evidenceId`、`source`、`observedAt` 和受限原因。

## 11. 嵌入式入口

OpsGraph 组件需要被多个页面复用，而不是只存在于 `/opsgraph`。

### 11.1 Case 工作台

Case 右侧上下文栏复用：

- Impact Summary。
- Root Cause Path Summary。
- Asset Match Summary。
- Evidence Refs。

Case 页面不显示完整大图，只展示可扫描摘要和跳转到 `/opsgraph` 的入口。

### 11.2 ERP 健康页面

ERPHealth 页面复用：

- ERP module 到 BusinessCapability 的映射摘要。
- 受影响服务和中间件列表。
- 当前业务指标对应的 Root Cause Path 入口。

### 11.3 Debug Trace 页面

Debug 事件完成后跳入 OpsGraph：

```text
FrontendPage -> UserAction -> APIRoute -> Service -> MiddlewareResource -> TraceSpanSignature
```

Debug 页面展示路径摘要和慢点，完整边证据在 OpsGraph 中查看。

### 11.4 Experience Pack Review

经验包审核页复用：

- 匹配实体列表。
- 适用边和禁用边。
- 生成的 OpsGraphPatch preview。
- Patch Review 入口。

## 12. 前端数据模型

新增或迁移到 `web/src/api/opsgraph.ts`：

```ts
export type OpsGraphEntityType =
  | "BusinessDomain"
  | "BusinessCapability"
  | "ERPModule"
  | "ERPJob"
  | "Tenant"
  | "FrontendPage"
  | "UserAction"
  | "APIRoute"
  | "Service"
  | "Deployment"
  | "RuntimeResource"
  | "Host"
  | "Pod"
  | "MiddlewareResource"
  | "Database"
  | "Queue"
  | "Cache"
  | "SLO"
  | "TraceSpanSignature"
  | "ChangeRecord"
  | "RunbookVersion"
  | "WorkflowVersion"
  | "ExperiencePack"
  | "Case";

export interface GraphEntityView {
  id: string;
  type: OpsGraphEntityType;
  displayName: string;
  environment?: string;
  labels: Record<string, string>;
  sourceRefs: EvidenceRefView[];
  redactionStatus: "visible" | "redacted" | "restricted";
  updatedAt?: string;
}

export interface GraphEdgeView {
  id: string;
  fromId: string;
  toId: string;
  type: string;
  source: "coroot" | "erp" | "trace" | "workflow" | "user" | "experience" | "import" | "manual";
  confidence: number;
  validFrom?: string;
  validTo?: string;
  updatedAt?: string;
  evidenceRefs: EvidenceRefView[];
  automationEligible: boolean;
}

export interface EvidenceRefView {
  evidenceId: string;
  source: string;
  summary?: string;
  observedAt?: string;
  rawRef?: string;
  redactionStatus: "visible" | "redacted" | "restricted";
}

export interface OpsGraphNeighborhoodView {
  center: GraphEntityView;
  nodes: GraphEntityView[];
  edges: GraphEdgeView[];
}

export interface ImpactMapView {
  rootEntity: GraphEntityView;
  capabilities: BusinessImpactView[];
  nodes: GraphEntityView[];
  edges: GraphEdgeView[];
  activeCases: CaseRefView[];
}

export interface RootCausePathView {
  id: string;
  rank: number;
  confidence: number;
  status: "candidate" | "confirmed" | "rejected";
  nodes: GraphEntityView[];
  edges: GraphEdgeView[];
  supportingEvidence: EvidenceRefView[];
  contradictingEvidence: EvidenceRefView[];
  blockingUnknowns: string[];
}

export interface AssetMatchView {
  id: string;
  type: "runbook" | "workflow" | "experience_pack" | "memory" | "eval_case" | "historical_case";
  name: string;
  version?: string;
  status: "draft" | "approved" | "published" | "deprecated" | "disabled";
  risk: "low" | "medium" | "high" | "critical";
  matchReason: string;
  compatibility: string[];
  disabledReasons: string[];
  lastOutcome?: string;
}

export interface OpsGraphPatchView {
  id: string;
  sourceCaseId?: string;
  sourceExperiencePackId?: string;
  status: "candidate" | "approved" | "rejected" | "applied";
  risk: "low" | "medium" | "high" | "critical";
  operations: OpsGraphPatchOperationView[];
  evidenceRefs: EvidenceRefView[];
  reviewer?: string;
  reviewedAt?: string;
}
```

## 13. API 契约

页面优先使用模块设计中的 API：

```text
GET    /api/v1/opsgraph/entities/{id}
GET    /api/v1/opsgraph/entities/{id}/neighbors
POST   /api/v1/opsgraph/query
POST   /api/v1/opsgraph/impact
POST   /api/v1/opsgraph/root-cause-paths
POST   /api/v1/opsgraph/experience-match
POST   /api/v1/opsgraph/patches
GET    /api/v1/opsgraph/patches/{patch_id}
POST   /api/v1/opsgraph/patches/{patch_id}/review
```

为了兼容当前前端和后端初版，API client 可以同时支持：

```text
POST /api/v1/opsgraph/lookup
GET  /api/v1/opsgraph/entities/{id}/neighborhood
GET  /api/v1/opsgraph/entities/{id}/business-impact
```

兼容逻辑必须封装在 `web/src/api/opsgraph.ts`，页面组件只消费统一 view-model。

## 14. 状态与数据流

```text
Route query / User input
  -> OpsGraph API client
  -> normalizeOpsGraph* view-model
  -> Page state
  -> Tabs
  -> Detail Drawer
```

要求：

- 页面不直接调用裸 `fetch`。
- 页面不创建私有 WebSocket 或 SSE。
- URL query 是当前实体、tab、时间窗口和来源过滤的事实来源。
- Search、Impact、Root Cause、Asset、Patch Review 的 loading 和 error 状态相互隔离。
- 同一个 entity id 的实体详情应在当前页面内复用，避免重复请求和不一致展示。

## 15. 错误、空态与加载态

错误态：

- 数据源不可用：显示 source 和失败原因，不阻塞其他 source 的数据。
- 权限不足：显示受限证据，不显示敏感内容。
- 实体不存在：显示 entity id 和重新搜索入口。
- 图谱查询超时：保留已有数据，显示本次查询失败。
- patch 审核失败：保留用户审核意见，显示后端错误。

空态：

- 无搜索结果：提示调整实体类型、环境或来源过滤。
- 无业务影响：说明当前实体没有已确认业务边，提供查看低置信候选边入口。
- 无根因路径：提示补充 trace、ERP job 或 case 证据。
- 无匹配资产：提示可从 case 复盘或经验包生成资产候选。
- 无待审 patch：显示最近已处理 patch 摘要。

加载态：

- Header 和 Context Bar 不随 tab 切换闪烁。
- 图谱主区使用骨架布局，避免节点加载造成布局跳动。
- Detail Drawer 在加载新实体时保留旧实体标题并显示局部 loading。

## 16. 安全与治理显示规则

- ERP 租户、订单、用户、请求 payload、cookie、token、password 等敏感字段不能出现在 DOM。
- `redactionStatus=restricted` 的证据不能通过 tooltip、title、aria-label 泄露正文。
- 低置信边必须在 UI 上清楚标识，不能作为自动修复建议的唯一依据。
- Root Cause Path 只展示候选路径，不能在未确认前显示为确定根因。
- Patch Review 必须显示来源 case 或 experience pack，不能接受无来源补丁。
- 高风险 patch 审核前必须展示受影响业务能力、技术实体和证据质量。
- 所有执行入口只跳转或创建 Governed Action Plane 的 ActionProposal，不能绕过审批和主机锁。

## 17. 验收标准

- `/opsgraph` 支持 Entity Search、Impact Map、Root Cause Path、Asset Map、Patch Review 五个核心 tabs。
- 从 Coroot service、ERP module、ERP job、Debug trace、case 都能跳入对应 OpsGraph 查询上下文。
- 每条图谱边都能展开查看 source、confidence、updatedAt、validFrom、validTo 和 evidenceRefs。
- Impact Map 能从技术实体展示 ERP module、BusinessCapability、tenant impact、SLO 和 active case。
- Root Cause Path 能展示候选路径、支持证据、反证、缺失证据和推荐下一步。
- Asset Map 能展示 Runbook、Workflow、Experience Pack、Memory、EvalCase 和 Historical Case 的匹配原因和禁用条件。
- Patch Review 能查看、批准、拒绝 OpsGraphPatch，且审批意见和证据引用必填。
- 页面请求都经过 `web/src/api/opsgraph.ts`，不在 `OpsGraphPage.tsx` 中保留裸 `fetch`。
- ERP 敏感数据默认脱敏，权限不足证据不泄露正文。
- Case、ERP Health、Debug Trace、Experience Pack Review 能复用 OpsGraph 摘要组件或跳转到 `/opsgraph`。
