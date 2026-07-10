import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

import { fetchCorootConfig } from "@/api/coroot";

export function CorootEntryPage() {
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    fetchCorootConfig()
      .then((config) => {
        if (cancelled) return;
        if (!config.configured) {
          navigate("/coroot/config", { replace: true });
          return;
        }
        navigate(config.entryPath || `/coroot/p/${config.project || "default"}/applications`, { replace: true });
      })
      .catch(() => navigate("/coroot/config", { replace: true }));

    return () => {
      cancelled = true;
    };
  }, [navigate]);

  return <div className="p-6 text-sm text-slate-500">正在打开 Coroot...</div>;
}
