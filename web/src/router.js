import { createRouter, createWebHistory } from "vue-router";

const ChatPage = () => import("./pages/ChatPage.vue");
const HostsPage = () => import("./pages/HostsPage.vue");
const ExperiencePacksPage = () => import("./pages/ExperiencePacksPage.vue");
const ProtocolWorkspacePage = () => import("./pages/ProtocolWorkspacePage.vue");
const TerminalPage = () => import("./pages/TerminalPage.vue");
const AgentProfilePage = () => import("./pages/AgentProfilePage.vue");
const SkillCatalogPage = () => import("./pages/SkillCatalogPage.vue");
const McpCatalogPage = () => import("./pages/McpCatalogPage.vue");
const McpServersPage = () => import("./pages/McpServersPage.vue");
const ApprovalManagementPage = () => import("./pages/ApprovalManagementPage.vue");
const CapabilityCenterPage = () => import("./pages/CapabilityCenterPage.vue");
const UICardManagementPage = () => import("./pages/UICardManagementPage.vue");
const ScriptConfigPage = () => import("./pages/ScriptConfigPage.vue");
const SettingsPage = () => import("./pages/SettingsPage.vue");
const LLMConfigPage = () => import("./pages/LLMConfigPage.vue");
const CorootOverviewPage = () => import("./pages/CorootOverviewPage.vue");
const LabEnvironmentPage = () => import("./pages/LabEnvironmentPage.vue");
const GeneratorWorkshopPage = () => import("./pages/GeneratorWorkshopPage.vue");
const PromptTracePage = () => import("./pages/PromptTracePage.vue");
const IncidentListPage = () => import("./pages/IncidentListPage.vue");
const IncidentWorkbenchPage = () => import("./pages/IncidentWorkbenchPage.vue");
const ERPHealthPage = () => import("./pages/ERPHealthPage.vue");
const OpsGraphPage = () => import("./pages/OpsGraphPage.vue");
const RunbookCatalogPage = () => import("./pages/RunbookCatalogPage.vue");
const RunbookDetailPage = () => import("./pages/RunbookDetailPage.vue");
const PostmortemPage = () => import("./pages/PostmortemPage.vue");
const RunnerStudioPage = () => import("./pages/RunnerStudioPage.vue");

const routes = [
  {
    path: "/",
    name: "chat",
    component: ChatPage,
  },
  {
    path: "/hosts",
    redirect: "/settings/hosts",
  },
  {
    path: "/experience-packs",
    redirect: "/settings/experience-packs",
  },
  {
    path: "/protocol",
    name: "protocol",
    component: ProtocolWorkspacePage,
  },
  {
    path: "/incidents",
    name: "incidents",
    component: IncidentListPage,
  },
  {
    path: "/incidents/:incidentId",
    name: "incident-detail",
    component: IncidentWorkbenchPage,
    props: true,
  },
  {
    path: "/erp",
    name: "erp-health",
    component: ERPHealthPage,
  },
  {
    path: "/opsgraph",
    name: "opsgraph",
    component: OpsGraphPage,
  },
  {
    path: "/runbooks",
    name: "runbooks",
    component: RunbookCatalogPage,
  },
  {
    path: "/runbooks/:runbookId",
    name: "runbook-detail",
    component: RunbookDetailPage,
    props: true,
  },
  {
    path: "/runner",
    name: "runner-ui",
    component: RunnerStudioPage,
  },
  {
    path: "/runner/:workflowName",
    name: "runner-workflow",
    component: RunnerStudioPage,
    props: true,
  },
  {
    path: "/postmortems/:postmortemId",
    name: "postmortem",
    component: PostmortemPage,
    props: true,
  },
  {
    path: "/terminal/:hostId",
    name: "terminal",
    component: TerminalPage,
    props: true,
  },
  {
    path: "/settings",
    name: "settings",
    component: SettingsPage,
  },
  {
    path: "/settings/llm",
    name: "settings-llm",
    component: LLMConfigPage,
  },
  {
    path: "/settings/hosts",
    name: "settings-hosts",
    component: HostsPage,
  },
  {
    path: "/settings/experience-packs",
    name: "settings-experience-packs",
    component: ExperiencePacksPage,
  },
  {
    path: "/settings/agent",
    name: "settings-agent",
    component: AgentProfilePage,
  },
  {
    path: "/settings/skills",
    name: "settings-skills",
    component: SkillCatalogPage,
  },
  {
    path: "/settings/mcp",
    name: "settings-mcp",
    component: McpCatalogPage,
  },
  {
    path: "/mcp",
    name: "mcp",
    component: McpServersPage,
  },
  {
    path: "/approval-management",
    name: "approval-management",
    component: ApprovalManagementPage,
  },
  {
    path: "/capability-center",
    name: "capability-center",
    component: CapabilityCenterPage,
  },
  {
    path: "/ui-cards",
    name: "ui-cards",
    component: UICardManagementPage,
  },
  {
    path: "/script-configs",
    name: "script-configs",
    component: ScriptConfigPage,
  },
  {
    path: "/coroot",
    name: "coroot",
    component: CorootOverviewPage,
  },
  {
    path: "/lab",
    name: "lab",
    component: LabEnvironmentPage,
  },
  {
    path: "/generator",
    name: "generator",
    component: GeneratorWorkshopPage,
  },
  {
    path: "/debug/prompts",
    name: "prompt-traces",
    component: PromptTracePage,
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

export default router;
