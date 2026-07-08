import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { matchPath, NavLink, Outlet, useLocation } from "react-router-dom";

import { useAppShellChrome } from "@/app/AppShellChromeContext";
import { navigationSections, routeInventory } from "@/app/navigation";
import { Button } from "@/components/ui/button";
import { prefetchRouteData } from "@/queries/routePrefetch";

function currentTitle(pathname: string) {
  for (const route of routeInventory) {
    if (matchPath({ path: route.path, end: !route.path.includes(":") }, pathname)) {
      return route;
    }
  }
  return null;
}

export function AppShell() {
  const location = useLocation();
  const queryClient = useQueryClient();
  const active = currentTitle(location.pathname);
  const { headerActions, headerContent, headerDescription, headerTitle } = useAppShellChrome();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try {
      return window.localStorage.getItem("aiops.sidebarCollapsed") === "true";
    } catch {
      return false;
    }
  });
  const title = headerTitle ?? active?.title ?? "AIOps Workspace";
  const description = headerDescription ?? active?.description ?? "React shell placeholder during migration";

  useEffect(() => {
    try {
      window.localStorage.setItem("aiops.sidebarCollapsed", String(sidebarCollapsed));
    } catch {
      // Ignore storage failures; the sidebar still works for the current session.
    }
  }, [sidebarCollapsed]);

  useEffect(() => {
    document.documentElement.style.setProperty("--aiops-shell-sidebar-width", sidebarCollapsed ? "5rem" : "18rem");
    document.documentElement.style.setProperty("--aiops-shell-header-height", "3.5rem");
  }, [sidebarCollapsed]);

  return (
    <div className="flex h-screen overflow-hidden bg-slate-50 text-slate-900">
      <aside
        data-testid="app-shell-sidebar"
        data-collapsed={sidebarCollapsed ? "true" : "false"}
        className={[
          "hidden h-full shrink-0 border-r border-slate-200 bg-slate-100/80 transition-[width] duration-200 lg:flex lg:flex-col",
          sidebarCollapsed ? "w-20" : "w-72",
        ].join(" ")}
      >
        <div className={`border-b border-slate-200 py-4 ${sidebarCollapsed ? "px-3" : "px-5"}`}>
          <div className={`flex items-start gap-2 ${sidebarCollapsed ? "flex-col items-center" : "justify-between"}`}>
            <div className={sidebarCollapsed ? "text-center" : "min-w-0"}>
              <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">V2</div>
              {sidebarCollapsed ? null : <div className="mt-1 text-lg font-semibold text-slate-950">AIOPS</div>}
            </div>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="h-8 w-8 shrink-0 bg-white"
              aria-label={sidebarCollapsed ? "展开侧边栏" : "收起侧边栏"}
              title={sidebarCollapsed ? "展开侧边栏" : "收起侧边栏"}
              onClick={() => setSidebarCollapsed((current) => !current)}
            >
              {sidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </Button>
          </div>
        </div>
        <nav className={`flex-1 overflow-y-auto py-4 ${sidebarCollapsed ? "px-2" : "px-3"}`}>
          {navigationSections.map((section) => (
            <div key={section.title} className={sidebarCollapsed ? "mb-4" : "mb-6"}>
              {sidebarCollapsed ? <div className="sr-only">{section.title}</div> : <div className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">{section.title}</div>}
              <div className="space-y-1">
                {section.items
                  .filter((item) => item.nav)
                  .map((item) => {
                    const Icon = item.icon;
                    return (
                      <NavLink
                        key={item.path}
                        to={item.path}
                        end={item.path === "/"}
                        title={item.title}
                        aria-label={sidebarCollapsed ? item.title : undefined}
                        onMouseEnter={() => prefetchRouteData(queryClient, item.path)}
                        onFocus={() => prefetchRouteData(queryClient, item.path)}
                        className={({ isActive }) =>
                          [
                            "flex rounded-lg transition-colors",
                            sidebarCollapsed ? "items-center justify-center px-2 py-3" : "items-start gap-3 px-3 py-2.5",
                            isActive ? "bg-white text-slate-950 shadow-sm" : "text-slate-600 hover:bg-white/80 hover:text-slate-950",
                          ].join(" ")
                        }
                      >
                        <Icon className="mt-0.5 h-4 w-4 shrink-0" />
                        {sidebarCollapsed ? null : (
                          <span className="min-w-0">
                            <span className="block text-sm font-medium">{item.title}</span>
                            {item.description ? <span className="block text-xs text-slate-500">{item.description}</span> : null}
                          </span>
                        )}
                      </NavLink>
                    );
                  })}
              </div>
            </div>
          ))}
        </nav>
      </aside>

      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <header className="relative z-[70] shrink-0 overflow-visible border-b border-slate-200 bg-white/90 px-4 py-3 backdrop-blur lg:px-6" data-testid="app-shell-header">
          {headerContent ? (
            <div className="min-w-0 overflow-visible">{headerContent}</div>
          ) : (
            <div className="flex items-center justify-between gap-3 overflow-visible">
              <div className="flex min-w-0 items-center gap-3 overflow-hidden">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="lg:hidden"
                  aria-label="navigation"
                >
                  <PanelLeftOpen className="h-4 w-4" />
                </Button>
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold text-slate-950">{title}</div>
                  {description ? <div className="truncate text-xs text-slate-500">{description}</div> : null}
                </div>
              </div>
              {headerActions ? <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 overflow-visible">{headerActions}</div> : null}
            </div>
          )}
        </header>
        <div className="min-h-0 flex-1 overflow-hidden">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
