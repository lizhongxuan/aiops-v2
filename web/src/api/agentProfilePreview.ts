export type CapabilitySnapshotItem = {
  id: string;
  kind: string;
  enabled: boolean;
  source?: string;
  sourceScope?: string;
  reason?: string;
  policy?: string;
  runtimeStatus?: string;
  risk?: string;
  invocationMode?: string;
  approvalStatus?: string;
};

export type CapabilitySnapshot = {
  profileId?: string;
  fingerprint?: string;
  items?: CapabilitySnapshotItem[];
};

export type AgentProfilePreview = {
  profileId?: string;
  capabilitySnapshot?: CapabilitySnapshot;
};

export async function fetchAgentProfilePreview(profileId: string): Promise<AgentProfilePreview> {
  const response = await fetch(`/api/v1/agent-profile/preview?profileId=${encodeURIComponent(profileId)}`, {
    credentials: "include",
  });
  const text = await response.text();
  const payload = text.trim() ? JSON.parse(text) : {};
  if (!response.ok) {
    const message = typeof payload?.error === "string" ? payload.error : typeof payload?.message === "string" ? payload.message : "Agent Profile preview failed";
    throw new Error(message);
  }
  return payload as AgentProfilePreview;
}
