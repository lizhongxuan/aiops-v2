import httpClient from "./httpClient";

export type HostInventoryItem = {
  id?: string;
  hostId?: string;
  hostname?: string;
  ip?: string;
  name?: string;
  address?: string;
  sshUser?: string;
  sshPort?: number;
};

export async function listHostInventory(): Promise<HostInventoryItem[]> {
  const response = await httpClient.get("/api/v1/hosts");
  const data = response?.data ?? response;
  if (Array.isArray(data)) {
    return data;
  }
  if (Array.isArray(data?.items)) {
    return data.items;
  }
  if (Array.isArray(data?.hosts)) {
    return data.hosts;
  }
  return [];
}
