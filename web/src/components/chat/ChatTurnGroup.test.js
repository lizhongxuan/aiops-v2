import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ChatTurnGroup from "./ChatTurnGroup.vue";
import { buildCodexProcessTranscript } from "../../lib/codexProcessTranscript";

describe("ChatTurnGroup", () => {
  it("uses transcript as the only single-host process surface", () => {
    const wrapper = mount(ChatTurnGroup, {
      props: {
        showLiveStatus: true,
        turn: {
          id: "turn-1",
          active: true,
          phase: "thinking",
          processLabel: "正在思考",
          statusCard: {
            state: "failed",
            message: "Failed after 0s",
          },
          userMessage: {
            id: "user-1",
            card: {
              id: "user-1",
              type: "UserMessageCard",
              role: "user",
              text: "查看A股情况",
            },
          },
          processTranscript: buildCodexProcessTranscript({
            turnId: "turn-1",
            active: true,
            status: "running",
            elapsedLabel: "1s",
            modelRunning: true,
            processItems: [
              {
                id: "search-1",
                kind: "search",
                displayKind: "browser.search",
                text: "正在搜索网页",
                inputSummary: "2026-04-29 A股 大盘",
                status: "running",
              },
            ],
          }),
        },
      },
      slots: {
        "live-status": "<div data-testid=\"single-live-status\">Working for 1s</div>",
      },
    });

    expect(wrapper.find('[data-testid="chat-process-fold-turn-1"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="single-live-status"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("Failed after 0s");
    expect(wrapper.find(".chat-turn-status").exists()).toBe(false);
  });

  it("does not reset the user-controlled process fold state on same-turn streaming updates", async () => {
    const makeTurn = (elapsedLabel) => ({
      id: "turn-1",
      active: true,
      phase: "thinking",
      userMessage: {
        id: "user-1",
        card: {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看A股情况",
        },
      },
      processTranscript: buildCodexProcessTranscript({
        turnId: "turn-1",
        active: true,
        status: "running",
        elapsedLabel,
        modelRunning: true,
        processItems: [
          {
            id: "search-1",
            kind: "search",
            displayKind: "browser.search",
            inputSummary: "2026-04-29 A股 大盘",
            status: "completed",
          },
        ],
      }),
    });
    const wrapper = mount(ChatTurnGroup, {
      props: {
        turn: makeTurn("56s"),
      },
    });

    const header = wrapper.get('[data-testid="chat-process-header"]');
    expect(header.attributes("aria-expanded")).toBe("true");

    await header.trigger("click");
    expect(header.attributes("aria-expanded")).toBe("false");

    await wrapper.setProps({ turn: makeTurn("57s") });

    expect(wrapper.get('[data-testid="chat-process-header"]').attributes("aria-expanded")).toBe("false");
  });

  it("auto-collapses the process fold when the same turn finishes with a final answer", async () => {
    const makeTurn = ({ active, finalMessage = null, status = "running" }) => ({
      id: "turn-auto-collapse",
      active,
      phase: active ? "thinking" : "completed",
      userMessage: {
        id: "user-1",
        card: {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "查看主机的资源情况",
        },
      },
      processTranscript: buildCodexProcessTranscript({
        turnId: "turn-auto-collapse",
        active,
        status,
        elapsedLabel: active ? "5s" : "7s",
        modelRunning: active,
        processItems: [
          {
            id: "cmd-1",
            kind: "command",
            displayKind: "host.command",
            inputSummary: "sysctl -n hw.ncpu",
            command: "sysctl -n hw.ncpu",
            status: active ? "running" : "completed",
          },
        ],
      }),
      finalMessage,
    });
    const wrapper = mount(ChatTurnGroup, {
      props: {
        turn: makeTurn({ active: true }),
      },
    });

    expect(wrapper.get('[data-testid="chat-process-header"]').attributes("aria-expanded")).toBe("true");

    await wrapper.setProps({
      turn: makeTurn({
        active: false,
        status: "completed",
        finalMessage: {
          id: "assistant-final",
          card: {
            id: "assistant-final",
            type: "AssistantMessageCard",
            role: "assistant",
            status: "completed",
            text: "主机资源情况正常。",
          },
        },
      }),
    });

    expect(wrapper.get('[data-testid="chat-process-header"]').attributes("aria-expanded")).toBe("false");
  });

  it("renders the live thinking status below the streaming final answer", () => {
    const wrapper = mount(ChatTurnGroup, {
      props: {
        turn: {
          id: "turn-thinking-bottom",
          active: true,
          phase: "thinking",
          userMessage: {
            id: "user-1",
            card: {
              id: "user-1",
              type: "UserMessageCard",
              role: "user",
              text: "帮我启动docker,跑一个nginx的容器",
            },
          },
          processTranscript: buildCodexProcessTranscript({
            turnId: "turn-thinking-bottom",
            active: true,
            status: "running",
            elapsedLabel: "2s",
            modelRunning: true,
            processItems: [],
          }),
          finalMessage: {
            id: "assistant-streaming",
            card: {
              id: "assistant-streaming",
              type: "AssistantMessageCard",
              role: "assistant",
              status: "inProgress",
              text: "我将先检查这台 macOS 主机上的 Docker 是否已安装且服务是否可用。",
            },
          },
        },
      },
    });

    const processFold = wrapper.get('[data-testid="chat-process-fold-turn-thinking-bottom"]');
    const finalMessage = wrapper.get(".chat-turn-final");
    const thinking = wrapper.get('[data-testid="turn-thinking-status"]');
    expect(processFold.find('[data-testid="process-thinking-status"]').exists()).toBe(false);
    expect(thinking.text()).toContain("正在思考");
    const thinkingRow = wrapper.get('[data-testid="turn-thinking-row"]');
    expect(thinkingRow.classes()).toContain("chat-turn-thinking-row");
    expect(thinkingRow.classes()).not.toContain("stream-row");
    expect(thinkingRow.classes()).not.toContain("row-assistant");
    expect(thinking.element.closest(".stream-row.row-assistant")).toBeNull();
    expect(finalMessage.element.compareDocumentPosition(thinking.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("keeps the live thinking status below the final answer when only the final message is still streaming", () => {
    const wrapper = mount(ChatTurnGroup, {
      props: {
        turn: {
          id: "turn-final-only-streaming",
          active: false,
          phase: "completed",
          userMessage: {
            id: "user-1",
            card: {
              id: "user-1",
              type: "UserMessageCard",
              role: "user",
              text: "查看今天BTC行情",
            },
          },
          processTranscript: buildCodexProcessTranscript({
            turnId: "turn-final-only-streaming",
            active: false,
            status: "completed",
            elapsedLabel: "27s",
            modelRunning: false,
            processItems: [],
          }),
          finalMessage: {
            id: "assistant-streaming",
            card: {
              id: "assistant-streaming",
              type: "AssistantMessageCard",
              role: "assistant",
              status: "streaming",
              text: "已拿到 CoinGecko 和 CoinMarketCap 两个来源的当日数据。",
            },
          },
        },
      },
    });

    const finalMessage = wrapper.get(".chat-turn-final");
    const thinkingRow = wrapper.get('[data-testid="turn-thinking-row"]');
    expect(wrapper.get('[data-testid="turn-thinking-status"]').text()).toContain("正在思考");
    expect(finalMessage.element.compareDocumentPosition(thinkingRow.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(wrapper.element.lastElementChild).toBe(thinkingRow.element);
  });

  it("does not render stale process thinking placeholders above the streaming final answer", () => {
    const wrapper = mount(ChatTurnGroup, {
      props: {
        turn: {
          id: "turn-thinking-placeholder",
          active: true,
          phase: "thinking",
          userMessage: {
            id: "user-1",
            card: {
              id: "user-1",
              type: "UserMessageCard",
              role: "user",
              text: "查看今天A股行情",
            },
          },
          processTranscript: buildCodexProcessTranscript({
            turnId: "turn-thinking-placeholder",
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
                id: "reasoning-placeholder",
                kind: "reasoning",
                displayKind: "reasoning.summary",
                status: "completed",
                text: "正在思考",
              },
              {
                id: "search-1",
                kind: "search",
                displayKind: "browser.search",
                inputSummary: "今天 A 股行情",
                status: "completed",
              },
            ],
          }),
          finalMessage: {
            id: "assistant-streaming",
            card: {
              id: "assistant-streaming",
              type: "AssistantMessageCard",
              role: "assistant",
              status: "streaming",
              text: "目前仅拿到部分可验证结果，下一步我会继续交叉核验。",
            },
          },
        },
      },
    });

    const processFold = wrapper.get('[data-testid="chat-process-fold-turn-thinking-placeholder"]');
    const finalMessage = wrapper.get(".chat-turn-final");
    const thinking = wrapper.get('[data-testid="turn-thinking-status"]');
    expect(processFold.text()).not.toContain("正在思考");
    expect(wrapper.findAll('[data-testid="turn-thinking-status"]')).toHaveLength(1);
    expect(finalMessage.text()).toContain("目前仅拿到部分可验证结果");
    expect(finalMessage.element.compareDocumentPosition(thinking.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("keeps the live thinking status as the last visible row while the turn is active", () => {
    const wrapper = mount(ChatTurnGroup, {
      global: {
        stubs: {
          McpBundleHost: { template: '<section data-testid="result-bundle"><slot /></section>' },
          McpUiCardHost: { template: '<section data-testid="action-surface"><slot /></section>' },
        },
      },
      props: {
        turn: {
          id: "turn-thinking-last",
          active: true,
          phase: "thinking",
          userMessage: {
            id: "user-1",
            card: {
              id: "user-1",
              type: "UserMessageCard",
              role: "user",
              text: "查看A股行情",
            },
          },
          processTranscript: buildCodexProcessTranscript({
            turnId: "turn-thinking-last",
            active: true,
            status: "running",
            elapsedLabel: "35s",
            modelRunning: true,
            processItems: [
              {
                id: "search-1",
                kind: "search",
                displayKind: "browser.search",
                inputSummary: "今天 A 股行情",
                status: "completed",
              },
            ],
          }),
          finalMessage: {
            id: "assistant-streaming",
            card: {
              id: "assistant-streaming",
              type: "AssistantMessageCard",
              role: "assistant",
              status: "streaming",
              text: "已拿到当日收盘涨跌幅和两市成交额概况。",
            },
          },
          resultBundles: [{ id: "bundle-1", model: { title: "行情概览" } }],
          actionSurfaces: [{ id: "action-1", model: { title: "继续补充" } }],
        },
      },
    });

    const thinkingRow = wrapper.get('[data-testid="turn-thinking-row"]');
    const resultBundle = wrapper.get('[data-testid="result-bundle"]');
    const actionSurface = wrapper.get('[data-testid="action-surface"]');

    expect(resultBundle.element.compareDocumentPosition(thinkingRow.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(actionSurface.element.compareDocumentPosition(thinkingRow.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(wrapper.element.lastElementChild).toBe(thinkingRow.element);
  });

  it("keeps process fold before the final Markdown message and renders final text once", () => {
    const finalText = "最终结论：支付服务 5xx 上升，建议先回滚。\n\n- 影响：payment-api\n- 风险：高";
    const wrapper = mount(ChatTurnGroup, {
      props: {
        turn: {
          id: "turn-final-order",
          active: false,
          userMessage: {
            id: "user-1",
            card: {
              id: "user-1",
              type: "UserMessageCard",
              role: "user",
              text: "分析支付服务 5xx",
            },
          },
          processTranscript: buildCodexProcessTranscript({
            turnId: "turn-final-order",
            status: "completed",
            elapsedLabel: "11s",
            processItems: [
              {
                id: "plan-1",
                kind: "plan",
                displayKind: "plan",
                text: "查 Loki 日志",
                steps: [
                  { id: "step-1", text: "查 Prometheus metrics", status: "completed" },
                  { id: "step-2", text: "查 Loki 日志", status: "completed" },
                ],
              },
            ],
            finalText,
          }),
          finalMessage: {
            id: "assistant-final",
            card: {
              id: "assistant-final",
              type: "AssistantMessageCard",
              role: "assistant",
              status: "completed",
              text: finalText,
            },
          },
        },
      },
    });

    const processFold = wrapper.get('[data-testid="chat-process-fold-turn-final-order"]');
    const finalMessage = wrapper.get(".chat-turn-final");
    expect(processFold.element.compareDocumentPosition(finalMessage.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(wrapper.find(".chat-turn-final .message-text.markdown-body").exists()).toBe(true);
    expect(wrapper.find(".chat-turn-final .message-text.markdown-body").findAll("li")).toHaveLength(2);
    expect((wrapper.text().match(/最终结论/g) || [])).toHaveLength(1);
    expect(processFold.text()).not.toContain("最终结论");
  });
});
