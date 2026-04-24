<script setup>
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { onBeforeRouteLeave, useRouter } from "vue-router";
import { ArrowLeftIcon, RefreshCcwIcon, SaveIcon } from "lucide-vue-next";
import { useAppStore } from "../store";
import { NSelect, NSwitch } from "naive-ui";

const store = useAppStore();
const router = useRouter();

const draft = reactive(deepClone(store.activeAgentProfile || store.agentProfiles[0] || {}));
const baseline = ref("");
const promptExpanded = ref(true);
const skillSearch = ref("");
const skillStatusFilter = ref("all");
const selectedSkillCatalogId = ref("");
const mcpSearch = ref("");
const mcpStatusFilter = ref("all");
const selectedMcpCatalogId = ref("");
const importInputRef = ref(null);

function deepClone(value) {
  return JSON.parse(JSON.stringify(value || {}));
}

function syncDraft(profile) {
  const next = normalizeDraftProfile(profile);
  for (const key of Object.keys(draft)) {
    delete draft[key];
  }
  Object.assign(draft, next);
  baseline.value = JSON.stringify(next);
}

function normalizeDraftProfile(profile) {
  const next = deepClone(profile);
  next.skills = (next.skills || []).map((item) => ({
    ...item,
    enabled: item?.enabled !== false,
    activationMode: normalizeSkillActivationMode(item?.activationMode, item?.enabled),
  }));
  next.mcps = (next.mcps || []).map((item) => ({
    ...item,
    enabled: item?.enabled !== false,
    permission: normalizeMcpPermission(item?.permission),
  }));
  return next;
}

function normalizeSkillActivationMode(value, enabled = true) {
  const mode = String(value || "").trim().toLowerCase();
  if (mode === "default" || mode === "default_enabled" || mode === "default-enabled") return "default_enabled";
  if (mode === "explicit" || mode === "explicit_only" || mode === "explicit-only") return "explicit_only";
  if (mode === "disabled") return "disabled";
  return enabled === false ? "disabled" : "default_enabled";
}

function normalizeMcpPermission(value) {
  const permission = String(value || "").trim().toLowerCase();
  if (permission === "readwrite" || permission === "read-write") return "readwrite";
  if (permission === "readonly" || permission === "read-only") return "readonly";
  return "readonly";
}

function normalizeSearchText(value) {
  return String(value || "").trim().toLowerCase();
}

function matchesSearchText(text, query) {
  if (!query) return true;
  return normalizeSearchText(text).includes(query);
}

function skillStatusLabel(item) {
  if (!item?.enabled) return "disabled";
  const mode = normalizeSkillActivationMode(item.activationMode, item.enabled);
  return mode === "disabled" ? "disabled" : mode;
}

function skillStatusText(item) {
  const mode = skillStatusLabel(item);
  return mode === "disabled" ? "disabled" : `enabled · ${mode}`;
}

function skillModeDescription(mode) {
  switch (normalizeSkillActivationMode(mode)) {
    case "default_enabled":
      return "默认启用，跟随 profile 的常规开放状态。";
    case "explicit_only":
      return "仅在显式触发时启用。";
    case "disabled":
      return "当前不启用。";
    default:
      return "";
  }
}

function mcpStatusText(item) {
  return item?.enabled ? `enabled · ${normalizeMcpPermission(item?.permission)}` : "disabled";
}

function mcpPermissionDescription(permission) {
  switch (normalizeMcpPermission(permission)) {
    case "readonly":
      return "只读访问，默认不允许写入。";
    case "readwrite":
      return "读写访问，允许变更型操作。";
    default:
      return "";
  }
}

function normalizeCatalogSkill(item) {
  const enabled = item?.defaultEnabled === true || item?.enabled === true;
  return {
    id: String(item?.id || ""),
    name: String(item?.name || item?.id || ""),
    description: String(item?.description || ""),
    source: String(item?.source || "local"),
    defaultEnabled: enabled,
    defaultActivationMode: normalizeSkillActivationMode(item?.defaultActivationMode ?? item?.activationMode, enabled),
  };
}

function normalizeCatalogMcp(item) {
  const enabled = item?.defaultEnabled === true || item?.enabled === true;
  return {
    id: String(item?.id || ""),
    name: String(item?.name || item?.id || ""),
    type: String(item?.type || "stdio"),
    source: String(item?.source || "local"),
    defaultEnabled: enabled,
    permission: normalizeMcpPermission(item?.permission),
    requiresExplicitUserApproval: Boolean(item?.requiresExplicitUserApproval),
  };
}

function buildSkillBindingFromCatalog(item) {
  const normalized = normalizeCatalogSkill(item);
  return {
    id: normalized.id,
    name: normalized.name,
    description: normalized.description,
    source: normalized.source,
    enabled: normalized.defaultEnabled,
    activationMode: normalized.defaultActivationMode,
  };
}

function buildMcpBindingFromCatalog(item) {
  const normalized = normalizeCatalogMcp(item);
  return {
    id: normalized.id,
    name: normalized.name,
    type: normalized.type,
    source: normalized.source,
    enabled: normalized.defaultEnabled,
    permission: normalized.permission,
    requiresExplicitUserApproval: normalized.requiresExplicitUserApproval,
  };
}

function skillRowMatches(item) {
  const query = normalizeSearchText(skillSearch.value);
  const status = skillStatusFilter.value;
  const mode = normalizeSkillActivationMode(item?.activationMode, item?.enabled);
  const matchesStatus =
    status === "all" ||
    (status === "enabled" && item?.enabled) ||
    (status === "disabled" && (!item?.enabled || normalizeSkillActivationMode(item?.activationMode, item?.enabled) === "disabled")) ||
    (status === "default_enabled" && mode === "default_enabled") ||
    (status === "explicit_only" && mode === "explicit_only");
  if (!matchesStatus) return false;
  return (
    matchesSearchText(item?.name, query) ||
    matchesSearchText(item?.id, query) ||
    matchesSearchText(item?.description, query) ||
    matchesSearchText(item?.source, query) ||
    matchesSearchText(mode, query)
  );
}

function mcpRowMatches(item) {
  const query = normalizeSearchText(mcpSearch.value);
  const status = mcpStatusFilter.value;
  const permission = normalizeMcpPermission(item?.permission);
  const matchesStatus =
    status === "all" ||
    (status === "enabled" && item?.enabled) ||
    (status === "disabled" && !item?.enabled) ||
    (status === "readonly" && permission === "readonly") ||
    (status === "readwrite" && permission === "readwrite");
  if (!matchesStatus) return false;
  return (
    matchesSearchText(item?.name, query) ||
    matchesSearchText(item?.id, query) ||
    matchesSearchText(item?.type, query) ||
    matchesSearchText(item?.source, query) ||
    matchesSearchText(permission, query)
  );
}

function categoryModeSummary(item) {
  return `${item.label}: ${item.mode}`;
}

function capabilitySummary(item) {
  return `${item.label}: ${item.state}`;
}

function promptTemplateContent() {
  return [
    "# 角色定义",
    "你是一个负责执行与协作的 Agent，请保持高准确性、可审计性和可回滚性。",
    "",
    "# 执行原则",
    "优先遵循当前 profile 中的权限边界，先确认上下文，再执行工具。",
    "",
    "# 安全约束",
    "遇到写入、服务重启、包管理、权限提升等高风险操作时，优先走审批或显式确认。",
    "",
    "# 输出风格",
    "回答要简洁、结构化，并明确说明结果、风险和后续建议。",
    "",
    "# 工具使用规则",
    "只使用当前 profile 允许的能力与工具，不要越权调用未启用的功能。",
  ].join("\n");
}

function normalizePromptLines(text) {
  const normalized = String(text || "").replace(/\r\n/g, "\n");
  if (!normalized) return [];
  return normalized.split("\n");
}

function buildPromptDiffRows(baseText, currentText) {
  const baseLines = normalizePromptLines(baseText);
  const currentLines = normalizePromptLines(currentText);
  const rows = [];
  const m = baseLines.length;
  const n = currentLines.length;
  const dp = Array.from({ length: m + 1 }, () => Array(n + 1).fill(0));

  for (let i = m - 1; i >= 0; i -= 1) {
    for (let j = n - 1; j >= 0; j -= 1) {
      dp[i][j] = baseLines[i] === currentLines[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  let i = 0;
  let j = 0;
  while (i < m && j < n) {
    if (baseLines[i] === currentLines[j]) {
      rows.push({
        type: "unchanged",
        text: currentLines[j],
        baseLine: i + 1,
        currentLine: j + 1,
      });
      i += 1;
      j += 1;
      continue;
    }
    if (dp[i + 1][j] >= dp[i][j + 1]) {
      rows.push({
        type: "removed",
        text: baseLines[i],
        baseLine: i + 1,
        currentLine: null,
      });
      i += 1;
    } else {
      rows.push({
        type: "added",
        text: currentLines[j],
        baseLine: null,
        currentLine: j + 1,
      });
      j += 1;
    }
  }

  while (i < m) {
    rows.push({
      type: "removed",
      text: baseLines[i],
      baseLine: i + 1,
      currentLine: null,
    });
    i += 1;
  }

  while (j < n) {
    rows.push({
      type: "added",
      text: currentLines[j],
      baseLine: null,
      currentLine: j + 1,
    });
    j += 1;
  }

  return rows;
}

function promptDiffLabel(type) {
  switch (type) {
    case "added":
      return "+";
    case "removed":
      return "-";
    default:
      return " ";
  }
}

function fieldErrorLabel(field) {
  const map = {
    name: "Profile Name",
    description: "Description",
    systemPrompt: "System Prompt",
    "systemPrompt.content": "System Prompt",
    commandPermissions: "Command Permissions",
    capabilityPermissions: "Capability Permissions",
    skills: "Skills",
    mcps: "MCP",
    runtime: "Runtime",
    riskConfirmed: "高风险确认",
  };
  return map[field] || field || "general";
}

function normalizeFieldErrors(value) {
  if (!value) return [];
  if (typeof value === "string") {
    return [{ field: "general", message: value }];
  }
  if (Array.isArray(value)) {
    return value
      .map((item, index) => {
        if (typeof item === "string") {
          return { field: "general", message: item, key: `${index}` };
        }
        if (item && typeof item === "object") {
          return {
            field: String(item.field || item.name || item.key || "general"),
            message: String(item.message || item.error || item.text || JSON.stringify(item)),
            key: String(item.field || item.name || item.key || index),
          };
        }
        return null;
      })
      .filter(Boolean);
  }
  if (typeof value === "object") {
    return Object.entries(value).flatMap(([field, detail]) => {
      if (Array.isArray(detail)) {
        return detail.map((item, index) => ({
          field,
          message: typeof item === "string" ? item : String(item?.message || item?.error || JSON.stringify(item)),
          key: `${field}-${index}`,
        }));
      }
      if (typeof detail === "string") {
        return [{ field, message: detail, key: field }];
      }
      if (detail && typeof detail === "object") {
        return [{
          field,
          message: String(detail.message || detail.error || detail.text || JSON.stringify(detail)),
          key: field,
        }];
      }
      return [];
    });
  }
  return [];
}

const activeProfile = computed(() => store.activeAgentProfile);
const defaultProfile = computed(() => {
  const targetId = String(store.activeAgentProfileId || draft.id || "main-agent");
  const defaults = Array.isArray(store.agentProfileDefaults) ? store.agentProfileDefaults : [];
  return (
    defaults.find((profile) => profile.id === targetId || profile.type === targetId) ||
    defaults[0] ||
    store.activeAgentProfile ||
    (Array.isArray(store.agentProfiles) ? store.agentProfiles[0] : null) ||
    null
  );
});
const isDirty = computed(() => JSON.stringify(draft) !== baseline.value);
const promptCharCount = computed(() => (draft.systemPrompt?.content || "").length);
const promptLineCount = computed(() => {
  const text = draft.systemPrompt?.content || "";
  if (!text) return 0;
  return text.split("\n").length;
});
const promptSections = [
  { title: "角色定义", hint: "说明 Agent 的职责、边界、身份。" },
  { title: "执行原则", hint: "说明判断顺序、优先级、协作方式。" },
  { title: "安全约束", hint: "说明审批、回退、禁止事项。" },
  { title: "输出风格", hint: "说明回答长度、结构、语言风格。" },
  { title: "工具使用规则", hint: "说明工具调用边界与失败处理。" },
];
const promptDiffRows = computed(() => buildPromptDiffRows(defaultProfile.value?.systemPrompt?.content || "", draft.systemPrompt?.content || ""));
const promptDiffStats = computed(() => {
  return promptDiffRows.value.reduce(
    (acc, row) => {
      acc[row.type] = (acc[row.type] || 0) + 1;
      return acc;
    },
    { added: 0, removed: 0, unchanged: 0 },
  );
});
const fieldErrors = computed(() => normalizeFieldErrors(store.agentProfileFieldErrors));
const hasHighRisk = computed(() => riskWarnings.value.length > 0);

const riskWarnings = computed(() => {
  const warnings = [];
  if (draft.commandPermissions?.enabled && draft.commandPermissions?.allowSudo) {
    warnings.push("命令执行已开启且允许 sudo，这会显著提高变更风险。");
  }
  const categoryPolicies = draft.commandPermissions?.categoryPolicies || [];
  const packageMutation = categoryPolicies.find((item) => item.id === "package_mutation")?.mode;
  const filesystemMutation = categoryPolicies.find((item) => item.id === "filesystem_mutation")?.mode;
  const serviceMutation = categoryPolicies.find((item) => item.id === "service_mutation")?.mode;
  if (packageMutation === "allow") {
    warnings.push("package_mutation 处于 allow，包管理相关变更会直接放行。");
  }
  if (filesystemMutation === "allow") {
    warnings.push("filesystem_mutation 处于 allow，文件系统写入会直接放行。");
  }
  if (serviceMutation === "allow") {
    warnings.push("service_mutation 处于 allow，服务启停/重启会直接放行。");
  }
  if ((draft.runtime?.sandboxMode || "") === "danger-full-access") {
    warnings.push("sandboxMode 为 danger-full-access，主 agent 会获得极宽松的执行边界。");
  }
  return warnings;
});

const dependencyWarnings = computed(() => {
  const warnings = [];
  const capabilityStates = Object.fromEntries((draft.capabilityPermissions || []).map((item) => [item.id, item.state]));
  if (capabilityStates.commandExecution === "disabled") {
    warnings.push("commandExecution disabled 后，fileChange / terminal / 审批流会失去主要执行入口。");
  } else if (capabilityStates.commandExecution === "approval_required") {
    warnings.push("commandExecution 需要审批，所有命令执行都会先进入审核链路。");
  }
  if (capabilityStates.fileChange === "disabled") {
    warnings.push("fileChange disabled 后，保存型变更只能通过其他受控路径完成。");
  }
  if (capabilityStates.terminal === "disabled") {
    warnings.push("terminal disabled 后，页面里的终端入口将不可用。");
  }
  if (capabilityStates.multiAgent === "disabled") {
    warnings.push("multiAgent disabled 后，无法并行启动子 agent 来分拆任务。");
  }
  if (capabilityStates.approval === "disabled") {
    warnings.push("approval disabled 后，审批请求入口会失效，高风险流程需要改为硬拦截。");
  }
  return warnings;
});

const writableRootsText = computed({
  get: () => (draft.commandPermissions?.allowedWritableRoots || []).join("\n"),
  set: (value) => {
    if (!draft.commandPermissions) return;
    draft.commandPermissions.allowedWritableRoots = String(value || "")
      .split("\n")
      .map((item) => item.trim())
      .filter(Boolean);
  },
});

const localPreview = computed(() => {
  const profile = draft || {};
  return {
    systemPrompt: profile.systemPrompt?.content || "",
    commandSummary: (profile.commandPermissions?.categoryPolicies || []).map(categoryModeSummary),
    capabilitySummary: (profile.capabilityPermissions || []).map(capabilitySummary),
    enabledSkills: (profile.skills || []).filter((item) => item.enabled),
    enabledMcps: (profile.mcps || []).filter((item) => item.enabled),
    runtime: profile.runtime || {},
  };
});

const preview = computed(() => (isDirty.value ? localPreview.value : store.agentProfilePreview || localPreview.value));

const structuredReadInterfaces = computed(() => [
  { name: "host.summary", description: "系统概览（hostname, uptime, load, memory, disk）" },
  { name: "host.process.top", description: "按 CPU/内存排序的进程列表" },
  { name: "host.service.status", description: "systemd 服务状态" },
  { name: "host.journal.tail", description: "systemd 日志尾部" },
  { name: "host.file.exists", description: "文件/目录存在性检查" },
  { name: "host.file.read", description: "读取文件内容" },
  { name: "host.file.search", description: "文件内容搜索" },
  { name: "host.network.listeners", description: "监听端口列表" },
  { name: "host.network.connections", description: "活跃网络连接" },
  { name: "host.package.version", description: "已安装包版本" },
  { name: "host.nginx.status", description: "Nginx 状态" },
  { name: "host.mysql.summary", description: "MySQL 摘要" },
  { name: "host.redis.summary", description: "Redis 摘要" },
  { name: "host.jvm.summary", description: "JVM 进程列表" },
]);

const controlledMutationInterfaces = computed(() => [
  { name: "service.restart", description: "重启 systemd 服务（需审批）" },
  { name: "service.stop", description: "停止 systemd 服务（需审批）" },
  { name: "config.apply", description: "写入配置文件（需审批）" },
  { name: "package.install", description: "安装系统包（需审批）" },
  { name: "package.upgrade", description: "升级系统包（需审批）" },
]);

const policySource = computed(() => {
  const profileType = String(draft.type || "").trim().toLowerCase();
  if (profileType.includes("host-agent-override") || profileType === "host_agent_override") {
    return { label: "host-agent-override", description: "当前策略来自 host-agent 覆盖配置。" };
  }
  if (profileType.includes("host-agent-default") || profileType === "host_agent_default") {
    return { label: "host-agent-default", description: "当前策略来自 host-agent 默认配置。" };
  }
  return { label: "main-agent", description: "当前策略来自主 agent 配置。" };
});
const filteredSkills = computed(() => (draft.skills || []).filter(skillRowMatches));
const filteredMcps = computed(() => (draft.mcps || []).filter(mcpRowMatches));
const availableSkillCatalog = computed(() => {
  const boundIds = new Set((draft.skills || []).map((item) => String(item?.id || "").trim()).filter(Boolean));
  return (Array.isArray(store.skillCatalog) ? store.skillCatalog : [])
    .map(normalizeCatalogSkill)
    .filter((item) => item.id && !boundIds.has(item.id));
});
const availableMcpCatalog = computed(() => {
  const boundIds = new Set((draft.mcps || []).map((item) => String(item?.id || "").trim()).filter(Boolean));
  return (Array.isArray(store.mcpCatalog) ? store.mcpCatalog : [])
    .map(normalizeCatalogMcp)
    .filter((item) => item.id && !boundIds.has(item.id));
});

function syncSelectedBindingId(targetRef, items) {
  const nextItems = Array.isArray(items) ? items : [];
  if (!nextItems.length) {
    targetRef.value = "";
    return;
  }
  if (!nextItems.some((item) => item.id === targetRef.value)) {
    targetRef.value = nextItems[0].id;
  }
}

function addSkillBinding() {
  const selected = availableSkillCatalog.value.find((item) => item.id === selectedSkillCatalogId.value) || availableSkillCatalog.value[0];
  if (!selected) return;
  draft.skills = [...(draft.skills || []), buildSkillBindingFromCatalog(selected)];
}

function removeSkillBinding(skillId) {
  const targetId = String(skillId || "").trim();
  if (!targetId) return;
  draft.skills = (draft.skills || []).filter((item) => String(item?.id || "").trim() !== targetId);
}

function addMcpBinding() {
  const selected = availableMcpCatalog.value.find((item) => item.id === selectedMcpCatalogId.value) || availableMcpCatalog.value[0];
  if (!selected) return;
  draft.mcps = [...(draft.mcps || []), buildMcpBindingFromCatalog(selected)];
}

function removeMcpBinding(mcpId) {
  const targetId = String(mcpId || "").trim();
  if (!targetId) return;
  draft.mcps = (draft.mcps || []).filter((item) => String(item?.id || "").trim() !== targetId);
}

function fillPromptTemplate() {
  const template = promptTemplateContent();
  if ((draft.systemPrompt?.content || "").trim() && !window.confirm("当前 System Prompt 已有内容，确认用推荐模板覆盖吗？")) {
    return;
  }
  if (!draft.systemPrompt) {
    draft.systemPrompt = {};
  }
  draft.systemPrompt.content = template;
  promptExpanded.value = true;
}

async function loadProfiles() {
  await store.fetchAgentProfiles();
  if (store.activeAgentProfileId) {
    await store.fetchAgentProfilePreview(store.activeAgentProfileId);
  }
  if (store.activeAgentProfile) {
    syncDraft(store.activeAgentProfile);
  }
}

async function switchProfile(profileId) {
  if (isDirty.value) {
    const confirmed = window.confirm("当前有未保存修改，确认切换 profile 吗？");
    if (!confirmed) return;
  }
  if (store.selectAgentProfile(profileId)) {
    await store.fetchAgentProfilePreview(profileId);
  }
}

async function saveProfile() {
  let riskConfirmed = false;
  if (hasHighRisk.value) {
    const confirmed = window.confirm(`当前配置包含 ${riskWarnings.value.length} 条高风险提示，确认继续保存吗？`);
    if (!confirmed) return;
    riskConfirmed = true;
  }
  const ok = await store.saveAgentProfile(draft, { riskConfirmed });
  if (!ok) return;
  if (store.activeAgentProfile) {
    syncDraft(store.activeAgentProfile);
  }
}

async function resetProfile() {
  const confirmed = window.confirm(`确认恢复 ${store.activeAgentProfileId} 的默认配置吗？`);
  if (!confirmed) return;
  const ok = await store.resetAgentProfile(store.activeAgentProfileId);
  if (!ok) return;
  if (store.activeAgentProfile) {
    syncDraft(store.activeAgentProfile);
  }
}

function discardDraft() {
  if (!store.activeAgentProfile) return;
  syncDraft(store.activeAgentProfile);
}

async function exportProfiles() {
  const result = await store.exportAgentProfiles();
  if (!result?.filename || !result?.content) return;
  const { filename, content } = result;
  const blob = new Blob([content], { type: "application/json;charset=utf-8" });
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
}

function openImportDialog() {
  importInputRef.value?.click();
}

async function handleImportFileChange(event) {
  const file = event.target?.files?.[0];
  event.target.value = "";
  if (!file) return;
  const result = await store.importAgentProfiles(file);
  if (!result?.ok) return;
  if (store.activeAgentProfile) {
    syncDraft(store.activeAgentProfile);
  }
}

function confirmLeave() {
  if (!isDirty.value) return true;
  return window.confirm("当前 Agent Profile 有未保存修改，确认离开页面吗？");
}

onBeforeRouteLeave(() => {
  if (!confirmLeave()) {
    return false;
  }
  return true;
});

function handleBeforeUnload(event) {
  if (!isDirty.value) return;
  event.preventDefault();
  event.returnValue = "";
}

watch(
  () => activeProfile.value,
  (profile) => {
    if (profile) {
      syncDraft(profile);
    }
  },
);

watch(
  () => availableSkillCatalog.value,
  (items) => {
    syncSelectedBindingId(selectedSkillCatalogId, items);
  },
  { immediate: true },
);

watch(
  () => availableMcpCatalog.value,
  (items) => {
    syncSelectedBindingId(selectedMcpCatalogId, items);
  },
  { immediate: true },
);

onMounted(() => {
  loadProfiles();
  window.addEventListener("beforeunload", handleBeforeUnload);
});

onBeforeUnmount(() => {
  window.removeEventListener("beforeunload", handleBeforeUnload);
});
</script>

<template>
  <div class="agent-profile-page" data-testid="agent-profile-page">
    <aside class="agent-sidebar">
      <button class="back-link" @click="router.push('/')">
        <ArrowLeftIcon size="16" />
        <span>返回工作区</span>
      </button>

      <div class="sidebar-card">
        <div class="sidebar-title">Agent Profiles</div>
        <div class="sidebar-subtitle">统一管理主 agent 与 host-agent 的能力配置、skills 与 MCP 绑定。</div>
        <div class="profile-list">
          <button
            v-for="profile in store.agentProfiles"
            :key="profile.id"
            class="profile-item"
            :class="{ active: profile.id === store.activeAgentProfileId }"
            @click="switchProfile(profile.id)"
            :data-testid="`profile-item-${profile.id}`"
          >
            <span class="profile-item-title">{{ profile.name }}</span>
            <span class="profile-item-meta">{{ profile.type }}</span>
          </button>
        </div>
      </div>
    </aside>

    <main class="agent-main">
      <header class="page-header">
        <div>
          <div class="eyebrow">Settings / Agent Profile</div>
          <h1>{{ draft.name || "Agent Profile" }}</h1>
          <p>{{ draft.description || "管理 system prompt、权限边界、skills 与 MCP。" }}</p>
        </div>
        <div class="header-actions">
          <button class="header-btn secondary" :disabled="store.agentProfileSaving" @click="exportProfiles" data-testid="export-profiles-btn">
            <span>导出</span>
          </button>
          <button class="header-btn secondary" :disabled="store.agentProfileSaving" @click="openImportDialog" data-testid="import-profiles-btn">
            <span>导入</span>
          </button>
          <button class="header-btn secondary" :disabled="!isDirty || store.agentProfileSaving" @click="discardDraft">取消修改</button>
          <button class="header-btn secondary" :disabled="store.agentProfileSaving" @click="resetProfile">
            <RefreshCcwIcon size="15" />
            <span>恢复默认</span>
          </button>
          <button class="header-btn primary" :disabled="!isDirty || store.agentProfileSaving" @click="saveProfile" data-testid="save-profile-btn">
            <SaveIcon size="15" />
            <span>{{ store.agentProfileSaving ? "保存中..." : "保存" }}</span>
          </button>
        </div>
      </header>

      <div v-if="store.agentProfilesError" class="page-alert error">{{ store.agentProfilesError }}</div>
      <div v-else-if="isDirty" class="page-alert warn" data-testid="dirty-warning">当前有未保存修改。切换 profile 或离开页面前建议先保存。</div>
      <input ref="importInputRef" class="visually-hidden-file-input" type="file" accept="application/json,.json" @change="handleImportFileChange" data-testid="agent-profile-import-input" />

      <div v-if="fieldErrors.length" class="section-card alert-card error-card">
        <div class="section-header">
          <h2>字段错误</h2>
          <span>{{ fieldErrors.length }} items</span>
        </div>
        <ul class="alert-list">
          <li v-for="item in fieldErrors" :key="item.key || `${item.field}-${item.message}`">
            <strong>{{ fieldErrorLabel(item.field) }}</strong>
            <span>{{ item.message }}</span>
          </li>
        </ul>
      </div>

      <div class="editor-grid">
        <section class="editor-sections">
          <div class="section-card">
            <div class="section-header">
              <h2>Profile 概览</h2>
              <span>{{ draft.type }}</span>
            </div>
            <n-form label-placement="top">
              <n-grid :cols="2" :x-gap="14" :y-gap="14">
                <n-gi>
                  <n-form-item label="Profile Name">
                    <div data-testid="profile-name-input">
                      <n-input v-model:value="draft.name" />
                    </div>
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="Profile ID">
                    <n-input :value="draft.id" readonly />
                  </n-form-item>
                </n-gi>
                <n-gi :span="2">
                  <n-form-item label="Description">
                    <n-input v-model:value="draft.description" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="Model">
                    <n-input v-model:value="draft.runtime.model" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="Reasoning">
                    <n-select v-model:value="draft.runtime.reasoningEffort" :options="[{label:'low',value:'low'},{label:'medium',value:'medium'},{label:'high',value:'high'}]" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="Approval Policy">
                    <n-input v-model:value="draft.runtime.approvalPolicy" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="Sandbox">
                    <n-select v-model:value="draft.runtime.sandboxMode" :options="[{label:'workspace-write',value:'workspace-write'},{label:'read-only',value:'read-only'},{label:'danger-full-access',value:'danger-full-access'}]" />
                  </n-form-item>
                </n-gi>
              </n-grid>
            </n-form>
          </div>

          <div v-if="riskWarnings.length" class="section-card alert-card warn-card" data-testid="risk-warning">
            <div class="section-header">
              <h2>风险提示</h2>
              <span>{{ riskWarnings.length }} items</span>
            </div>
            <ul class="alert-list">
              <li v-for="item in riskWarnings" :key="item">{{ item }}</li>
            </ul>
          </div>

          <div v-if="dependencyWarnings.length" class="section-card alert-card info-card">
            <div class="section-header">
              <h2>依赖提示</h2>
              <span>{{ dependencyWarnings.length }} items</span>
            </div>
            <ul class="alert-list">
              <li v-for="item in dependencyWarnings" :key="item">{{ item }}</li>
            </ul>
          </div>

          <div class="section-card">
            <div class="section-header">
              <h2>System Prompt</h2>
              <span>{{ promptCharCount }} chars / {{ promptLineCount }} lines</span>
            </div>
            <div class="prompt-toolbar">
              <div class="prompt-toolbar-actions">
                <button class="header-btn secondary" type="button" @click="fillPromptTemplate">填入推荐模板</button>
                <button class="header-btn secondary" type="button" @click="promptExpanded = !promptExpanded">
                  {{ promptExpanded ? "折叠 Prompt" : "展开 Prompt" }}
                </button>
              </div>
              <div class="prompt-section-hints">
                <span v-for="item in promptSections" :key="item.title" class="prompt-section-chip">
                  {{ item.title }}
                </span>
              </div>
            </div>
            <div v-if="promptExpanded" class="prompt-expanded">
              <n-form-item label="Prompt Content">
                <div data-testid="system-prompt-input">
                  <n-input v-model:value="draft.systemPrompt.content" type="textarea" :rows="12" :show-count="true" />
                </div>
              </n-form-item>
              <div class="prompt-guidance">
                <div v-for="item in promptSections" :key="item.title" class="prompt-guidance-item">
                  <div class="prompt-guidance-title">{{ item.title }}</div>
                  <div class="prompt-guidance-hint">{{ item.hint }}</div>
                </div>
              </div>
            </div>
            <div v-else class="prompt-collapsed">
              <pre class="preview-text">{{ draft.systemPrompt?.content || "当前未填写 System Prompt。" }}</pre>
              <div class="prompt-collapsed-meta">
                <span>{{ promptCharCount }} chars</span>
                <span>{{ promptLineCount }} lines</span>
              </div>
            </div>
            <n-form-item label="Notes">
              <n-input v-model:value="draft.systemPrompt.notes" />
            </n-form-item>
          </div>

          <div class="section-card">
            <div class="section-header">
              <h2>Command Permissions</h2>
              <span>{{ draft.commandPermissions?.defaultMode }}</span>
            </div>
            <n-form label-placement="top">
              <n-grid :cols="2" :x-gap="14" :y-gap="14">
                <n-gi>
                  <n-form-item label="允许执行命令">
                    <n-switch v-model:value="draft.commandPermissions.enabled" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="允许 shell wrapper">
                    <n-switch v-model:value="draft.commandPermissions.allowShellWrapper" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="允许 sudo">
                    <n-switch v-model:value="draft.commandPermissions.allowSudo" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="默认超时（秒）">
                    <n-input-number v-model:value="draft.commandPermissions.defaultTimeoutSeconds" :min="1" :max="3600" />
                  </n-form-item>
                </n-gi>
                <n-gi :span="2">
                  <n-form-item label="允许写入路径">
                    <n-input v-model:value="writableRootsText" type="textarea" :rows="3" />
                  </n-form-item>
                </n-gi>
              </n-grid>
            </n-form>

            <n-data-table
              :columns="[
                { title: '类别', key: 'label' },
                { title: '模式', key: 'mode', render: (row) => h(NSelect, { value: row.mode, options: [{label:'allow',value:'allow'},{label:'approval_required',value:'approval_required'},{label:'readonly_only',value:'readonly_only'},{label:'deny',value:'deny'}], size: 'small', style: 'width:180px', onUpdateValue: (v) => { row.mode = v; } }) },
              ]"
              :data="draft.commandPermissions.categoryPolicies"
              :row-key="(row) => row.id"
              :bordered="false"
              size="small"
            />
          </div>

          <div class="section-card">
            <div class="section-header">
              <h2>Capability Permissions</h2>
              <span>{{ draft.capabilityPermissions?.length || 0 }} capabilities</span>
            </div>
            <n-data-table
              :columns="[
                { title: '能力', key: 'label' },
                { title: '状态', key: 'state', render: (row) => h(NSelect, { value: row.state, options: [{label:'enabled',value:'enabled'},{label:'approval_required',value:'approval_required'},{label:'disabled',value:'disabled'}], size: 'small', style: 'width:180px', onUpdateValue: (v) => { row.state = v; } }) },
              ]"
              :data="draft.capabilityPermissions"
              :row-key="(row) => row.id"
              :bordered="false"
              size="small"
            />
          </div>

          <div class="section-card">
            <div class="section-header">
              <h2>Skills</h2>
              <span>{{ filteredSkills.length }} / {{ (draft.skills || []).length }} shown</span>
            </div>
            <div class="section-toolbar">
              <div class="section-toolbar-row">
                <input v-model="skillSearch" type="search" class="text-input" placeholder="搜索 skill 名称 / ID / 来源 / 描述" />
                <select v-model="skillStatusFilter" class="text-input">
                  <option value="all">全部状态</option>
                  <option value="enabled">enabled</option>
                  <option value="disabled">disabled</option>
                  <option value="default_enabled">default_enabled</option>
                  <option value="explicit_only">explicit_only</option>
                </select>
              </div>
              <div class="section-toolbar-row section-toolbar-row-attach">
                <select
                  v-model="selectedSkillCatalogId"
                  class="text-input"
                  :disabled="!availableSkillCatalog.length"
                  data-testid="skill-binding-select"
                >
                  <option v-if="!availableSkillCatalog.length" value="">没有可添加的 Skill</option>
                  <option v-for="item in availableSkillCatalog" :key="item.id" :value="item.id">
                    {{ item.name || item.id }} · {{ item.id }}
                  </option>
                </select>
                <button
                  class="header-btn secondary toolbar-btn"
                  type="button"
                  :disabled="!availableSkillCatalog.length"
                  @click="addSkillBinding"
                  data-testid="add-skill-binding-btn"
                >
                  添加绑定
                </button>
              </div>
            </div>
            <div v-if="!(draft.skills || []).length" class="empty-state">当前 profile 还没有挂载任何 skills 条目，可从上方 catalog 添加。</div>
            <div v-else-if="!(filteredSkills || []).length" class="empty-state">当前筛选条件下没有匹配的 skills。</div>
            <n-data-table
              v-if="filteredSkills.length"
              :columns="[
                { title: 'Skill', key: 'name', render: (row) => h('div', {}, [h('div', { class: 'row-title' }, row.name), h('div', { class: 'row-subtitle' }, row.description || row.id)]) },
                { title: 'Source', key: 'source', render: (row) => h('div', {}, [h('div', { class: 'row-title' }, row.source || 'local'), h('div', { class: 'row-subtitle' }, row.id)]) },
                { title: '启用', key: 'enabled', width: 80, render: (row) => h(NSwitch, { value: row.enabled, onUpdateValue: (v) => { row.enabled = v; } }) },
                { title: 'Activation', key: 'activationMode', render: (row) => h(NSelect, { value: row.activationMode, options: [{label:'default_enabled',value:'default_enabled'},{label:'explicit_only',value:'explicit_only'},{label:'disabled',value:'disabled'}], size: 'small', style: 'width:160px', onUpdateValue: (v) => { row.activationMode = v; } }) },
                { title: 'Binding', key: 'binding', width: 80, render: (row) => h('span', { 'data-testid': `remove-skill-binding-${row.id}` }, [h(NButton, { size: 'small', quaternary: true, onClick: () => removeSkillBinding(row.id) }, { default: () => '移除' })]) },
              ]"
              :data="filteredSkills"
              :row-key="(row) => row.id"
              :bordered="false"
              size="small"
            />
          </div>

          <div class="section-card">
            <div class="section-header">
              <h2>MCP</h2>
              <span>{{ filteredMcps.length }} / {{ (draft.mcps || []).length }} shown</span>
            </div>
            <div class="section-toolbar">
              <div class="section-toolbar-row">
                <input v-model="mcpSearch" type="search" class="text-input" placeholder="搜索 MCP 名称 / ID / 来源 / 类型" />
                <select v-model="mcpStatusFilter" class="text-input">
                  <option value="all">全部状态</option>
                  <option value="enabled">enabled</option>
                  <option value="disabled">disabled</option>
                  <option value="readonly">readonly</option>
                  <option value="readwrite">readwrite</option>
                </select>
              </div>
              <div class="section-toolbar-row section-toolbar-row-attach">
                <select
                  v-model="selectedMcpCatalogId"
                  class="text-input"
                  :disabled="!availableMcpCatalog.length"
                  data-testid="mcp-binding-select"
                >
                  <option v-if="!availableMcpCatalog.length" value="">没有可添加的 MCP</option>
                  <option v-for="item in availableMcpCatalog" :key="item.id" :value="item.id">
                    {{ item.name || item.id }} · {{ item.id }}
                  </option>
                </select>
                <button
                  class="header-btn secondary toolbar-btn"
                  type="button"
                  :disabled="!availableMcpCatalog.length"
                  @click="addMcpBinding"
                  data-testid="add-mcp-binding-btn"
                >
                  添加绑定
                </button>
              </div>
            </div>
            <div v-if="!(draft.mcps || []).length" class="empty-state">当前 profile 还没有挂载任何 MCP 条目，可从上方 catalog 添加。</div>
            <div v-else-if="!(filteredMcps || []).length" class="empty-state">当前筛选条件下没有匹配的 MCP。</div>
            <n-data-table
              v-if="filteredMcps.length"
              :columns="[
                { title: 'MCP', key: 'name', render: (row) => h('div', {}, [h('div', { class: 'row-title' }, row.name), h('div', { class: 'row-subtitle' }, row.type || row.id)]) },
                { title: 'Source', key: 'source', render: (row) => h('div', {}, [h('div', { class: 'row-title' }, row.source || 'local'), h('div', { class: 'row-subtitle' }, row.id)]) },
                { title: '启用', key: 'enabled', width: 80, render: (row) => h(NSwitch, { value: row.enabled, onUpdateValue: (v) => { row.enabled = v; } }) },
                { title: 'Permission', key: 'permission', render: (row) => h(NSelect, { value: row.permission, options: [{label:'readonly',value:'readonly'},{label:'readwrite',value:'readwrite'}], size: 'small', style: 'width:140px', onUpdateValue: (v) => { row.permission = v; } }) },
                { title: '显式确认', key: 'requiresExplicitUserApproval', width: 80, render: (row) => h(NSwitch, { value: row.requiresExplicitUserApproval, onUpdateValue: (v) => { row.requiresExplicitUserApproval = v; } }) },
                { title: 'Binding', key: 'binding', width: 80, render: (row) => h('span', { 'data-testid': `remove-mcp-binding-${row.id}` }, [h(NButton, { size: 'small', quaternary: true, onClick: () => removeMcpBinding(row.id) }, { default: () => '移除' })]) },
              ]"
              :data="filteredMcps"
              :row-key="(row) => row.id"
              :bordered="false"
              size="small"
            />
          </div>

          <div class="section-card" data-testid="structured-read-interfaces">
            <div class="section-header">
              <h2>Structured Read Interfaces</h2>
              <span>{{ structuredReadInterfaces.length }} tools</span>
            </div>
            <div class="interface-description">只读结构化接口，映射到预定义的安全命令，无需审批即可执行。</div>
            <table class="config-table">
              <thead>
                <tr>
                  <th>接口名称</th>
                  <th>说明</th>
                  <th>层级</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in structuredReadInterfaces" :key="item.name">
                  <td><span class="interface-name">{{ item.name }}</span></td>
                  <td>{{ item.description }}</td>
                  <td><span class="status-pill">structured_read</span></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="section-card" data-testid="controlled-mutation-interfaces">
            <div class="section-header">
              <h2>Controlled Mutation Interfaces</h2>
              <span>{{ controlledMutationInterfaces.length }} tools</span>
            </div>
            <div class="interface-description">受控变更接口，所有调用强制走审批流程。</div>
            <table class="config-table">
              <thead>
                <tr>
                  <th>接口名称</th>
                  <th>说明</th>
                  <th>层级</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in controlledMutationInterfaces" :key="item.name">
                  <td><span class="interface-name">{{ item.name }}</span></td>
                  <td>{{ item.description }}</td>
                  <td><span class="status-pill mutation">controlled_mutation</span></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="section-card" data-testid="policy-source">
            <div class="section-header">
              <h2>Policy Source</h2>
              <span>{{ policySource.label }}</span>
            </div>
            <div class="policy-source-row">
              <span class="status-pill" :class="{ mutation: policySource.label !== 'main-agent' }">{{ policySource.label }}</span>
              <span class="policy-source-description">{{ policySource.description }}</span>
            </div>
          </div>
        </section>

        <aside class="preview-panel">
          <div class="section-card sticky">
            <div class="section-header">
              <h2>生效预览</h2>
              <span v-if="store.agentProfilePreviewLoading">刷新中</span>
            </div>

            <div class="preview-group">
              <div class="preview-label">Runtime</div>
              <div class="preview-chip-row">
                <span class="preview-chip">{{ preview.runtime?.model || "gpt-5.4" }}</span>
                <span class="preview-chip">{{ preview.runtime?.reasoningEffort || "medium" }}</span>
                <span class="preview-chip">{{ preview.runtime?.approvalPolicy || "untrusted" }}</span>
              </div>
            </div>

            <div class="preview-group">
              <div class="preview-label">System Prompt</div>
              <pre class="preview-text" data-testid="preview-system-prompt">{{ preview.systemPrompt }}</pre>
            </div>

            <div class="preview-group">
              <div class="preview-label">Default Diff</div>
              <div class="preview-diff-meta">
                相对默认值 · +{{ promptDiffStats.added }} / -{{ promptDiffStats.removed }} / ={{ promptDiffStats.unchanged }}
              </div>
              <div v-if="!promptDiffRows.length" class="preview-empty">与默认值完全一致</div>
              <div v-else class="prompt-diff-list" data-testid="system-prompt-diff">
                <div
                  v-for="(row, index) in promptDiffRows"
                  :key="`${row.type}-${index}-${row.text}`"
                  class="prompt-diff-row"
                  :class="`is-${row.type}`"
                  :data-testid="`prompt-diff-row-${row.type}`"
                >
                  <span class="prompt-diff-mark">{{ promptDiffLabel(row.type) }}</span>
                  <span class="prompt-diff-line-meta">{{ row.baseLine || row.currentLine || index + 1 }}</span>
                  <span class="prompt-diff-text">{{ row.text || " " }}</span>
                </div>
              </div>
            </div>

            <div class="preview-group">
              <div class="preview-label">Command Summary</div>
              <ul class="preview-list">
                <li v-for="item in preview.commandSummary || []" :key="item">{{ item }}</li>
              </ul>
            </div>

            <div class="preview-group">
              <div class="preview-label">Capability Summary</div>
              <ul class="preview-list">
                <li v-for="item in preview.capabilitySummary || []" :key="item">{{ item }}</li>
              </ul>
            </div>

            <div class="preview-group">
              <div class="preview-label">Enabled Skills</div>
              <div v-if="!(preview.enabledSkills || []).length" class="preview-empty">暂无启用技能</div>
              <ul v-else class="preview-list">
                <li v-for="item in preview.enabledSkills" :key="item.id">{{ item.name || item.id }}</li>
              </ul>
            </div>

            <div class="preview-group">
              <div class="preview-label">Enabled MCPs</div>
              <div v-if="!(preview.enabledMcps || []).length" class="preview-empty">暂无启用 MCP</div>
              <ul v-else class="preview-list">
                <li v-for="item in preview.enabledMcps" :key="item.id">{{ item.name || item.id }}</li>
              </ul>
            </div>
          </div>
        </aside>
      </div>
    </main>
  </div>
</template>

<style scoped>
.prompt-toolbar {
  display: grid;
  gap: 0.75rem;
  margin-bottom: 0.9rem;
}

.prompt-toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.prompt-section-hints {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.prompt-section-chip {
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(15, 23, 42, 0.04);
  color: rgba(15, 23, 42, 0.74);
  padding: 0.35rem 0.7rem;
  font-size: 0.78rem;
  letter-spacing: 0.01em;
}

.prompt-guidance {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
  margin-top: 0.9rem;
}

.prompt-guidance-item {
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 14px;
  padding: 0.75rem 0.9rem;
  background: rgba(255, 255, 255, 0.72);
}

.prompt-guidance-title {
  font-size: 0.9rem;
  font-weight: 700;
  color: rgba(15, 23, 42, 0.9);
}

.prompt-guidance-hint {
  margin-top: 0.25rem;
  color: rgba(71, 85, 105, 0.92);
  font-size: 0.85rem;
  line-height: 1.5;
}

.prompt-collapsed {
  display: grid;
  gap: 0.75rem;
}

.prompt-collapsed-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  font-size: 0.8rem;
  color: rgba(71, 85, 105, 0.92);
}

.preview-diff-meta {
  margin-top: -0.15rem;
  margin-bottom: 0.55rem;
  font-size: 0.8rem;
  color: rgba(71, 85, 105, 0.92);
}

.prompt-diff-list {
  display: grid;
  gap: 0.4rem;
  max-height: 360px;
  overflow: auto;
  padding: 0.2rem 0;
}

.prompt-diff-row {
  display: grid;
  grid-template-columns: 20px 52px minmax(0, 1fr);
  gap: 0.55rem;
  align-items: start;
  padding: 0.35rem 0.55rem;
  border-radius: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.82rem;
  line-height: 1.55;
}

.prompt-diff-row.is-unchanged {
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.18);
}

.prompt-diff-row.is-added {
  background: rgba(16, 185, 129, 0.08);
  border: 1px solid rgba(16, 185, 129, 0.22);
}

.prompt-diff-row.is-removed {
  background: rgba(244, 63, 94, 0.08);
  border: 1px solid rgba(244, 63, 94, 0.2);
}

.prompt-diff-mark,
.prompt-diff-line-meta {
  color: rgba(71, 85, 105, 0.92);
}

.prompt-diff-text {
  white-space: pre-wrap;
  word-break: break-word;
  color: rgba(15, 23, 42, 0.92);
}

.error-card .alert-list {
  display: grid;
  gap: 0.45rem;
}

.error-card .alert-list li {
  display: grid;
  gap: 0.18rem;
  line-height: 1.45;
}

.error-card .alert-list strong {
  font-weight: 700;
  color: rgba(127, 29, 29, 0.95);
}

@media (max-width: 960px) {
  .prompt-guidance {
    grid-template-columns: 1fr;
  }

  .prompt-diff-row {
    grid-template-columns: 18px 40px minmax(0, 1fr);
  }
}

.interface-name {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.85rem;
  color: #bfdbfe;
}

.interface-description {
  color: #94a3b8;
  font-size: 0.85rem;
  margin-bottom: 0.5rem;
}

.status-pill.mutation {
  background: rgba(245, 158, 11, 0.18);
  color: #fde68a;
}

.policy-source-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.policy-source-description {
  color: #94a3b8;
  font-size: 0.85rem;
}
</style>

<style scoped>
.agent-profile-page {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  background:
    radial-gradient(circle at top left, rgba(34, 197, 94, 0.16), transparent 26%),
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.12), transparent 30%),
    #09111f;
  color: #e5eef8;
}

.visually-hidden-file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.agent-sidebar {
  padding: 28px 22px;
  border-right: 1px solid rgba(148, 163, 184, 0.14);
  background: rgba(8, 15, 29, 0.72);
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: none;
  background: transparent;
  color: #cbd5e1;
  cursor: pointer;
  margin-bottom: 20px;
}

.sidebar-card,
.section-card {
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(15, 23, 42, 0.8);
  border-radius: 20px;
  padding: 18px;
}

.sidebar-title,
.section-header h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.sidebar-subtitle,
.section-header span,
.row-subtitle,
.preview-empty,
.empty-state {
  color: #94a3b8;
  font-size: 13px;
}

.profile-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 18px;
}

.profile-item {
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 16px;
  padding: 12px 14px;
  background: rgba(15, 23, 42, 0.56);
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.profile-item.active {
  border-color: rgba(96, 165, 250, 0.64);
  background: rgba(30, 41, 59, 0.96);
}

.profile-item-title,
.row-title {
  display: block;
  font-weight: 600;
}

.profile-item-meta {
  display: block;
  margin-top: 4px;
  color: #94a3b8;
  font-size: 12px;
}

.agent-main {
  padding: 28px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  align-items: flex-start;
  margin-bottom: 18px;
}

.page-header h1 {
  margin: 8px 0 6px;
  font-size: 30px;
}

.page-header p,
.eyebrow {
  margin: 0;
  color: #94a3b8;
}

.eyebrow {
  text-transform: uppercase;
  font-size: 12px;
  letter-spacing: 0.08em;
}

.header-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.header-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border-radius: 999px;
  padding: 10px 16px;
  cursor: pointer;
  border: 1px solid rgba(148, 163, 184, 0.18);
}

.header-btn.primary {
  background: linear-gradient(135deg, #22c55e, #0f766e);
  color: white;
  border-color: transparent;
}

.header-btn.secondary {
  background: rgba(15, 23, 42, 0.72);
  color: #e2e8f0;
}

.header-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.toolbar-btn,
.table-action-btn {
  justify-content: center;
}

.page-alert {
  margin-bottom: 16px;
  border-radius: 14px;
  padding: 12px 14px;
  font-size: 14px;
}

.page-alert.warn {
  background: rgba(245, 158, 11, 0.14);
  color: #fde68a;
}

.page-alert.error {
  background: rgba(239, 68, 68, 0.14);
  color: #fecaca;
}

.editor-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(300px, 420px);
  gap: 18px;
}

.editor-sections {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: center;
  margin-bottom: 16px;
}

.section-toolbar {
  display: grid;
  gap: 10px;
}

.section-toolbar-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 10px;
}

.section-toolbar-row-attach {
  grid-template-columns: minmax(0, 1fr) 132px;
}

.form-grid {
  display: grid;
  gap: 14px;
}

.two-col {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.field,
.toggle-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field-span-2 {
  grid-column: span 2;
}

.text-input,
.prompt-editor,
.mini-editor,
.table-select {
  width: 100%;
  border-radius: 14px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background: rgba(2, 6, 23, 0.6);
  color: #f8fafc;
  padding: 11px 12px;
}

.prompt-editor,
.mini-editor {
  resize: vertical;
  font-family: "IBM Plex Mono", monospace;
}

.text-input.muted {
  color: #94a3b8;
}

.toggle-field {
  flex-direction: row;
  align-items: center;
  gap: 10px;
}

.toggle-field.compact {
  gap: 8px;
  font-size: 13px;
}

.inline-stack {
  display: grid;
  gap: 8px;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 6px 10px;
  background: rgba(37, 99, 235, 0.16);
  color: #bfdbfe;
  font-size: 12px;
  line-height: 1.2;
}

.status-pill.muted {
  background: rgba(148, 163, 184, 0.12);
  color: #cbd5e1;
}

.config-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 14px;
}

.config-table th,
.config-table td {
  padding: 12px 10px;
  border-top: 1px solid rgba(148, 163, 184, 0.12);
  vertical-align: top;
  text-align: left;
}

.preview-panel .sticky {
  position: sticky;
  top: 24px;
}

.preview-group {
  margin-top: 18px;
}

.preview-label {
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #94a3b8;
  margin-bottom: 8px;
}

.preview-chip-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.preview-chip {
  display: inline-flex;
  border-radius: 999px;
  padding: 6px 10px;
  background: rgba(30, 41, 59, 0.95);
  color: #bfdbfe;
  font-size: 12px;
}

.preview-text {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  border-radius: 16px;
  padding: 14px;
  background: rgba(2, 6, 23, 0.66);
  color: #dbeafe;
  max-height: 320px;
  overflow: auto;
}

.preview-list {
  margin: 0;
  padding-left: 18px;
  color: #dbeafe;
}

.alert-card {
  border-width: 1px;
}

.warn-card {
  background: linear-gradient(180deg, rgba(180, 83, 9, 0.28), rgba(15, 23, 42, 0.82));
  border-color: rgba(251, 191, 36, 0.42);
}

.info-card {
  background: linear-gradient(180deg, rgba(30, 64, 175, 0.2), rgba(15, 23, 42, 0.82));
  border-color: rgba(96, 165, 250, 0.36);
}

.alert-list {
  margin: 0;
  padding-left: 18px;
  color: #dbeafe;
  display: grid;
  gap: 8px;
}

@media (max-width: 1180px) {
  .agent-profile-page {
    grid-template-columns: 1fr;
  }

  .agent-sidebar {
    border-right: none;
    border-bottom: 1px solid rgba(148, 163, 184, 0.14);
  }

  .editor-grid {
    grid-template-columns: 1fr;
  }

  .preview-panel .sticky {
    position: static;
  }
}

@media (max-width: 760px) {
  .agent-main {
    padding: 20px;
  }

  .page-header {
    flex-direction: column;
  }

  .two-col {
    grid-template-columns: 1fr;
  }

  .field-span-2 {
    grid-column: span 1;
  }
}
</style>
