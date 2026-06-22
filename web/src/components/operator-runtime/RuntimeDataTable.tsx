import { CheckCircle2, CircleAlert } from "lucide-react";
import type { ReactNode } from "react";

import type { OperatorRuntimeItem } from "@/api/operatorRuntime";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ToneBadge } from "@/pages/settingsComponents";

import { itemId, itemLabel, ruleEnabled, valueText } from "./operatorRuntimeModels";

export function RuntimeDataTable({
  title,
  description,
  rows,
  columns,
  action,
  emptyLabel = "暂无数据",
}: {
  title: string;
  description: string;
  rows: OperatorRuntimeItem[];
  columns: string[];
  action?: (row: OperatorRuntimeItem) => ReactNode;
  emptyLabel?: string;
}) {
  return (
    <Card className="rounded-lg bg-white" size="sm">
      <CardHeader className="pb-0">
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
        <CardAction>
          <ToneBadge>{rows.length}</ToneBadge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {rows.length ? (
          <div className="overflow-auto rounded-lg border">
            <table className="w-full min-w-[520px] border-collapse text-left text-xs">
              <thead className="bg-slate-50 text-slate-500">
                <tr>
                  {columns.map((column) => (
                    <th key={column} className="border-b px-3 py-2 font-medium">
                      {column}
                    </th>
                  ))}
                  {action ? <th className="border-b px-3 py-2 font-medium">操作</th> : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => (
                  <tr key={itemId(row, `row-${index}`)} className="align-top">
                    {columns.map((column) => (
                      <td key={column} className="max-w-[240px] break-words border-b px-3 py-2 text-slate-700">
                        {column === "enabled" ? <RuleState enabled={ruleEnabled(row)} /> : valueText(row[column])}
                      </td>
                    ))}
                    {action ? <td className="border-b px-3 py-2">{action(row)}</td> : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="rounded-lg border border-dashed px-3 py-6 text-center text-sm text-slate-500">{emptyLabel}</div>
        )}
      </CardContent>
    </Card>
  );
}

export function RuleState({ enabled }: { enabled: boolean }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      {enabled ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-600" /> : <CircleAlert className="h-3.5 w-3.5 text-amber-600" />}
      {enabled ? "enabled" : "disabled"}
    </span>
  );
}

export function RowButton({
  row,
  label,
  onClick,
  disabled,
}: {
  row: OperatorRuntimeItem;
  label: string;
  onClick: (row: OperatorRuntimeItem) => void;
  disabled?: boolean;
}) {
  return (
    <Button size="sm" variant="outline" type="button" disabled={disabled} onClick={() => onClick(row)}>
      {label || itemLabel(row)}
    </Button>
  );
}
