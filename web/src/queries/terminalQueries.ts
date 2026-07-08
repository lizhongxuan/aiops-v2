import { queryOptions } from "@tanstack/react-query";

import { fetchTerminalSessions } from "@/pages/settingsApi";
import { queryKeys } from "@/queries/queryKeys";

export function terminalSessionListQuery() {
  return queryOptions({
    queryKey: queryKeys.terminalSessions.list(),
    queryFn: ({ signal }) => fetchTerminalSessions({ signal }),
    staleTime: 30_000,
    gcTime: 10 * 60_000,
  });
}
