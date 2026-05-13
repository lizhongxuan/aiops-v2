# aiops-v2 运维经验包与自我演化设计

日期：2026-05-11
范围：`aiops-v2` Coroot 集成、用户侧慢请求调试、中间件自治修复、事故工作台、Runbook 目录、Runner Studio 工作流编排与运行系统。
目标：把真实 AI Chat 运维、浏览器插件 Debug Trace 和中间件修复过程蒸馏成可审核、可复用、可执行、可验证、可演化的运维经验包。经验包采用简单版 `EvoMap + Runner workflow engine + Skills 经验蒸馏`：EvoMap 只做轻量演化账本，Runner 只复用现有工作流引擎和 catalog，Skills 只定义经验蒸馏格式；自动生成内容先进入候选，只有经过人工审核后，才能影响 AI Chat 推荐、发布为 Runbook 或发布为 Workflow。
前端设计：[2026-05-11-aiops-v2-10a-experience-packs-frontend-design.zh.md](2026-05-11-aiops-v2-10a-experience-packs-frontend-design.zh.md)
实施清单：[2026-05-11-aiops-v2-10b-experience-packs-frontend-todo.zh.md](2026-05-11-aiops-v2-10b-experience-packs-frontend-todo.zh.md)

关系：本专题是 09 Learning 的 Experience Pack 细化，不新建第二套 Learning 控制面。Candidate Store、Activation Index、Review Queue、Memory 和 Eval 仍按 09 的统一契约实现。

## 0. 设计定位

经验包是 `aiops-v2` 从“能解决一次问题”进化到“越用越会运维”的核心资产层。但它不能变成普通用户必须学习的一套复杂概念，也不能变成自动修改生产的黑盒。

本方案采用三段式设计，但保持简单版本：

```text
Skills 经验蒸馏
  -> 把真实 case 提炼成 ProblemSignature / EvidenceRecipe / DiagnosisLogic / ActionRecipe / Guardrails / Verification

简单版 EvoMap 演化账本
  -> 记录经验从哪里来、在哪里成功/失败、如何变体、何时降权或替代

现有 Runner workflow 引擎
  -> 只绑定并复用现有 Runner workflow / Runbook，不修改 Runner 页面和功能
```

用户侧呈现要足够简单：

- 普通用户只在现有 AI Chat / Case 中看到“命中了哪个经验、为什么适用、风险是什么、是否要执行”。
- SRE/平台管理员在经验包审核台看证据链、技能内容、EvoMap lineage、Runner workflow 绑定和发布门禁。

第一阶段必须支持：

- 从 DebugCase 生成 Debug RCA candidate。
- 从 PG 修复生成 RepairPlan candidate。
- 从候选派生 Runbook draft / Workflow draft。
- 人工审核后写入 Activation Index。
- AI Chat、DebugCase 和 Middleware Repair 只读取 Activation Index。
- 不修改现有 AI Chat 页面和 Runner 工作流页面；只通过后端服务、工具注册、Case API、经验索引和 Runner 运行 API 接入。

Evolution Map 先作为后台 append-only 账本和 lineage 查询实现；大型图谱 UI、复杂环境变体自演化、fitness 自动调参和 Artifact Studio 不作为当前设计的前置依赖。

## 1. 背景与现状

当前 `aiops-v2` 已具备以下基础：

- Coroot 已接入为只读工具和 webhook 来源：可读取服务列表、服务指标、RCA、拓扑、SLO、告警规则、事故时间线，并可把 Coroot webhook 映射为事故与证据。
- 事故与证据模型已经存在：事故可包含 Coroot evidence、假设排序和关闭后的 postmortem draft。
- Runbook 目录已经存在：`internal/runbooks` 支持 YAML catalog、匹配、实例进度、动作提案和 action token。
- Runner Studio 已具备工作流编排闭环：Visual Workflow 支持图模型、保存 draft、校验、dry-run、提交运行、审批节点、运行事件、发布状态和审计。
- `ExperiencePacksPage` 目前主要是静态数据页面，还没有后端经验包服务。
- 浏览器插件慢请求调试和中间件修复需要成为新的经验来源：前者把一次按钮点击到后端链路的 trace 转成 DebugCase，后者把一次 PG 修复转成 `middleware_repair` case。

EvoMap/Evolver 的核心启发是：不要让系统自由修改自身，而是把运行历史提炼成受协议约束、可审计、可复用的演化资产。它通过 memory 扫描、Gene/Capsule 选择、EvolutionEvent 审计、review mode 和可回滚机制，把临时经验变成可治理资产。AIOps 借鉴这些思想，但资产对象换成运维 Gene、Capsule、Debug RCA、RepairPlan、Runbook、Workflow 和环境变体。

因此本方案不另起一套执行系统，也不改现有 Runner 工作流页面，而是在现有 Incident、Runbook、Runner Workflow、Approval、Audit 之上新增“经验蒸馏层”“简单 EvoMap 账本”“经验包审核层”和“环境适配记录”。

## 2. 设计原则

1. 技能化蒸馏：经验包保存可复用判断和操作技能，不保存原始日志堆。
2. 候选和发布强隔离：自动生成的内容只能进入 candidate / draft，不能直接进入推荐、Runbook 正式目录或 Workflow 正式列表。
3. 发布资产不可变：已发布 Runbook / Workflow 不原地自改，任何演化都生成新版本或环境变体。
4. 证据驱动：经验包必须保存来源、适用环境、操作结果、验证结果、失败原因和审核记录。
5. 环境优先匹配：匹配 Runbook / Workflow 前先匹配环境画像，避免 Linux、macOS、Kubernetes、不同发行版之间误用。
6. Runner 负责执行：经验包只绑定现有 Runner workflow / Runbook，不直接执行生产动作，也不要求改 Runner 页面。
7. 与现有门禁复用：Workflow 继续使用 validate、dry-run、publish、risk acknowledgement、audit；生产写动作继续走 Governed Action。
8. AI Chat 推荐强门禁：未审核 Gene、Capsule、Debug RCA、RepairPlan、经验包、Runbook draft、Workflow draft 不能进入 PromptCompiler、检索排序、推荐提示或自动建议上下文。
9. 先观察再固化：系统可以自动记录和生成候选，但不能把候选当成最佳实践；只有经过 reviewer 批准并写入 Activation Index 的资产才能影响下一次对话。

## 3. 已确认架构

本设计采用 `Experience Pack + Simple EvoMap + Runner Artifact Binding` 作为资产架构。

系统自动生成“经验包候选”，并把每次 AI Chat、浏览器插件 Debug Trace、Coroot 证据、中间件修复、工具调用、审批、Workflow 运行和结果验证写入 Simple EvoMap。经验包可包含 SkillCard、Debug RCA candidate、RepairPlan draft、Runbook draft、Workflow graph draft、环境画像、证据链、审核记录、Runner artifact binding 和版本 lineage。Gene/Capsule 可以作为后续细分能力，但当前简单版不要求先落地。人工审核后，系统按审核决定激活 SkillCard/Debug RCA/RepairPlan，或发布到 Runbook、Workflow。

这个架构把四件事解耦：

- 经验蒸馏：Experience Pack 负责把 case 提炼成可复用技能。
- 演化关系：Simple EvoMap 负责 lineage、环境、成功/失败反馈和降权，不要求复杂图 UI。
- 推荐入口：Activation Index 负责已审核经验进入 AI Chat、DebugCase 和 Middleware Repair。
- 执行底座：Runbook / Runner Workflow 负责最终固化和执行。

## 4. 核心概念

### SkillCard

SkillCard 是经验包面向 AI 推理的最小技能描述，类似 GPT Skills 的蒸馏结果。它不直接执行动作，只告诉 AI “什么时候适用、需要什么证据、如何判断、下一步建议是什么”。

```yaml
id: skill_pg_lock_wait_checkout_v1
title: PG lock wait 导致 checkout 提交慢
status: candidate | approved | deprecated
problem_signature:
  symptoms: [checkout_submit_slow, api_p95_spike]
  trace_pattern: frontend->gateway->checkout-api->postgres
  slow_span_type: db.lock_wait
evidence_recipe:
  required:
    - trace.summary
    - coroot.service_metrics
    - pg.lock_snapshot
    - pg.stat_activity
  optional:
    - recent_change
    - slow_query_fingerprint
diagnosis_logic:
  summary: 如果 checkout-api 慢 span 集中在 PG lock wait，且存在长事务 blocker，则优先判断锁等待。
action_recipe:
  readonly_first:
    - inspect blocking query
    - identify owner and age
  remediation_candidates:
    - terminate confirmed idle-in-transaction blocker
guardrails:
  disabled_conditions:
    - blocker is migration process
    - no recent backup
    - replication lag above threshold
verification:
  checks:
    - lock wait below threshold
    - checkout p95 recovered
    - new debug trace no longer waits on PG lock
runner_bindings:
  diagnostic_workflow_id: wf_pg_lock_diagnose
  remediation_workflow_draft_id: wf_draft_pg_lock_remediate
```

SkillCard 被审核激活后可以进入 Activation Index；未审核时只能在经验包审核台展示。

### Simple EvoMap

Simple EvoMap 是 aiops-v2 在目标环境里的运维经验演化账本。它不是普通用户要看的大图，也不是复杂自我进化系统，而是一组 append-only 事件和轻量索引，用来回答经验来源、适用环境、成功失败、版本关系和 Runner 绑定。

核心节点：

- `EnvironmentProfile`：环境画像。
- `ProblemSignature`：问题签名，例如 SLO burn、5xx spike、DB saturation。
- `DebugTraceEvent`：用户侧慢请求调试事件，包含页面动作、trace id、前端耗时和后端链路摘要。
- `TraceContext`：跨浏览器、网关、服务、中间件和数据库的 trace 关联对象。
- `MiddlewareResource`：PG、Redis、MQ、Elasticsearch 等中间件资源画像。
- `OperationEvent`：一次 AI Chat 运维中的计划、工具、审批和执行过程。
- `RepairPlan`：中间件修复的计划、风险、回滚和验证草稿。
- `RecoveryAttempt`：一次修复执行及其成功、失败、回滚、验证结果。
- `OutcomeMetric`：Coroot 或验证工具确认的结果。
- `ExperiencePack`：候选和审核容器。
- `RunbookVersion`：人工固化的人类操作方案。
- `WorkflowVersion`：人工固化的可执行 DAG。
- `ReviewRecord`：审核、拒绝、退回、发布记录。
- `RunnerArtifact`：绑定的 Runbook、Workflow、验证 workflow 或回滚 workflow。

核心边：

- `observed_in`：问题出现在哪个环境。
- `activated`：一次 Chat 使用了哪个已审核 SkillCard/Debug RCA/RepairPlan。
- `produced_candidate`：一次操作生成了哪些候选资产。
- `derived_from_trace`：候选资产来自哪个用户侧 trace 或慢请求 DebugCase。
- `derived_from_repair`：候选资产来自哪个中间件 RepairPlan 或 RecoveryAttempt。
- `verified_by`：哪些指标或检查证明有效。
- `variant_of`：某资产是哪个基础资产的环境变体。
- `supersedes`：新版本替代旧版本。
- `incompatible_with`：在哪些环境不能推荐或执行。
- `executes_via`：某经验绑定哪个 Runner artifact。

### Gene / Capsule

Gene 和 Capsule 是后续可选的细粒度经验形态。当前简单版可以先用 SkillCard 表达判断规则和完整处置套路，不要求先拆成 Gene/Capsule。

示例：

```yaml
id: gene_nginx_reload_precheck_systemd
status: approved
scope:
  problem_signature: nginx_reload_5xx_spike
  environment_profile: linux/systemd
rule:
  summary: nginx reload 前必须先执行 nginx -t
  recommendation: 如果 nginx -t 失败，禁止 reload，并优先检查配置变更。
evidence_refs:
  - op_evt_20260511_001
fitness:
  success_rate: 0.91
  activation_count: 11
review:
  decision: approved
  reviewer: sre
```

只有 `status=approved` 且写入 Activation Index 的 Gene/Capsule 才能被 AI Chat 检索和推荐；未实现 Gene/Capsule 时，SkillCard 承担同样入口。

### Capsule

Capsule 是完整处置模式，包含适用条件、诊断步骤、动作步骤、验证项、回滚、失败边界和环境兼容性。Capsule 可以派生 Runbook 或 Workflow。

Gene 适合“下一步怎么判断”，Capsule 适合“这个问题的完整处理套路是什么”。

### Experience Pack

经验包是可审核、可版本化、可派生的运维技能容器。简单版优先保存 SkillCard、Debug RCA、RepairPlan，以及 Runbook/Workflow draft 等执行资产的绑定关系。

关键字段：

```yaml
id: exp_nginx_reload_001
title: Nginx reload 后 5xx 突增处理经验
status: candidate | review_pending | approved | rejected | published | deprecated
scenario:
  symptoms: [http_5xx_spike, latency_p95_spike]
  entities: [service.checkout, deployment.nginx]
  sources: [coroot, chat, frontend_debug, trace, middleware_repair, workflow_run, approval_audit, postmortem]
environment_profile:
  os_family: linux
  distro: ubuntu
  init_system: systemd
  runtime: kubernetes
  arch: amd64
  agent_capabilities: [shell.run, kubectl, systemctl]
confidence:
  score: 0.82
  evidence_count: 6
  successful_runs: 3
  failed_runs: 1
artifacts:
  skill_card_ids: [skill_pg_lock_wait_checkout_v1]
  gene_candidate_ids: [gene_nginx_reload_precheck_systemd]
  capsule_candidate_id: capsule_nginx_reload_recovery_001
  runbook_draft_id: rb_draft_nginx_reload_001
  workflow_draft_id: wf_draft_nginx_reload_001
  debug_trace_signature_id: ""
  repair_plan_draft_id: ""
runner_bindings:
  diagnostic_workflow_id: ""
  remediation_workflow_draft_id: wf_draft_nginx_reload_001
  rollback_workflow_id: ""
  verification_workflow_id: ""
activation:
  eligible_for_chat: false
  activation_indexed_at: ""
lineage:
  base_pack_id: ""
  parent_version: ""
  variant_key: linux/ubuntu/systemd/k8s
review:
  required_roles: [sre, service_owner]
  reviewers: []
  decision: pending
```

### Environment Profile

环境画像用于判断经验能不能安全复用。

采集来源：

- runner agent heartbeat / probe。
- Coroot project、namespace、service、deployment、topology。
- 只读命令探测：`uname`、`os-release`、`systemctl --version`、`kubectl version`、package manager、shell、权限级别。
- 工作流运行上下文：inventory、host vars、agent capabilities、action catalog precheck。
- 用户侧调试上下文：页面、路由、动作、trace id、接口耗时、慢 span、错误 span。
- 中间件上下文：集群角色、主从状态、复制延迟、连接数、锁等待、磁盘水位、WAL、备份和 failover 风险。

匹配维度：

- 操作系统：Linux distro/version、macOS、Windows、容器镜像。
- 运行形态：bare metal、VM、Docker、Kubernetes、systemd、launchd。
- 权限和工具：root/sudo、kubectl、helm、systemctl、journalctl、ps/top/free/df。
- 业务上下文：Coroot service、namespace、project、SLO、依赖拓扑。

### Candidate Artifact

经验包候选可以同时产生多类草稿：

- SkillCard Candidate：面向 AI 推理的经验蒸馏卡，描述适用条件、证据配方、诊断逻辑、动作建议、禁用条件和验证方式。
- Gene Candidate：面向 AI Chat 的小粒度推荐规则，简单版可延后。
- Capsule Candidate：面向场景的完整处置模式，简单版可由 SkillCard 先承载。
- Runbook Draft：面向人，描述适用条件、步骤、验证项、回滚、审批要求。
- Workflow Graph Draft：面向 Runner Studio，包含可执行 DAG、变量、审批节点、dry-run 输出、风险摘要。
- Debug RCA Candidate：面向用户侧慢请求调试，描述 trace 签名、慢点定位、证据链和可自动化处置建议。
- RepairPlan Draft：面向中间件自治修复，描述假设、诊断步骤、修复步骤、风险、回滚和验证。

候选草稿不出现在正式 Runbook / Workflow 列表，也不进入 AI Chat、DebugCase 或中间件修复推荐索引，只在“经验包审核队列”展示。

### Activation Index

Activation Index 是 AI Chat、DebugCase 和中间件修复流程唯一可用的经验检索入口，只包含人工审核通过的 SkillCard、Debug RCA、RepairPlan、Runbook 和 Workflow metadata；Gene/Capsule 以后实现时也必须走同一索引。Candidate Store 和 Activation Index 必须物理或逻辑隔离。

PromptCompiler、Runbook matcher、Workflow 推荐器、Debug RCA matcher 和 RepairPlan matcher 只能读取 Activation Index。Candidate Store 只允许审核页面、经验包详情页和离线评估访问。

## 5. 总体架构

```text
Coroot / Browser Plugin Debug / Trace / Middleware Repair / Chat / Tool / Workflow / Approval / Postmortem / Outcome
        |
        v
Experience Collector -> Append-only EvolutionEvent
        |
        v
Case Builder + Secret Redactor + Evidence Normalizer
        |
        v
Environment Fingerprinter
        |
        v
Skill Distiller + Candidate Synthesizer
        |
        v
Candidate Store  ->  Review Queue
        |                  |
        |                  v
        |          Human Review + Validate + Dry Run
        |                  |
        v                  v
Simple EvoMap      Activation Index + Runbook / Runner Workflow
        |
        v
Next AI Chat / Debug / Repair recommendation
```

新增后端模块建议：

- `internal/experience`：经验包领域模型、候选生成、环境变体、匹配和审核。
- `internal/evolutionmap`：EvolutionEvent、Simple EvoMap、Activation Index、基础 fitness 和 lineage 查询。
- `internal/experience/skilldistill`：从真实 case 蒸馏 SkillCard、EvidenceRecipe、Guardrails 和 VerificationRecipe。
- `internal/appui/experience_service.go`：appui 层服务接口，供 HTTP 和前端使用。
- `internal/server/experience_api.go`：`/api/v1/experience-packs/*` API。
- Debug Trace 与 Middleware Repair 不在本专题内新建事实源。如果现有代码需要适配层，可增加 thin adapter，但 `DebugEvent` / `TraceContext` 仍归 04，`MiddlewareResource` / `RepairPlan` / `RecoveryAttempt` 仍归 08。
- `web/src/pages/ExperiencePacksPage.tsx`：从静态数据改为后端 API 驱动。
- Runner 集成只使用现有 workflow draft / validate / dry-run / run / publish 能力；不要求修改 Runner Studio 页面。

## 6. 从 AI Chat 到经验包的正向飞轮

### 6.1 环境启动期

aiops-v2 部署到环境后，先建立基础环境画像和初始 Evolution Map：

1. Coroot 提供 service、SLO、topology、RCA、timeline。
2. runner agent 提供 OS、runtime、权限、可用 action、工具能力。
3. Runbook / Workflow 目录提供已有 approved/published 资产。
4. 浏览器插件和 gateway/backend tracing 提供 trace id 贯通能力。
5. 中间件探针或只读工具提供 PG/Redis/MQ 等资源画像。
6. 系统生成 `EnvironmentProfile`，但不自动生成最佳实践。

### 6.2 AI Chat 运维期

用户在 AI Chat 中提出运维请求后，Chat 侧流程固定为：

1. 解析问题签名：服务、症状、环境、SLO、时间窗口。
2. 从 Activation Index 检索已审核 SkillCard/Debug RCA/RepairPlan/Runbook/Workflow。
3. 检查环境兼容性和禁用条件。
4. 调用 Coroot 工具补充实时证据。
5. 如果来源是用户侧慢请求调试，关联 DebugEvent、TraceContext、前端耗时、慢 span、后端服务路径和 Coroot RCA。
6. 如果来源是“修复 xxx 中间件集群”，先构建 MiddlewareResource 和 RepairPlan，再判断是否命中已审核经验。
7. 给出计划：说明推荐来源、适用环境、风险、验证项、回滚方式和 Runner 执行绑定。
8. 只读诊断可直接执行；有风险动作继续走现有审批和权限系统。
9. 每个计划、工具调用、审批、结果都写入 `OperationEvent`。

硬约束：Candidate Store 不参与步骤 2。未审核候选不能影响推荐排序、提示词、默认计划或自动建议。

### 6.3 执行验证期

执行完成后，系统不能只看命令退出码，还要进入 observation window：

- 重新读取 Coroot SLO、错误率、延迟、资源指标和拓扑状态。
- 对 DebugCase 重新采集或重放用户侧 trace，比较页面动作耗时、接口耗时和慢 span。
- 对中间件 RepairPlan 检查集群角色、复制延迟、锁等待、连接数、磁盘水位、备份状态和业务依赖恢复。
- 执行 Runbook / Workflow 中定义的 verification。
- 记录用户反馈：解决、部分解决、未解决、误报。
- 生成 `OutcomeMetric`，更新本次 OperationEvent 的结果。

### 6.4 经验提炼期

Experience Miner 基于 OperationEvent 和 OutcomeMetric 生成候选：

- 成功且可复用的问题判断，生成 SkillCard candidate。
- 成功且可复用的判断，可以生成 Gene candidate；简单版也可以先合并进 SkillCard。
- 成功且步骤完整的处置，可以生成 Capsule candidate；简单版也可以先合并进 SkillCard 或 Runbook draft。
- 面向人工执行的内容，生成 Runbook draft。
- 步骤稳定且可自动化的内容，生成 Workflow graph draft。
- 用户侧慢请求 DebugCase 生成 Debug RCA candidate、trace 签名、慢点定位 Gene 或前后端联动处置 Capsule。
- 中间件修复 case 生成 RepairPlan draft、中间件恢复 Capsule、Runbook draft 或 Workflow graph draft。
- 失败或误用，生成 Anti-pattern candidate 或 incompatibility edge。

这些候选只进入 Candidate Store 和 Review Queue。

### 6.5 人工固化期

Reviewer 审核候选时，可以：

- 批准 SkillCard：写入 Activation Index，可影响 AI Chat、DebugCase 和 Middleware Repair 的推理与推荐。
- 批准 Gene/Capsule：如果已实现，写入 Activation Index；简单版可先不实现独立 Gene/Capsule。
- 批准 Debug RCA：写入 Activation Index，可被相似用户侧慢请求调试复用。
- 批准 RepairPlan：写入 Activation Index，可被相似中间件修复 case 复用。
- 批准 Runbook：进入 Runbook catalog。
- 批准 Workflow Draft：进入 Runner Studio draft，并继续走 validate/dry-run/publish。
- 退回修改：保留候选和 reviewer 意见。
- 拒绝：写入 rejected，不推荐。

### 6.6 下一轮优化期

下一次相似问题出现时，AI Chat 会优先激活已审核、环境兼容、fitness 更高的 SkillCard/Debug RCA/RepairPlan。新的执行结果继续回写 Simple EvoMap，形成：

```text
更多真实运维与调试修复 -> 更多证据 -> 更准确的 Skill/RepairPlan -> 更好的推荐和 Workflow
-> 更短 MTTR / 更低风险 -> 更高 fitness -> 更精准的下一次推荐
```

## 7. 自动沉淀流程

### 7.1 事件采集

Collector 订阅以下来源：

- Coroot webhook：事故标题、服务、严重级别、raw ref、evidence。
- Coroot tools：metrics、RCA、SLO、topology、timeline 的结构化结果。
- Browser Plugin Debug：页面、路由、用户动作、trace id、插件注入状态、前端耗时、接口耗时、脱敏 headers 摘要。
- Trace backend：span path、慢 span、错误 span、DB span、cache span、MQ span、gateway span。
- Middleware repair：集群拓扑、角色、复制、锁等待、连接数、磁盘水位、WAL、备份、failover 状态和只读诊断结果。
- Chat / Runtime event：用户意图、模型计划、工具调用、审批等待和审批结果。
- Workflow run：workflow name、graph hash、节点状态、stdout/stderr 摘要、导出变量、最终结果。
- Runbook instance：匹配结果、步骤进度、action proposal、observe result。
- Incident close：root cause、mitigation、follow ups、postmortem timeline。
- Outcome verification：Coroot 指标恢复、workflow verification、用户确认和失败原因。

所有采集数据先经过 secret redaction 和 size limit，只保存引用、摘要和 hash，不把敏感原始输出直接写入经验包正文。

### 7.2 Case Builder

Case Builder 把一次事故或一次操作会话规整为 `ExperienceCase`：

- `problem_signature`：症状、指标、服务、拓扑、错误关键字。
- `trace_signature`：如果来自 DebugCase，保存页面动作、trace path、慢 span 类型、服务路径和慢点位置。
- `middleware_signature`：如果来自中间件修复，保存中间件类型、集群角色、异常模式、风险点和受影响业务。
- `actions_taken`：执行了哪些工具或 workflow 节点。
- `outcome`：成功、失败、回滚、部分成功。
- `verification`：哪些指标恢复、哪些检查通过。
- `operator_decisions`：人工审批、拒绝原因、风险确认。
- `environment_profile_id`：当时环境画像。
- `activated_assets`：本次 AI Chat 使用了哪些已审核 SkillCard、Debug RCA、RepairPlan、Runbook 或 Workflow。

相似 case 会聚合到同一个经验包候选，避免每次运行都生成一个新包。

### 7.3 Skill Distiller 与 Candidate Synthesizer

Skill Distiller 和 Synthesizer 只生成候选，不发布。

生成策略：

- 从一次成功或失败 case 中提炼 SkillCard：问题签名、证据配方、诊断逻辑、动作建议、禁用条件、验证方式。
- 从稳定判断生成 Gene candidate。
- 从完整处置路径生成 Capsule candidate。
- 从成功 case 生成初版 Runbook。
- 从连续成功且步骤稳定的 case 生成 Workflow graph draft。
- 从用户侧慢请求 case 生成 Debug RCA candidate：包含 trace 签名、前端/后端/中间件慢点定位、证据链、建议和可自动化修复边界。
- 从中间件修复 case 生成 RepairPlan draft：包含 RCA、诊断步骤、修复步骤、回滚、验证、禁用条件和审批要求。
- 从失败 case 生成“禁用条件、风险提示、环境不适配说明”。
- 从修复失败 case 生成 failure capsule 或 incompatibility edge，避免下次继续推荐同一路径。
- 从 postmortem follow-up 生成补充验证项候选或前置检查。

生成的 workflow draft 必须带标签：

```yaml
labels:
  source: experience-pack
  experience_pack_id: exp_nginx_reload_001
  environment_profile_id: env_linux_ubuntu_systemd_k8s
```

并且默认状态为 draft，必须走现有 validate -> dry-run -> publish。

## 8. 人工审核与发布门禁

审核队列展示：

- 经验包摘要、适用场景、环境画像、置信度。
- 证据链：Coroot evidence、workflow run、tool result ref、approval audit、postmortem。
- SkillCard diff：新增了哪些问题签名、推荐规则、触发条件、禁用条件、证据配方、诊断逻辑、guardrails、verification 和 Runner bindings。
- Gene/Capsule diff：如果后续拆分 Gene/Capsule，再展示小粒度规则和完整处置模式差异。
- Debug RCA diff：新增了哪些 trace 签名、慢点定位规则、可自动化建议和证据约束。
- RepairPlan diff：新增了哪些中间件诊断步骤、修复步骤、风险、回滚、验证和禁用条件。
- Runbook diff：新增/修改了哪些步骤、验证项、回滚。
- Workflow diff：节点、边、action、变量、审批节点、风险变化。
- 安全检查：高风险 action、权限要求、环境不匹配、未覆盖回滚。

审核动作：

- `approve_skill`：写入 Activation Index，允许影响 AI 推理和推荐。
- `approve_gene`：可选后续动作，写入 Activation Index，允许影响 AI Chat 推荐。
- `approve_capsule`：可选后续动作，写入 Activation Index，允许作为 Chat 处置模式。
- `approve_debug_rca`：写入 Activation Index，允许相似 DebugCase 推荐诊断路径。
- `approve_repair_plan`：写入 Activation Index，允许相似中间件修复 case 推荐处置路径。
- `approve_as_runbook`：写入 Runbook draft catalog，状态为 reviewed 或 published。
- `approve_as_workflow_draft`：写入 Runner Workflow draft，仍需 validate/dry-run/publish。
- `approve_both`：同时生成 Runbook 和 Workflow draft。
- `request_changes`：退回候选并记录意见。
- `reject`：保留证据但不再推荐。

进入正式列表的规则：

- AI Chat 推荐只读取 Activation Index，不读取 candidate。
- SkillCard/Debug RCA/RepairPlan 只有人工审核通过后才能进入 Activation Index；Gene/Capsule 后续实现时同样适用。
- Runbook 列表只展示 `approved/published`。
- Workflow 列表只展示现有 `draft/validated/dry_run_passed/published`，但由经验包生成的 draft 必须带 lineage 和 review record。
- Candidate 永远不进入 AI Chat 推荐、DebugCase 推荐、中间件修复推荐、正式 Runbook 或正式 Workflow 列表。

## 9. 自我演化与环境变体

### 9.1 不原地修改

已发布资产不可变。系统遇到新环境时，只能生成：

- 新经验包候选；
- 既有经验包的新 variant；
- 既有 Runbook / Workflow 的新 draft version。

原版本继续保持原有 semantic hash、published graph hash 和适用环境，不受影响。

### 9.2 变体模型

```yaml
base_pack_id: exp_nginx_reload_001
variant_id: exp_nginx_reload_001__macos_launchd
variant_key: macos/launchd/arm64
compatibility:
  os_family: macos
  init_system: launchd
  forbidden_actions: [systemctl]
  required_actions: [launchctl, shell.run]
changes:
  - replace: systemctl reload nginx
    with: brew services restart nginx
  - add_verification: launchctl list | grep nginx
status: review_pending
```

Variant Resolver 匹配时按以下顺序选择：

1. 精确环境变体。
2. 同 OS family 的兼容变体。
3. 基础经验包的 read-only 诊断步骤。
4. 无匹配时只生成待审核候选，不进入 AI Chat 推荐。

### 9.3 演化触发条件

- 当前环境和经验包 compatibility 不匹配。
- 同一个 Runbook / Workflow 在某类环境连续失败。
- Coroot topology 显示运行形态变化，例如服务迁移到 Kubernetes。
- 操作系统、包管理器、init system、agent capability 发生变化。
- 人工审核时选择“为当前环境创建变体”。

### 9.4 适配策略

适配分三层：

- 参数适配：host、namespace、service、threshold、timeout、路径等。
- 动作适配：`systemctl` -> `launchctl`，`apt` -> `yum/brew`，裸机命令 -> `kubectl rollout`。
- 编排适配：单机顺序流程 -> 多 pod 并行检查、Kubernetes readiness gate、人工审批节点。

所有动作适配都必须经过 action catalog、capability precheck、dry-run 和人工审核。

## 10. Fitness 与降权机制

每个已审核 SkillCard/Debug RCA/RepairPlan/Runbook/Workflow 都维护基础 fitness，用于推荐排序和演化判断；Gene/Capsule 后续实现时复用同一机制。

关键指标：

- `activation_count`：被 AI Chat 激活次数。
- `success_rate`：激活后问题解决比例。
- `mttr_delta`：相比未使用经验时的 MTTR 改善。
- `rollback_rate`：触发回滚比例。
- `rejection_rate`：人工审批拒绝比例。
- `environment_mismatch_rate`：环境不兼容触发比例。
- `cost`：工具调用、token、运行时长。
- `freshness`：最近成功时间和最近失败时间。

降权规则：

- 连续失败达到阈值后，从推荐索引降权，并生成 repair candidate。
- 某环境失败集中出现时，不修改原资产，而是新增 `incompatible_with` 边或 variant candidate。
- reviewer 可以手动 pause 一个 SkillCard/Debug RCA/RepairPlan，使其暂时不影响 Chat 推荐。
- deprecated 资产保留 lineage 和历史，不再进入 Activation Index。

## 11. API 草案

```text
GET    /api/v1/evolution-map/entities/{id}
GET    /api/v1/evolution-map/lineage/{asset_id}
GET    /api/v1/evolution-map/events
POST   /api/v1/evolution-map/query
GET    /api/v1/activation-index/search
GET    /api/v1/experience-packs
GET    /api/v1/experience-packs/{id}
POST   /api/v1/experience-packs/match
GET    /api/v1/experience-packs/review-queue
POST   /api/v1/experience-packs/{id}/review
POST   /api/v1/experience-packs/from-debug-trace
POST   /api/v1/experience-packs/from-repair-case
POST   /api/v1/experience-packs/{id}/distill-skill
POST   /api/v1/experience-packs/{id}/generate-runbook-draft
POST   /api/v1/experience-packs/{id}/generate-workflow-draft
POST   /api/v1/experience-packs/{id}/variants
GET    /api/v1/debug-traces/{trace_id}/experience-candidates
GET    /api/v1/repair-cases/{case_id}/experience-candidates
GET    /api/v1/environment-profiles
GET    /api/v1/environment-profiles/{id}
POST   /api/v1/environment-profiles/probe
```

`review` 请求示例：

```json
{
  "decision": "approve_as_workflow_draft",
  "reviewer": "sre",
  "comment": "已确认仅适用于 Ubuntu + systemd",
  "required_dry_run": true
}
```

## 12. 前端体验

### 经验包库

将当前静态 `ExperiencePacksPage` 改为 API 驱动，增加：

- 候选、待审核、已发布、已拒绝 tabs。
- 环境画像筛选：OS、runtime、namespace、service、agent capability。
- SkillCard 候选列表和审核状态。
- Gene/Capsule 可作为后续高级筛选，不是简单版首要页面。
- Debug RCA 和 RepairPlan 候选列表，展示 trace 签名、中间件类型、适用环境、成功率和禁用条件。
- 证据链视图。
- 版本和变体树。
- “生成 Runbook Draft”“生成 Workflow Draft”“创建环境变体”按钮。
- Workflow Draft 只写入现有 Runner workflow draft/catalog；不要求修改 Runner 页面。

### 审核页面

审核页需要三块核心内容：

- Evidence：事故、Coroot、运行日志摘要、审批记录。
- AI Chat Impact：批准后会影响哪些推荐场景、哪些环境、哪些问题签名。
- Artifact Diff：Runbook diff 和 Workflow graph diff。
- Debug / Repair Diff：慢请求 trace 签名、中间件修复计划、失败点和变体验证要求。
- Gate Result：风险、环境兼容性、校验、dry-run、缺失回滚、缺失验证项。
- Runner Binding：展示将复用哪个现有 Runner workflow、runbook 或 tool，不展示新的 Runner 页面设计。

### AI Chat 集成

Chat 回复中展示已激活经验来源：

- “命中 Skill：PG lock wait 慢请求诊断，适用 checkout + postgres，成功率 91%。”
- “命中 Debug RCA：checkout 提交按钮慢请求，trace 签名为 api->pg lock_wait，建议先检查锁等待。”
- “命中 RepairPlan：PG 复制延迟恢复 v2，适用 primary/standby + streaming replication。”
- “未使用候选经验：存在 2 条待审核候选，需 SRE 审核后才能推荐。”

Chat 不展示 candidate 的具体推荐内容，避免用户绕过审核。

### 用户侧调试集成

浏览器插件 Debug Mode 结束后，事故工作台和经验包库应提供“从本次 Debug Trace 生成经验候选”入口。候选详情展示页面动作、trace id、慢 span、Coroot RCA、服务路径、建议修复动作、插件注入状态和脱敏状态。

### 中间件修复集成

中间件修复报告页应提供“生成/更新经验包”入口。候选详情展示 MiddlewareResource、RepairPlan、执行步骤、失败点、回滚结果、验证指标和后续是否可自动化。

### Runner workflow metadata 集成

经验服务写入现有 Runner workflow draft metadata：

- `source=experience-pack`
- `experience_pack_id`
- `variant_key`
- `review_record_id`

简单版不要求改 Runner Studio 页面。经验服务只需要把 `source=experience-pack`、`experience_pack_id`、`variant_key`、`review_record_id` 写入 workflow draft metadata；如果未来改 Runner Studio，再展示这些来源提示。

## 13. 存储方案

MVP 阶段可使用文件存储，保持和现有 JSON/YAML 轻量风格一致：

```text
.data/experience/
  cases/*.json
  packs/*.yaml
  candidates/*.yaml
  reviews/*.json
  environment-profiles/*.json
  debug-traces/*.json
  repair-cases/*.json
.data/evolution-map/
  events/*.jsonl
  skills/*.yaml
  genes/*.yaml
  capsules/*.yaml
  debug-rca/*.yaml
  repair-plans/*.yaml
  activation-index/*.json
  lineage/*.json
```

生产阶段建议迁移到事件表和资产表：

- `experience_events`
- `experience_cases`
- `experience_packs`
- `experience_artifacts`
- `experience_reviews`
- `experience_skills`
- `debug_trace_events`
- `repair_cases`
- `middleware_resources`
- `environment_profiles`
- `evolution_events`
- `evolution_nodes`
- `evolution_edges`
- `activation_index`
- `asset_lineage`

无论文件还是数据库，都必须保留 lineage、semantic hash、review record 和 source refs。

## 14. 与现有模块的集成点

- Coroot：复用 `internal/integrations/coroot` 的 normalized tools 和 webhook evidence。
- Browser Plugin Debug / Trace：接入 DebugEvent、TraceContext、W3C traceparent、插件注入状态、前端耗时和 trace backend span 摘要。
- Middleware Ops：接入 PG/Redis/MQ 等只读诊断、RepairPlan、RecoveryAttempt 和集群画像。
- Incident：复用事故、证据、postmortem 作为经验来源。
- Runbook：扩展 catalog 加载源，但正式列表只读 approved/published。
- Runner Workflow：复用现有 graph draft、validate、dry-run、run、publish、history 和 hash，不改 Runner 页面和功能。
- Approval/Audit：候选生成、审核、发布都记录 audit；高风险动作继续走现有审批。
- PromptCompiler：只读取 Activation Index，不读取 Candidate Store。
- AI Chat / RuntimeKernel：不改现有 Chat 页面；后端每次相关 Chat 写入 OperationEvent，记录 activated asset 和 outcome。
- Model trace/eval：用现有 trace/eval 思路为“候选生成质量”加回归用例。

## 15. 安全与治理

- 默认不保存完整 stdout/stderr，只保存摘要、引用、hash 和截断片段。
- secret redactor 必须覆盖 token、password、Authorization、kubeconfig、private key、连接串。
- Debug Trace 禁止保存请求体、cookie、token、密码和用户敏感输入，只保存 trace id、span、指标、页面动作和脱敏引用。
- 中间件修复候选必须标记数据破坏风险、failover 风险、备份状态和回滚可行性；高风险候选默认不能自动执行。
- 自动生成内容默认 `candidate`，没有任何执行权限，也没有任何 AI Chat 推荐权。
- 未审核候选不能进入 PromptCompiler、Activation Index、Runbook matcher 或 Workflow 推荐器。
- 高风险候选必须包含 rollback 和 verification，否则不能提交发布审核。
- 环境不匹配时，禁止执行，只允许创建变体候选。
- 审核和发布必须可追溯到 reviewer、时间、证据、graph hash。

## 16. 分阶段落地

### Phase 1：Simple EvoMap 与环境画像

- 新增 append-only EvolutionEvent。
- 建立 EnvironmentProfile。
- AI Chat 每轮记录 activated asset、工具调用、审批和 outcome。
- Candidate 不进入 AI Chat 推荐。

### Phase 2：经验包候选与审核队列

- 新增 `internal/experience` 模型、文件存储和 API。
- 从 Incident、Coroot webhook、Debug Trace、Middleware Repair、Workflow run history 生成只读候选。
- 前端经验包页面改为 API 驱动。
- 候选不能发布，只能展示证据和摘要。

### Phase 3：SkillCard/Debug RCA/RepairPlan 人工激活

- 从真实 case 蒸馏 SkillCard candidate。
- 生成 Debug RCA candidate 和 RepairPlan draft。
- 增加审核动作 `approve_skill`、`approve_debug_rca`、`approve_repair_plan`。
- 审核通过后写入 Activation Index。
- AI Chat 只检索 Activation Index。

### Phase 4：Runbook Draft 发布

- 从候选生成 Runbook draft。
- 增加人工审核、reject/request changes/approve。
- Runbook catalog 只加载 approved/published。
- 增加匹配时的环境画像过滤。

### Phase 5：Workflow Draft 发布

- 从候选生成 Runner graph draft。
- 接入 validate、dry-run、publish。
- workflow draft metadata 写入来源、证据链、环境范围和 diff；不要求改 Runner Studio 页面。
- 发布后写入 lineage。

### Phase 6：环境变体与简单自我演化

- 引入 environment probe 和 EnvironmentProfile。
- 实现 Variant Resolver。
- 不匹配时生成变体候选。
- 根据运行成功率和失败聚类更新 confidence，不自动覆盖发布资产。

## 17. 验收标准

- Coroot 事故关闭后，系统能生成一个经验包候选，包含 evidence、环境画像、建议 Runbook 步骤。
- 浏览器插件 Debug Mode 产生慢请求 trace 后，系统能生成 Debug RCA candidate，包含页面动作、trace id、插件注入状态、慢 span、Coroot RCA、服务路径、建议修复和脱敏状态。
- 中间件修复 case 完成后，系统能生成 RepairPlan 或 Capsule 候选，包含 RCA、诊断步骤、修复步骤、风险、回滚、验证、成功或失败原因。
- 候选不出现在 AI Chat 推荐、Runbook 正式列表或 Workflow 正式列表。
- 未审核 SkillCard/Debug RCA/RepairPlan 不会进入 PromptCompiler 输入、Activation Index 或推荐排序。
- 审核通过的 SkillCard/Debug RCA/RepairPlan 会在下一次相似 AI Chat、DebugCase 或中间件修复 case 中被展示为推荐来源。
- 人工审核通过后，Runbook 才能被 `/api/v1/runbooks` 返回。
- 从经验包生成的 workflow draft 必须能通过现有 Runner workflow validate 和 dry-run。
- 本方案不要求修改现有 AI Chat 页面和 Runner 工作流页面/功能。
- 已发布 workflow 遇到新 OS 时不会被修改，只生成新的 variant candidate。
- 匹配 Runbook / Workflow 时优先选择环境兼容版本；不兼容时只提示创建变体。
- 所有审核、发布、拒绝、变体生成都可追溯到证据和 audit record。
