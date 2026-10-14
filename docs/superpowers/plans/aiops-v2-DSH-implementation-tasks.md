# aiops-v2 DSH 实施任务与依赖

更新：2026-10-14 UTC｜状态：bridge 级 POC 仍通过既有测试；RuntimeKernel 接线设计已写入 section 10.1，生产集成待接口边界审阅；P0 未验收；Draft PR #22 head 811a3f3 的最新 CI：web 通过，backend hardening 与 Workflow Canvas snapshot 失败，详见 section 10.2

## 1. 当前进度与范围

- 已完成：基于 aiops-v2 `ce539e3`、DSH `d743267` 的只读调研；迁移方案已交付并获确认；T1 本地基线检查通过。
- 当前实现：本任务使用独立工作区 `/Users/zhongxuan_li/Desktop/dot/aiops-v2-dsh-harness-current`、分支 `dsh-harness-poc`，当前 HEAD `19a1ed3`，上游 `https://github.com/lizhongxuan/aiops-v2.git`。另外两份旧 checkout（Desktop `aiops-v2-dsh-harness-poc` 与 Documents `task-2/aiops-v2`）均保留且未在本阶段修改；Documents checkout 的未跟踪 `internal/runtimekernel/.data/` 仍留在原处。
- 已完成 POC 部分：新增 `internal/dshbridge` stdio JSON-RPC host bridge 与无密钥模拟测试；pinned DSH 扩展源码基于 `d743267388641bc76f17c45ce8b4c231aed1d32c` 修改，并将作为 patch 交付。Go 测试覆盖审批等待、精确 turn/call 绑定、取消与 Go dispatch 静止、durable turn/end、stream 不进事件游标和游标后恢复。
- 尚未接线：当前 Go bridge 的 `ToolDispatcher` seam 是强制注入接口，但尚未由生产 `RuntimeKernel` 提供；没有修改 RuntimeGateway/modelrouter/appui transport 来启动 DSH 并把用户 turn 映射到该 seam。因此这些测试验证协议桥和安全契约，不冒充生产端到端运行。
- 单独处理：小 bug 任务 `01a13847-d8ac-7187-a273-b9bcabed4005` 使用独立分支/PR，不混入迁移。
- 本次范围：DSH 执行引擎、受控工具桥、状态/流适配、测试、部署配置与使用文档。
- 保留：React/API、Go 运维控制面、Runner、现有数据库、安全门禁和工具实现。先保持模型选择，旧会话沿用旧引擎。
- 不包含：生产部署、合并 PR、未经确认的全量历史迁移或安全策略放宽。用户授权 dot 云电脑本地预览并在云浏览器展示，不要求公网链接；不向公网开放管理入口。DSH 官方仍是未经安全审计的开发预览版本。

## 2. 接口与安全边界

目标链：React → 现有 Go runtime 入口 → DSH 进程 → 工具桥 → Go ToolDispatcher → host-agent / Runner / MCP。

- **引擎接口**：在 `runtimekernel` 入口选择实现；Go 管理会话准入和引擎归属，DSH 驱动其会话的模型/工具循环。同一 turn 只能有一个 driver。
- **协议接口**：新增 `internal/dshbridge/`，使用 pinned SDK stdio JSON-RPC 扩展。启动握手、入队、事件、turn/call 取消、退出分别处理；stdout 只允许协议帧。
- **工具接口**：新增 `dsh/plugins/aiops-tools/`；schema 来自统一 tooling assembly。调用携带 session/turn/call ID、目标及参数指纹，执行必须回到 `dispatch.go` / `tool_dispatch_pipeline.go`。模型不能提供可信授权结论。
- **状态接口**：DSH 日志保存模型上下文；Go 保存审批、业务记录与 UI 投影。维护 session/turn/事件游标映射，明确唯一写入方、持久化顺序和去重键。
- **界面接口**：事件映射进入 `appui/transport_*`，继续使用 AssistantTransport；不增加页面自己的流或从 final 文本推断执行状态。
- **权限边界**：保留 Policy、Permission、ActionToken、资源锁、主机绑定及验证门禁；DSH 不直接获得生产 shell/数据库凭据或执行旁路。审批 UI 的“通过”必须由 Go 校验对应目标、参数及有效状态。

## 3. 任务与验收

任务受理不等于实现。当前状态以本表和第 6 节的实测证据为准；T1 已完成基线验证；T2–T4 已有协议桥 POC，但生产 RuntimeKernel/appui 接线、上游依赖测试和完整 P0 验收尚未完成。

| ID | 交付与关键文件 | 完成条件 | 当前状态 |
| --- | --- | --- | --- |
| T0 | 调研、迁移方案、实施授权 | 已完成；基线 `ce539e3` / `d743267` 已确认 | 完成（父线程已交付并获授权） |
| T1 | 独立迁移分支；行为契约与旧引擎基线 | 记录现有命令及结果；确认未带入 bug 分支改动 | 完成（独立 clone；四个 Go 包基线测试全通过） |
| T2 | `internal/dshbridge/`、固定版本 profile、进程隔离 | 初始化协商、严格 JSON-RPC framing、stdout 错误 fail-closed；待进程 launcher、stderr 日志和上游类型/运行测试 | 进行中（Go bridge 测试与 race 通过；pinned patch 可应用且 Bun 语法构建通过；进程 launcher、上游类型与运行测试待完成） |
| T3 | 工具桥与审批恢复，保留 Go Dispatcher | schema 限定到 Go surface；请求回 Go 注入 dispatcher；挂起期间不回结果；取消/超时/拒绝保持 fail-closed；生产 dispatcher 必须接 RuntimeKernel 和 durable approval | 协议层完成（`ErrApprovalPending` 保留原 JSON-RPC request，`ResolveToolCall` 按原 binding/call 恢复，取消先撤销 Go approval；真实 `RuntimeKernel` dispatch adapter 未实现，不能声称通过 ActionToken/资源锁端到端） |
| T4 | `session.go` / `types.go`、`appui/transport_*` | 取消确实停止执行；重连去重；崩溃后无重复变更；流、pending、终态和刷新显示一致 | 进行中（Go durable cursor/replay、stream 与 end 语义模拟测试通过；DSH 源码补丁增加持久 Session 恢复、流和取消；未连到 AssistantTransport/appui） |
| T5 | promptcompiler / agentassembly / modelrouter 单会话适配 | 工具面一致；主机作用域不扩大；模型参数/错误/fallback 契约测试通过；新旧会话正确分流 | 待开始（P0 评审通过后） |
| T6 | agentmgr、多主机资源锁、压缩、MCP/Skill、Runner | 无跨主机串线；子 Agent 状态正确；压缩后审批/目标保留；真实隔离工作流完成预检→审批→执行→验证→Run Record | 待开始（P1 通过后） |
| T7 | 启动脚本、Docker/K8s 配置、日志和操作文档 | 固定依赖可构建；数据持久化与健康检查有效；密钥不进日志；启动、停止、恢复、回滚步骤可复现 | 待开始（本地云预览安全约束已补充） |
| T8 | 集成回归、Agent Eval、故障演练与 PR 材料 | 最终提交完成适用检查；记录通过/失败/未运行；审阅用 PR 或 PR 草稿材料就绪；不合并、不部署 | 进行中（最新 head CI：web 通过；backend hardening 因未改动旧 UI 字段扫描命中失败；workflow-canvas 因未改动的 workflow-studio screenshot 失败。不是全绿，详情见第 9 节） |

**依赖顺序**：T0 → T1 → T2 → T3/T4 → P0 评审门 → T5 → T6 → T8。当前下一依赖是验证上游补丁，并为 RuntimeKernel 实现 DSH turn/dispatcher 接线、挂起审批恢复、持久化 EventStore 和 AssistantTransport projection；P0 门未过前不扩展 T5/T6。T7 在 T2 后可并行设计，最终验收依赖 T5/T6；T8 同时依赖 T7。

**里程碑**：P0＝T1–T4 通过，证明接入与安全语义可保持；P1＝T5 单会话可用；P2＝T6 完整业务闭环；P3＝T7–T8 验证、文档和审阅材料齐备。P0 前不承诺工期；失败门槛未解决不得扩大切换。

## 4. 已知缺口与失败处理

- **官方 SDK 原始协议缺口；本分支提出的 pinned 补丁**：基线 `d743267` 无 turn cancel、server-to-client 审批请求、assistant stream 转发或 protocol negotiation。本分支补丁为 `aiops-dsh-bridge.v1` 增加显式版本/工具 schema 协商、服务端 `aiops.tool.dispatch` 请求、精确 turn cancel、assistant stream 与 durable event replay；这些能力仅存在于补丁，尚未运行上游测试，不代表官方 SDK 已提供。
- **审批执行边界**：补丁把模型 tool call 转成 Go host request，并把 Agent tool scope 限制为 Go 传入的 exact schema。Go `ToolDispatcher`/ActionToken/目标与参数校验仍需 RuntimeKernel adapter 接线；当前模拟 callback 不代表生产审批已接通。审批桥不可用时须拒绝执行，不降级为自动允许。
- **入队、idle 与结果不等价**：Go bridge 把 prompt receipt、idle notification 当作非终态，仅在 durable `turn/end` 事件写入 Go EventStore 后结束 DSH turn。assistant stream 帧是瞬态 UI 数据，不能推进 durable cursor；DSH session replay 也不授权重放变更操作。
- **执行后、记账前崩溃**：以调用 ID、参数指纹和执行台账对账；结果不明标记“待核实”，不自动重试变更操作。只读重试也须限次、可观测。
- **协议/版本变化**：固定 DSH 版本；握手和事件 schema 不匹配时停止接入，保留日志。不得把未知字段/状态默认为成功。
- **回滚**：停止新建 DSH 会话，安全结束/取消活动任务，切回旧引擎和已验证镜像。保留数据库备份与 DSH 日志；不强转活动 checkpoint，不做双引擎生产执行。

## 5. 完成定义与进度记录

完成必须包含：最终代码、适用单元/集成/前端检查、隔离环境真实 Runner 工作流、重启/取消/审批/回滚演练、Agent Eval 对比及更新后的操作文档。只跑通原型、mock 或只读问答不算迁移完成。

安全用例必须全过；质量、时延和成本阈值在基线测量后确定。所有未覆盖项、失败和运行限制须显式列出。若发布分支/PR 所需权限待确认，先交付 diff 与 PR 草稿材料，不自行扩大授权；生产合并/部署另行确认。

每次进度更新记录：任务 ID、状态（待开始/进行中/阻塞/完成）、commit、验收命令与结果、阻塞、下一依赖。没有可复查证据不标“完成”；不承诺零 bug。

## 依据

- [aiops-v2 行为契约](https://github.com/lizhongxuan/aiops-v2/blob/ce539e3634f678c74b801b1b5cbcfd718ebaedf9/AGENTS.md)；[架构与部署](https://github.com/lizhongxuan/aiops-v2/blob/ce539e3634f678c74b801b1b5cbcfd718ebaedf9/README.md)
- [DSH SDK 协议与限制](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/packages/sdk/protocol/README.md)；[客户端结果语义](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/packages/sdk/client/README.md)；[官方安全说明](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/SAFETY.md)


## 6. 本地执行进度与行为契约

### T1 — 完成（2026-10-14）

- 工作区：`/Users/zhongxuan_li/Desktop/dot/aiops-v2-dsh-harness-poc`；分支 `dsh-harness-poc`；基线 `ce539e3634f678c74b801b1b5cbcfd718ebaedf9`。专用 clone 从基线独立建立，没有导入 sibling checkout 的未提交更改。
- 环境：`go version go1.27.1 darwin/arm64`；Node `v26.8.2`；仓库要求 Go 1.24.3。
- 基线测试实际通过：`env GOPROXY=https://proxy.golang.org,direct GOCACHE=/tmp/aiops-dsh-gocache GOPATH=/tmp/aiops-dsh-gopath GOMODCACHE=/tmp/aiops-dsh-gomodcache go test ./internal/runtimekernel ./internal/tooling ./internal/appui ./internal/server`。四个包均返回 `ok`。代理和缓存只在该命令上设置；没有改系统配置。
- Library 原始方案与实施清单已按确切 ID 调用 `prepare_materialize`，并用官方 transfer helper 完成本机传输：`docs/superpowers/specs/aiops-v2-DSH-migration-plan.md`（4478 bytes，ID `libfile_b476ca2eba9c8191bf06ef89f411dc9c`）和本文件（7199 bytes，ID `libfile_519aa1a28f5881919241e96b379f490a`）。已用本地读取、文件大小和 `user.library-file-id` xattr 验证；首次受限网络下载失败，之后按授权提升网络后成功。
- T1 不包含 DSH 代码。完整构建、全仓测试、CI、Agent Eval 尚未运行；不得从四个包的通过推导为全仓通过。
- 固定 DSH `d743267388641bc76f17c45ce8b4c231aed1d32c` 源码显示：SDK server 仅转发已提交 Session event 和 agent 状态；同一版本内部 `agent/assistant-stream` 事件供 SessionController 使用，Agent 也提供 `cancel()`，但 SDK wire 未暴露这些能力。客户端已有 frame revision、dense index、settlement seq/eventType 匹配与 reconnect rebaseline 规则；扩展必须保留这些语义，不能用到达顺序推断终态。

### P0 行为契约（改动前约束）

- **用户可见结果**：获准使用 DSH 的新会话仍通过现有 React/AssistantTransport 显示有序流、审批等待、取消和恢复；已有会话继续走原引擎。模型默认选择保持不变。
- **生产链路**：`ChatCommand → appui.RuntimeGateway → runtimekernel.RuntimeKernel.RunTurn/ResumeTurn/CancelTurn → model/engine → Go ToolDispatcher → Policy/Permission/approval/ActionToken/resource lock → checkpoint/session persistence → appui transport projection → AssistantTransport`。外部 DSH tool 请求必须回到同一 dispatcher，并使用 Go 侧可信 session/turn/target/policy snapshot。
- **权威事实源**：Go `SessionState` / `TurnSnapshot` / checkpoints / `PendingApproval` / action-token 校验和持久化 `AgentEvent` 决定业务状态及 UI 状态；DSH session log 仅作为 DSH 模型上下文。事件映射需使用稳定去重键，不得把 DSH `idle`、prompt 入队回执或 SDK `finalResponse` 单独当成业务 turn 完成。
- **确认的协议缺口**：官方 `d743267` 协议没有版本协商、turn cancel/session-close 方法，也没有可用的 server→client 请求（因此无原生审批请求）；`run()` 以整 agent idle 收集事件，`finalResponse` 不是因果绑定到该次 prompt 的业务结果。官方还明确标记 Harness 为未经安全审计的开发预览；`sdk-minimal` 默认 shell policy 可修改进程可访问的任意路径。证据见官方 [SDK protocol](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/packages/sdk/protocol/README.md)、[SDK client](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/packages/sdk/client/README.md)、[SAFETY](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/SAFETY.md)、[sdk-minimal](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/packages/bundle/sdk-minimal/README.md)。
- **不可降低的安全门**：模型/DSH 只能提出动作；Go Dispatcher 必须重新验证已组装工具面、目标绑定、参数指纹、权限快照与 ActionToken。批准拒绝、过期、错参、身份不匹配都不得执行；变更动作不得在重启后自动重放。关闭 DSH 进程不等于确认 Go 在途 mutation 已停止或已经回滚。
- **同一待审批操作不能重复发起**：Go 的该操作 pending approval 是权威状态；UI 在其“审批中”时禁用这一操作的重复提交，并在拒绝、过期或批准结果落定后按真实状态恢复。后端需以 session/turn/operation/input identity 实施并发 admission 与幂等校验，阻止连续点击、并发 HTTP、刷新/重连后重复事件造成重复执行；不得全局禁用其他操作。验收覆盖连续点击、并发请求、刷新重连、拒绝/过期/批准后的重复事件。
- **本轮 Runtime/UI 回归**：`RunTurn` 新增 pending controlled approval/evidence admission guard；两个用例确认原 turn/approval 不被覆盖且不会执行 mutation。Runner Candidate Apply 统一 artifact/server-controlled pending 状态、重复决策 identity 和等待态焦点。它补足该 UI 路径，但不能证明跨实例 HTTP 幂等，也不能代替 DSH RuntimeKernel 接线。
- **必须新增的失败用例**：握手/协议版本未知、stdout 混入非协议日志、目标或参数篡改、ActionToken 过期/复用、审批拒绝和过期、同一 pending 操作并发/重复提交、审批结果后的重复事件、回连去重、取消期间 Go mutation、执行后记账前崩溃、恢复后重复调用、session/host 串线。P0 任一安全用例失败即停止迁移。

### 部署与预览边界

仅允许在 dot 云电脑本机启动应用并由同一云电脑的浏览器展示；不需要公网链接。要提供入站路径时必须有 HTTPS 与认证保护；持久数据目录必须独立保存；管理服务绑定 loopback；gRPC 不得对外暴露。父线程提醒的 Workflow Canvas nightly 既有失败要与本分支结果分开核对；本任务没有生产部署授权。

### 7. 本轮实现与验证记录（2026-10-14）

- T2/T3/T4 bridge POC：新增 `internal/dshbridge/bridge.go`。标准库 stdio JSON-RPC client 验证精确版本与必需能力，按 Go tool schema 白名单处理 server-to-client dispatcher request，绑定 host-owned session/turn/target/tool-surface/permission identity，以参数 SHA-256 交给必需的 injected dispatcher；没有 direct executor、shell 或 fallback。未知版本、工具、call 重放和绑定不匹配均 fail closed。
- durable 与 transient 分流：`EventStore` idempotently commits `(sessionID, seq)`；`assistant.stream` 不推进 cursor；`WaitTurn` 只将 durable `turn/end` 视为 DSH turn 终态。`RecoverTurn` 从 Go cursor 请求 DSH replay，不重新执行 tool call；终态提交前取消并等待在途 Go dispatch quiesce。
- POC 模拟回归命令：`env GOPROXY=https://proxy.golang.org,direct GOCACHE=/tmp/aiops-dsh-gocache GOPATH=/tmp/aiops-dsh-gopath GOMODCACHE=/tmp/aiops-dsh-gomodcache go test ./internal/dshbridge -count=1`，通过。Fixture 不使用真实 provider key，也不执行真实运维工具；approval 通过阻塞 dispatcher 的明确 approve channel 模拟。
- AIOps 运行时回归：`env GOPROXY=https://proxy.golang.org,direct GOCACHE=/tmp/aiops-dsh-gocache GOPATH=/tmp/aiops-dsh-gopath GOMODCACHE=/tmp/aiops-dsh-gomodcache go test ./internal/runtimekernel ./internal/appui ./internal/server -count=1` 通过；`env GOPROXY=https://proxy.golang.org,direct GOCACHE=/tmp/aiops-dsh-gocache GOPATH=/tmp/aiops-dsh-gopath GOMODCACHE=/tmp/aiops-dsh-gomodcache go test -race ./internal/dshbridge -count=1` 通过。新增 RunTurn admission 用例覆盖 pending controlled approval 与 pending approval 不被新 turn 覆盖。
- Web 回归：`npm test -- --run src/runner/WorkflowCandidateActionBar.test.tsx src/runner/WorkflowCandidateWorkspaceActionBar.test.tsx` 通过（36/36）；`npm run typecheck` 通过；使用临时 Chrome channel 配置运行 `npx playwright test e2e/workflow-candidate-apply-dialog.spec.js --config playwright.local-chrome.config.js --project=chromium`，`1/1` 通过。首次 Vitest 有一条旧焦点断言失败；将新等待态焦点改为可访问状态提示后重跑通过。新增 pending screenshot 已目检；临时配置、Playwright 结果与本地 trace 目录已清理。
- 交付/CI：本地 `git diff --cached --check` 通过；commit `1ce2228` 已推送到 `origin/dsh-harness-poc`；[Draft PR #22](https://github.com/lizhongxuan/aiops-v2/pull/22) 已创建，页面显示 backend-agent-harness、web、workflow-canvas-pr 三项 CI 排队，尚无结果。GitHub CLI token 无效，PR 通过已登录的 Chrome 会话创建。
- 上游 pinned 扩展：`/tmp/deepseek-harness-d743`，base `d743267388641bc76f17c45ce8b4c231aed1d32c`；补丁文件位于 `docs/superpowers/patches/deepseek-harness-d743-aiops-bridge.patch`。扩展内容包括 exact tool proxy/restriction、turn cancel、durable replay、assistant stream、Session persistence resume 与无密钥 Vitest。`bun build ... --packages=external` 通过（218 modules/37ms），这是语法转译，不是 TypeScript 类型检查或 Vitest 运行。
- 上游依赖验证受阻：`pnpm install --frozen-lockfile --filter=@deepseek-ai/dsh-sdk-jsonrpc-server... --ignore-scripts` 无输出持续约 90 秒，未创建 `node_modules`，随后中止；上游 TypeScript typecheck/Vitest 尚未运行。补丁须在依赖可用后补跑 typecheck 和 SDK server tests。
- 范围与限制：当前 `ToolDispatcher` 为必需的注入接口，未由生产 `RuntimeKernel` 构造；没有 process launcher、Engine selector、`appui`/`AssistantTransport` stream projection 或旧/新引擎 session rollback。故 P0 未验收，不能进入 T5。基于 Runtimeshell 的生产审批、ActionToken、资源锁、持久化 DB 与 Runner 操作尚未通过端到端验证。

### 下一步

先在依赖可安装时验证上游 patch typecheck/tests。之后需要为 `RuntimeKernel` 设计并实现唯一的 DSH turn owner/ToolDispatcher adapter、进程生命周期与 `AssistantTransport` projection，并用真实 production-path integration fixture 验证拒绝/批准/取消/恢复与 no-replay。只有 P0 所有安全门和端到端测试通过后，才进入 T5。


### 8. RuntimeKernel 接线核验与 PR CI 跟进（2026-10-14）

- 本轮工作区：`/Users/zhongxuan_li/Documents/Codex/2026-10-14/task-2/aiops-v2`，分支 `dsh-harness-poc`，基线仍是 `ce539e3634f678c74b801b1b5cbcfd718ebaedf9`。该目录下原有未跟踪 `internal/runtimekernel/.data/` trace 未触碰。原 Desktop checkout 在当前执行 sandbox 中只读；后续改动转在本任务 writable checkout，并以独立分支继续。
- CI 首轮（原 `d42c54e` head）结果：`web` 成功；`backend-agent-harness` 失败，报告单个提交包含 6 个 production files / 1,564 行且 screenshot baseline 缺 `AIOps-Change-Exception: baseline`。已把功能代码拆成满足每 commit 预算的提交，并让 screenshot 单独提交，带有 baseline 类型、理由和可核验的本任务 Chromium 目检记录。没有修改 CI gate，也没有把运行时代码申报为 mechanical exception。
- `workflow-canvas-pr` 首轮失败于 `web/tests/workflow-studio.snapshot.spec.js:65`：截图有 13,790 像素差异（图像 0.02 比例），涉及 `workflow-scheme-c-overlay.png` 基线。本 PR 未修改该 spec 或其基线；先记录为 CI 可见失败，不能称为通过。重推后仍需检查新 run。
- 本地回归（在拆分源码文件后重新执行）：
  - `env GOPROXY=https://proxy.golang.org,direct GOCACHE=/tmp/aiops-dsh-gocache GOPATH=/tmp/aiops-dsh-gopath GOMODCACHE=/tmp/aiops-dsh-gomodcache go test ./internal/dshbridge -count=1` — 通过。
  - 同一环境变量下 `go test -race ./internal/dshbridge -count=1` — 通过。
  - `go test ./internal/runtimekernel ./internal/appui ./internal/server -count=1` 第一次被 sandbox 拒绝 `httptest` 对 `[::1]:0` 绑定；以本地 loopback 权限重跑同一命令后，三个包均通过。`go test ./internal/server -run '^TestAgentProfileAPI$' -count=1` 在本地 loopback 权限下也通过。
  - Web：`npm test -- --run src/runner/WorkflowCandidateActionBar.test.tsx src/runner/WorkflowCandidateWorkspaceActionBar.test.tsx` — 36/36 通过；`npm run typecheck` — 通过；Chromium `npx playwright test e2e/workflow-candidate-apply-dialog.spec.js --config playwright.local-chrome.config.js --project=chromium` — 1/1 通过。该临时配置和运行产物已清理。
- pinned DSH：从干净 worktree 检查 commit `d743267388641bc76f17c45ce8b4c231aed1d32c` 后，`git apply --unidiff-zero --check docs/superpowers/patches/deepseek-harness-d743-aiops-bridge.patch` — 通过。应用补丁后 `bun build packages/sdk/server/src/index.ts --target=bun --packages=external --outdir=/tmp/dsh-aiops-syntax-check` — 通过（93 modules）。该命令只验证可转译/打包，不是 TypeScript typecheck 或 Vitest。先前默认 pnpm install 报 `ERR_PNPM_LOCKFILE_WRITE_FILE`，目标为只读的 `~/Library/pnpm/global/v11`；将 `PNPM_HOME`、`XDG_CONFIG_HOME`、npm cache、pnpm store/global-dir 全重定向到 `/tmp` 后再次运行 `pnpm install --frozen-lockfile --filter=@deepseek-ai/dsh-sdk-jsonrpc-server... --ignore-scripts`，仍静默约 90 秒，手动中止（exit 130），无 `node_modules/.bin/vitest`；TypeScript typecheck/Vitest 仍未运行。
- RuntimeKernel 核验发现直接 dispatcher 注入尚不构成安全接线：
  1. `RunTurn` 由 `runHostIterationLoop` 独占管理 prompt/StepContext/dispatch/final gate；工具结果在 `processDispatchResult` 挂起时会执行 `markTurnBlocked` 并把 `blocked` 返回给调用方（`internal/runtimekernel/runtime_kernel.go`）。
  2. 现有 `ResumeTurn` 验证 ActionToken 后调用 `resumePendingToolCall`，随后继续旧的 `runHostIterationLoop`（同文件）。上游 DSH 没有专门的恢复 RPC，但 JSON-RPC 原 request ID 可由 Go 挂起并稍后回答。本轮新增 bridge 级 continuation：dispatcher 仅在 Go approval 已持久化后返回 `ErrApprovalPending`；bridge 保存原 request ID，`WaitTurn` 返回 `ErrTurnSuspendedForApproval`，宿主返回现有 blocked 状态；Go `ResumeTurn` 完成并校验对应操作后，宿主调用 `ResolveToolCall` 回答同一 request。取消要求 `PendingApprovalCanceler` 先撤销持久化 Go approval。该机制目前是 in-memory protocol POC，未连生产 RuntimeKernel，也不能跨进程恢复。
  3. 仓库没有 DSH `SessionEvent` 持久化表/接口。现有 `AgentEventService` 自己分配 per-session `Seq`，GORM 记录是 `aiops_agent_events`，且 append 与 projection save 是两个存储操作；它不能直接满足 DSH `(sessionID, seq)` 原子去重游标及准确 turn attribution。
  4. 因此 P0 不能通过简单增加 `RuntimeKernelConfig.DSHRunner` 完成。仍需将上述挂起调用 seam 接入 RuntimeKernel `RunTurn`/`ResumeTurn`/`CancelTurn` 生命周期，给 EventStore 增加原子序号/turn 归属并持久保存 pending call→request handle 映射，还需把 DSH event/stream 映射到既有 `TurnSnapshot`、checkpoint 和 AssistantTransport。引擎归属应写入 session，并保留旧引擎按会话恢复；模型继续通过现有 ModelRouter 解析。此方案保留 Harness 编排并满足原授权方向，完成前保持 fail closed。
- 状态：已完成 bridge protocol POC、安全 admission guard 和 UI waiting 修复；生产 RuntimeKernel/AssistantTransport 适配仍阻塞，P0 未验收。没有真实模型、真实运维工具、双执行、部署或合并。

### 9. Go approval continuation 补充与最新 CI（2026-10-14）

- 新增协议 continuation：`ToolDispatcher.Dispatch` 可在 Go 已持久化准确审批后返回 `ErrApprovalPending`。bridge 保留原始 JSON-RPC request ID 和 `ToolCall`，不向 DSH 返回 success/error；`WaitTurn` 返回 `ErrTurnSuspendedForApproval`，宿主可映射为现有 blocked 结果。Go `ResumeTurn` 经过 ActionToken/参数/目标验证、执行且结果可确定后，接线方再以相同 host binding 与 `callId` 调用 `ResolveToolCall`；错 turn、重复 resolution、终态/cancel 中 resolution 都被拒绝。bridge 拒绝有未决 Go approval 的 `turn/end`。
- pending cancellation：整个 turn 取消先停止本地 dispatch 并等待 quiesce；pending approval 必须由 `PendingApprovalCanceler` 按 binding 和原 `ToolCall` 撤销后，bridge 才发送 DSH `session/cancel`。没有撤销接口时拒绝取消；取消 RPC 结果不明时关闭 bridge，保持 fail closed。此逻辑仅是协议 seam，没有生产 RuntimeKernel adapter。pending request handle 仍驻留进程内；DSH 进程崩溃后审批恢复/重新绑定尚未实现。
- 新增回归：`TestPendingGoApprovalKeepsOriginalDshRequestOpenUntilResume` 与 `TestCancelPendingApprovalRevokesGoApprovalBeforeDshCancel`；现有 stream/idle/end、call replay、turn cancel、dispatch drain 和 event replay tests 继续覆盖。命令：`env GOPROXY=https://proxy.golang.org,direct GOCACHE=/tmp/aiops-dsh-gocache GOPATH=/tmp/aiops-dsh-gopath GOMODCACHE=/tmp/aiops-dsh-gomodcache go test ./internal/dshbridge -count=1` — 通过；同环境 `go test -race ./internal/dshbridge -count=1` — 通过。
- PR 最新已完成 CI run `38512186242`（head `521a59c`）：`web` 通过；`backend-agent-harness` 失败于 `scripts/check-aiops-single-assistant-message.sh` repo-wide residue scan，命中既有 `candidateState` 字符串（例如 `web/src/runner/workspace/workflowInteractionContract.test.ts`、`web/src/runner/workspace/workflowWorkspaceSelectors.ts` 和 `web/src/components/runner/RunnerCanvas.tsx`）。这些路径不在本 PR diff；未改该无关 UI 行为或修改 gate。`workflow-canvas-pr` 失败于 `web/tests/workflow-studio.snapshot.spec.js:65`，`workflow-scheme-c-overlay.png` 与截图基线有 13,790 像素（0.02 比例）差异；本 PR 未修改该 spec 或其基线。PR 保持 Draft、不合并。
- pinned Harness patch 状态不变：base `d743267388641bc76f17c45ce8b4c231aed1d32c` 上 clean `git apply --unidiff-zero --check` 通过；应用后 Bun server bundle/syntax 构建通过（93 modules），不是 typecheck/Vitest。pnpm 目标目录重定向后依赖安装仍约 90 秒无输出并中止，故上游 TypeScript typecheck 和 SDK Vitest 未运行。没有真实模型 key、生产工具或部署。
- 该增量完成的是可审查的 bridge compatibility seam，不是生产迁移阶段完成：仍须把它接到 `RuntimeKernel.RunTurn/ResumeTurn/CancelTurn`，持久化 pending RPC mapping 与 DSH SessionEvent，接好 event/stream projection、启动和进程恢复，并保留 per-session legacy engine rollback。P0 继续未验收。

## 10. RuntimeKernel 单会话集成行为契约（2026-10-14）

- 用户可见现象：当前 PR 只有 dshbridge 协议 seam；生产 RuntimeKernel.RunTurn 仍直接调用旧 runHostIterationLoop，因此 UI turn 不会进入 DSH。
- 已确认根因：RunTurn 的输入准入、用户消息/checkpoint 持久化、模型采样、tool dispatch 和最终 Assistant 输出都在同一 RuntimeKernel 路径；ResumeTurn 对审批验证 ActionToken 并执行 resumePendingToolCall 后仍调用旧 loop；CancelTurn 只取消 in-flight Go context。bridge 尚未作为 session engine 接入，也没有可复用的外部最终输出提交入口。
- 生产调用链：ChatCommand → appui admission → RuntimeKernel.RunTurn → turn assembly / checkpoint → 当前旧 runHostIterationLoop → ToolDispatcher / FinalContract → AiopsTransportState / AssistantTransport。DSH 路径必须由同一 RuntimeKernel 管理准入、session/turn identity、工具权限、approval/resume、cancel、终态、checkpoint 和 UI projection。
- 权威事实源：持久化 SessionState / TurnSnapshot、canonical rollout/checkpoint、Go ToolDispatcher、Go PendingApproval 与 ActionToken、AssistantTransport 投影；DSH 事件仅能按稳定 session/turn binding 重放，idle/queue receipt/transient stream 不表示业务完成。
- 必须保持：默认旧引擎；已归属旧引擎的会话不自动切换；新 DSH 会话需由 host 配置选择并持久化归属；同一 turn 不能双执行；工具执行必须调用 Go dispatcher；target、tool surface、permission snapshot、ActionToken、resource lock 和幂等约束不变；最终状态只能由结构化状态提交；取消必须停止精确 turn 并先撤销未决 Go approval。
- 失败复现（待先加并运行）：新增生产 RunTurn 集成用例，注入假 DSH peer，要求被显式选为 DSH 的单会话确实调用 DSH peer；要求受限 tool call 只进入 Go dispatcher，审批挂起时不返回 Harness 成功、Resume 使用原 DSH callId、Cancel 阻止后续 mutation、终态仅由 durable turn/end 决定。修复前该用例应因 RuntimeKernel 没有 DSH engine seam 而失败。
- 验收命令：go test ./internal/runtimekernel ./internal/dshbridge ./internal/appui -run 'TestRunTurn_DSH|TestResumeTurn_DSH|TestCancelTurn_DSH|TestRecoverTurn_DSH' -count=1，另加受影响包完整测试和 race。失败与未运行结果均需记录。
- 实施边界：先建立 RuntimeKernel 单会话 contract 和 production-path red/green，再评估是否能在 5 个生产文件 / 500 行及不跨 3 个以上 runtime layer 的预算内接线。若 final gate、pending approval recovery 或事件持久化要求突破边界，先形成接口/迁移设计和具体证据；期间 DSH 选择必须 fail closed，禁止用简化 loop 或仅 SDK.run 演示冒充集成。

### 10.1 接口评估与分阶段设计

源码证据（当前 HEAD 19a1ed3）：

- internal/runtimekernel/runtime_kernel.go:576 的 RunTurn 在 admission、message/checkpoint persistence 后直接进入 runHostIterationLoop（调用点约 775）；internal/runtimekernel/runtime_kernel.go:952 的 ResumeTurn 验证 ActionToken、调用 resumePendingToolCall 后仍进入同一个旧 loop（调用点约 1132）；CancelTurn 在 1567 只请求取消已注册的 Go context。
- runHostIterationLoop 从约 1994 行起构建 context、tool surface、TurnAssembly/StepContext 和 checkpoint。step dispatcher 在收到旧模型响应之后、tool dispatch 之前创建（约 3034）；tool 结果 materialization、canonical checkpoint、transport projection 与 completion/final gate 也在同一 loop 的局部处理闭包/代码段里。把 DSH 放在 loop 前面会缺少经过冻结的 StepContext 和 dispatcher；把它放在 loop 后面则会重复调用模型。
- ResumeTurn 的 resumePendingToolCall（约 5195）会用当前 Go ActionToken 和重新构建的 dispatcher 执行工具，但只把已校验结果交回旧 loop；它没有恢复 DSH 挂起的 JSON-RPC request。CancelTurn 没有调用 bridge 的 session/cancel；没有生产 adapter 时既无法确保远端循环停止，也无法在取消前撤销 DSH 映射的 approval。
- internal/dshbridge.Bridge 的 active turn、request ID 和 pending call 在内存 map 中；EventStore 只有接口与测试 memory store，没有生产事务实现。internal/appui 没有 DSH event/stream 到 canonical TurnSnapshot / AssistantTransport 的生产映射。
- 这些证据说明协议层可通过 pinned patch 扩展，但生产接线不能只给 RuntimeKernelConfig 填一个 runner。直接接入需同时改变 RuntimeKernel 生命周期/dispatcher、bridge pending handles、持久 EventStore 和 appui transport（超过 3 个 runtime layers）。

推荐的实现设计（沿已确认迁移方向，不更换模型配置）：

1. Go RuntimeKernel 保留唯一公开 turn admission 与最终状态提交职责；新增 typed session-engine owner，默认 legacy，且由 host 配置仅对新 session 选择 DSH。持久化 owner 字段，拒绝从普通用户 metadata 接收引擎切换。已有 legacy session 不变；engine owner 只允许在无 active/pending turn 时由受控管理路径回切。
2. 抽取共享的 Go turn services：从旧 loop 中形成可供两个 engine 使用的 step preparation、dispatch-result materialization/checkpoint 和 FinalContract finalization 边界。DSH engine 只驱动模型循环；每个可见 tool schema 必须来源于当前冻结的 TurnAssembly/StepContext，Go callback 通过既有 ToolDispatcher 执行，禁止 sdk-minimal shell 或 Harness 原生执行 surface。
3. 将 session.event、sequence cursor 与事件原子提交到生产数据库；pending Go approval 与 DSH JSON-RPC request id/call id/binding 的映射按 session/turn/call 持久保存。Resume 仅在 Go 完成 ActionToken 校验和 Approved dispatch 后回答原 request；Cancel 先取消并排空 Go dispatch、撤销 pending approval，再发精确的 DSH session/cancel。
4. DSH durable events 投影成 TurnSnapshot / AgentItems / checkpoint，transient assistant.stream 只更新现有 AssistantTransport streaming state；queue receipt、idle 和 stream 结束均不可完成业务 turn。完整 durable turn/end 后由共享 Go FinalContract 路径校验并提交。
5. 将实现拆成可审核里程碑：A) RuntimeKernel engine ownership 与 legacy characterization；B) frozen step/dispatcher/finalizer 的共享契约与 production-path fake-peer red/green；C) GORM transactional EventStore 与 pending approval recovery；D) AssistantTransport events/stream replay；E) pinned DSH process launcher、终止/重启和端到端测试。每阶段独立提交，单提交遵守仓库 5 文件 / 500 production-line 预算。

本阶段结论：根据仓库 AGENTS.md 的“跨越三个以上 runtime layers 时先完成接口/契约设计并取得确认”停止条件，本轮先交付以上接口评估，不改 production code。现有已确认方案给出了方向，但尚未确认共享 Go finalizer / per-step dispatch API 这一具体边界；在此边界获接受前，继续拼接会跳过现有 final gate 或把旧 loop 与 DSH 同时跑。下一步应以这份设计为审阅对象确认接口边界，再从 A/B 开始实现。此结论不是 DSH 协议不可兼容；它是当前 RuntimeKernel 没有支持两个 engine 共用相同安全/lifecycle 机制的抽象。

### 10.2 Draft PR #22 当前 CI 结果

- Commit：811a3f37a6d30181bbb24690b010ce8d30b033ec；CI run：38513834983，已完成，aggregate conclusion 为 failure。
- web：通过（unit tests 与 web build 均成功）。
- backend-agent-harness：失败于 agent harness hardening gate 最后的 forbidden residue scan；命中既有 workflow 代码中的 candidateState（例如 workflowInteractionContract.test.ts、workflowWorkspaceSelectors.ts、RunnerCanvas.tsx）。PR diff 只修改本计划文档，没有改这些文件。
- workflow-canvas-pr：失败于 web/tests/workflow-studio.snapshot.spec.js:65；workflow-scheme-c-overlay.png 有 13,790 像素（0.02 比例）不一致。该 spec 和截图基线不在本次 diff，没有更新或绕过快照。
- run 链接：https://github.com/lizhongxuan/aiops-v2/actions/runs/38513834983
