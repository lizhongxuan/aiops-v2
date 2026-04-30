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
});
