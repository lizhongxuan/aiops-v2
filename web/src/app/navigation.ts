import {
  Activity,
  AlertTriangle,
  BookOpen,
  Bot,
  Boxes,
  Cable,
  FileCog,
  FlaskConical,
  LayoutGrid,
  MessageSquare,
  Network,
  ScrollText,
  Server,
  Settings,
  TerminalSquare,
  Wrench,
} from "lucide-react";

import type { LucideIcon } from "lucide-react";

export type NavigationItem = {
  path: string;
  title: string;
  description: string;
  icon: LucideIcon;
  nav?: boolean;
};

export type NavigationSection = {
  title: string;
  items: NavigationItem[];
};

const coreItems: NavigationItem[] = [
  { path: "/", title: "单机会话", description: "AI Chat", icon: MessageSquare, nav: true },
  { path: "/protocol", title: "协作工作台", description: "复杂运维 AI Chat", icon: ScrollText, nav: true },
  { path: "/incidents", title: "Incidents", description: "Incident list", icon: AlertTriangle, nav: true },
  { path: "/erp", title: "ERP", description: "ERP health workbench", icon: Activity, nav: true },
  { path: "/opsgraph", title: "OpsGraph", description: "Service graph", icon: Network, nav: true },
  { path: "/runbooks", title: "Runbooks", description: "Runbook catalog", icon: BookOpen, nav: true },
  { path: "/runner", title: "Runner", description: "Workflow editor", icon: Boxes, nav: true },
  { path: "/runner/:workflowName", title: "Runner Workflow", description: "Workflow detail", icon: Boxes },
  { path: "/terminal/:hostId", title: "Terminal", description: "Host terminal", icon: TerminalSquare },
  { path: "/postmortems/:postmortemId", title: "Postmortem", description: "Postmortem workspace", icon: FileCog },
];

const adminItems: NavigationItem[] = [
  { path: "/settings", title: "Settings", description: "Web settings", icon: Settings, nav: true },
  { path: "/settings/llm", title: "LLM Config", description: "Model and provider config", icon: Bot },
  { path: "/settings/hosts", title: "Hosts", description: "Host inventory", icon: Server },
  { path: "/settings/experience-packs", title: "Experience Packs", description: "Experience packs", icon: LayoutGrid },
  { path: "/settings/agent", title: "Agent Profile", description: "Agent profile editor", icon: Bot },
  { path: "/settings/skills", title: "Skills", description: "Skill catalog", icon: Wrench },
  { path: "/settings/mcp", title: "MCP Catalog", description: "Catalog bindings", icon: Cable },
  { path: "/mcp", title: "MCP Servers", description: "Server runtime management", icon: Cable, nav: true },
  { path: "/approval-management", title: "Approvals", description: "Approval queue", icon: AlertTriangle, nav: true },
  { path: "/capability-center", title: "Capability Center", description: "Capability registry", icon: LayoutGrid, nav: true },
];

const toolsItems: NavigationItem[] = [
  { path: "/ui-cards", title: "UI Cards", description: "Card registry", icon: LayoutGrid },
  { path: "/script-configs", title: "Script Configs", description: "Script configuration", icon: FileCog },
  { path: "/coroot", title: "Coroot", description: "Coroot overview", icon: Activity, nav: true },
  { path: "/lab", title: "Lab", description: "Lab environment", icon: FlaskConical },
  { path: "/generator", title: "Generator", description: "Generator workshop", icon: Wrench },
  { path: "/debug/prompts", title: "Prompt Traces", description: "Prompt trace viewer", icon: ScrollText, nav: true },
];

export const navigationSections: NavigationSection[] = [
  { title: "Workspace", items: coreItems },
  { title: "Administration", items: adminItems },
  { title: "Tools", items: toolsItems },
];

export const routeInventory: NavigationItem[] = [
  ...coreItems,
  ...adminItems,
  ...toolsItems,
  { path: "/incidents/:incidentId", title: "Incident Detail", description: "Incident workbench", icon: AlertTriangle },
  { path: "/runbooks/:runbookId", title: "Runbook Detail", description: "Runbook detail", icon: BookOpen },
];
