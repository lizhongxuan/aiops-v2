import httpClient from "./httpClient";

export function fetchSkillCatalog() {
  return httpClient.get("/api/v1/agent-skills");
}

export function saveSkillCatalogItem(item) {
  return httpClient.put(`/api/v1/agent-skills/${encodeURIComponent(item.id)}`, item);
}

export function deleteSkillCatalogItem(skillId) {
  return httpClient.delete(`/api/v1/agent-skills/${encodeURIComponent(skillId)}`);
}

export function fetchMcpCatalog() {
  return httpClient.get("/api/v1/agent-mcps");
}

export function saveMcpCatalogItem(item) {
  return httpClient.put(`/api/v1/agent-mcps/${encodeURIComponent(item.id)}`, item);
}

export function deleteMcpCatalogItem(mcpId) {
  return httpClient.delete(`/api/v1/agent-mcps/${encodeURIComponent(mcpId)}`);
}

export function fetchAgentProfiles() {
  return httpClient.get("/api/v1/agent-profiles");
}

export function fetchSingleAgentProfile() {
  return httpClient.get("/api/v1/agent-profile");
}

export function exportAgentProfiles() {
  return httpClient.get("/api/v1/agent-profiles/export");
}

export function importAgentProfiles(payload) {
  return httpClient.post("/api/v1/agent-profiles/import", payload);
}

export function saveAgentProfile(profile) {
  return httpClient.put("/api/v1/agent-profile", profile);
}

export function resetAgentProfile(profileId) {
  return httpClient.post("/api/v1/agent-profile/reset", { profileId });
}

export function fetchAgentProfilePreview(profileId) {
  return httpClient.get(`/api/v1/agent-profile/preview?profileId=${encodeURIComponent(profileId)}`);
}
