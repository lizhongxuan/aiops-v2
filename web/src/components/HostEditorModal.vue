<script setup>
import { computed, ref } from "vue";

const props = defineProps({
  host: {
    type: Object,
    default: null,
  },
  intent: {
    type: String,
    default: "create",
    validator: (value) => ["create", "install", "reinstall", "edit"].includes(value),
  },
});

const emit = defineEmits(["close", "save"]);

const isEditing = computed(() => props.intent === "edit");
const shouldDefaultInstall = computed(() => ["install", "reinstall"].includes(props.intent));
const modalTitle = computed(() => ({
  create: "接入主机",
  install: "安装 client",
  reinstall: "重装 client",
  edit: "主机接入配置",
}[props.intent] || "接入主机"));
const submitText = computed(() => (props.intent === "edit" ? "保存" : "确认"));

const form = ref({
  id: props.host?.id || props.host?.address || "",
  name: props.host?.name || "",
  sshUser: props.host?.sshUser || "",
  sshPort: props.host?.sshPort || 22,
  installViaSsh: shouldDefaultInstall.value,
});

function submit() {
  const id = form.value.id.trim();
  const address = props.host?.address || id;

  emit("save", {
    id,
    name: form.value.name.trim(),
    address,
    sshUser: form.value.sshUser.trim(),
    sshPort: Number(form.value.sshPort) || 22,
    labels: props.host?.labels || {},
    installViaSsh: !!form.value.installViaSsh,
  });
}
</script>

<template>
  <n-modal
    :show="true"
    preset="card"
    :title="modalTitle"
    :bordered="false"
    style="width: 480px; max-width: 90vw;"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <n-form label-placement="top" @submit.prevent="submit">
      <n-form-item label="Host ID/IP">
        <n-input v-model:value="form.id" :disabled="isEditing" placeholder="web-01" />
      </n-form-item>

      <n-form-item label="显示名称">
        <n-input v-model:value="form.name" placeholder="web-01 / 支付-应用节点" />
      </n-form-item>

      <n-grid :cols="2" :x-gap="12">
        <n-gi>
          <n-form-item label="SSH 用户">
            <n-input v-model:value="form.sshUser" placeholder="ubuntu / root" />
          </n-form-item>
        </n-gi>
        <n-gi>
          <n-form-item label="SSH 端口">
            <n-input-number v-model:value="form.sshPort" :min="1" :max="65535" style="width: 100%" />
          </n-form-item>
        </n-gi>
      </n-grid>

      <n-form-item>
        <n-checkbox v-model:checked="form.installViaSsh">
          安装 client
        </n-checkbox>
      </n-form-item>
    </n-form>

    <template #action>
      <n-space justify="end">
        <n-button @click="emit('close')">取消</n-button>
        <n-button type="primary" @click="submit">
          {{ submitText }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>
