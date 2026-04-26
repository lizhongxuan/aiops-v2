<script setup>
const props = defineProps({
  approvals: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["accept", "decline", "detail"]);

function approvalTitle(approval = {}) {
  return String(approval.title || approval.reason || approval.id || "待确认操作").trim();
}
</script>

<template>
  <div v-if="approvals.length" class="approval-dock" data-testid="approval-dock">
    <div v-for="approval in approvals" :key="approval.id" class="approval-dock-card">
      <div class="approval-copy">
        <span class="approval-kicker">等待确认</span>
        <strong>{{ approvalTitle(approval) }}</strong>
        <p v-if="approval.reason">{{ approval.reason }}</p>
      </div>
      <div class="approval-actions">
        <button type="button" class="ghost" @click="emit('detail', approval)">详情</button>
        <button type="button" class="ghost" @click="emit('decline', approval)">拒绝</button>
        <button type="button" class="primary" @click="emit('accept', approval)">同意</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.approval-dock {
  position: sticky;
  bottom: 12px;
  z-index: 5;
  width: min(920px, calc(100% - 28px));
  margin: 8px auto 0;
  display: grid;
  gap: 8px;
}

.approval-dock-card {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid #e7e5e4;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.06);
  backdrop-filter: blur(12px);
}

.approval-copy {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.approval-kicker {
  color: #a16207;
  font-size: 12px;
  font-weight: 600;
}

.approval-copy strong {
  color: #262626;
  font-size: 14px;
  font-weight: 600;
}

.approval-copy p {
  margin: 0;
  color: #6b7280;
  font-size: 12.5px;
  line-height: 1.5;
}

.approval-actions {
  display: flex;
  align-items: center;
  gap: 7px;
  flex-shrink: 0;
}

.approval-actions button {
  border: 1px solid #e5e7eb;
  border-radius: 999px;
  padding: 7px 11px;
  background: #fff;
  color: #52525b;
  font-size: 12.5px;
  cursor: pointer;
}

.approval-actions .primary {
  border-color: #222;
  background: #171717;
  color: #fff;
}
</style>
