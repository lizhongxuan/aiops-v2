<script setup>
import { useRouter } from "vue-router";
import { useAppStore } from "../store";
import { ServerIcon, CheckCircle2Icon, TerminalIcon } from "lucide-vue-next";

const emit = defineEmits(["close"]);
const store = useAppStore();
const router = useRouter();

async function selectHost(hostId) {
  const ok = await store.selectHost(hostId);
  if (!ok) return;
  emit("close");
}

async function openTerminal(hostId) {
  const ok = await store.selectHost(hostId);
  if (!ok) return;
  emit("close");
  router.push(`/terminal/${hostId}`);
}
</script>

<template>
  <n-modal
    :show="true"
    preset="card"
    title="Select Host Environment"
    :bordered="false"
    style="width: 520px; max-width: 90vw;"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <div class="host-list">
      <div
        v-for="host in store.snapshot.hosts"
        :key="host.id"
        class="host-item"
        :class="{ active: store.snapshot.selectedHostId === host.id }"
        @click="selectHost(host.id)"
      >
        <div class="host-icon">
          <ServerIcon size="20" />
        </div>
        <div class="host-info">
          <div class="host-name">{{ host.name }}</div>
          <div class="host-id">ID: {{ host.id }}</div>
          <div class="host-meta">
            <n-tag :type="host.status === 'online' ? 'success' : 'default'" size="small" round>
              {{ host.status }}
            </n-tag>
            <span class="host-kind">{{ host.kind }}</span>
            <n-tag v-if="host.kind === 'agent' && host.executable" size="small" type="info" round>远程可管</n-tag>
            <n-tag v-else-if="host.terminalCapable && !host.executable" size="small" type="info" round>远程终端</n-tag>
            <n-tag v-else-if="!host.executable" size="small" type="warning" round>只读展示</n-tag>
          </div>
        </div>
        <n-button
          v-if="(host.terminalCapable || host.executable) && host.status === 'online'"
          size="small"
          round
          @click.stop="openTerminal(host.id)"
        >
          <template #icon><TerminalIcon size="14" /></template>
          进入终端
        </n-button>
        <div class="host-check" v-if="store.snapshot.selectedHostId === host.id">
          <CheckCircle2Icon size="20" class="text-blue" />
        </div>
      </div>
      
      <n-empty v-if="!store.snapshot.hosts.length" description="No hosts available." />
    </div>
  </n-modal>
</template>

<style scoped>
.host-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.host-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  background: #ffffff;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
}

.host-item:hover {
  border-color: #cbd5e1;
  background: #f8fafc;
}

.host-item.active {
  border-color: #3b82f6;
  background: #eff6ff;
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.1);
}

.host-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: #f1f5f9;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #64748b;
}

.host-item.active .host-icon {
  background: #dbeafe;
  color: #3b82f6;
}

.host-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.host-name {
  font-weight: 600;
  font-size: 15px;
  color: #0f172a;
}

.host-id {
  font-size: 12px;
  color: #64748b;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.host-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.host-kind {
  color: #64748b;
  text-transform: uppercase;
  font-weight: 500;
}

.text-blue {
  color: #3b82f6;
}
</style>
