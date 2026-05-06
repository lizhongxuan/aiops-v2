<script setup>
import { computed, ref, watch } from "vue";
import { DownloadIcon, HistoryIcon, RotateCcwIcon, UploadIcon, XIcon } from "lucide-vue-next";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  workflowName: {
    type: String,
    default: "",
  },
  currentYaml: {
    type: String,
    default: "",
  },
  versions: {
    type: Array,
    default: () => [],
  },
  loading: {
    type: Boolean,
    default: false,
  },
  error: {
    type: String,
    default: "",
  },
  exportText: {
    type: String,
    default: "",
  },
  importOnly: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["close", "refresh", "rollback", "export-bundle", "import-workflow"]);

const selectedVersionId = ref("");
const importMode = ref("bundle");
const importText = ref("");
const overwrite = ref(false);

const selectedVersion = computed(() => {
  const fallback = props.versions?.[0] || null;
  const selected = props.versions.find((version) => version.id === selectedVersionId.value);
  return selected || fallback;
});

watch(
  () => props.versions,
  (versions) => {
    if (!selectedVersionId.value && versions?.length) {
      selectedVersionId.value = versions[0].id || "";
    }
  },
  { immediate: true },
);

function previewVersion(versionId) {
  selectedVersionId.value = versionId;
}

function submitImport() {
  emit("import-workflow", {
    mode: importMode.value,
    text: importText.value,
    overwrite: overwrite.value,
  });
}

function versionTitle(version) {
  return version?.id || "unknown";
}

function formatDate(value) {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return String(value);
  return parsed.toLocaleString("zh-CN", { hour12: false });
}
</script>

<template>
  <section
    v-if="show"
    class="runner-version-backdrop"
    role="dialog"
    aria-modal="true"
    aria-label="工作流版本历史"
    data-testid="runner-version-history-panel"
    @click.self="emit('close')"
  >
    <aside class="runner-version-panel">
      <header class="runner-version-head">
        <div>
          <p>{{ importOnly ? "IMPORT WORKFLOW" : "VERSION HISTORY" }}</p>
          <h2>{{ importOnly ? "导入工作流" : workflowName }}</h2>
          <span>{{ importOnly ? "导入 YAML、graph JSON 或 bundle，导入后进入 draft。" : "查看版本、导出 bundle、恢复到历史版本。" }}</span>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="17" />
        </button>
      </header>

      <p v-if="error" class="runner-version-error">{{ error }}</p>

      <section v-if="!importOnly" class="runner-version-toolbar">
        <button type="button" data-testid="runner-version-refresh" :disabled="loading" @click="emit('refresh', workflowName)">
          <HistoryIcon :size="15" />
          <span>刷新版本</span>
        </button>
        <button type="button" data-testid="runner-version-export" :disabled="loading || !workflowName" @click="emit('export-bundle', workflowName)">
          <DownloadIcon :size="15" />
          <span>导出 Bundle</span>
        </button>
      </section>

      <section v-if="!importOnly" class="runner-version-grid">
        <div class="runner-version-list">
          <article
            v-for="version in versions"
            :key="version.id"
            class="runner-version-row"
            :class="{ active: version.id === selectedVersion?.id }"
          >
            <button
              type="button"
              :data-testid="`runner-version-preview-${version.id}`"
              @click="previewVersion(version.id)"
            >
              <strong>{{ versionTitle(version) }}</strong>
              <span>{{ version.reason || version.status || "snapshot" }}</span>
              <small>{{ formatDate(version.created_at || version.createdAt) }}</small>
              <em v-if="version.save_note">{{ version.save_note }}</em>
            </button>
            <button
              type="button"
              class="runner-version-restore"
              :data-testid="`runner-version-rollback-${version.id}`"
              :disabled="loading"
              @click="emit('rollback', version.id)"
            >
              <RotateCcwIcon :size="14" />
              <span>恢复</span>
            </button>
          </article>
          <p v-if="versions.length === 0" class="runner-studio-empty">暂无历史版本。</p>
        </div>

        <div class="runner-version-preview-grid">
          <section>
            <h3>当前版本</h3>
            <pre data-testid="runner-version-current-preview">{{ currentYaml || "暂无当前 YAML。" }}</pre>
          </section>
          <section>
            <h3>目标版本</h3>
            <pre data-testid="runner-version-preview">{{ selectedVersion?.yaml || "请选择要预览的版本。" }}</pre>
          </section>
        </div>
      </section>

      <section v-if="exportText" class="runner-version-export-block">
        <h3>导出内容</h3>
        <pre data-testid="runner-version-export-text">{{ exportText }}</pre>
      </section>

      <section class="runner-version-import-block">
        <header>
          <div>
            <h3>导入</h3>
            <span>支持 workflow bundle JSON、YAML 和 visual graph JSON。导入后只生成 draft。</span>
          </div>
          <UploadIcon :size="17" />
        </header>
        <div class="runner-version-import-controls">
          <label>
            <span>格式</span>
            <select v-model="importMode" data-testid="runner-version-import-mode">
              <option value="bundle">Bundle JSON</option>
              <option value="yaml">YAML</option>
              <option value="graph">Graph JSON</option>
            </select>
          </label>
          <label class="runner-version-overwrite">
            <input v-model="overwrite" type="checkbox" />
            <span>允许覆盖同名 draft</span>
          </label>
        </div>
        <textarea
          v-model="importText"
          spellcheck="false"
          data-testid="runner-version-import-text"
          placeholder="粘贴 YAML、graph JSON 或 bundle JSON"
        />
        <footer>
          <button
            type="button"
            class="primary"
            data-testid="runner-version-import-submit"
            :disabled="loading || !importText.trim()"
            @click="submitImport"
          >
            导入为 draft
          </button>
        </footer>
      </section>
    </aside>
  </section>
</template>
