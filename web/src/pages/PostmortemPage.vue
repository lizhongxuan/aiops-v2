<script setup>
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useAppStore } from "../store";
import ActionProposalCard from "../components/runbooks/ActionProposalCard.vue";
import IncidentTimeline from "../components/incidents/IncidentTimeline.vue";
import "./erpSrePages.css";

const route = useRoute();
const store = useAppStore();
const postmortemId = computed(() => String(route.params.postmortemId || ""));
const postmortem = computed(() => store.incidents.active?.postmortem || {});

function asArray(value) {
  return Array.isArray(value) ? value : [];
}
</script>

<template>
  <section class="erp-sre-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">Postmortem</p>
          <h2>复盘草稿</h2>
          <div class="erp-sre-metadata">
            <span class="erp-sre-pill">{{ postmortem.id || postmortemId || "未选择复盘" }}</span>
            <span class="erp-sre-pill">Draft</span>
          </div>
        </div>
        <div class="erp-sre-actions">
          <RouterLink class="erp-sre-link-secondary" to="/incidents">事故工作台</RouterLink>
        </div>
      </header>

      <section class="erp-sre-grid two">
        <article class="erp-sre-panel">
          <h3>时间线</h3>
          <IncidentTimeline :items="asArray(postmortem.timeline)" empty-text="暂无时间线" />
        </article>
        <article class="erp-sre-panel">
          <h3>影响面</h3>
          <IncidentTimeline :items="asArray(postmortem.impact)" empty-text="暂无影响面" />
        </article>
        <article class="erp-sre-panel">
          <h3>根因与促成因素</h3>
          <p>{{ postmortem.rootCause || "待补充" }}</p>
        </article>
        <article class="erp-sre-panel">
          <h3>后续行动</h3>
          <div class="postmortem-action-list">
            <ActionProposalCard v-for="item in asArray(postmortem.actions)" :key="item.id || item.title" :action="item" />
            <p v-if="!asArray(postmortem.actions).length">暂无行动项</p>
          </div>
        </article>
        <article class="erp-sre-panel">
          <h3>Approvals</h3>
          <IncidentTimeline :items="asArray(postmortem.approvals)" empty-text="暂无审批记录" />
        </article>
        <article class="erp-sre-panel">
          <h3>Verification</h3>
          <IncidentTimeline :items="asArray(postmortem.verification)" empty-text="暂无验证记录" />
        </article>
        <article class="erp-sre-panel">
          <h3>Follow-ups</h3>
          <IncidentTimeline :items="asArray(postmortem.followUps)" empty-text="暂无后续事项" />
        </article>
      </section>
    </div>
  </section>
</template>

<style scoped>
.postmortem-action-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
