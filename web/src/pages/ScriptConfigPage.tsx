import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";

type ScriptConfig = {
  id: string;
  scriptName: string;
  description?: string;
  status?: string;
  approvalPolicy?: string;
  environmentRef?: string;
  runnerProfile?: string;
};

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function ScriptConfigPage() {
  const [configs, setConfigs] = useState<ScriptConfig[]>([]);
  const [selectedScript, setSelectedScript] = useState("");
  const [selectedConfig, setSelectedConfig] = useState<ScriptConfig | null>(null);
  const [editDraft, setEditDraft] = useState<ScriptConfig | null>(null);
  const [dryRunParams, setDryRunParams] = useState('{"service":"nginx"}');
  const [dryRunResult, setDryRunResult] = useState<Record<string, unknown> | null>(null);

  async function load() {
    const payload = await requestJson<{ items?: ScriptConfig[] }>("/api/v1/script-configs");
    setConfigs(payload.items || []);
    setSelectedScript((current) => current || payload.items?.[0]?.scriptName || "");
  }

  useEffect(() => { void load().catch(() => undefined); }, []);

  const scriptNames = useMemo(() => Array.from(new Set(configs.map((config) => config.scriptName))), [configs]);
  const filteredConfigs = configs.filter((config) => !selectedScript || config.scriptName === selectedScript);

  async function dryRun() {
    if (!selectedConfig) return;
    setDryRunResult(await requestJson<Record<string, unknown>>(`/api/v1/script-configs/${encodeURIComponent(selectedConfig.id)}/dry-run`, { method: "POST", body: JSON.stringify({ params: JSON.parse(dryRunParams || "{}") }) }));
  }

  return (
    <SettingsPageFrame
      title="脚本配置管理"
      description="维护 ScriptConfigProfile，保留创建、详情和 Dry-Run API 操作。"
      actions={<Button onClick={() => setEditDraft({ id: "", scriptName: "new-script", status: "draft" })}>+ 新建配置</Button>}
    >
      <div className="grid gap-4 xl:grid-cols-[260px_1fr_420px]">
        <Card className="rounded-lg bg-white">
          <CardHeader><CardTitle>脚本</CardTitle><CardDescription>{scriptNames.length} groups</CardDescription></CardHeader>
          <CardContent>
            <ul className="script-list grid gap-2">
              {scriptNames.map((name) => (
                <li key={name} className={`cursor-pointer rounded-lg border p-2 text-sm ${selectedScript === name ? "active bg-slate-900 text-white" : "bg-white"}`} onClick={() => setSelectedScript(name)}>{name}</li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card className="rounded-lg bg-white">
          <CardHeader><CardTitle>配置列表</CardTitle></CardHeader>
          <CardContent>
            <table className="data-table w-full border-collapse text-sm">
              <tbody>
                {filteredConfigs.map((config) => (
                  <tr key={config.id} className={selectedConfig?.id === config.id ? "selected" : ""}>
                    <td className="border-b p-2"><code>{config.id}</code></td>
                    <td className="border-b p-2">{config.description}</td>
                    <td className="border-b p-2"><ToneBadge>{config.status}</ToneBadge></td>
                    <td className="border-b p-2"><Button size="sm" variant="outline" onClick={() => { setSelectedConfig(config); setEditDraft(null); }}>详情</Button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
        </Card>

        <div className="grid gap-3">
          {selectedConfig && !editDraft ? (
            <Card className="rounded-lg bg-white">
              <CardHeader><CardTitle>{selectedConfig.scriptName}</CardTitle></CardHeader>
              <CardContent className="grid gap-3">
                <div className="detail-grid grid gap-2 text-sm">
                  <div>环境：{selectedConfig.environmentRef}</div>
                  <div>审批：{selectedConfig.approvalPolicy}</div>
                  <div>Runner：{selectedConfig.runnerProfile}</div>
                </div>
                <label className="dry-run-label grid gap-2 text-sm font-medium">Dry Run 参数<textarea className="rounded-lg border p-2 font-mono text-xs" rows={3} value={dryRunParams} onChange={(event) => setDryRunParams(event.target.value)} /></label>
                <Button onClick={() => void dryRun()}>Dry-Run</Button>
                {dryRunResult ? <pre className="preview-output overflow-auto rounded-lg bg-slate-950 p-3 text-xs text-white">{JSON.stringify(dryRunResult, null, 2)}</pre> : null}
              </CardContent>
            </Card>
          ) : null}

          {editDraft ? (
            <Card className="rounded-lg bg-white">
              <CardHeader><CardTitle>编辑配置</CardTitle></CardHeader>
              <CardContent className="ops-form grid gap-3">
                <Input value={editDraft.scriptName} onChange={(event) => setEditDraft({ ...editDraft, scriptName: event.target.value })} />
                <Input value={editDraft.description || ""} onChange={(event) => setEditDraft({ ...editDraft, description: event.target.value })} placeholder="description" />
                <Button onClick={() => void requestJson("/api/v1/script-configs", { method: "POST", body: JSON.stringify(editDraft) })}>保存</Button>
              </CardContent>
            </Card>
          ) : null}
        </div>
      </div>
    </SettingsPageFrame>
  );
}
