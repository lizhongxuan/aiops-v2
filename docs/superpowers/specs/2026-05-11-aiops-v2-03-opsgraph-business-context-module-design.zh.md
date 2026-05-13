# aiops-v2 OpsGraph & ERP Business Context 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)
前端设计：[2026-05-11-aiops-v2-03a-opsgraph-business-context-frontend-design.zh.md](2026-05-11-aiops-v2-03a-opsgraph-business-context-frontend-design.zh.md)
实施清单：[2026-05-11-aiops-v2-03b-opsgraph-business-context-frontend-todo.zh.md](2026-05-11-aiops-v2-03b-opsgraph-business-context-frontend-todo.zh.md)

## 0. 场景边界

本模块本阶段提供两个闭环需要的最小图谱路径：`BrowserDebugAction -> APIRoute -> Service -> MiddlewareResource`，以及 `MiddlewareResource -> Service -> BusinessCapability`。它用于解释慢请求影响路径、PG 影响业务和经验适配环境。

全 ERP 图谱、CMDB 化资产管理、复杂图谱补丁审核、资产地图页面和环境变体推理不作为本次场景改造的前置条件。OpsGraph 是上下文查询层，不是新的事实源，也不因为一次 AI 推理自动改图。

## 1. 模块定位

OpsGraph 是 `aiops-v2` 的运维推理图谱。它把 ERP 业务能力、服务、接口、前端动作、中间件、主机、Pod、SLO、变更、Runbook、Workflow、经验包和 case 连接起来，回答“这个技术异常影响什么业务”和“这个业务异常可能由哪些技术路径导致”。

它不是传统 CMDB 的镜像，也不是人工维护的静态拓扑。OpsGraph 是面向事故推理、业务影响分析、经验匹配和修复验证的上下文层。

## 2. 设计目标

- 支持从业务到技术的影响链路：ERP 模块 -> 业务能力 -> 服务 -> API -> 中间件 -> 主机/Pod。
- 支持从技术到业务的反向定位：Coroot service、trace span、DB、MQ、Redis、host 能反推业务影响。
- 支持用户侧调试链路：页面动作 -> API route -> service -> middleware -> trace span。
- 支持经验资产关联：实体能关联 Runbook、Workflow、Experience Pack、Memory 和 Eval case。
- 支持图谱补丁：事故复盘和经验包审核后可以产生待审核 OpsGraph patch。

## 3. 核心节点

```text
BusinessDomain
BusinessCapability
ERPModule
ERPJob
Tenant
FrontendPage
UserAction
APIRoute
Service
Deployment
RuntimeResource
Host
Pod
MiddlewareResource
Database
Queue
Cache
SLO
TraceSpanSignature
ChangeRecord
RunbookVersion
WorkflowVersion
ExperiencePack
Case
```

## 4. 核心边

```text
ERPModule -> BusinessCapability: implements
BusinessCapability -> Service: served_by
FrontendPage -> UserAction: contains
UserAction -> APIRoute: calls
APIRoute -> Service: handled_by
Service -> MiddlewareResource: depends_on
Service -> RuntimeResource: runs_on
RuntimeResource -> Host: scheduled_on
Service -> SLO: measured_by
TraceSpanSignature -> Service: observed_in
TraceSpanSignature -> MiddlewareResource: waits_on
Service -> ChangeRecord: recently_changed_by
Entity -> RunbookVersion: has_runbook
Entity -> WorkflowVersion: has_workflow
Entity -> ExperiencePack: has_experience
Case -> Entity: affected
```

所有边都必须有来源、置信度和更新时间：

```text
GraphEdge
  id
  fromId
  toId
  type
  source: coroot | erp | trace | workflow | user | experience | import | manual
  confidence
  validFrom
  validTo
  evidenceRefs
```

## 5. 数据来源

### ERP

ERP 提供：

- 模块、业务流程、租户、任务、订单、SLA、业务指标。
- 业务错误码与服务/API 的映射。
- 业务恢复验证接口，例如任务恢复、订单状态回填、队列积压恢复。

ERP 数据进入 OpsGraph 时必须做脱敏。租户、订单、用户等对象默认保存业务引用和 hash，不保存敏感原文。

### Coroot

Coroot 提供：

- project、application、service、topology、SLO、RCA、metrics、timeline。
- service 依赖和资源瓶颈。

Coroot 的 topology 作为技术依赖证据，不直接覆盖人工确认的业务关系。

### Trace

Trace 提供：

- frontend action、api route、service path、DB span、cache span、MQ span、slow span、error span。

Trace 不长期保存原始 payload。OpsGraph 只保存 span signature 和资源关系。

### Runner / Host Agent

Runner 和 agent 提供：

- host labels、agent capabilities、runtime、namespace、service deployment。
- workflow 执行时发现的目标主机组和实际资源。

### Experience Pack

经验包提供：

- 已审核处置模式和适用实体。
- 禁用条件和环境变体。
- 修复后确认的新关系或不兼容边。

## 6. 图谱查询能力

业务影响查询：

```text
Given Service checkout-api
Return BusinessCapability, ERPModule, TenantImpact, SLO, active cases
```

根因路径查询：

```text
Given ERP job failure
Return likely path:
ERPJob -> BusinessCapability -> Service -> APIRoute -> MiddlewareResource -> TraceSpanSignature
```

经验匹配查询：

```text
Given Case symptoms + affected entities
Return approved runbooks, workflows, experience packs, memory snippets
```

调试链路查询：

```text
Given trace id / frontend action
Return FrontendPage -> UserAction -> APIRoute -> Service -> MiddlewareResource -> slow span
```

## 7. API 草案

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

## 8. 图谱补丁

事故复盘和经验包审核可以生成 `OpsGraphPatch`：

```text
OpsGraphPatch
  id
  sourceCaseId
  sourceExperiencePackId
  operations:
    - add_node
    - add_edge
    - update_edge_confidence
    - mark_incompatible
  risk
  reviewer
  status: candidate | approved | rejected | applied
```

补丁必须人工审核。系统不能因为一次 AI 推理就永久修改业务图谱。

## 9. ERP 闭环

ERP 闭环包含四步：

1. 发现：ERP 业务异常创建或更新 case。
2. 定位：OpsGraph 把 ERP 异常映射到服务、API、中间件和主机。
3. 修复：Governed Action Plane 执行受治理动作。
4. 验证：ERP 指标或业务接口确认订单、任务、SLA 恢复。

只有技术指标恢复但 ERP 业务未恢复时，case 不能直接关闭，只能进入 `mitigated` 或 `partially_resolved`。

## 10. 前端体验

OpsGraph 在 UI 中提供三种视图：

- Impact Map：从事故看到业务影响。
- Root Cause Path：从业务异常看到技术路径。
- Asset Map：从服务或中间件看到可用 Runbook、Workflow、经验包和历史 case。

图谱视图必须可解释：每条边显示来源、置信度、最近更新时间和证据引用。

## 11. 安全与治理

- ERP 租户、订单、用户数据默认脱敏。
- 图谱边按来源和置信度展示，不把推断关系伪装成事实。
- 低置信度边不能作为高风险自动动作的唯一依据。
- OpsGraph patch 必须审核后才能应用。

## 12. 验收标准

- Coroot service 能映射到 OpsGraph service，并关联 SLO、依赖和 case。
- ERP module 能映射到 BusinessCapability，并能反查受影响服务。
- Debug Trace 能生成 FrontendPage -> UserAction -> APIRoute -> Service -> MiddlewareResource 路径。
- 中间件异常能反推出受影响业务能力。
- AI Reasoning Plane 能使用 OpsGraph 查询结果解释根因路径和业务影响。
- Experience Pack 审核后能生成 OpsGraph patch candidate。
