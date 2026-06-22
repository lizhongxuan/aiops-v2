// @ts-check
import { expect, test } from "@playwright/test";

test.describe("Operator Runtime", () => {
  test("creates and enables a PG guard rule, then approves a GuardRun", async ({ page }) => {
    const postedProblemBodies = [];
    const postedRuleBodies = [];
    const state = {
      resources: [],
      templates: [],
      problemTypes: [],
      actions: [],
      bindings: [],
      rules: [],
      runs: [
        {
          id: "guard-run-1",
          name: "guard-run-1",
          status: "pending_approval",
          ruleName: "pg-runtime-autoheal-rule",
          resourceName: "postgres-prod-primary",
          problemTypeId: "pg.replication.receiver_stopped",
          actionRef: "postgres.replication.reconnect_replica.v1",
          evidence: { "replica.receiverRunning": false, recommendation: "approve workflow" },
          createdAt: "2026-06-22T01:00:00Z",
        },
      ],
    };

    await page.route("**/api/v1/guards/**", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname;
      const method = request.method();

      if (method === "GET" && path === "/api/v1/guards/resources") return route.fulfill({ json: { items: state.resources } });
      if (method === "GET" && path === "/api/v1/guards/inspection-templates") return route.fulfill({ json: { items: state.templates } });
      if (method === "GET" && path === "/api/v1/guards/problem-types") return route.fulfill({ json: { items: state.problemTypes } });
      if (method === "GET" && path === "/api/v1/guards/actions") return route.fulfill({ json: { items: state.actions } });
      if (method === "GET" && path === "/api/v1/guards/workflow-bindings") return route.fulfill({ json: { items: state.bindings } });
      if (method === "GET" && path === "/api/v1/guards/rules") return route.fulfill({ json: { items: state.rules } });
      if (method === "GET" && path === "/api/v1/guards/runs") return route.fulfill({ json: { items: state.runs } });

      const runMatch = path.match(/^\/api\/v1\/guards\/runs\/([^/]+)(?:\/(approve|reject))?$/);
      if (runMatch) {
        const id = decodeURIComponent(runMatch[1]);
        const decision = runMatch[2];
        const run = state.runs.find((item) => item.id === id) || state.runs[0];
        if (method === "POST" && decision) {
          run.status = decision === "approve" ? "approved" : "rejected";
          return route.fulfill({ json: { item: run } });
        }
        return route.fulfill({ json: { item: run } });
      }

      const ruleStateMatch = path.match(/^\/api\/v1\/guards\/rules\/([^/]+)\/(enable|disable)$/);
      if (method === "POST" && ruleStateMatch) {
        const id = decodeURIComponent(ruleStateMatch[1]);
        const rule = state.rules.find((item) => item.id === id) || { id, name: id };
        rule.enabled = ruleStateMatch[2] === "enable";
        rule.status = rule.enabled ? "enabled" : "disabled";
        state.rules = state.rules.filter((item) => item.id !== id).concat(rule);
        return route.fulfill({ json: { item: rule } });
      }

      if (method === "POST" && path === "/api/v1/guards/resources") {
        const body = request.postDataJSON();
        const item = { id: "managed-resource-1", ...body };
        state.resources.push(item);
        return route.fulfill({ json: { item } });
      }
      if (method === "POST" && path === "/api/v1/guards/inspection-templates") {
        const body = request.postDataJSON();
        const item = { id: "inspection-template-1", ...body };
        state.templates.push(item);
        return route.fulfill({ json: { item } });
      }
      if (method === "POST" && path === "/api/v1/guards/problem-types") {
        const body = request.postDataJSON();
        postedProblemBodies.push(body);
        const item = { id: "problem-type-1", ...body };
        state.problemTypes.push(item);
        return route.fulfill({ json: { item } });
      }
      if (method === "POST" && path === "/api/v1/guards/actions") {
        const body = request.postDataJSON();
        const item = { id: "action-1", ...body };
        state.actions.push(item);
        return route.fulfill({ json: { item } });
      }
      if (method === "POST" && path === "/api/v1/guards/workflow-bindings") {
        const body = request.postDataJSON();
        const item = { id: "workflow-binding-1", ...body };
        state.bindings.push(item);
        return route.fulfill({ json: { item } });
      }
      if (method === "POST" && path === "/api/v1/guards/rules") {
        const body = request.postDataJSON();
        postedRuleBodies.push(body);
        const item = { id: "guard-rule-1", status: "disabled", ...body };
        state.rules.push(item);
        return route.fulfill({ json: { item } });
      }

      return route.fulfill({ status: 404, json: { error: `unhandled ${method} ${path}` } });
    });

    await page.goto("/operator-runtime");

    await expect(page.locator("main")).toContainText("通用自愈 Operator Runtime");
    await expect(page.getByTestId("operator-runtime-host")).toHaveValue("120.77.239.90");

    await page.getByTestId("operator-runtime-resource").click();
    await expect(page.locator("main")).toContainText("postgres-prod-primary");
    await expect(page.locator("main")).toContainText("120.77.239.90");

    await page.getByTestId("operator-runtime-inspectionTemplate").click();
    await expect(page.locator("main")).toContainText("postgres.replication.basic.v1");

    await page.getByRole("tab", { name: "问题类型" }).click();
    await page.getByTestId("operator-runtime-problem-preset").selectOption("receiver_stopped");
    await page.getByTestId("operator-runtime-problem-save").click();
    await expect(page.locator("main")).toContainText("pg.replication.receiver_stopped");
    expect(postedProblemBodies[0]).toMatchObject({
      id: "pg.replication.receiver_stopped",
      displayName: "PG WAL receiver 停止",
      recommendedActionRefs: ["postgres.replication.reconnect_replica.v1"],
    });
    expect(postedProblemBodies[0].conditions).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ field: "replica.receiverRunning", operator: "==", value: expect.objectContaining({ bool: false }) }),
      ]),
    );

    await page.getByTestId("operator-runtime-action").click();
    await expect(page.locator("main")).toContainText("postgres.replication.reconnect_replica.v1");

    await page.getByTestId("operator-runtime-workflowBinding").click();
    await expect(page.locator("main")).toContainText("builtin.postgres.replication_reconnect_replica.v1");

    await page.getByTestId("operator-runtime-rule").click();
    await expect(page.locator("main")).toContainText("pg-runtime-autoheal-rule");
    await expect(page.locator("main")).toContainText("enabled");
    expect(postedRuleBodies[0].resourceRef).toBe("postgres-prod-primary");
    expect(postedRuleBodies[0].problemTypeRefs).toEqual(["pg.replication.receiver_stopped"]);

    await page.getByTestId("operator-runtime-run-guard-run-1").click();
    await expect(page.locator("main")).toContainText("pending_approval");
    await page.getByTestId("operator-runtime-approve-run").click();
    await expect(page.locator("main")).toContainText("approved");
  });
});
