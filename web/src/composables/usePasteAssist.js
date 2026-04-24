import { computed, onBeforeUnmount, ref, watch } from "vue";

function countLines(text) {
  if (!text) return 0;
  return String(text).split(/\r?\n/).length;
}

function isLargePaste(text, minChars, minLines) {
  const normalized = String(text || "");
  return normalized.length >= minChars || countLines(normalized) >= minLines;
}

function normalizePathCandidate(line) {
  return String(line || "").trim().replace(/^['"]|['"]$/g, "");
}

function isPathLikeLine(line) {
  const value = normalizePathCandidate(line);
  if (!value) return false;
  if (value.length > 320) return false;
  if (/^file:\/\//i.test(value)) return true;
  if (/^(~\/|\.{1,2}\/|\/|[A-Za-z]:[\\/])/.test(value)) return true;
  if ((value.includes("/") || value.includes("\\")) && !/\s{2,}/.test(value) && !/: /.test(value)) {
    return true;
  }
  return false;
}

function extractPathList(text) {
  const lines = String(text || "")
    .split(/\r?\n/)
    .map((line) => normalizePathCandidate(line))
    .filter(Boolean);

  if (!lines.length || lines.length > 12) return [];
  if (!lines.every(isPathLikeLine)) return [];
  return lines;
}

function fileArrayFromTransfer(transfer) {
  if (!transfer?.files) return [];
  return Array.from(transfer.files).filter(Boolean);
}

function resolveImageArtifact(transfer, source) {
  const files = fileArrayFromTransfer(transfer).filter((file) => String(file.type || "").startsWith("image/"));
  if (!files.length) return null;
  return {
    kind: "image",
    source,
    count: files.length,
    names: files.map((file) => file.name || "image"),
  };
}

function resolvePathArtifact(transfer, source) {
  const text = transfer?.getData?.("text/plain") ?? "";
  const paths = extractPathList(text);
  if (!paths.length) return null;
  return {
    kind: "paths",
    source,
    count: paths.length,
    items: paths,
  };
}

function summarizeArtifactLabel(artifact) {
  if (!artifact) return "";
  if (artifact.kind === "image") {
    return artifact.count > 1 ? `图片 ${artifact.count}` : "图片 1";
  }
  return artifact.count > 1 ? `路径 ${artifact.count}` : "路径 1";
}

export function usePasteAssist(
  modelValue,
  {
    minChars = 240,
    minLines = 6,
    bufferMs = 420,
    readyMs = 2600,
    focusHintMs = 1800,
  } = {},
) {
  const state = ref("idle");
  const pasteMeta = ref(null);
  const artifactMeta = ref(null);
  const focusHintKind = ref("");

  let bufferTimer = null;
  let readyTimer = null;
  let focusHintTimer = null;

  function clearBufferTimers() {
    if (bufferTimer) {
      clearTimeout(bufferTimer);
      bufferTimer = null;
    }
    if (readyTimer) {
      clearTimeout(readyTimer);
      readyTimer = null;
    }
  }

  function clearFocusHintTimer() {
    if (focusHintTimer) {
      clearTimeout(focusHintTimer);
      focusHintTimer = null;
    }
  }

  function clearBufferState() {
    clearBufferTimers();
    state.value = "idle";
    pasteMeta.value = null;
  }

  function clearPendingArtifact() {
    artifactMeta.value = null;
    focusHintKind.value = "";
    clearFocusHintTimer();
  }

  function resetPasteState() {
    clearBufferState();
    clearPendingArtifact();
  }

  function scheduleReadyState() {
    bufferTimer = setTimeout(() => {
      state.value = "ready";
      readyTimer = setTimeout(() => {
        clearBufferState();
      }, readyMs);
      bufferTimer = null;
    }, bufferMs);
  }

  function setPasteMeta(text) {
    pasteMeta.value = {
      text,
      charCount: String(text || "").length,
      lineCount: countLines(text),
    };
  }

  function setArtifactMeta(meta) {
    clearBufferState();
    artifactMeta.value = meta;
    focusHintKind.value = "";
    clearFocusHintTimer();
  }

  function maybeShowFocusHint(kind) {
    if (!kind) return false;
    focusHintKind.value = kind;
    clearFocusHintTimer();
    focusHintTimer = setTimeout(() => {
      focusHintKind.value = "";
      focusHintTimer = null;
    }, focusHintMs);
    return true;
  }

  function handlePaste(event) {
    const transfer = event?.clipboardData;
    const imageArtifact = resolveImageArtifact(transfer, "paste");
    if (imageArtifact) {
      event?.preventDefault?.();
      setArtifactMeta(imageArtifact);
      return true;
    }

    const pathArtifact = resolvePathArtifact(transfer, "paste");
    if (pathArtifact) {
      event?.preventDefault?.();
      setArtifactMeta(pathArtifact);
      return true;
    }

    const text = transfer?.getData?.("text/plain") ?? "";
    if (!isLargePaste(text, minChars, minLines)) {
      return false;
    }
    clearPendingArtifact();
    state.value = "buffering";
    setPasteMeta(text);
    scheduleReadyState();
    return true;
  }

  function handleDrop(event) {
    const transfer = event?.dataTransfer;
    const imageArtifact = resolveImageArtifact(transfer, "drop");
    if (imageArtifact) {
      event?.preventDefault?.();
      setArtifactMeta(imageArtifact);
      return true;
    }

    const pathArtifact = resolvePathArtifact(transfer, "drop");
    if (pathArtifact) {
      event?.preventDefault?.();
      setArtifactMeta(pathArtifact);
      return true;
    }

    return false;
  }

  function handleFocus() {
    if (artifactMeta.value) {
      maybeShowFocusHint("artifact");
      return true;
    }
    if (state.value === "buffering" || state.value === "ready") {
      maybeShowFocusHint("paste");
      return true;
    }
    return false;
  }

  function handleBlur() {
    focusHintKind.value = "";
    clearFocusHintTimer();
  }

  const sendBlocked = computed(() => state.value === "buffering");
  const hasPendingArtifact = computed(() => !!artifactMeta.value);
  const artifactPills = computed(() => {
    if (!artifactMeta.value) return [];
    return [
      {
        id: `artifact-${artifactMeta.value.kind}`,
        kind: artifactMeta.value.kind,
        label: summarizeArtifactLabel(artifactMeta.value),
      },
    ];
  });

  const indicator = computed(() => {
    if (focusHintKind.value === "artifact" && artifactMeta.value) {
      if (artifactMeta.value.kind === "image") {
        return {
          kind: "focus",
          text: `已恢复输入焦点，${artifactMeta.value.count} 张图片仍待处理，可继续描述需求。`,
        };
      }
      return {
        kind: "focus",
        text: `已恢复输入焦点，${artifactMeta.value.count} 个路径仍待处理，可继续说明用途。`,
      };
    }

    if (focusHintKind.value === "paste" && pasteMeta.value) {
      return {
        kind: "focus",
        text: "已恢复输入焦点，可继续检查刚才粘贴的内容。",
      };
    }

    if (artifactMeta.value?.kind === "image") {
      return {
        kind: "artifact",
        text: `已识别 ${artifactMeta.value.count} 张图片，先保留为输入提示，可继续描述需求。`,
      };
    }

    if (artifactMeta.value?.kind === "paths") {
      return {
        kind: "artifact",
        text: `已识别 ${artifactMeta.value.count} 个路径，先保留为输入提示，可继续补充说明。`,
      };
    }

    if (!pasteMeta.value || state.value === "idle") return null;
    if (state.value === "buffering") {
      return {
        kind: "buffering",
        text: `已粘贴 ${pasteMeta.value.lineCount} 行内容，正在整理，稍后可发送。`,
      };
    }
    return {
      kind: "ready",
      text: `已粘贴 ${pasteMeta.value.lineCount} 行内容，可继续检查后发送。`,
    };
  });

  watch(
    () => modelValue?.value,
    (value) => {
      if (!value && !artifactMeta.value) {
        clearBufferState();
      }
    },
  );

  onBeforeUnmount(() => {
    clearBufferTimers();
    clearFocusHintTimer();
  });

  return {
    artifactPills,
    handleBlur,
    handleDrop,
    handleFocus,
    handlePaste,
    hasPendingArtifact,
    indicator,
    sendBlocked,
    clearPendingArtifact,
    resetPasteState,
  };
}
