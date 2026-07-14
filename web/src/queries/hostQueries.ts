import { queryOptions } from "@tanstack/react-query";

import { fetchHosts } from "@/pages/settingsApi";
import { queryKeys } from "@/queries/queryKeys";

export function hostListQuery() {
  return queryOptions({
    queryKey: queryKeys.hosts.list(),
    queryFn: ({ signal }) => fetchHosts({ signal }),
    staleTime: 30_000,
    gcTime: 10 * 60_000,
  });
}
