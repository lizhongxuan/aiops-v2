import { describe, expect, it } from "vitest";
import {
  buildHostTerminalEntry,
  buildHostProfileDetail,
  buildHostExecutionRisks,
  buildHostProfileRows,
  normalizeHostLease,
  normalizeHostProfile,
} from "./hostProfileViewModels";

describe("hostProfileViewModels", () => {
  it("normalizes HostProfile fields with Chinese labels and safe fallbacks", () => {
    const profile = normalizeHostProfile({
      host_id: "host-1",
      display_name: "prod-web-1",
      status: "online",
      os: "linux",
      arch: "x86_64",
      labels: { env: "prod", role: "web" },
      last_heartbeat_at: "2026-05-12T02:00:00Z",
      profile_expires_at: "2026-05-12T03:00:00Z",
    });

    expect(profile).toMatchObject({
      hostId: "host-1",
      displayName: "prod-web-1",
      statusLabel: "在线",
      statusTone: "success",
      osLabel: "Linux",
      archLabel: "x86_64",
      envLabel: "prod",
      roleLabel: "web",
      labelsText: "env=prod, role=web",
    });
  });

  it("normalizes HostLease status and encodes ownership labels", () => {
    const lease = normalizeHostLease({
      lease_id: "lease-1",
      host_id: "host-1",
      status: "active",
      mission_id: "mission-1",
      owner_session_id: "session-1",
      expires_at: "2026-05-12T03:00:00Z",
    });

    expect(lease).toMatchObject({
      leaseId: "lease-1",
      hostId: "host-1",
      statusLabel: "占用中",
      statusTone: "warning",
      missionLabel: "mission-1",
      ownerSessionLabel: "session-1",
    });
  });

  it("builds HostProfile rows with conflict and lease context", () => {
    const rows = buildHostProfileRows({
      profiles: [
        {
          host_id: "host-1",
          display_name: "prod-web-1",
          status: "online",
          os: "linux",
          arch: "x86_64",
          labels: { env: "prod", role: "web" },
        },
        {
          host_id: "host-1",
          display_name: "duplicate-web",
          status: "online",
          os: "linux",
          arch: "arm64",
          labels: { env: "prod" },
        },
      ],
      leases: [{ lease_id: "lease-1", host_id: "host-1", status: "active", mission_id: "mission-1" }],
    });

    expect(rows).toHaveLength(2);
    expect(rows[0]).toMatchObject({
      hostId: "host-1",
      displayName: "prod-web-1",
      activeLeaseCount: 1,
      riskCount: 1,
    });
    expect(rows[0].riskKeys).toContain("host_id_conflict");
  });

  it("covers offline, expired profile, missing environment label, host id conflict, and platform mismatch risks", () => {
    const risks = buildHostExecutionRisks({
      profiles: [
        {
          host_id: "host-1",
          status: "offline",
          os: "linux",
          arch: "arm64",
          labels: { role: "web" },
          profile_expires_at: "2026-05-12T01:00:00Z",
        },
        {
          host_id: "host-1",
          status: "online",
          os: "darwin",
          arch: "x86_64",
          labels: { env: "prod" },
        },
      ],
      requiredOs: "linux",
      requiredArch: "x86_64",
      now: "2026-05-12T02:00:00Z",
    });

    expect(risks.map((risk) => risk.key)).toEqual([
      "host_offline",
      "profile_expired",
      "missing_env_label",
      "host_id_conflict",
      "platform_mismatch",
      "platform_mismatch",
    ]);
    expect(risks.map((risk) => risk.label)).toEqual([
      "客户端离线",
      "HostProfile 已过期",
      "环境标签缺失",
      "host_id 冲突",
      "OS/架构不匹配",
      "OS/架构不匹配",
    ]);
    expect(risks.every((risk) => risk.message.length > 0 && risk.tone === "danger")).toBe(true);
  });

  it("builds a safe HostProfile detail without tokens, passwords, cookies, private keys, authorization, or request bodies", () => {
    const [row] = buildHostProfileRows({
      profiles: [
        {
          host_id: "host-1",
          display_name: "prod-web-1",
          status: "online",
          os: "linux",
          arch: "x86_64",
          labels: { env: "prod", role: "web" },
          agent_version: "1.8.4",
          agent_id: "agent-prod-1",
          runtime: { language: "node", version: "22.11.0", request_body: "card-number=4111111111111111" },
          service_runtime: { supervisor: "systemd", unit: "aiops-agent.service" },
          env: { PATH: "/usr/bin", API_TOKEN: "secret-token" },
          password: "secret-password",
          private_key: "-----BEGIN PRIVATE KEY-----",
          cookie: "sid=secret-cookie",
          authorization: "Bearer secret-auth",
        },
      ],
    });

    const detail = buildHostProfileDetail({
      profile: row,
      leases: [
        {
          lease_id: "lease-1",
          host_id: "host-1",
          status: "active",
          mission_id: "case-1",
          owner_session_id: "session-1",
          expires_at: "2026-05-12T03:00:00Z",
        },
      ],
      reports: [
        {
          report_id: "report-1",
          host_id: "host-1",
          status: "accepted",
          summary: "last run ok",
        },
      ],
    });

    expect(detail.sections.map((section) => section.title)).toEqual([
      "基础信息",
      "运行环境",
      "已安装 Agent",
      "service runtime",
      "最近 Case",
      "当前 HostLease",
    ]);
    expect(detail.sections.flatMap((section) => section.items).map((item) => `${item.label}:${item.value}`)).toContain(
      "Agent 版本:1.8.4",
    );
    expect(JSON.stringify(detail)).toContain("aiops-agent.service");
    expect(JSON.stringify(detail)).toContain("case-1");
    expect(JSON.stringify(detail)).not.toMatch(/secret-token|secret-password|PRIVATE KEY|secret-cookie|secret-auth|4111111111111111/i);
  });

  it("opens independent host terminals only for online terminal-capable or executable hosts", () => {
    expect(buildHostTerminalEntry({ id: "host-1", status: "online", terminalCapable: true })).toMatchObject({
      canOpenTerminal: true,
      disabledReason: "",
    });
    expect(buildHostTerminalEntry({ id: "host-2", status: "online", executable: true })).toMatchObject({
      canOpenTerminal: true,
      disabledReason: "",
    });
    expect(buildHostTerminalEntry({ id: "host-2b", status: "stale", terminalCapable: true })).toMatchObject({
      canOpenTerminal: true,
      disabledReason: "",
    });
    expect(buildHostTerminalEntry({ id: "host-3", status: "offline", terminalCapable: true })).toMatchObject({
      canOpenTerminal: false,
      disabledReason: "主机离线",
    });
    expect(buildHostTerminalEntry({ id: "host-4", status: "online" })).toMatchObject({
      canOpenTerminal: false,
      disabledReason: "主机未启用终端",
    });
  });
});
