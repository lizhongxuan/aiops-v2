import { describe, expect, it } from "vitest";
import { buildHostListViewModel } from "../src/lib/hostListViewModel";

const NOW = new Date("2026-05-03T10:00:00Z");

function minutesAgo(minutes) {
  return new Date(NOW.getTime() - minutes * 60_000).toISOString();
}

describe("hostListViewModel", () => {
  it("excludes server-local and maps rows to the simplified host table", () => {
    const model = buildHostListViewModel({
      hosts: [
        { id: "server-local", name: "server-local", status: "online", transport: "local" },
        {
          id: "host-online",
          name: "web-a",
          address: "10.0.2.15",
          sshUser: "root",
          sshPort: 22,
          status: "online",
          kind: "agent",
          transport: "grpc_reverse",
          agentVersion: "v0.8.1",
          lastHeartbeat: minutesAgo(0.5),
          terminalCapable: true,
          executable: true,
        },
      ],
      sessions: [
        { id: "s1", selectedHostId: "host-online", kind: "single_host" },
        { id: "s2", selectedHostId: "host-online", kind: "single_host" },
        { id: "workspace-1", selectedHostId: "host-online", kind: "workspace" },
      ],
      terminalSessions: [
        { id: "term-1", status: "running" },
        { id: "term-2", status: "starting" },
        { id: "term-3", status: "closed" },
      ],
      now: NOW,
    });

    expect(model.rows).toHaveLength(1);
    expect(model.rows[0]).toMatchObject({
      id: "host-online",
      ip: "10.0.2.15",
      user: "root",
      title: "10.0.2.15 / root",
      subtitle: "client v0.8.1 · key 10.0.2.15:root",
      heartbeat: "online",
      heartbeatLabel: "在线",
      heartbeatTone: "success",
      sourceLabel: "client",
      sshLabel: "可 SSH",
      sessionCount: 2,
      canOpenSsh: true,
      primaryAction: "session",
    });
    expect(model.stats).toEqual([
      { label: "心跳正常", value: 1 },
      { label: "超过 60s 未心跳", value: 0 },
      { label: "活跃终端会话", value: 2 },
    ]);
  });

  it("classifies online, stale, installing, and offline hosts", () => {
    const model = buildHostListViewModel({
      hosts: [
        {
          id: "host-online",
          address: "10.0.2.15",
          sshUser: "root",
          status: "online",
          transport: "grpc_reverse",
          lastHeartbeat: minutesAgo(0.25),
        },
        {
          id: "host-stale",
          address: "172.16.8.9",
          sshUser: "ubuntu",
          status: "online",
          transport: "grpc_reverse",
          lastHeartbeat: minutesAgo(2),
        },
        {
          id: "host-installing",
          address: "192.168.1.42",
          sshUser: "deploy",
          status: "installing",
          transport: "ssh_bootstrap",
          installState: "pending_install",
        },
        {
          id: "host-offline",
          address: "10.0.2.16",
          status: "offline",
          transport: "grpc_reverse",
        },
      ],
      now: NOW,
    });

    expect(model.rows.map((row) => row.heartbeat)).toEqual(["online", "stale", "installing", "offline"]);
    expect(model.rows.map((row) => row.primaryAction)).toEqual(["session", "reinstall", "install", "reinstall"]);
    expect(model.rows.map((row) => row.heartbeatLabel)).toEqual(["在线", "超时", "待安装", "离线"]);
  });

  it("searches by IP plus username", () => {
    const model = buildHostListViewModel({
      hosts: [
        { id: "a", address: "10.0.2.15", sshUser: "root", status: "online", lastHeartbeat: minutesAgo(0.2) },
        { id: "b", address: "192.168.1.42", sshUser: "deploy", status: "installing" },
      ],
      query: "10.0.2.15 root",
      now: NOW,
    });

    expect(model.rows.map((row) => row.id)).toEqual(["a"]);
  });

  it("filters by heartbeat, source, and SSH state", () => {
    const hosts = [
      {
        id: "client",
        address: "10.0.2.15",
        sshUser: "root",
        status: "online",
        transport: "grpc_reverse",
        lastHeartbeat: minutesAgo(0.2),
        terminalCapable: true,
      },
      {
        id: "manual",
        address: "192.168.1.42",
        sshUser: "deploy",
        status: "installing",
        transport: "ssh_bootstrap",
      },
      {
        id: "offline",
        address: "172.16.8.9",
        status: "offline",
        transport: "grpc_reverse",
      },
    ];

    expect(buildHostListViewModel({ hosts, filters: { heartbeat: "installing" }, now: NOW }).rows.map((row) => row.id)).toEqual(["manual"]);
    expect(buildHostListViewModel({ hosts, filters: { source: "client" }, now: NOW }).rows.map((row) => row.id)).toEqual(["client", "offline"]);
    expect(buildHostListViewModel({ hosts, filters: { ssh: "无密码" }, now: NOW }).rows.map((row) => row.id)).toEqual(["offline"]);
  });

  it("paginates filtered rows", () => {
    const hosts = Array.from({ length: 3 }, (_, index) => ({
      id: `host-${index + 1}`,
      address: `10.0.2.${index + 1}`,
      status: "online",
      lastHeartbeat: minutesAgo(0.2),
    }));

    const model = buildHostListViewModel({ hosts, page: 2, pageSize: 2, now: NOW });

    expect(model.total).toBe(3);
    expect(model.pageRows.map((row) => row.id)).toEqual(["host-3"]);
    expect(model.canPrev).toBe(true);
    expect(model.canNext).toBe(false);
  });

  it("surfaces install run details and retry action for failed installs", () => {
    const model = buildHostListViewModel({
      hosts: [
        {
          id: "host-failed",
          address: "10.0.9.10",
          sshUser: "deploy",
          status: "install_failed",
          installState: "failed",
          installRunId: "run-install-123",
          installWorkflowId: "builtin.host-agent-install/v1",
          installStep: "verify_heartbeat",
          lastError: "heartbeat timeout",
          transport: "ssh_bootstrap",
        },
      ],
      now: NOW,
    });

    expect(model.rows[0]).toMatchObject({
      id: "host-failed",
      heartbeat: "install_failed",
      heartbeatLabel: "安装失败",
      heartbeatTone: "error",
      installRunId: "run-install-123",
      installWorkflowId: "builtin.host-agent-install/v1",
      installStep: "verify_heartbeat",
      lastError: "heartbeat timeout",
      installDetailLabel: "verify_heartbeat · run-install-123",
      canRetryInstall: true,
      canOpenSsh: false,
      primaryAction: "retry_install",
    });
  });

  it("marks unsupported platform with a dedicated label", () => {
    const model = buildHostListViewModel({
      hosts: [
        {
          id: "host-unsupported",
          address: "10.0.9.11",
          sshUser: "admin",
          status: "install_failed",
          installState: "unsupported_platform",
          lastError: "freebsd/amd64 is not supported",
          transport: "ssh_bootstrap",
        },
      ],
      now: NOW,
    });

    expect(model.rows[0]).toMatchObject({
      heartbeat: "unsupported_platform",
      heartbeatLabel: "不支持的平台",
      heartbeatTone: "error",
      lastError: "freebsd/amd64 is not supported",
      canRetryInstall: true,
      primaryAction: "retry_install",
    });
  });

  it("enables terminal only when remote host is online and terminal capable", () => {
    const hosts = [
      {
        id: "online-terminal",
        address: "10.0.2.15",
        sshUser: "root",
        status: "online",
        transport: "grpc_reverse",
        lastHeartbeat: minutesAgo(0.2),
        terminalCapable: true,
      },
      {
        id: "online-no-terminal",
        address: "10.0.2.16",
        sshUser: "root",
        status: "online",
        transport: "grpc_reverse",
        lastHeartbeat: minutesAgo(0.2),
        terminalCapable: false,
        executable: false,
      },
      {
        id: "manual-installing",
        address: "10.0.2.17",
        sshUser: "deploy",
        sshPort: 22,
        status: "installing",
        transport: "ssh_bootstrap",
        terminalCapable: true,
      },
    ];

    expect(buildHostListViewModel({ hosts, now: NOW }).rows.map((row) => [row.id, row.canOpenSsh])).toEqual([
      ["online-terminal", true],
      ["online-no-terminal", false],
      ["manual-installing", false],
    ]);
  });
});
