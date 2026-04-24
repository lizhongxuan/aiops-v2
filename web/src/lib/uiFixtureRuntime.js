import { cloneUiFixturePayload, resolveUiFixturePreset } from "./uiFixturePresets";

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : null;
}

function readUiFixtureOverride() {
  if (typeof window === "undefined") return null;
  return asObject(window.__CODEX_UI_FIXTURE__) || null;
}

function readUiFixtureKey() {
  if (typeof window === "undefined") return "";
  try {
    const params = new URLSearchParams(window.location.search);
    return String(params.get("fixture") || params.get("uiFixture") || params.get("fixtureKey") || "").trim();
  } catch {
    return "";
  }
}

export function resolveUiFixtureRuntime() {
  const override = readUiFixtureOverride();
  if (override) {
    return cloneUiFixturePayload(override);
  }
  const key = readUiFixtureKey();
  if (!key) return null;
  return resolveUiFixturePreset(key);
}

export function hasUiFixtureRuntime() {
  return !!resolveUiFixtureRuntime();
}

export function resolveUiFixtureState() {
  return resolveUiFixtureRuntime()?.state || null;
}

export function resolveUiFixtureSessions() {
  return resolveUiFixtureRuntime()?.sessions || null;
}
