<script setup>
import { ArchiveIcon, GitBranchIcon, WorkflowIcon } from "lucide-vue-next";
import "./erpSrePages.css";

const migrationItems = [
  "workflow templates",
  "graph utils",
  "mock fixtures",
  "canvas interaction",
];
</script>

<template>
  <section class="erp-sre-page runner-entry-page">
    <div class="erp-sre-shell">
      <header class="erp-sre-heading">
        <div>
          <p class="erp-sre-kicker">Legacy Debug UI</p>
          <h2>旧 Runner UI</h2>
          <p>主应用产品入口已统一到 /runner 的 Runner Studio。本页不再出现在主导航，只保留给开发者识别旧代码生命周期。</p>
        </div>
      </header>

      <div class="erp-sre-grid two">
        <section class="erp-sre-panel runner-entry-card">
          <div class="runner-entry-icon">
            <WorkflowIcon :size="22" />
          </div>
          <h3>生命周期</h3>
          <p>Standalone Runner UI 仅作为 runner server 的 legacy/debug 页面保留，不再承载主应用工作流编排体验。</p>
        </section>

        <section class="erp-sre-panel runner-entry-card">
          <div class="runner-entry-icon">
            <ArchiveIcon :size="22" />
          </div>
          <h3>可迁移代码清单</h3>
          <ul class="runner-migration-list">
            <li v-for="item in migrationItems" :key="item">{{ item }}</li>
          </ul>
        </section>

        <section class="erp-sre-panel runner-entry-card">
          <div class="runner-entry-icon">
            <GitBranchIcon :size="22" />
          </div>
          <h3>主流程入口</h3>
          <p>/runner 由主应用原生 Runner Studio 承接，后续只接入同源 /api/runner-studio/* API。</p>
        </section>
      </div>
    </div>
  </section>
</template>

<style scoped>
.runner-entry-page {
  min-height: 100%;
}

.runner-entry-card {
  display: flex;
  min-height: 230px;
  flex-direction: column;
  gap: 12px;
}

.runner-entry-card h3 {
  margin: 0;
}

.runner-entry-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 8px;
  background: #e0f2fe;
  color: #0369a1;
}

.runner-migration-list {
  display: grid;
  gap: 8px;
  margin: 0;
  padding-left: 18px;
  color: #475569;
  font-size: 14px;
  line-height: 1.6;
}
</style>
