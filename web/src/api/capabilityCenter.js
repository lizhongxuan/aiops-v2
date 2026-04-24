import httpClient from "./httpClient";

export function fetchCapabilityBindings() {
  return httpClient.get("/api/v1/capability-bindings");
}
