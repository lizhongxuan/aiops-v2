import { useEffect, useMemo, useState } from "react";

import { fetchAgentUiArtifacts } from "@/api/agentUiArtifactsClient";
import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

type ArtifactFeedResponse = {
  items?: AiopsTransportAgentUiArtifact[];
  total?: number;
  nextCursor?: string;
};

const filterFields = [
  ["source", "source filter", "source"],
  ["type", "type filter", "type"],
  ["status", "status filter", "status"],
  ["caseId", "caseId filter", "caseId"],
  ["promptTraceId", "promptTraceId filter", "promptTraceId"],
] as const;

export function AgentUICenterPage() {
  const [items, setItems] = useState<AiopsTransportAgentUiArtifact[]>([]);
  const [total, setTotal] = useState(0);
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<AiopsTransportAgentUiArtifact | null>(null);
  const [error, setError] = useState("");

  async function load(nextFilters = filters) {
    try {
      const payload = await fetchAgentUiArtifacts(nextFilters) as ArtifactFeedResponse;
      setItems(payload.items || []);
      setTotal(payload.total ?? payload.items?.length ?? 0);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载 Agent UI 产物失败");
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function updateFilter(key: string, value: string) {
    const next = { ...filters, [key]: value };
    setFilters(next);
    void load(next);
  }

  const metrics = useMemo(() => {
    const warningOrError = items.filter((item) => ["warning", "error", "blocked"].includes(String(item.status))).length;
    const restricted = items.filter((item) => ["restricted", "denied", "forbidden"].includes(String(item.permissionScope))).length;
    const unsupported = items.filter((item) => item.type === "unsupported" || item.originalType).length;
    return { warningOrError, restricted, unsupported };
  }, [items]);

  return (
    <SettingsPageFrame title="Agent UI" description="卡片产物与渲染追踪">
      {error ? <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div> : null}

      <section className="grid gap-3 md:grid-cols-4">
        <Metric label="总计" value={total || items.length} />
        <Metric label="异常" value={metrics.warningOrError} />
        <Metric label="受限" value={metrics.restricted} />
        <Metric label="未支持" value={metrics.unsupported} />
      </section>

      <section className="rounded-lg border bg-white p-3">
        <div className="grid gap-2 md:grid-cols-5">
          {filterFields.map(([key, aria, placeholder]) => (
            <Input
              key={key}
              aria-label={aria}
              value={filters[key] || ""}
              placeholder={placeholder}
              onChange={(event) => updateFilter(key, event.target.value)}
            />
          ))}
        </div>
      </section>

      <section className="rounded-lg border bg-white">
        <div className="border-b px-4 py-3">
          <h2 className="text-sm font-semibold text-slate-950">Agent UI 产物</h2>
        </div>
        <div className="divide-y">
          {items.map((item) => (
            <button
              key={item.id}
              type="button"
              className="grid w-full gap-2 px-4 py-3 text-left hover:bg-slate-50 md:grid-cols-[1.2fr_0.8fr_0.7fr_0.8fr]"
              onClick={() => setSelected(item)}
            >
              <span className="min-w-0">
                <span className="block truncate font-medium text-slate-950">{titleOf(item)}</span>
                <span className="mt-1 block truncate text-xs text-slate-500">{item.id}</span>
              </span>
              <span className="flex flex-wrap gap-1">
                <Badge variant="secondary">{item.type}</Badge>
                {item.source ? <Badge variant="outline">{item.source}</Badge> : null}
              </span>
              <span><ToneBadge tone={statusTone(item.status)}>{item.status || "ready"}</ToneBadge></span>
              <span className="truncate text-xs text-slate-500">{subjectOf(item)}</span>
            </button>
          ))}
        </div>
      </section>

      <Sheet open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
          {selected ? (
            <>
              <SheetHeader>
                <SheetTitle>{titleOf(selected)}</SheetTitle>
                <SheetDescription>{selected.id}</SheetDescription>
              </SheetHeader>
              <div className="grid gap-4 px-4 pb-4">
                <AgentUiArtifactPart artifact={selected} />
                <section className="rounded-lg border bg-slate-50 p-3">
                  <h3 className="text-sm font-semibold">Metadata</h3>
                  <DescriptionRows artifact={selected} />
                </section>
                <section className="rounded-lg border bg-slate-50 p-3">
                  <h3 className="text-sm font-semibold">Actions</h3>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {(selected.actions || []).map((action) => (
                      <a key={String(action.id || action.label)} className="rounded-md border bg-white px-2 py-1 text-xs" href={String(action.href || "#")}>
                        {String(action.label || action.id || "action")}
                      </a>
                    ))}
                    {selected.promptTraceId ? <a className="rounded-md border bg-white px-2 py-1 text-xs" href={`/debug/prompts?trace_id=${encodeURIComponent(selected.promptTraceId)}`}>Prompt Trace {selected.promptTraceId}</a> : null}
                  </div>
                </section>
                <section className="rounded-lg border bg-slate-950 p-3 text-white">
                  <h3 className="text-sm font-semibold">Normalized JSON</h3>
                  <pre className="mt-2 max-h-96 overflow-auto text-xs">{JSON.stringify(selected, null, 2)}</pre>
                </section>
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </SettingsPageFrame>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border bg-white px-4 py-3">
      <div className="text-xs text-slate-500">{label}</div>
      <div className="mt-1 text-xl font-semibold text-slate-950">{value}</div>
    </div>
  );
}

function DescriptionRows({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const rows = [
    ["type", artifact.type],
    ["source", artifact.source],
    ["caseId", artifact.caseId || stringFromRecord(artifact.metadata, "caseId")],
    ["promptTraceId", artifact.promptTraceId || stringFromRecord(artifact.metadata, "promptTraceId")],
    ["updatedAt", artifact.updatedAt],
  ];
  return (
    <dl className="mt-2 grid gap-1 text-xs">
      {rows.map(([key, value]) => value ? (
        <div key={key} className="grid grid-cols-[120px_1fr] gap-2">
          <dt className="text-slate-500">{key}</dt>
          <dd className="break-words font-mono text-slate-800">{value}</dd>
        </div>
      ) : null)}
    </dl>
  );
}

function titleOf(item: AiopsTransportAgentUiArtifact) {
  return item.titleZh || item.title || item.summaryZh || item.summary || item.type;
}

function subjectOf(item: AiopsTransportAgentUiArtifact) {
  const record = item as unknown as Record<string, unknown>;
  const kind = String(record.subjectKind || "");
  const id = String(record.subjectId || "");
  return [kind, id].filter(Boolean).join(" / ") || "-";
}

function statusTone(status?: string): "default" | "success" | "warning" | "danger" {
  if (status === "success" || status === "ready" || status === "passed") return "success";
  if (status === "warning" || status === "blocked") return "warning";
  if (status === "error" || status === "failed") return "danger";
  return "default";
}

function stringFromRecord(value: unknown, key: string) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return "";
  const found = (value as Record<string, unknown>)[key];
  return typeof found === "string" ? found : "";
}
