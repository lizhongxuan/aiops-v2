import { Navigate, useRoutes } from "react-router-dom";

import { AppShell } from "@/app/AppShell";
import { routeInventory } from "@/app/navigation";
import { ChatPage } from "@/chat/ChatPage";
import { AgentProfilePage } from "@/pages/AgentProfilePage";
import { AgentUICenterPage } from "@/pages/AgentUICenterPage";
import { ApprovalManagementPage } from "@/pages/ApprovalManagementPage";
import { CapabilityCenterPage } from "@/pages/CapabilityCenterPage";
import { CorootEntryPage } from "@/pages/coroot/CorootEntryPage";
import { CorootMonitorSettingsPage } from "@/pages/coroot/CorootMonitorSettingsPage";
import { CorootWorkspacePage } from "@/pages/coroot/CorootWorkspacePage";
import { ERPHealthPage } from "@/pages/ERPHealthPage";
import { GeneratorWorkshopPage } from "@/pages/GeneratorWorkshopPage";
import { HostsPage } from "@/pages/HostsPage";
import { IncidentListPage } from "@/pages/IncidentListPage";
import { IncidentWorkbenchPage } from "@/pages/IncidentWorkbenchPage";
import { LabEnvironmentPage } from "@/pages/LabEnvironmentPage";
import { LLMConfigPage } from "@/pages/LLMConfigPage";
import { McpServersPage } from "@/pages/McpServersPage";
import { OpsGraphPage } from "@/pages/OpsGraphPage";
import { OpsGraphListPage } from "@/pages/opsgraph/OpsGraphListPage";
import { OpsManualsPage } from "@/pages/OpsManualsPage";
import { PlaceholderPage } from "@/pages/PlaceholderPage";
import { PostmortemPage } from "@/pages/PostmortemPage";
import { PromptTracePage } from "@/pages/PromptTracePage";
import { ProtocolWorkspacePage } from "@/pages/ProtocolWorkspacePage";
import { RunbookCatalogPage } from "@/pages/RunbookCatalogPage";
import { RunbookDetailPage } from "@/pages/RunbookDetailPage";
import { RunnerStudioPage } from "@/pages/RunnerStudioPage";
import { RuntimeSettingsPage } from "@/pages/RuntimeSettingsPage";
import { ScriptConfigPage } from "@/pages/ScriptConfigPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { TerminalPage } from "@/pages/TerminalPage";
import { UICardManagementPage } from "@/pages/UICardManagementPage";

function placeholderElement(title: string, description: string, routePath: string) {
  return <PlaceholderPage title={title} description={description} routePath={routePath} />;
}

const concreteRoutes: Record<string, React.ReactNode> = {
  "/protocol": <ProtocolWorkspacePage />,
  "/erp": <ERPHealthPage />,
  "/opsgraph": <Navigate to="/opsgraph/graphs" replace />,
  "/opsgraph/:graphId": <OpsGraphPage />,
  "/opsgraph/graphs": <OpsGraphListPage />,
  "/incidents": <IncidentListPage />,
  "/incidents/:incidentId": <IncidentWorkbenchPage />,
  "/coroot": <CorootEntryPage />,
  "/coroot/config": <Navigate to="/settings/coroot" replace />,
  "/coroot/p/:projectId/:view?/:id?/:report?": <CorootWorkspacePage />,
  "/runbooks": <RunbookCatalogPage />,
  "/runbooks/:runbookId": <RunbookDetailPage />,
  "/runner": <RunnerStudioPage />,
  "/runner/:workflowName": <RunnerStudioPage />,
  "/terminal/:hostId": <TerminalPage />,
  "/postmortems/:postmortemId": <PostmortemPage />,
  "/settings": <SettingsPage />,
  "/settings/llm": <LLMConfigPage />,
  "/settings/runtime": <RuntimeSettingsPage />,
  "/settings/coroot": <CorootMonitorSettingsPage />,
  "/settings/hosts": <HostsPage />,
  "/settings/ops-manuals": <OpsManualsPage />,
  "/settings/experience-packs": <OpsManualsPage />,
  "/settings/agent": <AgentProfilePage />,
  "/settings/skills": <Navigate to="/capabilities" replace />,
  "/settings/mcp": <Navigate to="/capabilities" replace />,
  "/settings/connectors": <Navigate to="/capabilities" replace />,
  "/mcp": <McpServersPage />,
  "/approval-management": <ApprovalManagementPage />,
  "/capabilities": <CapabilityCenterPage />,
  "/capability-center": <Navigate to="/capabilities" replace />,
  "/agent-ui": <AgentUICenterPage />,
  "/ui-cards": <UICardManagementPage />,
  "/script-configs": <ScriptConfigPage />,
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
