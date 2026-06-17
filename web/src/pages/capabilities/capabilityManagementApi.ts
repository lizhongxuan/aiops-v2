import type { CapabilityListResponse } from "./capabilityManagementTypes";

export async function fetchCapabilities(fetchImpl: typeof fetch = fetch): Promise<CapabilityListResponse> {
  const response = await fetchImpl("/api/v1/capabilities", {
    method: "GET",
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as CapabilityListResponse & { error?: string; message?: string };
  if (!response.ok) {
    throw new Error(payload.error || payload.message || `Request failed: ${response.status}`);
  }
  return payload;
}
