import type { QueryClient } from "@tanstack/react-query";

import { hostListQuery } from "@/queries/hostQueries";
import { sessionListQuery } from "@/queries/sessionQueries";
import { llmConfigQuery } from "@/queries/settingsQueries";
import { terminalSessionListQuery } from "@/queries/terminalQueries";

export function prefetchRouteData(queryClient: QueryClient, pathname: string) {
  if (pathname === "/") {
    void queryClient.prefetchQuery(sessionListQuery());
    void queryClient.prefetchQuery(hostListQuery());
    void queryClient.prefetchQuery(llmConfigQuery());
    return;
  }
  if (pathname === "/settings/hosts" || pathname === "/hosts") {
    void queryClient.prefetchQuery(hostListQuery());
    void queryClient.prefetchQuery(sessionListQuery());
    void queryClient.prefetchQuery(terminalSessionListQuery());
  }
}
