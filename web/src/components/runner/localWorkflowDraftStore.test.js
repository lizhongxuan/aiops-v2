import { beforeEach, describe, expect, it } from "vitest";
import {
  listLocalWorkflowDrafts,
  loadLocalWorkflowDraft,
  removeLocalWorkflowDraft,
  saveLocalWorkflowDraft,
} from "./localWorkflowDraftStore";

describe("localWorkflowDraftStore", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("stores and restores a local runner workflow draft", () => {
    saveLocalWorkflowDraft({
      id: "host-check",
      name: "检查主机资源",
      title: "检查主机资源",
      status: "draft",
      graph: { workflow: { name: "host-check" }, nodes: [], edges: [] },
    });

    expect(loadLocalWorkflowDraft("host-check")).toMatchObject({
      id: "host-check",
      name: "host-check",
      title: "检查主机资源",
      local_draft: true,
      graph: { workflow: { name: "host-check" }, nodes: [], edges: [] },
    });
  });

  it("lists and removes drafts by id", () => {
    saveLocalWorkflowDraft({ id: "one", title: "One", graph: { nodes: [], edges: [] } });
    saveLocalWorkflowDraft({ id: "two", title: "Two", graph: { nodes: [], edges: [] } });

    expect(listLocalWorkflowDrafts().map((draft) => draft.id)).toEqual(["two", "one"]);

    removeLocalWorkflowDraft("two");

    expect(loadLocalWorkflowDraft("two")).toBeNull();
    expect(listLocalWorkflowDrafts().map((draft) => draft.id)).toEqual(["one"]);
  });
});
