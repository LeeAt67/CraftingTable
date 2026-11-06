import { beforeEach, describe, expect, it } from "vitest";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";

const folder = {
  id: "folder-1",
  parentId: null,
  kind: "folder" as const,
  title: "工作",
  sortOrder: 0,
  createdAt: "2026-11-06T00:00:00Z",
  updatedAt: "2026-11-06T00:00:00Z",
};

const document = {
  ...folder,
  id: "doc-1",
  kind: "document" as const,
  title: "接口设计",
};

beforeEach(() => {
  useDocsStore.setState(initialDocsState, true);
});

describe("docsStore", () => {
  it("stores the tree and replaces single summaries", () => {
    useDocsStore.getState().setTree([folder, document]);
    useDocsStore.getState().replaceSummary({ ...document, title: "改名" });

    const state = useDocsStore.getState();
    expect(state.nodes).toHaveLength(2);
    expect(state.nodes[1].title).toBe("改名");
  });

  it("removes a subtree by identifiers", () => {
    useDocsStore.getState().setTree([folder, document]);
    useDocsStore.getState().removeNodes(["folder-1", "doc-1"]);

    expect(useDocsStore.getState().nodes).toHaveLength(0);
  });

  it("keeps the active node when the draft is saved", () => {
    useDocsStore.getState().setActiveNode({
      ...document,
      content: "# 旧内容",
    });
    useDocsStore.getState().setDraftContent("# 新内容");
    useDocsStore.getState().setSaveStatus("saving");
    useDocsStore.getState().markSaved({
      id: "doc-1",
      content: "# 新内容",
      updatedAt: "2026-11-06T00:00:05Z",
    });

    const state = useDocsStore.getState();
    expect(state.saveStatus).toBe("saved");
    expect(state.draftContent).toBe("# 新内容");
    expect(state.activeNode?.content).toBe("# 新内容");
    expect(state.activeNode?.updatedAt).toBe("2026-11-06T00:00:05Z");
  });

  it("clears search state when the query is emptied", () => {
    useDocsStore.getState().setSearchQuery("接口");
    useDocsStore.getState().setSearchResults(
      [
        {
          id: "doc-1",
          kind: "document",
          title: "接口设计",
          parentId: null,
          path: [],
          updatedAt: "2026-11-06T00:00:00Z",
        },
      ],
      "cursor-1",
    );
    useDocsStore.getState().clearSearch();

    const state = useDocsStore.getState();
    expect(state.searchQuery).toBe("");
    expect(state.searchResults).toHaveLength(0);
    expect(state.searchCursor).toBeNull();
  });

  it("does not mark the active document saved when another document finishes saving", () => {
    useDocsStore.getState().setActiveNode({
      ...document,
      id: "doc-active",
      content: "",
    });

    useDocsStore.getState().markSaved({
      id: "doc-other",
      content: "# 别的文档",
      updatedAt: "2026-11-06T00:00:05Z",
    });

    const state = useDocsStore.getState();
    expect(state.saveStatus).toBe("idle");
    expect(state.activeNode?.id).toBe("doc-active");
    expect(state.activeNode?.content).toBe("");
  });
});
