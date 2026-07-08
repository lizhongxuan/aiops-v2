import { useEffect, useState } from "react";
import { ShieldCheck, X } from "lucide-react";
import type { WorkflowPatch } from "./workflowAiTypes";

export function WorkflowAiPermissionDialog({
  open,
  patch,
  onConfirm,
  onReject,
}: {
  open: boolean;
  patch?: WorkflowPatch;
  onConfirm: () => void;
  onReject: () => void;
}) {
  const [showJson, setShowJson] = useState(false);

  useEffect(() => {
    if (!open) return undefined;
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") onReject();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onReject]);

  if (!open) return null;

  return (
    <div className="workflow-ai-permission-backdrop" data-testid="workflow-ai-permission-dialog" onMouseDown={onReject}>
      <div className="workflow-ai-permission-dialog" role="dialog" aria-modal="true" onMouseDown={(event) => event.stopPropagation()}>
        <header>
          <ShieldCheck size={18} />
          <h3>确认修改</h3>
          <button type="button" aria-label="Close" onClick={onReject}>
            <X size={16} />
          </button>
        </header>
        <dl className="workflow-ai-permission-facts">
          <div><dt>修改</dt><dd>{patch?.summary || patch?.id || "-"}</dd></div>
          <div><dt>影响</dt><dd>{patch?.operations?.length || 0} operations</dd></div>
          <div><dt>风险</dt><dd>需要用户确认</dd></div>
          <div><dt>校验</dt><dd>apply 前必须通过 revision guard</dd></div>
          <div><dt>可撤销</dt><dd>会创建 undo checkpoint</dd></div>
        </dl>
        <button type="button" className="workflow-ai-link-button" onClick={() => setShowJson((value) => !value)}>查看 JSON</button>
        {showJson ? <pre className="workflow-ai-json">{JSON.stringify(patch, null, 2)}</pre> : null}
        <footer>
          <button type="button" onClick={onReject}>Reject</button>
          <button type="button" className="primary" onClick={onConfirm}>Apply</button>
        </footer>
      </div>
    </div>
  );
}
