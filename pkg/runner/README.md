# Runner 使用指南

本指南面向希望在其他项目中直接使用 `runner` 的同事，快速了解如何运行 YAML 工作流、如何启用 Agent 分发、以及如何查看执行日志。

---

## 1. 快速开始（本地运行）

最简单方式：**不启用 agent**，直接在本地执行。

```bash
go run ./runner/examples/runner-simple ./examples/simple.yaml
```

---

## 2. Agent 模式（分发执行）

### 2.1 启动 3 个 agent

```bash
go run ./runner/examples/agent-server --addr :7072 --token runner-token
go run ./runner/examples/agent-server --addr :7073 --token runner-token
go run ./runner/examples/agent-server --addr :7074 --token runner-token
```

### 2.2 通过 agent-dispatch 运行

```bash
go run ./runner/examples/agent-dispatch --token runner-token ./runner/examples/macos-multi-agent.yaml
```

运行过程中会输出 step/host 级别日志，长任务会通过轮询显示增量日志。

---

## 3. Web UI（静态页面）

```bash
go run ./runner/examples/web-ui
```

打开浏览器：
```
http://localhost:8088
```

页面可以直接粘贴 YAML，点击运行并实时看到终端输出。

---

## 4. YAML 基本结构

```yaml
version: v0.1
name: demo
inventory:
  hosts:
    local:
      address: "http://127.0.0.1:7072"
steps:
  - name: hello
    targets: [local]
    action: cmd.run
    args:
      cmd: |
        echo "hello"
```

说明：
- `inventory.hosts` 指定执行目标。
- `steps` 是执行步骤序列。

---

## 5. 常用 action

### cmd.run
执行单行/多行命令：
```yaml
action: cmd.run
args:
  cmd: |
    echo "hello"
```

### shell.run（整段 shell 脚本）
直接写完整脚本：
```yaml
action: shell.run
args:
  script: |
    echo "hello"
```

---

## 6. 变量导出与传递

启用导出后，可在 stdout 中输出：
```
BOPS_EXPORT:KEY=value
```

YAML 示例：
```yaml
action: shell.run
args:
  export_vars: true
  script: |
    echo "BOPS_EXPORT:TOKEN=abc"
```

后续步骤可直接使用 `${TOKEN}`。

---

## 7. 变量校验（expect_vars）

```yaml
steps:
  - name: step-1
    action: shell.run
    args:
      export_vars: true
      script: |
        echo "BOPS_EXPORT:OK=true"
    expect_vars: ["OK"]
```

若缺失变量，会立即失败（除非设置 `continue_on_error: true`）。

---

## 8. 条件执行（when 表达式）

支持 `${VAR}`、`== != > < >= <=`、`&& ||`：

```yaml
when: ${OK} == "true" && ${COUNT} > 1
```

---

## 9. 长任务与实时日志

agent 执行超过 4 秒会自动转异步：
- `/run` 返回 `task_id`
- dispatcher 会轮询 `/status`
- 前端可看到增量日志输出

---

## 10. 输出限制与落盘

```yaml
args:
  max_output_bytes: 2048
  output_path: /tmp/runner-artifacts
```

stdout/stderr 会被截断并落盘保存。

---

## 11. 取消任务

```bash
curl -X POST "http://127.0.0.1:7072/cancel?task_id=task-xxx"
```

---

## 12. 常见问题

### 心跳 401
说明 token 不匹配，确保 agent-server 与 dispatch 使用相同 `--token`。

### sudo 权限不足
脚本需要 root 时，请使用 root 启动 agent-server：
```bash
sudo go run ./runner/examples/agent-server --addr :7072 --token runner-token
```

---

## 13. 运行状态持久化（run_id）

Runner 现在支持按 `run_id` 持久化和查询一次运行的状态快照。

生命周期状态：
- `queued`
- `running`
- `success`
- `failed`
- `canceled`
- `interrupted`（重启对账后标记）

### 13.1 可插拔存储接口

Runner 核心不绑定数据库，业务程序通过 `state.RunStateStore` 注入：

- `CreateRun(ctx, run)`
- `UpdateRun(ctx, run)`
- `GetRun(ctx, runID)`
- `ListRuns(ctx, filter)`
- `MarkInterruptedRunning(ctx, reason)`

默认实现：
- `state.NewInMemoryRunStore()`：仅进程内，重启丢失。
- `state.NewFileStore(path)`：文件持久化（示例可用）。

若使用内存存储，Runner 会输出非持久化告警。

### 13.2 回调接口

可选注入 `state.RunStateNotifier`，Runner 在状态变化时异步回调：

回调字段：
- `run_id`
- `workflow_name`
- `status`
- `step`
- `host`
- `timestamp`
- `error`
- `version`

回调失败不会改变运行结果状态；失败会记录到 `last_notify_error`。

### 13.3 Engine 注入示例

```go
eng := engine.New(registry)
eng.RunStore = myStore
eng.Notifier = myNotifier
eng.NotifyRetry = 2
eng.NotifyDelay = 500 * time.Millisecond

run, err := eng.ApplyWithRun(ctx, wf, engine.RunOptions{
  RunID: "run-20260212-001",
})
```

### 13.4 示例服务查询接口

- `runner/examples/web-ui`
  - `GET /run-status?run_id=<id>`
- `runner/examples/agent-server`
  - `GET /run-status?run_id=<id>`

未知 `run_id` 返回 404（not found）。

---

如果需要更高级功能（持久化、审批、回滚），请参考 `exception.md` 与 `todo.md`。  
如需扩展模块或接入自定义 agent，请联系维护者。
