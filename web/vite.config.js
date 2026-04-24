import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

function resolveMonacoChunkName(id) {
  const marker = "/monaco-editor/esm/vs/";
  const markerIndex = id.indexOf(marker);
  if (markerIndex === -1) return null;

  const segments = id
    .slice(markerIndex + marker.length)
    .split("/")
    .filter(Boolean);

  const scope = segments[0] || "core";
  const area = segments[1] || "shared";

  if (scope === "platform") {
    return "monaco-platform";
  }

  if (scope === "editor") {
    const feature = segments[2] || "shared";
    if (area === "common" && (feature === "model" || feature === "services")) {
      return "monaco-editor-common-runtime";
    }
    if (area === "browser" && (feature === "services" || feature.startsWith("editorExtensions"))) {
      return "monaco-editor-browser-runtime";
    }
    return `monaco-editor-${area}-${feature}`;
  }

  return `monaco-${scope}-${area}`;
}

export default defineConfig({
  plugins: [vue()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          const monacoChunkName = resolveMonacoChunkName(id);
          if (monacoChunkName) {
            return monacoChunkName;
          }
          if (id.includes("/node_modules/@xterm/")) {
            return "xterm";
          }
          return undefined;
        },
      },
    },
  },
  server: {
    port: 5173,
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
