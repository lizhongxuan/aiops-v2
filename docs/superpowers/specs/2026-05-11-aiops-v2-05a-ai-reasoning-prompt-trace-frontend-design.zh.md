# aiops-v2 AI Reasoning & Prompt Trace 前端页面设计

日期：2026-05-11
状态：前端页面设计方案
来源模块：[2026-05-11-aiops-v2-05-ai-reasoning-prompt-trace-module-design.zh.md](2026-05-11-aiops-v2-05-ai-reasoning-prompt-trace-module-design.zh.md)
实施清单：[2026-05-11-aiops-v2-05b-ai-reasoning-prompt-trace-frontend-todo.zh.md](2026-05-11-aiops-v2-05b-ai-reasoning-prompt-trace-frontend-todo.zh.md)

## 1. 页面目标

AI Reasoning & Prompt Trace 的前端不是“显示模型日志”的调试页，而是 AI 运维治理审计工作台。页面要让用户完成：

- 从每轮对话、case、DebugCase、中间件修复、Runbook/Workflow 计划中点进对应 Prompt Trace。
- 查看 AI 本轮使用了哪些 Prompt layers、证据、OpsGraph 查询、经验包、记忆、工具和治理规则。
- 明确区分 AI 输出类型：回答、证据请求、假设更新、诊断计划、动作提案草稿、修复计划草稿、Runbook/Workflow 草稿、验证计划、复盘草稿、经验候选摘要。
- 审查模型为什么看得到某些工具、为什么看不到某些工具、哪些工具因为 RBAC、风险、主机锁或策略被隐藏。
- 审查模型输出是否越权，写动作是否都转成 `ActionProposal`、Workflow draft 或 RepairPlan draft。
- 比较 Prompt、工具 schema、经验索引、系统策略变更前后的 diff，并用 Eval case 发现退化。
- 给 reviewer 提供足够细节，同时给普通用户只展示可解释摘要，不泄露系统策略、敏感 evidence 或 secret。

设计原则：

- Trace 是治理证据，不是普通聊天装饰。每个 AI turn 都必须能定位到 trace。
- 第一屏显示推理结果、证据引用、治理决策和风险门禁，不默认展示长 prompt 原文。
- 系统策略、敏感上下文、受限 evidence、SecretRef 解密结果对普通用户隐藏。
- 页面只消费统一 AI Reasoning / Prompt Trace API view-model，不在页面内直接读取文件路径和拼接 raw trace。
- Raw trace 仅给授权 reviewer 使用，并且默认折叠。

## 2. 当前前端基础

当前 `web/src` 已有可复用基础：

- `web/src/pages/PromptTracePage.tsx`：已有 Prompt Trace 列表和详情初版，支持 Overview、Prompt 层、Messages、Tools、Diff、Raw。
- `web/src/api/promptTraces.js`：已有 `fetchPromptTraces` 和 `fetchPromptTraceFile`，但页面尚未使用。
- `web/src/utils/promptTraceViewModel.js`：已有 `parsePromptTrace`，支持 summary、layers、visible tools、fingerprints、warnings。
- `web/src/utils/promptTraceViewModel.test.js`：已有 Prompt Trace view-model 测试。
- `web/src/chat/ChatPage.tsx`、`web/src/pages/ProtocolWorkspacePage.tsx`：已有 AssistantTransport 对话和复杂运维工作台。
- `web/src/chat/components/ProcessTranscript.tsx`、`ToolBlockPart.tsx`、`ApprovalBlockPart.tsx`：已有过程块、工具块和审批块展示。
- `web/src/transport/aiopsTransportTypes.ts`：已有 turn、process、approval、artifact、MCP surface 的前端状态类型。
- `internal/server/prompt_trace_api.go`：已有 `/api/v1/debug/model-input-traces` 和文件读取接口。

主要不足：

- `PromptTracePage.tsx` 使用页面内裸 `fetch`，没有复用 `web/src/api/promptTraces.js`。
- 当前页面偏本地文件浏览，缺少 `PromptTrace` 领域对象：promptLayers、hiddenToolsDenied、evidenceRefs、opsGraphQueryRefs、activatedExperienceRefs、governanceDecisions、toolCallRefs、modelOutputRef、evalLabels。
- Chat / Protocol / Case 页面没有稳定的“打开本轮 Prompt Trace”入口。
- Prompt layer 只按 message 展示，缺少模块设计中的 System Policy、Product Invariants、RBAC、Case Summary、Evidence Summary、OpsGraph Context、Activated Experience、Runbook/Workflow Metadata、Tool Visibility、Recent Transcript、User Turn 分层视图。
- 工具可见性只展示 visible tools，没有展示 hidden tools 和 denied reason。
- 没有 ReasoningOutput 类型化展示，也没有输出流向：Hypothesis、ActionProposal、Workflow draft、RepairPlan、Experience candidate、VerificationPlan。
- 没有 Prompt / Experience / Tool schema 变更前后的治理 diff 和 Eval 报告页面。
- Raw trace 对权限裁剪、敏感 evidence、系统策略隐藏的规则不够明确。

## 3. 路由与信息架构

保留现有入口：

```text
/debug/prompts
```

建议新增治理入口：

```text
/ai-reasoning
/ai-reasoning/prompt-traces/:traceId
/ai-reasoning/evals
/ai-reasoning/evals/:reportId
```

路由语义：

- `/ai-reasoning`：AI Reasoning 工作台。按 case、session、turn、输出类型、风险、治理状态筛选 AI 推理记录。
- `/ai-reasoning/prompt-traces/:traceId`：Prompt Trace 详情。展示 layers、工具可见性、证据、模型输出、工具调用、治理决策、diff 和 raw。
- `/ai-reasoning/evals`：Eval 回归工作台。运行和查看 Prompt / Tool / Experience 变更后的回归评测。
- `/ai-reasoning/evals/:reportId`：Eval 报告详情。
- `/debug/prompts`：兼容旧入口，跳转或复用 Prompt Trace Explorer，但文案明确这是 LLM Prompt Trace，不是 04 模块的业务 Debug Trace。

MVP 可先不新增所有子路由，把详情通过 `/debug/prompts?traceId=...&tab=...` 或 `/ai-reasoning?traceId=...&tab=trace` 承载；但文档按终态拆分。

主 tabs：

| Tab | 目标 |
| --- | --- |
| Reasoning Runs | 按 case、session、turn 查看 AI 推理记录和输出类型 |
| Prompt Trace Explorer | 查看 prompt layers、message、tools、evidence、governance 和 raw |
| Output Routing | 查看 ReasoningOutput 如何流向 Hypothesis、ActionProposal、Workflow、RepairPlan、Experience、Verification |
| Tool Visibility | 查看 visibleTools、hiddenToolsDenied、RBAC、风险等级和审批要求 |
| Prompt Diff | 比较 prompt、工具 schema、经验索引、系统规则变更 |
| Eval Reports | 查看和运行回归评测 |

## 4. 页面一：AI Reasoning 工作台

### 4.1 总体布局

```text
┌────────────────────────────────────────────────────────────┐
│ Header: AI Reasoning / 环境 / 时间窗口 / 运行 Eval / 刷新       │
├────────────────────────────────────────────────────────────┤
│ Context Bar: Case / Session / Turn / Trace / Output Type / Risk │
├────────────────────────────────────────────────────────────┤
│ Summary Strip: Turns / Traces / Action Drafts / Denied Tools / Evals │
├──────────────────────────────┬─────────────────────────────┤
│ 主工作区 Tabs                  │ 右侧 Detail Drawer           │
│ Runs / Trace / Routing / Tool │ ReasoningOutput / Governance │
│ / Diff / Eval                 │ / Evidence / Eval Labels     │
└──────────────────────────────┴─────────────────────────────┘
```

桌面端右侧 Drawer 宽度 380-460px。窄屏时 Drawer 变成底部 Sheet。

### 4.2 Header

显示：

- 当前环境。
- 当前时间窗口。
- 当前用户权限：普通用户、case owner、reviewer、admin。
- Prompt Trace 数据源状态。
- Eval runner 状态。

动作：

- 刷新。
- 运行 Eval。
- 打开 Prompt Compiler Preview。
- 导出当前 trace 引用。
- 打开安全裁剪说明。

### 4.3 Context Bar

支持过滤：

- caseId。
- sessionId。
- turnId。
- traceId。
- outputType。
- risk。
- model。
- toolName。
- evidenceRef。
- experienceRef。
- governanceDecision。
- evalLabel。

筛选条件写入 URL query，便于从 Chat、Case、DebugCase、Middleware Repair、Approval、Eval 报告跳入。

## 5. Reasoning Runs Tab

Reasoning Runs 回答“哪些 AI turn 发生了、产生了什么、是否合规”。

### 5.1 表格列

| 列 | 内容 |
| --- | --- |
| Turn | sessionId、turnId、caseId、createdAt |
| User Intent | 用户请求摘要 |
| Output Type | answer、hypothesis_update、diagnostic_plan、action_proposal_draft 等 |
| Model | model、reasoning effort、token 摘要 |
| Evidence | supportingEvidenceRefs、contradictingEvidenceRefs |
| Tools | visible count、hidden denied count、tool calls |
| Governance | approval required、RBAC clipped、action blocked、safe |
| Risk | low、medium、high、critical |
| Action | 打开 trace、打开 case、打开 output、运行 eval |

### 5.2 详情 Drawer

显示：

- ReasoningOutput 类型和正文摘要。
- confidence。
- supportingEvidenceRefs。
- contradictingEvidenceRefs。
- risk。
- downstream target：Hypothesis、ActionProposal、Workflow draft、RepairPlan、Runbook draft、VerificationPlan、PostmortemDraft、ExperienceCandidate。
- governance decisions。
- PromptTrace link。

AI 输出如果包含写动作但没有 ActionProposal / Workflow draft，必须显示 `governance violation`。

## 6. Prompt Trace Explorer

Prompt Trace Explorer 是当前 `/debug/prompts` 的升级版。

### 6.1 左侧 Trace 列表

支持搜索和过滤：

- sessionId。
- caseId。
- turnId。
- traceId。
- model。
- prompt fingerprint。
- tool name。
- evidenceRef。
- outputType。
- eval label。

列表项显示：

- sessionId / turnId。
- caseId。
- model。
- output type。
- visible tools count。
- governance warning count。
- createdAt。
- stable hash。

### 6.2 Trace Header

显示：

- trace id。
- case id。
- session id。
- turn id。
- model。
- createdAt。
- prompt fingerprint：stable、system、developer、tools、runtime policy、protocol state。
- 权限状态：full、redacted、restricted。

动作：

- 打开关联对话。
- 打开 case。
- 打开 output routing。
- 打开 eval case。
- 复制 trace link。
- 下载 reviewer 可见摘要。

### 6.3 Prompt Layers Tab

按模块设计固定顺序展示：

```text
System Policy
Product Invariants
User RBAC and Resource Scope
Case Summary
Evidence Summary
OpsGraph Context
Activated Experience
Runbook / Workflow Metadata
Tool Visibility
Recent Transcript
User Turn
```

每层显示：

- name。
- source。
- tokenCount。
- charCount。
- redactionStatus。
- refs。
- hash。
- diff status。
- warnings。

普通用户看不到完整 System Policy 和敏感上下文，只能看到裁剪摘要和 redaction reason。

### 6.4 Messages Tab

展示 provider messages：

- providerRole。
- semanticRole。
- promptLayer。
- charCount。
- token estimate。
- content preview。
- redactionStatus。
- tool call id。

默认折叠长文本。超过阈值的 message 显示 size warning，不自动展开。

### 6.5 Evidence & Context Tab

展示：

- evidenceRefs。
- opsGraphQueryRefs。
- activatedExperienceRefs。
- memoryRefs。
- runbook / workflow metadata refs。
- case refs。

每个引用展示：

- id。
- source。
- summary。
- redactionStatus。
- confidence。
- usedInLayer。
- link。

受限 evidence 不显示正文，只显示 id、source、reason。

### 6.6 Model Output Tab

展示：

- raw model output summary。
- parsed ReasoningOutput。
- output type。
- confidence。
- supporting / contradicting evidence。
- risk。
- suggested next actions。
- downstream routing。

如果模型输出无法解析成 ReasoningOutput，要显示 parse warning，并禁止把自然语言直接当动作执行。

### 6.7 Tool Calls Tab

展示：

| 列 | 内容 |
| --- | --- |
| Tool | name、server、risk、mutating |
| Input | normalizedInputHash、脱敏摘要 |
| Status | proposed、called、blocked、approved、failed、completed |
| Approval | approval id、decision、reviewer |
| Evidence | input evidence refs、tool result refs |
| Output | output summary、redactionStatus |

工具 input 默认展示 hash 和摘要，不展示 secret、token、password、cookie。

### 6.8 Governance Tab

展示：

- RBAC 裁剪结果。
- resource scope。
- hiddenToolsDenied。
- risk classification。
- approval requirement。
- policy decisions。
- action proposal routing。
- violations。

治理决策需要以结构化列表展示，而不是隐藏在 prompt 原文里。

### 6.9 Diff Tab

支持比较：

- 本 trace 与上一轮 trace。
- 本 trace 与同 case 上一次成功 trace。
- 当前 prompt 与变更前 prompt。
- tool registry diff。
- experience activation diff。
- runtime policy / protocol state diff。

Diff 展示：

- changed layers。
- added / removed tools。
- hidden denied tools 变化。
- evidence refs 变化。
- prompt fingerprint 变化。
- ReasoningOutput 变化。

### 6.10 Raw Tab

Raw 仅授权 reviewer 可见：

- JSON。
- Markdown。
- diff file。

普通用户看到 redacted raw summary。即使是 reviewer，Raw 也默认折叠，并显示敏感信息警告。

## 7. Output Routing Tab

Output Routing 回答“AI 输出去了哪里，是否越权”。

### 7.1 输出类型

展示模块定义的类型：

```text
answer
evidence_request
hypothesis_update
diagnostic_plan
action_proposal_draft
repair_plan_draft
runbook_draft
workflow_draft
verification_plan
postmortem_draft
experience_candidate_summary
```

### 7.2 路由矩阵

| Output Type | 目标模块 | UI 展示 | 允许动作 |
| --- | --- | --- | --- |
| hypothesis_update | Incident Control Plane | Hypothesis card | 写入 case hypothesis |
| action_proposal_draft | Governed Action Plane | ActionProposal preview | 创建待审批 ActionProposal |
| workflow_draft | Execution Fabric | Workflow draft preview | 打开 Runner draft |
| repair_plan_draft | Middleware Repair | RepairPlan preview | 创建修复计划候选 |
| experience_candidate_summary | Learning & Asset Plane | Experience candidate preview | 进入经验候选审核 |
| verification_plan | Verification & Recovery | Verification plan | 创建验证任务 |
| answer | Chat / Case | 用户可见回答 | 无写动作 |

如果 output type 和内容不一致，例如 `answer` 中包含重启命令，UI 必须显示 `output type violation`。

## 8. Tool Visibility Tab

Tool Visibility 回答“模型看到了什么工具、为什么看不到某些工具”。

### 8.1 Visible Tools

列：

- tool name。
- server / provider。
- risk level。
- mutating。
- approval requirement。
- result budget。
- visible reason。

### 8.2 Hidden Tools Denied

列：

- tool name。
- denied reason。
- policy rule。
- RBAC scope。
- resource scope。
- risk。
- replacement suggestion。

常见 denied reason：

- role_not_allowed。
- resource_out_of_scope。
- host_locked_by_other_session。
- missing_case_context。
- high_risk_requires_action_proposal。
- destructive_blocked。
- evidence_quality_insufficient。

### 8.3 Tool Policy Explainer

展示只读、低风险、高风险、破坏性工具的差异：

- readonly 可直接建议或调用。
- low_risk 视策略可能需要 approval。
- high_risk 必须生成 ActionProposal。
- destructive 默认阻断或需要 break-glass。

## 9. Prompt Compiler Preview

Prompt Compiler Preview 面向 reviewer 和开发者，用于在不发起真实模型调用时预览上下文拼装。

### 9.1 输入

- caseId。
- sessionId。
- user turn。
- target hosts / resource scope。
- evidence refs。
- enabled experience packs。
- desired output type。
- model。

### 9.2 输出

- prompt layers。
- token estimate。
- visible tools。
- hidden tools denied。
- redaction report。
- prompt fingerprint。
- warnings。

Preview 不能触发生产动作，也不能写入 case。它只能生成预览和可选的 PromptTrace draft。

## 10. Eval Reports

Eval Reports 回答“Prompt / Tool / Experience 变更是否让行为退化”。

### 10.1 Eval Run 表单

字段：

- eval suite。
- case category：incident、debug_case、middleware_repair、approval_safety、experience_activation。
- baseline prompt fingerprint。
- candidate prompt fingerprint。
- tool registry version。
- experience index version。
- model。
- sample size。

### 10.2 Eval Report 列表

| 列 | 内容 |
| --- | --- |
| Report | id、suite、createdAt |
| Change | prompt、tools、experience、policy |
| Pass Rate | 总体通过率 |
| Safety | 越权动作、错误工具、缺证据 |
| Regression | 新失败、修复、持平 |
| Action | 查看报告、打开失败 trace、创建经验候选 |

### 10.3 Eval Report 详情

展示：

- summary。
- failed cases。
- regression matrix。
- expected vs actual ReasoningOutput。
- evidence citation correctness。
- action governance correctness。
- missing evidence handling。
- trace links。
- suggested fixes。

失败 case 必须能跳到对应 Prompt Trace 和 case。

## 11. 嵌入式入口

### 11.1 Chat / Protocol Workspace

每个 assistant turn 增加：

- `Prompt Trace` 链接。
- ReasoningOutput badge。
- evidence refs 摘要。
- tool visibility warning。
- governance decision warning。

过程块中只展示用户可见推理摘要，不展示 raw chain-of-thought 或隐藏系统策略。

### 11.2 Case 工作台

Case 工作台复用：

- Reasoning Runs 摘要。
- 当前 hypothesis 对应的 trace。
- 证据请求。
- 动作提案草稿。
- 验证计划。
- 经验候选摘要。

### 11.3 DebugCase

DebugCase AI 结论需要链接：

- slow point classification 对应 Prompt Trace。
- supporting / contradicting evidence。
- generated ActionProposal draft。
- verification plan。

### 11.4 Middleware Repair

中间件修复工作台需要链接：

- RCA trace。
- matched RepairPlan / Runbook / Experience 的激活证据。
- no-match 时生成 RepairPlan draft 的 trace。
- 用户确认前后的治理决策。

### 11.5 Governed Action Plane

ActionProposal 详情中展示：

- source PromptTrace。
- model output type。
- evidence refs。
- risk reason。
- approval requirement。
- tool call preview。

## 12. 前端数据模型

新增 `web/src/api/aiReasoning.ts`，并将 `web/src/api/promptTraces.js` 迁移或扩展为 TypeScript：

```ts
export type ReasoningOutputType =
  | "answer"
  | "evidence_request"
  | "hypothesis_update"
  | "diagnostic_plan"
  | "action_proposal_draft"
  | "repair_plan_draft"
  | "runbook_draft"
  | "workflow_draft"
  | "verification_plan"
  | "postmortem_draft"
  | "experience_candidate_summary";

export interface PromptLayerView {
  name: string;
  source: string;
  tokenCount?: number;
  charCount?: number;
  redactionStatus: "visible" | "redacted" | "restricted";
  refs: PromptRefView[];
  hash?: string;
  contentPreview?: string;
  warnings: string[];
}

export interface PromptTraceView {
  id: string;
  caseId?: string;
  sessionId?: string;
  turnId?: string;
  model?: string;
  promptLayers: PromptLayerView[];
  visibleTools: ToolVisibilityView[];
  hiddenToolsDenied: HiddenToolDeniedView[];
  memoryRefs: PromptRefView[];
  evidenceRefs: PromptRefView[];
  opsGraphQueryRefs: PromptRefView[];
  activatedExperienceRefs: PromptRefView[];
  modelOutput?: ReasoningOutputView;
  toolCalls: ToolCallTraceView[];
  governanceDecisions: GovernanceDecisionView[];
  evalLabels: string[];
  fingerprints: PromptFingerprintView[];
  createdAt?: string;
  redactionStatus: "full" | "redacted" | "restricted";
}

export interface ReasoningOutputView {
  type: ReasoningOutputType;
  caseId?: string;
  title?: string;
  summary: string;
  confidence?: number;
  supportingEvidenceRefs: string[];
  contradictingEvidenceRefs: string[];
  risk: "low" | "medium" | "high" | "critical" | "unknown";
  downstreamTarget?: string;
  routingStatus: "none" | "drafted" | "created" | "blocked" | "violation";
}

export interface ToolVisibilityView {
  name: string;
  server?: string;
  risk: "readonly" | "low_risk" | "high_risk" | "destructive" | "unknown";
  mutating: boolean;
  approvalRequired: boolean;
  resultBudget?: number;
  visibleReason?: string;
}

export interface HiddenToolDeniedView {
  name: string;
  deniedReason: string;
  policyRule?: string;
  rbacScope?: string;
  resourceScope?: string;
  risk?: string;
  replacementSuggestion?: string;
}

export interface ToolCallTraceView {
  id: string;
  toolName: string;
  inputSummary?: string;
  normalizedInputHash?: string;
  status: "proposed" | "called" | "blocked" | "approved" | "failed" | "completed";
  approvalId?: string;
  evidenceRefs: string[];
  toolResultRefs: string[];
  outputSummary?: string;
  redactionStatus: "visible" | "redacted" | "restricted";
}

export interface GovernanceDecisionView {
  id: string;
  type: "rbac_clip" | "tool_hidden" | "approval_required" | "action_blocked" | "output_violation" | "safe";
  summary: string;
  severity: "info" | "warning" | "danger";
  refs: string[];
}

export interface EvalReportView {
  id: string;
  suite: string;
  status: "queued" | "running" | "passed" | "failed" | "canceled";
  passRate?: number;
  safetyFailures: number;
  regressions: number;
  fixedCases: number;
  traceRefs: string[];
  createdAt?: string;
}
```

## 13. API 契约

使用模块设计中的 API：

```text
POST   /api/v1/ai/chat
POST   /api/v1/ai/diagnose
POST   /api/v1/ai/plan
POST   /api/v1/ai/compile-prompt
GET    /api/v1/prompt-traces/{trace_id}
GET    /api/v1/cases/{case_id}/prompt-traces
POST   /api/v1/evals/run
GET    /api/v1/evals/reports/{id}
```

兼容当前已存在 API：

```text
GET    /api/v1/debug/model-input-traces
GET    /api/v1/debug/model-input-traces/file
```

建议新增：

```text
GET    /api/v1/ai/reasoning-runs
GET    /api/v1/ai/reasoning-runs/{run_id}
GET    /api/v1/prompt-traces
GET    /api/v1/prompt-traces/{trace_id}/diff
GET    /api/v1/prompt-traces/{trace_id}/raw
POST   /api/v1/prompt-traces/{trace_id}/eval-labels
GET    /api/v1/evals/reports
GET    /api/v1/evals/reports/{id}/cases
```

兼容逻辑必须封装在 API client 和 normalize 层。页面组件只消费统一 view-model。

## 14. 状态与数据流

```text
Chat / Case / DebugCase / Middleware Repair / Eval
  -> promptTraceId / caseId / sessionId / turnId
  -> AI Reasoning API client
  -> normalize PromptTraceView / ReasoningRunView / EvalReportView
  -> Reasoning workbench tabs
  -> Detail Drawer
```

Prompt Compiler Preview：

```text
Preview input
  -> POST /api/v1/ai/compile-prompt
  -> prompt layers + tool visibility + redaction report
  -> reviewer preview
  -> optional eval run
```

要求：

- 页面不直接调用裸 `fetch`。
- 页面不直接读取任意本地文件路径。
- traceId、caseId、sessionId、turnId、tab 写入 URL query。
- Raw trace、Prompt layers、Eval report 的 loading/error 状态相互隔离。
- 同一个 trace id 在当前页面内复用缓存，避免不同 tabs 展示不一致。

## 15. 错误、空态与加载态

错误态：

- trace 不存在：显示 trace id、caseId、sessionId、重新搜索入口。
- trace 文件损坏：保留列表和元数据，详情显示解析失败。
- 权限不足：显示 redacted summary，不显示系统策略和敏感上下文。
- eval runner 不可用：禁用运行 Eval，保留报告浏览。
- raw trace 读取失败：不影响 Overview、Layers、Tools 和 Governance tabs。

空态：

- 无 Reasoning Runs：提示从 Chat、Case 或 DebugCase 进入。
- 无 Prompt Trace：提示确认 trace 开关和数据目录。
- 无 tool calls：显示“本轮没有工具调用”，而不是错误。
- 无 governance warning：显示 safe 状态。
- 无 eval report：显示运行 Eval 表单。

加载态：

- Trace 列表和 Trace 详情分开 loading。
- Prompt layers 使用稳定行高 skeleton。
- Raw tab lazy load，避免阻塞第一屏。

## 16. 安全与治理显示规则

- 普通用户不能查看完整 System Policy、Developer Policy、RBAC 细节和隐藏工具完整原因，只能查看安全摘要。
- reviewer 可查看更多细节，但 Raw 默认折叠。
- 页面不能显示 raw chain-of-thought，只显示模型输出摘要、ReasoningOutput 和可审计决策。
- SecretRef 解密结果、token、password、cookie、authorization header、私钥、完整连接串不能出现在 DOM。
- 敏感 evidence 只显示 EvidenceRef 和脱敏摘要。
- 未审核经验不能显示为 activated experience，只能显示为 candidate 或 denied。
- 写动作必须通过 ActionProposal / Workflow draft / RepairPlan draft 表示，不能从自然语言回答直接执行。
- high_risk 和 destructive 工具必须显示 approval requirement 或 blocked reason。
- Eval 失败不能自动修改 prompt、工具 schema 或经验包，只能生成建议或候选变更。

## 17. 验收标准

- 每轮 AI turn 在 Chat、Protocol Workspace、Case 工作台中都有 Prompt Trace 入口。
- `/debug/prompts` 继续可用，并升级为统一 Prompt Trace Explorer 或跳转到 `/ai-reasoning`。
- Prompt Trace 详情能展示 Prompt layers、visible tools、hiddenToolsDenied、Evidence refs、OpsGraph query refs、Experience refs、model output、tool calls、governance decisions、diff 和 raw。
- 普通用户看不到完整系统策略、敏感 evidence、SecretRef 解密结果或 raw chain-of-thought。
- ReasoningOutput 以类型化方式展示，并能看到流向 Hypothesis、ActionProposal、Workflow draft、RepairPlan、VerificationPlan、Experience candidate。
- AI 写动作如果没有进入 ActionProposal / Workflow draft / RepairPlan draft，UI 显示治理违规。
- Tool Visibility 能解释工具可见和不可见原因。
- Prompt Diff 能展示 layer、tool、evidence、experience、fingerprint 的变化。
- Eval Reports 能展示 pass rate、安全失败、回归、新失败和失败 trace。
- 页面请求都经过 `web/src/api/aiReasoning.ts` 或迁移后的 `web/src/api/promptTraces.ts`，页面不保留裸 `fetch`。
