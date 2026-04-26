<script setup>
import { ref, onMounted, computed } from "vue";
import { fetchLlmConfig, updateLlmConfig } from "../api/llm";
import {
  KeyIcon,
  ServerIcon,
  CheckCircleIcon,
  AlertCircleIcon,
  RefreshCwIcon,
} from "lucide-vue-next";

const loading = ref(false);
const saving = ref(false);
const message = ref(null);
const messageType = ref("success");

const form = ref({
  provider: "openai",
  model: "gpt-5.4",
  apiKey: "",
  baseURL: "",
  fallbackProvider: "",
  fallbackModel: "",
  fallbackApiKey: "",
  compactModel: "gpt-5.4-mini",
});

const currentConfig = ref(null);

const providerOptions = [
  { label: "OpenAI", value: "openai" },
  { label: "Anthropic (Claude)", value: "anthropic" },
  { label: "Ollama (本地)", value: "ollama" },
];

const modelPresets = {
  openai: [
    "gpt-5.4",
    "gpt-5.4-mini",
    "gpt-4o",
    "gpt-4o-mini",
    "gpt-3.5-turbo",
    "o1",
    "o1-mini",
    "o3-mini",
  ],
  anthropic: [
    "claude-sonnet-4-20250514",
    "claude-3-5-sonnet-20241022",
    "claude-3-haiku-20240307",
    "claude-3-opus-20240229",
  ],
  ollama: [
    "qwen2.5:7b",
    "qwen2.5:14b",
    "llama3.1:8b",
    "deepseek-coder-v2:16b",
    "mistral:7b",
    "codellama:13b",
  ],
};

const modelOptions = computed(() => {
  const presets = modelPresets[form.value.provider] || [];
  return presets.map((m) => ({ label: m, value: m }));
});

const needsApiKey = computed(() => {
  return form.value.provider !== "ollama";
});

const defaultBaseURL = computed(() => {
  switch (form.value.provider) {
    case "openai":
      return "https://api.openai.com/v1";
    case "anthropic":
      return "https://api.anthropic.com";
    case "ollama":
      return "http://127.0.0.1:11434/v1";
    default:
      return "";
  }
});

async function fetchConfig() {
  loading.value = true;
  try {
    const data = await fetchLlmConfig();
    currentConfig.value = data;
    form.value.provider = data.provider || "openai";
    form.value.model = data.model || "gpt-5.4";
    form.value.baseURL = data.baseURL || "";
    form.value.fallbackProvider = data.fallbackProvider || "";
    form.value.fallbackModel = data.fallbackModel || "";
    form.value.compactModel = data.compactModel || "gpt-5.4-mini";
    // Don't populate API key — it's masked
  } catch (e) {
    showMessage("error", "加载配置失败: " + e.message);
  } finally {
    loading.value = false;
  }
}

async function saveConfig() {
  saving.value = true;
  message.value = null;
  try {
    const body = { ...form.value };
    // Don't send empty API key (would clear it)
    if (!body.apiKey) delete body.apiKey;
    if (!body.fallbackApiKey) delete body.fallbackApiKey;

    const data = await updateLlmConfig(body);
    if (data.ok) {
      showMessage("success", data.message || "配置已保存，LLM 运行时已重启。");
      await fetchConfig();
    } else {
      showMessage("warning", data.message || data.error || "保存成功但运行时未激活。");
      await fetchConfig();
    }
  } catch (e) {
    showMessage("error", "保存失败: " + e.message);
  } finally {
    saving.value = false;
  }
}

function showMessage(type, text) {
  messageType.value = type;
  message.value = text;
  setTimeout(() => {
    message.value = null;
  }, 6000);
}

function onProviderChange() {
  // Auto-set model to first preset when switching provider
  const presets = modelPresets[form.value.provider];
  if (presets && presets.length > 0) {
    form.value.model = presets[0];
  }
  // Auto-set base URL for ollama
  if (form.value.provider === "ollama") {
    form.value.baseURL = "http://127.0.0.1:11434/v1";
  } else {
    form.value.baseURL = "";
  }
}

onMounted(fetchConfig);
</script>

<template>
  <section class="llm-config-page">
    <header class="page-header">
      <div class="header-left">
        <div class="page-kicker">
          <KeyIcon :size="14" />
          <span>LLM Configuration</span>
        </div>
        <h2>模型配置</h2>
        <p>配置 LLM Provider、API Key 和模型参数。保存后运行时会自动重启。</p>
      </div>
      <div class="status-badge" :class="currentConfig?.bifrostActive ? 'active' : 'inactive'">
        <component :is="currentConfig?.bifrostActive ? CheckCircleIcon : AlertCircleIcon" :size="16" />
        <span>{{ currentConfig?.bifrostActive ? 'Runtime 运行中' : 'Runtime 未激活' }}</span>
      </div>
    </header>

    <!-- Alert message -->
    <n-alert v-if="message" :type="messageType" closable @close="message = null" style="margin-bottom: 16px;">
      {{ message }}
    </n-alert>

    <!-- Current status card -->
    <n-card v-if="currentConfig" size="small">
      <div class="status-grid">
        <div class="stat-item">
          <span class="stat-label">Provider</span>
          <strong>{{ currentConfig.provider }}</strong>
        </div>
        <div class="stat-item">
          <span class="stat-label">Model</span>
          <strong>{{ currentConfig.model }}</strong>
        </div>
        <div class="stat-item">
          <span class="stat-label">API Key</span>
          <strong>{{ currentConfig.apiKeySet ? currentConfig.apiKeyMasked : '未设置' }}</strong>
        </div>
        <div class="stat-item">
          <span class="stat-label">状态</span>
          <strong :style="{ color: currentConfig.bifrostActive ? '#16a34a' : '#dc2626' }">
            {{ currentConfig.bifrostActive ? '✓ 已连接' : '✗ 未连接' }}
          </strong>
        </div>
      </div>
    </n-card>

    <!-- Config form -->
    <n-card title="主 LLM 配置">
      <n-form label-placement="left" label-width="120">
        <n-form-item label="Provider">
          <n-select
            v-model:value="form.provider"
            :options="providerOptions"
            @update:value="onProviderChange"
          />
        </n-form-item>

        <n-form-item label="Model">
          <n-auto-complete
            v-model:value="form.model"
            :options="modelOptions"
            placeholder="输入或选择模型名称"
            clearable
          />
        </n-form-item>

        <n-form-item label="API Key" v-if="needsApiKey">
          <n-input
            v-model:value="form.apiKey"
            type="password"
            show-password-on="click"
            :placeholder="currentConfig?.apiKeySet ? '已设置 (留空保持不变)' : '输入 API Key'"
          />
        </n-form-item>

        <n-form-item label="Base URL">
          <n-input
            v-model:value="form.baseURL"
            :placeholder="defaultBaseURL || '默认 (留空使用官方地址)'"
            clearable
          />
          <template #feedback>
            <span style="font-size: 12px; color: #94a3b8;">
              自定义 API 端点。兼容 OpenAI 格式的私有网关、vLLM、DeepSeek 等可填此项。
            </span>
          </template>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Fallback config -->
    <n-card title="Fallback 配置">
      <template #header-extra>
        <n-tag size="small" type="info">可选</n-tag>
      </template>
      <n-form label-placement="left" label-width="120">
        <n-form-item label="Fallback Provider">
          <n-select
            v-model:value="form.fallbackProvider"
            :options="[{ label: '无', value: '' }, ...providerOptions]"
            placeholder="选择备用 Provider"
          />
        </n-form-item>

        <n-form-item label="Fallback Model" v-if="form.fallbackProvider">
          <n-input v-model:value="form.fallbackModel" placeholder="备用模型名称" />
        </n-form-item>

        <n-form-item label="Fallback Key" v-if="form.fallbackProvider && form.fallbackProvider !== 'ollama'">
          <n-input
            v-model:value="form.fallbackApiKey"
            type="password"
            show-password-on="click"
            placeholder="备用 Provider 的 API Key"
          />
        </n-form-item>

        <n-form-item label="压缩模型">
          <n-input v-model:value="form.compactModel" placeholder="gpt-4o-mini" />
          <template #feedback>
            <span style="font-size: 12px; color: #94a3b8;">
              用于上下文压缩的便宜模型，建议使用小模型以降低成本。
            </span>
          </template>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Actions -->
    <div class="actions">
      <n-button @click="fetchConfig" :loading="loading" quaternary>
        <template #icon><RefreshCwIcon :size="16" /></template>
        刷新
      </n-button>
      <n-button type="primary" @click="saveConfig" :loading="saving" :disabled="loading">
        保存并重启 Runtime
      </n-button>
    </div>
  </section>
</template>

<style scoped>
.llm-config-page {
  padding: 20px 24px;
  overflow-y: auto;
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.header-left h2 { margin: 6px 0 4px; font-size: 22px; }
.header-left p { margin: 0; color: #475569; font-size: 13px; line-height: 1.5; }

.page-kicker {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.status-badge.active { background: #dcfce7; color: #16a34a; }
.status-badge.inactive { background: #fef2f2; color: #dc2626; }

.status-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
@media (max-width: 720px) {
  .status-grid { grid-template-columns: repeat(2, 1fr); }
}

.stat-item { display: flex; flex-direction: column; gap: 2px; }
.stat-item strong { word-break: break-all; font-size: 13px; }
.stat-label {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #64748b;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 8px 0;
  position: sticky;
  bottom: 0;
  background: linear-gradient(to top, #f8fafc 60%, transparent);
  z-index: 5;
}
</style>
