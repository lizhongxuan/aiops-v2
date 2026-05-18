# AIOps-v2 主机 SSH 接入与 host-agent 安装设计方案

日期：2026-05-18（Asia/Shanghai）  
状态：设计方案  
适用范围：AIOps-v2 主机管理、SSH 终端、Runner 工作流、host-agent 首版安装启动闭环  
关联文档：

- `docs/2026-05-18-aiops-v2-first-release-scope.zh.md`
- `pkg/runner/VISUAL_WORKFLOW_UI_DESIGN.md`
- `pkg/runner/README.md`
- `internal/appui/host_service.go`
- `internal/server/host_api.go`
- `internal/appui/terminal_service.go`
- `pkg/runner/server/api/visual_workflow_handler.go`

## 1. 背景与目标

AIOps-v2 现有主机管理已经具备主机 CRUD、选择主机、主机画像/租约展示、Terminal 页面入口和嵌入式 Runner Studio API，但“添加主机 -> SSH 连通 -> 安装 host-agent -> 启动 -> 心跳在线”的闭环尚未真正打通。

本方案要让用户在主机管理列表中添加一台真实主机，填写 SSH 接入信息和受控凭据引用后，由 AIOps-v2 发起内置 Runner 工作流，通过 SSH 预检、探测平台、上传/安装 host-agent、启动服务并校验心跳。成功后主机进入 `managed` / `online`，并可从主机列表打开 SSH 终端。

首版只支持两个 host-agent 安装目标：

- macOS Apple Silicon：`darwin/arm64`，面向 M1/M2/M3 等 Apple Silicon 主机，使用 `launchd` 管理服务。
- Ubuntu Linux：`linux/ubuntu`，面向 systemd Ubuntu 主机，使用 `systemd` 管理服务。

不支持的系统、架构或服务管理器必须在预检或平台探测阶段失败，并给出 `unsupported_platform` 或等价明确错误，不得继续伪装安装成功。

## 2. 当前代码现状

主机服务和页面已经存在，但安装闭环缺失：

- `HostRecord` 已有 `address`、`sshUser`、`sshPort`、`transport`、`status`、`installState`、`agentVersion`、`lastHeartbeat`、`terminalCapable`、`executable` 等字段。
- `HostUpsert` 已有 `installViaSsh` 字段，`HostService.CreateHost()` 会把主机置为 `ssh_bootstrap`、`installing`、`pending_install`，但不会触发 Runner 工作流。
- `HostsPage` 可以创建/编辑主机，但当前 payload 没有显式传递 `installViaSsh`，也没有凭据引用、安装进度、失败重试和 run 详情入口。
- `TerminalService` 会校验主机在线和 `terminalCapable`/`executable`，但 `terminal.Manager` 目前只启动本机 shell；远程 SSH 终端尚未接入。
- `cmd/ai-server` 默认启动嵌入式 Runner runtime，并通过 `/api/runner-studio/*` 暴露 Runner API。
- Runner 已支持 graph run、run state、SSE/history、Run Record、`script.shell` / `script.python`，但默认 registry 和前端 action catalog 仍包含 `cmd.run`、`shell.run`，需要按首版范围收敛为 `script.shell` 唯一 Shell 入口。
- `pkg/runner/agent` 已有 HTTP `/run`、`/status`、`/cancel`、`/heartbeat` 执行代理能力，可作为 host-agent v0 的执行协议基础，但还没有完整安装包、主动注册/心跳上报和 HostRecord 状态更新链路。

## 3. 方案选择

### 推荐方案：Runner 驱动的受控 SSH Bootstrap

添加主机时，AIOps-v2 创建 HostRecord，并由 `HostBootstrapService` 调用嵌入式 Runner 提交内置 `builtin.host-agent-install/v1` 工作流。工作流在 `server-local` 上执行受控 `script.shell` 脚本，通过 SSH/SCP 操作目标主机。

优点：

- 复用 Runner 的 run state、事件、Run Record、失败步骤和审计能力。
- SSH 行为被固定在内置安装脚本中，不向用户开放任意 SSH 命令入口。
- 与现有 `/api/runner-studio/runs`、run graph、event history、Runner UI 能力一致。
- 可以把安装失败直接映射到 HostRecord 的 `install_failed`、`lastError`、`installRunID`。

代价：

- 需要新增 HostBootstrapService、凭据解析、host-agent artifact manifest、host-agent heartbeat 接收端和远程 SSH terminal command factory。
- Runner 当前 `cmd.run` / `shell.run` 仍未删除，必须作为同一发布切片内的前置收敛项处理。

### 备选方案 A：ai-server 内直接执行 SSH 安装

在 `HostService.CreateHost()` 中直接调用本机 `ssh/scp` 完成安装。

不推荐。它绕过 Runner run state 和审计，失败步骤难以回放，安装过程无法在 Runner Studio 中查看，也会让 HostService 变成编排器。

### 备选方案 B：只提供手工安装命令

页面保存主机信息后生成命令，让用户手工 SSH 到目标主机执行。

只能作为失败兜底，不满足“使用 Runner 工作流安装并启动 host-agent”的目标，也不能证明 AIOps-v2 完成了首版真实闭环。

## 4. 总体架构

```text
HostsPage
  -> POST /api/v1/hosts
  -> HostService 保存 HostRecord(pending_install)
  -> HostBootstrapService 提交 Runner graph run
  -> builtin.host-agent-install/v1
       -> server-local script.shell
       -> ssh/scp 到目标主机
       -> 安装 host-agent
       -> 启动 systemd / launchd
       -> 验证本机 health 和 AIOps heartbeat
  -> host-agent register / heartbeat
  -> HostAgentService 更新 HostRecord(online, managed)
  -> HostsPage 展示 run 进度、失败原因、重试、终端入口
```

核心边界：

- `HostService` 只负责主机记录 CRUD 和状态查询，不直接跑 SSH。
- `HostBootstrapService` 负责把主机安装意图转换为 Runner run，并把 run 状态回写到 HostRecord。
- `Runner` 负责编排、执行、记录、事件流和失败定位。
- `CredentialResolver` 只在服务端解析 `secret_ref`，不得把凭据写入 HostRecord、前端 payload、Run Record 或日志。
- `HostAgentService` 接收 host-agent 注册和心跳，更新在线状态和画像字段。
- `TerminalService` 根据 HostRecord 和凭据引用创建 SSH-backed terminal session，不再把非本机主机误接成本机 shell。

## 5. 数据模型

在现有 `store.HostRecord` 基础上增加或明确以下字段语义：

```go
type HostRecord struct {
    ID              string
    Name            string
    Kind            string            // inventory | managed
    Address         string            // 用户录入的 SSH 地址或主机名
    SSHUser         string
    SSHPort         int
    SSHCredentialRef string          // secret://team/key 或 vault 引用，仅保存引用
    Transport       string            // ssh_bootstrap | agent_http | grpc_reverse | local
    AgentURL        string            // host-agent / runner-agent HTTP endpoint，例如 http://10.0.0.11:7072
    AgentTokenRef   string            // host-agent token 引用或服务端托管 token id
    Status          string            // offline | installing | online | install_failed | stale
    InstallState    string            // inventory | pending_install | running | installed | failed | unsupported_platform
    InstallRunID    string
    InstallWorkflowID string
    InstallStep     string
    LastError       string
    OS              string
    Arch            string
    AgentVersion    string
    LastHeartbeat   string
    TerminalCapable bool
    Executable      bool
    ControlMode     string            // inventory | managed
    Labels          map[string]string
}
```

如果希望减少一次性 schema 迁移，`AgentURL`、`AgentTokenRef`、`InstallRunID`、`InstallWorkflowID`、`InstallStep` 可以先放入 `Metadata map[string]any`。但长期建议升为强类型字段，因为这些字段会被主机列表、Runner 回调、终端服务和健康检查共同使用。

新增安装运行读模型：

```go
type HostInstallRun struct {
    HostID       string
    RunID        string
    WorkflowID   string
    Status       string
    CurrentStep  string
    StartedAt    time.Time
    FinishedAt   time.Time
    LastError    string
    Platform     string
    AgentVersion string
}
```

`HostInstallRun` 可以由 HostRecord 的安装字段和 Runner run state 合成，不必首版单独建表。

## 6. API 设计

### 创建主机

`POST /api/v1/hosts`

请求：

```json
{
  "id": "prod-web-01",
  "name": "prod-web-01",
  "address": "10.0.0.11",
  "sshUser": "ubuntu",
  "sshPort": 22,
  "sshCredentialRef": "secret://ops/prod-web-01-ssh-key",
  "labels": { "env": "prod", "role": "web" },
  "installViaSsh": true,
  "agentVersion": "v0.1.0"
}
```

行为：

- `installViaSsh=false`：只保存 inventory 主机，状态为 `offline` / `inventory`。
- `installViaSsh=true`：保存为 `ssh_bootstrap`、`installing`、`pending_install`，随后提交内置安装 workflow。
- 返回 `host`、`items`，如果触发安装，还返回 `installRunId` 和 `installWorkflowId`。

### 重试安装

`POST /api/v1/hosts/{id}/install`

请求：

```json
{
  "agentVersion": "v0.1.0",
  "sshCredentialRef": "secret://ops/prod-web-01-ssh-key",
  "force": false
}
```

行为：

- 主机不存在返回 404。
- 主机正在安装时返回当前 `installRunId`，除非 `force=true` 且当前 run 已终止。
- 重新提交 Runner workflow，并把 HostRecord 更新为 `installing`。

### SSH 预检

`POST /api/v1/hosts/{id}/ssh/test`

用途是保存前或安装前测试 SSH 连通性、认证、sudo 能力和基础平台探测。该接口也通过受控 `script.shell` 或 HostBootstrapService 执行，不开放任意命令。

响应示例：

```json
{
  "status": "ok",
  "platform": "linux/ubuntu",
  "os": "ubuntu",
  "arch": "amd64",
  "sudo": "passwordless",
  "message": "SSH preflight passed"
}
```

### host-agent 注册和心跳

`POST /api/v1/host-agents/register`

```json
{
  "hostId": "prod-web-01",
  "hostname": "prod-web-01",
  "os": "ubuntu",
  "arch": "amd64",
  "agentVersion": "v0.1.0",
  "capabilities": ["script.shell", "script.python", "terminal"],
  "labels": { "env": "prod" }
}
```

`POST /api/v1/host-agents/heartbeat`

```json
{
  "hostId": "prod-web-01",
  "agentVersion": "v0.1.0",
  "timestamp": "2026-05-18T10:00:00+08:00"
}
```

两类请求都必须带 host-agent token。服务端只保存 token 引用或 hash，不把明文 token 返回前端。

## 7. 内置 Runner 工作流

工作流 ID：`builtin.host-agent-install/v1`  
工作流名称：`host-agent-install`  
触发入口：HostBootstrapService，不作为普通用户可编辑工作流暴露。  
执行目标：`server-local`。安装动作由本机 Runner 使用 SSH/SCP 连接目标主机完成。
节点约束：该工作流必须拆成多个独立 Runner graph node；每个安装、预检、校验、收尾节点的 action 都固定为 `script.shell`，不得使用 `cmd.run`、`shell.run`、LLM、prompt、chat/completion、agent planning 或任意模型调用节点。

输入变量：

```yaml
vars:
  host_id: prod-web-01
  ssh_host: 10.0.0.11
  ssh_user: ubuntu
  ssh_port: 22
  ssh_credential_ref: secret://ops/prod-web-01-ssh-key
  agent_version: v0.1.0
  agent_server_url: http://aiops.example.internal:8080
  agent_listen_port: 7072
  labels:
    env: prod
    role: web
```

步骤。下列每一项都是一个独立 Runner graph node，`action=script.shell`，节点之间只通过受控 outputs 传递必要状态：

1. `validate-inputs` (`script.shell`)
   - 校验 `host_id`、`ssh_host`、`ssh_user`、`ssh_credential_ref`、`agent_version`。
   - 确认凭据引用可解析，但不输出明文。
   - 只做确定性参数校验，不调用 LLM 生成脚本或判断安装策略。

2. `tcp-preflight` (`script.shell`)
   - 使用固定脚本探测 `${ssh_host}:${ssh_port}`，优先调用 `nc -z`，无 `nc` 时使用 `/dev/tcp` 兜底。
   - 不引入 `builtin.tcp_ping` 或其他非 `script.shell` 节点，避免安装工作流出现第二套执行语义。

3. `ssh-preflight` (`script.shell`)
   - 通过 SSH 执行固定只读命令：`echo aiops-ssh-ok`、`id -u`、`command -v sudo`。
   - 验证认证、主机可达、sudo 策略。
   - 不允许用户传入任意命令片段。

4. `detect-platform` (`script.shell`)
   - 执行 `uname -s`、`uname -m`、`sw_vers` 或读取 `/etc/os-release`。
   - 输出 `platform=darwin/arm64` 或 `platform=linux/ubuntu`。
   - 非支持平台失败。

5. `resolve-artifact` (`script.shell`)
   - 从 host-agent artifact manifest 选择二进制和 sha256。
   - 支持 `darwin/arm64` 和 `linux/ubuntu`。

6. `upload-artifact` (`script.shell`)
   - 使用 `scp` 或 `ssh cat > file` 上传二进制、配置文件和校验文件到临时目录。
   - 校验 sha256。

7. `install-files` (`script.shell`)
   - Linux Ubuntu：安装到 `/opt/aiops/host-agent/host-agent`，配置到 `/etc/aiops/host-agent.yaml`。
   - macOS arm64：安装到 `/usr/local/aiops/host-agent/host-agent`，配置到 `/usr/local/etc/aiops/host-agent.yaml`。

8. `install-service` (`script.shell`)
   - Ubuntu 写入 `aiops-host-agent.service` 并执行 `systemctl daemon-reload`。
   - macOS 写入 `com.aiops.host-agent.plist` 并执行 `launchctl bootstrap`。

9. `start-service` (`script.shell`)
   - 启动服务，并读取服务状态。

10. `verify-local-health` (`script.shell`)
    - 在目标主机本机请求 `http://127.0.0.1:${agent_listen_port}/health`。

11. `verify-aiops-heartbeat` (`script.shell`)
    - 轮询 AIOps-v2 的 HostRecord 或 host-agent heartbeat endpoint，直到 `lastHeartbeat` 更新且状态为 `online`。

12. `finalize-host` (`script.shell`)
    - 更新 HostRecord：`transport=agent_http` 或后续 `grpc_reverse`、`status=online`、`installState=installed`、`controlMode=managed`、`terminalCapable=true`、`executable=true`、`agentVersion`。

非 LLM 约束：

- host-agent 安装全链路是确定性 Runner graph run，只执行仓库内置的 `script.shell` 模板和服务端注入的受控变量。
- 工作流定义、节点参数、平台分支、artifact 选择、服务文件内容、健康检查和失败映射都由代码和配置决定，不允许运行时调用 LLM 生成命令、补全脚本、解释错误或选择下一步。
- Runner workflow validator 必须拒绝包含 `llm.*`、`prompt.*`、`chat.*`、`completion.*`、`agent.*` 或其他模型调用语义的节点。
- LLM 可以用于设计、代码生成或离线文档辅助，但不得参与生产安装 run 的执行路径。

失败处理：

- 任一步骤失败时，HostBootstrapService 将 HostRecord 更新为 `status=install_failed`、`installState=failed`、`lastError=<redacted error>`、`installStep=<step>`。
- `unsupported_platform` 单独保留为 `installState=unsupported_platform`，方便 UI 给出清晰原因。
- Runner Run Record 保留步骤、stdout/stderr 摘要、错误码和脱敏参数。

## 8. host-agent v0 行为

首版 host-agent 可以复用 `pkg/runner/agent` 的执行协议，但产品语义上统一称为 `host-agent`。二进制应支持以下能力：

- 启动时读取 `host-agent.yaml`。
- 主动向 AIOps-v2 注册和定期心跳。
- 暴露 `/health`、`/heartbeat`、`/run`、`/status`、`/cancel`，供 Runner dispatcher 调用。
- 默认只注册 `script.shell`、`script.python` 和必要只读探针能力，不注册 `cmd.run`、`shell.run`。
- 所有任务输出经过 max output 限制和敏感字段脱敏。
- token 从配置读取，配置文件权限为 root/admin only。

配置示例：

```yaml
host_id: prod-web-01
server_url: http://aiops.example.internal:8080
listen_addr: 0.0.0.0:7072
token_ref: file:///etc/aiops/host-agent.token
labels:
  env: prod
  role: web
capabilities:
  - script.shell
  - script.python
  - terminal
heartbeat_interval: 15s
```

当首版需要更强的网络穿透或统一终端/文件协议时，再把现有 `internal/server/grpc.go` 的 stub 扩展为真实 gRPC reverse channel。首版不把 gRPC 作为安装成功的必要条件，避免把多个未完成协议同时压入关键路径。

## 9. SSH 终端设计

主机列表的“终端”按钮应根据 HostRecord 选择终端后端：

- `server-local`：继续使用本机 shell。
- 有 `sshCredentialRef` 的 `ssh_bootstrap` / `managed` 主机：使用 SSH-backed terminal。
- 后续 `grpc_reverse` 完成后：可使用 host-agent terminal channel。

SSH-backed terminal 由 `TerminalService` 根据 HostRecord 构造受控命令：

```text
ssh -tt -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=15 -p <port> -i <temp_key> <user>@<address>
```

约束：

- 临时 key 文件由 CredentialResolver 生成，权限 `0600`，session 结束后删除。
- 页面和 WebSocket 事件只展示终端输出，不展示凭据路径、key 内容或密码。
- 没有凭据引用、主机不在线、主机被标记不可终端时，按钮禁用或接口返回明确错误。
- 密码凭据如需支持，使用受控 askpass/sshpass 替代明文命令参数；首版推荐 SSH key 作为默认路径。

## 10. 前端交互

`HostsPage` 在“接入主机”对话框中增加：

- `通过 SSH 安装 host-agent` 开关。
- `SSH 凭据引用` 输入框，占位提示使用 `secret://team/host-key`。
- `host-agent 版本` 选择或默认值。
- `安装状态`、`当前步骤`、`安装 Run ID` 和 `查看 Runner 详情`。
- 失败态展示 `lastError`、失败步骤、重试按钮。

列表行为：

- `pending_install` / `running` 显示“安装中”，并轮询 host 和 run 状态。
- `install_failed` 显示“安装失败”，保留重试。
- `unsupported_platform` 显示“不支持的平台”。
- `online` 显示“在线”，开放终端和后续 Runner 执行入口。

由于这是可见 UI 变更，必须按照 `AGENTS.md` 增加或更新 Playwright screenshot snapshot，优先复用 `web/tests/helpers/uiFixtureHarness.js`。

## 11. 安全与审计

凭据安全：

- HostRecord 只保存 `sshCredentialRef`、`agentTokenRef` 或 token hash。
- Runner vars 和 Run Record 中敏感字段统一写入引用或 `[redacted]`。
- `ssh` 命令构造不能把密码放入命令行参数。
- 临时凭据文件必须在 workflow step 结束或 run 结束时清理。

执行边界：

- 添加主机安装只允许调用内置 `builtin.host-agent-install/v1`。
- 用户不能通过主机接入页面输入任意 shell。
- Runner 默认 action registry 必须移除 `cmd.run` 和 `shell.run`，首版只保留 `script.shell` 作为 Shell 入口。
- `script.shell` 在安装 workflow 中只执行固定模板脚本，变量通过参数注入并进行 shell escaping。
- `builtin.host-agent-install/v1` 的所有节点必须是 `script.shell`，不得混入 LLM、prompt、chat/completion、agent planning 或模型工具调用节点。
- 生产安装 run 的输入、分支、错误处理和重试策略只能来自代码、配置、Runner state 和目标主机探测结果，不得由 LLM 参与决策。

审计：

- Host create/install/retry/terminal open 写入审计事件。
- Runner Run Record 包含 host id、workflow id、run id、操作者、agent version、平台、步骤状态和脱敏错误。
- host-agent register/heartbeat 更新 HostRecord 时记录更新时间和来源。

## 12. 状态流转

```text
inventory/offline
  -> pending_install/installing
  -> running/installing
  -> installed/online/managed

pending_install/installing
  -> failed/install_failed
  -> unsupported_platform/install_failed
  -> retry -> pending_install/installing

online
  -> stale
  -> offline
  -> reinstall -> pending_install/installing
```

状态判定：

- 最近心跳小于等于 60 秒：`online`。
- 最近心跳超过 60 秒但小于等于 5 分钟：`stale`。
- 超过 5 分钟或主动失败：`offline`。
- 安装 run 未结束时优先展示 `installing`。
- 安装失败优先展示 `install_failed`，直到用户重试或手动清除。

## 13. 测试策略

后端单元测试：

- `HostService.CreateHost()` 保存 `installViaSsh`、`sshCredentialRef`、安装状态和 run id。
- `HostBootstrapService` 使用 fake RunnerClient 断言提交的 workflow、vars、idempotency key 和状态回写。
- `HostAgentService` 注册/心跳更新 HostRecord，并拒绝错误 token。
- `TerminalService` 对远程主机构造 SSH command factory，验证无凭据泄漏。
- Runner registry 校验 `cmd.run`、`shell.run` 不再出现在默认 action catalog。

Runner 测试：

- `builtin.host-agent-install/v1` graph validate 通过。
- `builtin.host-agent-install/v1` 至少包含 `validate-inputs`、`tcp-preflight`、`ssh-preflight`、`detect-platform`、`resolve-artifact`、`upload-artifact`、`install-files`、`install-service`、`start-service`、`verify-local-health`、`verify-aiops-heartbeat`、`finalize-host` 这 12 个独立节点。
- 安装 workflow 的每个节点 action 都必须是 `script.shell`。
- 旧工作流引用 `cmd.run` 或 `shell.run` 在校验阶段失败。
- 安装 workflow 引用 `llm.*`、`prompt.*`、`chat.*`、`completion.*`、`agent.*` 或其他模型调用语义节点时，必须在校验阶段失败。
- 安装 workflow 的每个失败分支输出明确错误码：`ssh_unreachable`、`auth_failed`、`sudo_required`、`unsupported_platform`、`artifact_missing`、`service_start_failed`、`heartbeat_timeout`。

前端测试：

- 主机创建对话框覆盖安装开关和凭据引用。
- 列表覆盖 `installing`、`install_failed`、`unsupported_platform`、`online`。
- 终端按钮只在可连接主机上启用。
- 更新 Playwright screenshot snapshot。

集成验收：

- 真实 Ubuntu 主机：添加、安装、systemd 启动、heartbeat online、打开 SSH 终端。
- 真实 macOS arm64 主机：添加、安装、launchd 启动、heartbeat online、打开 SSH 终端。
- 错误凭据、端口不通、无 sudo、不支持平台都产生明确失败，不进入 online。

## 14. 分阶段落地

第一阶段：主机接入和安装编排

- 扩展 HostUpsert、HostRecord、HostService。
- 增加 HostBootstrapService 和 RunnerClient。
- 前端提交 `installViaSsh`、`sshCredentialRef`。
- 创建内置 `host-agent-install` workflow。

第二阶段：host-agent 包装和心跳

- 将 `pkg/runner/agent` 打包为 host-agent v0 或增加 host-agent wrapper。
- 增加 register/heartbeat endpoint。
- 增加 HostAgentMonitor 或主动 heartbeat 状态更新。
- 生成 macOS arm64 与 Ubuntu artifact manifest。

第三阶段：SSH 终端

- TerminalService 接入 HostRepository 和 CredentialResolver。
- TerminalManager 支持按 HostRecord 构造远程 SSH command。
- 前端错误态和禁用态补齐。

第四阶段：Runner 能力收敛

- 默认 registry 和 action catalog 删除 `cmd.run`、`shell.run`。
- `script.shell` schema 改为首选 inline controlled script，不再默认 `script_ref`。
- 工作流校验阻止旧 action。

第五阶段：真实环境验收

- 用一台干净 Ubuntu 和一台 macOS arm64 主机跑通安装。
- 保留 Run Record、截图和失败路径证据。
- 若发现产品问题，按 `docs/self-improvement/` 格式沉淀问题文档。

## 15. 验收标准

- 用户能在主机管理列表新增主机并选择 SSH 安装 host-agent。
- AIOps-v2 能通过 SSH 完成预检、平台探测、安装、启动和心跳校验。
- 只支持 macOS arm64 和 Ubuntu；其他平台明确失败。
- 安装过程由 Runner run 承载，能查看当前步骤、日志摘要、失败原因和 Run ID。
- Runner 安装工作流拆成多个明确步骤，且每个安装节点都是 `script.shell`。
- host-agent 安装和启动过程不调用 LLM，也不依赖 LLM 生成命令、选择分支或解释失败。
- 安装成功后 HostRecord 进入 `online` / `managed`，有 `lastHeartbeat` 和 `agentVersion`。
- 主机列表能打开 SSH 终端，且不会连接成本机 shell。
- 凭据、token、私钥、密码不出现在页面、日志、Run Record、Prompt Trace 或文档中。
- `cmd.run` 和 `shell.run` 不再作为首版默认 Shell 执行入口出现。

## 16. 非目标

- 不实现完整 CMDB。
- 不支持 Windows、CentOS、Debian 非 Ubuntu、macOS Intel 或非 systemd Linux。
- 不开放任意 SSH 命令执行页面。
- 不把 gRPC reverse channel 作为首版安装成功的必要条件。
- 不做 host-agent 自动升级策略；首版只要求安装指定版本和重装。
- 不把明文 SSH 私钥或密码保存在 AIOps-v2 数据文件中。
