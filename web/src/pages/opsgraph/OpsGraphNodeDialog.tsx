import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

import type { OpsGraphNode } from "./opsGraphTypes";
import { nodeTypeLabel } from "./opsGraphViewModel";

export function OpsGraphNodeDialog({ node, open, onOpenChange }: { node: OpsGraphNode | null; open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{node?.name || "节点"}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-2 text-sm text-slate-600">
          <div>类型：{nodeTypeLabel(node?.type)}</div>
          <div>ID：{node?.id || "-"}</div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
