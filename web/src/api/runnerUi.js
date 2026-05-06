// Legacy/debug Runner UI compatibility only.
// New Runner Studio code must use runnerStudioClient.js and same-origin
// /api/runner-studio/* APIs instead of the external standalone Runner UI.
export const RUNNER_UI_LEGACY_ONLY = true;

export function normalizeRunnerUiUrl(value = "") {
  const url = String(value || "").trim();
  if (!url) return "";
  return url.endsWith("/") ? url : `${url}/`;
}

export function getRunnerUiUrl(env = import.meta.env) {
  return normalizeRunnerUiUrl(env?.VITE_RUNNER_UI_URL);
}
