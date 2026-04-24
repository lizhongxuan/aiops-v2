<script setup>
import { computed } from "vue";
import { normalizeToolDisplayPayload, toolDisplayKindLabel } from "../../../lib/toolDisplayModel";

const props = defineProps({
  display: {
    type: Object,
    default: null,
  },
});

const displayModel = computed(() => normalizeToolDisplayPayload(props.display));

const blocks = computed(() => displayModel.value?.blocks || []);

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value ?? "").trim();
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function blockLabel(block) {
  return compactText(block?.title) || toolDisplayKindLabel(block?.kind);
}

function blockText(block) {
  return compactText(block?.text || block?.title);
}

function itemLabel(item) {
  return compactText(item?.label || item?.key || item?.name || item?.title || item?.path || item?.query);
}

function itemValue(item) {
  return compactText(item?.value || item?.text || item?.summary || item?.content || item?.body || item?.result || item?.description);
}

function itemHref(item) {
  return compactText(item?.url || item?.href || item?.link);
}

function itemPath(item) {
  return compactText(item?.path || item?.filePath || item?.file || item?.filename);
}

function diffSummary(item) {
  const summary = compactText(item?.summary || itemValue(item));
  if (summary) return summary;
  const parts = [];
  if (item?.added !== undefined && item?.added !== null && item?.added !== "") parts.push(`+${item.added}`);
  if (item?.removed !== undefined && item?.removed !== null && item?.removed !== "") parts.push(`-${item.removed}`);
  return parts.join(" ");
}

function isBlockEmpty(block) {
  return !blockText(block) && asArray(block?.items).length === 0;
}
</script>

<template>
  <section v-if="displayModel" class="tool-display-renderer" data-testid="tool-display-renderer">
    <div v-if="displayModel.summary || displayModel.activity" class="tool-display-meta">
      <div v-if="displayModel.summary" class="tool-display-summary">{{ displayModel.summary }}</div>
      <div v-if="displayModel.activity && displayModel.activity !== displayModel.summary" class="tool-display-activity">{{ displayModel.activity }}</div>
    </div>

    <div class="tool-display-blocks">
      <section
        v-for="(block, index) in blocks"
        :key="block.id || `${block.kind}-${index}`"
        class="tool-display-block"
        :class="[`tool-display-block--${block.kind || 'text'}`, { 'tool-display-block--empty': isBlockEmpty(block) }]"
        :data-testid="`tool-display-block-${block.kind || 'unknown'}`"
      >
        <header v-if="block.kind !== 'text'" class="tool-display-block-header">
          <span class="tool-display-block-kind">{{ toolDisplayKindLabel(block.kind) }}</span>
          <span v-if="block.title" class="tool-display-block-title">{{ block.title }}</span>
        </header>

        <div v-if="block.kind === 'text'" class="tool-display-text">
          {{ blockText(block) || blockLabel(block) }}
        </div>

        <div v-else-if="block.kind === 'kv_list'" class="tool-display-kv-list">
          <div v-for="(item, itemIndex) in asArray(block.items)" :key="item.id || `${block.kind}-${itemIndex}`" class="tool-display-kv-row">
            <div class="tool-display-kv-label">{{ itemLabel(item) || `项 ${itemIndex + 1}` }}</div>
            <div class="tool-display-kv-value">{{ itemValue(item) || itemPath(item) || itemHref(item) || "—" }}</div>
          </div>
          <div v-if="!asArray(block.items).length && blockText(block)" class="tool-display-text">{{ blockText(block) }}</div>
        </div>

        <div v-else-if="block.kind === 'search_queries'" class="tool-display-query-list">
          <div v-for="(item, itemIndex) in asArray(block.items)" :key="item.id || `${block.kind}-${itemIndex}`" class="tool-display-query-row">
            <code class="tool-display-chip">{{ itemLabel(item) || itemValue(item) || `查询 ${itemIndex + 1}` }}</code>
          </div>
          <div v-if="!asArray(block.items).length && blockText(block)" class="tool-display-text">{{ blockText(block) }}</div>
        </div>

        <div v-else-if="block.kind === 'link_list'" class="tool-display-link-list">
          <a
            v-for="(item, itemIndex) in asArray(block.items)"
            :key="item.id || `${block.kind}-${itemIndex}`"
            class="tool-display-link"
            :href="itemHref(item) || undefined"
            target="_blank"
            rel="noreferrer noopener"
          >
            {{ itemLabel(item) || itemHref(item) || `链接 ${itemIndex + 1}` }}
          </a>
          <div v-if="!asArray(block.items).length && blockText(block)" class="tool-display-text">{{ blockText(block) }}</div>
        </div>

        <div v-else-if="block.kind === 'warning'" class="tool-display-warning">
          <div class="tool-display-warning-copy">{{ blockText(block) || block.title || blockLabel(block) }}</div>
          <div v-if="asArray(block.items).length" class="tool-display-warning-items">
            <div v-for="(item, itemIndex) in asArray(block.items)" :key="item.id || `${block.kind}-${itemIndex}`" class="tool-display-warning-item">
              {{ itemLabel(item) || itemValue(item) || itemHref(item) || itemPath(item) }}
            </div>
          </div>
        </div>

        <div v-else-if="block.kind === 'file_preview'" class="tool-display-file-preview">
          <div v-for="(item, itemIndex) in asArray(block.items)" :key="item.id || `${block.kind}-${itemIndex}`" class="tool-display-file-preview-item">
            <div class="tool-display-file-path">{{ itemPath(item) || itemLabel(item) || `文件 ${itemIndex + 1}` }}</div>
            <pre class="tool-display-file-content">{{ itemValue(item) || blockText(block) || "—" }}</pre>
          </div>
          <pre v-if="!asArray(block.items).length && blockText(block)" class="tool-display-file-content">{{ blockText(block) }}</pre>
        </div>

        <div v-else-if="block.kind === 'file_diff_summary'" class="tool-display-diff-summary">
          <div v-for="(item, itemIndex) in asArray(block.items)" :key="item.id || `${block.kind}-${itemIndex}`" class="tool-display-diff-row">
            <div class="tool-display-diff-path">{{ itemPath(item) || itemLabel(item) || `文件 ${itemIndex + 1}` }}</div>
            <div class="tool-display-diff-copy">{{ diffSummary(item) || blockText(block) || "—" }}</div>
          </div>
          <div v-if="!asArray(block.items).length && blockText(block)" class="tool-display-diff-copy">{{ blockText(block) }}</div>
        </div>

        <div v-else-if="block.kind === 'result_stats'" class="tool-display-stats">
          <div v-for="(item, itemIndex) in asArray(block.items)" :key="item.id || `${block.kind}-${itemIndex}`" class="tool-display-stat">
            <span class="tool-display-stat-label">{{ itemLabel(item) || `指标 ${itemIndex + 1}` }}</span>
            <strong class="tool-display-stat-value">{{ itemValue(item) || itemPath(item) || "—" }}</strong>
          </div>
          <div v-if="!asArray(block.items).length && blockText(block)" class="tool-display-text">{{ blockText(block) }}</div>
        </div>

        <div v-else-if="block.kind === 'command'" class="tool-display-command">
          <pre class="tool-display-command-text">{{ blockText(block) || block.title || blockLabel(block) }}</pre>
        </div>

        <div v-else class="tool-display-fallback">
          <div class="tool-display-text">{{ blockText(block) || block.title || blockLabel(block) }}</div>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.tool-display-renderer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 6px;
}

.tool-display-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  color: #475569;
  font-size: 12.5px;
  line-height: 1.45;
}

.tool-display-summary {
  font-weight: 600;
  color: #334155;
}

.tool-display-activity {
  color: #64748b;
}

.tool-display-blocks {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tool-display-block {
  border: 1px solid rgba(226, 232, 240, 0.98);
  background: rgba(255, 255, 255, 0.78);
  border-radius: 12px;
  padding: 10px 12px;
}

.tool-display-block-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  color: #475569;
  font-size: 11.5px;
  font-weight: 600;
  line-height: 1.35;
}

.tool-display-block-kind {
  padding: 1px 6px;
  border-radius: 999px;
  background: #e2e8f0;
  color: #475569;
}

.tool-display-block-title {
  color: #64748b;
}

.tool-display-text,
.tool-display-command-text,
.tool-display-file-content,
.tool-display-diff-copy {
  margin: 0;
  color: #334155;
  font-size: 12.5px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

.tool-display-kv-list,
.tool-display-query-list,
.tool-display-link-list,
.tool-display-warning,
.tool-display-file-preview,
.tool-display-diff-summary,
.tool-display-stats {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-display-kv-row,
.tool-display-stat,
.tool-display-diff-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.tool-display-kv-label,
.tool-display-stat-label,
.tool-display-file-path,
.tool-display-diff-path {
  flex: 0 0 auto;
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.45;
}

.tool-display-kv-value,
.tool-display-stat-value {
  color: #334155;
  font-size: 12.5px;
  line-height: 1.45;
}

.tool-display-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 12px;
  border: 1px solid rgba(191, 219, 254, 0.8);
}

.tool-display-link {
  color: #1d4ed8;
  font-size: 12.5px;
  line-height: 1.45;
  text-decoration: none;
  word-break: break-word;
}

.tool-display-link:hover {
  text-decoration: underline;
}

.tool-display-warning {
  padding: 10px 12px;
  border-radius: 10px;
  background: #fffbeb;
  border: 1px solid rgba(245, 158, 11, 0.22);
}

.tool-display-warning-copy {
  color: #92400e;
  font-size: 12.5px;
  line-height: 1.5;
}

.tool-display-warning-items {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 6px;
}

.tool-display-warning-item {
  color: #b45309;
  font-size: 12px;
  line-height: 1.45;
}

.tool-display-file-content,
.tool-display-command-text {
  padding: 8px 10px;
  border-radius: 10px;
  background: #0f172a;
  color: #e2e8f0;
  overflow: auto;
}

.tool-display-file-preview-item + .tool-display-file-preview-item,
.tool-display-diff-row + .tool-display-diff-row,
.tool-display-stat + .tool-display-stat,
.tool-display-kv-row + .tool-display-kv-row {
  padding-top: 4px;
}

.tool-display-block--empty {
  opacity: 0.95;
}
</style>
