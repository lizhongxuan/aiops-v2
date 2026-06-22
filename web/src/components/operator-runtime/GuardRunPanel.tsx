import { Check, X } from "lucide-react";

import type { OperatorRuntimeItem } from "@/api/operatorRuntime";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ToneBadge } from "@/pages/settingsComponents";

import { itemId, itemLabel, runStatus, valueText } from "./operatorRuntimeModels";

export function GuardRunPanel({
  runs,
  selectedRun,
  onSelectRun,
  onApprove,
  onReject,
  busy,
}: {
  runs: OperatorRuntimeItem[];
  selectedRun?: OperatorRuntimeItem;
  onSelectRun: (run: OperatorRuntimeItem) => void;
  onApprove: (run: OperatorRuntimeItem) => void;
  onReject: (run: OperatorRuntimeItem) => void;
  busy?: string;
}) {
  const selectedId = itemId(selectedRun);

  return (
    <Card className="rounded-lg bg-white" size="sm">
      <CardHeader className="pb-0">
        <CardTitle>GuardRun</CardTitle>
        <CardDescription>查看运行记录并处理需要人工确认的自愈动作。</CardDescription>
        <CardAction>
          <ToneBadge>{runs.length}</ToneBadge>
        </CardAction>
      </CardHeader>
      <CardContent className="grid gap-3 lg:grid-cols-[minmax(260px,0.9fr)_1.1fr]">
        <div className="overflow-hidden rounded-lg border">
          {runs.length ? (
            <div className="max-h-[360px] overflow-auto">
              {runs.map((run, index) => {
                const id = itemId(run, `run-${index}`);
                return (
                  <button
                    key={id}
                    type="button"
                    data-testid={`operator-runtime-run-${id}`}
                    className={`grid w-full gap-1 border-b px-3 py-2 text-left text-sm hover:bg-slate-50 ${
                      selectedId === id ? "bg-slate-100" : "bg-white"
                    }`}
                    onClick={() => onSelectRun(run)}
                  >
                    <span className="font-medium text-slate-900">{itemLabel(run, id)}</span>
                    <span className="text-xs text-slate-500">
                      {runStatus(run)} · {valueText(run.resourceName ?? run.resourceId ?? run.clusterName ?? run.clusterId ?? run.ruleName)}
                    </span>
                  </button>
                );
              })}
            </div>
          ) : (
            <div className="px-3 py-8 text-center text-sm text-slate-500">暂无 GuardRun</div>
          )}
        </div>
        <div className="rounded-lg border bg-slate-50 p-3">
          {selectedRun ? (
            <div className="grid gap-3">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div>
                  <div className="text-sm font-medium text-slate-900">{itemLabel(selectedRun)}</div>
                  <div className="text-xs text-slate-500">{itemId(selectedRun)}</div>
                </div>
                <ToneBadge tone={runStatus(selectedRun).includes("approved") ? "success" : "warning"}>
                  {runStatus(selectedRun)}
                </ToneBadge>
              </div>
              <dl className="grid grid-cols-2 gap-2 text-xs">
                {["ruleName", "resourceName", "resourceId", "problemType", "createdAt", "updatedAt"].map((key) => (
                  <div key={key} className="rounded-md bg-white p-2 ring-1 ring-slate-200">
                    <dt className="text-slate-500">{key}</dt>
                    <dd className="mt-1 break-words font-medium text-slate-800">{valueText(selectedRun[key]) || "-"}</dd>
                  </div>
                ))}
              </dl>
              <pre className="max-h-44 overflow-auto rounded-lg bg-slate-950 p-3 text-xs text-slate-100">
                {JSON.stringify(selectedRun, null, 2)}
              </pre>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  size="sm"
                  data-testid="operator-runtime-approve-run"
                  disabled={Boolean(busy)}
                  onClick={() => onApprove(selectedRun)}
                >
                  <Check className="h-4 w-4" />
                  Approve
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="destructive"
                  data-testid="operator-runtime-reject-run"
                  disabled={Boolean(busy)}
                  onClick={() => onReject(selectedRun)}
                >
                  <X className="h-4 w-4" />
                  Reject
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex min-h-[260px] items-center justify-center text-sm text-slate-500">选择一条 GuardRun 查看详情</div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
