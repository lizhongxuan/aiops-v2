# AIOps @主机 Plan/Subagents Test Report

日期：2026-06-04

## 覆盖范围

- host mention parser/resolver。
- multi-host mandatory plan gate。
- one child agent per resolved host。
- host binding enforcement。
- host-agent/runner execution boundary。
- compact status panel above composer。
- subagent status rows in the same panel。
- subagent drawer independent transcript。
- transport command decode、host mission lifecycle projection、transcript API。

## 命令结果

| Command | Result |
| --- | --- |
| `go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/agentmgr` | PASS |
| `go test -count=1 ./internal/hostops ./internal/appui ./internal/server ./internal/runtimekernel ./internal/planning ./internal/agentmgr ./cmd/host-agent` | PASS |
| `(cd pkg/runner && go test -count=1 ./scheduler)` | PASS |
| `go test -count=1 ./internal/policyengine -run 'TestGatewayPolicyApprovalPaths\|TestExecuteModePolicy_MutationNeedsApproval\|TestChatModeRequiresApprovalForUnsafeTerminalCommand'` | PASS |
| `npm run test` | PASS |
| `npm run build` | PASS |
| `npm run test:ui -- e2e/host-ops-status-panel.spec.js --project=chromium` | PASS |
| `npm run test:ui:snapshots` | 8/9 PASS；`context compaction and externalized evidence states` 为实施前已存在失败 |

说明：`pkg/runner` 是独立 Go module，最终验证从 `pkg/runner` 目录执行 `go test -count=1 ./scheduler`，没有使用根 module 下无效的 `./pkg/runner/scheduler` import path。

## Browser-in-app 验证

- 入口：`http://127.0.0.1:53173/?fixture=host-ops-three-hosts`。
- 截图：`/tmp/aiops-hostops-browser-in-app-20260604.png`。
- 验证内容：
  - 输入框上方显示 Codex 风格紧凑状态面板。
  - 计划区域显示 `共 5 个任务，已经完成 0 个`。
  - 同一面板底部显示 `3 个后台智能体`。
  - 点击子 Agent 行可以打开右侧 drawer。
  - drawer 显示选中子 Agent 的独立 transcript，例如 `检查PG版本` 与 `PostgreSQL 15 已检测到`。

## 残余风险

- 真实 host-agent 网络抖动仍需灰度观察。
- PostgreSQL 角色自动分配需要用户确认。
- 高风险操作的真实审批链路需要在接入真实 host-agent 后继续做灰度回归。
- `npm run test:ui:snapshots` 的剩余失败是 baseline 已记录问题，本次 hostops 实施没有更新该 snapshot。
