import httpClient from "./httpClient";

export function login(payload) {
  return httpClient.post("/api/v1/auth/login", payload);
}

export function logout() {
  return httpClient.post("/api/v1/auth/logout");
}

export function startOAuth() {
  return httpClient.get("/api/v1/auth/oauth/start");
}

export function getOAuthStartUrl() {
  return "/api/v1/auth/oauth/start";
}

