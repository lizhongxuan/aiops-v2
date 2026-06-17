import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

import type { OpsGraphRelationship } from "./opsGraphTypes";
import { relationshipLabel } from "./opsGraphViewModel";

export function OpsGraphRelationshipDialog({
  relationship,
  open,
  onOpenChange,
}: {
  relationship: OpsGraphRelationship | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{relationship ? relationshipLabel(relationship.type) : "关系"}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-2 text-sm text-slate-600">
          <div>源：{relationship?.from || "-"}</div>
          <div>目标：{relationship?.to || "-"}</div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
