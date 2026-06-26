import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { AnswerDocumentRenderer } from "./AnswerDocumentRenderer";

describe("AnswerDocumentRenderer", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("renders a deferred Coroot chart notice after the root cause text", async () => {
    await act(async () => {
      root.render(
        <AnswerDocumentRenderer
          finalText={[
            "根因：外部依赖 external:18090 unknown。",
            "",
            "证据：",
            "- Coroot RCA 查询成功",
          ].join("\n")}
          artifacts={[]}
          deferredArtifacts={[{ id: "artifact-coroot-net", type: "coroot_chart" }]}
        />,
      );
    });

    const text = container.textContent || "";
    expect(text).toContain("根因");
    expect(text.indexOf("外部依赖 external:18090 unknown")).toBeLessThan(text.indexOf("已生成 Coroot 图表，分析完成后展开"));
    expect(text.indexOf("已生成 Coroot 图表，分析完成后展开")).toBeLessThan(text.indexOf("Coroot RCA 查询成功"));
  });

  it("marks the rendered final answer with a stable selector", async () => {
    await act(async () => {
      root.render(<AnswerDocumentRenderer finalText="根因：timeline 分叉。" artifacts={[]} deferredArtifacts={[]} />);
    });

    expect(container.querySelector('[data-testid="aiops-final-text"]')?.textContent).toContain("timeline 分叉");
  });

  it("renders a ready Coroot chart at the semantic slot", async () => {
    await act(async () => {
      root.render(
        <AnswerDocumentRenderer
          finalText="根因：CPU 使用率升高。"
          deferredArtifacts={[]}
          artifacts={[
            {
              id: "artifact-coroot-cpu",
              type: "coroot_chart",
              titleZh: "aiops-host-agent 服务",
              inlineData: {
                mcpCard: {
                  uiKind: "readonly_chart",
                  title: "指标趋势",
                  visual: {
                    kind: "timeseries",
                    series: [{ name: "cpu_usage", data: [{ timestamp: 1, value: 1 }] }],
                  },
                },
              },
            },
          ]}
        />,
      );
    });

    expect(container.textContent).toContain("aiops-host-agent 服务");
    expect(container.textContent).toContain("cpu_usage");
    expect(container.textContent).not.toContain("已生成 Coroot 图表，分析完成后展开");
  });
});
