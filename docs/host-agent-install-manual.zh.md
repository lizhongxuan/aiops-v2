# AIOps-v2 host-agent SSH 安装验收手册

本文用于验证“添加主机 -> SSH 连通 -> Runner 多步骤 `script.shell` 安装 host-agent -> 启动服务 -> 心跳在线 -> SSH 终端”的首版闭环。安装 workflow 固定使用仓库内置脚本，不在安装、启动、平台分支或失败判断过程中调用 LLM。

## 1. Ubuntu 准备

- 目标系统必须是 Ubuntu x86_64/amd64，且使用 systemd。
- SSH 端口需要从 AIOps-v2 所在机器可达。
- 建议使用 SSH key；如使用密码，密码只放在 `AIOPS_SECRET_DIR` 本地文件中。
- 非 root 用户需要具备 passwordless sudo；root 用户可直接安装。
- 目标主机需要允许 host-agent 监听 `0.0.0.0:7072`，本机健康检查使用 `127.0.0.1:7072`。
- 如果目标主机无法访问 AIOps-v2 的 HTTP 地址，需要先提供可达的 `AIOPS_AGENT_SERVER_URL`。本地开发可用 SSH reverse tunnel，例如让目标主机访问自己的 `127.0.0.1:18080` 转回本机 AIOps-v2。

## 2. macOS arm64 准备

- 目标系统必须是 Apple Silicon macOS，`uname -m` 返回 `arm64`。
- 开启 Remote Login，并确认 AIOps-v2 所在机器可 SSH 登录。
- 安装用户需要具备 sudo 权限；首版按 launchd system daemon 安装。
- 确认 `/usr/local/aiops`、`/usr/local/etc/aiops`、`/usr/local/var/log/aiops` 可由 sudo 创建。
- 若目标主机无法访问 AIOps-v2 HTTP 地址，同样需要设置可达的 `AIOPS_AGENT_SERVER_URL`。

## 3. 凭据目录格式

默认凭据目录是 `.data/secrets`，也可通过环境变量指定：

```bash
export AIOPS_SECRET_DIR=/secure/path/aiops-secrets
```

`secret://` 引用会映射到该目录下的相对路径：

```text
secret://lab/ubuntu-smoke -> $AIOPS_SECRET_DIR/lab/ubuntu-smoke
secret://prod/web-01-key  -> $AIOPS_SECRET_DIR/prod/web-01-key
```

密码文件内容为单行密码，私钥文件内容为 OpenSSH/RSA/EC 私钥。文件权限建议：

```bash
mkdir -p "$AIOPS_SECRET_DIR/lab"
chmod 700 "$AIOPS_SECRET_DIR" "$AIOPS_SECRET_DIR/lab"
chmod 600 "$AIOPS_SECRET_DIR/lab/ubuntu-smoke"
```

不要把明文密码、私钥或 host-agent token 写入 HostRecord、Run vars、文档或截图。Run Record 只应保留 `secret://...` 引用。

## 4. 添加主机请求示例

Ubuntu：

```bash
curl -sS -X POST http://127.0.0.1:18080/api/v1/hosts \
  -H 'Content-Type: application/json' \
  -d '{"id":"ubuntu-smoke","name":"ubuntu-smoke","address":"<ubuntu-ip>","sshUser":"ubuntu","sshPort":22,"sshCredentialRef":"secret://lab/ubuntu-smoke","installViaSsh":true,"agentVersion":"v0.1.0"}'
```

macOS arm64：

```bash
curl -sS -X POST http://127.0.0.1:18080/api/v1/hosts \
  -H 'Content-Type: application/json' \
  -d '{"id":"macos-smoke","name":"macos-smoke","address":"<mac-ip>","sshUser":"<user>","sshPort":22,"sshCredentialRef":"secret://lab/macos-smoke","installViaSsh":true,"agentVersion":"v0.1.0"}'
```

成功提交后，响应应包含 `installRunId` 和 `installWorkflowId=builtin.host-agent-install/v1`。Runner Run 需要包含 12 个 `script.shell` 节点：`validate-inputs`、`tcp-preflight`、`ssh-preflight`、`detect-platform`、`resolve-artifact`、`upload-artifact`、`install-files`、`install-service`、`start-service`、`verify-local-health`、`verify-aiops-heartbeat`、`finalize-host`。

## 5. 成功验收

Ubuntu：

```bash
ssh <user>@<ubuntu-ip> 'systemctl is-active aiops-host-agent.service && curl -fsS http://127.0.0.1:7072/health'
curl -sS http://127.0.0.1:18080/api/v1/hosts
```

预期结果：

- `aiops-host-agent.service` 为 `active`。
- `/health` 返回 `status=ok`、正确 `host_id` 和 `version`。
- HostRecord 变为 `status=online`、`installState=installed`、`controlMode=managed`。
- 主机列表中的终端按钮可用，并打开远端 SSH shell。

macOS arm64：

```bash
ssh <user>@<mac-ip> 'sudo launchctl print system/com.aiops.host-agent >/dev/null && curl -fsS http://127.0.0.1:7072/health'
curl -sS http://127.0.0.1:18080/api/v1/hosts
```

预期结果同 Ubuntu，服务由 `com.aiops.host-agent` 管理。

## 6. 失败场景

- 错误凭据：`ssh-preflight` 失败，HostRecord 不应进入 `online`。
- SSH 端口不通：`tcp-preflight` 失败，Run Record 保留失败步骤。
- 无 sudo：`ssh-preflight` 或安装步骤失败，错误信息不能包含密码、私钥或 token。
- 不支持平台：`detect-platform` 失败，平台不是 `linux/ubuntu` 或 `darwin/arm64`。

失败后需要保留 `installRunId`、失败步骤、脱敏错误摘要和截图。确认 `.data/runner/run-records.jsonl` 中不包含明文凭据。

## 7. 证据保留

每次验收保留：

- 主机创建响应中的 `installRunId`。
- Runner Run Record 和 Run State。
- 主机列表截图，包含安装状态、Run ID 或 online 状态。
- 目标主机服务状态输出。
- 终端会话打开远端 shell 的截图或事件记录。

最终安全扫描：

```bash
rg -n 'BEGIN OPENSSH PRIVATE KEY|BEGIN RSA PRIVATE KEY|password=|Authorization: Bearer [A-Za-z0-9._-]+' .data docs web/src internal pkg/runner
```
