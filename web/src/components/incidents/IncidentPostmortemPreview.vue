<script setup>
import { computed } from "vue";

const props = defineProps({
  postmortem: {
    type: Object,
    default: () => ({}),
  },
});

const actions = computed(() => Array.isArray(props.postmortem.actions) ? props.postmortem.actions : []);
</script>

<template>
  <section class="incident-postmortem-preview" data-testid="incident-postmortem-preview">
    <p>{{ postmortem.summary || "复盘草稿会在事故关闭后生成。" }}</p>
    <ul v-if="actions.length">
      <li v-for="action in actions" :key="action.id || action.title">
        <strong>{{ action.title }}</strong>
        <span v-if="action.owner">{{ action.owner }}</span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.incident-postmortem-preview {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.incident-postmortem-preview p {
  margin: 0;
  color: #64748b;
  line-height: 1.6;
}

.incident-postmortem-preview ul {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.incident-postmortem-preview li {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #f8fafc;
}

.incident-postmortem-preview strong {
  color: #0f172a;
  font-size: 13px;
}

.incident-postmortem-preview span {
  color: #64748b;
  font-size: 12px;
}
</style>
