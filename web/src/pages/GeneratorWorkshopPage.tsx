import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame } from "@/pages/settingsComponents";

type DraftPayload = Record<string, unknown>;

async function requestJson<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function GeneratorWorkshopPage() {
  const [source, setSource] = useState("mcp_tool");
  const [step, setStep] = useState("generate");
  const [toolName, setToolName] = useState("list-services");
  const [serviceType, setServiceType] = useState("web-api");
  const [scriptConfigText, setScriptConfigText] = useState('{"scriptName":"restart-service","description":"..."}');
  const [draft, setDraft] = useState<DraftPayload | null>(null);
  const [lintResult, setLintResult] = useState<DraftPayload | null>(null);
  const [publishResult, setPublishResult] = useState<DraftPayload | null>(null);

  async function generate() {
    const payload = {
      source,
      toolName,
      serviceType,
      scriptConfig: source === "script_config" ? JSON.parse(scriptConfigText || "{}") : undefined,
    };
    setDraft(await requestJson<DraftPayload>("/api/v1/generator/generate", payload));
    setStep("preview");
  }

  async function lint() {
    setLintResult(await requestJson<DraftPayload>("/api/v1/generator/lint", { draft }));
  }

  async function publish() {
    setPublishResult(await requestJson<DraftPayload>("/api/v1/generator/publish-draft", { draft }));
  }

  return (
    <SettingsPageFrame title="Generator Workshop" description="从 MCP tool、脚本配置或 Coroot 服务生成可发布草稿。">
      <div className="grid gap-4 xl:grid-cols-[360px_1fr]">
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>选择生成来源</CardTitle>
            <CardDescription>请求沿用 /api/v1/generator/*。</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            {[
              ["mcp_tool", "MCP 工具"],
              ["script_config", "脚本配置"],
              ["coroot", "Coroot 服务"],
            ].map(([key, label]) => (
              <label key={key} className="ops-radio flex cursor-pointer items-center gap-2 rounded-lg border p-2">
                <input type="radio" checked={source === key} onChange={() => setSource(key)} />
                <span className="ops-radio__label">{label}</span>
              </label>
            ))}
            {source === "mcp_tool" ? <Input value={toolName} onChange={(event) => setToolName(event.target.value)} placeholder="list-services" /> : null}
            {source === "script_config" ? <textarea className="min-h-32 rounded-lg border p-2 font-mono text-xs" value={scriptConfigText} onChange={(event) => setScriptConfigText(event.target.value)} placeholder='{"scriptName":"restart-service","description":"..."}' /> : null}
            {source === "coroot" ? <Input value={serviceType} onChange={(event) => setServiceType(event.target.value)} placeholder="web-api" /> : null}
            <Button onClick={() => void generate()}>生成草稿</Button>
          </CardContent>
        </Card>

        <div className="grid gap-3">
          <div className="flex flex-wrap gap-2">
            {[
              ["preview", "草稿预览"],
              ["lint", "校验"],
              ["publish", "发布"],
            ].map(([key, label]) => (
              <button key={key} className={`ops-step rounded-lg border px-3 py-2 text-sm ${step === key ? "bg-slate-900 text-white" : "bg-white"}`} type="button" onClick={() => setStep(key)}>
                {label}
              </button>
            ))}
          </div>

          {step === "preview" ? (
            <Card className="rounded-lg bg-white">
              <CardHeader><CardTitle asChild><h2>草稿预览</h2></CardTitle></CardHeader>
              <CardContent><pre className="preview-output overflow-auto rounded-lg bg-slate-950 p-4 text-xs text-slate-50">{JSON.stringify(draft || { hint: "点击生成草稿" }, null, 2)}</pre></CardContent>
            </Card>
          ) : null}

          {step === "lint" ? (
            <Card className="rounded-lg bg-white">
              <CardHeader><CardTitle>校验</CardTitle></CardHeader>
              <CardContent className="grid gap-3">
                <Button onClick={() => void lint()}>运行校验</Button>
                {lintResult ? <div className={`lint-status rounded-lg border p-3 ${lintResult.valid === false ? "invalid" : "valid"}`}>校验{lintResult.valid === false ? "未通过" : "通过"}<pre className="mt-2 text-xs">{JSON.stringify(lintResult, null, 2)}</pre></div> : null}
              </CardContent>
            </Card>
          ) : null}

          {step === "publish" ? (
            <Card className="rounded-lg bg-white">
              <CardHeader><CardTitle>发布</CardTitle></CardHeader>
              <CardContent className="grid gap-3">
                <Button onClick={() => void publish()}>确认发布</Button>
                {publishResult ? <div className="lint-status valid rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-emerald-700">发布成功<pre className="preview-output mt-2 overflow-auto rounded bg-white p-3 text-xs text-slate-700">{JSON.stringify(publishResult, null, 2)}</pre></div> : null}
              </CardContent>
            </Card>
          ) : null}
        </div>
      </div>
    </SettingsPageFrame>
  );
}
