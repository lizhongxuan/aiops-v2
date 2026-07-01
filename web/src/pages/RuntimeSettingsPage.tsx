import { Save } from "lucide-react";
import type { PropsWithChildren } from "react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  fetchRuntimeSettings,
  type RuntimeSettingsUpdate,
  type RuntimeSettingsView,
  updateRuntimeSettings,
} from "@/pages/settingsApi";
import { Field, LoadingState, SelectField, SettingsPageFrame, StatusAlert } from "@/pages/settingsComponents";

type RuntimeSettingsForm = {
  agentRuntime: {
    intentFrameRouting: string;
    diagnosticProtocol: boolean;
  };
  tooling: {
    readOnlyRetryEnabled: boolean;
    readOnlyRetryMaxPerCall: string;
    readOnlyRetryMaxPerTurn: string;
    readOnlyRetryBackoffBaseMs: string;
    readOnlyRetryBackoffMaxMs: string;
  };
  workflow: {
    referenceGuardMode: string;
    validationProvider: string;
    validationImage: string;
  };
  opsManual: {
    autoRetrieval: boolean;
  };
  debug: {
    modelInputTrace: boolean;
    finalState: boolean;
    transportProjection: boolean;
    transcriptProjection: boolean;
  };
  publicWeb: {
    enabled: boolean;
  };
};

const defaultRuntimeSettings: RuntimeSettingsView = {
  agentRuntime: {
    intentFrameRouting: "trace_only",
    diagnosticProtocol: true,
  },
  tooling: {
    readOnlyRetryEnabled: false,
    readOnlyRetryMaxPerCall: 1,
    readOnlyRetryMaxPerTurn: 3,
    readOnlyRetryBackoffBaseMs: 300,
    readOnlyRetryBackoffMaxMs: 2000,
  },
  workflow: {
    referenceGuardMode: "enforce",
    validationProvider: "static",
    validationImage: "python:3.12-slim",
  },
  opsManual: {
    autoRetrieval: false,
  },
  debug: {
    modelInputTrace: true,
    finalState: false,
    transportProjection: false,
    transcriptProjection: false,
  },
  publicWeb: {
    enabled: true,
  },
};

function effectiveSettings(settings?: RuntimeSettingsView, defaults?: RuntimeSettingsView): RuntimeSettingsView {
  return {
    agentRuntime: { ...defaultRuntimeSettings.agentRuntime, ...defaults?.agentRuntime, ...settings?.agentRuntime },
    tooling: { ...defaultRuntimeSettings.tooling, ...defaults?.tooling, ...settings?.tooling },
    workflow: { ...defaultRuntimeSettings.workflow, ...defaults?.workflow, ...settings?.workflow },
    opsManual: { ...defaultRuntimeSettings.opsManual, ...defaults?.opsManual, ...settings?.opsManual },
    debug: { ...defaultRuntimeSettings.debug, ...defaults?.debug, ...settings?.debug },
    publicWeb: { ...defaultRuntimeSettings.publicWeb, ...defaults?.publicWeb, ...settings?.publicWeb },
    updatedAt: settings?.updatedAt || defaults?.updatedAt,
  };
}

function formFromSettings(settings: RuntimeSettingsView): RuntimeSettingsForm {
  return {
    agentRuntime: {
      intentFrameRouting: String(settings.agentRuntime?.intentFrameRouting || "trace_only"),
      diagnosticProtocol: Boolean(settings.agentRuntime?.diagnosticProtocol),
    },
    tooling: {
      readOnlyRetryEnabled: Boolean(settings.tooling?.readOnlyRetryEnabled),
      readOnlyRetryMaxPerCall: String(settings.tooling?.readOnlyRetryMaxPerCall ?? 1),
      readOnlyRetryMaxPerTurn: String(settings.tooling?.readOnlyRetryMaxPerTurn ?? 3),
      readOnlyRetryBackoffBaseMs: String(settings.tooling?.readOnlyRetryBackoffBaseMs ?? 300),
      readOnlyRetryBackoffMaxMs: String(settings.tooling?.readOnlyRetryBackoffMaxMs ?? 2000),
    },
    workflow: {
      referenceGuardMode: String(settings.workflow?.referenceGuardMode || "enforce"),
      validationProvider: String(settings.workflow?.validationProvider || "static"),
      validationImage: String(settings.workflow?.validationImage || "python:3.12-slim"),
    },
    opsManual: {
      autoRetrieval: Boolean(settings.opsManual?.autoRetrieval),
    },
    debug: {
      modelInputTrace: Boolean(settings.debug?.modelInputTrace),
      finalState: Boolean(settings.debug?.finalState),
      transportProjection: Boolean(settings.debug?.transportProjection),
      transcriptProjection: Boolean(settings.debug?.transcriptProjection),
    },
    publicWeb: {
      enabled: Boolean(settings.publicWeb?.enabled),
    },
  };
}

function numberValue(value: string) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? Math.trunc(numeric) : 0;
}

function changedNumber(current: string, baseline: string) {
  return numberValue(current) !== numberValue(baseline);
}

function buildPatch(form: RuntimeSettingsForm, baseline: RuntimeSettingsForm): RuntimeSettingsUpdate {
  const patch: RuntimeSettingsUpdate = {};
  if (form.agentRuntime.intentFrameRouting !== baseline.agentRuntime.intentFrameRouting || form.agentRuntime.diagnosticProtocol !== baseline.agentRuntime.diagnosticProtocol) {
    patch.agentRuntime = {};
    if (form.agentRuntime.intentFrameRouting !== baseline.agentRuntime.intentFrameRouting) patch.agentRuntime.intentFrameRouting = form.agentRuntime.intentFrameRouting;
    if (form.agentRuntime.diagnosticProtocol !== baseline.agentRuntime.diagnosticProtocol) patch.agentRuntime.diagnosticProtocol = form.agentRuntime.diagnosticProtocol;
  }
  if (
    form.tooling.readOnlyRetryEnabled !== baseline.tooling.readOnlyRetryEnabled ||
    changedNumber(form.tooling.readOnlyRetryMaxPerCall, baseline.tooling.readOnlyRetryMaxPerCall) ||
    changedNumber(form.tooling.readOnlyRetryMaxPerTurn, baseline.tooling.readOnlyRetryMaxPerTurn) ||
    changedNumber(form.tooling.readOnlyRetryBackoffBaseMs, baseline.tooling.readOnlyRetryBackoffBaseMs) ||
    changedNumber(form.tooling.readOnlyRetryBackoffMaxMs, baseline.tooling.readOnlyRetryBackoffMaxMs)
  ) {
    patch.tooling = {};
    if (form.tooling.readOnlyRetryEnabled !== baseline.tooling.readOnlyRetryEnabled) patch.tooling.readOnlyRetryEnabled = form.tooling.readOnlyRetryEnabled;
    if (changedNumber(form.tooling.readOnlyRetryMaxPerCall, baseline.tooling.readOnlyRetryMaxPerCall)) patch.tooling.readOnlyRetryMaxPerCall = numberValue(form.tooling.readOnlyRetryMaxPerCall);
    if (changedNumber(form.tooling.readOnlyRetryMaxPerTurn, baseline.tooling.readOnlyRetryMaxPerTurn)) patch.tooling.readOnlyRetryMaxPerTurn = numberValue(form.tooling.readOnlyRetryMaxPerTurn);
    if (changedNumber(form.tooling.readOnlyRetryBackoffBaseMs, baseline.tooling.readOnlyRetryBackoffBaseMs)) patch.tooling.readOnlyRetryBackoffBaseMs = numberValue(form.tooling.readOnlyRetryBackoffBaseMs);
    if (changedNumber(form.tooling.readOnlyRetryBackoffMaxMs, baseline.tooling.readOnlyRetryBackoffMaxMs)) patch.tooling.readOnlyRetryBackoffMaxMs = numberValue(form.tooling.readOnlyRetryBackoffMaxMs);
  }
  if (
    form.workflow.referenceGuardMode !== baseline.workflow.referenceGuardMode ||
    form.workflow.validationProvider !== baseline.workflow.validationProvider ||
    form.workflow.validationImage !== baseline.workflow.validationImage
  ) {
    patch.workflow = {};
    if (form.workflow.referenceGuardMode !== baseline.workflow.referenceGuardMode) patch.workflow.referenceGuardMode = form.workflow.referenceGuardMode;
    if (form.workflow.validationProvider !== baseline.workflow.validationProvider) patch.workflow.validationProvider = form.workflow.validationProvider;
    if (form.workflow.validationImage !== baseline.workflow.validationImage) patch.workflow.validationImage = form.workflow.validationImage;
  }
  if (form.opsManual.autoRetrieval !== baseline.opsManual.autoRetrieval) {
    patch.opsManual = { autoRetrieval: form.opsManual.autoRetrieval };
  }
  if (
    form.debug.modelInputTrace !== baseline.debug.modelInputTrace ||
    form.debug.finalState !== baseline.debug.finalState ||
    form.debug.transportProjection !== baseline.debug.transportProjection ||
    form.debug.transcriptProjection !== baseline.debug.transcriptProjection
  ) {
    patch.debug = {};
    if (form.debug.modelInputTrace !== baseline.debug.modelInputTrace) patch.debug.modelInputTrace = form.debug.modelInputTrace;
    if (form.debug.finalState !== baseline.debug.finalState) patch.debug.finalState = form.debug.finalState;
    if (form.debug.transportProjection !== baseline.debug.transportProjection) patch.debug.transportProjection = form.debug.transportProjection;
    if (form.debug.transcriptProjection !== baseline.debug.transcriptProjection) patch.debug.transcriptProjection = form.debug.transcriptProjection;
  }
  if (form.publicWeb.enabled !== baseline.publicWeb.enabled) {
    patch.publicWeb = { enabled: form.publicWeb.enabled };
  }
  return patch;
}

const intentRoutingOptions = [
  { label: "trace_only", value: "trace_only" },
  { label: "shadow", value: "shadow" },
  { label: "active", value: "active" },
];

const referenceGuardOptions = [
  { label: "enforce", value: "enforce" },
  { label: "warning", value: "warning" },
];

const validationProviderOptions = [
  { label: "static", value: "static" },
  { label: "docker", value: "docker" },
];

function ToggleField({ label, testId, checked, onChange }: { label: string; testId: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex h-8 items-center gap-2 text-sm text-slate-700">
      <input data-testid={testId} type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      {label}
    </label>
  );
}

function Section({ title, children }: PropsWithChildren<{ title: string }>) {
  return (
    <Card className="rounded-lg bg-white">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{children}</CardContent>
    </Card>
  );
}

export function RuntimeSettingsPage() {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<RuntimeSettingsForm>(() => formFromSettings(defaultRuntimeSettings));
  const [baseline, setBaseline] = useState<RuntimeSettingsForm>(() => formFromSettings(defaultRuntimeSettings));
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);

  async function load() {
    setLoading(true);
    try {
      const payload = await fetchRuntimeSettings();
      const next = formFromSettings(effectiveSettings(payload.settings, payload.defaults));
      setForm(next);
      setBaseline(next);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载失败" });
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    setSaving(true);
    try {
      const payload = await updateRuntimeSettings(buildPatch(form, baseline));
      const next = formFromSettings(effectiveSettings(payload.settings, payload.defaults));
      setForm(next);
      setBaseline(next);
      setMessage({ type: "success", text: "已保存，下次请求生效" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存失败" });
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="运行时配置"
      description=""
      actions={
        <Button data-testid="runtime-settings-save" onClick={() => void save()} disabled={loading || saving}>
          <Save />
          {saving ? "保存中" : "保存"}
        </Button>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "保存失败" : "保存成功"} message={message.text} /> : null}
      {loading ? (
        <LoadingState label="加载运行时配置" />
      ) : (
        <>
          <Section title="Agent Runtime">
            <Field label="IntentFrame">
              <SelectField
                data-testid="runtime-intent-frame-routing"
                aria-label="IntentFrame"
                value={form.agentRuntime.intentFrameRouting}
                options={intentRoutingOptions}
                onChange={(intentFrameRouting) => setForm((prev) => ({ ...prev, agentRuntime: { ...prev.agentRuntime, intentFrameRouting } }))}
              />
            </Field>
            <Field label="Diagnostic">
              <ToggleField
                testId="runtime-diagnostic-protocol"
                label="启用"
                checked={form.agentRuntime.diagnosticProtocol}
                onChange={(diagnosticProtocol) => setForm((prev) => ({ ...prev, agentRuntime: { ...prev.agentRuntime, diagnosticProtocol } }))}
              />
            </Field>
          </Section>

          <Section title="Tooling">
            <Field label="Read-only Retry">
              <ToggleField
                testId="runtime-readonly-retry-enabled"
                label="启用"
                checked={form.tooling.readOnlyRetryEnabled}
                onChange={(readOnlyRetryEnabled) => setForm((prev) => ({ ...prev, tooling: { ...prev.tooling, readOnlyRetryEnabled } }))}
              />
            </Field>
            <Field label="Per Call">
              <Input
                data-testid="runtime-readonly-retry-per-call"
                type="number"
                min={0}
                value={form.tooling.readOnlyRetryMaxPerCall}
                onChange={(event) => setForm((prev) => ({ ...prev, tooling: { ...prev.tooling, readOnlyRetryMaxPerCall: event.target.value } }))}
              />
            </Field>
            <Field label="Per Turn">
              <Input
                data-testid="runtime-readonly-retry-per-turn"
                type="number"
                min={0}
                value={form.tooling.readOnlyRetryMaxPerTurn}
                onChange={(event) => setForm((prev) => ({ ...prev, tooling: { ...prev.tooling, readOnlyRetryMaxPerTurn: event.target.value } }))}
              />
            </Field>
            <Field label="Backoff Base">
              <Input
                data-testid="runtime-readonly-retry-backoff-base"
                type="number"
                min={1}
                value={form.tooling.readOnlyRetryBackoffBaseMs}
                onChange={(event) => setForm((prev) => ({ ...prev, tooling: { ...prev.tooling, readOnlyRetryBackoffBaseMs: event.target.value } }))}
              />
            </Field>
            <Field label="Backoff Max">
              <Input
                data-testid="runtime-readonly-retry-backoff-max"
                type="number"
                min={1}
                value={form.tooling.readOnlyRetryBackoffMaxMs}
                onChange={(event) => setForm((prev) => ({ ...prev, tooling: { ...prev.tooling, readOnlyRetryBackoffMaxMs: event.target.value } }))}
              />
            </Field>
          </Section>

          <Section title="Workflow">
            <Field label="Reference Guard">
              <SelectField
                data-testid="runtime-reference-guard-mode"
                aria-label="Reference Guard"
                value={form.workflow.referenceGuardMode}
                options={referenceGuardOptions}
                onChange={(referenceGuardMode) => setForm((prev) => ({ ...prev, workflow: { ...prev.workflow, referenceGuardMode } }))}
              />
            </Field>
            <Field label="Validation">
              <SelectField
                data-testid="runtime-validation-provider"
                aria-label="Validation"
                value={form.workflow.validationProvider}
                options={validationProviderOptions}
                onChange={(validationProvider) => setForm((prev) => ({ ...prev, workflow: { ...prev.workflow, validationProvider } }))}
              />
            </Field>
            {form.workflow.validationProvider === "docker" ? (
              <Field label="Image">
                <Input
                  data-testid="runtime-validation-image"
                  value={form.workflow.validationImage}
                  onChange={(event) => setForm((prev) => ({ ...prev, workflow: { ...prev.workflow, validationImage: event.target.value } }))}
                />
              </Field>
            ) : null}
          </Section>

          <Section title="Ops Manual">
            <Field label="Auto Retrieval">
              <ToggleField
                testId="runtime-ops-manual-auto-retrieval"
                label="启用"
                checked={form.opsManual.autoRetrieval}
                onChange={(autoRetrieval) => setForm((prev) => ({ ...prev, opsManual: { ...prev.opsManual, autoRetrieval } }))}
              />
            </Field>
          </Section>

          <Section title="Debug">
            <Field label="Model Input">
              <ToggleField
                testId="runtime-debug-model-input-trace"
                label="启用"
                checked={form.debug.modelInputTrace}
                onChange={(modelInputTrace) => setForm((prev) => ({ ...prev, debug: { ...prev.debug, modelInputTrace } }))}
              />
            </Field>
            <Field label="Final State">
              <ToggleField
                testId="runtime-debug-final-state"
                label="启用"
                checked={form.debug.finalState}
                onChange={(finalState) => setForm((prev) => ({ ...prev, debug: { ...prev.debug, finalState } }))}
              />
            </Field>
            <Field label="Transport">
              <ToggleField
                testId="runtime-debug-transport-projection"
                label="启用"
                checked={form.debug.transportProjection}
                onChange={(transportProjection) => setForm((prev) => ({ ...prev, debug: { ...prev.debug, transportProjection } }))}
              />
            </Field>
            <Field label="Transcript">
              <ToggleField
                testId="runtime-debug-transcript-projection"
                label="启用"
                checked={form.debug.transcriptProjection}
                onChange={(transcriptProjection) => setForm((prev) => ({ ...prev, debug: { ...prev.debug, transcriptProjection } }))}
              />
            </Field>
          </Section>

          <Section title="Public Web">
            <Field label="Enabled">
              <ToggleField
                testId="runtime-public-web-enabled"
                label="启用"
                checked={form.publicWeb.enabled}
                onChange={(enabled) => setForm((prev) => ({ ...prev, publicWeb: { ...prev.publicWeb, enabled } }))}
              />
            </Field>
          </Section>
        </>
      )}
    </SettingsPageFrame>
  );
}
