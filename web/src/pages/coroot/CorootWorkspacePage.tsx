import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";

import { fetchCorootConfig } from "@/api/coroot";
import { useRegisterAppShellWorkspace } from "@/app/AppShellChromeContext";
import { CorootSidebar } from "@/pages/coroot/CorootSidebar";
import { CorootWorkspaceErrorPanel } from "@/pages/coroot/CorootWorkspaceErrorPanel";
import { fromCorootRouteMessage, toCorootIframePath } from "@/pages/coroot/corootRoutes";

export function CorootWorkspacePage() {
  const location = useLocation();
  const navigate = useNavigate();
  const { projectId = "default" } = useParams();
  const [frameKey, setFrameKey] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [timedOut, setTimedOut] = useState(false);
  const [originUrl, setOriginUrl] = useState<string | undefined>();
  const iframeSrc = useMemo(() => toCorootIframePath(location), [location]);
  const sidebar = useMemo(
    () =>
      ({ collapsed, toggleCollapsed }: { collapsed: boolean; toggleCollapsed: () => void }) => (
        <CorootSidebar collapsed={collapsed} onToggleCollapsed={toggleCollapsed} />
      ),
    [],
  );

  useRegisterAppShellWorkspace(
    useMemo(
      () => ({
        mode: "coroot",
        hideHeader: true,
        mainClassName: "overflow-hidden",
        sidebar,
      }),
      [sidebar],
    ),
  );

  useEffect(() => {
    let cancelled = false;
    fetchCorootConfig()
      .then((config) => {
        if (cancelled) return;
        if (!config.configured) {
          navigate("/coroot/config", { replace: true });
          return;
        }
        if (config.baseUrl) setOriginUrl(`${config.baseUrl.replace(/\/$/, "")}${iframeSrc.replace(/^\/_coroot/, "")}`);
      })
      .catch(() => {
        if (!cancelled) setOriginUrl(undefined);
      });
    return () => {
      cancelled = true;
    };
  }, [iframeSrc, navigate]);

  useEffect(() => {
    setLoaded(false);
    setTimedOut(false);
    const timeoutId = window.setTimeout(() => {
      setTimedOut(true);
    }, 15000);
    return () => window.clearTimeout(timeoutId);
  }, [iframeSrc, frameKey]);

  useEffect(() => {
    function onMessage(event: MessageEvent) {
      if (event.origin !== window.location.origin) return;
      const nextPath = fromCorootRouteMessage(event.data);
      if (!nextPath) return;
      if (nextPath !== `${location.pathname}${location.search}`) navigate(nextPath, { replace: true });
    }

    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [location.pathname, location.search, navigate]);

  return (
    <section className="relative h-full min-h-0 bg-white" data-testid="coroot-workspace">
      <iframe
        key={frameKey}
        title="Coroot"
        src={iframeSrc}
        className="h-full w-full border-0 bg-white"
        onLoad={() => {
          setLoaded(true);
          setTimedOut(false);
        }}
      />
      {timedOut && !loaded ? (
        <CorootWorkspaceErrorPanel
          message={`项目 ${projectId} 的 Coroot 嵌入页超过 15 秒未完成加载。`}
          originUrl={originUrl}
          onRetry={() => setFrameKey((current) => current + 1)}
        />
      ) : null}
    </section>
  );
}
