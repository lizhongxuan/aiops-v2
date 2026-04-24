<script setup>
import { computed } from "vue";
import { ShieldAlertIcon } from "lucide-vue-next";
import { NCard, NCode, NButton, NTag, NSpace } from "naive-ui";
import { resolveHostBadge } from "../lib/hostDisplay";
import { useAppStore } from "../store";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  isOverlay: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["approval"]);
const store = useAppStore();

const isCommand = computed(() => props.card.type === "CommandApprovalCard" || !!props.card.command);
const isFileChange = computed(() => props.card.type === "FileChangeApprovalCard" || !!props.card.changes?.length);
const decisions = computed(() => props.card.approval?.decisions || []);
const hostCaption = computed(() => {
  const host = store.snapshot.hosts.find((item) => item.id === props.card.hostId) || {};
  return resolveHostBadge({
    ...host,
    hostId: props.card.hostId,
    hostName: props.card.hostName,
  });
});
const fileChanges = computed(() => (props.card.changes || []).map((change, index) => normalizeFileChange(change, index)));
const fileChangeCount = computed(() => fileChanges.value.length);
const fileChangePrimaryPath = computed(() => fileChanges.value[0]?.path || "");
const fileChangeModeSummary = computed(() => {
  if (!fileChanges.value.length) return "";
  const kinds = new Set(fileChanges.value.map((item) => item.kindLabel));
  if (kinds.size === 1) return fileChanges.value[0].writeModeLabel;
  return "混合写入";
});
const fileChangeSummary = computed(() => {
  if (!fileChanges.value.length) return "";
  if (fileChanges.value.length === 1) {
    const item = fileChanges.value[0];
    return item.diffPreview || "没有可展示的 diff 片段。";
  }
  const first = fileChanges.value[0];
  return first.diffPreview || `共 ${fileChanges.value.length} 个文件变更。`;
});
const fileChangeReason = computed(() => (props.card.text || "要执行以下文件修改，你要允许吗？").trim());

const resolvedText = computed(() => {
  if (props.card.status === "accept" || props.card.status === "accepted" || props.card.status === "accepted_for_session") {
    return "已批准执行";
  }
  if (props.card.status === "accepted_for_session_auto") {
    return "已按本会话规则自动批准";
  }
  if (props.card.status === "decline" || props.card.status === "declined") {
    return "已拒绝";
  }
  return "已处理";
});

function submitDecision(decision) {
  if (!props.card.approval?.requestId) return;
  emit("approval", {
    approvalId: props.card.approval.requestId,
    decision,
  });
}

function normalizeFileChange(change, index) {
  const path = (change?.path || "").trim();
  const kind = (change?.kind || "update").trim() || "update";
  return {
    index,
    path,
    kind,
    kindLabel: fileChangeKindLabel(kind),
    writeModeLabel: describeWriteMode(kind),
    diff: (change?.diff || "").trim(),
    diffPreview: summarizeDiff(change?.diff || ""),
  };
}

function fileChangeKindLabel(kind) {
  switch ((kind || "").toLowerCase()) {
    case "create": return "新建";
    case "append": return "追加";
    case "delete": return "删除";
    default: return "更新";
  }
}

function describeWriteMode(kind) {
  switch ((kind || "").toLowerCase()) {
    case "create": return "新建并写入";
    case "append": return "追加写入";
    case "delete": return "删除文件";
    default: return "覆盖写入";
  }
}

function summarizeDiff(diff) {
  const text = (diff || "").trim();
  if (!text) return "";
  const lines = text.split("\n");
  const preview = lines.slice(0, 6).join("\n").trim();
  if (preview.length >= 260 || lines.length <= 6) return preview;
  return `${preview}\n...`;
}
</script>

<template>
  <n-card
    class="auth-card"
    :class="{ 'is-overlay': isOverlay }"
    :bordered="!isOverlay"
    size="small"
  >
    <template #header>
      <div class="auth-intent">
        <ShieldAlertIcon size="18" class="intent-icon" />
        <span>{{ card.text || "要执行以下操作，你要允许吗？" }}</span>
      </div>
    </template>

    <div class="auth-preview" v-if="card.command || card.changes?.length">
      <n-tag v-if="hostCaption" size="small" round>{{ hostCaption }}</n-tag>

      <template v-if="isCommand">
        <div v-if="card.cwd" class="cwd-badge">{{ card.cwd }}</div>
        <n-code v-if="card.command" :code="card.command" language="bash" word-wrap />
      </template>

      <template v-else-if="isFileChange">
        <div class="file-change-overview">
          <n-space :size="8">
            <n-tag size="small">目标: {{ fileChangePrimaryPath || "未指定" }}</n-tag>
            <n-tag size="small">{{ fileChangeCount }} 个文件</n-tag>
            <n-tag size="small">{{ fileChangeModeSummary }}</n-tag>
          </n-space>
          <div v-if="fileChangeReason" class="file-change-reason">{{ fileChangeReason }}</div>
        </div>

        <div class="changes-list">
          <article v-for="change in fileChanges" :key="`${change.path}-${change.index}`" class="change-item">
            <div class="change-row">
              <n-tag size="tiny" :type="change.kind === 'delete' ? 'error' : change.kind === 'create' ? 'success' : 'default'">{{ change.kindLabel }}</n-tag>
              <span class="change-path">{{ change.path }}</span>
            </div>
            <n-code v-if="change.diffPreview" :code="change.diffPreview" language="diff" word-wrap />
          </article>
        </div>
      </template>
    </div>

    <div v-if="card.status === 'pending' && card.approval" class="auth-actions">
      <n-space :size="8">
        <n-button type="primary" size="small" @click="submitDecision('accept')">
          同意
        </n-button>
        <n-button
          v-if="isCommand || decisions.includes('accept_session')"
          type="primary"
          size="small"
          ghost
          @click="submitDecision('accept_session')"
        >
          同意并记住
        </n-button>
        <n-button type="error" size="small" ghost @click="submitDecision('decline')">
          拒绝
        </n-button>
      </n-space>
    </div>

    <div v-if="card.status !== 'pending'" class="auth-resolved">
      {{ resolvedText }}
    </div>
  </n-card>
</template>

<style scoped>
.auth-card {
  margin-top: 6px;
  margin-left: 36px;
  max-width: 660px;
}

.auth-card.is-overlay {
  margin: 0;
  max-width: none;
}

.auth-intent {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12.5px;
  line-height: 1.45;
  color: #374151;
  font-weight: 600;
}

.intent-icon {
  color: #f59e0b;
  flex-shrink: 0;
  margin-top: 1px;
}

.auth-preview {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 4px;
}

.cwd-badge {
  font-size: 10px;
  color: #6b7280;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.file-change-overview {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.file-change-reason {
  font-size: 12px;
  line-height: 1.5;
  color: #475569;
}

.changes-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.change-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.change-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.change-path {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  color: #374151;
  word-break: break-word;
  font-size: 12px;
}

.auth-actions {
  margin-top: 12px;
}

.auth-resolved {
  margin-top: 8px;
  text-align: center;
  color: #6b7280;
  font-size: 11px;
  font-weight: 500;
  padding: 6px 0;
  background: #f9fafb;
  border-radius: 6px;
}

@media (max-width: 640px) {
  .auth-card {
    margin-left: 0;
    max-width: none;
  }
}
</style>
