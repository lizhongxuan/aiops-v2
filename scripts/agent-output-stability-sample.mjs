const activeRuntimePattern = /正在等待模型返回|正在思考|正在搜索网页|正在运行|正在执行|处理中\s+\d/;

const runtimeActivityTestIds = new Set([
  "aiops-process-transcript",
  "aiops-process-header",
  "aiops-process-transcript-body",
  "aiops-model-wait-pill",
  "aiops-inline-status-indicator",
]);

export function hasActiveRuntimeWork(sample) {
  const statusText = String(sample?.statusText || "");
  const runtimeBlockText = Array.isArray(sample?.processBlocks)
    ? sample.processBlocks
        .filter((block) => runtimeActivityTestIds.has(String(block?.testId || "")))
        .map((block) => String(block?.text || ""))
        .join("\n")
    : "";
  return activeRuntimePattern.test(`${statusText}\n${runtimeBlockText}`);
}
