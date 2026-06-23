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

import { zhNavigationTitle } from "@/lib/zhLabels";

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
  { path: "/", title: zhNavigationTitle("/"), description: "AI 对话", icon: MessageSquare, nav: true },
  { path: "/protocol", title: "协作工作台", description: "复杂运维 AI Chat", icon: ScrollText },
  { path: "/incidents", title: zhNavigationTitle("/incidents"), description: "Case 队列", icon: AlertTriangle, nav: true },
  { path: "/erp", title: "ERP", description: "ERP health workbench", icon: Activity },
  { path: "/opsgraph", title: zhNavigationTitle("/opsgraph"), description: "手工运维关系图谱", icon: Network, nav: true },
  { path: "/runbooks", title: "Runbooks", description: "Runbook catalog", icon: BookOpen },
  { path: "/runner", title: zhNavigationTitle("/runner"), description: "Workflow 编排", icon: Boxes, nav: true },
  { path: "/runner/:workflowName", title: "Runner Workflow", description: "Workflow detail", icon: Boxes },
  { path: "/terminal/:hostId", title: "Terminal", description: "Host terminal", icon: TerminalSquare },
  { path: "/postmortems/:postmortemId", title: "Postmortem", description: "Postmortem workspace", icon: FileCog },
];

const adminItems: NavigationItem[] = [
  { path: "/settings", title: "设置", description: "系统设置", icon: Settings },
  { path: "/settings/llm", title: zhNavigationTitle("/settings/llm"), description: "", icon: Bot, nav: true },
  { path: "/settings/hosts", title: zhNavigationTitle("/settings/hosts"), description: "主机列表", icon: Server, nav: true },
  { path: "/settings/ops-manuals", title: zhNavigationTitle("/settings/ops-manuals"), description: "运维手册与工作流库", icon: BookOpen, nav: true },
  { path: "/settings/experience-packs", title: zhNavigationTitle("/settings/experience-packs"), description: "已迁移到运维手册", icon: LayoutGrid },
  { path: "/settings/agent", title: "Agent Profile", description: "Agent profile editor", icon: Bot },
  { path: "/settings/skills", title: "Skills", description: "Skill catalog", icon: Wrench },
  { path: "/settings/mcp", title: "MCP Catalog", description: "Catalog bindings", icon: Cable },
  { path: "/settings/connectors", title: "Connectors", description: "Connector catalog", icon: Cable },
  { path: "/mcp", title: zhNavigationTitle("/mcp"), description: "服务运行管理", icon: Cable },
  { path: "/capabilities", title: zhNavigationTitle("/capabilities"), description: "能力统一管理", icon: LayoutGrid, nav: true },
  { path: "/approval-management", title: "确认与审批", description: "审批队列", icon: AlertTriangle },
  { path: "/capability-center", title: "Capability Center", description: "Capability registry", icon: LayoutGrid },
];

const toolsItems: NavigationItem[] = [
  { path: "/agent-ui", title: zhNavigationTitle("/agent-ui"), description: "卡片产物与渲染追踪", icon: LayoutGrid, nav: true },
  { path: "/ui-cards", title: "UI Cards", description: "Card registry", icon: LayoutGrid },
  { path: "/script-configs", title: "Script Configs", description: "Script configuration", icon: FileCog },
  { path: "/lab", title: "Lab", description: "Lab environment", icon: FlaskConical },
  { path: "/generator", title: "Generator", description: "Generator workshop", icon: Wrench },
  { path: "/debug/prompts", title: zhNavigationTitle("/debug/prompts"), description: "Prompt Trace 查看器", icon: ScrollText, nav: true },
];

export const navigationSections: NavigationSection[] = [
  { title: "工作区", items: coreItems },
  { title: "管理", items: adminItems },
  { title: "工具", items: toolsItems },
];

export const routeInventory: NavigationItem[] = [
  ...coreItems,
  ...adminItems,
  ...toolsItems,
  { path: "/opsgraph/graphs", title: zhNavigationTitle("/opsgraph/graphs"), description: "图谱列表", icon: Network },
  { path: "/opsgraph/:graphId", title: "OpsGraph Editor", description: "OpsGraph editor", icon: Network },
  { path: "/incidents/:incidentId", title: "Incident Detail", description: "Incident workbench", icon: AlertTriangle },
  { path: "/runbooks/:runbookId", title: "Runbook Detail", description: "Runbook detail", icon: BookOpen },
];
