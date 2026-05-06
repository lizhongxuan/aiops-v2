<script setup lang="ts">
import { NConfigProvider, NMessageProvider, darkTheme } from "naive-ui";
import RunnerWorkbench from "./components/RunnerWorkbench.vue";
</script>

<template>
  <NConfigProvider :theme="darkTheme">
    <NMessageProvider>
      <RunnerWorkbench />
    </NMessageProvider>
  </NConfigProvider>
</template>
