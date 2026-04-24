<script setup>
import { computed } from "vue";
import { useAppStore } from "../store";
import { useMessage } from "naive-ui";
import { getOAuthStartUrl, login as loginApi, logout as logoutApi } from "../api/auth";

const emit = defineEmits(["close"]);
const store = useAppStore();
const msg = useMessage();

const loginResultMessages = {
  success: "GPT 登录成功，当前页面会自动恢复登录态。",
  oauth_not_configured: "服务端还没有配置 OAuth，请改用本机 ChatGPT 登录或补齐 OAuth 配置。",
  missing_code: "登录回调缺少 code 参数，请重新发起登录。",
  invalid_state: "登录状态校验失败，请重新点击 Connect GPT。",
  exchange_failed: "OAuth code 换取 token 失败，请检查服务端 OAuth 配置。",
  codex_login_failed: "已完成 OAuth，但注入 Codex 登录态失败，请查看服务端日志。",
};

const loginHint = computed(() => {
  const params = new URLSearchParams(window.location.search);
  return params.get("login");
});

const loginHintText = computed(() => {
  if (!loginHint.value) return "";
  return loginResultMessages[loginHint.value] || `登录结果: ${loginHint.value}`;
});

const loginHintState = computed(() => {
  if (!loginHint.value) return "";
  return loginHint.value === "success" ? "hint" : "error-text";
});

async function login() {
  store.errorMessage = "";
  try {
    const data = await loginApi(store.authForm);
    await store.fetchState();
    if (data.result && !["success", "oauth_started"].includes(data.result)) {
      store.errorMessage = loginResultMessages[data.result] || `登录失败：${data.result}`;
    }
    if (data.authUrl) {
      window.open(data.authUrl, "_blank", "noopener,noreferrer");
    }
    emit("close");
  } catch (e) {
    msg.error("Network error during login");
  }
}

async function logout() {
  await logoutApi();
  store.errorMessage = "";
  await store.fetchState();
}

function startConfiguredOAuth() {
  window.location.href = getOAuthStartUrl();
}
</script>

<template>
  <n-modal
    :show="true"
    preset="card"
    title="GPT Status & Login"
    :bordered="false"
    style="width: 480px; max-width: 90vw;"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <div class="login-container">
      <div class="status-box" :class="{ connected: store.snapshot.auth.connected }">
        <span class="status-dot"></span>
        {{ store.snapshot.auth.connected ? "已连接 GPT" : "未连接" }}
      </div>

      <p v-if="loginHintText" class="login-hint" :class="loginHintState">{{ loginHintText }}</p>
      
      <div v-if="store.snapshot.auth.email" class="info-row">
        <span class="label">账号</span>
        <span class="value">{{ store.snapshot.auth.email }}</span>
      </div>
      <div v-if="store.snapshot.auth.mode" class="info-row">
        <span class="label">模式</span>
        <span class="value">{{ store.snapshot.auth.mode }}</span>
      </div>
      <div v-if="store.snapshot.auth.planType" class="info-row">
        <span class="label">计划</span>
        <n-tag size="small">{{ store.snapshot.auth.planType }}</n-tag>
      </div>
      
      <p v-if="store.snapshot.auth.pending" class="hint-text text-amber">正在等待浏览器认证完成...</p>
      <p v-if="store.snapshot.auth.lastError || store.errorMessage" class="error-text">
        {{ store.snapshot.auth.lastError || store.errorMessage }}
      </p>

      <div class="form-row" v-if="store.snapshot.config.oauthConfigured">
        <n-button type="primary" block @click="startConfiguredOAuth">使用已配置 OAuth 登录</n-button>
      </div>

      <n-divider />

      <n-form label-placement="top">
        <n-form-item label="登录模式">
          <n-select
            v-model:value="store.authForm.mode"
            :options="[
              { label: 'ChatGPT 登录', value: 'chatgpt' },
              { label: '外部 Auth Tokens', value: 'chatgptAuthTokens' },
              { label: 'API Key', value: 'apiKey' },
            ]"
          />
        </n-form-item>

        <template v-if="store.authForm.mode === 'chatgptAuthTokens'">
          <n-form-item label="Access Token">
            <n-input v-model:value="store.authForm.accessToken" type="textarea" :rows="3" />
          </n-form-item>
          <n-form-item label="ChatGPT Account ID">
            <n-input v-model:value="store.authForm.chatgptAccountId" />
          </n-form-item>
          <n-form-item label="Plan Type">
            <n-input v-model:value="store.authForm.chatgptPlanType" placeholder="plus / pro / team" />
          </n-form-item>
          <n-form-item label="Email (Optional)">
            <n-input v-model:value="store.authForm.email" />
          </n-form-item>
        </template>

        <template v-if="store.authForm.mode === 'apiKey'">
          <n-form-item label="API Key">
            <n-input v-model:value="store.authForm.apiKey" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item label="Email (Optional)">
            <n-input v-model:value="store.authForm.email" />
          </n-form-item>
        </template>
      </n-form>

      <div class="actions">
        <n-button v-if="store.snapshot.auth.connected" @click="logout">注销</n-button>
        <n-button type="primary" style="flex: 1" @click="login">
          {{ store.snapshot.auth.connected ? "更换账号" : "Connect GPT" }}
        </n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.login-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.status-box {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  background: #f1f5f9;
  font-weight: 500;
  font-size: 14px;
  color: #475569;
}

.status-box.connected {
  background: #f0fdf4;
  color: #166534;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.status-box.connected .status-dot {
  background: #22c55e;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 14px;
}

.info-row .label {
  color: #64748b;
}

.info-row .value {
  font-weight: 500;
  color: #0f172a;
}

.actions {
  display: flex;
  gap: 12px;
  margin-top: 8px;
}

.error-text {
  color: #ef4444;
  font-size: 13px;
  margin: 0;
}

.hint-text {
  color: #64748b;
  font-size: 13px;
  margin: 0;
}

.text-amber {
  color: #d97706;
}
</style>
