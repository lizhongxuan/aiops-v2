import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: "../dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 3000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("/node_modules/@vue-flow/")) return "vue-flow";
          if (id.includes("/node_modules/naive-ui/") || id.includes("/node_modules/vueuc/")) return "naive-ui";
          if (id.includes("/node_modules/lucide-vue-next/")) return "icons";
          if (id.includes("/node_modules/monaco-editor/")) return "monaco-editor";
          if (id.includes("/node_modules/vue/") || id.includes("/node_modules/@vue/")) return "vue-vendor";
          return undefined;
        },
      },
    },
  },
  server: {
    port: 5174,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:18080",
        changeOrigin: true,
      },
      "/ws": {
        target: "ws://127.0.0.1:18080",
        ws: true,
      },
    },
  },
});
