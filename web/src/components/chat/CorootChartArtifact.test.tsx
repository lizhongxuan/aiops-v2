import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { CorootChartArtifact } from "./CorootChartArtifact";

describe("CorootChartArtifact", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("uses metadata placement topic as default Coroot report", async () => {
    await act(async () => {
      root.render(
        <CorootChartArtifact
          artifact={{
            id: "artifact-coroot",
            type: "coroot_chart",
            metadata: { placement: { topic: "net" } },
            inlineData: {
              mcpCard: {
                visual: {
                  kind: "coroot_report_charts",
                  reports: [
                    {
                      name: "CPU",
                      widgets: [{ chart: { title: "CPU usage", series: [{ name: "cpu", data: [1, 2] }] } }],
                    },
                    {
                      name: "Net",
                      widgets: [{ chart: { title: "Failed TCP connections", series: [{ name: "net", data: [3, 4] }] } }],
                    },
                  ],
                },
              },
            },
          }}
        />,
      );
    });

    expect(container.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toContain("Net");
  });

  it("renders native charts with the compact chat dimensions", async () => {
    await act(async () => {
      root.render(
        <CorootChartArtifact
          artifact={{
            id: "artifact-coroot",
            type: "coroot_chart",
            inlineData: {
              chartReports: [
                {
                  name: "CPU",
                  widgets: [
                    {
                      chart: {
                        title: "CPU usage",
                        series: [{ name: "cpu", data: [1, 2, 3] }],
                      },
                    },
                  ],
                },
              ],
            },
          }}
        />,
      );
    });

    expect(container.querySelector('[data-testid="coroot-native-charts"]')?.className).toContain("max-w-[640px]");
    expect(container.querySelector('svg[role="img"]')?.className.baseVal).toContain("h-[220px]");
  });

  it("filters low-value CPU charts from the Coroot report", async () => {
    await act(async () => {
      root.render(
        <CorootChartArtifact
          artifact={{
            id: "artifact-coroot",
            type: "coroot_chart",
            inlineData: {
              chartReports: [
                {
                  name: "CPU",
                  widgets: [
                    { chart: { title: "CPU usage", series: [{ name: "usage", data: [1, 2] }] } },
                    { chart: { title: "CPU delay", series: [{ name: "delay", data: [1, 2] }] } },
                    { chart: { title: "Throttled time", series: [{ name: "throttled", data: [1, 2] }] } },
                    {
                      chart_group: {
                        title: "Node CPU usage <selector>, %",
                        charts: [
                          { title: "overview", series: [{ name: "overview", data: [1, 2] }] },
                          { title: "cosmic4eye-21", series: [{ name: "cosmic4eye-21", data: [1, 2] }] },
                          { title: "cosmic4eye-22", series: [{ name: "cosmic4eye-22", data: [2, 3] }] },
                        ],
                      },
                    },
                    {
                      chart_group: {
                        title: "CPU consumers on <selector>, cores",
                        charts: [{ title: "cosmic4eye-21", series: [{ name: "rabbitmq-server", data: [1, 2] }] }],
                      },
                    },
                  ],
                },
              ],
            },
          }}
        />,
      );
    });

    const widgetLabels = Array.from(container.querySelectorAll('[data-testid="coroot-widget-tab"]')).map((tab) => tab.textContent);
    expect(widgetLabels).toEqual(["CPU usage", "CPU delay", "Node CPU usage"]);

    const nodeCpuTab = Array.from(container.querySelectorAll('[data-testid="coroot-widget-tab"]'))
      .find((tab) => tab.textContent === "Node CPU usage");
    await act(async () => {
      nodeCpuTab?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const nodeCpuChartLabels = Array.from(container.querySelectorAll('[data-testid="coroot-chart-tabs"] [role="tab"]')).map((tab) => tab.textContent);
    expect(nodeCpuChartLabels).toEqual(["cosmic4eye-21", "cosmic4eye-22"]);
  });
});
