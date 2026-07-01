# aiops-v2 与 Codex Agent Runtime 机制和 Prompt 对比报告

日期：2026-06-24  
范围：`aiops-v2/` 与 `codex/` 本地代码库的 agent runtime、工具面、审批、安全策略、上下文压缩和关键 runtime prompt。

## 结论摘要

`aiops-v2` 已经有很多 Codex 化组件：统一 `EinoKernel` turn loop、四层 prompt compiler、Tool Index、tool surface snapshot、evidence gate、approval/resume、host-bound 子 Agent、context governance 和 model input trace。问题不是“缺少机制”，而是机制分叉和 prompt 过载：chat route、hostops route、runtime loop、host command approval、prompt policy、tool permission checker、policy engine 各自都能改变行为，导致用户看到的运维操作体验不够连续、可预测。

Codex 的好体验主要不是来自某一段神奇 prompt，而是来自一个更简单的 runtime 合约：

- 一个 thread/turn loop 贯穿用户输入、工具调用、审批、恢复、压缩和最终回答。
- Prompt 基座短而稳定，把权限、环境、技能、插件、AGENTS.md、工具发现作为动态上下文片段注入。
- 工具暴露是“当前可见 schema + deferred tool_search”的明确模型；不可见工具不会被模型误以为可调用。
- `exec_command`、sandbox、approval、process manager 在一条链路里完成，用户审批和模型恢复路径清楚。
- UI 看到的是同一个 turn 的事件流，而不是某些路径直接写一个 synthetic completed turn。

因此，建议 `aiops-v2` 不要直接复制 Codex 的整段 prompt，而是复刻 Codex 的 runtime contract：薄 base prompt、统一 turn loop、统一 approval、清晰 tool surface、模型驱动的 hostops manager、证据引用和可回放 trace。

## Codex Agent Runtime 机制

### 1. Thread / Session / Turn

Codex 的核心对象是 `Session` 和 thread manager。`Session` 保存 thread id、事件通道、会话状态、active turn、mailbox、goal runtime、guardian review、services 等运行时状态，见 `codex/codex-rs/core/src/session/session.rs:14`。`SessionConfiguration` 把 `base_instructions`、`developer_instructions`、`user_instructions`、`compact_prompt`、`approval_policy`、`permission_profile`、`cwd` 等长期配置统一在一起，见 `codex/codex-rs/core/src/session/session.rs:40`。

Thread 创建和消息投递由 `ThreadManager` 负责。新线程可带动态工具启动，见 `codex/codex-rs/core/src/thread_manager.rs:557`；发送用户或系统操作最终走 `thread.submit(op)`，见 `codex/codex-rs/core/src/thread_manager.rs:935`。

每个 turn 由 `run_turn` 驱动，入口见 `codex/codex-rs/core/src/session/turn.rs:139`。主要流程：

1. 预采样 compaction，必要时 reset model client session。
2. 记录 turn context，并解析技能、插件、connector、MCP 工具。
3. 进入循环：处理 pending input，克隆历史作为 model input，调用 `run_sampling_request`。
4. 如果模型输出工具调用，dispatch 工具并把 tool result 写回历史。
5. 如果还需要 follow-up 或达到上下文阈值，继续同一 turn 或自动 compact。
6. 无需 follow-up 时跑 stop hook，记录最终 assistant message。

关键点：Codex 的用户输入、工具调用、审批、失败重试、自动压缩都在同一个 turn 生命周期里推进，而不是从 UI 层提前分叉。

### 2. Prompt 构造

Codex 发送给模型的是 `Prompt { input, tools, parallel_tool_calls, base_instructions, personality, output_schema }`，见 `codex/codex-rs/core/src/session/turn.rs:979`。`run_sampling_request` 在每次采样前调用 `built_tools` 构造工具 router，再用当前历史和 `base_instructions` 生成 prompt，见 `codex/codex-rs/core/src/session/turn.rs:1004`。

Codex 的 base prompt 相对薄：

- 角色和能力：`codex/codex-rs/core/gpt_5_2_prompt.md:1`
- AGENTS.md 作用域规则：`codex/codex-rs/core/gpt_5_2_prompt.md:17`
- 自主完成任务的行为约束：`codex/codex-rs/core/gpt_5_2_prompt.md:29`
- plan tool 约束：`codex/codex-rs/core/gpt_5_2_prompt.md:36`
- 不猜测、持续完成、使用 `apply_patch`：`codex/codex-rs/core/gpt_5_2_prompt.md:109`
- 验证策略：`codex/codex-rs/core/gpt_5_2_prompt.md:136`
- 最终回答格式：`codex/codex-rs/core/gpt_5_2_prompt.md:160`
- shell / apply_patch / update_plan 工具规则：`codex/codex-rs/core/gpt_5_2_prompt.md:244`

此外，Codex 把权限、环境、技能、插件、AGENTS.md 等拆成 contextual fragments，而不是塞进 base prompt：

- `TurnContext` 明确包含 cwd、日期、时区、developer/user instructions、collaboration mode、approval policy、permission profile、tools config、dynamic tools、turn skills 等，见 `codex/codex-rs/core/src/session/turn_context.rs:55`。
- `to_turn_context_item` 把当前 turn 的 cwd、日期、审批策略、sandbox policy、permission profile、model、collaboration mode 等写成可注入上下文，见 `codex/codex-rs/core/src/session/turn_context.rs:347`。
- 权限提示由 `PermissionsInstructions` 按 permission profile 动态生成，见 `codex/codex-rs/core/src/context/permissions_instructions.rs:61`。
- AGENTS.md 用 user fragment 注入，见 `codex/codex-rs/core/src/context/user_instructions.rs:9`。
- 可用技能用 developer fragment 注入，见 `codex/codex-rs/core/src/context/available_skills_instructions.rs:23`。
- Apps/Connectors 的 lazy-load 和 `tool_search` 规则由 app fragment 注入，见 `codex/codex-rs/core/src/context/apps_instructions.rs:20`。

### 3. Tool Surface / Tool Search

Codex 的工具面由 `ToolRouter` 统一构造和 dispatch。`ToolRouter::from_config` 先构造完整 specs/registry，再过滤出 model-visible specs，deferred dynamic tool 不直接暴露给模型，见 `codex/codex-rs/core/src/tools/router.rs:54`。

工具注册由 `build_tool_registry_builder` 控制，按配置注册 shell/unified exec、MCP resources、plan、request_user_input、tool_search、request_plugin_install、apply_patch、web_search、image generation、collab multi-agent、MCP namespace tools 等，见 `codex/codex-rs/core/src/tools/spec_plan.rs:69`、`codex/codex-rs/core/src/tools/spec_plan.rs:191`、`codex/codex-rs/core/src/tools/spec_plan.rs:223`、`codex/codex-rs/core/src/tools/spec_plan.rs:260`、`codex/codex-rs/core/src/tools/spec_plan.rs:300`。

`tool_search` 是真正的 lazy-load 搜索器。它把 deferred MCP tools 和 deferred dynamic tools 转成 BM25 搜索条目，返回 `LoadableToolSpec`，见 `codex/codex-rs/core/src/tools/handlers/tool_search.rs:25`、`codex/codex-rs/core/src/tools/tool_search_entry.rs:16`。这意味着模型先看到“可搜索的工具目录”，搜索后才拿到可加载 schema，降低初始工具面噪声。

工具调用 dispatch 也集中：`ToolRegistry::dispatch_any` 做 handler 查找、pre/post tool hooks、mutating gate、telemetry、goal accounting、hook replacement 等，见 `codex/codex-rs/core/src/tools/registry.rs:270`。

### 4. Exec / Sandbox / Approval

Codex 的 `Unified Exec` 设计目标写得很清楚：交互进程、审批、sandbox 选择、失败重试在一个 flow 中完成，见 `codex/codex-rs/core/src/unified_exec/mod.rs:1`。流程是“approval/cache/prompt -> select sandbox -> run -> sandbox denial 时按策略重试”，见 `codex/codex-rs/core/src/unified_exec/mod.rs:12`。

`exec_command` handler 会：

- 把参数解析成 command、workdir、shell、sandbox_permissions、additional_permissions、justification、prefix_rule，见 `codex/codex-rs/core/src/tools/handlers/unified_exec.rs:28`。
- 判断命令是否 mutating，安全命令可并发，见 `codex/codex-rs/core/src/tools/handlers/unified_exec/exec_command.rs:99`。
- 应用已批准的 turn/session permissions，校验 additional permissions，见 `codex/codex-rs/core/src/tools/handlers/unified_exec/exec_command.rs:215`。
- 将 `apply_patch` 命令拦截进安全 patch 处理，见 `codex/codex-rs/core/src/tools/handlers/unified_exec/exec_command.rs:272`。
- 最终交给 `UnifiedExecProcessManager`，见 `codex/codex-rs/core/src/tools/handlers/unified_exec/exec_command.rs:298`。

审批策略由 `ExecPolicyManager` 做 command 分段、规则匹配、heuristics、prefix_rule 建议和 approval requirement 生成，见 `codex/codex-rs/core/src/exec_policy.rs:272`。对危险命令或缺少 sandbox 保护的情况，按 approval policy 决定 prompt / forbid / allow，见 `codex/codex-rs/core/src/exec_policy.rs:656`。

权限 prompt 也非常具体：`on_request.md` 告诉模型如何用 `sandbox_permissions=require_escalated`、`justification`、`prefix_rule` 发起审批，见 `codex/codex-rs/core/src/context/prompts/permissions/approval_policy/on_request.md:24`；`never.md` 明确禁止传 `sandbox_permissions`，见 `codex/codex-rs/core/src/context/prompts/permissions/approval_policy/never.md:1`。sandbox mode 只用三类短提示：danger-full-access、workspace-write、read-only，见 `codex/codex-rs/core/src/context/prompts/permissions/sandbox_mode/danger_full_access.md:1`、`codex/codex-rs/core/src/context/prompts/permissions/sandbox_mode/workspace_write.md:1`、`codex/codex-rs/core/src/context/prompts/permissions/sandbox_mode/read_only.md:1`。

### 5. Multi-agent

Codex 的 multi-agent 是普通工具面的一部分，在 `collab_tools` 打开时注册 `spawn_agent`、`send_message`、`followup_task`、`wait_agent`、`close_agent`、`list_agents` 等工具，见 `codex/codex-rs/core/src/tools/spec_plan.rs:300`。`spawn_agent` handler 会发送 `CollabAgentSpawnBeginEvent`，构造 child config，继承环境选择，再通过 agent_control 创建子线程，见 `codex/codex-rs/core/src/tools/handlers/multi_agents_v2/spawn.rs:46`。

关键点：Codex 的子 Agent 是模型在同一个 turn loop 中通过工具显式启动的，不是 UI route 在 runtime 前写一个“已完成任务”快照。

### 6. Context Compaction

Codex 的 compaction prompt 极短：创建 handoff summary，包含进度、关键决策、约束、剩余步骤、关键数据，见 `codex/codex-rs/core/templates/compact/prompt.md:1`。`turn_context.compact_prompt()` 默认使用这个 prompt，见 `codex/codex-rs/core/src/session/turn_context.rs:341`。自动 compact 可在 pre-sampling 和 mid-turn 触发，见 `codex/codex-rs/core/src/session/turn.rs:158`、`codex/codex-rs/core/src/session/turn.rs:493`。

## aiops-v2 Agent Runtime 机制

### 1. Chat Front Door 与 Route

`defaultChatService.SendMessage` 是 chat 前门，先构造 `runtimekernel.TurnRequest`，再在 `ChatRuntimeV2Enabled` 时抽取 user evidence、主机 mentions、route，并写入 route/tool/evidence metadata，见 `aiops-v2/internal/appui/chat_service.go:92`、`aiops-v2/internal/appui/chat_service.go:129`。

Route 有四类：`chat_advisory`、`evidence_rca`、`host_bound_ops`、`multi_host_ops`，见 `aiops-v2/internal/appui/chat_runtime_route.go:12`。单主机 mention 会切到 host session 并允许 `exec_command`，多主机 mention 会切到 workspace plan mode 且不允许主 Agent 直接 exec，见 `aiops-v2/internal/appui/chat_runtime_route.go:55`、`aiops-v2/internal/appui/chat_runtime_route.go:129`。

工具面 metadata 由 `applyChatRuntimeToolSurfaceMetadata` 写入：`toolProfile`、`aiops.tool.execCommandAllowed`、host mutation gate、Coroot RCA gate、public_web pack、host_ops pack 等，见 `aiops-v2/internal/appui/chat_tool_surface.go:9`。

一个明显分叉点是 `handleHostOpsRoute`：检测到 hostops route 后，创建 hostops mission，然后 `writeHostOpsMissionTurn` 直接写一个 completed `TurnSnapshot` 和 final output，见 `aiops-v2/internal/appui/chat_service.go:177`、`aiops-v2/internal/appui/chat_service.go:224`。这条路径绕过了 `EinoKernel` 的模型 loop、工具调用、approval/resume 和 evidence gate。

### 2. EinoKernel Turn Loop

`EinoKernel` 是 host/workspace 共享的唯一 turn runtime kernel，字段包括 tools、compiler、policy、permissions、hooks、projector、modelRouter、sessions、agentMgr、observer、compressor、evidenceService 等，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:122`。

`RunTurn` 入口先 validate request，创建/恢复 session，追加 user message，获取 planner/worker model，然后调用 `runHostIterationLoop`，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:398`。代码注释明确 host 和 workspace 已经收敛到共享 loop，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:565`。

共享 loop 每轮做：

- 应用 context pipeline、pending approvals/evidence、budget policy，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1597`。
- observation dedupe，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1613`。
- enrich compile context、应用 tool surface policy、追加 skill/evidence/protocol state，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1617`。
- 编译 prompt，做 prompt section retention 和 prompt section cache，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1655`。
- 计算 stable prompt hash、prompt fingerprint、tool fingerprint、visible tools、tool surface snapshot，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1679`。
- 组装 Eino tool pool，构造 model input，写 model input debug trace，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1693`、`aiops-v2/internal/runtimekernel/eino_kernel.go:1717`。
- 持久化 assistant message、iteration、visible tools、prompt delta、checkpoint，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1901`。
- 无工具调用时执行 premature final、plan completion、verification、manager synthesis、completion readiness、mandatory skill、final completeness、final evidence、missing evidence 等 final gates，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:1955`。
- 有工具调用时走 dispatcher，支持并发安全工具批量并发，默认每 turn 最多 12 个计入预算的工具调用，见 `aiops-v2/internal/runtimekernel/eino_kernel.go:2139`、`aiops-v2/internal/runtimekernel/eino_kernel.go:2257`、`aiops-v2/internal/runtimekernel/eino_kernel.go:2356`。

`SessionState` 记录 messages、pending approvals/evidence、approval grants、plan mode、checkpoints、compacted segments、external refs、observation state、environment context、tool discovery、skill activation、MCP instructions，见 `aiops-v2/internal/runtimekernel/types.go:21`。

### 3. Prompt Compiler

`PromptCompilerImpl` 是四层 prompt truth source：system、developer、tool prompt set、runtime policy，见 `aiops-v2/internal/promptcompiler/compiler.go:12`。编译顺序是 System Prompt -> Developer Instructions -> Tool Prompt Set -> Runtime Policy Prompt，见 `aiops-v2/internal/promptcompiler/compiler.go:28`。

aiops-v2 把 system/stable developer/tools 放进 stable prompt，把 skill assets、tool delta、protocol state、policy 放进 dynamic prompt，见 `aiops-v2/internal/promptcompiler/compiler.go:53`。model input builder 会按 compiled prompt、dynamic prompt、ops context capsule、memory、runtime history 的顺序构造 Eino messages，见 `aiops-v2/internal/promptinput/builder.go:12`。旧 tool-call/result 噪声会在最新 user turn 前被过滤，见 `aiops-v2/internal/promptinput/builder.go:98`。

关键 prompt：

- System 行为基线：精确、安全、有帮助；简单问题简洁；大量工具前给短进展；简单事实查询可静默用工具，见 `aiops-v2/internal/promptcompiler/system_rules.go:36`。
- Role：planner / worker / host / workspace AIOps assistant，见 `aiops-v2/internal/promptcompiler/system_rules.go:46`。
- Developer sections 覆盖 Operating Contract、Task Triage、Task Depth、Planning、Host Operations Manager、Responsiveness、Evidence、Diagnostic Protocol、AIOps Investigation Loop、Tool Boundaries、Risk、Completion、Mode Rules 等，见 `aiops-v2/internal/promptcompiler/developer_rules.go:39`。
- HostOps Manager 要求多 @主机 先结构化计划、主 Agent 不直接在主机执行命令、每个主机启动独立 host-bound 子 Agent，见 `aiops-v2/internal/promptcompiler/developer_rules.go:88`。
- Evidence rules 要求区分事实和推断，当前公共事实用 web_search，本地 runtime/prompt/session/private deployment 不用 web，验证 local agent 行为要先收集本地证据，见 `aiops-v2/internal/promptcompiler/developer_rules.go:178`。
- AIOps Investigation Loop 包含 incident/RCA、ops manual、preflight、表单、变更前后验证等大量领域规则，见 `aiops-v2/internal/promptcompiler/developer_rules.go:199`。
- Tool Use Boundaries 要求当前 host/resource 优先环境绑定工具，read-only exec 不用 shell wrapper / pipes / redirection / chaining，见 `aiops-v2/internal/promptcompiler/developer_rules.go:245`。
- Risk and Approval Boundaries 定义低/中/高风险，高风险用 scoped tool 触发 runtime approval，不在拒绝后扩大范围，见 `aiops-v2/internal/promptcompiler/developer_rules.go:270`。
- Runtime Policy 按 mode 生成约束，execute mode 明确“不要用 prose 要审批，调用 scoped tool 让 runtime pause”，见 `aiops-v2/internal/promptcompiler/runtime_policy_prompt.go:33`。

### 4. Tool Index / Tool Assembly / Tool Search

Tool Prompt Set 会为当前 assembled tools 生成 compact Tool Index 和 common tool policy，见 `aiops-v2/internal/promptcompiler/tool_registry.go:21`。Common policy 明确只读工具失败不是状态证明、权限拒绝不是健康证明、非零退出需解释 stderr/exit code、mutating tools 要 explicit user intent/scoped target/runtime approval/verification，见 `aiops-v2/internal/promptcompiler/tool_registry.go:143`。Deferred Tool Directory 明确 deferred families 不是当前 callable schema，要先 `tool_search/select`，见 `aiops-v2/internal/promptcompiler/tool_registry.go:345`。

统一工具 registry 通过 `AssembleOptions` 控制 extra tools、enabled packs/tools、profile、tenant/user、runtime capabilities、MCP health、deferred catalog、metadata transform/filter，见 `aiops-v2/internal/tooling/registry.go:13`。工具组装按 always callable、enabled、profile、pack、layer、MCP、debug、mutation 等规则过滤，见 `aiops-v2/internal/tooling/registry.go:141`。

Turn metadata 会改变工具可见性，例如 `aiops.tool.execCommandAllowed=false` 会隐藏 `exec_command`，Coroot RCA 和 ops manual 也通过 metadata gate，见 `aiops-v2/internal/tooling/turn_metadata_filter.go:154`。

但 base registry 里 `exec_command` 是 mandatory initial tool，并且 `IsAlwaysModelCallableTool` 返回 true，见 `aiops-v2/internal/tooling/base_registry.go:13`、`aiops-v2/internal/tooling/base_registry.go:29`。这会让“route metadata 隐藏 exec”和“exec 总是 model-callable”的语义变复杂。

### 5. Exec / Policy / Approval

`exec_command` 是内置核心工具，AlwaysLoad，高风险，输入支持 `command` + `args` 或兼容 `cmd`，并带 `actionToken`，见 `aiops-v2/internal/integrations/localtools/register.go:207`。它拒绝不安全 shell 语法，read-only 由 terminal policy 判断，remote host-agent 在 chat 中只允许 read-only，非 read-only 需要 action token / approval path，见 `aiops-v2/internal/integrations/localtools/register.go:261`、`aiops-v2/internal/integrations/localtools/register.go:273`。

实际执行分本地和 remote host-agent：remote 通过 `HostAgentCommandRunner.RunHostAgentCommand`，结果写成 `aiops.terminal/v1` payload 并记录 evidence refs，见 `aiops-v2/internal/integrations/localtools/register.go:423`、`aiops-v2/internal/integrations/localtools/register.go:467`。

`ToolDispatcher` 的 policy pipeline 是 lookup -> hidden policy -> MCP health -> hooks -> dedicated tool preference -> schema validation -> permission checker -> policy engine -> hook override -> permissions engine -> execute，见 `aiops-v2/internal/runtimekernel/dispatch.go:216`、`aiops-v2/internal/runtimekernel/dispatch.go:240`。approval_needed / evidence_needed 会返回 blocked dispatch result，见 `aiops-v2/internal/runtimekernel/dispatch.go:372`、`aiops-v2/internal/runtimekernel/dispatch.go:411`。

Mode policy 对 terminal command 单独判断：chat mode 允许 read-only terminal，非 read-only 需要 approval；inspect/plan 只允许 read-only terminal；execute mode 非 read-only terminal 需要 approval；governance high-risk 也会 NeedApproval，见 `aiops-v2/internal/policyengine/mode.go:221`、`aiops-v2/internal/policyengine/mode.go:288`、`aiops-v2/internal/policyengine/mode.go:358`、`aiops-v2/internal/policyengine/mode.go:421`、`aiops-v2/internal/policyengine/mode.go:467`。

Approval service 同时汇总 runtime pending approvals 和 host command approvals，见 `aiops-v2/internal/appui/approval_service.go:56`。runtime approval 决策会调用 `ResumeTurn`，denied 后可能启动一个新的 fallback chat turn，见 `aiops-v2/internal/appui/approval_service.go:83`、`aiops-v2/internal/appui/approval_fallback_controller.go:14`。fallback prompt 会告诉 agent 不要再次请求同类命令，只基于已有证据和公开资料输出受限分析，见 `aiops-v2/internal/appui/approval_fallback_controller.go:83`。

### 6. HostOps Manager Tools

HostOps 管理工具包括 `spawn_host_agent`、`send_host_agent_message`、`wait_host_agents`、`stop_host_agent`，见 `aiops-v2/internal/hostops/tools.go:11`。`spawn_host_agent` 是 mutating、medium risk、RecordEvidence，但 `RequiresApproval=false`，见 `aiops-v2/internal/hostops/tools.go:46`。这些工具只在 workspace 的 plan/execute mode 可见，见 `aiops-v2/internal/hostops/tools.go:171`。

机制方向是对的：让主 Agent 作为 manager，子 Agent host-bound。但当前 chat front door 的 `handleHostOpsRoute` 会在模型 loop 前创建 mission 并写 completed turn，这和 developer prompt 里“模型必须启动子 Agent”的机制不一致。

## 关键 Runtime Prompt 对比

| 类别 | Codex 关键 prompt | aiops-v2 关键 prompt | 主要差异 |
| --- | --- | --- | --- |
| 角色 | `gpt_5_2_prompt.md` 开头只定义 Codex CLI coding assistant、能力和安全帮助边界 | `system_rules.go` 根据 planner/worker/host/workspace 定义 AIOps 角色 | aiops-v2 更领域化，适合运维；Codex 更通用、更短 |
| 行为基线 | concise/direct/friendly、自主完成、不要猜、验证后交付 | precise/safe/helpful、简单任务短答、复杂任务进展、工具前后更新 | 方向一致；aiops-v2 prompt 重复度更高 |
| 计划 | Codex 只规定何时使用 plan、一个 in_progress、及时更新 | aiops-v2 也要求多步/RCA/运维任务使用结构化计划 | 基本一致；aiops-v2 还叠加 task depth/final gates |
| 权限 | Codex 动态注入 sandbox/approval prompt，`on_request` 清楚规定 `sandbox_permissions` 和 `justification` | aiops-v2 runtime policy 要求 execute mode 用 scoped tool 触发 approval；policy engine/permission checker 也会拦截 | Codex 权限语言更少但和执行器强绑定；aiops-v2 规则分布更散 |
| 工具 | Codex 工具说明来自 model-visible specs；deferred 工具通过 `tool_search` lazy-load | aiops-v2 有 Tool Index、common policy、Deferred Tool Directory | aiops-v2 prompt 更强约束，但 callable/visible 语义需简化 |
| 环境 | Codex 用 turn context 注入 cwd/date/timezone/sandbox/profile/network | aiops-v2 用 env context、host binding、route metadata、ops capsule | aiops-v2 运维上下文更丰富，但 route 可能提前改变 runtime 路径 |
| 多 Agent | Codex collab prompt/tool 注册，模型通过 `spawn_agent` 显式启动 | aiops-v2 HostOps Manager prompt 要求每主机 child agent，但 front door 可绕过模型创建 mission | aiops-v2 应统一到模型工具调用路径 |
| 压缩 | Codex compaction prompt 只要求 handoff summary | aiops-v2 有 context pipeline、compacted segments、observation dedupe、prompt retention | aiops-v2 机制更复杂，应该保持输出摘要更简单 |
| 完成 | Codex prompt 要求验证和清晰 final；runtime 有 stop hook | aiops-v2 有多层 final gates：plan、verification、manager synthesis、evidence、completeness | aiops-v2 更严格，但容易造成“迟迟不给结论”或重复 retry |

## 机制对比与体验影响

| 维度 | Codex | aiops-v2 | 体验影响 |
| --- | --- | --- | --- |
| Runtime 主路径 | 一个 thread/turn loop 贯穿工具、审批、压缩、final | 大部分走 Eino loop，但 hostops route 和 approval fallback 可分叉 | 用户容易看到“不像一个连续 Agent 在工作” |
| Prompt 形态 | 短 base prompt + 动态 fragments | 四层 prompt + 大量 AIOps/ops manual/domain rules | 模型可获得很多规则，但注意力和冲突成本高 |
| 工具面 | 当前可见 specs + deferred `tool_search` | Tool Index + metadata gate + deferred directory + always callable exec | 机制强，但模型可能不清楚“隐藏但可调用/不可调用” |
| Exec | Unified Exec + exec policy + sandbox + process manager | `exec_command` + terminal policy + action token + remote host-agent | aiops-v2 更适合远程运维，但缺少 Codex 那种统一 sandbox/approval/process 叙事 |
| Approval | command/MCP/sandbox approval 均归入 runtime flow | runtime pending approvals + host command approvals + fallback new turn | 审批后恢复体验不稳定，denied 后上下文可能割裂 |
| Multi-host | 普通 multi-agent tools，由模型显式调用 | appui route 可直接创建 mission 并 completed turn | 多主机操作不像 Codex 的“可观察协调过程” |
| Evidence | 工具输出和 final 验证由 prompt + hooks + tests 约束 | evidence refs、final evidence verifier、coverage gates 很强 | 证据能力是 aiops-v2 优势，但需要 UI 和 final 格式稳定呈现 |
| Context | 自动 compact，短 handoff prompt | context pipeline、retention、observation dedupe、model trace | aiops-v2 可观测性更强，但 prompt 输入复杂度更高 |

## aiops-v2 体验差的主要原因

### 1. HostOps 前门绕过了 Codex 式 turn loop

`handleHostOpsRoute` 在 runtime 前创建 mission 并写 completed turn。用户要的是“Agent 看见多个主机，制定计划，启动子 Agent，等待结果，综合回答”的连续过程；现在某些路径会直接看到一个 synthetic mission summary。这破坏了 Codex 体验里最重要的“同一个 turn 持续推进”的感觉。

### 2. Prompt 过载，且领域规则常驻

`developerAIOpsInvestigationLines` 把 ops manual、preflight、表单、reference_only/no_match、workflow、mutation、artifact、Letta 等大量细则常驻到 developer prompt。它们是有价值的规则，但不应该每个普通运维 chat 都承担这些 token 和注意力成本。Codex 的做法是把大部分能力放到工具 schema、动态 context、skills、plugins，而 base prompt 保持短。

### 3. 工具可见性与可调用性语义不够单一

`aiops.tool.execCommandAllowed=false` 可以隐藏 `exec_command`，但 `IsAlwaysModelCallableTool` 又让 `exec_command` 总是 model-callable。即使最终 dispatcher 能挡住错误调用，模型侧仍可能形成错误预期。Codex 的 model-visible specs 过滤更直接：不该当前调用的 schema 尽量不出现。

### 4. 审批链路分散

aiops-v2 有 `ToolPermissionChecker`、`PolicyEngine`、hook override、permissions engine、runtime pending approvals、host command approvals、approval fallback。它们各自合理，但组合后很难预测“某次命令会静默执行、被拒、要 evidence、要 approval，还是开启 fallback new turn”。Codex 把 exec policy、sandbox、approval 和 process manager 串成一个叙事，用户更容易理解。

### 5. Route 过早替模型决定任务形态

`BuildChatRuntimeRoute` 用 mentions/evidence/environment 直接切 session type、mode、exec permission 和 tool profile。route 是必要的，但如果 route 同时决定“是否进入 runtime loop、能不能 exec、是否创建 mission”，就会让用户一句话中的细微意图被硬分流。Codex 更倾向把环境/权限作为 context 给模型和工具层，而不是在 UI 前门提前完成任务。

### 6. Final gates 过多，可能压制交互节奏

aiops-v2 的 final gate 很强，有助于避免无证据结论，但太多 retry prompt 会让模型在普通问题上显得“过度流程化”。Codex 的关键是：简单任务短答，复杂任务推进到底；严格性更多落在工具、sandbox、tests、hooks，而不是每次 final 都叠多个门禁。

## 优化建议

### P0：先定义并落地一个 Codex-like Runtime Contract

目标不是“复制 Codex prompt”，而是把 aiops-v2 的运行时行为收敛成一个用户可预测的协议：

1. 所有可观察 agent 工作都发生在同一个 `RunTurn` / `ResumeTurn` 生命周期里。
2. UI 只展示同一种 turn timeline：user message、assistant preamble、tool call、tool result、approval pause、resume、final answer。
3. Route 只负责补充 context 和初始 tool profile，不负责提前完成任务。
4. 工具是否能被模型调用只由当前 model-visible tool surface 决定。
5. 审批、拒绝、fallback 尽量回到同一个 turn；只有无法恢复时才新开 fallback turn。

### P0：把 multi-host HostOps 改为模型驱动

建议修改 `aiops-v2/internal/appui/chat_service.go`：

- 将 `handleHostOpsRoute` 的 synthetic completed turn 行为 behind feature flag，默认 runtime v2 不走这条短路。
- 多主机 route 只写 metadata：`aiops.route.mode=multi_host_ops`、`enableToolPack=hostops`、mentions、missionId、planRequired。
- 让 planner model 在 `EinoKernel` loop 中调用 `spawn_host_agent`、`send_host_agent_message`、`wait_host_agents`。
- `CreateMission` 可改为 `spawn_host_agent` 的工具内部副作用，或在 turn start 建立 pending mission 但不写 completed final。
- UI 把 `spawn_host_agent` / `wait_host_agents` tool result 渲染成 manager timeline，而不是单独 mission summary。

这样可以直接复刻 Codex 的 multi-agent 体验：模型提出计划、工具启动子 Agent、等待、综合，而不是前门替模型创建 mission。

### P0：统一 approval 模型

建议将 runtime pending approval 和 host command approval 合并为一个 `PendingApproval` 数据模型：

- 字段必须包含 `sessionId`、`turnId`、`toolCallId`、`toolName`、`targetHost`、`command`、`risk`、`reason`、`requestedScope`、`preChangeEvidenceRefs`、`approvalOptions`。
- `ApprovalService.List` 不再拼两套来源，而是读一个 pending approval store。
- `ApprovalService.Decide` 都走 `ResumeTurn`，host command approval 也变成 tool result / resume continuation。
- Denied 结果优先作为 tool result 回给同一 turn，提示模型“用户拒绝了该 scoped action，请改用只读分析或说明 blocker”；只有 turn 已无法恢复时才触发 fallback new turn。

### P0：修正 `exec_command` 的可见/可调用语义

建议修改 `aiops-v2/internal/tooling/base_registry.go` 和组装逻辑：

- 不要让 `IsAlwaysModelCallableTool` 无条件覆盖 turn metadata visibility。
- 如果确实需要 emergency callable，拆成两个概念：`AlwaysRuntimeRegistered` 和 `ModelVisibleWhenAllowed`。
- Advisory / evidence_rca 且 `aiops.tool.execCommandAllowed=false` 时，模型不应看到 `exec_command` schema，也不应能调用成功。
- 如果用户明确请求“本地只读检查当前 ai-server”，route 可以允许 local read-only exec，但必须在 prompt 里明确 target 是 server-local。

这样能减少模型误调工具和“prompt 说不能用但 schema 还在”的冲突。

### P1：把 Prompt 拆成薄 base + route/skill/tool 动态资产

建议以 Codex base prompt 为结构参考，但保留 AIOps 领域能力：

Base prompt 保留：

- Role：AIOps assistant / manager / host worker。
- Operating contract：精确、安全、证据、不编造。
- Task triage：简单短答，复杂用计划和工具。
- Responsiveness：工具前一句话，里程碑更新。
- Tool use：只调用 visible tools，失败不是证据。
- Approval：scoped tool call 触发 runtime gate。
- Final：结果、证据、限制、验证状态。

移出 base prompt：

- ops manual 的 long decision tree。
- Coroot 特定选择逻辑。
- Letta、artifact、Workflow、form 的细枝末节。
- 大量示例句。

迁移位置：

- ops manual 规则进入 `ops-manual` skill 或 `search_ops_manuals`/`resolve_ops_manual_params` tool metadata。
- Coroot RCA 进入 Coroot skill/plugin metadata。
- HostOps manager 规则进入 `hostops` pack 的 dynamic prompt asset，只在 `multi_host_ops` route 注入。
- Approval fallback prompt 保留，但作为同 turn continuation context，而不是新 turn 默认策略。

### P1：按 route 做 prompt profile

建议定义四个 prompt profile，而不是所有规则常驻：

| Profile | 适用 | 初始工具面 | Prompt 特征 |
| --- | --- | --- | --- |
| `advisor` | 无主机绑定、概念解释、普通问答 | web/public docs、resource read | 短答、少计划、少 AIOps gates |
| `evidence_rca` | 用户给了日志/报错/症状但不能 exec | web、observability、resource/MCP | 事实/推断/缺失证据 |
| `host_worker` | 单主机只读/受控操作 | exec_command、host resources、approval | read-only direct evidence，mutation approval |
| `host_manager` | 多主机协调 | spawn/wait/send host agents、plan、summary | manager plan、分派、等待、综合 |

这些 profile 可由 `CompileContext` 选择注入短 prompt fragments，而不是在 `developer_rules.go` 里常驻所有细则。

### P1：把 Tool Search 做成 Codex 式 Loadable Spec

aiops-v2 已有 Deferred Tool Directory 和 discovery state，但建议进一步贴近 Codex：

- `tool_search` 返回可加载工具的完整 schema 或 namespaced loadable spec，而不是只返回文字目录。
- 模型调用 `tool_search` 后，下一轮 tool surface snapshot 明确新增这些工具，并在 prompt delta 中只提示新增项。
- 对 deferred MCP/plugin 工具设置 bucket limit，避免一次搜索暴露过多工具。
- model input trace 里保留 search query、返回工具、实际加载工具、隐藏原因，用于 golden trace eval。

### P1：简化 final gates，按 task depth 分层

建议把 final gates 改为分层启用：

- `trivial/simple_read`：只跑 final completeness，不跑 plan/verification/evidence retry。
- `investigation`：跑 evidence coverage、final evidence verifier、missing evidence blocker。
- `operations`：额外跑 pre/post verification gate。
- `multi_agent`：额外跑 manager synthesis gate。

同时限制每类 gate 最多一次 retry，并把 retry prompt 合并成一个 compact continuation，避免连续多轮“门禁提示”稀释任务目标。

### P1：统一 exec/sandbox 叙事

aiops-v2 不一定要照搬 Codex 的本地 sandbox，但应复制它的可解释模型：

- 明确三类权限 profile：`read-only`、`host-write-with-approval`、`danger-full-access/disabled`。
- Prompt 中只告诉模型当前 profile、网络状态、如何请求 approval。
- `exec_command` 返回结果时始终包含 target、permission mode、source、exit code、evidence refs。
- 对 mutation command，approval UI 展示 command、host、scope、risk、why、pre-change evidence、rollback/verification plan。
- Denial 后模型收到结构化 tool result：`approval_denied`，而不是只在 UI 层状态变化。

### P2：把 evidence 变成最终回答的稳定引用格式

aiops-v2 的 evidence refs 是优势，应在 prompt 和 UI 中标准化：

- tool result 自动生成 `evidenceRefs`。
- final answer 对调查/运维任务使用短 evidence bullets，每条引用 evidence ref 或说明缺失。
- model input trace 和 final evidence verifier 对齐同一 evidence ledger。
- compaction summary 保留 evidence ref、结论、限制，不保留大段 stdout/stderr。

### P2：Codex-like UX Timeline

React Chat 已经要求唯一生产路径 `TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React`，见 `aiops-v2/AGENTS.md`。建议 runtime 层保证以下 turn item：

- `assistant_progress`：工具前一句话或里程碑。
- `tool_call`：工具名、目标 host/resource、风险、是否等待 approval。
- `tool_result`：状态、关键摘要、evidence refs。
- `approval_requested`：同一 turn pause。
- `approval_decided`：approved/denied/approved_for_session。
- `child_agent_started` / `child_agent_result`：hostops manager timeline。
- `final_answer`：结论、验证状态、限制。

重点是不要再让 hostops route 写 synthetic completed turn；HostOps 应该变成这些 timeline item 的自然结果。

### P2：建立 Golden Trace Evals

建议建立 Codex-like trace eval，不只测 final text：

1. 单主机只读检查：用户 `@host 看下 CPU/内存/磁盘`，期望绑定 host、可见 exec、执行 read-only、final 有 evidence refs。
2. 多主机只读对比：用户 `@a @b 对比负载`，期望 workspace manager plan、spawn 两个 child agents、wait、summary。
3. 用户禁止执行：用户贴日志并说“不要连机器”，期望 evidence_rca，不显示 exec，不请求 approval。
4. 变更需审批：用户要求重启服务，期望 pre-check、approval_requested、approved 后执行、post-check。
5. 审批拒绝：denied 后同 turn 输出受限分析和 blocker，不重复请求同类命令。
6. deferred tool search：用户请求 Coroot RCA，期望 tool_search/load Coroot tools，再调用 read-only observability。
7. 上下文压缩：长调查后 compact，期望保留目标、证据、决策、剩余步骤。

指标建议：

- Time to first useful evidence。
- 不必要 approval 次数。
- 工具误调次数。
- route 误分类率。
- final answer evidence coverage。
- 用户拒绝后重复请求率。
- prompt token / tool schema token。
- synthetic completed turn 数量，应降为 0。

## 建议落地路线

### Phase 1：Prompt 瘦身和 Profile 化

- 在 `promptcompiler` 增加 route profile fragments。
- 把 ops manual 长规则迁到 skill/tool metadata，仅在 manual tools 可见或已选中时注入。
- 删除或合并 `developer_rules.go` 中重复的 evidence/completion/final 规则。
- 建立 prompt snapshot tests：advisor、evidence_rca、host_worker、host_manager 四类。

### Phase 2：HostOps 回归 EinoKernel Loop

- feature flag 禁用 `handleHostOpsRoute` synthetic turn。
- 多主机 route 只写 metadata 和 host_ops pack。
- manager model 通过 `spawn_host_agent` / `wait_host_agents` 工具推进。
- UI 渲染 hostops tool results 为 timeline。

### Phase 3：Tool Surface 语义收敛

- 拆分 always registered 与 model visible。
- 修正 `exec_command` 在 advisory/evidence_rca 的可见性。
- `tool_search` 返回 loadable spec，并在下一轮更新 tool surface snapshot。
- 增加 hidden/called mismatch 测试。

### Phase 4：统一 Approval / Exec

- 合并 runtime pending approval 和 host command approval。
- Denied 作为同 turn tool result 回灌。
- Approval UI 展示 host、command、risk、scope、evidence、rollback/verify。
- 把 action token、policy need approval、tool checker need approval 都归一到同一 pending approval 类型。

### Phase 5：Evidence 与 Eval

- 建立 evidence ledger final answer contract。
- Golden trace eval 覆盖 7 类核心运维场景。
- 接入 model input trace diff，比较每次改动对 prompt/tool surface/approval 的影响。

## 最小改动优先级清单

1. 先关掉或 feature flag 化 `handleHostOpsRoute` 的 completed turn 短路，让 multi-host 进入 `EinoKernel`。
2. 修改 `IsAlwaysModelCallableTool`，避免 `exec_command` 覆盖 route visibility。
3. 把 `developerAIOpsInvestigationLines` 中 ops manual 长规则拆到动态 asset，只在 ops manual 工具启用时注入。
4. 把 approval denied fallback 改成同 turn continuation tool result，保留 new turn fallback 作为兜底。
5. 为四个 route profile 加 prompt snapshot/golden trace。

完成以上 5 项后，aiops-v2 的体验会更接近 Codex：用户能看到 Agent 连续工作、工具和审批边界清楚、多主机协作由模型显式推进、final answer 有证据和验证状态，而不是被 route 和 prompt 规则分叉成多个不透明路径。
