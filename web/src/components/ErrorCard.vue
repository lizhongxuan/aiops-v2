<script setup>
import { computed } from "vue";
import { RotateCcwIcon, RefreshCwIcon } from "lucide-vue-next";
import { NAlert, NButton, NSpace } from "naive-ui";
import { resolveHostDisplay } from "../lib/hostDisplay";
import { useAppStore } from "../store";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["retry", "refresh"]);
const store = useAppStore();

const errorText = computed(() => {
  return [props.card.title, props.card.message, props.card.text, props.card.error]
    .filter(Boolean)
    .join("\n")
    .toLowerCase();
});

const errorTypeLabel = computed(() => {
  const text = errorText.value;
  if (text.includes("declined") || text.includes("拒绝") || text.includes("decline")) return "审批已拒绝";
  if (text.includes("回退到本地执行") || (text.includes("server-local") && text.includes("请改用"))) return "执行路径不允许";
  if (text.includes("permission denied") || text.includes("operation not permitted") || text.includes("权限")) return "OS 权限不足";
  if (text.includes("heartbeat timed out") || text.includes("心跳超时") || text.includes("network timeout")) return "网络 / 心跳超时";
  if (text.includes("remote host disconnected") || text.includes("host-agent") || text.includes("主机断连") || text.includes("离线")) return "Agent 断连";
  if (text.includes("terminal") && (text.includes("disconnected") || text.includes("closed") || text.includes("断开"))) return "终端断开";
  if (text.includes("exit code") || text.includes("执行失败") || text.includes("command failed")) return "命令退出失败";
  return "执行异常";
});

const needsAlternative = computed(() => {
  const text = errorText.value;
  return (
    text.includes("declined") || text.includes("拒绝") || text.includes("decline") ||
    text.includes("回退到本地执行") ||
    text.includes("permission denied") || text.includes("operation not permitted") ||
    text.includes("执行失败") || text.includes("command failed")
  );
});

const isRetryable = computed(() => {
  if (props.card.retryable === false) return false;
  return !needsAlternative.value;
});

const hostLabel = computed(() => {
  const host = store.snapshot.hosts.find((item) => item.id === props.card.hostId) || {};
  return resolveHostDisplay({
    ...host,
    hostId: props.card.hostId,
    hostName: props.card.hostName,
  });
});

const errorHint = computed(() => {
  const text = errorText.value;
  if (text.includes("declined") || text.includes("拒绝") || text.includes("decline")) return "这次请求已经停止。可以调整方案后重新发起，或改成更小范围的只读检查。";
  if (text.includes("回退到本地执行")) return "当前会话已绑定远程主机。继续处理前，需要改用对应的远程 execute_* 工具，或先切换回正确主机。";
  if (text.includes("permission denied") || text.includes("operation not permitted") || text.includes("权限")) return "建议切换更高权限用户，或先确认目标路径 / 服务是否允许当前账号访问。";
  if (text.includes("heartbeat timed out") || text.includes("心跳超时") || text.includes("network timeout")) return "建议先确认 ai-server 与目标主机网络恢复，再重试当前操作。";
  if (text.includes("remote host disconnected") || text.includes("host-agent") || text.includes("主机断连") || text.includes("离线")) return "建议刷新主机状态，确认 host-agent 恢复在线后再继续。";
  if (text.includes("terminal") && (text.includes("disconnected") || text.includes("closed") || text.includes("断开"))) return "可以重新进入终端；如果频繁断开，优先检查目标主机连通性和 agent 状态。";
  if (text.includes("exit code") || text.includes("执行失败") || text.includes("command failed")) return "建议查看命令输出和退出码，修正命令参数后重试。";
  return "建议刷新状态后重试；如果持续失败，需要换一种执行方案。";
});

function onRetry() {
  emit("retry", { cardId: props.card.id });
}

function onRefresh() {
  emit("refresh");
}
</script>

<template>
  <div class="error-card-wrapper">
    <n-alert type="error" :title="`${errorTypeLabel}${hostLabel ? ' · ' + hostLabel : ''}`">
      <p class="error-message">{{ card.message || card.text || card.title || '发生了未知错误，请重试。' }}</p>
      <p class="error-hint">{{ errorHint }}</p>
      <template #action>
        <n-space :size="8">
          <n-button v-if="isRetryable" size="small" type="error" @click="onRetry">
            <template #icon><RotateCcwIcon size="14" /></template>
            重试
          </n-button>
          <n-button size="small" @click="onRefresh">
            <template #icon><RefreshCwIcon size="14" /></template>
            刷新
          </n-button>
        </n-space>
      </template>
    </n-alert>
  </div>
</template>

<style scoped>
.error-card-wrapper {
  margin-top: 2px;
  margin-left: 36px;
  max-width: 640px;
}

.error-message {
  margin: 0 0 6px;
  font-size: 13px;
  line-height: 1.5;
}

.error-hint {
  margin: 0;
  font-size: 11px;
  line-height: 1.5;
  opacity: 0.8;
}

@media (max-width: 640px) {
  .error-card-wrapper {
    margin-left: 0;
    max-width: none;
  }
}
</style>
