# runner/agent

轻量任务执行 agent，用于接收其他 runner 分发的 task，并提供任务状态查询。

## 启动

```bash
go run ./runner/agent --addr :7072 --token runner-token
```

可选参数：

- `--addr` 监听地址，默认 `:7072`
- `--token` 鉴权 token，默认 `runner-token`；设置为空表示不鉴权
- `--log-level` 日志级别，默认 `info`
- `--log-format` 日志格式，默认 `console`
- `--async-threshold-sec` 自动切异步阈值（wait 未显式传入时），默认 `4`
- `--max-output-bytes` 每个任务内存中保留的 stdout/stderr 最大字节数，默认 `65536`

## 接口

- `POST /run` 提交任务
- `POST|GET /status` 查询任务状态
- `POST|GET /cancel` 取消任务
- `POST /heartbeat` 心跳
- `GET /health` 健康检查

请求/响应结构与 `runner/scheduler/hybrid_dispatcher.go` 的 agent 协议保持兼容。

