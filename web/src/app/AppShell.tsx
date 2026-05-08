import { PanelLeftOpen } from "lucide-react";
import { matchPath, NavLink, Outlet, useLocation } from "react-router-dom";

import { useAppShellChrome } from "@/app/AppShellChromeContext";
import { navigationSections, routeInventory } from "@/app/navigation";
import { Button } from "@/components/ui/button";

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
  const active = currentTitle(location.pathname);
  const { headerContent } = useAppShellChrome();

  return (
    <div className="flex h-screen overflow-hidden bg-slate-50 text-slate-900">
      <aside className="hidden h-full w-72 shrink-0 border-r border-slate-200 bg-slate-100/80 lg:flex lg:flex-col">
        <div className="border-b border-slate-200 px-5 py-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">V2</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">AIOPS</div>
        </div>
        <nav className="flex-1 overflow-y-auto px-3 py-4">
          {navigationSections.map((section) => (
            <div key={section.title} className="mb-6">
              <div className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">{section.title}</div>
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
                        className={({ isActive }) =>
                          [
                            "flex items-start gap-3 rounded-lg px-3 py-2.5 transition-colors",
                            isActive ? "bg-white text-slate-950 shadow-sm" : "text-slate-600 hover:bg-white/80 hover:text-slate-950",
                          ].join(" ")
                        }
                      >
                        <Icon className="mt-0.5 h-4 w-4 shrink-0" />
                        <span className="min-w-0">
                          <span className="block text-sm font-medium">{item.title}</span>
                          <span className="block text-xs text-slate-500">{item.description}</span>
                        </span>
                      </NavLink>
                    );
                  })}
              </div>
            </div>
          ))}
        </nav>
      </aside>

      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <header className="shrink-0 border-b border-slate-200 bg-white/90 px-4 py-3 backdrop-blur lg:px-6">
          <div className="flex items-center justify-between gap-3 overflow-hidden">
            <div className="flex min-w-0 items-center gap-3">
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
                <div className="text-sm font-semibold text-slate-950">{active?.title ?? "AIOps Workspace"}</div>
                <div className="text-xs text-slate-500">{active?.description ?? "React shell placeholder during migration"}</div>
              </div>
            </div>
            {headerContent ? <div className="min-w-0 shrink-0 overflow-hidden">{headerContent}</div> : null}
          </div>
        </header>
        <div className="min-h-0 flex-1 overflow-hidden">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
