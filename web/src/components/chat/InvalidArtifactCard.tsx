import { AlertTriangle } from "lucide-react";

import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { TerminalArtifactMeta } from "./UnsupportedArtifactCard";

type InvalidArtifactCardProps = {
  artifact: AiopsTransportAgentUiArtifact;
  reason?: string;
};

export function InvalidArtifactCard({ artifact, reason = "卡片数据不符合前端渲染约束。" }: InvalidArtifactCardProps) {
  return (
    <section className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-950" data-testid="invalid-agent-ui-artifact">
      <div className="flex items-start gap-2">
        <span className="mt-0.5 rounded-md bg-red-100 p-1.5 text-red-700">
          <AlertTriangle className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="font-medium">Agent UI 卡片数据无效</h3>
          <p className="mt-1 text-xs leading-5 text-red-800">{reason}</p>
        </div>
      </div>
      <TerminalArtifactMeta artifact={artifact} />
    </section>
  );
}
