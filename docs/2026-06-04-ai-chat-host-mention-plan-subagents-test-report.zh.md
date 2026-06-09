# AIOps @主机 Plan/Subagents Test Report

日期：2026-06-04

## 覆盖范围

- host mention parser/resolver。
- multi-host mandatory plan gate。
- one child agent per resolved host。
- host binding enforcement。
- host-agent/runner execution boundary。
- host-bound child agent 复用 AI Chat shared runtime runner。
- child spawn/follow-up 进入真实 child turn。
- compact status panel above composer。
- subagent status rows in the same panel。
- subagent drawer independent transcript。
- transport command decode、host mission lifecycle projection、transcript API。
- comprehensive Playwright user flow for LLM config、host inventory、multi-host chat input、plan panel、subagent drawer and approval composer。
- env-gated real PostgreSQL smoke harness。

## 命令结果

| Command | Result |
| --- | --- |
| `go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/runtimekernel ./internal/agentmgr ./internal/integrations/localtools ./cmd/ai-server ./cmd/host-agent` | PASS |
| `go test -count=1 ./internal/runtimekernel -run 'TestAgentConfigRunner'` | PASS |
| `go test -count=1 ./internal/agentmgr -run 'TestRunAgentReturnsErrorWhenRunnerMissing\|TestRunAgent'` | PASS |
| `go test -count=1 ./internal/agentmgr -run 'TestKernelAdapter.*HostChild'` | PASS |
| `go test -count=1 ./internal/integrations/localtools -run 'TestEnsurePostgreSQLInstalled\|TestExecCommandToolRunsReadOnlyCommandViaSelectedHostAgent'` | PASS |
| `go test -count=1 ./internal/appui -run 'TestLocalHostAgentTokenStore\|TestDirectHostAgentInstallerInstallsUbuntuAgentWithScriptedCommands'` | PASS |
| `go test -count=1 ./internal/server -run 'TestGRPC\|TestAgentGRPC'` | PASS |
| `go test -count=1 ./cmd/ai-server -run 'TestHostAgentGRPCAuthenticator\|TestHostAgentCommandRunnerFallsBackToHTTPRun\|TestNewServerAgentRunnerUsesRuntimeKernelRunner'` | PASS |
| `npm run test` | PASS |
| `npm run build` | PASS |
| `npm run test:ui -- e2e/host-ops-comprehensive-user-flow.spec.js --project=chromium` | PASS |
| `PLAYWRIGHT_SKIP_WEB_SERVER=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:18080 npm run test:ui -- e2e/host-ops-status-panel.spec.js --project=chromium` | PASS |
| `PLAYWRIGHT_SKIP_WEB_SERVER=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:18080 npm run test:ui -- e2e/host-ops-real-pg.spec.js --project=chromium` | SKIPPED by design: `1 skipped` because real/isolated smoke env flags were not enabled |
| `npm run test:ui:snapshots` | 8/9 PASS；`context compaction and externalized evidence states` 为实施前已存在失败 |

说明：真实 PostgreSQL smoke 必须显式设置 `AIOPS_REAL_HOST_OPS_SMOKE=1`、`AIOPS_REAL_HOST_OPS_ISOLATED=1` 和 `AIOPS_TEST_*` 环境变量后才会访问 live host。测试源码不保存 LLM API key、SSH 密码或 host-agent token。启用后应使用隔离 ai-server 数据目录；测试会删除本次新建 host，但 LLM config 仍是目标 ai-server 的全局配置。

## Browser-in-app 验证

- 入口：`http://127.0.0.1:18080/`。
- 验证内容：
  - 页面标题为 `AIOps Codex MVP`。
  - `omnibar-input` 可见。
  - `omnibar-primary-action` 可见。
  - 当前页面没有活跃 hostops mission，因此 `host-ops-status-panel` 计数为 0。
- 另通过 Playwright fixture 验证：
  - 输入多主机请求后，输入框上方显示 Codex 风格紧凑状态面板。
  - 计划区域显示 `共 5 个任务，已经完成 0 个`。
  - 同一面板底部显示后台智能体状态行。
  - 点击子 Agent 行可以打开右侧 drawer。
  - drawer 显示选中子 Agent 的独立 transcript，例如 `检查PG版本` 与 `PostgreSQL 15 已检测到`。
- 另通过 browser-in-app fixture 验证：
  - 入口：`http://127.0.0.1:18080/?fixture=host-ops-three-hosts`。
  - 面板显示 `计划共 5 个任务，已经完成 0 个` 和 `3 个后台智能体`。
  - 点击 `host-subagent-status-row-child-1` 后，`host-subagent-drawer` 打开。
  - drawer transcript 包含 `检查PG版本` 和 `PostgreSQL 15 已检测到`。
  - 截图保存到 `/tmp/aiops-hostops-comprehensive-browser-in-app-20260604.png`。

## @主机模糊搜索验证

- Vitest PASS：`src/chat/hostMentionSearch.test.ts`、`HostMentionSuggestionPopover.test.tsx`、`AiopsComposer.test.tsx`。
- Focused regression PASS：`src/chat/hostMentions.test.ts`、`HostMentionComposer.test.tsx`、`HostOpsStatusPanel.test.tsx`、`HostSubagentDrawer.test.tsx`。
- Playwright PASS：`e2e/host-mention-fuzzy-search.spec.js --project=chromium`。
- Existing HostOps Playwright PASS：`PLAYWRIGHT_SKIP_WEB_SERVER=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:18080 npm run test:ui -- e2e/host-ops-status-panel.spec.js --project=chromium`。
- Browser-in-app PASS：输入 `@` 后显示紧凑 command-menu；继续输入后按 name/IP 过滤；当前真实清单只有 `server-local`，候选数为 1 且满足最多 10 条；点击候选可插入 mention。
- Browser-in-app 还验证了 `@120.77` 不会被 `.` 截断 active token；当前真实清单没有该 IP 时显示紧凑空态。
- Screenshot：`/tmp/aiops-host-mention-fuzzy-browser-in-app-20260604.png`。
- 搜索字段限制：只匹配 host `name` 和 `ip/address`；不匹配 hostname、id/hostId、sshUser、labels、status/installState/controlMode。
- 敏感信息扫描：源码、测试、文档和截图未发现本次 live credential 片段；本地 `.data` 运行态配置中存在既有凭据文件，已由全局 gitignore 忽略，未纳入交付面。

### 2026-06-04 复验

- Vitest focused PASS：7 files / 24 tests。
- Playwright fuzzy search PASS：`e2e/host-mention-fuzzy-search.spec.js --project=chromium`，1 test passed。
- Existing HostOps Playwright PASS：`e2e/host-ops-status-panel.spec.js --project=chromium`，1 test passed。
- Build PASS：`npm run build`，仅保留既有 chunk-size warning。
- Browser-in-app PASS：重启本地 `ai-server` 后在 `http://localhost:18080/` 创建会话，使用可视层逐键输入验证：
  - 输入 `@` 后显示 `host-mention-suggestion-popover`。
  - 输入 `@server` 后候选为 `server-local`，候选数 `1 <= 10`。
  - 按 Enter 插入 `@server-local ` 并关闭候选。
  - 输入 `@120.77` 时 `.` 不截断 active token，当前清单无匹配时显示紧凑空态。
- Browser-in-app screenshot：`/tmp/aiops-host-mention-fuzzy-browser-in-app-rerun-20260604.png`。
- Deliverable secret scan PASS：`web/src`、`web/tests`、`docs` 和复验截图没有 live credential 片段。

## 当前闭环状态

- 已补齐 production `AgentManager` 的 shared runtime runner：`agentmgr.AgentManager` 通过 `runtimekernel.AgentConfigRunner` 运行 host-bound child agent，不再是 nil runner。
- 已补齐 child spawn/follow-up 进入真实 child turn：spawn 后异步运行，drawer follow-up 进入同一 child session。
- 已补齐 host-agent HTTP `/run` fallback 和本地 host-agent token secret resolver。
- 已补齐 gRPC host-agent 注册认证：生产 `GRPCServer` 在接受 executable stream 前验证 register payload token 与 host `AgentTokenRef` 匹配，错误 token 不会进入 connected hosts。
- 已新增 `ensure_postgresql_installed` host tool：
  - 先执行 `psql --version`。
  - 已安装时跳过重装并返回版本。
  - 未安装时要求审批，再通过绑定 host-agent 执行包安装、服务启动和版本检查。
  - 安装脚本要求 root 或 passwordless sudo；systemd 可用时服务启动/active/readiness 失败会返回失败。
- 已新增 env-gated Playwright live smoke，且加强为隔离运行门禁、结束清理新建 host、断言 child 完成和 transcript tool events；本报告仍未声明 live host PostgreSQL 安装 PASS。
- 已新增综合 Playwright 用户流程 `web/tests/e2e/host-ops-comprehensive-user-flow.spec.js`，覆盖 LLM 配置、主机清单、多主机 `@host` 输入、transport metadata、计划/子 Agent 面板、drawer transcript 和审批 composer；测试只使用 fixture/占位密钥。

## 残余风险

- 真实 “起一个子 agent 安装 PG” 尚未在 live host 上跑到 PASS；当前只验证了脚本默认 skip 和安全工具链。
- child turn 中 package install 触发 approval 后的 resume 路径仍需补齐；否则缺少 PostgreSQL 时 child 可能被记录为 blocked/failed，而不是在用户审批后继续执行。
- gRPC host-agent 已做 token 认证，但当前 host-agent gRPC transport 仍使用 insecure credentials；跨不可信网络部署时仍需要 TLS/mTLS 或等价通道保护。
- 真实 host-agent 网络抖动仍需灰度观察。
- PostgreSQL 主从/监控角色自动分配仍需用户确认，不应在单主机 smoke 中隐式做拓扑配置。
- 高风险操作的真实审批链路需要在接入 live host 后继续做灰度回归。
- `npm run test:ui:snapshots` 的剩余失败是 baseline 已记录问题，本次 hostops 实施没有更新该 snapshot。
