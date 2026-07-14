import { queryOptions } from "@tanstack/react-query";

import {
  fetchSessions,
  type SessionListResponse,
  type SessionSummary,
} from "@/pages/settingsApi";
import { queryKeys } from "@/queries/queryKeys";

export function normalizeSessionItems(
  payload: SessionListResponse | undefined,
): SessionSummary[] {
  return payload?.sessions || payload?.items || [];
}

export function sessionListQuery() {
  return queryOptions({
    queryKey: queryKeys.sessions.list(),
    queryFn: ({ signal }) => fetchSessions({ signal }),
    staleTime: 15_000,
    gcTime: 15 * 60_000,
  });
}
