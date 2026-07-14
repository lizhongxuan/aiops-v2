import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  Bell,
  CloudLightning,
  LayoutDashboard,
  LayoutGrid,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  ScrollText,
  Server,
  Settings,
  ShipWheel,
} from "lucide-react";
import { NavLink, useNavigate, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";

const corootViews = [
  { view: "applications", title: "Applications", description: "服务与应用", icon: LayoutGrid },
  { view: "incidents", title: "Incidents", description: "异常事件", icon: AlertTriangle },
  { view: "alerts", title: "Alerts", description: "告警", icon: Bell },
  { view: "map", title: "Service Map", description: "服务拓扑", icon: Network },
  { view: "traces", title: "Traces", description: "链路追踪", icon: Activity },
  { view: "logs", title: "Logs", description: "日志检索", icon: ScrollText },
  { view: "nodes", title: "Nodes", description: "节点", icon: Server },
  { view: "kubernetes", title: "Kubernetes", description: "集群资源", icon: ShipWheel },
  { view: "risks", title: "Risks", description: "风险", icon: CloudLightning },
  { view: "dashboards", title: "Dashboards", description: "仪表盘", icon: LayoutDashboard },
] as const;

export function CorootSidebar({
  collapsed = false,
  onToggleCollapsed,
}: {
  collapsed?: boolean;
  onToggleCollapsed?: () => void;
}) {
  const navigate = useNavigate();
  const { projectId = "default" } = useParams();

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={`border-b border-slate-200 py-4 ${collapsed ? "px-3" : "px-5"}`}>
        <div className={collapsed ? "flex flex-col items-center gap-2" : "flex items-center gap-2"}>
          <Button
            type="button"
            variant="outline"
            size={collapsed ? "icon" : "lg"}
            className={collapsed ? "h-8 w-8 bg-white" : "h-9 flex-1 justify-start bg-white"}
            aria-label="返回 AIOps"
            title="返回 AIOps"
            onClick={() => navigate(resolveReturnPath())}
          >
            <ArrowLeft className="h-4 w-4" />
            {collapsed ? null : <span>返回 AIOps</span>}
          </Button>
          {onToggleCollapsed ? (
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="h-8 w-8 shrink-0 bg-white"
              aria-label={collapsed ? "展开侧边栏" : "收起侧边栏"}
              title={collapsed ? "展开侧边栏" : "收起侧边栏"}
              onClick={onToggleCollapsed}
            >
              {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </Button>
          ) : null}
        </div>
      </div>

      <nav className={`flex-1 overflow-y-auto py-4 ${collapsed ? "px-2" : "px-3"}`}>
        <div className={collapsed ? "sr-only" : "px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-slate-500"}>Coroot</div>
        <div className="space-y-1">
          {corootViews.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.view}
                to={`/coroot/p/${projectId}/${item.view}`}
                title={item.title}
                aria-label={collapsed ? item.title : undefined}
                className={({ isActive }) =>
                  [
                    "flex rounded-lg transition-colors",
                    collapsed ? "items-center justify-center px-2 py-3" : "items-start gap-3 px-3 py-2.5",
                    isActive ? "bg-white text-slate-950 shadow-sm" : "text-slate-600 hover:bg-white/80 hover:text-slate-950",
                  ].join(" ")
                }
              >
                <Icon className="mt-0.5 h-4 w-4 shrink-0" />
                {collapsed ? null : (
                  <span className="min-w-0">
                    <span className="block text-sm font-medium">{item.title}</span>
                    <span className="block text-xs text-slate-500">{item.description}</span>
                  </span>
                )}
              </NavLink>
            );
          })}
        </div>
      </nav>

      <div className={`border-t border-slate-200 py-3 ${collapsed ? "px-2" : "px-3"}`}>
        {collapsed ? null : (
          <div className="mb-2 rounded-lg bg-white px-3 py-2 text-xs shadow-sm">
            <div className="font-medium text-slate-500">Project</div>
            <div className="mt-0.5 truncate font-mono text-slate-900">{projectId}</div>
          </div>
        )}
        <div className="space-y-1">
          <NavLink
            to={`/coroot/p/${projectId}/settings`}
            title="Settings"
            aria-label={collapsed ? "Settings" : undefined}
            className={({ isActive }) =>
              [
                "flex rounded-lg transition-colors",
                collapsed ? "items-center justify-center px-2 py-3" : "items-center gap-3 px-3 py-2.5",
                isActive ? "bg-white text-slate-950 shadow-sm" : "text-slate-600 hover:bg-white/80 hover:text-slate-950",
              ].join(" ")
            }
          >
            <Settings className="h-4 w-4 shrink-0" />
            {collapsed ? null : <span className="text-sm font-medium">Settings</span>}
          </NavLink>
        </div>
      </div>
    </div>
  );
}

function resolveReturnPath() {
  try {
    const saved = window.sessionStorage.getItem("aiops.coroot.returnTo");
    if (saved && saved.startsWith("/") && !saved.startsWith("/coroot")) return saved;
  } catch {
    // Keep fallback.
  }
  return "/";
}
