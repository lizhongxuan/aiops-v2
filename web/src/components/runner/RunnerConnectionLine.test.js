import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerConnectionLine from "./RunnerConnectionLine.vue";

describe("RunnerConnectionLine", () => {
  it("renders a Dify-style preview path from source to target", () => {
    const wrapper = mount(RunnerConnectionLine, {
      props: {
        sourceX: 20,
        sourceY: 40,
        targetX: 180,
        targetY: 120,
        connectionStatus: "valid",
      },
    });

    const path = wrapper.get('[data-testid="runner-connection-line"]');
    expect(path.attributes("d")).toContain("M20,40");
    expect(path.classes()).toContain("valid");
  });
});
