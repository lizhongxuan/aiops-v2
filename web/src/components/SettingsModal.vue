<script setup>
import { ref, onMounted } from "vue";
import { useAppStore } from "../store";

const emit = defineEmits(["close"]);
const store = useAppStore();

const localModel = ref("");
const localEffort = ref("medium");
const isSaving = ref(false);

onMounted(async () => {
  await store.fetchSettings();
  localModel.value = store.settings.model || "gpt-4-turbo";
  localEffort.value = store.settings.reasoningEffort || "medium";
});

const modelOptions = [
  { label: "GPT-4o", value: "gpt-4o" },
  { label: "GPT-4 Turbo", value: "gpt-4-turbo" },
  { label: "Claude 3 Opus", value: "claude-3-opus" },
];

const effortOptions = [
  { label: "Low", value: "low" },
  { label: "Medium", value: "medium" },
  { label: "High", value: "high" },
];

async function save() {
  if (isSaving.value) return;
  isSaving.value = true;
  await store.updateSettings({
    model: localModel.value,
    reasoningEffort: localEffort.value,
  });
  isSaving.value = false;
  emit("close");
}
</script>

<template>
  <n-modal
    :show="true"
    preset="card"
    title="Settings"
    :bordered="false"
    style="width: 440px; max-width: 90vw;"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <n-form label-placement="top">
      <n-form-item label="Account Quota">
        <div class="quota-display">
          <span class="quota-amount">{{ store.settings.quota || 'Unlimited' }}</span>
          <span class="quota-label">Remaining Requests</span>
        </div>
      </n-form-item>

      <n-form-item label="Provider & Model">
        <n-select
          v-model:value="localModel"
          :options="store.settings.models?.length
            ? store.settings.models.map(m => ({ label: m.name || m.id, value: m.id }))
            : modelOptions"
        />
      </n-form-item>

      <n-form-item label="Reasoning Intensity">
        <n-radio-group v-model:value="localEffort">
          <n-radio-button v-for="opt in effortOptions" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </n-radio-button>
        </n-radio-group>
        <template #feedback>
          Higher intensity provides better reasoning but may take longer.
        </template>
      </n-form-item>
    </n-form>

    <template #action>
      <n-space justify="end">
        <n-button @click="emit('close')">Cancel</n-button>
        <n-button type="primary" @click="save" :loading="isSaving">
          Save Settings
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.quota-display {
  display: inline-flex;
  align-items: baseline;
  background: #f3f4f6;
  padding: 12px 16px;
  border-radius: 8px;
  gap: 8px;
}
.quota-amount {
  font-size: 24px;
  font-weight: 700;
  color: #111827;
}
.quota-label {
  font-size: 14px;
  color: #6b7280;
}
</style>
