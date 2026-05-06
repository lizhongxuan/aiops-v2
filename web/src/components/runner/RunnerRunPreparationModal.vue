<script setup>
import { computed } from "vue";
import { AlertCircleIcon, CheckCircle2Icon, ServerOffIcon, XIcon } from "lucide-vue-next";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  mode: {
    type: String,
    default: "run",
  },
  workflowName: {
    type: String,
    default: "",
  },
  readiness: {
    type: Object,
    default: () => ({ ready: false, blockers: [], warnings: [], infos: [] }),
  },
  serverReason: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["close"]);

const blockers = computed(() => (Array.isArray(props.readiness?.blockers) ? props.readiness.blockers : []));
const canRun = computed(() => Boolean(props.readiness?.ready) && blockers.value.length === 0 && !props.serverReason);
const title = computed(() => (props.mode === "dry-run" ? "Dry Run 准备检查" : "运行准备检查"));
const modeLabel = computed(() => (props.mode === "dry-run" ? "Dry Run" : "运行"));
const summary = computed(() => {
  if (blockers.value.length > 0) return `${blockers.value.length} 个配置问题需要处理`;
  if (props.serverReason) return `${modeLabel.value} 需要 Runner 服务端`;
  return `${modeLabel.value} 准备完成`;
});

function itemKey(item, index) {
  return `${item?.code || "item"}-${item?.nodeId || item?.edgeId || index}`;
}
</script>

<template>
  <section
    v-if="show"
    class="runner-readiness-backdrop"
    data-testid="runner-run-preparation-modal"
    @click.self="emit('close')"
  >
    <div class="runner-readiness-modal" role="dialog" aria-modal="true" :aria-label="title">
      <header class="runner-readiness-head">
        <div>
          <p>RUN PREFLIGHT</p>
          <h2>{{ title }}</h2>
          <span>{{ workflowName ? `${workflowName} · ${summary}` : summary }}</span>
        </div>
        <button type="button" class="workflow-icon-button" :aria-label="`关闭${title}`" @click="emit('close')">
          <XIcon :size="18" />
        </button>
      </header>

      <main class="runner-readiness-body">
        <section class="runner-readiness-status" :class="{ ready: canRun }">
          <CheckCircle2Icon v-if="canRun" :size="22" />
          <ServerOffIcon v-else-if="serverReason" :size="22" />
          <AlertCircleIcon v-else :size="22" />
          <div>
            <strong>{{ canRun ? "可以运行" : "暂时不能运行" }}</strong>
            <span v-if="serverReason">{{ serverReason }}</span>
            <span v-else-if="blockers.length">先补齐下面的配置，再重新点击{{ modeLabel }}。</span>
            <span v-else>前端 readiness 已通过，可以提交到 Runner 服务端。</span>
          </div>
        </section>

        <section v-if="blockers.length" class="runner-readiness-section">
          <h3>需要补齐</h3>
          <article v-for="(item, index) in blockers" :key="itemKey(item, index)" class="runner-readiness-item blocker">
            <AlertCircleIcon :size="16" />
            <span>{{ item.message || item.code }}</span>
          </article>
        </section>

        <section v-if="serverReason" class="runner-readiness-section">
          <h3>服务端依赖</h3>
          <article class="runner-readiness-item warning">
            <ServerOffIcon :size="16" />
            <span>保存草稿可以继续使用本地模式；{{ modeLabel }}、发布和真实执行需要 ai-server 接入 Runner API。</span>
          </article>
        </section>
      </main>

      <footer class="runner-readiness-footer">
        <button type="button" class="primary" data-testid="runner-run-preparation-close" @click="emit('close')">
          知道了
        </button>
      </footer>
    </div>
  </section>
</template>
