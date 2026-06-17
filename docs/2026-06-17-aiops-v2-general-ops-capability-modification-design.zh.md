# aiops-v2 通用运维能力优先改造设计方案

日期：2026-06-17  
状态：修改设计方案  
关联文档：`docs/2026-06-17-aiops-v2-pg-coroot-workflow-acceptance.zh.md`  
适用范围：AI Chat、RuntimeKernel、OpsManual、Workflow 生成、Runner、host-agent、Coroot/观测源能力、eval 回归

## 1. 结论

aiops-v2 后续改造的第一优先级是提升通用运维能力，而不是为 PG、pg_mon、Coroot 或任何单个验收样例写专项逻辑。

本文中的 PG 主从恢复、PG workflow 生成、Coroot 跨服务 RCA 只作为三类通用能力的验收样本：

| 验收样本 | 真实要提升的通用能力 |
|---|---|
| PG 主从恢复 | 有状态中间件集群的诊断、方案选择、受治理修复和恢复验证 |
| PG workflow 生成 | 面向运维资源的受控 Workflow 生成、验证和审核 |
| Coroot A->B->C RCA | 观测源驱动的依赖链 RCA、证据解释和置信度控制 |

`pg_mon` 不是需要 aiops-v2 单独内置集成的对象。它是用户环境中 PG 集群的监控/辅助组件，aiops-v2 应在执行过程中理解它的角色、关系和验证价值，再通过通用 probe、已有 workflow 能力或用户环境提供的访问方式读取状态。只有确实需要专属 API、领域命令或展示组件时，才把对应能力做成插件/能力包适配。

## 2. 现状问题

### 2.1 样例容易诱导专项硬编码

当前验收样本里出现了 PG、pg_mon、Coroot、A/B/C 服务链。如果实现时直接在 core router、prompt、workflowgen、opsmanual 或 runtime 中加入这些名字，会导致：

- 只能通过样例，无法迁移到 Redis、MySQL、Kafka、Nginx、Kubernetes、其他监控源或其他主机拓扑。
- core 决策路径被 provider-specific 规则污染。
- 后续每个中间件、监控组件和工具都要做内置分支，系统会变成一组专项脚本集合。

### 2.2 Operation Frame 还不够表达真实运维上下文

现有 Operation Frame 能表达目标对象、动作、环境和风险，但对复杂运维场景仍不够：

- 难以表达主节点、从节点、监控节点、代理、sidecar、backup agent 等不同资源角色。
- 难以表达“用户提到的组件是目标系统的一部分，但不是目标数据节点”。
- 难以表达执行面和观察点的区别，例如 host-agent 负责执行，pg_mon 负责观察。
- 难以表达用户允许数据丢失只是风险偏好，不等于审批通过。

### 2.3 Workflow 生成偏任务模板，不够运维资源化

当前 `workflowgen` 更偏通用文本任务、新闻、安全摘要、故障复盘等生成路径。它缺少面向运维资源的通用能力：

- 资源角色和关系建模。
- preflight / execute / verify / rollback 阶段约束。
- 参数 schema、secret ref、风险等级和审批策略自动对齐。
- 真实或模拟 lab validation 的 proof bundle。

### 2.4 观测源 RCA 能力还没有抽象成通用证据流

Coroot adapter 已经有较好的 edge evidence、hypotheses 和 propagation path，但 Chat 层验收不应只等价于“会调用 Coroot”。真正要沉淀的是：

- 环境到观测项目的解析。
- 服务到观测实体的解析。
- 依赖链证据提取。
- 症状服务、传播服务、根因服务的区分。
- 证据不足时的低置信输出。

### 2.5 测试需要验证“通用能力路径”，不只验证样例答案

现有 eval case 可以检查回答和工具调用，但还需要把“不能靠样例专项硬编码通过”作为验收要求。否则模型或实现可以只针对 PG/Coroot 文案过关。

## 3. 修改目标

### 3.1 P0 必须达成

1. 新增或强化通用运维能力 contract，而不是新增 PG/Coroot 专项主路径。
2. Operation Frame 能表达资源角色、关系、执行面、观察点、风险偏好和证据需求。
3. 有状态中间件集群恢复走通用 repair flow：
   - 识别资源和角色。
   - 收集只读证据。
   - 判断是否单方案或多方案。
   - 生成受治理 RepairPlan。
   - 通过 Runner/host-agent 执行。
   - 独立验证恢复结果。
4. Workflow 生成走通用资源型 workflow generation：
   - 先计划和补槽。
   - 再生成 graph。
   - 再静态/实验环境验证。
   - 最后进入 draft / pending_review。
5. 观测源 RCA 走通用 observability evidence flow，Coroot 只是首个 provider。
6. eval case 必须要求“通用能力路径”，并禁止样例专项硬编码说法。

### 3.2 P0 不做

1. 不为 pg_mon 做默认内置集成。
2. 不为每个 monitor/exporter/proxy/sidecar 都新增 adapter。
3. 不在 core 中写 `if postgres`、`if pg_mon`、`if coroot`、固定主机名或固定服务链。
4. 不让 LLM 直接发布 verified workflow 或绕过审批执行高风险动作。
5. 不把“数据可以不要”解释为可以跳过审批、备份检查或 Run Record。

## 4. 修改方案

### 4.1 新增通用运维能力优先 Gate

在设计、实现和代码 review 中加入强制判断：

```text
这个改动是否提升通用能力？
是否能迁移到同类中间件、同类观测源或同类执行面？
是否把 provider-specific 名称写进了 core 决策路径？
是否能通过 plugin / skill / MCP adapter / capability pack / Runner action 承载专项知识？
```

如果答案显示改动只服务一个样例，不能进入 core。最多作为能力包内部实现。

### 4.2 Operation Frame v2

扩展 Operation Frame 的表达能力。建议新增或等价支持以下字段：

```yaml
target:
  kind: middleware_cluster | service | host_group | workflow | unknown
  subtype: postgres | redis | mysql | kafka | unknown
  name: string

roles:
  - id: role-primary
    kind: data_node | monitor | proxy | backup_agent | observer | executor
    resource_ref: host-a
    user_label: 主机A
    inferred_from: user_input | opsgraph | inventory | evidence

relationships:
  - from: host-c
    to: pg-cluster
    type: monitors
  - from: host-a
    to: host-b
    type: replicates_to

execution_surface:
  kind: host_agent | runner | mcp | api | sql | unknown
  resources: [host-a, host-b]

observation_points:
  - kind: monitor_component
    resource_ref: host-c
    role: pg_mon
    access: host_agent | http | unknown

risk_preference:
  data_loss_acceptable: true
  still_requires_approval: true

evidence_requirements:
  - cluster_role
  - member_health
  - replication_status
  - storage_health
  - observer_health
```

关键点：

- `pg_mon` 应落到 `observation_points` 或 `roles.kind=monitor`，不是数据节点。
- `data_loss_acceptable` 只是用户目标和风险偏好，不能降低审批要求。
- 如果 monitor 访问方式未知，系统应先追问或用通用 probe 探测，不直接假设有专属 API。

### 4.3 通用资源理解与补槽

Chat route 不应通过固定关键词直接进入 PG 专用流程，而应先做资源理解：

```text
用户输入
  -> 识别目标对象、角色、关系、动作和风险偏好
  -> 缺少关键输入时补槽
  -> 形成 Operation Frame v2
  -> 根据 frame 选择 capability contract
```

补槽规则：

| 缺口 | 行为 |
|---|---|
| 不知道哪个是 primary | 询问用户，或根据只读证据推断并标置信心 |
| 不知道 monitor 访问方式 | 询问或用通用 host/HTTP probe 探测 |
| 不知道是否可覆盖数据目录 | 默认不可覆盖，必须明确审批 |
| 不知道执行面 | 根据 inventory、host-agent、Runner、MCP 可用性选择 |
| 证据不足 | 不给高置信方案，不执行高风险动作 |

### 4.4 通用 Stateful Middleware Repair Flow

新增或整理一个通用 repair flow，而不是 PG 专用流程：

```text
Operation Frame v2
  -> capability match: stateful_middleware_cluster_repair
  -> readonly evidence collection
  -> diagnosis summary
  -> repair option generation
  -> option selection
  -> governed execution
  -> independent verification
  -> Run Record / learning candidate
```

通用证据类型：

- 资源角色和拓扑。
- 成员健康。
- 存储健康。
- 网络/端口连通。
- 数据同步或一致性状态。
- 观察点健康。
- 最近变更。
- 回滚和恢复约束。

PG 能力包只负责把通用证据类型映射为 PG 的具体只读 probe，例如 `pg_isready`、`pg_is_in_recovery()`、`pg_stat_replication`。pg_mon 只作为观察点参与验证，不要求默认内置 API。

### 4.5 通用 Workflow Generation Flow

把 workflow 生成从“文本任务模板”升级为“资源型运维 workflow 生成”：

```text
用户请求写 workflow
  -> Operation Frame v2
  -> Requirement Slot Resolver
  -> Workflow Plan
  -> Graph Generator
  -> Static Validator
  -> Lab Validator
  -> Draft Workflow
  -> Review / Publish Gate
```

通用阶段：

| 阶段 | 规则 |
|---|---|
| preflight | 只读，确认资源、权限、端口、磁盘、版本、已有状态 |
| execute | 参数化，敏感值只能用 secret ref，高风险节点要求审批 |
| verify | 独立读取证据，不能只复用 execute 输出 |
| rollback | 能回滚就写步骤，不能回滚就写人工接管和阻断条件 |

PG 主从 + pg_mon 样例应通过这个通用 flow 生成，而不是硬编码固定节点模板。

### 4.6 通用 Observability RCA Flow

把 Coroot RCA 抽象成观测源证据流：

```text
RCA request
  -> resolve environment/project
  -> resolve service/entity
  -> collect observability evidence pack
  -> build dependency graph
  -> rank hypotheses
  -> synthesize answer with evidence and limits
```

通用证据 contract：

```yaml
observability_evidence:
  target_status: []
  dependency_edges: []
  incidents: []
  metrics: []
  logs: []
  traces: []
  deployments: []
  hypotheses: []
  missing_evidence: []
```

Coroot adapter 可以贡献 edge evidence、hypotheses、propagation path，但 Chat 层最终回答不应依赖 Coroot 专有字段名才能成立。

### 4.7 Capability Pack 边界

专项知识只进入能力包：

| 类型 | 可以包含 | 不能包含 |
|---|---|---|
| Capability pack | probe 映射、参数 schema、验证断言、禁用条件、文档 | core router 分支 |
| Skill | 领域解释、证据边界、失败处理 | 绕过 ToolDispatcher 的执行规则 |
| MCP adapter | 外部 API 读取和摘要 | 生产主路径硬编码 |
| Runner action | 可执行步骤和输出 envelope | 绕过审批/ActionToken |
| Eval fixture | 样例输入、预期行为、禁用说法 | 作为生产逻辑来源 |

## 5. 需要修改的模块边界

### 5.1 `internal/opsmanual`

建议修改：

- Operation Frame 支持角色、关系、观察点、风险偏好。
- 检索和参数解析优先基于通用 contract。
- provider-specific hint 只来自 capability pack。

验收信号：

- PG 样例能识别 A/B 为数据节点、C 为 monitor。
- MySQL/Redis/Kafka 类似请求不被 PG 规则污染。

### 5.2 `internal/workflowgen`

建议修改：

- 增加资源型 plan builder。
- 增加 slot resolver。
- 增加 preflight/execute/verify/rollback graph constraints。
- 增加 proof bundle 输出。

验收信号：

- 不显式 `@add_workflow` 时也能识别“写 workflow”意图。
- PG 样例产物是参数化 draft，不是 verified。
- 生成逻辑不依赖固定 pg_mon 模板。

### 5.3 `internal/runtimekernel` / Chat route

建议修改：

- Chat route 以 Operation Frame 和 capability contract 路由。
- 不新增 provider-specific core branch。
- 多方案选择、审批、resume 和 final answer 都走现有主链。

验收信号：

- 高风险/破坏性动作审批前不执行。
- final answer 不伪造执行结果。

### 5.4 `internal/integrations/coroot`

建议修改：

- 保持 Coroot adapter 在 provider 边界内。
- 输出继续对齐通用 observability evidence contract。
- 外部依赖、证据不足、置信度限制必须结构化表达。

验收信号：

- A->B->C 样例能定位 C，但回答可解释为通用依赖链 RCA，不是 Coroot 文案复述。

### 5.5 Eval

建议修改：

- 保留三个样例 case。
- 增加通用性断言：禁止“只支持 PG”“只支持 Coroot”“为这个例子写死”。
- 后续补充同类反例：Redis/Mysql/Kafka/其他监控组件/其他观测源。

验收信号：

- mock eval 能加载。
- server eval 需要同时检查 tool calls、turn items、answer 和 evidence。

## 6. 分阶段落地

### Phase 0：规则和测试固化

已完成或正在进行：

- README 写入通用运维能力优先规则。
- acceptance 文档明确样例不是产品边界。
- 三个 eval case 加入禁止专项硬编码的断言。

### Phase 1：Operation Frame v2

目标：

- 表达 resource roles、relationships、observation points、risk preference。
- 让 pg_mon 这类组件作为 monitor role 进入上下文。

建议测试：

- 用户给 A/B/C 三台主机时，A/B 是 data_node，C 是 monitor。
- 用户说“数据可以不要”时，风险偏好记录为 true，但审批仍 required。

### Phase 2：通用 repair flow

目标：

- stateful middleware cluster repair contract。
- PG cluster pack 作为首个实现。
- 只读诊断、多方案、受治理执行、独立验证闭环。

建议测试：

- PG 主从恢复 case。
- Redis 主从/哨兵恢复变体。
- MySQL 主从恢复变体。

### Phase 3：资源型 workflow generation

目标：

- 资源型 plan builder / slot resolver / graph generator。
- preflight/execute/verify/rollback 强约束。
- draft + proof bundle + review gate。

建议测试：

- PG 主从 + monitor workflow。
- Redis backup workflow。
- Nginx upstream reload workflow。

### Phase 4：通用 observability RCA

目标：

- Observability evidence contract。
- Coroot adapter 对齐 contract。
- Chat final answer 稳定表达症状、传播、根因、方案、缺失证据。

建议测试：

- Coroot A->B->C。
- 其他观测源 fixture。
- 证据不足时低置信回答。

## 7. 验收标准

必须同时满足：

1. 三个样例 case 能通过真实 server eval。
2. 实现没有在 core 中新增样例专项分支。
3. pg_mon 被识别为监控/辅助组件角色，不被当成 PG 数据节点，也不强制要求内置集成。
4. 高风险或 destructive 动作审批前不执行。
5. Workflow 生成结果保持 draft / pending_review，不能自动 verified。
6. RCA 结论必须引用观测证据；证据不足时不能高置信。
7. 至少一个同类非 PG 或非 Coroot 变体能复用同一条通用链路。

## 8. 推荐验证命令

基础验证：

```bash
go test ./internal/eval ./cmd/agent-eval ./cmd/agent-eval-case
go run ./cmd/agent-eval -agent mock -cases testdata/eval_cases -priority P1 -out .data/eval_runs/general-ops-capability-mock
```

实现后真实回归：

```bash
go run ./cmd/agent-eval -agent server -server-url http://127.0.0.1:8080 -cases testdata/eval_cases -priority P1 -repetitions 3 -out .data/eval_runs/general-ops-capability-server
```

通用性扫描：

```bash
rg -n "if .*postgres|if .*pg_mon|if .*coroot|strings\\.Contains\\(.*postgres|strings\\.Contains\\(.*pg_mon|strings\\.Contains\\(.*coroot" internal web/src
```

发现命中时必须判断：

- 是否在 plugin / adapter / fixture / test 边界内。
- 是否泄漏到 core routing / runtime / policy / workflowgen 主路径。

## 9. 风险与取舍

| 风险 | 处理 |
|---|---|
| 通用抽象过度，第一版落地变慢 | 只抽象最小 contract，PG/Coroot 作为首个样本验证 |
| 能力包边界不清 | 所有 provider-specific 逻辑必须说明贡献的通用 contract |
| 模型把 monitor 当成 data node | Operation Frame v2 加 role/relationship，并在 eval 中加入禁用断言 |
| 用户希望“数据不要”导致误删 | 风险偏好和审批分离，高风险动作仍 blocked |
| RCA 过度依赖单一观测源 | 观测源 evidence contract 支持多 provider，Coroot 只是首个实现 |

## 10. 最终判断标准

这次改造成功的标志不是“PG 例子能跑通”或“Coroot 例子能回答”，而是：

```text
同一套通用运维主链
  能理解资源和角色
  能收集证据
  能选择方案
  能生成和验证 workflow
  能受治理执行
  能独立验证恢复
  能沉淀 run record
  能迁移到同类问题
```

PG、pg_mon、Coroot 只是第一批用来证明这条主链有效的样本。
