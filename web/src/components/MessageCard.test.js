import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import MessageCard from "./MessageCard.vue";

describe("MessageCard", () => {
  it("renders streaming assistant markdown incrementally instead of plain text", () => {
    const wrapper = mount(MessageCard, {
      props: {
        showCopyButton: false,
        card: {
          id: "assistant-streaming",
          role: "assistant",
          status: "inProgress",
          text: "主机当前资源情况如下：\n\n- **CPU**：空闲 80%\n- **内存**：压力正常",
        },
      },
    });

    expect(wrapper.find('[data-testid="message-streaming-plain"]').exists()).toBe(false);
    const markdown = wrapper.find(".message-text.markdown-body.is-streaming");
    expect(markdown.exists()).toBe(true);
    expect(markdown.find("ul").exists()).toBe(true);
    expect(markdown.findAll("li")).toHaveLength(2);
    expect(markdown.find("strong").text()).toBe("CPU");
  });

  it("treats provider streaming status as active streaming markdown", () => {
    const wrapper = mount(MessageCard, {
      props: {
        showCopyButton: false,
        card: {
          id: "assistant-provider-streaming",
          role: "assistant",
          status: "streaming",
          text: "BTC 概况：\n\n- 价格：$75,410.87\n- 24 小时涨跌：+1.7%",
        },
      },
    });

    expect(wrapper.find('[data-testid="message-streaming-plain"]').exists()).toBe(false);
    const markdown = wrapper.find(".message-text.markdown-body.is-streaming");
    expect(markdown.exists()).toBe(true);
    expect(markdown.find("ul").exists()).toBe(true);
  });

  it("does not decorate partial streaming markdown with inline chips", () => {
    const wrapper = mount(MessageCard, {
      props: {
        showCopyButton: false,
        card: {
          id: "assistant-streaming-market",
          role: "assistant",
          status: "streaming",
          text: "BTC 价格：$75,410.87\n\n- 24 小时涨跌：+1.7%",
        },
      },
    });

    expect(wrapper.find(".message-text.markdown-body.is-streaming").exists()).toBe(true);
    expect(wrapper.find(".inline-entity-chip").exists()).toBe(false);
  });
});
