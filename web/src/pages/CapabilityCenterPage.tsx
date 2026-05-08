import { useEffect, useState } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";

type CatalogItem = Record<string, unknown>;

async function requestJson<T>(path: string): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include" });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

function DataTable({ rows }: { rows: CatalogItem[] }) {
  const keys = Array.from(new Set(rows.flatMap((row) => Object.keys(row)))).slice(0, 6);
  return (
    <div className="ops-data-table-table overflow-auto">
      <table className="data-table w-full border-collapse text-sm">
        <tbody>
          {rows.map((row, index) => (
            <tr key={String(row.id || row.name || index)}>
              {keys.map((key) => <td key={key} className="border-b p-2">{typeof row[key] === "boolean" ? String(row[key]) : String(row[key] ?? "")}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function CapabilityCenterPage() {
  const [tab, setTab] = useState("skills");
  const [skills, setSkills] = useState<CatalogItem[]>([]);
  const [mcps, setMcps] = useState<CatalogItem[]>([]);
  const [bindings, setBindings] = useState<CatalogItem[]>([]);

  useEffect(() => {
    async function load() {
      const [snapshot, bindingPayload] = await Promise.all([
        requestJson<{ skillCatalog?: CatalogItem[]; mcpCatalog?: CatalogItem[] }>("/api/v1/snapshot"),
        requestJson<{ items?: CatalogItem[] }>("/api/v1/capability-bindings"),
      ]);
      setSkills(snapshot.skillCatalog || []);
      setMcps(snapshot.mcpCatalog || []);
      setBindings(bindingPayload.items || []);
    }
    void load().catch(() => undefined);
  }, []);

  return (
    <SettingsPageFrame title="能力中心" description="集中查看 Skills、MCP Servers 与 Capability Bindings。">
      <div className="flex flex-wrap gap-2">
        {[
          ["skills", "Skills"],
          ["mcps", "MCP Servers"],
          ["bindings", "Bindings"],
        ].map(([key, label]) => (
          <button key={key} className={`ops-tabs-tab rounded-lg border px-3 py-2 text-sm ${tab === key ? "active bg-slate-900 text-white" : "bg-white"}`} type="button" onClick={() => setTab(key)}>{label}</button>
        ))}
      </div>
      {tab === "skills" ? <Card className="rounded-lg bg-white"><CardHeader><CardTitle>Skills Catalog</CardTitle><CardDescription>{skills.length} skills</CardDescription></CardHeader><CardContent><DataTable rows={skills} /></CardContent></Card> : null}
      {tab === "mcps" ? <Card className="rounded-lg bg-white"><CardHeader><CardTitle>MCP Servers Catalog</CardTitle><CardDescription>{mcps.length} servers</CardDescription></CardHeader><CardContent><DataTable rows={mcps} /></CardContent></Card> : null}
      {tab === "bindings" ? <Card className="rounded-lg bg-white"><CardHeader><CardTitle>Capability Bindings</CardTitle><CardDescription><ToneBadge>{bindings.length} bindings</ToneBadge></CardDescription></CardHeader><CardContent><DataTable rows={bindings} /></CardContent></Card> : null}
    </SettingsPageFrame>
  );
}
