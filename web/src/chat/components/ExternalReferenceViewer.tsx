import { useEffect, useId, useState } from "react";
import { AlertTriangle, CheckCircle2, ChevronDown, ChevronUp, Database, FileWarning } from "lucide-react";

import {
  getExternalReference,
  verifyExternalReferenceDigest,
  type ExternalReferenceContent,
} from "@/api/externalReferences";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

type ExternalReferenceViewerProps = {
  referenceId: string;
  summary?: string;
  title?: string;
  loadReference?: (referenceId: string) => Promise<ExternalReferenceContent>;
};

type LoadState = "idle" | "loading" | "loaded" | "failed";

export function ExternalReferenceViewer({
  referenceId,
  summary = "",
  title = "",
  loadReference = getExternalReference,
}: ExternalReferenceViewerProps) {
  const panelId = useId();
  const [open, setOpen] = useState(false);
  const [state, setState] = useState<LoadState>("idle");
  const [reference, setReference] = useState<ExternalReferenceContent | null>(null);
  const [error, setError] = useState("");
  const [digestValid, setDigestValid] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (!open || state !== "loading") return undefined;

    loadReference(referenceId)
      .then(async (nextReference) => {
        const digestResult = await verifyExternalReferenceDigest(nextReference);
        if (cancelled) return;
        setReference(nextReference);
        setDigestValid(digestResult);
        setState("loaded");
      })
      .catch((loadError) => {
        if (cancelled) return;
        setError(loadError instanceof Error ? loadError.message : String(loadError || "unknown error"));
        setReference(null);
        setDigestValid(null);
        setState("failed");
      });

    return () => {
      cancelled = true;
    };
  }, [loadReference, open, referenceId, state]);

  function toggleOpen() {
    const nextOpen = !open;
    setOpen(nextOpen);
    if (nextOpen && (state === "idle" || state === "failed")) {
      setState("loading");
    }
  }

  const displayTitle = title || reference?.title || referenceId;
  const displaySummary = summary || reference?.summary || "";
  const hasContent = Boolean(reference?.content.trim());
  const isUnknownKind = reference?.kind === "unknown";

  return (
    <div className="mt-2 rounded-md border border-zinc-200 bg-white px-2 py-2 text-xs text-zinc-700" data-testid="external-reference-viewer">
      <div className="flex flex-wrap items-center gap-2">
        <Database className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
        <span className="min-w-0 flex-1 truncate font-medium text-zinc-800">{displayTitle}</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 rounded-md"
          aria-controls={panelId}
          aria-expanded={open}
          onClick={toggleOpen}
        >
          {open ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          查看原始证据
        </Button>
      </div>
      {displaySummary ? <p className="mt-1 break-words leading-5 text-zinc-500">{displaySummary}</p> : null}
      {open ? (
        <div id={panelId} className="mt-2 border-t border-zinc-100 pt-2">
          {state === "loading" ? <div className="text-zinc-500">正在读取原始证据</div> : null}
          {state === "failed" ? (
            <div className="rounded-md border border-red-200 bg-red-50 px-2 py-1.5 text-red-800" role="status">
              <div className="flex items-center gap-1.5 font-medium">
                <AlertTriangle className="h-3.5 w-3.5" />
                原始证据读取失败
              </div>
              {error ? <div className="mt-1 break-words text-red-700">{error}</div> : null}
            </div>
          ) : null}
          {state === "loaded" && reference ? (
            <div className="space-y-2">
              <div className="flex flex-wrap gap-1.5">
                <Badge variant="outline" className="bg-zinc-50 text-zinc-600">
                  {reference.kind}
                </Badge>
                {reference.contentType ? (
                  <Badge variant="outline" className="bg-zinc-50 text-zinc-600">
                    {reference.contentType}
                  </Badge>
                ) : null}
                {reference.bytes ? (
                  <Badge variant="outline" className="bg-zinc-50 text-zinc-600">
                    {reference.bytes} bytes
                  </Badge>
                ) : null}
                {digestValid === true ? (
                  <Badge variant="outline" className="bg-emerald-50 text-emerald-700">
                    <CheckCircle2 className="h-3 w-3" />
                    digest verified
                  </Badge>
                ) : null}
                {digestValid === false ? (
                  <Badge variant="outline" className="bg-red-50 text-red-700">
                    <FileWarning className="h-3 w-3" />
                    digest mismatch
                  </Badge>
                ) : null}
                {isUnknownKind ? (
                  <Badge variant="outline" className="bg-amber-50 text-amber-700">
                    unknown kind
                  </Badge>
                ) : null}
              </div>
              {hasContent ? (
                <pre className="max-h-72 overflow-auto rounded-md border border-zinc-200 bg-zinc-950 px-3 py-2 text-xs leading-5 text-zinc-100">
                  <code className="break-words whitespace-pre-wrap">{reference.content}</code>
                </pre>
              ) : (
                <div className="rounded-md border border-amber-200 bg-amber-50 px-2 py-1.5 text-amber-900">
                  没有可展示的原始内容
                </div>
              )}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
