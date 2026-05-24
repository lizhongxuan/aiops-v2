import { describe, expect, it } from "vitest";
import {
  getRunnerActionCategoryLabel,
  getRunnerActionDescription,
  getRunnerPaletteActions,
} from "./runnerActionPalette";

describe("runnerActionPalette", () => {
  it("keeps only useful interactive workflow nodes in the palette without manual approval", () => {
    const actions = [
      { action: "cmd.run", label: "Command", category: "command", description: "Run a command." },
      { action: "shell.run", label: "Shell Script", category: "script", description: "Run a shell script." },
      { action: "script.shell", label: "Shell Script", category: "script", description: "Run shell script content." },
      { action: "script.python", label: "Python Script", category: "script", description: "Run Python script content." },
      { action: "http.request", label: "HTTP Request", category: "network", description: "Send a governed HTTP request." },
      { action: "builtin.tcp_ping", label: "TCP Ping", category: "network", description: "Check TCP reachability." },
      { action: "builtin.dns_resolve", label: "DNS Resolve", category: "network", description: "Resolve DNS records." },
      { action: "condition.evaluate", label: "Condition", category: "control", description: "Branch by expression." },
      { action: "manual.approval", label: "Manual Approval", category: "control", description: "Pause for approval." },
      { action: "notify.send", label: "Notify", category: "control", description: "Send a notification." },
      { action: "variable.aggregate", label: "Variable Aggregator", category: "control", description: "Aggregate variables." },
      { action: "wait.event", label: "Wait For Event", category: "control", description: "Not ready for useful canvas runs." },
      { action: "workflow.run", label: "Subflow", category: "control", description: "Needs a child workflow." },
    ];

    expect(getRunnerPaletteActions(actions).map((action) => action.action)).toEqual([
      "script.shell",
      "script.python",
      "http.request",
      "builtin.tcp_ping",
      "builtin.dns_resolve",
      "condition.evaluate",
    ]);
  });

  it("uses functional descriptions instead of raw category names for cards", () => {
    expect(getRunnerActionDescription({ action: "cmd.run", category: "command" })).toContain("命令");
    expect(getRunnerActionDescription({ action: "cmd.run", category: "command" })).not.toBe("command");
    expect(getRunnerActionCategoryLabel({ action: "condition.evaluate", category: "control" })).toBe("逻辑");
    expect(getRunnerActionCategoryLabel({ action: "http.request", category: "network" })).toBe("网络");
  });

  it("uses localized known-node descriptions even when the backend catalog is English", () => {
    expect(
      getRunnerActionDescription({
        action: "cmd.run",
        category: "command",
        description: "Run a shell command through /bin/sh -c on each target.",
      }),
    ).toBe("执行单条命令，适合检查、查询和轻量操作。");
  });
});
