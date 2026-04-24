<script setup>
import { computed, h } from "vue";
import { useRouter } from "vue-router";
import { useAppStore } from "../store";
import { resolveHostDisplay } from "../lib/hostDisplay";
import {
  ArrowRightIcon,
  HistoryIcon,
  KeyIcon,
  ServerIcon,
  SettingsIcon,
  UserCircleIcon,
} from "lucide-vue-next";

const router = useRouter();
const store = useAppStore();

const currentSession = computed(() => store.activeSessionSummary || null);
const workspaceSession = computed(() => {
  if (currentSession.value?.kind === "workspace") return currentSession.value;
  return store.sessionList.find((session) => session.kind === "workspace") || null;
});

const currentHostLabel = computed(() => resolveHostDisplay(store.selectedHost) || "server-local");
const missionLabel = computed(() => workspaceSession.value?.title || "协作工作台");
const missionStatus = computed(() => {
  if (store.runtime.turn.active) return phaseLabel(store.runtime.turn.phase);
  if (workspaceSession.value?.status === "completed") return "已完成";
  if (workspaceSession.value?.status === "failed") return "失败";
  return "可用";
});

const entryCards = [
  {
    key: "llm",
    title: "LLM 配置",
    subtitle: "Provider、API Key、模型选择",
    description: "配置 LLM 提供商和 API Key，支持 OpenAI、Anthropic、Ollama 等。",
    icon: KeyIcon,
    href: "/settings/llm",
  },
  {
    key: "hosts",
    title: "Hosts",
    subtitle: "主机清单、标签、会话与接入状态",
    description: "查看主机生命周期，维护执行范围，并从这里进入主机管理。",
    icon: ServerIcon,
    href: "/settings/hosts",
  },
  {
    key: "packs",
    title: "Experience Packs",
    subtitle: "经验包、playbook、版本演进",
    description: "把成功经验沉淀为可复用的运维包，并按场景加载到主 Agent。",
    icon: HistoryIcon,
    href: "/settings/experience-packs",
  },
  {
    key: "agent",
    title: "Agent Profile",
    subtitle: "System prompt、权限、skills、MCP",
    description: "维护 Agent 的执行边界与工具策略，控制它如何规划和行动。",
    icon: UserCircleIcon,
    href: "/settings/agent",
  },
];

function phaseLabel(phase) {
  switch (phase) {
    case "thinking": return "主 Agent 思考中";
    case "planning": return "主 Agent 生成计划";
    case "waiting_approval": return "等待审批";
    case "waiting_input": return "等待补充输入";
    case "executing": return "执行中";
    case "finalizing": return "结果汇总中";
    case "completed": return "已完成";
    case "failed": return "失败";
    case "aborted": return "已停止";
    default: return "待命";
  }
}

function openRoute(href) {
  router.push(href);
}
</script>

<template>
  <section class="settings-page">
    <header class="settings-hero">
      <div class="settings-hero-copy">
        <div class="settings-kicker">
          <SettingsIcon size="14" />
          <span>Settings Center</span>
        </div>
        <h2>设置</h2>
        <p>把主机、经验包和 Agent Profile 收敛到一个入口，避免再在主侧边栏里铺开一层运维目录。</p>
      </div>

      <n-grid :cols="2" :x-gap="10" :y-gap="10" style="min-width: 320px;">
        <n-gi>
          <n-card size="small">
            <template #header><span class="stat-label">当前会话</span></template>
            <strong>{{ currentSession?.title || "新建会话" }}</strong>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small">
            <template #header><span class="stat-label">工作台</span></template>
            <strong>{{ missionLabel }}</strong>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small">
            <template #header><span class="stat-label">状态</span></template>
            <strong>{{ missionStatus }}</strong>
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small">
            <template #header><span class="stat-label">当前主机</span></template>
            <strong>{{ currentHostLabel }}</strong>
          </n-card>
        </n-gi>
      </n-grid>
    </header>

    <n-grid :cols="3" :x-gap="12" :y-gap="12" responsive="screen" :item-responsive="true">
      <n-gi span="3 m:1">
        <n-card hoverable @click="openRoute('/')">
          <template #header><span class="stat-label">会话</span></template>
          <h3>{{ currentSession?.title || "暂无活动会话" }}</h3>
          <p class="context-desc">{{ currentSession?.preview || "切换到对话或工作台后，这里会显示当前会话概览。" }}</p>
        </n-card>
      </n-gi>
      <n-gi span="3 m:1">
        <n-card hoverable @click="openRoute('/protocol')">
          <template #header><span class="stat-label">工作台</span></template>
          <h3>{{ missionLabel }}</h3>
          <p class="context-desc">{{ workspaceSession?.preview || "工作台的 mission 摘要会出现在这里。" }}</p>
        </n-card>
      </n-gi>
      <n-gi span="3 m:1">
        <n-card hoverable>
          <template #header><span class="stat-label">终端</span></template>
          <h3>{{ currentHostLabel }}</h3>
          <p class="context-desc">继续沿用当前选中的主机上下文，便于从设置页直接回到执行面。</p>
        </n-card>
      </n-gi>
    </n-grid>

    <n-grid :cols="4" :x-gap="14" :y-gap="14" responsive="screen" :item-responsive="true">
      <n-gi v-for="card in entryCards" :key="card.key" span="4 m:2 l:1">
        <n-card hoverable class="settings-entry" @click="openRoute(card.href)">
          <div class="settings-entry-head">
            <component :is="card.icon" size="18" class="settings-entry-icon" />
            <ArrowRightIcon size="16" style="color: #94a3b8;" />
          </div>
          <div class="settings-entry-copy">
            <strong>{{ card.title }}</strong>
            <span class="stat-label">{{ card.subtitle }}</span>
            <p class="context-desc">{{ card.description }}</p>
          </div>
        </n-card>
      </n-gi>
    </n-grid>

    <footer class="settings-footer">
      <n-button quaternary @click="openRoute('/')">回到对话</n-button>
      <n-button quaternary @click="openRoute('/protocol')">打开工作台</n-button>
      <n-button quaternary @click="openRoute('/settings/agent')">直接进入 Agent Profile</n-button>
    </footer>
  </section>
</template>

<style scoped>
.settings-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.settings-hero {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 22px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.05);
}

.settings-hero-copy { min-width: 0; }

.settings-kicker {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.settings-hero h2 { margin: 12px 0 8px; font-size: 30px; }
.settings-hero p { margin: 0; color: #475569; line-height: 1.7; max-width: 760px; }

.stat-label {
  display: block;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.context-desc { margin: 6px 0 0; color: #64748b; line-height: 1.7; }

.settings-entry { cursor: pointer; }
.settings-entry-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.settings-entry-icon { color: #2563eb; }
.settings-entry-copy strong { display: block; font-size: 18px; margin-top: 6px; color: #0f172a; }
.settings-entry-copy p { margin: 12px 0 0; }

.settings-footer { display: flex; flex-wrap: wrap; gap: 10px; }

@media (max-width: 960px) {
  .settings-hero { flex-direction: column; }
}
</style>
