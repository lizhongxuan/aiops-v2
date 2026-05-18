import { useEffect, useMemo, useState } from "react";

import { previewUiCard, fetchUiCards, updateUiCardStatus } from "@/api/uiCards";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SelectField, SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";

type UiCard = {
  id: string;
  name: string;
  kind?: string;
  renderer?: string;
  rendererVersion?: string;
  schemaVersion?: string;
  payloadSchema?: Record<string, unknown>;
  metadataSchema?: Record<string, unknown>;
  actionPolicy?: Record<string, unknown>;
  displayPolicy?: Record<string, unknown>;
  redactionPolicy?: Record<string, unknown>;
  samplePayloads?: Array<Record<string, unknown>>;
  placementDefaults?: string[];
  status?: string;
  builtIn?: boolean;
  version?: number;
  summary?: string;
};

type CardsResponse = { items?: UiCard[]; stats?: Record<string, number>; total?: number };

const tabs = ["Overview", "Registry", "Detail", "Preview", "Versions"] as const;
const statusLabels: Record<string, string> = {
  active: "启用",
  draft: "草稿",
  deprecated: "已废弃",
  disabled: "已禁用",
};

export function UICardManagementPage() {
  const [cards, setCards] = useState<UiCard[]>([]);
  const [stats, setStats] = useState<Record<string, number>>({});
  const [tab, setTab] = useState<typeof tabs[number]>("Overview");
  const [selectedId, setSelectedId] = useState("");
  const [previewInput, setPreviewInput] = useState("{}");
  const [previewContext, setPreviewContext] = useState("normal");
  const [previewResult, setPreviewResult] = useState<Record<string, unknown> | null>(null);
  const [localError, setLocalError] = useState("");
  const [notice, setNotice] = useState("");

  async function load() {
    const payload = await fetchUiCards() as CardsResponse;
    const nextCards = payload.items || [];
    setCards(nextCards);
    setStats({ total: payload.total ?? nextCards.length, ...(payload.stats || {}) });
    setSelectedId((current) => current || nextCards[0]?.id || "");
    if (previewInput === "{}" && nextCards[0]) {
      setPreviewInput(JSON.stringify(sampleArtifact(nextCards[0]), null, 2));
    }
  }

  useEffect(() => {
    void load().catch((error) => setLocalError(error instanceof Error ? error.message : "加载 UI Card 失败"));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const selectedCard = useMemo(() => cards.find((card) => card.id === selectedId) || cards[0] || null, [cards, selectedId]);

  useEffect(() => {
    if (selectedCard) {
      setPreviewInput(JSON.stringify(sampleArtifact(selectedCard), null, 2));
      setPreviewResult(null);
      setLocalError("");
    }
  }, [selectedCard?.id]);

  async function runPreview() {
    if (!selectedCard) return;
    setLocalError("");
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(previewInput || "{}") as Record<string, unknown>;
    } catch {
      setLocalError("Preview JSON 格式不正确");
      return;
    }
    const result = await previewUiCard(selectedCard.id, { payload: parsed, context: { permission: previewContext } }) as Record<string, unknown>;
    setPreviewResult(result);
  }

  async function setCardStatus(status: string) {
    if (!selectedCard) return;
    const updated = await updateUiCardStatus(selectedCard.id, status) as UiCard;
    setCards((current) => current.map((card) => (card.id === selectedCard.id ? { ...card, ...updated } : card)));
    setNotice(`已将 ${selectedCard.name} 设置为 ${statusLabels[status] || status}`);
  }

  return (
    <SettingsPageFrame title="UI Cards" description="Agent-to-UI 卡片注册、预览和版本治理。">
      <section className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="总计" value={stats.total ?? cards.length} />
        <Stat label="启用" value={stats.active ?? countByStatus(cards, "active")} />
        <Stat label="草稿" value={stats.draft ?? countByStatus(cards, "draft")} />
        <Stat label="已废弃" value={stats.deprecated ?? countByStatus(cards, "deprecated")} />
        <Stat label="已禁用" value={stats.disabled ?? countByStatus(cards, "disabled")} />
        <Stat label="内置" value={stats.builtIn ?? cards.filter((card) => card.builtIn).length} />
      </section>

      <div className="flex flex-wrap gap-2">
        {tabs.map((item) => (
          <button
            key={item}
            type="button"
            className={`rounded-lg border px-3 py-2 text-sm ${tab === item ? "bg-slate-950 text-white" : "bg-white text-slate-700"}`}
            onClick={() => setTab(item)}
          >
            {item}
          </button>
        ))}
      </div>

      {notice ? <div className="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">{notice}</div> : null}
      {localError ? <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{localError}</div> : null}

      <div className="grid min-h-0 gap-4 xl:grid-cols-[minmax(320px,0.9fr)_minmax(0,1.4fr)]">
        <section className="rounded-lg border bg-white">
          <div className="border-b px-4 py-3">
            <h2 className="text-sm font-semibold text-slate-950">Registry</h2>
          </div>
          <div className="grid gap-2 p-3">
            {cards.map((card) => (
              <button
                key={card.id}
                type="button"
                className={`rounded-lg border p-3 text-left transition hover:border-slate-300 ${selectedCard?.id === card.id ? "border-slate-900 bg-slate-50" : "bg-white"}`}
                onClick={() => setSelectedId(card.id)}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate font-medium text-slate-950">{card.name}</div>
                    <div className="mt-1 flex flex-wrap gap-1 text-xs text-slate-500">
                      <code>{card.kind}</code>
                      <span>{card.renderer}</span>
                    </div>
                  </div>
                  <ToneBadge tone={toneForStatus(card.status)}>{statusLabels[card.status || "active"] || card.status}</ToneBadge>
                </div>
                <div className="mt-2 text-xs text-slate-500">版本 {card.version || 1}{card.builtIn ? " · built-in" : ""}</div>
              </button>
            ))}
          </div>
        </section>

        <section className="rounded-lg border bg-white p-4">
          {tab === "Overview" ? <Overview cards={cards} /> : null}
          {tab === "Registry" ? <RegistryTable cards={cards} onSelect={(id) => setSelectedId(id)} /> : null}
          {tab === "Detail" && selectedCard ? <DetailPanel card={selectedCard} onDisable={() => void setCardStatus("disabled")} /> : null}
          {tab === "Preview" && selectedCard ? (
            <PreviewPanel
              card={selectedCard}
              previewInput={previewInput}
              previewContext={previewContext}
              previewResult={previewResult}
              onInput={setPreviewInput}
              onContext={setPreviewContext}
              onPreview={() => void runPreview()}
            />
          ) : null}
          {tab === "Versions" && selectedCard ? <VersionPanel card={selectedCard} /> : null}
        </section>
      </div>
    </SettingsPageFrame>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border bg-white px-4 py-3">
      <div className="text-xs text-slate-500">{label}</div>
      <div className="mt-1 text-xl font-semibold text-slate-950">{value}</div>
    </div>
  );
}

function Overview({ cards }: { cards: UiCard[] }) {
  const byKind = Object.entries(cards.reduce<Record<string, number>>((acc, card) => {
    const key = card.kind || "unknown";
    acc[key] = (acc[key] || 0) + 1;
    return acc;
  }, {}));
  return (
    <div className="grid gap-3">
      <h2 className="text-base font-semibold">Overview</h2>
      <div className="grid gap-2 md:grid-cols-2">
        {byKind.map(([kind, count]) => (
          <div key={kind} className="rounded-lg border bg-slate-50 px-3 py-2 text-sm">
            <code>{kind}</code>
            <span className="ml-2 text-slate-500">{count} cards</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function RegistryTable({ cards, onSelect }: { cards: UiCard[]; onSelect: (id: string) => void }) {
  return (
    <div className="grid gap-3">
      <h2 className="text-base font-semibold">Registry</h2>
      <div className="overflow-auto rounded-lg border">
        <table className="w-full border-collapse text-sm">
          <thead className="bg-slate-50 text-left text-xs text-slate-500">
            <tr>
              <th className="p-2">Name</th>
              <th className="p-2">Type</th>
              <th className="p-2">Renderer</th>
              <th className="p-2">Status</th>
              <th className="p-2">Version</th>
            </tr>
          </thead>
          <tbody>
            {cards.map((card) => (
              <tr key={card.id} className="border-t">
                <td className="p-2"><button type="button" className="font-medium text-slate-950 hover:underline" onClick={() => onSelect(card.id)}>{card.name}</button></td>
                <td className="p-2"><code>{card.kind}</code></td>
                <td className="p-2"><code>{card.renderer}</code></td>
                <td className="p-2">{card.status}</td>
                <td className="p-2">版本 {card.version || 1}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function DetailPanel({ card, onDisable }: { card: UiCard; onDisable: () => void }) {
  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{card.name}</h2>
          <p className="mt-1 text-sm text-slate-500">{card.summary || "无摘要"}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge variant="outline">{card.kind}</Badge>
          <ToneBadge tone={toneForStatus(card.status)}>{statusLabels[card.status || "active"] || card.status}</ToneBadge>
          <Badge variant="secondary">版本 {card.version || 1}</Badge>
        </div>
      </div>
      <DescriptionGrid rows={[
        ["Renderer", card.renderer || "-"],
        ["Renderer Version", card.rendererVersion || "-"],
        ["Schema Version", card.schemaVersion || "-"],
        ["Placement Defaults", (card.placementDefaults || []).join(", ") || "-"],
        ["Payload Schema", JSON.stringify(card.payloadSchema || {}, null, 2)],
        ["Action Policy", JSON.stringify(card.actionPolicy || {}, null, 2)],
        ["Redaction Policy", JSON.stringify(card.redactionPolicy || {}, null, 2)],
      ]} />
      <div>
        <Button variant="outline" onClick={onDisable}>设为禁用</Button>
      </div>
    </div>
  );
}

function PreviewPanel({
  card,
  previewInput,
  previewContext,
  previewResult,
  onInput,
  onContext,
  onPreview,
}: {
  card: UiCard;
  previewInput: string;
  previewContext: string;
  previewResult: Record<string, unknown> | null;
  onInput: (value: string) => void;
  onContext: (value: string) => void;
  onPreview: () => void;
}) {
  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold">Preview</h2>
        <SelectField
          aria-label="permission context"
          value={previewContext}
          onChange={onContext}
          options={[
            { label: "normal", value: "normal" },
            { label: "redacted", value: "redacted" },
            { label: "restricted", value: "restricted" },
          ]}
        />
      </div>
      <div className="text-sm text-slate-500">当前卡片：{card.name}</div>
      <textarea
        className="min-h-48 rounded-lg border bg-slate-950 p-3 font-mono text-xs text-white outline-none focus:ring-2 focus:ring-slate-300"
        value={previewInput}
        onChange={(event) => onInput(event.target.value)}
      />
      <div><Button onClick={onPreview}>运行 Preview</Button></div>
      {previewResult ? (
        <div className="grid gap-3 lg:grid-cols-2">
          <div className="rounded-lg border bg-slate-50 p-3">
            <div className="text-sm font-medium">API Result</div>
            <pre className="mt-2 max-h-72 overflow-auto text-xs">{JSON.stringify(previewResult, null, 2)}</pre>
          </div>
          <div className="rounded-lg border bg-slate-50 p-3">
            <div className="text-sm font-medium">Local Preview</div>
            <div className="mt-2 text-sm">{previewTitle(previewResult)}</div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function VersionPanel({ card }: { card: UiCard }) {
  return (
    <div className="grid gap-3">
      <h2 className="text-base font-semibold">Versions</h2>
      <div className="rounded-lg border bg-slate-50 p-3 text-sm">
        <div className="font-medium">{card.id}</div>
        <div className="mt-1 text-slate-500">当前版本 {card.version || 1}，状态 {card.status || "active"}</div>
      </div>
    </div>
  );
}

function DescriptionGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid gap-2 text-sm">
      {rows.map(([label, value]) => (
        <div key={label} className="grid gap-1 rounded-lg border bg-slate-50 p-3">
          <dt className="text-xs font-medium uppercase text-slate-500">{label}</dt>
          <dd className="whitespace-pre-wrap break-words text-slate-800">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function sampleArtifact(card: UiCard) {
  const first = card.samplePayloads?.[0];
  if (first && typeof first.artifact === "object" && first.artifact) {
    return first.artifact as Record<string, unknown>;
  }
  return { id: `sample-${card.id}`, type: card.kind || card.id, payload: {} };
}

function countByStatus(cards: UiCard[], status: string) {
  return cards.filter((card) => (card.status || "active") === status).length;
}

function toneForStatus(status?: string): "default" | "success" | "warning" | "danger" {
  if (status === "active") return "success";
  if (status === "deprecated" || status === "draft") return "warning";
  if (status === "disabled") return "danger";
  return "default";
}

function previewTitle(result: Record<string, unknown>) {
  const normalized = result.normalizedArtifact;
  if (normalized && typeof normalized === "object") {
    const title = (normalized as Record<string, unknown>).titleZh || (normalized as Record<string, unknown>).title;
    if (typeof title === "string") return title;
  }
  return "Preview 已生成";
}
