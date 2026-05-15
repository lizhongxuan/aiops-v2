import { Navigate, useRoutes } from "react-router-dom";

import { AppShell } from "@/app/AppShell";
import { routeInventory } from "@/app/navigation";
import { ChatPage } from "@/chat/ChatPage";
import { AgentProfilePage } from "@/pages/AgentProfilePage";
import { ApprovalManagementPage } from "@/pages/ApprovalManagementPage";
import { CapabilityCenterPage } from "@/pages/CapabilityCenterPage";
import { CorootOverviewPage } from "@/pages/CorootOverviewPage";
import { ERPHealthPage } from "@/pages/ERPHealthPage";
import { GeneratorWorkshopPage } from "@/pages/GeneratorWorkshopPage";
import { HostsPage } from "@/pages/HostsPage";
import { IncidentListPage } from "@/pages/IncidentListPage";
import { IncidentWorkbenchPage } from "@/pages/IncidentWorkbenchPage";
import { LabEnvironmentPage } from "@/pages/LabEnvironmentPage";
import { LLMConfigPage } from "@/pages/LLMConfigPage";
import { McpCatalogPage } from "@/pages/McpCatalogPage";
import { McpServersPage } from "@/pages/McpServersPage";
import { OpsGraphPage } from "@/pages/OpsGraphPage";
import { OpsManualsPage } from "@/pages/OpsManualsPage";
import { PlaceholderPage } from "@/pages/PlaceholderPage";
import { PostmortemPage } from "@/pages/PostmortemPage";
import { PromptTracePage } from "@/pages/PromptTracePage";
import { ProtocolWorkspacePage } from "@/pages/ProtocolWorkspacePage";
import { RunbookCatalogPage } from "@/pages/RunbookCatalogPage";
import { RunbookDetailPage } from "@/pages/RunbookDetailPage";
import { RunnerStudioPage } from "@/pages/RunnerStudioPage";
import { ScriptConfigPage } from "@/pages/ScriptConfigPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { SkillCatalogPage } from "@/pages/SkillCatalogPage";
import { TerminalPage } from "@/pages/TerminalPage";
import { UICardManagementPage } from "@/pages/UICardManagementPage";

function placeholderElement(title: string, description: string, routePath: string) {
  return <PlaceholderPage title={title} description={description} routePath={routePath} />;
}

const concreteRoutes: Record<string, React.ReactNode> = {
  "/protocol": <ProtocolWorkspacePage />,
  "/erp": <ERPHealthPage />,
  "/opsgraph": <OpsGraphPage />,
  "/incidents": <IncidentListPage />,
  "/incidents/:incidentId": <IncidentWorkbenchPage />,
  "/runbooks": <RunbookCatalogPage />,
  "/runbooks/:runbookId": <RunbookDetailPage />,
  "/runner": <RunnerStudioPage />,
  "/runner/:workflowName": <RunnerStudioPage />,
  "/terminal/:hostId": <TerminalPage />,
  "/postmortems/:postmortemId": <PostmortemPage />,
  "/settings": <SettingsPage />,
  "/settings/llm": <LLMConfigPage />,
  "/settings/hosts": <HostsPage />,
  "/settings/ops-manuals": <OpsManualsPage />,
  "/settings/experience-packs": <OpsManualsPage />,
  "/settings/agent": <AgentProfilePage />,
  "/settings/skills": <SkillCatalogPage />,
  "/settings/mcp": <McpCatalogPage />,
  "/mcp": <McpServersPage />,
  "/approval-management": <ApprovalManagementPage />,
  "/capability-center": <CapabilityCenterPage />,
  "/ui-cards": <UICardManagementPage />,
  "/script-configs": <ScriptConfigPage />,
  "/coroot": <CorootOverviewPage />,
  "/lab": <LabEnvironmentPage />,
  "/generator": <GeneratorWorkshopPage />,
  "/debug/prompts": <PromptTracePage />,
};

export function AppRouter() {
  return useRoutes([
    {
      path: "/",
      element: <AppShell />,
      children: [
        {
          index: true,
          element: <ChatPage />,
        },
        {
          path: "hosts",
          element: <Navigate to="/settings/hosts" replace />,
        },
        {
          path: "experience-packs",
          element: <Navigate to="/settings/experience-packs" replace />,
        },
        ...routeInventory
          .filter((route) => route.path !== "/")
          .map((route) => ({
            path: route.path.replace(/^\//, ""),
            element: concreteRoutes[route.path] || placeholderElement(route.title, route.description, route.path),
          })),
      ],
    },
  ]);
}
