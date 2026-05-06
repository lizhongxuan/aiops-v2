<script setup>
import { computed } from "vue";
import { AlertCircleIcon, CheckCircle2Icon, InfoIcon, XIcon } from "lucide-vue-next";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  result: {
    type: Object,
    default: () => ({ ready: false, blockers: [], warnings: [], infos: [] }),
  },
  serverReason: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["close", "focus-node"]);

const blockers = computed(() => (Array.isArray(props.result?.blockers) ? props.result.blockers : []));
const warnings = computed(() => (Array.isArray(props.result?.warnings) ? props.result.warnings : []));
const infos = computed(() => (Array.isArray(props.result?.infos) ? props.result.infos : []));
const ready = computed(() => Boolean(props.result?.ready) && blockers.value.length === 0);
const summary = computed(() => {
  if (!ready.value) return `${blockers.value.length} 个问题需要处理`;
  if (props.serverReason) return "本地校验通过，服务端能力待接入";
  if (warnings.value.length > 0) return `校验通过，仍有 ${warnings.value.length} 个提醒`;
  return "校验通过，可以继续运行";
});

function itemKey(item, index) {
  return `${item?.code || "item"}-${item?.nodeId || item?.edgeId || index}`;
}
</script>

<template>
  <section
    v-if="show"
    class="runner-readiness-backdrop"
    data-testid="runner-readiness-modal"
    @click.self="emit('close')"
  >
    <div class="runner-readiness-modal" role="dialog" aria-modal="true" aria-label="工作流校验结果">
      <header class="runner-readiness-head">
        <div>
          <p>WORKFLOW CHECK</p>
          <h2>校验结果</h2>
          <span>{{ summary }}</span>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭校验结果" @click="emit('close')">
          <XIcon :size="18" />
        </button>
      </header>

      <main class="runner-readiness-body">
        <section class="runner-readiness-status" :class="{ ready }">
          <CheckCircle2Icon v-if="ready" :size="22" />
          <AlertCircleIcon v-else :size="22" />
          <div>
            <strong>{{ ready ? "本地结构校验通过" : "校验未通过" }}</strong>
            <span v-if="serverReason">{{ serverReason }}</span>
            <span v-else>{{ ready ? "节点、动作、必填参数和连线结构满足运行前检查。" : "先处理下面的阻塞项，再保存或运行。" }}</span>
          </div>
        </section>

        <section v-if="blockers.length" class="runner-readiness-section">
          <h3>需要处理</h3>
          <article v-for="(item, index) in blockers" :key="itemKey(item, index)" class="runner-readiness-item blocker">
            <AlertCircleIcon :size="16" />
            <span>{{ item.message || item.code }}</span>
            <button v-if="item.nodeId" type="button" @click="emit('focus-node', item.nodeId)">定位节点</button>
          </article>
        </section>

        <section v-if="warnings.length" class="runner-readiness-section">
          <h3>提醒</h3>
          <article v-for="(item, index) in warnings" :key="itemKey(item, index)" class="runner-readiness-item warning">
            <InfoIcon :size="16" />
            <span>{{ item.message || item.code }}</span>
            <button v-if="item.nodeId" type="button" @click="emit('focus-node', item.nodeId)">定位节点</button>
          </article>
        </section>

        <section v-if="infos.length" class="runner-readiness-section">
          <h3>默认行为</h3>
          <article v-for="(item, index) in infos" :key="itemKey(item, index)" class="runner-readiness-item info">
            <InfoIcon :size="16" />
            <span>{{ item.message || item.code }}</span>
          </article>
        </section>

        <section v-if="ready && !blockers.length && !warnings.length && !infos.length" class="runner-readiness-empty">
          当前工作流没有发现结构问题。
        </section>
      </main>

      <footer class="runner-readiness-footer">
        <button type="button" class="primary" data-testid="runner-readiness-close" @click="emit('close')">
          知道了
        </button>
      </footer>
    </div>
  </section>
</template>
