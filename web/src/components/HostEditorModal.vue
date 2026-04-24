<script setup>
import { computed, ref } from "vue";

const props = defineProps({
  host: {
    type: Object,
    default: null,
  },
});

const emit = defineEmits(["close", "save"]);

const isEditing = computed(() => !!props.host?.id);

function labelsToText(labels) {
  if (!labels) return "";
  return Object.entries(labels)
    .map(([key, value]) => (value ? `${key}=${value}` : key))
    .join("\n");
}

const form = ref({
  id: props.host?.id || "",
  name: props.host?.name || "",
  address: props.host?.address || "",
  sshUser: props.host?.sshUser || "",
  sshPort: props.host?.sshPort || 22,
  labelsText: labelsToText(props.host?.labels),
  installViaSsh: false,
});

function parseLabels(raw) {
  return String(raw || "")
    .split(/\n|,|;/)
    .map((item) => item.trim())
    .filter(Boolean)
    .reduce((acc, item) => {
      const [key, ...rest] = item.split("=");
      const normalizedKey = key?.trim();
      if (!normalizedKey) return acc;
      acc[normalizedKey] = rest.join("=").trim();
      return acc;
    }, {});
}

function submit() {
  emit("save", {
    id: form.value.id.trim(),
    name: form.value.name.trim(),
    address: form.value.address.trim(),
    sshUser: form.value.sshUser.trim(),
    sshPort: Number(form.value.sshPort) || 22,
    labels: parseLabels(form.value.labelsText),
    installViaSsh: !!form.value.installViaSsh,
  });
}
</script>

<template>
  <n-modal
    :show="true"
    preset="card"
    :title="isEditing ? '编辑主机' : '新增主机'"
    :bordered="false"
    style="width: 480px; max-width: 90vw;"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <n-form label-placement="top" @submit.prevent="submit">
      <n-form-item label="Host ID">
        <n-input v-model:value="form.id" :disabled="isEditing" placeholder="web-01" />
      </n-form-item>

      <n-form-item label="显示名称">
        <n-input v-model:value="form.name" placeholder="web-01 / 支付-应用节点" />
      </n-form-item>

      <n-form-item label="目标机器">
        <n-input v-model:value="form.address" placeholder="10.0.0.21 或 web-01.internal" />
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

      <n-form-item label="标签">
        <n-input
          v-model:value="form.labelsText"
          type="textarea"
          :rows="4"
          placeholder="env=prod&#10;role=web&#10;app=nginx"
        />
      </n-form-item>

      <n-form-item v-if="!isEditing">
        <n-checkbox v-model:checked="form.installViaSsh">
          保存后通过 SSH 安装 host-agent
        </n-checkbox>
      </n-form-item>

      <n-alert type="info" :bordered="false" style="margin-bottom: 12px;">
        SSH 安装依赖当前机器已经可以访问目标主机，例如本机已有 SSH key、跳板机或 agent 转发。
      </n-alert>
    </n-form>

    <template #action>
      <n-space justify="end">
        <n-button @click="emit('close')">取消</n-button>
        <n-button type="primary" @click="submit">
          {{ isEditing ? "保存主机" : "创建主机" }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>
