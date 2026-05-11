# aiops-v2 Execution Fabric, Runbook & Workflow 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-06-execution-fabric-runbook-workflow-module-design.zh.md](2026-05-11-aiops-v2-06-execution-fabric-runbook-workflow-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-06b-execution-fabric-runbook-workflow-frontend-todo.zh.md](2026-05-11-aiops-v2-06b-execution-fabric-runbook-workflow-frontend-todo.zh.md)

## 1. 页面目标

Execution Fabric, Runbook & Workflow 的前端不是“脚本执行器”或“画布编辑器”，而是生产执行编排控制台。页面要让用户完成：

场景调整约束：本阶段不修改现有 Runner 工作流页面和功能。浏览器插件慢请求调试和 PG 修复只复用现有 Runner workflow 引擎、catalog、validate、dry-run、run、publish、history 和事件能力；新增的治理上下文、经验来源、RepairPlan 来源和恢复验证优先在 Case / Execution Run Detail / Experience 页面表达。

- 查看所有可用工具、Runbook、Workflow、运行实例、审批、目标锁、输出、证据和回调状态。
- 手动或通过 AI 创建 Runbook / Workflow draft，并明确 draft、validated、dry_run_passed、published、deprecated 的生命周期。
- 在 Runbook 中按步骤组织观察、判断、动作提案、验证和回滚，但不能从 Runbook 直接执行生产写动作。
- 在 Workflow Start 中定义主机组、标签选择、变量、锁策略和 fanout 策略，并在 dry-run 中看到解析后的目标。
- 在 Workflow 每个步骤中指定目标标签、风险、输入变量、输出变量、审批要求、ActionToken 和 HostLease 依赖。
- 在 Workflow End 中查看输出变量、验证结果、回调状态和失败重试。
- 在运行态查看每个节点、每台主机、每个批次、每个审批、每个变量和每个回调的结果。
- 从 Case、AI Reasoning、OpsGraph、Verification、Experience Pack 跳入对应 Runbook/Workflow，并回写 timeline、evidence 和 audit。

设计原则：

- 第一屏展示执行治理状态，不做单纯画布或脚本列表。
- Workflow 画布服务于生产编排语义：目标、锁、变量、审批、fanout、验证、回调必须可见。
- Runbook 是知识型处置路径，只能生成 `ActionProposal` 或 Workflow draft，不能绕过 Governed Action Plane。
- Workflow 运行生产写动作前必须展示 ActionToken、HostLease、审批和 normalizedInputHash 状态。
- AI 生成的 Runbook/Workflow 默认是 draft，发布必须经过 validate、dry-run、risk review 和人工确认。
- 长输出、敏感变量、SecretRef 和工具原始结果必须脱敏，只展示摘要和 ref。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/RunbookCatalogPage.tsx`：已有 Runbook 列表初版。
- `web/src/pages/RunbookDetailPage.tsx`：已有 Runbook 详情、步骤、验证项、匹配测试和动作提案 Sheet 初版。
- `web/src/api/runbooks.js`：已有 `listRunbooks`、`getRunbook`、`matchRunbooks`、`listRunbookInstances`。
- `web/src/pages/RunnerStudioPage.tsx`：已有 Workflow 画布、工作流库、节点配置、变量面板、运行详情、发布审核、AI 生成草稿。
- `web/src/api/runnerStudioClient.js`：已有 Runner Studio workflow、graph、validate、dry-run、run、run history、action catalog API client。
- `web/src/components/runner/RunnerCanvas.tsx`、`RunnerCanvasNode.tsx`、`RunnerCanvasEdge.tsx`：已有画布和节点边展示。
- `web/src/components/runner/nodeTypeRegistry.js`：已有 start、end、command、shell、stored-script、condition、variable、approval、wait、notify 等节点类型。
- `web/src/components/runner/fallbackActionCatalog.js`：已有基础 action catalog 和输入/输出 schema。
- `web/src/components/runner/runStateReducer.js`、`runEventHistory.js`、`runnerRunVisualState.js`：已有运行事件、节点状态、主机结果、审批、变量和运行历史聚合。
- `web/src/components/runner/runnerVariables.js`：已有输入、环境、系统、节点输出变量收集和引用校验。
- `pkg/runner/VISUAL_WORKFLOW_UI_DESIGN.md` 与相关 TODO：已有生产级 Runner graph/DAG 设计背景。

主要不足：

- `RunbookCatalogPage` 和 `RunbookDetailPage` 仍通过 `complexPagesApi.ts` 取数，应迁移到 `web/src/api/runbooks.ts` 和 Runbook view-model。
- `RunnerStudioPage.tsx` 仍有页面内 `requestJson`，这是后续 Runner 页面治理债务；本次场景调整不要求修改。
- Runbook 缺少创建、编辑、版本、review、publish、instance 运行过程、ActionProposal 生成和 verification/rollback 细节。
- Runner Studio 已有 start host groups 和 end callbacks 雏形；更完整的 targetSelector、lockPolicy、fanout、failureThreshold、outputs、callbacks、verification 属于后续专家页面增强。
- Workflow 运行提交目前更多是 Runner Studio 运行语义；本阶段在 Execution Run Detail / Case timeline 中补齐 ActionToken、HostLease、normalizedInputHash、tool schema、audit、EvidenceRef 的统一展示，不要求改 Runner 页面。
- 缺少 Tool Catalog 页面，用户无法查看工具风险、权限、输入输出 schema、幂等性、超时、脱敏规则和 allowlist 状态。
- 缺少 Workflow Run 独立详情页，运行结果仍主要在 Runner 画布 Drawer 内查看，不利于 Case、Approval、Verification、Audit 链接。
- AI 生成 workflow 草稿已有入口，但缺少 AI draft 的来源 trace、diff、risk review、validate/dry-run gate 和 publish gate 解释。

## 3. 路由与信息架构

保留现有入口：

```text
/runbooks
/runbooks/:runbookId
/runner
/runner/:workflowName
```

建议新增企业级入口：

```text
/execution
/execution/tools
/execution/runs
/execution/runs/:runId
/runbook-instances/:instanceId
/workflows/:workflowName/runs/:runId
```

路由语义：

- `/execution`：Execution Fabric 总览。展示工具、Runbook、Workflow、运行中实例、待审批、目标锁、失败回调和证据。
- `/execution/tools`：Tool Catalog。查看工具 schema、风险、权限、能力、幂等性、超时和脱敏规则。
- `/runbooks`：Runbook 目录。支持筛选、匹配、创建、AI draft、review、publish。
- `/runbooks/:runbookId`：Runbook 详情和编辑。展示版本、步骤、验证、回滚、匹配测试、instance 和动作提案模板。
- `/runbook-instances/:instanceId`：Runbook instance 工作台。展示当前步骤、观察、提案、验证、回滚和 case 回写。
- `/runner`：Workflow Library / Runner Studio 入口。
- `/runner/:workflowName`：Workflow Studio。编辑 graph、Start、节点、End、validate、dry-run、publish、run。
- `/execution/runs/:runId`：Workflow Run 详情。展示节点、主机、变量、审批、EvidenceRef、验证和回调。

MVP 可以先在现有 `/runner/:workflowName` 内承载 Workflow Studio，在 `/runbooks/:runbookId` 内承载 Runbook 详情；但 API、组件和文档按终态拆分。

主导航分区：

| 页面 | 目标 |
| --- | --- |
| Execution Overview | 执行态总览、待审批、失败运行、锁、回调 |
| Tool Catalog | 工具 schema、风险、权限、能力和脱敏规则 |
| Runbook Catalog | Runbook 搜索、匹配、版本、发布和实例 |
| Runbook Instance | Case 中按步骤执行 Runbook 但只生成 ActionProposal |
| Workflow Library | Workflow 目录、状态、版本、风险、最近运行 |
| Workflow Studio | 画布编排、Start、节点、End、校验、dry-run、发布 |
| Workflow Run Detail | 运行态、fanout、主机结果、变量、审批、证据、回调 |

## 4. 页面一：Execution Overview

Execution Overview 回答“当前执行面是否安全、是否有阻塞、是否有失败需要处理”。

### 4.1 布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: Execution Fabric / 环境 / 时间窗口 / 创建 Runbook / 创建 Workflow │
├────────────────────────────────────────────────────────────┤
│ Governance Strip: ActionToken / HostLease / Approval / Tool Health │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: Running / Failed / Awaiting Approval / Callback Retry │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区                       │ 右侧上下文栏                  │
│ Runs / Approvals / Locks /    │ Case / Evidence / Workflow / │
│ Tool Results / Callbacks      │ Runbook / Audit              │
└──────────────────────────────┴─────────────────────────────┘
```

### 4.2 指标

- 运行中 Workflow runs。
- 运行中 Runbook instances。
- 待审批 ActionProposal / workflow node approval。
- 已获取 HostLease 数量。
- 锁冲突数量。
- callback retry 数量。
- 最近失败节点数。
- agent_unavailable 节点数。
- 大输出转 object store 数量。

### 4.3 列表

默认列表按风险和阻塞排序：

| 列 | 内容 |
| --- | --- |
| Item | run id、instance id、workflow/runbook 名称 |
| Source | case、AI reasoning、manual、experience、schedule |
| Status | running、blocked、failed、verifying、callback_retry |
| Governance | approval、ActionToken、HostLease、policy |
| Target | 标签、主机、批次、fanout 策略 |
| Evidence | ToolResult、RunEvent、Artifact、EvidenceRef |
| Next | 审批、查看失败、重试回调、打开运行详情 |

## 5. Tool Catalog 页面

Tool Catalog 回答“系统有哪些工具，执行前需要什么约束”。

### 5.1 表格列

| 列 | 内容 |
| --- | --- |
| Tool | name、description、provider |
| Risk | readonly、low_risk、high_risk、destructive |
| Permissions | requiredPermissions、requiredCapabilities |
| Schema | inputSchema、outputSchema |
| Runtime | timeout、idempotency、retry safety |
| Governance | approval、ActionToken、allowlist、redaction |
| Usage | recent runs、failure rate、last used |
| Action | 查看 schema、创建 ActionProposal 模板、打开审计 |

### 5.2 Tool Detail

详情 Drawer 展示：

- ToolDefinition。
- 输入输出 schema。
- risk。
- requiredPermissions。
- requiredCapabilities。
- idempotency。
- timeout。
- redactionRules。
- allowlist 状态。
- 最近工具调用。
- 最近失败。
- 关联 Runbook / Workflow 节点。

高风险和破坏性工具必须显示为什么不能直接运行，以及如何生成 ActionProposal。

## 6. Runbook Catalog

Runbook Catalog 回答“有哪些已审核处置路径，能匹配什么问题”。

### 6.1 筛选

- status：draft、reviewed、published、deprecated。
- risk。
- environment。
- entityTypes。
- problemSignatures。
- owner。
- capability。
- disabledConditions。
- updatedAt。

### 6.2 表格列

| 列 | 内容 |
| --- | --- |
| Runbook | title、id、version |
| Status | draft、reviewed、published、deprecated |
| Match | problem signatures、entity types、environment profile |
| Risk | max step risk、rollback available |
| Verification | verification coverage |
| Instances | active、success rate、last outcome |
| Lineage | AI generated、case derived、manual |
| Action | 查看、匹配测试、创建 instance、review、publish |

### 6.3 创建入口

创建方式：

- 手动创建：从空白 Runbook 开始。
- 从 Case 提炼：选择 case、evidence、verification、postmortem。
- 从 AI draft 创建：从 ReasoningOutput 的 `runbook_draft` 进入。
- 从 Experience Pack 创建：复用经验包中的步骤和禁用条件。

所有 AI 创建的 Runbook 默认是 draft，不能直接 published。

## 7. Runbook 详情与编辑

Runbook 详情回答“这个处置路径怎么执行、风险在哪里、能否用于当前 case”。

### 7.1 Header

显示：

- title、id、version。
- status。
- owner。
- environment profile。
- max risk。
- match summary。
- verification coverage。
- lineage。

动作：

- 编辑草稿。
- 创建新版本。
- 匹配测试。
- 生成 Runbook instance。
- review。
- publish。
- deprecate。

### 7.2 Tabs

| Tab | 内容 |
| --- | --- |
| Overview | 适用范围、风险、验证、回滚、历史效果 |
| Steps | observe、decide、propose_action、verify、rollback 步骤 |
| Match | problemSignatures、entityTypes、disabledConditions、匹配测试 |
| Action Templates | 只生成 ActionProposal 的模板，不直接执行 |
| Verification | 验证项、窗口、证据需求、失败处理 |
| Rollback | 回滚条件、回滚动作提案、验证 |
| Instances | 关联 case 的运行实例 |
| Lineage | 来源 case、PromptTrace、ExperiencePack、OpsGraph patch |

### 7.3 Step 编辑

每个步骤字段：

- id。
- type：observe、decide、propose_action、verify、rollback。
- title。
- instructions。
- evidenceRequirements。
- actionTemplate。
- risk。
- expectedResult。
- next。

规则：

- `observe` 只能触发只读证据请求。
- `propose_action` 只能生成 ActionProposal，不直接调用工具。
- `verify` 必须引用 VerificationSpec 或 EvidenceRequirement。
- `rollback` 必须说明触发条件和回滚验证。

## 8. Runbook Instance 工作台

Runbook Instance 回答“当前 case 正在按哪个 Runbook 走到哪一步”。

### 8.1 布局

```text
Header: instance / case / runbook / current step / status
Step Rail: observe -> decide -> propose_action -> verify -> rollback
Main: current step detail
Right Rail: evidence / action proposals / verification / timeline
```

### 8.2 当前步骤

显示：

- step title。
- instructions。
- evidence requirements。
- observations。
- decision options。
- generated ActionProposal refs。
- verification refs。
- blocking items。

### 8.3 动作

允许：

- 记录观察。
- 请求更多证据。
- 生成 ActionProposal。
- 打开已生成审批。
- 执行下一步。
- 标记验证通过或失败。

不允许：

- 直接执行写工具。
- 跳过 required evidence。
- 绕过审批和 HostLease。

## 9. Workflow Library

Workflow Library 回答“有哪些可执行编排，当前是否可发布或可运行”。

### 9.1 表格列

| 列 | 内容 |
| --- | --- |
| Workflow | title、name、version |
| Status | draft、validated、dry_run_passed、published、deprecated |
| Risk | riskSummary、high risk node count |
| Targets | targetSelector、host groups、fanout |
| Gates | validate、dry-run、risk review、publish |
| Runs | last run、success rate、active run |
| Lineage | AI generated、manual、case derived |
| Action | 打开 Studio、validate、dry-run、publish、run |

### 9.2 状态显示

- draft：只可编辑、validate、dry-run。
- validated：可 dry-run。
- dry_run_passed：可进入 risk review / publish。
- published：可运行。
- deprecated：不可新运行，但可查看历史。

AI generated draft 必须用独立标识，且不能直接 publish 或 run。

## 10. Workflow Studio

Workflow Studio 是当前 `RunnerStudioPage` 的企业级目标形态，但不是本次浏览器插件调试和 PG 修复闭环的改动范围。

### 10.1 总体布局

```text
Top Bar: workflow / status / save / validate / dry-run / publish / run / AI draft
Left: Action Catalog / Tool Catalog / Templates
Center: Workflow Canvas
Right: Node / Start / End Property Panel
Bottom or Drawer: Validation / Run / Variables / Audit / Diff
```

### 10.2 Start 节点

Start 节点必须能定义：

- targetSelector.labels。
- explicitHosts。
- variables。
- lockPolicy.mode。
- lockPolicy.acquire。
- fanout.mode：sequential、parallel、batch。
- batchSize。
- failureThreshold。

Dry-run 后必须展示：

- 解析到的主机。
- 标签来源。
- 权限检查结果。
- HostLease 预检查。
- fanout 批次。
- 失败阈值策略。

### 10.3 节点属性

所有执行节点显示：

- node id。
- action。
- target labels / explicit hosts。
- input variables。
- output schema。
- risk。
- requiredPermissions。
- requiredCapabilities。
- timeout。
- retry safety。
- idempotency。
- approval requirement。
- ActionProposal / ActionToken 状态。
- HostLease 状态。

高风险工具节点不能直接运行，必须显示待创建或已绑定的 ActionProposal。

### 10.4 Fanout 节点与多主机运行

Fanout 展示：

- 目标标签。
- 解析主机列表。
- sequential / parallel / batch。
- batch size。
- failure threshold。
- 每台主机独立结果。
- 继续、暂停、回滚条件。

运行态中每个 host result 要能展开 stdout/stderr 摘要、exit code、duration、EvidenceRef 和 Artifact ref。

### 10.5 Approval 节点

Approval 节点显示：

- approvers。
- timeout。
- risk reason。
- approved / rejected 分支。
- approval id。
- reviewer。
- decision。
- comment。
- evidence refs。

Approval 节点不能替代 Governed Action Plane。它只是 workflow 内暂停点，高风险工具仍需要 ActionToken。

### 10.6 Condition 节点

Condition 节点显示：

- expression。
- 引用变量。
- IF / ELSE 输出。
- 当前运行匹配分支。
- 变量缺失警告。

表达式必须来自可用变量或证据，不允许引用不存在的上游输出。

### 10.7 End 节点

End 节点必须能定义：

- outputs：key、source、type、redaction。
- callbacks：incident.update、erp.update、webhook、experience.candidate。
- verification。
- callback retry policy。

运行完成后展示：

- 输出变量。
- 验证结果。
- 回调成功 / retry / failed。
- 失败回调的下一步。

## 11. Workflow 生命周期和发布审核

### 11.1 Validate

Validate 展示：

- graph 结构问题。
- node schema 问题。
- target selector 问题。
- variable reference 问题。
- risk / approval 问题。
- callbacks 问题。

问题必须可定位到画布节点、边和字段。

### 11.2 Dry-run

Dry-run 展示：

- target resolution。
- host lease precheck。
- ActionToken requirement。
- variable resolution。
- graph path。
- fanout batches。
- callbacks preview。
- evidence and audit refs preview。

Dry-run 不执行生产写动作，不创建真实 ActionToken。

### 11.3 Risk Review

Risk Review 展示：

- high risk / destructive nodes。
- mutating tools。
- missing rollback。
- missing verification。
- disabled conditions。
- impacted services / business capabilities。
- required approvals。

### 11.4 Publish

发布确认必须显示：

- validate status。
- dry-run status。
- risk summary。
- semantic diff。
- AI generated draft 标识。
- reviewer note。
- publish blocker。

未通过 validate、dry-run 或 risk review 时不能 publish。

## 12. Workflow Run Detail

Workflow Run Detail 是运行态事实源。

### 12.1 Header

显示：

- run id。
- workflow name/version。
- status。
- source：manual、case、AI reasoning、runbook、experience、schedule。
- triggeredBy。
- caseId。
- startedAt、finishedAt。
- risk。
- target summary。

动作：

- 取消。
- 查看 graph。
- 查看 case。
- 查看 EvidenceRef。
- 重试失败回调。
- 生成复盘 / 经验候选。

### 12.2 Tabs

| Tab | 内容 |
| --- | --- |
| Graph | 节点和边运行状态 |
| Events | RunEvent 时间线 |
| Hosts | 每台主机结果、锁、stdout/stderr 摘要 |
| Variables | inputs、outputs、exports、nodeResults |
| Approvals | workflow approval、ActionProposal、ActionToken |
| Tool Results | ToolResult、Artifact、object store refs |
| Verification | 验证结果和窗口 |
| Callbacks | incident、ERP、webhook、experience candidate 回调 |
| Audit | operator、policy、normalizedInputHash、PromptTrace |

### 12.3 状态

- queued。
- running。
- blocked。
- awaiting_approval。
- verifying。
- callback_retry。
- completed。
- failed。
- canceled。
- agent_unavailable。

`agent_unavailable` 不能显示为无限 pending，必须给出可处理下一步。

## 13. AI 创建 Runbook / Workflow

### 13.1 AI draft 输入

来源：

- case。
- PromptTrace。
- Observability evidence。
- OpsGraph path。
- ExperiencePack。
- user instruction。

AI draft 表单显示：

- 目标问题。
- 影响实体。
- 证据 refs。
- 目标环境。
- 允许的工具范围。
- 风险上限。
- 输出类型：Runbook draft / Workflow draft。

### 13.2 AI draft 结果

展示：

- draft summary。
- graph diff 或 runbook step diff。
- evidence refs。
- tool refs。
- risk summary。
- disabled conditions。
- required validation。
- PromptTrace。

AI draft 只能应用为本地或后端 draft，不能直接发布或运行。

## 14. 嵌入式入口

### 14.1 Case 工作台

Case 页面复用：

- matched Runbooks。
- active Runbook instances。
- recommended Workflow drafts。
- active Workflow runs。
- ActionProposal refs。
- verification refs。
- execution timeline。

### 14.2 Governed Action Plane

ActionProposal 详情链接：

- source Runbook step。
- source Workflow node。
- normalizedInputHash。
- ActionToken。
- HostLease。
- ToolResult。
- rollback / verification。

### 14.3 AI Reasoning

AI Reasoning 输出：

- `runbook_draft` 跳到 Runbook draft。
- `workflow_draft` 写入现有 Runner workflow draft/catalog，并在 Case 或 Experience 页面展示来源、风险、validate/dry-run 状态。
- `action_proposal_draft` 跳到 Governed Action。
- PromptTrace 关联到 draft 和 run。

### 14.4 Verification

Verification 页面复用：

- Workflow End verification。
- Runbook verification。
- run output variables。
- callback result。
- post-fix evidence。

### 14.5 Experience Pack

Experience Pack 审核页复用：

- 从成功 case 生成 Runbook draft。
- 从成功 Workflow run 生成经验候选。
- 从失败 run 生成 disabled condition 或 rollback 改进。

## 15. 前端数据模型

新增 `web/src/api/executionFabric.ts`：

```ts
export interface ToolDefinitionView {
  name: string;
  description?: string;
  inputSchema: Record<string, unknown>;
  outputSchema: Record<string, unknown>;
  risk: "readonly" | "low_risk" | "high_risk" | "destructive" | "unknown";
  requiredPermissions: string[];
  requiredCapabilities: string[];
  idempotency: "idempotent" | "non_idempotent" | "unknown";
  timeout?: string;
  redactionRules: string[];
  allowlisted: boolean;
}

export interface RunbookView {
  id: string;
  title: string;
  version: string;
  status: "draft" | "reviewed" | "published" | "deprecated";
  match: RunbookMatchView;
  steps: RunbookStepView[];
  verification: VerificationRefView[];
  rollback: RunbookStepView[];
  owners: string[];
  lineage: LineageRefView[];
}

export interface RunbookStepView {
  id: string;
  type: "observe" | "decide" | "propose_action" | "verify" | "rollback";
  title: string;
  instructions: string;
  evidenceRequirements: string[];
  actionTemplate?: ActionTemplateView;
  risk: "low" | "medium" | "high" | "critical";
  expectedResult?: string;
  next: string[];
}

export interface RunbookInstanceView {
  id: string;
  caseId?: string;
  runbookId: string;
  currentStepId: string;
  status: "created" | "observing" | "awaiting_action" | "verifying" | "completed" | "failed" | "rolled_back";
  observations: EvidenceRefView[];
  actionProposalRefs: string[];
  verificationRefs: string[];
}

export interface WorkflowView {
  id: string;
  title: string;
  version: string;
  status: "draft" | "validated" | "dry_run_passed" | "published" | "deprecated";
  start: WorkflowStartView;
  graph: RunnerGraphView;
  end: WorkflowEndView;
  riskSummary: RiskSummaryView;
  lineage: LineageRefView[];
}

export interface WorkflowStartView {
  targetSelector: TargetSelectorView;
  variables: WorkflowVariableView[];
  lockPolicy: { mode: "exclusive" | "shared"; acquire: "before_run" | "per_node" };
  fanout: { mode: "sequential" | "parallel" | "batch"; batchSize?: number; failureThreshold?: number };
}

export interface WorkflowEndView {
  outputs: Array<{ key: string; source: string; type?: string; redaction?: string }>;
  callbacks: Array<{ type: "incident.update" | "erp.update" | "webhook" | "experience.candidate"; target?: string; retryPolicy?: string }>;
  verification: VerificationRefView[];
}

export interface WorkflowRunView {
  runId: string;
  workflowId: string;
  workflowVersion?: string;
  caseId?: string;
  status: "queued" | "running" | "blocked" | "awaiting_approval" | "verifying" | "callback_retry" | "completed" | "failed" | "canceled" | "agent_unavailable";
  source: "manual" | "case" | "ai_reasoning" | "runbook" | "experience" | "schedule";
  targetSummary: TargetResolutionView;
  events: RunEventView[];
  nodes: Record<string, WorkflowNodeRunStateView>;
  hosts: Record<string, HostRunStateView>;
  variables: WorkflowRunVariablesView;
  approvals: ApprovalRefView[];
  actionTokens: ActionTokenRefView[];
  hostLeases: HostLeaseRefView[];
  evidenceRefs: EvidenceRefView[];
  callbackResults: CallbackResultView[];
}
```

## 16. API 契约

使用模块设计中的 API：

```text
GET    /api/v1/tools
POST   /api/v1/tools/{name}/execute
GET    /api/v1/runbooks
GET    /api/v1/runbooks/{id}
POST   /api/v1/runbook-instances
POST   /api/v1/runbook-instances/{id}/next
GET    /api/v1/workflows
POST   /api/v1/workflows
POST   /api/v1/workflows/{id}/validate
POST   /api/v1/workflows/{id}/dry-run
POST   /api/v1/workflows/{id}/publish
POST   /api/v1/workflows/{id}/runs
GET    /api/v1/workflow-runs/{run_id}
```

兼容当前已存在 API：

```text
GET    /api/v1/runbooks
GET    /api/v1/runbooks/{id}
POST   /api/v1/runbooks/match
GET    /api/v1/runbooks/instances
GET    /api/runner-studio/workflows
GET    /api/runner-studio/workflows/{name}/graph
POST   /api/runner-studio/workflows/graph
PUT    /api/runner-studio/workflows/{name}/graph
POST   /api/runner-studio/workflows/{name}/validate
POST   /api/runner-studio/workflows/graph/dry-run
POST   /api/runner-studio/workflows/{name}/publish
POST   /api/runner-studio/runs
GET    /api/runner-studio/runs/{run_id}/graph
GET    /api/runner-studio/runs/{run_id}/events/history
GET    /api/runner-studio/actions
```

兼容逻辑必须封装在 API client 和 normalize 层。页面组件只消费统一 view-model。

## 17. 状态与数据流

```text
Case / AI Reasoning / Manual / Experience
  -> Runbook / Workflow draft
  -> validate
  -> dry-run
  -> risk review
  -> publish
  -> create run or instance
  -> ActionToken / HostLease / ToolDispatcher / Runner Engine
  -> ToolResult / RunEvent / Artifact
  -> EvidenceRef / Verification / Callback / Audit
```

要求：

- 页面不直接调用裸 `fetch`。
- Workflow Studio 不私自绕过 runnerStudioClient / executionFabric API。
- Runbook 和 Workflow 都通过统一 risk / approval / evidence / audit view-model 展示。
- URL query 保存当前 workflow、runId、runbookId、instanceId、tab、caseId。
- 长运行 run 的状态必须能刷新或订阅，不允许停留在过期 running。
- 每个运行结果都必须能追溯到 node、host、tool、ActionToken、EvidenceRef 和 audit ref。

## 18. 错误、空态与加载态

错误态：

- Runner API 不可用：保留本地 draft，禁用 validate、dry-run、publish、run。
- validate 失败：定位节点、边和字段。
- dry-run 失败：展示 target、lock、variable、callback 哪一步失败。
- HostLease 冲突：展示锁持有者、过期时间和冲突主机。
- ActionToken 失效：要求重新审批，不能继续执行。
- agent_unavailable：展示受影响节点和目标，不能无限 pending。
- callback 失败：进入 callback_retry，保留 run state。

空态：

- 无 Runbook：显示创建入口和从 case 提炼入口。
- 无 Workflow：显示创建 workflow 和 AI draft 入口。
- 无 ToolDefinition：显示 action catalog 不可用和刷新入口。
- 无 run history：显示运行后会出现节点和主机结果。

加载态：

- Workflow 画布和属性面板分开 loading。
- run drawer 加载历史时保留当前 run summary。
- Tool schema 和 Runbook step 详情 lazy load，不阻塞列表。

## 19. 安全与治理显示规则

- 高风险和破坏性工具不能从前端直接执行，必须走 ActionProposal / ActionToken。
- Runbook `propose_action` 只生成 ActionProposal，不直接调用工具。
- Workflow publish 前必须通过 validate、dry-run 和 risk review。
- AI generated draft 不能直接 publish 或 run。
- 生产写动作启动前必须展示 HostLease 状态。
- 多主机 fanout 必须展示每个目标独立结果。
- 非幂等工具禁止自动重试，除非工具声明 retry safety。
- SecretRef、token、password、cookie、authorization header、私钥、完整连接串不能出现在 DOM。
- 大输出只展示摘要和 object store ref。
- 回调必须幂等；外部系统不可用时 run 进入 callback_retry，不丢失状态。

## 20. 验收标准

- Runbook 目录和详情支持 status、版本、match、steps、verification、rollback、lineage 和 instance。
- Runbook step 只能生成 ActionProposal，不能直接执行生产写动作。
- Workflow Studio 的 Start targetSelector、labels、explicitHosts、variables、lockPolicy、fanout、batchSize、failureThreshold 属于后续专家页面增强；本阶段只要求现有 Runner 引擎能承接已确认 workflow run。
- 每个 Workflow 节点能指定目标标签、输入变量、输出变量、风险、权限、审批要求和 ActionToken / HostLease 状态。
- End 节点能展示 outputs、callbacks、verification 和 callback retry。
- Validate、dry-run、risk review、publish gate 结果在 Case / Execution Run Detail / Experience 页面中清楚可见，未通过不能发布或执行。
- Workflow Run Detail 能展示 graph、events、hosts、variables、approvals、tool results、verification、callbacks、audit。
- 多主机 fanout 能按目标展示独立结果，并按 failureThreshold 解释继续、暂停或回滚。
- AI 生成 Runbook/Workflow 默认是 draft，不能直接发布或运行。
- 运行结果能回写 case timeline、workflow run event、prompt trace tool call、audit log 和 evidence store。
- 页面请求经过 `web/src/api/executionFabric.ts`、`web/src/api/runbooks.ts` 或 `web/src/api/runnerStudioClient.js`，页面不保留裸 `fetch`。
