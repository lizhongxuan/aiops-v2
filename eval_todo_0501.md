# eval_todo_0501.md

日期：2026-05-01

目标：把 Phoenix trace UI、本地 model input trace、`cmd/agent-eval`、可选 `pkg/runner` workflow 整成一套本地 agent 调试闭环。

核心结论：

- 第一版只做 MVP：`agent.turn`、`model_call`、`tool_call`、trace 文件路径、`cmd/agent-eval -agent server`、一份 runner smoke workflow。
- Phoenix 只负责看时间线，不保存完整 prompt。
- `.data/model-input-traces` 继续作为完整 prompt 和 `input.diff.md` 的真相源。
- `cmd/agent-eval` 负责回归，不迁移到 Phoenix。
- `pkg/runner` 只跑短 runbook 命令，不托管 Phoenix 或 aiops server。

---

## 0. MVP 边界

### 第一版必须完成

- [x] Phoenix 本地可打开：`http://localhost:6006`。
- [x] `AIOPS_OTEL_ENABLED=1` 时 aiops-v2 能向 Phoenix 发送 trace。
- [x] 默认不开 OTel，不影响现有测试和开发。
- [x] Phoenix 未启动或 endpoint 不通时，agent 仍能启动和执行。
- [x] Phoenix 能看到 `agent.turn` root span。
- [x] Phoenix 能看到 `model_call` span。
- [x] Phoenix 能看到 `tool_call.<tool_name>` span。
- [x] 每个 `model_call` 能定位到 `.data/model-input-traces/<session>/<turn>/iteration-*.md`。
- [x] 非首轮且存在相邻模型输入 diff 时，`model_call` 能定位到 `.data/model-input-traces/<session>/<turn>/input.diff.md`。
- [x] `cmd/agent-eval -agent server` 能调用本地 aiops server。
- [x] 真实 turn 输出能转换成现有 `eval.RunOutput`。
- [x] 能输出 `answer.txt`、`agent_events.json`、`tool_calls.json`、`turn_items.json`、`report.json`、`report.md`。
- [x] 有一个 runner workflow 能跑短 smoke 和 mock eval。

### 第一版明确不做

- [x] 不做项目内 Debug Workbench 页面。
- [x] 不引入 Langfuse、ClickHouse、长周期 trace 平台。
- [x] 不把 Phoenix 作为生产依赖。
- [x] 不默认把完整 prompt 写入 span attribute。
- [x] 不自动生成 eval case。
- [x] 不做自动根因诊断。
- [x] 不把 runner 接入 agent 主链路。
- [x] 不让 runner 托管 Phoenix 或 aiops server 长驻进程。

---

## 1. 文件总清单

### 新增文件

- [x] `internal/runtimekernel/observer.go`
- [x] `internal/runtimekernel/observer_test.go`
- [x] `internal/runtimekernel/observer_dispatch_test.go`
- [x] `internal/observability/config.go`
- [x] `internal/observability/config_test.go`
- [x] `internal/observability/otel.go`
- [x] `internal/observability/otel_test.go`
- [x] `internal/observability/runtime_observer.go`
- [x] `internal/observability/runtime_observer_test.go`
- [x] `internal/eval/server_agent.go`
- [x] `internal/eval/server_agent_test.go`
- [x] `internal/eval/root_cause_test.go`
- [x] `scripts/phoenix-smoke.sh`
- [x] `pkg/runner/examples/aiops-phoenix-agent-debug-smoke.yaml`

### 修改文件

- [x] `cmd/ai-server/main.go`
- [x] `internal/runtimekernel/eino_kernel.go`
- [x] `internal/runtimekernel/types.go`
- [x] `internal/runtimekernel/dispatch.go`
- [x] `internal/modeltrace/trace.go`
- [x] `cmd/agent-eval/main.go`
- [x] `internal/eval/types.go`
- [x] `internal/eval/loader.go`
- [x] `internal/eval/scorer.go`
- [x] `internal/eval/markdown.go`
- [x] `go.mod`
- [x] `go.sum`

---

## 2. Trace 字段规则

### 默认允许写入 Phoenix

- [x] `service.name`
- [x] `session.id`
- [x] `turn.id`
- [x] `client_turn_id`
- [x] `client_message_id`
- [x] `session_type`
- [x] `mode`
- [x] `host_id`
- [x] `turn.status`
- [x] `model.name`
- [x] `prompt.stable_hash`
- [x] `visible_tools`
- [x] `input.message_count`
- [x] `output.has_tool_calls`
- [x] `output.tool_call_count`
- [x] `trace.file`
- [x] `trace.diff`（仅有真实 diff 路径时写入）
- [x] `tool.name`
- [x] `tool.call_id`
- [x] `tool.risk`
- [x] `tool.outcome`
- [x] `tool.result_bytes`
- [x] `tool.result_truncated`
- [x] `tool.raw_ref`
- [x] `error`

### 默认禁止写入 Phoenix

- [x] 完整 prompt
- [x] 完整 tool output
- [x] 文件内容全文
- [x] 命令输出全文
- [x] 环境变量全文
- [x] token、secret、cookie、credential
- [x] 其他敏感上下文

---

## 3. Task 1 - Runtime Observer 接口

目标：让 `runtimekernel` 定义自己的观测接口，不直接依赖 OpenTelemetry/Phoenix。

文件：

- [x] 创建 `internal/runtimekernel/observer.go`
- [x] 创建 `internal/runtimekernel/observer_test.go`

实现内容：

- [x] 定义 `Observer` 接口：
  - [x] `ContextWithTraceContext`
  - [x] `StartTurn`
  - [x] `StartStage`
  - [x] `StartModelCall`
  - [x] `StartToolCall`
- [x] 定义 `ObservedSpan` 接口：
  - [x] `TraceContext`
  - [x] `SetAttributes`
  - [x] `SetStatus`
  - [x] `End`
- [x] 定义 `TraceContextCarrier`，用于 suspended turn 的 vendor-neutral trace context。
- [x] 定义 attr structs：
  - [x] `TurnSpanAttrs`
  - [x] `StageSpanAttrs`
  - [x] `ModelCallSpanAttrs`
  - [x] `ToolCallSpanAttrs`
- [x] 实现 `NoopObserver`
- [x] nil context 时 fallback 到 `context.Background()`

测试：

```bash
go test ./internal/runtimekernel -run 'TestNoopObserver|TestModelSpanAttrs' -count=1
```

验收：

- [x] `NoopObserver` 不 panic。
- [x] `ModelCallSpanAttrs` 能携带 `TraceFile` 和 `TraceDiffFile`。
- [x] `NoopObserver` 恢复 trace context 时仍为 no-op。
- [x] `runtimekernel` 没有 import `internal/observability`。

---

## 4. Task 2 - Observability Config

目标：通过环境变量 opt-in 打开 OTel。

文件：

- [x] 创建 `internal/observability/config.go`
- [x] 创建 `internal/observability/config_test.go`

环境变量：

- [x] `AIOPS_OTEL_ENABLED`
- [x] `AIOPS_OTEL_ENDPOINT`
- [x] `AIOPS_OTEL_SERVICE_NAME`
- [x] `AIOPS_OTEL_PROJECT`
- [x] `AIOPS_OTEL_INCLUDE_PROMPT`

默认值：

- [x] `Enabled=false`
- [x] `Endpoint=http://localhost:6006/v1/traces`
- [x] `ServiceName=aiops-v2-agent`
- [x] `IncludePrompt=false`

依赖：

```bash
go get go.opentelemetry.io/otel@latest
go get go.opentelemetry.io/otel/sdk@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest
```

测试：

```bash
go test ./internal/observability -run TestConfigFromEnv -count=1
```

验收：

- [x] 默认关闭。
- [x] 开启配置能正确读取 endpoint/service/project。
- [x] `AIOPS_OTEL_INCLUDE_PROMPT` 默认关闭。

---

## 5. Task 3 - OTLP Exporter 初始化

目标：初始化 OpenTelemetry HTTP exporter，Phoenix 不可用时不破坏 agent 主流程。

文件：

- [x] 创建 `internal/observability/otel.go`
- [x] 创建 `internal/observability/otel_test.go`

实现：

- [x] `Init(ctx, Config) (*Provider, error)`
- [x] `Provider.Enabled() bool`
- [x] `Provider.Tracer() trace.Tracer`
- [x] `Provider.Shutdown(ctx) error`
- [x] disabled config 返回 no-op provider。
- [x] enabled 但 endpoint 为空时返回清晰错误。
- [x] endpoint 不可达时不在 init 阶段同步 dial 失败。

强制测试：

- [x] `TestInitDisabledReturnsNoopProvider`
- [x] `TestInitEnabledWithEmptyEndpointFailsClearly`
- [x] `TestInitEnabledWithUnreachableEndpointStillReturnsProvider`

测试：

```bash
go test ./internal/observability -run TestInit -count=1
```

验收：

- [x] Phoenix 没启动时，`Init` 不应直接导致 agent 无法启动。
- [x] shutdown 有 timeout，不能卡死退出。

---

## 6. Task 4 - RuntimeObserver Adapter

目标：把 `runtimekernel.Observer` 适配到 OpenTelemetry span。

文件：

- [x] 创建 `internal/observability/runtime_observer.go`
- [x] 创建 `internal/observability/runtime_observer_test.go`

实现：

- [x] `NewRuntimeObserver(tracer trace.Tracer, cfg Config) RuntimeObserver`
- [x] `StartTurn` 生成 `agent.turn`
- [x] `StartModelCall` 生成 `model_call`
- [x] `StartToolCall` 生成 `tool_call.<tool_name>`
- [x] `StartStage` 先预留，第一版可少量使用。
- [x] `SetStatus("failed", msg)` 映射 `codes.Error`
- [x] 非失败映射 `codes.Ok`
- [x] map attrs 支持 string/bool/int/int64/float64 fallback。
- [x] 支持 W3C `traceparent` inject/extract，approval resume 可继承原 turn trace。
- [x] `TraceDiffFile` 为空时不写 `trace.diff` attribute。

安全测试：

- [x] `TestRuntimeObserverDoesNotRecordFullPromptByDefault`
- [x] 测试包含敏感字符串的 `TurnSpanAttrs.Input` 不进入 span attribute。

测试：

```bash
go test ./internal/observability -run TestRuntimeObserver -count=1
```

验收：

- [x] span 名称正确。
- [x] model span 是 turn span 的 child。
- [x] 默认不记录完整 prompt。
- [x] 能导出并恢复 turn trace context。
- [x] `observability` 可以 import `runtimekernel`。
- [x] `runtimekernel` 不能 import `observability`。

---

## 7. Task 5 - 接入 ai-server

目标：ai-server 启动时初始化 observability，并注入 runtime kernel。

文件：

- [x] 修改 `cmd/ai-server/main.go`
- [x] 修改 `internal/runtimekernel/eino_kernel.go`

实现：

- [x] `EinoKernelConfig` 增加 `Observer Observer`
- [x] `EinoKernel` 保存 `observer Observer`
- [x] `NewEinoKernel` 中 observer nil 时用 `NoopObserver{}`
- [x] `cmd/ai-server/main.go` 读取 `observability.ConfigFromEnv(os.Getenv)`
- [x] `observability.Init` 失败时记录日志并 fallback no-op provider。
- [x] 进程退出时 `Shutdown` flush。
- [x] `kernelCfg` 注入 `Observer: runtimeObserver`

测试：

```bash
go test ./internal/observability ./internal/runtimekernel ./cmd/ai-server -run 'TestConfigFromEnv|TestInit|TestEinoKernelConfigAcceptsObserver' -count=1
```

验收：

- [x] OTel 默认关闭时现有行为不变。
- [x] `AIOPS_OTEL_ENABLED=1` 时启用 observer。
- [x] endpoint 错误时 ai-server 不应直接崩。

---

## 8. Task 6 - Turn / Model Span

目标：每次 agent turn 和每次模型调用在 Phoenix 中可见。

文件：

- [x] 修改 `internal/runtimekernel/eino_kernel.go`
- [x] 修改或新增 runtimekernel 测试

实现：

- [x] turn 开始时创建 `agent.turn`
- [x] turn 完成时设置 `turn.status=completed`
- [x] turn 失败时设置 `turn.status=failed` 和 `error`
- [x] 每次模型调用创建 `model_call`
- [x] `model_call` attrs 包含：
  - [x] `session.id`
  - [x] `turn.id`
  - [x] `iteration`
  - [x] `model.name`
  - [x] `prompt.stable_hash`
  - [x] `visible_tools`
  - [x] `input.message_count`
  - [x] `trace.file`
  - [x] `trace.diff`
  - [x] `output.has_tool_calls`
  - [x] `output.tool_call_count`
- [x] 继续调用现有 `writeModelInputDebugTrace`
- [x] 从 trace path 推出 `input.diff.md` 路径。
- [x] `RunTurn` 创建 `agent.turn` 后把 trace context 保存到 `TurnSnapshot.TraceContext`。
- [x] `ResumeTurn` 从 `TurnSnapshot.TraceContext` 恢复 context 后再继续审批工具和后续模型调用。
- [x] approval resume 后不新增第二套 tool loop，仍走 `ResumeTurn -> ToolDispatcher -> runHostIterationLoop`。

测试：

```bash
go test ./internal/runtimekernel -run 'TestRunTurnObserverRecordsTurnAndModelCall|TestResumeTurnRestoresObservedTurnTraceContext|TestRunTurn_CreatesRootSpanForEachTurn|TestRunTurn_EmitsIterationStageActivityUpdates' -count=1
```

验收：

- [x] Phoenix 中能看见 turn -> model_call 层级。
- [x] model span 有 trace 文件路径。
- [x] approval resume 后的 tool/model span 能继承原 turn trace context。
- [x] 默认不开 debug trace 时不应报错。

---

## 9. Task 7 - Tool Span

目标：每个工具调用在 Phoenix 中可见，并能区分成功、失败、阻断。

文件：

- [x] 修改 `internal/runtimekernel/dispatch.go`
- [x] 修改 `internal/runtimekernel/eino_kernel.go`
- [x] 创建 `internal/runtimekernel/observer_dispatch_test.go`

实现：

- [x] `ToolDispatcher` 增加 `observer Observer`
- [x] 增加 `WithObserver(observer Observer) *ToolDispatcher`
- [x] 默认 `NoopObserver{}`
- [x] lookup 成功后创建 `tool_call.<tool_name>`
- [x] span attrs 包含：
  - [x] `tool.name`
  - [x] `tool.call_id`
  - [x] `tool.risk`
  - [x] `tool.outcome`
  - [x] `tool.result_bytes`
  - [x] `tool.result_truncated`
  - [x] `tool.raw_ref`
  - [x] `error`
- [x] 成功时 status completed。
- [x] tool failed 时 status failed。
- [x] approval/evidence 阻断时 outcome 分别标记。
- [x] `newIterationDispatcher` 注入 `k.observer`。

测试：

```bash
go test ./internal/runtimekernel -run 'TestToolDispatcherObserverRecordsToolOutcome|TestDispatch' -count=1
```

验收：

- [x] Phoenix 中能看到 tool child span。
- [x] 工具失败能直接从 span 看出来。
- [x] 工具大输出只记录 bytes/raw ref，不记录全文。

---

## 10. Task 8 - Phoenix Smoke

目标：提供最小本地验证脚本。

文件：

- [x] 创建 `scripts/phoenix-smoke.sh`

脚本要求：

- [x] 设置 `AIOPS_OTEL_ENABLED=1`
- [x] 设置 `AIOPS_OTEL_ENDPOINT=http://localhost:6006/v1/traces`
- [x] 设置 `AIOPS_DEBUG_MODEL_INPUT_TRACE=1`
- [x] 跑 observability/runtimekernel focused tests。
- [x] 输出手动 E2E 检查步骤。

测试：

```bash
chmod +x scripts/phoenix-smoke.sh
./scripts/phoenix-smoke.sh
```

验收：

- [x] 脚本能跑通 focused tests。
- [x] 脚本提示启动 Phoenix 和 aiops server。
- [x] 不要求脚本托管长驻服务。

---

## 11. Task 9 - agent-eval Server Adapter

目标：`cmd/agent-eval` 能测试真实本地 agent 行为。

文件：

- [x] 修改 `cmd/agent-eval/main.go`
- [x] 创建 `internal/eval/server_agent.go`
- [x] 创建 `internal/eval/server_agent_test.go`

CLI：

- [x] 增加 `-agent server`
- [x] 增加 `-server-url`
- [x] 增加 `-poll-timeout`
- [x] 增加 `-poll-interval`

ServerAgent 行为：

- [x] POST `/api/v1/chat/message`
- [x] body 包含 case input。
- [x] metadata 包含 `eval.caseId`
- [x] 解析 `sessionId` 和 `turnId`
- [x] 轮询 GET `/api/v1/state`
- [x] 等待 turn 非 active。
- [x] 抽取 assistant answer。
- [x] 抽取 tool calls。
- [x] 抽取 turn items。
- [x] 抽取 agent events。
- [x] 转成 `eval.RunOutput`
- [x] 复用现有 scorer/report/baseline comparison。

测试：

```bash
go test ./internal/eval ./cmd/agent-eval -run 'TestServerAgent|TestRunCLI' -count=1
```

验收：

- [x] `-agent mock` 仍然可用。
- [x] `-agent server` 能驱动本地 aiops server。
- [x] 无真实模型配置时，也应输出明确失败报告，而不是静默失败。

---

## 12. Task 10 - Eval Case 质量规范

目标：避免 eval “能跑但没判断力”。

每个真实失败 case 至少包含或明确配置：

- [x] `id`
- [x] `category`
- [x] `rootCauseCategory`
- [x] `input`
- [x] `expected.mustInclude`
- [x] `expected.mustNotInclude`
- [x] `expected.expectedToolCalls`
- [x] `expected.mustHavePlan`
- [x] `expected.maxIterations`
- [x] `expected.maxToolCalls`

case 质量要求：

- [x] `input` 保持最小复现。
- [x] `expectedToolCalls` 只写真正必要工具。
- [x] `mustInclude` 检查最终用户价值，不检查无关措辞。
- [x] `mustNotInclude` 覆盖真实错误模式。
- [x] 每个 case 标注根因类别，字段为 `rootCauseCategory`，允许值：
  - [x] `prompt`
  - [x] `tool`
  - [x] `policy`
  - [x] `context`
  - [x] `model`
  - [x] `completion_gate`

实现说明：

- [x] `Case.RootCauseCategory` 已加入 schema。
- [x] `LoadCases` 会校验 `rootCauseCategory` 枚举值；为空时兼容旧 case。
- [x] `CaseScore` 和 `report.json` 会透传 `rootCauseCategory`。
- [x] `report.md` 会显示 `rootCause=<category>`。
- [x] `ServerAgent` 会把 `eval.rootCauseCategory` 写入 chat metadata。

示例：

```json
{
  "id": "agent-debug-example",
  "category": "agent-debug",
  "rootCauseCategory": "prompt",
  "input": "触发失败的用户输入",
  "expected": {
    "mustInclude": ["必须出现的关键词"],
    "mustNotInclude": ["不应出现的错误内容"],
    "expectedToolCalls": ["read_file"],
    "mustHavePlan": true,
    "maxIterations": 4,
    "maxToolCalls": 6
  }
}
```

验收：

- [x] 每次真实失败修复前先补 case。
- [x] baseline/current 可以识别退化。
- [x] eval 分数只作为回归信号，不代替人工判断。

---

## 13. Task 11 - Runner Smoke Workflow

目标：用现有 `pkg/runner` 跑短 runbook，不新增平台能力。

文件：

- [x] 创建 `pkg/runner/examples/aiops-phoenix-agent-debug-smoke.yaml`

workflow 步骤：

- [x] 检查 aiops repo root。
- [x] 跑 focused package tests。
- [x] 跑 mock eval regression。
- [x] 检查 Phoenix UI 是否可达。
- [x] 打印人工 trace 检查步骤。

运行：

```bash
cd pkg/runner
go run ./examples/runner-simple ./examples/aiops-phoenix-agent-debug-smoke.yaml
```

验收：

- [x] 输出 `workflow applied`。
- [x] 如果 Phoenix 未启动，只提示手动启动，不阻断后续人工说明。
- [x] runner 不启动 Phoenix 容器。
- [x] runner 不启动 aiops server 长驻进程。
- [x] runner 不进入 agent 主链路。

---

## 14. Task 12 - 本地调试 Runbook

目标：最终只看这份清单和 runbook 就能调试 agent。

文件：

- [x] 不新增单独 runbook 文档，避免调试文档继续分散。
- [x] 核心流程保留在本文件。

runbook 必须包含：

- [x] 启动 Phoenix 命令。
- [x] 启动 aiops-v2 命令。
- [x] 复现失败 turn 的记录字段。
- [x] Phoenix 首查顺序。
- [x] 如何打开 `.data/model-input-traces`。
- [x] 如何固化 eval case。
- [x] 如何跑 baseline/current。
- [x] 如何使用 runner workflow。
- [x] 修复纪律：每次只改一个维度。
- [x] 常见误区。

验收：

- [x] 新同事按 runbook 能完成一次失败定位。
- [x] 不需要先读 Phoenix 设计文档。
- [x] 不需要先读 implementation plan。

---

## 15. 最终 E2E 验证

### 单元测试

```bash
go test ./internal/observability ./internal/runtimekernel ./internal/eval ./cmd/agent-eval -count=1
```

预期：

```text
ok  	aiops-v2/internal/observability
ok  	aiops-v2/internal/runtimekernel
ok  	aiops-v2/internal/eval
ok  	aiops-v2/cmd/agent-eval
```

2026-05-01 最新实测：

```text
ok  	aiops-v2/internal/observability
ok  	aiops-v2/internal/runtimekernel
ok  	aiops-v2/internal/eval
ok  	aiops-v2/cmd/agent-eval
ok  	aiops-v2/cmd/ai-server
```

### Mock eval

```bash
go run ./cmd/agent-eval \
  -agent mock \
  -cases testdata/eval_cases \
  -out .data/eval_runs/phoenix-plan-mock
```

预期：

- [x] 输出 summary。
- [x] 生成 `.data/eval_runs/phoenix-plan-mock/report.json`
- [x] 生成 `.data/eval_runs/phoenix-plan-mock/report.md`

### 手动 Phoenix trace

Terminal 1:

```bash
docker run --rm \
  -p 6006:6006 \
  -p 4317:4317 \
  arizephoenix/phoenix:latest
```

Terminal 2:

```bash
export AIOPS_DEBUG_MODEL_INPUT_TRACE=1
export AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR=.data/model-input-traces
export AIOPS_OTEL_ENABLED=1
export AIOPS_OTEL_ENDPOINT=http://localhost:6006/v1/traces
export AIOPS_OTEL_SERVICE_NAME=aiops-v2-agent
AIOPS_HTTP_ADDR=:18080 ./scripts/start.sh
```

浏览器：

```text
http://localhost:6006
```

预期：

- [x] 跑一次 chat turn 后 Phoenix 显示 trace。
- [x] trace 中有 `agent.turn`。
- [x] trace 中有 `model_call`。
- [x] 如果发生工具调用，Phoenix 中有 `tool_call.<tool_name>`。
- [x] `model_call` attrs 中有 `trace.file`。
- [x] `model_call` attrs 中有 `trace.diff`：仅当存在相邻模型输入 diff 时写入；首轮 model_call 不写，审批恢复后第二次 model_call 已能写入。
- [x] 本地对应 `trace.file` 文件存在。

2026-05-01 实测记录：

- Phoenix 使用本地 `uvx --from arize-phoenix phoenix serve` 启动，UI 为 `http://127.0.0.1:6006`，Docker Hub 拉取镜像时出现 EOF，未使用 Docker。
- aiops server 使用 `AIOPS_HTTP_ADDR=:18080`、`AIOPS_OTEL_ENABLED=1`、`AIOPS_DEBUG_MODEL_INPUT_TRACE=1` 启动。
- 完成 turn：`turn-1777569998850113000`，session `sess-phoenix-execute-focused`，状态 `completed`。
- Phoenix UI 已看到 `agent.turn`、`model_call`、`tool_call.exec_command`；早期实测中经过审批的 tool turn 会在 Phoenix 中拆成多个 trace，已在后续实现中修复。
- 完成 server eval smoke turn：`turn-1777570655522115000`，session `eval-server-smoke-20260501T013735-simple-chat-no-plan`，Phoenix 中 `agent.turn` 和 `model_call` 均为 `OK`。
- `trace.file` 示例：`.data/model-input-traces/eval-server-smoke-20260501T013735-simple-chat-no-plan/turn-1777570655522115000/iteration-000-20260430T173735.918427000Z.md`。
- 首轮 model_call 没有生成 `input.diff.md`，修复后不再写虚假的 `trace.diff` attribute。

2026-05-01 最新实测记录：

- 完成 approval resume turn：session `resume-trace-diff-20260430T175849Z`，turn `turn-1777571929315817000`，状态 `completed`。
- Phoenix DB 中同一个 trace `20fce60fc94a85cd6ff2b34b913f8cf1` 下包含：
  - `agent.turn`
  - 首轮 `model_call`
  - approval blocked 的 `tool_call.exec_command`
  - approval approved 后的 `tool_call.exec_command`
  - 第二轮 `model_call`
- approval resume 后的 tool/model span parent 均指向原 `agent.turn` span。
- 第二轮 `model_call` 已写入 `trace.diff`：`.data/model-input-traces/resume-trace-diff-20260430T175849Z/turn-1777571929315817000/input.diff.md`。
- 本地 trace 文件已生成：
  - `iteration-000-20260430T175849.663623000Z.md`
  - `iteration-001-20260430T175900.554224000Z.md`
  - `iteration-001-20260430T175900.554224000Z.diff.md`

### Server eval

aiops server 运行时执行：

```bash
go run ./cmd/agent-eval \
  -agent server \
  -server-url http://127.0.0.1:18080 \
  -cases testdata/eval_cases \
  -out .data/eval_runs/phoenix-plan-server \
  -poll-timeout 90s
```

预期：

- [x] 生成 `.data/eval_runs/phoenix-plan-server-smoke/report.json`
- [x] 生成 `.data/eval_runs/phoenix-plan-server-smoke/report.md`
- [x] smoke case 有 answer/tool_calls/turn_items/agent_events artifacts。

2026-05-01 实测记录：

- 命令：`go run ./cmd/agent-eval -agent server -server-url http://127.0.0.1:18080 -cases <单 case 临时目录> -out .data/eval_runs/phoenix-plan-server-smoke -run-id server-smoke-20260501T013735 -poll-timeout 90s -poll-interval 1s`
- 结果：命令退出码 0，成功生成 artifacts。
- score：`0/1 passed, avg score 0.82`。失败原因是该 case 期望最终答案不要包含 `update_plan` 且要精确包含 `不强制 plan`、提到 `internal/promptcompiler/developer_rules.go`；真实答案表达了规则和验证方式，但没有完全满足这些字符串约束。
- 修复项：`ServerAgent` 默认 session/clientTurn 已加 run 标识，避免重复跑同一 case 时复用历史会话；轮询也会校验 session/turn/clientTurn，避免把旧 UI 状态误判为当前 case。
- 全量 `testdata/eval_cases` 尚未在真实模型上跑完；原因是 24 个 case 可能触发审批等待和真实模型成本，建议确认 case 质量后再跑全量。
- 最新复测命令：`go run ./cmd/agent-eval -agent server -server-url http://127.0.0.1:18080 -cases <单 case 临时目录> -out .data/eval_runs/phoenix-plan-server-smoke-latest -run-id server-smoke-20260430T175927Z -poll-timeout 90s -poll-interval 1s`
- 最新复测结果：命令退出码 0，生成 `report.json`、`report.md`、`answer.txt`、`agent_events.json`、`tool_calls.json`、`turn_items.json`。
- 最新复测 score：`0/1 passed, avg score 0.82`，仍是 case 字符串期望不完全匹配；server E2E 和 artifacts 正常。
- Phoenix DB 中最新 server eval trace 为 `c17c6b1bc073f334a3cc0e49995e0b3a`，包含 `agent.turn` 和 `model_call`，状态均为 `OK`。

### Runner smoke

```bash
cd pkg/runner
go test ./workflow ./examples/runner-simple -count=1
go run ./examples/runner-simple ./examples/aiops-phoenix-agent-debug-smoke.yaml
```

预期：

- [x] 输出 `workflow applied`。
- [x] 生成 `.data/eval_runs/runner-phoenix-debug-mock/report.md`。

2026-05-01 最新实测记录：

- `go test ./workflow ./examples/runner-simple -count=1` 通过。
- `go run ./examples/runner-simple ./examples/aiops-phoenix-agent-debug-smoke.yaml` 输出 `workflow applied`。
- `.data/eval_runs/runner-phoenix-debug-mock/report.json` summary 为 `24/24 passed, avg score 1.00`。

---

## 16. 调试时的日常流程

### 复现

- 启动 Phoenix。
- 启动 aiops-v2，开启 `AIOPS_OTEL_ENABLED=1`。
- 开启 `AIOPS_DEBUG_MODEL_INPUT_TRACE=1`。
- 输入最小失败 case。
- 记录 session/turn/user input/expected/actual。

### 定位

- Phoenix 找最新 `agent.turn`。
- 看 turn 状态：completed/failed/blocked/cancelled。
- 看 `model_call` 次数是否异常。
- 看 `tool_call` 是否失败或阻断。
- 打开 `trace.file`。
- 打开 `trace.diff`。

### 判断根因

- 模型没调工具：先看 prompt、visible tools、tool schema。
- 工具参数错：看 model output 和 tool schema。
- 工具失败：看 tool error 和 input args。
- 工具成功但答案错：看 tool result 是否污染下一轮上下文。
- 多轮不收敛：看 `input.diff.md` 和 completion gate。
- 被阻断：看 approval/evidence/policy。

### 固化和回归

- 修复前补 eval case。
- 保存 baseline。
- 只改一个维度。
- 跑 current。
- 看 baseline/current delta。
- 确认没有引入退化。

---

## 17. 风险清单

### Trace 过大

- [x] 不写完整 prompt。
- [x] 不写完整 tool output。
- [x] 只写 bytes/hash/path/raw ref。

### 敏感信息泄漏

- [x] 默认不写 prompt body。
- [x] 默认不写 env。
- [x] 默认不写 token/secret/cookie。
- [x] `AIOPS_OTEL_INCLUDE_PROMPT` 即使存在，也只能本地临时使用。

### 观测影响主流程

- [x] OTel 默认关闭。
- [x] exporter 初始化失败 fallback no-op。
- [x] Phoenix 不可用时 agent 仍能执行。
- [x] shutdown flush 有 timeout。

### Eval 假安全

- [x] case 不完整时，分数不代表真实质量。
- [x] 每个真实失败都补 case。
- [x] baseline/current 只作为回归信号。

### Runner 变成新平台

- [x] runner 只跑短命令。
- [x] runner 不托管长驻服务。
- [x] runner 不替代 Phoenix。
- [x] runner 不替代 agent-eval scorer。
- [x] runner 不接入 agent runtime。

---

## 18. 最终完成标准

- [x] 默认关闭 OTel 时，项目行为不变。
- [x] 开启 OTel 后，Phoenix 能看到一次完整 turn 的 model/tool 时间线；普通 turn 和审批恢复 turn 都已验证。
- [x] 每个 model span 能定位到本地 `iteration-*.md`。
- [x] 每个相邻模型调用能查看 `input.diff.md`。
- [x] 工具失败、审批阻断能在 timeline 中直接看出。
- [x] `cmd/agent-eval -agent server` 能通过本地 aiops HTTP API 运行并生成 eval artifacts。
- [x] 真实失败能固化为 eval case。
- [x] 修改 prompt/tool/policy/model 后，能用 baseline/current 判断退化。
- [x] runner workflow 能跑短 smoke，但不成为 agent 依赖。
- [x] 不需要 Langfuse。
- [x] 不需要新增项目内 Debug 页面。

---

## 19. 建议实施顺序

1. [x] Task 1：Runtime Observer 接口。
2. [x] Task 2：Observability config。
3. [x] Task 3：OTLP exporter。
4. [x] Task 4：RuntimeObserver adapter。
5. [x] Task 5：ai-server 注入 observer。
6. [x] Task 6：turn/model spans。
7. [x] Task 7：tool spans。
8. [x] Task 8：Phoenix smoke。
9. [x] Task 9：agent-eval server adapter。
10. [x] Task 10：eval case 质量规范。
11. [x] Task 11：runner smoke workflow。
12. [x] Task 12：runbook 收敛。
13. [x] Task 15：最终 E2E 验证。

实施原则：

- [x] 每个实现 task 先写测试，再实现。
- [x] 每个 task 独立验证。
- [x] 每次只改一个明确边界。
- [x] 先 MVP，再补 approval/evidence/context/memory 的细粒度 span。
