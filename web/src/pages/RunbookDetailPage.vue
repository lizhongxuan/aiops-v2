<script setup>
import { computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useAppStore } from "../store";
import ActionProposalCard from "../components/runbooks/ActionProposalCard.vue";
import RunbookMatchResult from "../components/runbooks/RunbookMatchResult.vue";
import RunbookRiskBadge from "../components/runbooks/RunbookRiskBadge.vue";
import RunbookStepList from "../components/runbooks/RunbookStepList.vue";
import "./erpSrePages.css";

const route = useRoute();
const store = useAppStore();
const runbookId = computed(() => String(route.params.runbookId || ""));
const runbook = computed(() => store.runbooks.active || {});
const steps = computed(() => Array.isArray(runbook.value.steps) ? runbook.value.steps : []);
const verifications = computed(() => Array.isArray(runbook.value.verifications) ? runbook.value.verifications : []);
const proposals = computed(() => Array.isArray(runbook.value.proposals) ? runbook.value.proposals : []);

onMounted(() => {
  if (runbookId.value) {
    void store.loadRunbook(runbookId.value);
  }
});

async function runMatchTest() {
  await store.matchRunbooks({ runbookId: runbookId.value, mode: "test" });
}
</script>

<template>
  <section class="erp-sre-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">Runbook Detail</p>
          <h2>{{ runbook.title || runbook.name || "Runbook 详情" }}</h2>
          <div class="erp-sre-metadata">
            <span class="erp-sre-pill">{{ runbookId || "未选择 Runbook" }}</span>
            <RunbookRiskBadge :risk="runbook.risk" />
            <span class="erp-sre-pill">Plan-only</span>
          </div>
        </div>
        <div class="erp-sre-actions">
          <RouterLink class="erp-sre-link-secondary" to="/runbooks">返回目录</RouterLink>
          <button class="erp-sre-link" type="button" data-testid="runbook-match-test" @click="runMatchTest">匹配测试</button>
        </div>
      </header>

      <section class="erp-sre-grid two">
        <article class="erp-sre-panel">
          <h3>步骤</h3>
          <RunbookStepList :steps="steps" />
        </article>
        <article class="erp-sre-panel">
          <h3>验证项</h3>
          <ul class="runbook-detail-list">
            <li v-if="!verifications.length">暂无验证项</li>
            <li v-for="item in verifications" :key="item.id || item.title">{{ item.title || item.name || item.id }}</li>
          </ul>
        </article>
        <article class="erp-sre-panel">
          <h3>匹配测试</h3>
          <RunbookMatchResult :matches="store.runbooks.matches" />
        </article>
        <article class="erp-sre-panel">
          <h3>动作提案</h3>
          <ActionProposalCard v-for="item in proposals" :key="item.id || item.title" :action="item" />
          <p v-if="!proposals.length">暂无动作提案</p>
        </article>
      </section>
    </div>
  </section>
</template>

<style scoped>
.runbook-detail-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.runbook-detail-list li {
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
  color: #0f172a;
  font-size: 13px;
}
</style>
