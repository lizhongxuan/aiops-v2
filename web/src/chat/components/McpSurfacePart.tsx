import { Pin, RefreshCw, Settings2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import type { AiopsTransportMcpSurface } from "@/transport/aiopsTransportTypes";

type McpSurfacePartProps = {
  surface: AiopsTransportMcpSurface;
};

export function McpSurfacePart({ surface }: McpSurfacePartProps) {
  const commands = useAiopsTransportCommands();

  return (
    <div className="rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm shadow-sm">
      <div className="flex items-center gap-2">
        <Settings2 className="h-4 w-4 shrink-0 text-zinc-500" />
        <div className="min-w-0 flex-1 truncate font-medium text-zinc-800">{surface.title || surface.id}</div>
        {surface.status ? (
          <Badge variant="outline" className="bg-white text-zinc-600">
            {surface.status}
          </Badge>
        ) : null}
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => commands.mcpAction(surface.id, surface.status === "connected" ? "close" : "open")}
        >
          {surface.status === "connected" ? "Close" : "Open"}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={() => commands.mcpRefresh(surface.id)}>
          <RefreshCw className="h-3.5 w-3.5" />
          Refresh
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={() => commands.mcpPin(surface.id, !surface.pinned)}>
          <Pin className="h-3.5 w-3.5" />
          {surface.pinned ? "Unpin" : "Pin"}
        </Button>
      </div>
    </div>
  );
}
