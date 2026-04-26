<script setup>
const props = defineProps({
  artifacts: {
    type: Array,
    default: () => [],
  },
});
</script>

<template>
  <section v-if="artifacts.length" class="artifact-preview-drawer" data-testid="artifact-preview-drawer">
    <div class="artifact-title">Artifacts</div>
    <div class="artifact-list">
      <article v-for="artifact in artifacts" :key="artifact.id" class="artifact-item">
        <strong>{{ artifact.title || artifact.kind || artifact.id }}</strong>
        <span v-if="artifact.summary">{{ artifact.summary }}</span>
        <code v-if="artifact.uri">{{ artifact.uri }}</code>
      </article>
    </div>
  </section>
</template>

<style scoped>
.artifact-preview-drawer {
  width: min(920px, 100%);
  margin: 0 auto;
  padding: 10px 12px;
  border: 1px solid #e7e5e4;
  border-radius: 14px;
  background: #fbfaf8;
}

.artifact-title {
  margin-bottom: 8px;
  color: #8a8f98;
  font-size: 12px;
  font-weight: 500;
}

.artifact-list {
  display: grid;
  gap: 8px;
}

.artifact-item {
  display: grid;
  gap: 3px;
  color: #52525b;
  font-size: 12.5px;
}

.artifact-item strong {
  color: #262626;
  font-size: 13.5px;
}

.artifact-item code {
  width: fit-content;
  padding: 1px 5px;
  border-radius: 6px;
  background: #f1f5f9;
  color: #64748b;
  font-size: 12px;
}
</style>
