import httpClient from "./httpClient";
import { queryString } from "./query";

export function getERPHealthSummary(params = {}) {
  return httpClient.get(`/api/v1/erp/health${queryString(params)}`);
}

export function getERPBusinessMetrics(params = {}) {
  return httpClient.get(`/api/v1/erp/business-metrics${queryString(params)}`);
}

export function getERPTenantImpact(params = {}) {
  return httpClient.get(`/api/v1/erp/tenant-impact${queryString(params)}`);
}
