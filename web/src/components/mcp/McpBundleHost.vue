<script setup>
import { computed } from "vue";
import McpMonitorBundleCard from "./McpMonitorBundleCard.vue";
import McpRemediationBundleCard from "./McpRemediationBundleCard.vue";
import { normalizeMcpBundle } from "../../lib/mcpUiCardModel";

const props = defineProps({
  bundle: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["action", "open-detail", "pin"]);

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

const normalizedBundle = computed(() => normalizeMcpBundle(props.bundle || {}));
const placement = computed(() => compactText(props.bundle?.placement || "inline_final") || "inline_final");
const bundleComponent = computed(() => {
  return normalizedBundle.value.bundleKind === "remediation_bundle"
    ? McpRemediationBundleCard
    : McpMonitorBundleCard;
});

function handleAction(payload) {
  emit("action", payload);
}

function handleDetail(payload) {
  emit("open-detail", payload);
}

function handlePin(payload) {
  emit("pin", payload);
}
</script>

<template>
  <section
    class="mcp-bundle-host"
    :class="[`placement-${placement}`, `bundle-${normalizedBundle.bundleKind}`]"
    :data-placement="placement"
    :data-bundle-kind="normalizedBundle.bundleKind"
  >
    <component
      :is="bundleComponent"
      :bundle="normalizedBundle"
      compact
      @action="handleAction"
      @open-detail="handleDetail"
      @pin="handlePin"
    />
  </section>
</template>

<style scoped>
.mcp-bundle-host {
  min-width: 0;
}
</style>
