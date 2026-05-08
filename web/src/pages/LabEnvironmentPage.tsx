import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";

type LabEnvironment = {
  id: string;
  name: string;
  scenario?: string;
  status?: string;
  topology?: { nodes?: Array<{ id: string; name?: string; role?: string }>; links?: Array<Record<string, unknown>> };
  mockHostIds?: string[];
  updatedAt?: string;
};

type LabResponse = {
  items?: LabEnvironment[];
  stats?: { total?: number; running?: number; stopped?: number; draft?: number };
};

const templates = [
  { id: "web-db-2tier", label: "Web + DB 双层架构", description: "两台 mock host，覆盖服务恢复演练。" },
  { id: "cache-layer", label: "缓存层测试", description: "缓存故障、穿透与容量验证。" },
  { id: "k8s-canary", label: "K8s Canary", description: "发布、回滚与监控校验。" },
];

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), {
    credentials: "include",
    ...init,
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function LabEnvironmentPage() {
  const [items, setItems] = useState<LabEnvironment[]>([]);
  const [stats, setStats] = useState<LabResponse["stats"]>({});
  const [tab, setTab] = useState("envs");
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [scenario, setScenario] = useState(templates[0].id);

  async function load() {
    const payload = await requestJson<LabResponse>("/api/v1/lab-environments");
    setItems(payload.items || []);
    setStats(payload.stats || {});
  }

  useEffect(() => {
    void load().catch(() => undefined);
  }, []);

  const total = stats?.total ?? items.length;
  const running = stats?.running ?? items.filter((item) => item.status === "running").length;
  const stopped = stats?.stopped ?? items.filter((item) => item.status === "stopped").length;

  async function operate(id: string, action: "start" | "stop" | "reset") {
    await requestJson(`/api/v1/lab-environments/${encodeURIComponent(id)}/${action}`, { method: "POST" });
    await load();
  }

  async function createEnvironment() {
    await requestJson("/api/v1/lab-environments", {
      method: "POST",
      body: JSON.stringify({ name: name || "New Lab", scenario }),
    });
    setCreateOpen(false);
    setName("");
    await load();
  }

  const selected = useMemo(() => items[0], [items]);

  return (
    <SettingsPageFrame
      title="实验环境管理"
      description="管理本地/模拟实验拓扑，保留 start、stop、reset 和创建 API 操作入口。"
      actions={
        <Button className="create-btn" onClick={() => setCreateOpen(true)}>
          新建实验环境
        </Button>
      }
    >
      <div className="grid gap-3 md:grid-cols-3">
        <Card className="rounded-lg bg-white">
          <CardHeader className="lab-stat">
            <CardDescription>总计</CardDescription>
            <CardTitle>{total}</CardTitle>
          </CardHeader>
        </Card>
        <Card className="rounded-lg bg-white">
          <CardHeader className="lab-stat stat-ok">
            <CardDescription>运行中</CardDescription>
            <CardTitle>{running}</CardTitle>
          </CardHeader>
        </Card>
        <Card className="rounded-lg bg-white">
          <CardHeader className="lab-stat stat-warn">
            <CardDescription>已停止</CardDescription>
            <CardTitle>{stopped}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <div className="tab-bar flex gap-2">
        {[
          ["envs", "环境列表"],
          ["templates", "场景模板"],
          ["topology", "拓扑预览"],
        ].map(([key, label]) => (
          <button key={key} type="button" className={`ops-tabs-tab rounded-lg border px-3 py-2 text-sm ${tab === key ? "active bg-slate-900 text-white" : "bg-white"}`} onClick={() => setTab(key)}>
            {label}
          </button>
        ))}
      </div>

      {tab === "envs" ? (
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>环境列表</CardTitle>
            <CardDescription>实验环境状态与操作。</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="ops-data-table-table overflow-auto">
              <table className="data-table w-full border-collapse text-sm">
                <thead>
                  <tr className="text-left text-slate-500">
                    <th className="border-b p-2">名称</th>
                    <th className="border-b p-2">场景</th>
                    <th className="border-b p-2">状态</th>
                    <th className="border-b p-2">Mock Hosts</th>
                    <th className="border-b p-2">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => (
                    <tr key={item.id}>
                      <td className="border-b p-2 font-medium">{item.name}</td>
                      <td className="border-b p-2">{item.scenario}</td>
                      <td className="border-b p-2"><ToneBadge tone={item.status === "running" ? "success" : "warning"}>{item.status || "draft"}</ToneBadge></td>
                      <td className="border-b p-2">{item.mockHostIds?.join(", ") || "-"}</td>
                      <td className="border-b p-2">
                        <div className="flex flex-wrap gap-2">
                          {item.status === "running" ? <Button size="sm" variant="outline" onClick={() => void operate(item.id, "stop")}>停止</Button> : <Button size="sm" variant="outline" onClick={() => void operate(item.id, "start")}>启动</Button>}
                          <Button size="sm" variant="outline" onClick={() => void operate(item.id, "reset")}>重置</Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {tab === "templates" ? (
        <div className="grid gap-3 md:grid-cols-3">
          {templates.map((template) => (
            <Card key={template.id} className="ops-card rounded-lg bg-white">
              <CardHeader>
                <CardTitle>{template.label}</CardTitle>
                <CardDescription>{template.description}</CardDescription>
              </CardHeader>
              <CardContent>
                <Button size="sm" onClick={() => { setScenario(template.id); setCreateOpen(true); }}>使用模板</Button>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}

      {tab === "topology" ? (
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>拓扑预览</CardTitle>
            <CardDescription>{selected?.name || "未选择环境"}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2 text-sm">
            {(selected?.topology?.nodes || []).map((node) => (
              <div key={node.id} className="rounded-lg border bg-slate-50 p-3">{node.name || node.id} · {node.role || "node"}</div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      {createOpen ? (
        <section className="fixed inset-0 z-50 grid place-items-center bg-slate-950/30 p-4">
          <div className="dialog-box grid w-full max-w-lg gap-4 rounded-xl bg-white p-5 shadow-xl">
            <h2>新建实验环境</h2>
            <label className="grid gap-2 text-sm font-medium">
              名称
              <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Test Lab" />
            </label>
            <label className="grid gap-2 text-sm font-medium">
              场景
              <select className="rounded-lg border p-2" value={scenario} onChange={(event) => setScenario(event.target.value)}>
                {templates.map((template) => <option key={template.id} value={template.id}>{template.label}</option>)}
              </select>
            </label>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button>
              <Button className="action-start" onClick={() => void createEnvironment()}>创建</Button>
            </div>
          </div>
        </section>
      ) : null}
    </SettingsPageFrame>
  );
}
