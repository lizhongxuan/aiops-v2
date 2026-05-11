# aiops-v2 Middleware Repair 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-08-middleware-repair-module-design.zh.md](2026-05-11-aiops-v2-08-middleware-repair-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-08b-middleware-repair-frontend-todo.zh.md](2026-05-11-aiops-v2-08b-middleware-repair-frontend-todo.zh.md)

## 1. 页面目标

Middleware Repair 的前端不是“数据库管理页面”，也不是把 DBA 命令包装成按钮。它是中间件异常的受治理修复工作台，负责把“帮我修复 xxx 的 PG 集群”这类自然语言意图变成可审计的 `middleware_repair` case、只读诊断、经验匹配、RepairPlan、审批执行、恢复验证和经验沉淀。

页面要让用户完成：

- 识别 PG、Redis、MQ、Elasticsearch、Kafka、MySQL 等中间件资源、角色、拓扑、业务影响和安全画像。
- 对每个修复请求先做只读诊断和 RCA，不把自然语言请求直接映射为重启、failover、删数据或配置写入。
- 在 PG 场景中清楚展示 primary、standby、replica、proxy、replication lag、WAL、slot、锁等待、连接、磁盘、备份、PITR 和 failover 风险。
- 命中已审核 SkillCard、Debug RCA、RepairPlan、Runbook 或 Workflow metadata 时，展示来源、适用环境、成功率、最近失败、禁用条件、权限、风险、回滚和验证项。
- 未命中经验时生成 RepairPlan draft，并让用户确认假设、诊断步骤、修复步骤、风险、审批、回滚、验证和禁用条件。
- 高风险和破坏性步骤只能生成 ActionProposal 或 Workflow run，必须走 RBAC、HostLease、审批、ActionToken 和 Verification。
- 展示 RecoveryAttempt 的每步结果、失败点、回滚结果、验证记录和下一步建议。
- 修复成功或失败后生成经验包候选，保留环境画像、证据链、失败点、验证结果和禁用条件。
- 从现有 AI Chat、Incident、OpsGraph、Observability、Execution、Verification、Experience Pack 页面跳入同一个修复 case；AI Chat 页面本身不新增专用修复入口。

设计原则：

- 第一屏显示安全状态和诊断覆盖率，不先展示“执行修复”按钮。
- 默认只读诊断，任何写动作都必须经由 RepairPlan -> ActionProposal / Workflow -> Verification。
- 修复计划和执行计划分离：RepairPlan 描述怎么修，Execution Fabric 负责受治理执行。
- 经验复用不能绕过风险等级、审批、回滚和验证；未审核经验不能进入自动推荐。
- 中间件页面只展示脱敏摘要、SQL fingerprint、query signature、topic/consumer 摘要，不展示参数值、密码、token、连接串明文。
- PG、Redis、MQ 等不同中间件有专用健康基线，但共享相同的 case、plan、attempt、verification、experience 数据模型。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/IncidentWorkbenchPage.tsx`：已有 case 工作台初版，可承载 `middleware_repair` case。
- `web/src/pages/OpsGraphPage.tsx`：已有业务、服务、中间件、主机等图谱上下文页面。
- `web/src/pages/CorootOverviewPage.tsx` 与 `web/src/api/coroot.js`：已有 Coroot 服务、RCA、拓扑和指标接入基础。
- `web/src/pages/RunnerStudioPage.tsx`、`web/src/components/runner/*`：已有 Workflow 画布、运行状态、审批、变量、回调和事件聚合基础。
- `web/src/pages/ApprovalManagementPage.tsx`：已有审批和授权管理入口。
- `web/src/pages/RunbookCatalogPage.tsx`、`RunbookDetailPage.tsx`：已有 Runbook 目录和验证项展示初版。
- `web/src/pages/ExperiencePacksPage.tsx`：已有经验包库静态页面，可扩展为中间件修复候选入口。
- `web/src/pages/PostmortemPage.tsx`：已有复盘草稿页面。
- `docs/superpowers/specs/2026-05-11-aiops-v2-05a-ai-reasoning-prompt-trace-frontend-design.zh.md`：已定义 AI 输出 `repair_plan_draft` 应路由到 Middleware Repair。
- `docs/superpowers/specs/2026-05-11-aiops-v2-07a-verification-recovery-frontend-design.zh.md`：已定义 RepairPlan 的 VerificationSpec、Evidence Compare、Rollback Verification 和 Recovery Decision。
- `docs/superpowers/specs/2026-05-11-aiops-v2-10-experience-packs-design.zh.md`：已定义 RepairPlan、RecoveryAttempt、Experience candidate 和 Activation Index。

主要不足：

- 当前没有独立 Middleware Repair 导航、资源目录、修复 case 列表和 RepairPlan 工作台。
- AI Chat 后端产生的 `repair_plan_draft` 没有稳定落点；前端只承接后端创建的 `middleware_repair` case，不改 Chat 页面。
- Case 页面还不能专门表达中间件拓扑、角色、复制、锁、连接、磁盘、WAL、备份和 failover 风险。
- 缺少中间件只读诊断证据的结构化面板和“证据不足”阻断状态。
- 缺少已审核经验匹配页面，用户无法判断经验是否适用当前环境。
- 缺少 RepairPlan Designer，无法审核 assumptions、diagnosticSteps、remediationSteps、rollbackPlan、verificationSpec 和 disabledConditions。
- 缺少 RecoveryAttempt 详情，无法追踪每步执行结果、失败点、回滚结果和 VerificationRecord。
- ExperiencePacksPage 仍偏静态展示，缺少从修复过程生成候选的入口。

## 3. 路由与信息架构

建议新增统一入口：

```text
/middleware
/middleware/resources
/middleware/resources/:resourceId
/middleware/repairs
/middleware/repairs/:caseId
/middleware/repair-plans/:planId
/middleware/recovery-attempts/:attemptId
```

保留并增强嵌入入口：

```text
/incidents/:caseId?type=middleware_repair
/ai-reasoning?outputType=repair_plan_draft
/opsgraph/entities/:entityId?type=MiddlewareResource
/observability?entityType=middleware
/execution/runs/:runId?source=repair_plan
/verification/cases/:caseId
/experience-packs?source=middleware_repair
```

路由语义：

- `/middleware`：Middleware Repair 总览。展示资源健康、活跃修复 case、诊断阻塞、待确认 RepairPlan、运行中 RecoveryAttempt 和经验候选。
- `/middleware/resources`：中间件资源目录。筛选 PG、Redis、MQ、Kafka、Elasticsearch、MySQL 等资源。
- `/middleware/resources/:resourceId`：资源详情。展示拓扑、角色、安全画像、业务影响、只读诊断、历史修复和经验。
- `/middleware/repairs`：修复 case 队列。查看 `middleware_repair` case 的诊断、计划、审批、执行、验证和沉淀状态。
- `/middleware/repairs/:caseId`：修复 case 工作台。围绕单个中间件异常进行 RCA、经验匹配、RepairPlan、执行和验证。
- `/middleware/repair-plans/:planId`：RepairPlan 详情和审核。审查步骤、风险、回滚、验证、禁用条件和确认状态。
- `/middleware/recovery-attempts/:attemptId`：RecoveryAttempt 详情。查看执行步骤、ActionProposal、WorkflowRun、失败点、回滚和验证。

MVP 可以先把 `/middleware/repairs/:caseId` 做成核心页面，资源详情和 RepairPlan 详情通过 Drawer 承载；但组件、API 和文档按终态拆分。

主导航分区：

| 页面 | 目标 |
| --- | --- |
| Middleware Overview | 总览资源风险、活跃修复、待诊断、待确认、运行中、验证失败和经验候选 |
| Resource Inventory | 浏览中间件资源、拓扑、角色、安全画像和业务影响 |
| Resource Detail | 单资源诊断基线、历史事件、经验匹配和修复入口 |
| Repair Case Workspace | 单次修复 case 的 RCA、证据、经验、计划、执行、验证闭环 |
| RepairPlan Review | 审核 assumptions、diagnostic/remediation steps、风险、回滚、验证和禁用条件 |
| RecoveryAttempt Detail | 跟踪执行步骤、失败点、回滚、验证和审计 |
| Middleware Experience | 从成功或失败修复生成经验包候选 |

## 4. 页面一：Middleware Overview

Middleware Overview 回答“当前中间件修复面是否安全、哪些资源需要处理、哪些修复被阻塞”。

### 4.1 布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: Middleware Repair / 环境 / 时间窗口 / 新建修复 / 刷新       │
├────────────────────────────────────────────────────────────┤
│ Safety Strip: Readonly Probe / Backup / Replication / Approval / Verification │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: Active Cases / Evidence Gaps / Awaiting Confirm / Running / Failed │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Detail Drawer           │
│ Queue / Resources / Plans /   │ Resource / Case / Plan /     │
│ Attempts / Experience         │ Risk / Evidence / Audit      │
└──────────────────────────────┴─────────────────────────────┘
```

桌面端右侧 Drawer 宽度 400-480px。窄屏时 Drawer 变成底部 Sheet。

### 4.2 Header

显示：

- 当前环境。
- 当前时间窗口。
- 当前用户角色：viewer、operator、DBA、resource owner、approver、admin。
- 中间件探针健康状态。
- 最近一次数据刷新时间。

动作：

- 新建修复 case。
- 从 AI Draft 打开。
- 刷新。
- 查看只读诊断策略。
- 打开经验匹配规则。

### 4.3 Safety Strip

Safety Strip 展示中间件修复安全前置条件：

| 项 | 状态 | 说明 |
| --- | --- | --- |
| Readonly Probe | ready / partial / unavailable | 是否能采集只读诊断证据 |
| Backup / PITR | healthy / stale / unavailable / unknown | 最近备份和恢复能力 |
| Replication | healthy / lagging / broken / unknown | 复制状态和延迟 |
| Role Map | complete / partial / unknown | primary、standby、replica、proxy 是否识别 |
| HostLease | available / conflict / unavailable | 资源锁是否可获取 |
| Approval | configured / missing / no_approver | 高风险动作审批是否配置 |
| Verification | ready / partial / unavailable | 中间件和业务验证能力 |

当 Backup / PITR 为 `unavailable` 或 `unknown` 时，页面必须阻断 destructive 修复动作，只允许继续只读诊断或生成需要人工处理的计划。

### 4.4 Summary Strip

指标：

- 活跃 middleware_repair case。
- 证据不足 case。
- 待确认 RepairPlan。
- 命中经验但禁用条件不满足的 case。
- 运行中 RecoveryAttempt。
- 验证失败。
- 回滚中或回滚失败。
- 待生成经验候选。

## 5. Resource Inventory

Resource Inventory 回答“平台识别了哪些中间件资源，它们是否具备修复前置条件”。

### 5.1 筛选

- kind：postgres、redis、mq、kafka、elasticsearch、mysql、other。
- environment。
- clusterName。
- criticality。
- safetyProfile。
- role completeness。
- backup status。
- replication status。
- business capability。
- owner。
- has active repair。
- has approved experience。

筛选条件写入 URL query。

### 5.2 表格列

| 列 | 内容 |
| --- | --- |
| Resource | id、kind、clusterName、environment |
| Topology | roleMap completeness、node count、proxy |
| Safety | backup、PITR、replication、disk/WAL、HostLease |
| Business | affected services、business capabilities、tenant criticality |
| Current Signal | Coroot、trace、middleware probe、alert summary |
| Experience | matched approved plans、success rate、recent failures |
| Active Work | active cases、running attempts、pending verifications |
| Action | 打开详情、创建 repair case、运行只读诊断、匹配经验 |

### 5.3 资源行 Drawer

展示：

- MiddlewareResource 摘要。
- OpsGraph 关联实体。
- 业务影响。
- 当前安全画像。
- 最近告警和 Coroot RCA。
- 最近 Debug Trace / span signature。
- 最近修复 case。
- 已审核经验。

## 6. Resource Detail

Resource Detail 回答“这个中间件资源当前处于什么状态，是否可以安全修复”。

### 6.1 Header

显示：

- kind。
- clusterName。
- environment。
- criticality。
- owner。
- safetyProfile。
- current health。
- active repair case。
- approved experience count。

动作：

- 创建修复 case。
- 运行只读诊断。
- 匹配经验。
- 打开 OpsGraph。
- 打开 Coroot。
- 打开 Verification。

### 6.2 Tabs

| Tab | 目标 |
| --- | --- |
| Topology | 节点、角色、proxy、依赖服务、业务路径 |
| Safety Profile | 备份、PITR、复制、磁盘、WAL、权限、HostLease |
| Diagnostics | 只读诊断结果和证据质量 |
| Business Impact | 业务能力、租户、SLO、ERP 任务影响 |
| Experience | 已审核经验、适用条件、成功率、禁用条件 |
| History | 历史 repair case、RecoveryAttempt、验证结果 |

### 6.3 拓扑展示

中间件拓扑不要做成复杂自由图，默认用固定层级：

```text
BusinessCapability / ERPJob
  -> Service / APIRoute
  -> MiddlewareResource
  -> ClusterRole: primary / standby / replica / proxy / broker / shard
  -> Host / Pod / Volume
```

节点要显示：

- role。
- health。
- replication / lag。
- read/write routing。
- storage pressure。
- owner。
- last evidence time。

## 7. Repair Case Workspace

Repair Case Workspace 回答“这次中间件异常的证据、根因、修复计划和恢复状态是什么”。

### 7.1 Header

显示：

- caseId。
- resource。
- kind。
- environment。
- case status。
- repair phase。
- risk level。
- primary hypothesis。
- evidence coverage。
- selected experience。
- active RepairPlan。
- RecoveryStatus。

Repair phase：

```text
created
  -> identifying_resource
  -> collecting_evidence
  -> diagnosing
  -> matching_experience
  -> planning
  -> awaiting_confirmation
  -> awaiting_approval
  -> executing
  -> verifying
  -> succeeded
  -> failed
  -> rolled_back
  -> experience_candidate_ready
```

### 7.2 Status Rail

状态节点展示：

- 当前阶段。
- 进入时间。
- 触发事件。
- 证据数量。
- 阻塞项。
- 操作者或系统 actor。

如果用户从现有 AI Chat 说“帮我修复 xxx 的 PG 集群”，由后端创建或绑定 `middleware_repair` case；页面必须停在 `collecting_evidence` 或 `diagnosing`，不能直接进入 `executing`。

### 7.3 Tabs

| Tab | 目标 |
| --- | --- |
| Overview | 修复摘要、阶段、阻塞项、下一步 |
| Resource | 中间件资源、拓扑、角色和安全画像 |
| Diagnostics | 只读诊断、证据覆盖、RCA 假设 |
| Experience Match | 已审核经验匹配和禁用条件 |
| RepairPlan | 修复计划草稿、确认、审批和版本 |
| Execution | ActionProposal、WorkflowRun、HostLease、步骤结果 |
| Verification | VerificationSpec、VerificationRecord、恢复状态 |
| Rollback | 回滚条件、回滚计划、回滚验证 |
| Experience Candidate | 成功或失败经验沉淀 |
| Timeline / Audit | append-only 事件和审计 |

## 8. Diagnostics 与 RCA

Diagnostics 回答“证据是否足够支持修复，根因假设是什么”。

### 8.1 诊断覆盖率

每个 case 展示 Evidence Coverage：

| 分类 | 状态 | 说明 |
| --- | --- | --- |
| Topology | passed / missing / partial | 集群角色和节点识别是否完整 |
| Replication | passed / failed / missing | 复制、lag、slot、WAL、sender/receiver |
| Locks | passed / failed / missing | blocking query、blocked query、lock type、duration |
| Connections | passed / failed / missing | active、idle、idle in transaction、max connections |
| Storage | passed / failed / missing | disk、WAL、tablespace、volume |
| Performance | passed / failed / missing | slow query、CPU、IO、buffer、checkpoint |
| Backup | healthy / stale / unavailable / missing | 最近备份、PITR、恢复窗口 |
| Business Impact | passed / missing / partial | 业务能力、ERP、SLO、租户影响 |
| Trace Link | passed / missing / partial | trace slow span、SQL/cache/MQ signature |

缺少 required baseline 时，页面只能显示“证据不足，只能继续只读诊断”，不能显示可执行修复确认按钮。

### 8.2 RCA 假设列表

表格列：

| 列 | 内容 |
| --- | --- |
| Hypothesis | 根因假设摘要 |
| Confidence | 置信度和证据数量 |
| Supporting Evidence | 支持证据 |
| Contradicting Evidence | 反证 |
| Blast Radius | 影响服务、业务能力、租户 |
| Safety Risk | 数据风险、可回滚性、备份状态 |
| Next Diagnostic | 下一步只读诊断 |

假设状态：

- candidate。
- supported。
- contradicted。
- needs_more_evidence。
- confirmed_by_human。

### 8.3 证据详情

证据详情展示：

- EvidenceRef id。
- source：coroot、trace_backend、middleware_probe、host、k8s、erp、manual。
- timeWindow。
- entityRefs。
- quality。
- redactionStatus。
- rawRef。
- 可用于 RCA / Prompt / Automation / Verification 的资格。

SQL、cache、MQ operation 只展示 signature，不展示参数值。

## 9. PG 专用诊断面板

PG 是第一优先级中间件，页面必须有专用基线。

### 9.1 PG Cluster Baseline

展示：

- primary、standby、replica、proxy。
- replication lag。
- replication slot。
- WAL usage。
- sender / receiver。
- blocking query / blocked query。
- lock type。
- lock duration。
- active / idle / idle in transaction。
- max connections。
- disk usage。
- tablespace。
- slow query fingerprint。
- CPU、IO、buffer、checkpoint。
- latest backup。
- backup validity。
- PITR capability。
- failover data loss window。
- application connection / route。

### 9.2 PG 风险守卫

阻断条件：

- 没有最近备份或 PITR 能力未知。
- replication lag 超出 RepairPlan 限制。
- failover data loss window 无法接受。
- blocker 是 migration process。
- blocker query identity 不明确。
- 当前有高风险变更窗口。
- 业务影响路径不明确。
- required DBA approval 不存在。

阻断条件满足时，高风险动作按钮全部禁用，只允许继续诊断、请求人工确认或生成 manual follow-up。

### 9.3 PG 修复类型

常见修复类型以计划步骤展示，不做裸按钮：

| 类型 | 风险 | UI 行为 |
| --- | --- | --- |
| 只读诊断 | readonly | 可直接运行只读 probe |
| terminate idle-in-transaction blocker | high | 必须绑定 confirmed blocker、DBA 审批、回滚/补救和验证 |
| reload 参数 | medium/high | 需要参数 diff、影响说明、审批、验证 |
| 调整连接池 | medium | 需要服务 owner 审批和业务验证 |
| failover | high/destructive | 需要备份/PITR、lag、路由、审批、回滚和观察窗口 |
| 重建副本 | destructive | 默认不自动化，除非 break-glass 和强审计 |
| 清理 WAL | destructive | 默认禁止自动化，除非强制 break-glass 和 DBA 审批 |

## 10. Redis、MQ、Kafka、Elasticsearch、MySQL 面板

### 10.1 Redis

诊断项：

- role：master、replica、sentinel、cluster shard。
- memory usage。
- evictions。
- latency。
- slowlog signature。
- replication lag。
- connected clients。
- blocked clients。
- keyspace summary。
- persistence：RDB/AOF。
- failover state。

风险守卫：

- maxmemory policy 不明确。
- persistence 状态未知。
- failover quorum 不足。
- replica lag 高。
- 业务 key pattern 无法脱敏。

### 10.2 MQ / Kafka

诊断项：

- broker state。
- topic / queue health。
- consumer group lag。
- partition imbalance。
- ISR / replica state。
- disk usage。
- producer error。
- consumer error。
- throughput。

风险守卫：

- 数据保留策略未知。
- consumer group ownership 不明确。
- partition movement 风险未评估。
- broker disk pressure 高且无回滚方案。

### 10.3 Elasticsearch

诊断项：

- cluster health。
- shard allocation。
- unassigned shards。
- disk watermark。
- JVM heap。
- indexing/search latency。
- slow query signature。
- snapshot status。

风险守卫：

- snapshot 不可用。
- shard movement 风险过高。
- disk watermark 持续恶化。
- index operation 影响范围不明确。

### 10.4 MySQL

诊断项：

- primary / replica。
- replication lag。
- binlog。
- slow query fingerprint。
- lock wait。
- connection saturation。
- disk usage。
- backup / PITR。

风险守卫：

- binlog / backup 状态未知。
- replication topology 不完整。
- DDL / migration 相关锁未识别。

## 11. Experience Match

Experience Match 回答“是否存在已审核且适用于当前环境的处置经验”。

### 11.1 匹配来源

匹配来源：

- approved ExperiencePack。
- approved Capsule。
- approved RepairPlan。
- published Runbook。
- published Workflow。
- historical case。

未审核 candidate 不进入匹配结果。候选经验只能在 Experience Candidate 页面查看。

### 11.2 匹配列表

| 列 | 内容 |
| --- | --- |
| Experience | title、type、version、source case |
| Applicability | environment match、resource kind、topology match、business scope |
| Fitness | success rate、activation count、recent failures |
| Risk | max risk、approval、rollback、verification |
| Disabled Conditions | 当前是否触发禁用条件 |
| Evidence | 需要的证据和当前覆盖情况 |
| Action | 应用只读诊断、创建 RepairPlan、打开详情 |

命中但禁用条件触发时：

- 可以复用只读诊断步骤。
- 不能复用 remediationSteps。
- 必须显示禁用原因。

### 11.3 Experience Detail Drawer

展示：

- 来源 case。
- 适用环境。
- 成功率。
- 最近失败。
- 禁用条件。
- 所需权限。
- 风险和审批。
- 回滚。
- 验证项。
- lineage。
- reviewer。

## 12. RepairPlan Designer

RepairPlan Designer 回答“当前计划是否足够具体、可审批、可回滚、可验证”。

### 12.1 Plan Header

显示：

- plan id。
- caseId。
- middlewareResourceId。
- source：experience_pack、capsule、runbook、generated。
- status：draft、awaiting_confirmation、approved、running、succeeded、failed。
- riskLevel。
- approvalPolicy。
- selected experience。
- confirmation state。

### 12.2 Plan Sections

| 区域 | 内容 |
| --- | --- |
| Assumptions | 根因假设、环境假设、证据假设 |
| Diagnostic Steps | 只读诊断步骤、所需工具、预期输出、证据引用 |
| Remediation Steps | 修复步骤、目标资源、风险、输入、审批、ActionProposal / Workflow mapping |
| Risk Review | 数据风险、角色风险、复制风险、备份风险、业务影响、禁用条件 |
| Approval Policy | DBA、resource owner、service owner、break-glass 要求 |
| Rollback Plan | 回滚步骤、触发条件、回滚验证 |
| Verification Spec | 中间件健康、业务恢复、trace 对比和人工确认 |
| Disabled Conditions | 当前触发状态和阻断说明 |
| Diff | 来自经验或 AI draft 的 plan diff |

### 12.3 Step 表格

| 列 | 内容 |
| --- | --- |
| Step | 序号、名称、类型：diagnostic / remediation / rollback / verification |
| Target | resource、role、host/pod、scope |
| Risk | readonly、low、medium、high、destructive |
| Evidence | required evidence refs |
| Governance | approval、HostLease、ActionToken、policy |
| Inputs | 脱敏输入摘要、normalizedInputHash |
| Output | expected output、output variables |
| Blockers | disabled condition、missing evidence、approval missing |

### 12.4 确认规则

用户确认前必须满足：

- required diagnostic evidence 完成。
- assumptions 无 unresolved blocker。
- disabledConditions 未触发，或明确进入 manual follow-up。
- high/destructive steps 有 rollbackPlan 和 verificationSpec。
- backup/PITR 状态满足风险要求。
- approvalPolicy 可执行。
- remediationSteps 均能映射为 ActionProposal 或 Workflow step。

确认后 plan 进入 `awaiting_approval` 或 `approved`，不直接执行高风险动作。

## 13. Execution 与 RecoveryAttempt

Execution Tab 回答“RepairPlan 如何被受治理执行，以及每步执行到哪里”。

### 13.1 Execution Summary

展示：

- RepairPlan。
- RecoveryAttempt。
- WorkflowRun。
- ActionProposal refs。
- HostLease。
- Approval。
- current step。
- stepResults。
- failedPoint。
- rollbackResult。
- verificationRefs。

### 13.2 Step Result

每步展示：

- step id。
- step type。
- target。
- status。
- startedAt、finishedAt。
- actor。
- toolName。
- normalizedInputHash。
- output summary。
- evidenceRefs。
- error summary。
- next recommendation。

长输出、敏感输出、SecretRef 和连接信息只展示摘要和引用。

### 13.3 失败处理

失败时页面必须显示：

- failedPoint。
- failed step。
- failure category：evidence_missing、approval_denied、host_lock_conflict、tool_failed、verification_failed、rollback_failed、source_unavailable。
- impact。
- rollback availability。
- nextRecommendation。

## 14. Verification、Rollback 与 Recovery

Middleware Repair 的 Verification 不重新实现 07 模块，而是嵌入其组件。

### 14.1 Verification

展示：

- VerificationSpec。
- VerificationRecord。
- middleware checks。
- business checks。
- trace checks。
- observation window。
- manual DBA confirmation。
- RecoveryStatus。

PG 默认验证项：

- replication lag 降到阈值内。
- lock wait 恢复。
- connection saturation 恢复。
- disk/WAL 恢复到安全阈值。
- checkout 或受影响业务 p95 latency 恢复。
- ERP order / job 指标恢复。

### 14.2 Rollback

展示：

- rollback criteria。
- rollback plan。
- rollback ActionProposal / WorkflowRun。
- rollback result。
- rollback VerificationRecord。
- data consistency check。
- business not-worse check。
- HostLease / approval closure。

回滚失败时，case 不能进入 recovered；必须进入 failed 或 requires_manual_followup。

## 15. Experience Candidate

Experience Candidate 回答“这次修复能沉淀成什么资产”。

### 15.1 候选内容

成功或失败都可以生成：

- Gene candidate。
- Capsule candidate。
- RepairPlan candidate。
- Runbook draft。
- Workflow graph draft。
- Eval case。
- Memory candidate。
- OpsGraph patch。

候选必须包含：

- MiddlewareResource。
- environment profile。
- problem signature。
- RCA hypothesis。
- diagnostic evidence。
- RepairPlan。
- RecoveryAttempt。
- failedPoint。
- rollbackResult。
- Verification outcome。
- disabledConditions。
- reviewer requirements。

### 15.2 页面行为

- “生成经验候选”只创建 candidate，不发布。
- candidate 不进入 AI Chat、RepairPlan matcher 或 Activation Index。
- 跳转 ExperiencePacksPage 时必须带 source repair case。
- 如果修复失败，候选默认强调禁用条件、失败点和需要人工 follow-up。

## 16. 前端数据模型

建议新增 `web/src/api/middlewareRepair.ts`，不要把中间件修复 API 挤进 `complexPagesApi.ts`。

```ts
export type MiddlewareKind =
  | "postgres"
  | "redis"
  | "mq"
  | "elasticsearch"
  | "kafka"
  | "mysql"
  | "other";

export type RepairPhase =
  | "created"
  | "identifying_resource"
  | "collecting_evidence"
  | "diagnosing"
  | "matching_experience"
  | "planning"
  | "awaiting_confirmation"
  | "awaiting_approval"
  | "executing"
  | "verifying"
  | "succeeded"
  | "failed"
  | "rolled_back"
  | "experience_candidate_ready";

export type RepairPlanStatus =
  | "draft"
  | "awaiting_confirmation"
  | "approved"
  | "running"
  | "succeeded"
  | "failed";

export type MiddlewareResourceView = {
  id: string;
  kind: MiddlewareKind;
  clusterName: string;
  environment: string;
  topology: MiddlewareTopologyView;
  roleMap: MiddlewareRoleView[];
  criticality: "low" | "medium" | "high" | "critical";
  safetyProfile: MiddlewareSafetyProfileView;
  opsGraphEntityIds: string[];
  ownerTeamId?: string;
  activeRepairCaseIds: string[];
  approvedExperienceRefs: ExperienceRefView[];
};

export type MiddlewareRepairCaseView = {
  id: string;
  caseId: string;
  title: string;
  phase: RepairPhase;
  status: string;
  middlewareResource: MiddlewareResourceView;
  businessImpact: BusinessImpactView;
  evidenceCoverage: DiagnosticCoverageView;
  hypotheses: RootCauseHypothesisView[];
  experienceMatches: ExperienceMatchView[];
  repairPlans: RepairPlanView[];
  activeRepairPlanId?: string;
  recoveryAttempts: RecoveryAttemptView[];
  verificationRefs: string[];
  experienceCandidateRefs: string[];
  blockers: RepairBlockerView[];
};

export type RepairPlanView = {
  id: string;
  caseId: string;
  middlewareResourceId: string;
  source: "experience_pack" | "capsule" | "runbook" | "generated";
  rootCauseHypothesisIds: string[];
  assumptions: PlanAssumptionView[];
  diagnosticSteps: RepairStepView[];
  remediationSteps: RepairStepView[];
  riskLevel: "readonly" | "low" | "medium" | "high" | "destructive";
  approvalPolicy: ApprovalPolicyView;
  rollbackPlan: RollbackPlanView;
  verificationSpec: VerificationSpecRefView;
  disabledConditions: DisabledConditionView[];
  experiencePackRef?: ExperienceRefView;
  status: RepairPlanStatus;
};

export type RecoveryAttemptView = {
  id: string;
  repairPlanId: string;
  workflowRunId?: string;
  actionProposalRefs: string[];
  status: "pending" | "running" | "succeeded" | "failed" | "rolled_back";
  stepResults: RepairStepResultView[];
  failedPoint?: string;
  rollbackResult?: RollbackResultView;
  verificationRefs: string[];
};
```

## 17. API 契约

页面优先使用 canonical API：

```text
GET    /api/v1/middleware-resources
GET    /api/v1/middleware-resources/{id}
POST   /api/v1/middleware-resources/{id}/diagnose
POST   /api/v1/middleware-repair-cases
GET    /api/v1/middleware-repair-cases
GET    /api/v1/middleware-repair-cases/{case_id}
POST   /api/v1/middleware-repair-cases/{case_id}/match-experience
POST   /api/v1/middleware-repair-cases/{case_id}/repair-plans
GET    /api/v1/repair-plans/{id}
PATCH  /api/v1/repair-plans/{id}
POST   /api/v1/repair-plans/{id}/confirm
POST   /api/v1/repair-plans/{id}/execute
POST   /api/v1/repair-plans/{id}/verify
GET    /api/v1/recovery-attempts/{id}
POST   /api/v1/middleware-repair-cases/{case_id}/experience-candidates
```

建议补充查询 API：

```text
GET    /api/v1/middleware-repair-cases/{case_id}/diagnostics
GET    /api/v1/middleware-repair-cases/{case_id}/experience-matches
GET    /api/v1/middleware-repair-cases/{case_id}/timeline
POST   /api/v1/repair-plans/{id}/dry-run
POST   /api/v1/repair-plans/{id}/request-approval
POST   /api/v1/recovery-attempts/{id}/rollback
GET    /api/v1/recovery-attempts/{id}/verification-summary
```

兼容期可以让 `middlewareRepair.ts` fallback 到 case、coroot、opsgraph、runner、verification 现有 API，但页面组件只能依赖 `middlewareRepair.ts` 的 view-model。

## 18. 状态与数据流

```text
AI Chat / User / Alert
  -> create middleware_repair case
  -> middlewareRepair API
  -> normalizeMiddlewareRepairCase
  -> Repair Case Workspace
  -> RepairPlan / Execution / Verification / Experience
```

实时更新策略：

- MVP 使用刷新按钮和轻量 polling。
- 诊断和执行运行中每 5-15 秒刷新 case summary、step status、verification summary。
- 大证据正文和原始输出只在用户打开详情时加载。
- 后续接入统一 transport/realtime event，事件类型包括 middleware.resource.identified、middleware.diagnostic.completed、repair.plan.generated、repair.plan.confirmed、recovery.attempt.updated、repair.verification.completed、repair.experience_candidate.created。
- Workflow、Tool、Approval、HostLease 和 Verification 状态来自对应模块，不在 Middleware 页面重复维护。

## 19. 错误、空态与加载态

- 总览加载：固定高度 skeleton，不改变表格布局。
- 资源目录为空：显示“未发现中间件资源”，提供配置探针和导入入口。
- 资源详情加载失败：保留上一次成功数据并显示 StatusAlert。
- 诊断源不可用：对应 coverage 显示 missing 或 source_unavailable，不自动跳过。
- 备份状态未知：destructive 动作阻断。
- 经验匹配为空：显示“未命中已审核经验”，提供生成 RepairPlan draft 入口。
- RepairPlan 有 disabled condition：确认按钮禁用并显示阻断原因。
- 审批缺失：显示 approval policy blocker。
- HostLease 冲突：显示持有人、TTL、目标资源和下一步。
- 验证失败：显示 failedPoint 和 nextRecommendation。
- 权限不足：显示无权查看此证据或动作，不暴露连接串、SQL 参数、业务数据。

## 20. 响应式与可访问性

- 桌面端：主区 + 右侧 Drawer。
- 平板端：Drawer 降级为底部 Sheet。
- 手机端：表格改为列表，Tabs 横向滚动，Step 表格压缩为卡片。
- 状态不能只靠颜色表达，必须有文字。
- 风险和阻断必须在按钮附近显示原因。
- 图表必须有文本摘要。
- icon button 必须有 aria-label 或 tooltip。
- destructive、failover、rollback、confirm、execute 操作必须有二次确认。

## 21. 验收标准

- `/middleware` 能展示中间件修复总览、Safety Strip、活跃 case、待确认 RepairPlan、运行中 RecoveryAttempt 和经验候选。
- `/middleware/resources` 能展示 PG、Redis、MQ、Kafka、Elasticsearch、MySQL 等资源目录和安全画像。
- 用户从现有 AI Chat 请求“帮我修复 xxx 的 PG 集群”时，后端创建 `middleware_repair` case，页面只承接该 case 并停留在诊断或计划阶段，不能直接执行。
- PG 资源详情能展示拓扑、角色、复制、锁、连接、磁盘/WAL、备份、PITR 和 failover 风险。
- required diagnostic evidence 缺失时，只允许继续只读诊断或生成证据不足计划，不能确认高风险修复。
- 命中已审核经验时展示来源、适用环境、成功率、最近失败、禁用条件、权限、风险、回滚和验证项。
- 环境不匹配或 disabled condition 触发时，只能复用只读诊断，不能复用修复动作。
- RepairPlan Designer 能展示 assumptions、diagnosticSteps、remediationSteps、risk、approvalPolicy、rollbackPlan、verificationSpec 和 disabledConditions。
- 高风险和破坏性步骤必须展示审批、HostLease、ActionToken、回滚和验证状态。
- RecoveryAttempt 能展示每步结果、failedPoint、rollbackResult 和 verificationRefs。
- 修复完成后能嵌入 Verification & Recovery，验证中间件健康和业务恢复。
- 修复成功或失败都能生成经验包候选入口，candidate 不进入 Activation Index。
- 页面不直接调用裸 `fetch`，所有请求走 `web/src/api/middlewareRepair.ts` 或统一 API client。
- 页面不创建私有 WebSocket/SSE。
- SQL、cache、MQ operation、连接串、SecretRef 和敏感输出不在正文、tooltip、title、aria-label 中泄露。
