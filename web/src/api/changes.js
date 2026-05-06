import httpClient from "./httpClient";
import { queryString } from "./query";

export function getRecentDeployments(params = {}) {
  return httpClient.get(`/api/v1/changes/deployments${queryString(params)}`);
}

export function getRecentConfigChanges(params = {}) {
  return httpClient.get(`/api/v1/changes/config${queryString(params)}`);
}
