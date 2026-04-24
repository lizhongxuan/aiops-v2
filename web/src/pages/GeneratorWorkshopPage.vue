<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { ArrowLeftIcon } from "lucide-vue-next";
import { generateDraft, lintDraft, previewDraft, publishDraft } from "../api/generator";

const router = useRouter();

const activeTab = ref("source");
const loading = ref(false);

// Source selection
const source = ref("mcp_tool"); // mcp_tool | script_config | coroot
const toolName = ref("");
const toolDesc = ref("");
const inputSchemaText = ref("{}");
const scriptConfigText = ref("{}");
const serviceType = ref("");
const querySchemaText = ref("{}");

// Generated draft
const draftType = ref("");
const draftSkill = ref(null);
const draftCard = ref(null);

// Lint
const lintResult = ref(null);
const lintLoading = ref(false);

// Preview
const previewResult = ref(null);
const previewLoading = ref(false);

// Publish
const publishResult = ref(null);
const publishLoading = ref(false);

const currentDraft = computed(() => {
  if (draftType.value === "skill") return draftSkill.value;
  if (draftType.value === "card") return draftCard.value;
  return null;
});

const hasDraft = computed(() => currentDraft.value !== null);

async function generate() {
  loading.value = true;
  draftSkill.value = null;
  draftCard.value = null;
  draftType.value = "";
  lintResult.value = null;
  previewResult.value = null;
  publishResult.value = null;

  try {
    const body = { source: source.value };
    if (source.value === "mcp_tool") {
      body.toolName = toolName.value;
      body.toolDesc = toolDesc.value;
      try { body.inputSchema = JSON.parse(inputSchemaText.value); } catch { body.inputSchema = {}; }
    } else if (source.value === "script_config") {
      try { body.scriptConfig = JSON.parse(scriptConfigText.value); } catch { body.scriptConfig = {}; }
    } else if (source.value === "coroot") {
      body.serviceType = serviceType.value;
      try { body.querySchema = JSON.parse(querySchemaText.value); } catch { body.querySchema = {}; }
    }

    const data = await generateDraft(body);
    draftType.value = data.draftType || "";
    if (data.skill) draftSkill.value = data.skill;
    if (data.card) draftCard.value = data.card;
    activeTab.value = "preview";
  } catch { /* ignore */ }
  finally { loading.value = false; }
}

async function lint() {
  if (!hasDraft.value) return;
  lintLoading.value = true;
  lintResult.value = null;
  try {
    const body = { draftType: draftType.value };
    if (draftType.value === "skill") body.skill = draftSkill.value;
    else body.card = draftCard.value;

    lintResult.value = await lintDraft(body);
  } catch { /* ignore */ }
  finally { lintLoading.value = false; }
}

async function preview() {
  if (!hasDraft.value) return;
  previewLoading.value = true;
  previewResult.value = null;
  try {
    const body = { draftType: draftType.value };
    if (draftType.value === "skill") body.skill = draftSkill.value;
    else body.card = draftCard.value;

    previewResult.value = await previewDraft(body);
  } catch { /* ignore */ }
  finally { previewLoading.value = false; }
}

async function publish() {
  if (!hasDraft.value) return;
  publishLoading.value = true;
  publishResult.value = null;
  try {
    const body = { draftType: draftType.value };
    if (draftType.value === "skill") body.skill = draftSkill.value;
    else body.card = draftCard.value;

    publishResult.value = await publishDraft(body);
    activeTab.value = "publish";
  } catch { /* ignore */ }
  finally { publishLoading.value = false; }
}

function goBack() {
  router.push("/settings");
}

let previousTitle = "";
onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "Generator Workshop · AIOps";
});
onBeforeUnmount(() => {
  if (previousTitle) document.title = previousTitle;
});
</script>

<template>
  <section class="gen-page">
    <header class="gen-hero">
      <div class="gen-hero-copy">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon size="16" />
          <span>返回设置</span>
        </button>
        <div class="gen-kicker">Tools / Generator Workshop</div>
        <h1>Generator Workshop</h1>
        <p>从 MCP 工具、脚本配置或 Coroot 数据自动生成 Skill / UI Card / Bundle 草稿。</p>
      </div>
    </header>

    <n-steps :current="activeTab === 'source' ? 1 : activeTab === 'preview' ? 2 : activeTab === 'validate' ? 3 : 4" style="margin-bottom:18px;">
      <n-step title="来源选择" @click="activeTab = 'source'" />
      <n-step title="预览" :disabled="!hasDraft" @click="hasDraft && (activeTab = 'preview')" />
      <n-step title="校验" :disabled="!hasDraft" @click="hasDraft && (activeTab = 'validate')" />
      <n-step title="发布" :disabled="!hasDraft" @click="hasDraft && (activeTab = 'publish')" />
    </n-steps>

    <!-- Source Selection Tab -->
    <section v-if="activeTab === 'source'" class="tab-content">
      <n-card>
        <template #header>选择生成来源</template>
        <n-form label-placement="top">
          <n-form-item label="来源类型">
            <n-radio-group v-model:value="source">
              <n-radio value="mcp_tool">MCP 工具</n-radio>
              <n-radio value="script_config">脚本配置</n-radio>
              <n-radio value="coroot">Coroot 服务</n-radio>
            </n-radio-group>
          </n-form-item>

          <template v-if="source === 'mcp_tool'">
            <n-form-item label="工具名称"><n-input v-model:value="toolName" placeholder="e.g. list-services" /></n-form-item>
            <n-form-item label="工具描述"><n-input v-model:value="toolDesc" placeholder="e.g. Lists all monitored services" /></n-form-item>
            <n-form-item label="Input Schema (JSON)"><n-input v-model:value="inputSchemaText" type="textarea" :rows="4" placeholder='{"properties":{}}' /></n-form-item>
          </template>

          <template v-if="source === 'script_config'">
            <n-form-item label="ScriptConfigProfile (JSON)"><n-input v-model:value="scriptConfigText" type="textarea" :rows="6" placeholder='{"scriptName":"restart-service","description":"..."}' /></n-form-item>
          </template>

          <template v-if="source === 'coroot'">
            <n-form-item label="服务类型"><n-input v-model:value="serviceType" placeholder="e.g. web-api" /></n-form-item>
            <n-form-item label="Query Schema (JSON)"><n-input v-model:value="querySchemaText" type="textarea" :rows="4" placeholder='{"properties":{}}' /></n-form-item>
          </template>
        </n-form>

        <n-button type="primary" :loading="loading" @click="generate">
          {{ loading ? '生成中…' : '生成草稿' }}
        </n-button>
      </n-card>
    </section>

    <!-- Preview Tab -->
    <section v-if="activeTab === 'preview' && hasDraft" class="tab-content">
      <div class="section-card">
        <h2>草稿预览 — {{ draftType }}</h2>
        <pre class="preview-output">{{ JSON.stringify(currentDraft, null, 2) }}</pre>
        <div class="gen-actions">
          <button class="btn-sm" @click="preview" :disabled="previewLoading">
            {{ previewLoading ? '加载中…' : '渲染预览' }}
          </button>
          <button class="btn-sm" @click="activeTab = 'validate'">校验</button>
        </div>
        <pre v-if="previewResult" class="preview-output">{{ JSON.stringify(previewResult, null, 2) }}</pre>
      </div>
    </section>

    <!-- Validate Tab -->
    <section v-if="activeTab === 'validate' && hasDraft" class="tab-content">
      <div class="section-card">
        <h2>校验结果</h2>
        <div class="gen-actions">
          <button class="btn-primary" @click="lint" :disabled="lintLoading">
            {{ lintLoading ? '校验中…' : '运行校验' }}
          </button>
        </div>
        <div v-if="lintResult">
          <div class="lint-status" :class="{ valid: lintResult.valid, invalid: !lintResult.valid }">
            {{ lintResult.valid ? '✓ 校验通过' : '✗ 存在问题' }}
          </div>
          <div v-for="(issue, i) in (lintResult.issues || [])" :key="i" class="lint-issue" :class="issue.level">
            <span class="lint-level">{{ issue.level }}</span>
            <span class="lint-field">{{ issue.field }}</span>
            <span>{{ issue.message }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- Publish Tab -->
    <section v-if="activeTab === 'publish' && hasDraft" class="tab-content">
      <div class="section-card">
        <h2>发布草稿</h2>
        <p>将草稿状态从 draft 变更为 active 并持久化。</p>
        <div class="gen-actions">
          <button class="btn-primary" @click="publish" :disabled="publishLoading">
            {{ publishLoading ? '发布中…' : '确认发布' }}
          </button>
        </div>
        <div v-if="publishResult" class="publish-result">
          <div class="lint-status valid">✓ 发布成功</div>
          <pre class="preview-output">{{ JSON.stringify(publishResult, null, 2) }}</pre>
        </div>
      </div>
    </section>
  </section>
</template>

<style scoped>
.gen-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.gen-hero {
  padding: 22px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.05);
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: #475569;
  font: inherit;
  cursor: pointer;
}

.gen-kicker {
  display: inline-flex;
  margin-top: 12px;
  padding: 6px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.gen-hero h1 { margin: 12px 0 8px; font-size: 30px; }
.gen-hero p { margin: 0; color: #475569; line-height: 1.7; }

.tab-bar {
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  width: fit-content;
}
.tab-bar button {
  padding: 10px 20px;
  border: none;
  border-radius: 10px;
  background: transparent;
  font: inherit;
  cursor: pointer;
  color: #475569;
  font-weight: 500;
}
.tab-bar button.active { background: #0f172a; color: #fff; }
.tab-bar button:disabled { opacity: 0.4; cursor: default; }

.section-card {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}
.section-card h2 { margin: 0 0 14px; font-size: 18px; }

.source-selector {
  display: flex;
  gap: 18px;
  margin-bottom: 16px;
}
.source-selector label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  cursor: pointer;
}

.form-grid { display: grid; gap: 12px; }
.form-grid label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: #475569; }
.form-grid input, .form-grid textarea {
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
  width: 100%;
}

.gen-actions { margin-top: 16px; display: flex; gap: 8px; }

.btn-primary {
  padding: 8px 18px;
  border: none;
  border-radius: 10px;
  background: #1d4ed8;
  color: #fff;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.btn-primary:hover { background: #1e40af; }
.btn-primary:disabled { opacity: 0.5; cursor: default; }

.btn-sm {
  padding: 5px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
  font-size: 12px;
  cursor: pointer;
  color: #334155;
}
.btn-sm:hover { background: #f8fafc; }

.preview-output {
  margin-top: 14px;
  padding: 14px;
  border-radius: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
}

.lint-status {
  padding: 10px 16px;
  border-radius: 10px;
  font-weight: 600;
  font-size: 14px;
  margin-bottom: 12px;
}
.lint-status.valid { background: #f0fdf4; color: #166534; }
.lint-status.invalid { background: #fef2f2; color: #991b1b; }

.lint-issue {
  display: flex;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  margin-bottom: 6px;
}
.lint-issue.error { background: #fef2f2; }
.lint-issue.warning { background: #fffbeb; }
.lint-issue.info { background: #f0f9ff; }
.lint-level { font-weight: 700; text-transform: uppercase; font-size: 11px; min-width: 60px; }
.lint-field { color: #64748b; font-family: monospace; min-width: 80px; }

.publish-result { margin-top: 14px; }

@media (max-width: 760px) {
  .gen-page { padding: 16px; }
  .source-selector { flex-direction: column; gap: 8px; }
}
</style>
