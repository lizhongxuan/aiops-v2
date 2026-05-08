import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { TooltipProvider } from "@/components/ui/tooltip";

import type { PropsWithChildren } from "react";
import { useState } from "react";

export function Providers({ children }: PropsWithChildren) {
  const [queryClient] = useState(() => new QueryClient());

  return (
    <QueryClientProvider client={queryClient}>
      <AppShellChromeProvider>
        <TooltipProvider>
          <BrowserRouter>{children}</BrowserRouter>
        </TooltipProvider>
      </AppShellChromeProvider>
    </QueryClientProvider>
  );
}
