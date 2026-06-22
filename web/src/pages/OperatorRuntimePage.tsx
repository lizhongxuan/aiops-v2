import { RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  approveRun,
  createAction,
  createInspectionTemplate,
  createProblemType,
  createResource,
  createRule,
  createWorkflowBinding,
  enableRule,
  getRun,
  listActions,
  listInspectionTemplates,
  listProblemTypes,
  listResources,
  listRules,
  listRuns,
  listWorkflowBindings,
  rejectRun,
} from "@/api/operatorRuntime";
import type { OperatorRuntimeItem } from "@/api/operatorRuntime";
import { GuardRunPanel } from "@/components/operator-runtime/GuardRunPanel";
import { createRulePayload, itemId } from "@/components/operator-runtime/operatorRuntimeModels";
import type { RuntimeCollectionKey, RuntimeCollections } from "@/components/operator-runtime/operatorRuntimeModels";
import { RuntimeSetupForms } from "@/components/operator-runtime/RuntimeSetupForms";
import type { ProvisionStepKey } from "@/components/operator-runtime/SampleProvisioningPanel";
import { SampleProvisioningPanel } from "@/components/operator-runtime/SampleProvisioningPanel";
import { RowButton, RuntimeDataTable } from "@/components/operator-runtime/RuntimeDataTable";
import { Button } from "@/components/ui/button";
import { SettingsPageFrame, StatGrid, StatusAlert } from "@/pages/settingsComponents";

const emptyCollections: RuntimeCollections = {
  resources: [],
  inspectionTemplates: [],
  problemTypes: [],
  actions: [],
  workflowBindings: [],
  rules: [],
};

type Notice = { type: "success" | "error" | "info"; title: string; message: string };

export function OperatorRuntimePage() {
  const [collections, setCollections] = useState<RuntimeCollections>(emptyCollections);
  const [runs, setRuns] = useState<OperatorRuntimeItem[]>([]);
  const [selectedRun, setSelectedRun] = useState<OperatorRuntimeItem | undefined>();
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState<Notice | null>(null);

  const completed = useMemo(
    () => ({
      resource: collections.resources.length > 0,
      inspectionTemplate: collections.inspectionTemplates.length > 0,
      problemType: collections.problemTypes.length > 0,
      action: collections.actions.length > 0,
      workflowBinding: collections.workflowBindings.length > 0,
      rule: collections.rules.some((rule) => rule.enabled === true || rule.status === "enabled" || rule.state === "enabled"),
    }),
    [collections],
  );

  async function refresh() {
    const [resources, inspectionTemplates, problemTypes, actions, workflowBindings, rules, guardRuns] = await Promise.all([
      listResources(),
      listInspectionTemplates(),
      listProblemTypes(),
      listActions(),
      listWorkflowBindings(),
      listRules(),
      listRuns(),
    ]);
    setCollections({
      resources: resources.items,
      inspectionTemplates: inspectionTemplates.items,
      problemTypes: problemTypes.items,
      actions: actions.items,
      workflowBindings: workflowBindings.items,
      rules: rules.items,
    });
    setRuns(guardRuns.items);
    setSelectedRun((current) => current ?? guardRuns.items[0]);
  }

  useEffect(() => {
    setBusy("load");
    refresh()
      .catch((error) => setNotice({ type: "error", title: "加载失败", message: errorMessage(error) }))
      .finally(() => setBusy(""));
  }, []);

  async function createSample(key: ProvisionStepKey, payload: unknown) {
    setBusy(key);
    setNotice(null);
    try {
      if (key === "resource") {
        await createCollectionItem("resources", payload);
      } else if (key === "inspectionTemplate") {
        await createCollectionItem("inspectionTemplates", payload);
      } else if (key === "problemType") {
        await createCollectionItem("problemTypes", payload);
      } else if (key === "action") {
        await createCollectionItem("actions", payload);
      } else if (key === "workflowBinding") {
        await createCollectionItem("workflowBindings", payload);
      } else if (key === "rule") {
        await createAndEnableRule();
        return;
      }
      setNotice({ type: "success", title: "已创建", message: `${stepLabel(key)} 已写入运行时。` });
    } catch (error) {
      setNotice({ type: "error", title: "创建失败", message: errorMessage(error) });
    } finally {
      setBusy("");
    }
  }

  async function createCollectionItem(key: RuntimeCollectionKey, payload: unknown) {
    if (key === "resources") {
      const result = await createResource(payload);
      addItem("resources", result.item);
    } else if (key === "inspectionTemplates") {
      const result = await createInspectionTemplate(payload);
      addItem("inspectionTemplates", result.item);
    } else if (key === "problemTypes") {
      const result = await createProblemType(payload);
      addItem("problemTypes", result.item);
    } else if (key === "actions") {
      const result = await createAction(payload);
      addItem("actions", result.item);
    } else if (key === "workflowBindings") {
      const result = await createWorkflowBinding(payload);
      addItem("workflowBindings", result.item);
    }
  }

  async function createFromForm(key: RuntimeCollectionKey, payload: unknown) {
    setBusy(key);
    setNotice(null);
    try {
      await createCollectionItem(key, payload);
      setNotice({ type: "success", title: "已保存", message: `${collectionLabel(key)} 已写入运行时。` });
    } catch (error) {
      setNotice({ type: "error", title: "保存失败", message: errorMessage(error) });
    } finally {
      setBusy("");
    }
  }

  async function createAndEnableRule(payload?: unknown) {
    const body = payload ?? createRulePayload({
      resource: collections.resources[0],
      inspectionTemplate: collections.inspectionTemplates[0],
      problemType: collections.problemTypes[0],
      action: collections.actions[0],
      workflowBinding: collections.workflowBindings[0],
    });
    const created = await createRule(body);
    const ruleId = itemId(created.item, String((body as OperatorRuntimeItem).name ?? ""));
    const enabled = await enableRule(ruleId);
    addItem("rules", enabled.item);
    setNotice({ type: "success", title: "守护规则已启用", message: `${ruleId} 已进入启用状态。` });
  }

  async function createRuleFromForm(payload: unknown) {
    setBusy("rule");
    setNotice(null);
    try {
      await createAndEnableRule(payload);
    } catch (error) {
      setNotice({ type: "error", title: "守护规则保存失败", message: errorMessage(error) });
    } finally {
      setBusy("");
    }
  }

  function addItem(key: keyof RuntimeCollections, item: OperatorRuntimeItem) {
    setCollections((current) => {
      const id = itemId(item);
      const nextItems = id ? current[key].filter((existing) => itemId(existing) !== id) : current[key];
      return { ...current, [key]: [...nextItems, item] };
    });
  }

  async function selectRun(run: OperatorRuntimeItem) {
    const id = itemId(run);
    setSelectedRun(run);
    if (!id) return;
    setBusy(`run-${id}`);
    try {
      const detail = await getRun(id);
      setSelectedRun(detail.item);
    } catch (error) {
      setNotice({ type: "error", title: "GuardRun 详情加载失败", message: errorMessage(error) });
    } finally {
      setBusy("");
    }
  }

  async function decideRun(run: OperatorRuntimeItem, decision: "approve" | "reject") {
    const id = itemId(run);
    if (!id) return;
    setBusy(`${decision}-${id}`);
    try {
      const result = decision === "approve" ? await approveRun(id) : await rejectRun(id);
      setSelectedRun(result.item);
      setRuns((current) => current.map((item) => (itemId(item) === id ? result.item : item)));
      setNotice({ type: "success", title: decision === "approve" ? "已批准" : "已拒绝", message: `${id} 决策已提交。` });
    } catch (error) {
      setNotice({ type: "error", title: "决策提交失败", message: errorMessage(error) });
    } finally {
      setBusy("");
    }
  }

  return (
    <SettingsPageFrame
      title="自愈 Operator"
      description="通用自愈 Operator Runtime：把中间件或服务资源、巡检模板、问题类型、动作和 Workflow 绑定成可审计的自愈规则。"
      actions={
        <Button type="button" variant="outline" size="sm" disabled={Boolean(busy)} onClick={() => void refresh()}>
          <RefreshCw className="h-4 w-4" />
          刷新
        </Button>
      }
      contentClassName="max-w-[1500px]"
    >
      <StatGrid
        items={[
          { label: "受管资源", value: collections.resources.length },
          { label: "守护规则", value: collections.rules.length, tone: completed.rule ? "ok" : "warn" },
          { label: "GuardRun", value: runs.length },
          { label: "审批状态", value: selectedRun ? String(selectedRun.status ?? selectedRun.state ?? "pending") : "无" },
        ]}
      />
      {notice ? <StatusAlert type={notice.type} title={notice.title} message={notice.message} /> : null}
      <SampleProvisioningPanel busy={busy} completed={completed} onCreate={(key, payload) => void createSample(key, payload)} />
      <RuntimeSetupForms
        busy={busy}
        collections={collections}
        onCreate={(key, payload) => void createFromForm(key, payload)}
        onCreateRule={(payload) => void createRuleFromForm(payload)}
      />
      <div className="grid gap-3 xl:grid-cols-2">
        <RuntimeDataTable
          title="受管资源"
          description="Guard Operator 可管理的中间件或服务目标，PostgreSQL 是其中一个示例。"
          rows={collections.resources}
          columns={["id", "name", "kind", "endpoints"]}
        />
        <RuntimeDataTable
          title="巡检模板"
          description="运行时巡检项与调度配置。"
          rows={collections.inspectionTemplates}
          columns={["id", "name", "intervalSeconds"]}
        />
        <RuntimeDataTable
          title="问题类型"
          description="巡检信号归类后的治理问题。"
          rows={collections.problemTypes}
          columns={["id", "displayName", "severity"]}
        />
        <RuntimeDataTable
          title="动作库"
          description="可被守护规则引用的动作。"
          rows={collections.actions}
          columns={["id", "displayName", "riskLevel"]}
        />
        <RuntimeDataTable
          title="Workflow 绑定"
          description="动作到 Runner Workflow 的绑定关系。"
          rows={collections.workflowBindings}
          columns={["id", "workflowRef", "actionRef"]}
        />
        <RuntimeDataTable
          title="守护规则"
          description="连接资源、模板、问题类型、动作与 Workflow 绑定。"
          rows={collections.rules}
          columns={["id", "name", "enabled", "resourceRef"]}
          action={(row) => (
            <RowButton
              row={row}
              label="启用"
              disabled={Boolean(busy)}
              onClick={(rule) => {
                setBusy(`enable-${itemId(rule)}`);
                enableRule(itemId(rule))
                  .then((result) => addItem("rules", result.item))
                  .catch((error) => setNotice({ type: "error", title: "启用失败", message: errorMessage(error) }))
                  .finally(() => setBusy(""));
              }}
            />
          )}
        />
      </div>
      <GuardRunPanel
        runs={runs}
        selectedRun={selectedRun}
        busy={busy}
        onSelectRun={(run) => void selectRun(run)}
        onApprove={(run) => void decideRun(run, "approve")}
        onReject={(run) => void decideRun(run, "reject")}
      />
    </SettingsPageFrame>
  );
}

function stepLabel(key: ProvisionStepKey) {
  return {
    resource: "受管资源",
    inspectionTemplate: "巡检模板",
    problemType: "问题类型",
    action: "动作",
    workflowBinding: "Workflow 绑定",
    rule: "守护规则",
  }[key];
}

function collectionLabel(key: RuntimeCollectionKey) {
  return {
    resources: "受管资源",
    inspectionTemplates: "巡检模板",
    problemTypes: "问题类型",
    actions: "动作",
    workflowBindings: "Workflow 绑定",
    rules: "守护规则",
  }[key];
}

function errorMessage(error: unknown) {
  if (error && typeof error === "object" && "fieldErrors" in error && Array.isArray(error.fieldErrors)) {
    const details = error.fieldErrors
      .map((item) => {
        if (!item || typeof item !== "object") return "";
        const field = "field" in item ? String(item.field) : "";
        const message = "message" in item ? String(item.message) : "";
        return field && message ? `${field}: ${message}` : message;
      })
      .filter(Boolean)
      .join("\n");
    const message = error instanceof Error ? error.message : "请求失败";
    return details ? `${message}\n${details}` : message;
  }
  return error instanceof Error ? error.message : String(error);
}
