<script setup>
import { ref } from "vue";

const props = defineProps({
  count: {
    type: Number,
    default: 0,
  },
});

const emit = defineEmits(["close", "save"]);

const addText = ref("");
const removeText = ref("");

function parseAdd(raw) {
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

function parseRemove(raw) {
  return String(raw || "")
    .split(/\n|,|;/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function submit() {
  emit("save", {
    add: parseAdd(addText.value),
    remove: parseRemove(removeText.value),
  });
}
</script>

<template>
  <n-modal
    :show="true"
    preset="card"
    title="批量标签"
    :bordered="false"
    style="width: 480px; max-width: 90vw;"
    :mask-closable="true"
    @update:show="(val) => { if (!val) emit('close'); }"
  >
    <p class="tag-note">已选 {{ count }} 台主机。支持批量添加标签，也可以按 key 批量删除。</p>

    <n-form label-placement="top" @submit.prevent="submit">
      <n-form-item label="新增标签">
        <n-input
          v-model:value="addText"
          type="textarea"
          :rows="4"
          placeholder="env=prod&#10;role=web&#10;batch=blue"
        />
      </n-form-item>

      <n-form-item label="删除标签">
        <n-input
          v-model:value="removeText"
          type="textarea"
          :rows="3"
          placeholder="batch&#10;owner"
        />
      </n-form-item>
    </n-form>

    <template #action>
      <n-space justify="end">
        <n-button @click="emit('close')">取消</n-button>
        <n-button type="primary" @click="submit">应用到已选主机</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.tag-note {
  margin: 0 0 16px;
  font-size: 13px;
  color: #475569;
  line-height: 1.6;
}
</style>
