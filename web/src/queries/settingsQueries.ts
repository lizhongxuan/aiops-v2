import { queryOptions } from "@tanstack/react-query";

import { fetchLlmConfig, type LlmConfigView } from "@/pages/settingsApi";
import { queryKeys } from "@/queries/queryKeys";

export function llmConfigQuery(initialData?: LlmConfigView | null) {
  return queryOptions({
    queryKey: queryKeys.llmConfig(),
    queryFn: ({ signal }) => fetchLlmConfig({ signal }),
    staleTime: 5 * 60_000,
    gcTime: 30 * 60_000,
    initialData: initialData || undefined,
  });
}
