<script setup>
import { computed } from "vue";
import { NAlert } from "naive-ui";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  sessionKind: {
    type: String,
    default: "",
  },
});

const contextLabel = computed(() => (props.sessionKind === "workspace" ? "工作台通知" : "系统通知"));
</script>

<template>
  <div class="notice-wrapper">
    <n-alert type="info" :title="contextLabel" :bordered="false">
      {{ card.text || card.title || '系统通知' }}
    </n-alert>
  </div>
</template>

<style scoped>
.notice-wrapper {
  display: flex;
  justify-content: center;
  padding: 6px 0;
  max-width: 600px;
  margin: 0 auto;
}
</style>
