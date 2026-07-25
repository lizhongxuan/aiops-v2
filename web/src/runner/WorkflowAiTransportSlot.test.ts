import { describe, expect, it } from "vitest";

import { selectWorkflowEnvironmentBinding, workflowEnvironmentDigestInput } from "./WorkflowAiTransportSlot";

describe("workflowEnvironmentDigestInput", () => {
  it("matches Runner's ordered environment binding input without inventing values", () => {
    const result = workflowEnvironmentDigestInput({
      workflow: {
        validation_env: "staging",
        env_packages: ["curl", "jq"],
        inventory: {
          groups: { web: { hosts: ["web-1"], vars: { zone: "a" } } },
          hosts: { "web-1": { address: "10.0.0.1", vars: null } },
          vars: { region: "cn-south" },
        },
      },
    });

    expect(result).toEqual({
      name: "staging",
      payload: '["staging",["curl","jq"],{"groups":{"web":{"hosts":["web-1"],"vars":{"zone":"a"}}},"hosts":{"web-1":{"address":"10.0.0.1","vars":null}},"vars":{"region":"cn-south"}}]',
    });
  });

  it("keeps absent environment fields explicit so the digest matches a zero-value Runner model", () => {
    expect(workflowEnvironmentDigestInput({ workflow: {} })).toEqual({
      name: "默认环境",
      payload: '["",null,{"groups":null,"hosts":null,"vars":null}]',
    });
  });
});

describe("selectWorkflowEnvironmentBinding", () => {
  it("uses the server-owned committed binding instead of a browser digest", () => {
    expect(selectWorkflowEnvironmentBinding(
      { name: "staging", bindingVersion: "server-basis" },
      { name: "staging", bindingVersion: "browser-digest" },
    )).toEqual({ name: "staging", bindingVersion: "server-basis" });
  });

  it("keeps an explicitly unavailable server binding empty", () => {
    expect(selectWorkflowEnvironmentBinding(
      { bindingVersion: "" },
      { name: "staging", bindingVersion: "browser-digest" },
    )).toEqual({ bindingVersion: "" });
  });
});
