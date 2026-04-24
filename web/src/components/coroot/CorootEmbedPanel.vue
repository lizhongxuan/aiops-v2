<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from "vue";
import { XIcon, MaximizeIcon, MinimizeIcon, ExternalLinkIcon } from "lucide-vue-next";
import { fetchCorootJson } from "../../api/coroot";

const props = defineProps({
  title: { type: String, default: "Coroot" },
  url: { type: String, default: "" },
  displayMode: { type: String, default: "drawer" }, // "drawer" | "modal"
  mode: { type: String, default: "iframe" }, // "iframe" | "fetch"
  baseUrl: { type: String, default: "/api/v1/coroot/" },
});

const emit = defineEmits(["close"]);

const iframeLoading = ref(true);
const iframeError = ref(false);
const expanded = ref(false);

// fetch mode state (fallback)
const fetchLoading = ref(true);
const content = ref(null);

const iframeSrc = computed(() => {
  if (props.url) return props.url;
  return props.baseUrl;
});

function onIframeLoad() {
  iframeLoading.value = false;
  iframeError.value = false;
}

function onIframeError() {
  iframeLoading.value = false;
  iframeError.value = true;
}

// fetch mode fallback
async function fetchContent() {
  if (!props.url) return;
  fetchLoading.value = true;
  try {
    content.value = await fetchCorootJson(props.url);
  } catch (e) {
    content.value = { error: `请求异常: ${e.message}` };
  } finally {
    fetchLoading.value = false;
  }
}

function toggleExpand() {
  expanded.value = !expanded.value;
}

function close() {
  emit("close");
}

function openInNewWindow() {
  const target = iframeSrc.value || props.url;
  if (target) window.open(target, "_blank", "noopener,noreferrer");
}

function handleKeydown(e) {
  if (e.key === "Escape") close();
}

watch(
  () => props.url,
  () => {
    if (props.mode === "iframe") {
      iframeLoading.value = true;
      iframeError.value = false;
    } else {
      void fetchContent();
    }
  },
);

onMounted(() => {
  document.addEventListener("keydown", handleKeydown);
  if (props.mode !== "iframe") {
    void fetchContent();
  }
});

onBeforeUnmount(() => {
  document.removeEventListener("keydown", handleKeydown);
});
</script>

<template>
  <Teleport to="body">
    <div
      class="embed-overlay"
      :class="{ 'mode-modal': displayMode === 'modal', 'mode-drawer': displayMode !== 'modal' }"
      @click.self="close"
    >
      <div class="embed-panel" :class="{ expanded }" role="dialog" :aria-label="title">
        <header class="embed-header">
          <h3>{{ title }}</h3>
          <div class="embed-actions">
            <button
              type="button"
              class="icon-btn"
              :title="expanded ? '收起' : '展开'"
              @click="toggleExpand"
            >
              <MaximizeIcon v-if="!expanded" :size="16" />
              <MinimizeIcon v-else :size="16" />
            </button>
            <button
              v-if="iframeSrc || url"
              type="button"
              class="icon-btn"
              title="新窗口打开"
              @click="openInNewWindow"
            >
              <ExternalLinkIcon :size="16" />
            </button>
            <button type="button" class="icon-btn" title="关闭" @click="close">
              <XIcon :size="16" />
            </button>
          </div>
        </header>
        <div class="embed-body">
          <!-- iframe mode -->
          <template v-if="mode === 'iframe'">
            <div v-if="iframeLoading && !iframeError" class="embed-loading">
              <span class="spinner" aria-hidden="true"></span>
              加载中…
            </div>
            <div v-if="iframeError" class="embed-error">
              Dashboard 加载失败，请检查 Coroot 连接
            </div>
            <iframe
              v-show="!iframeError"
              :src="iframeSrc"
              class="coroot-iframe"
              sandbox="allow-scripts allow-same-origin allow-forms"
              referrerpolicy="no-referrer"
              @load="onIframeLoad"
              @error="onIframeError"
            />
          </template>
          <!-- fetch fallback mode -->
          <template v-else>
            <div v-if="fetchLoading" class="embed-loading">加载中…</div>
            <div v-else-if="content && content.error" class="embed-error">{{ content.error }}</div>
            <pre v-else-if="content" class="embed-content">{{ JSON.stringify(content, null, 2) }}</pre>
            <div v-else class="embed-empty">暂无数据</div>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.embed-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
}

.mode-drawer {
  justify-content: flex-end;
  background: rgba(15, 23, 42, 0.3);
}

.mode-modal {
  justify-content: center;
  align-items: center;
  background: rgba(15, 23, 42, 0.5);
}

.embed-panel {
  display: flex;
  flex-direction: column;
  background: #fff;
  box-shadow: -4px 0 24px rgba(15, 23, 42, 0.12);
  overflow: hidden;
  transition: width 0.2s ease, height 0.2s ease;
}

.mode-drawer .embed-panel {
  width: 480px;
  max-width: 90vw;
  height: 100%;
  border-radius: 16px 0 0 16px;
}

.mode-drawer .embed-panel.expanded {
  width: 80vw;
}

.mode-modal .embed-panel {
  width: 640px;
  max-width: 90vw;
  max-height: 80vh;
  border-radius: 16px;
}

.mode-modal .embed-panel.expanded {
  width: 90vw;
  max-height: 90vh;
}

.embed-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
  flex-shrink: 0;
}

.embed-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #0f172a;
}

.embed-actions {
  display: flex;
  gap: 6px;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 8px;
  background: transparent;
  color: #475569;
  cursor: pointer;
  text-decoration: none;
}
.icon-btn:hover { background: #f1f5f9; }

.embed-body {
  flex: 1;
  overflow: auto;
  padding: 0;
  position: relative;
}

.embed-loading {
  color: #64748b;
  font-size: 14px;
  text-align: center;
  padding: 40px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  position: absolute;
  inset: 0;
  z-index: 1;
  background: #fff;
}

.spinner {
  display: inline-block;
  width: 24px;
  height: 24px;
  border: 3px solid rgba(100, 116, 139, 0.2);
  border-top-color: #64748b;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.embed-error {
  color: #dc2626;
  font-size: 13px;
  padding: 12px;
  margin: 18px;
  background: #fee2e2;
  border-radius: 8px;
}

.coroot-iframe {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
}

.embed-content {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  color: #334155;
  background: #f8fafc;
  padding: 14px;
  margin: 18px;
  border-radius: 10px;
  border: 1px solid rgba(226, 232, 240, 0.9);
}

.embed-empty {
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
  padding: 40px 18px;
}
</style>
