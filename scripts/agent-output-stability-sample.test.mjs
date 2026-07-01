import test from "node:test";
import assert from "node:assert/strict";

import { hasActiveRuntimeWork } from "./agent-output-stability-sample.mjs";

test("does not treat user prompt text as post-final runtime activity", () => {
  const active = hasActiveRuntimeWork({
    statusText: "已处理 2m 47s",
    bodyText: "用户参考步骤里写着：确保pg正在运行，然后查看 archive recovery complete。",
    processBlocks: [
      { testId: "aiops-process-header", text: "已处理 2m 47s" },
      { testId: "aiops-final-text", text: "结论：需要先做只读证据核对。" },
    ],
  });

  assert.equal(active, false);
});

test("detects active runtime work from process status surfaces", () => {
  const active = hasActiveRuntimeWork({
    statusText: "处理中 1m 15s",
    bodyText: "用户参考步骤里写着：确保pg正在运行。",
    processBlocks: [
      { testId: "aiops-process-header", text: "处理中 1m 15s" },
      { testId: "aiops-model-wait-pill", text: "正在等待模型返回" },
    ],
  });

  assert.equal(active, true);
});
