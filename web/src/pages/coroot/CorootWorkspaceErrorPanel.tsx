import { ExternalLink, RefreshCw, Settings } from "lucide-react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";

export function CorootWorkspaceErrorPanel({
  message,
  onRetry,
  originUrl,
}: {
  message: string;
  onRetry: () => void;
  originUrl?: string;
}) {
  return (
    <div className="absolute inset-0 z-10 flex items-center justify-center bg-slate-50/90 p-6">
      <div className="w-full max-w-lg rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
        <div className="text-base font-semibold text-slate-950">Coroot 加载失败</div>
        <p className="mt-2 text-sm leading-6 text-slate-600">{message}</p>
        <div className="mt-5 flex flex-wrap gap-2">
          <Button type="button" onClick={onRetry}>
            <RefreshCw className="h-4 w-4" />
            重试
          </Button>
          <Button type="button" variant="outline" asChild>
            <Link to="/coroot/config">
              <Settings className="h-4 w-4" />
              修改配置
            </Link>
          </Button>
          {originUrl ? (
            <Button type="button" variant="outline" asChild>
              <a href={originUrl} target="_blank" rel="noreferrer">
                <ExternalLink className="h-4 w-4" />
                在 Coroot 原站打开
              </a>
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
