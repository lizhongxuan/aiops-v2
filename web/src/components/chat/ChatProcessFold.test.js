import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ChatProcessFold from "./ChatProcessFold.vue";
import { buildCodexProcessTranscript } from "../../lib/codexProcessTranscript";

function turnWithTranscript(overrides = {}) {
  const processTranscript = buildCodexProcessTranscript({
    turnId: "turn-1",
    status: "completed",
    elapsedLabel: "13s",
    assistantMessages: [
      { id: "intent-1", text: "我先拉取当前主要指数和板块行情信息，给你一个简短概览。" },
    ],
    processItems: [
      {
        id: "search-1",
        kind: "search",
        displayKind: "browser.search",
        status: "completed",
        inputSummary: "2026-04-29 A股 大盘",
        results: [{ title: "A股行情", url: "https://finance.example.test/a" }],
      },
    ],
    ...overrides,
  });
  return {
    id: "turn-1",
    active: false,
    collapsedByDefault: false,
    processTranscript,
  };
}

describe("ChatProcessFold", () => {
  it("renders only transcript blocks and removes legacy summary/surface DOM", () => {
    const wrapper = mount(ChatProcessFold, {
      props: {
        turn: turnWithTranscript(),
      },
    });

    expect(wrapper.find('[data-testid="chat-process-header"]').text()).toContain("已处理 13s");
    expect(wrapper.text()).toContain("我先拉取当前主要指数和板块行情信息");
    expect(wrapper.find('[data-testid="process-step-search"]').exists()).toBe(true);
    expect(wrapper.find(".chat-process-summary").exists()).toBe(false);
    expect(wrapper.find(".chat-process-surface").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("已记录");
    expect(wrapper.text()).not.toContain("明细已折叠");
    expect(wrapper.text()).not.toContain("准备上下文");
  });

  it("does not render an empty completed fold when there are no user-visible process blocks", () => {
    const processTranscript = buildCodexProcessTranscript({
      turnId: "turn-empty",
      status: "completed",
      elapsedLabel: "1s",
      processItems: [],
      assistantMessages: [],
      finalText: "我可以帮你查看 A 股情况。",
    });

    const wrapper = mount(ChatProcessFold, {
      props: {
        turn: {
          id: "turn-empty",
          active: false,
          processTranscript,
        },
      },
    });

    expect(wrapper.find('[data-testid="chat-process-header"]').exists()).toBe(false);
    expect(wrapper.find(".chat-process-fold").exists()).toBe(false);
  });

  it("expands command rows to a bounded terminal output panel", async () => {
    const output = Array.from({ length: 10 }, (_, index) => `line-${index + 1}`).join("\n");
    const wrapper = mount(ChatProcessFold, {
      props: {
        turn: turnWithTranscript({
          processItems: [
            {
              id: "cmd-1",
              kind: "command",
              displayKind: "host.command",
              text: ["exec", "command"].join("_"),
              command: "df -h",
              outputPreview: output,
              status: "completed",
            },
          ],
        }),
      },
    });

    const commandRow = wrapper.find('[data-testid="process-step-command"]');
    expect(commandRow.exists()).toBe(true);
    expect(commandRow.text()).toContain("已运行");
    expect(commandRow.text()).toContain("df -h");
    expect(commandRow.text()).not.toContain(["exec", "command"].join("_"));
    expect(wrapper.find('[data-testid="process-terminal-preview"]').exists()).toBe(false);

    await commandRow.trigger("click");

    const terminal = wrapper.find('[data-testid="process-terminal-preview"]');
    expect(terminal.exists()).toBe(true);
    expect(terminal.text()).toContain("$ df -h");
    expect(terminal.text()).toContain("line-10");
    const outputPanel = terminal.find(".chat-terminal-preview-output");
    expect(outputPanel.attributes("style")).toContain("height: 156px");
  });

  it("expands search rows with query and result details", async () => {
    const wrapper = mount(ChatProcessFold, {
      props: {
        turn: turnWithTranscript(),
      },
    });

    const searchRow = wrapper.find('[data-testid="process-step-search"]');
    expect(searchRow.exists()).toBe(true);
    expect(wrapper.find('[data-testid="process-search-preview"]').exists()).toBe(false);

    await searchRow.trigger("click");

    const preview = wrapper.find('[data-testid="process-search-preview"]');
    expect(preview.exists()).toBe(true);
    expect(preview.text()).toContain("2026-04-29 A股 大盘");
    expect(preview.text()).toContain("A股行情");
    expect(preview.text()).toContain("https://finance.example.test/a");
  });

  it("renders plan, evidence and inline approval blocks as structured process rows", () => {
    const wrapper = mount(ChatProcessFold, {
      props: {
        turn: turnWithTranscript({
          status: "blocked",
          processItems: [
            {
              id: "plan-1",
              kind: "plan",
              displayKind: "plan",
              status: "running",
              text: "查 Loki 日志",
              steps: [
                { id: "step-1", text: "查 Prometheus metrics", status: "completed" },
                { id: "step-2", text: "查 Loki 日志", status: "running" },
              ],
            },
            {
              id: "evidence-1",
              kind: "evidence",
              displayKind: "evidence.metric",
              status: "completed",
              text: "支付服务 5xx 上升（payment-api 5xx rate > 8%）",
              source: "prometheus",
              confidence: "high",
              window: "15m",
              rawRef: "promql:5xx",
            },
            {
              id: "approval-1",
              kind: "approval",
              displayKind: "approval.command",
              status: "blocked",
              text: "等待确认",
              approvalId: "approval-1",
              approvalType: "command",
              command: "kubectl rollout undo deploy/payment-api -n prod",
              reason: "需要回滚最近导致 5xx 上升的发布。",
              risk: "high",
              targets: ["prod/payment-api"],
            },
          ],
        }),
      },
    });

    expect(wrapper.find('[data-testid="process-step-plan"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="process-step-plan"]').text()).toContain("查 Prometheus metrics");
    expect(wrapper.find('[data-testid="process-step-evidence"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="process-step-evidence"]').text()).toContain("prometheus");
    expect(wrapper.find('[data-testid="process-step-evidence"]').text()).toContain("15m");
    expect(wrapper.find('[data-testid="process-step-approval"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="process-step-approval"]').text()).toContain("kubectl rollout undo deploy/payment-api -n prod");
    expect(wrapper.find('[data-testid="process-step-approval"]').text()).toContain("high");
  });

  it("keeps thinking status out of the process fold while the model is running", () => {
    const turn = turnWithTranscript({
      active: true,
      status: "running",
      elapsedLabel: "20s",
      modelRunning: true,
      processItems: [],
    });
    const wrapper = mount(ChatProcessFold, {
      props: {
        turn,
      },
    });

    expect(turn.processTranscript.showThinking).toBe(true);
    expect(wrapper.find('[data-testid="process-thinking-status"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="process-step-reasoning"]').exists()).toBe(false);
  });

  it("filters persisted pure thinking placeholders from the process transcript", () => {
    const turn = turnWithTranscript({
      active: true,
      status: "running",
      elapsedLabel: "27s",
      modelRunning: true,
      assistantMessages: [
        {
          id: "intent-1",
          text: "我先查询今天 A 股主要指数的最新收盘/盘中数据，并优先核验权威来源后再汇总给你。",
        },
      ],
      processItems: [
        {
          id: "reasoning-placeholder-1",
          kind: "reasoning",
          displayKind: "reasoning.summary",
          status: "completed",
          text: "正在思考",
        },
        {
          id: "search-1",
          kind: "search",
          displayKind: "browser.search",
          status: "completed",
          inputSummary: "今天 A 股行情",
        },
        {
          id: "reasoning-placeholder-2",
          kind: "reasoning",
          displayKind: "reasoning.summary",
          status: "",
          text: "正在思考…",
        },
      ],
    });

    const wrapper = mount(ChatProcessFold, {
      props: {
        turn,
      },
    });

    expect(turn.processTranscript.showThinking).toBe(true);
    expect(wrapper.find('[data-testid="process-step-search"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="process-step-reasoning"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("正在思考");
  });

  it("keeps a search row expanded when the same query receives a new event id", async () => {
    const wrapper = mount(ChatProcessFold, {
      props: {
        turn: turnWithTranscript({
          active: true,
          status: "running",
          processItems: [
            {
              id: "search-running-1",
              kind: "search",
              displayKind: "browser.search",
              status: "running",
              inputSummary: "BTC 今日行情",
              queries: ["BTC 今日行情"],
            },
          ],
        }),
      },
    });

    await wrapper.find('[data-testid="process-step-search"]').trigger("click");
    expect(wrapper.find('[data-testid="process-search-preview"]').text()).toContain("BTC 今日行情");

    await wrapper.setProps({
      turn: turnWithTranscript({
        active: true,
        status: "running",
        processItems: [
          {
            id: "search-completed-2",
            kind: "search",
            displayKind: "browser.search",
            status: "completed",
            inputSummary: "BTC 今日行情",
            queries: ["BTC 今日行情"],
            results: [{ title: "BTC Price", url: "https://example.test/btc" }],
          },
        ],
      }),
    });

    const preview = wrapper.find('[data-testid="process-search-preview"]');
    expect(preview.exists()).toBe(true);
    expect(preview.text()).toContain("BTC 今日行情");
    expect(preview.text()).toContain("BTC Price");
  });
});
