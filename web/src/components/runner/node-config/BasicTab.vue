<script setup>
import { computed, ref, watch } from "vue";

const props = defineProps({
  node: {
    type: Object,
    required: true,
  },
  actions: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:node"]);

const draftStep = ref({});
const targetRows = computed(() => (Array.isArray(draftStep.value.targets) ? draftStep.value.targets : []));

watch(
  () => props.node,
  (node) => {
    draftStep.value = { ...(node.step || {}) };
  },
  { immediate: true },
);

function actionValue(action) {
  return action.action || action.name || "";
}

function updateStep(patch) {
  draftStep.value = {
    ...draftStep.value,
    ...patch,
  };
  emit("update:node", {
    ...props.node,
    step: { ...draftStep.value },
  });
}

function updateTargets(targets) {
  const cleaned = targets.map((target) => String(target || "").trim()).filter(Boolean);
  if (cleaned.length) updateStep({ targets: cleaned });
  else {
    const next = { ...draftStep.value };
    delete next.targets;
    draftStep.value = next;
    emit("update:node", {
      ...props.node,
      step: { ...next },
    });
  }
}

function addTarget() {
  updateTargets([...targetRows.value, "local"]);
}

function updateTarget(index, value) {
  updateTargets(targetRows.value.map((target, targetIndex) => (targetIndex === index ? value : target)));
}

function removeTarget(index) {
  updateTargets(targetRows.value.filter((_, targetIndex) => targetIndex !== index));
}
</script>

<template>
  <section class="node-config-form" data-testid="basic-tab">
    <label>
      <span>name</span>
      <input
        :value="draftStep.name || ''"
        data-testid="basic-name"
        @input="updateStep({ name: $event.target.value })"
      />
    </label>

    <label>
      <span>action</span>
      <select
        :value="draftStep.action || ''"
        data-testid="basic-action"
        @change="updateStep({ action: $event.target.value })"
      >
        <option v-if="!draftStep.action" value="">选择 action</option>
        <option v-for="action in actions" :key="actionValue(action)" :value="actionValue(action)">
          {{ actionValue(action) }}
        </option>
      </select>
    </label>

    <section class="basic-target-section">
      <div class="runner-structured-section-head compact">
        <div>
          <strong>targets</strong>
          <p>默认 local；可添加多个目标，也可以清空后交给全局 inventory。</p>
        </div>
        <button type="button" class="node-panel-secondary" data-testid="action-target-add" @click="addTarget">
          + 添加目标
        </button>
      </div>
      <div v-if="targetRows.length" class="runner-kv-list compact">
        <div v-for="(target, index) in targetRows" :key="`${target}-${index}`" class="runner-kv-row">
          <input
            :value="target"
            :data-testid="`action-target-value-${index}`"
            placeholder="local / pg-01 / group:db"
            @input="updateTarget(index, $event.target.value)"
          />
          <button
            type="button"
            class="node-panel-danger"
            :data-testid="`action-target-delete-${index}`"
            @click="removeTarget(index)"
          >
            删除
          </button>
        </div>
      </div>
      <p v-else class="node-panel-empty">未设置目标。</p>
    </section>

    <label>
      <span>retries</span>
      <input
        type="number"
        min="0"
        :value="draftStep.retries ?? 0"
        data-testid="basic-retries"
        @input="updateStep({ retries: Number($event.target.value || 0) })"
      />
    </label>

    <label>
      <span>timeout</span>
      <input
        :value="draftStep.timeout || ''"
        data-testid="basic-timeout"
        placeholder="30s / 15m"
        @input="updateStep({ timeout: $event.target.value })"
      />
    </label>
  </section>
</template>
