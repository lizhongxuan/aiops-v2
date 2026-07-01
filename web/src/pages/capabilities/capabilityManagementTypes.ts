export type CapabilityRecord = {
  id?: string;
  name?: string;
  title?: string;
  description?: string;
  summary?: string;
  source?: unknown;
  kind?: unknown;
  connection?: unknown;
  connector?: unknown;
  permissions?: unknown;
  risks?: unknown;
  risk?: unknown;
  runtime?: unknown;
  audit?: unknown;
  [key: string]: unknown;
};

export type CapabilityListResponse = {
  items?: CapabilityRecord[];
  capabilities?: CapabilityRecord[];
};

export type CapabilityViewItem = {
  id: string;
  name: string;
  description: string;
  sourceLabel: string;
  connectionSummary: string;
  permissionRiskSummary: string;
  runtimeSummary: string;
  auditSummary: string;
  raw: CapabilityRecord;
};
