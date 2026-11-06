import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDocsEffect } from "@/controllers/effects/useDocsEffect";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";
import type { DocsModel, DocNode } from "@/models/docContracts";

const folder = {
  id: "folder-1",
  parentId: null,
  kind: "folder" as const,
  title: "工作",
  sortOrder: 0,
  createdAt: "2026-11-06T00:00:00Z",
  updatedAt: "2026-11-06T00:00:00Z",
};

const document: DocNode = {
  ...folder,
  id: "doc-1",
  kind: "document",
  title: "接口设计",
  content: "# 旧内容",
};

function createModel(overrides: Partial<DocsModel> = {}): DocsModel {
  return {
    listTree: vi.fn().mockResolvedValue([folder, document]),
    getNode: vi.fn().mockResolvedValue(document),
    createNode: vi.fn().mockResolvedValue(document),
    renameNode: vi.fn().mockResolvedValue(document),
    moveNode: vi.fn().mockResolvedValue(document),
    saveContent: vi
      .fn()
      .mockResolvedValue({ id: "doc-1", updatedAt: "2026-11-06T00:00:09Z" }),
    deleteNode: vi.fn().mockResolvedValue({ deletedIds: ["doc-1"] }),
    search: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
    ...overrides,
  };
}

const notify = { success: vi.fn(), error: vi.fn() };

beforeEach(() => {
  vi.useFakeTimers();
  useDocsStore.setState(initialDocsState, true);
  notify.success.mockReset();
  notify.error.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("docsEffect", () => {
  it("loads the tree into the store", async () => {
    const effect = createDocsEffect(createModel(), notify);

    await effect.loadTree();

    expect(useDocsStore.getState().nodes).toHaveLength(2);
    expect(useDocsStore.getState().phase).toBe("ready");
  });

  it("debounces content saves and flushes immediately", async () => {
    const model = createModel();
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 新内容");
    await vi.advanceTimersByTimeAsync(399);
    expect(model.saveContent).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(model.saveContent).toHaveBeenCalledWith({
      id: "doc-1",
      content: "# 新内容",
    });
    expect(useDocsStore.getState().saveStatus).toBe("saved");
  });

  it("flushes pending content before switching documents", async () => {
    const model = createModel();
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 待保存");
    await effect.flushSave();

    expect(model.saveContent).toHaveBeenCalledTimes(1);
    expect(useDocsStore.getState().activeNode?.content).toBe("# 待保存");
  });

  it("reports save failures without dropping the draft", async () => {
    const model = createModel({
      saveContent: vi.fn().mockRejectedValue(new Error("网络异常")),
    });
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 保留我");
    await effect.flushSave();

    expect(useDocsStore.getState().saveStatus).toBe("error");
    expect(useDocsStore.getState().draftContent).toBe("# 保留我");
    expect(notify.error).toHaveBeenCalledWith("网络异常");
  });

  it("debounces search and ignores stale responses", async () => {
    const first = {
      items: [
        {
          id: "old",
          kind: "document" as const,
          title: "旧结果",
          parentId: null,
          path: [],
          updatedAt: "2026-11-06T00:00:00Z",
        },
      ],
      nextCursor: null,
    };
    const second = { items: [], nextCursor: null };
    const search = vi
      .fn()
      .mockImplementationOnce(
        () => new Promise((resolve) => setTimeout(() => resolve(first), 50)),
      )
      .mockResolvedValue(second);

    const effect = createDocsEffect(createModel({ search }), notify);

    effect.search("接口");
    await vi.advanceTimersByTimeAsync(300);
    effect.search("文档");
    await vi.advanceTimersByTimeAsync(300);
    await vi.advanceTimersByTimeAsync(100);

    expect(search).toHaveBeenLastCalledWith(
      expect.objectContaining({ q: "文档", limit: 20 }),
    );
    expect(useDocsStore.getState().searchResults).toHaveLength(0);
  });

  it("debounces title saves for 600ms", async () => {
    const model = createModel();
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyTitleChange("doc-1", "新标题");
    await vi.advanceTimersByTimeAsync(599);
    expect(model.renameNode).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(model.renameNode).toHaveBeenCalledWith({
      id: "doc-1",
      title: "新标题",
    });
  });

  it("flushes a pending title when saving immediately", async () => {
    const model = createModel();
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyTitleChange("doc-1", "新标题");
    await effect.flushSave();

    expect(model.renameNode).toHaveBeenCalledWith({
      id: "doc-1",
      title: "新标题",
    });

    await vi.advanceTimersByTimeAsync(600);
    expect(model.renameNode).toHaveBeenCalledTimes(1);
  });

  it("serializes content saves for the same document", async () => {
    const resolvers: Array<() => void> = [];
    const saveContent = vi.fn(
      (_input: { id: string; content: string }) =>
        new Promise<{ id: string; updatedAt: string }>((resolve) => {
          resolvers.push(() =>
            resolve({ id: "doc-1", updatedAt: "2026-11-06T00:00:09Z" }),
          );
        }),
    );
    const model = createModel({ saveContent });
    const effect = createDocsEffect(model, notify);
    useDocsStore.getState().setActiveNode(document);

    effect.applyContentChange("doc-1", "# 第一次");
    void effect.flushSave();
    await Promise.resolve();
    expect(saveContent).toHaveBeenCalledTimes(1);

    effect.applyContentChange("doc-1", "# 第二次");
    await vi.advanceTimersByTimeAsync(400);
    expect(saveContent).toHaveBeenCalledTimes(1);

    resolvers[0]();
    await vi.advanceTimersByTimeAsync(0);
    expect(saveContent).toHaveBeenCalledTimes(2);
    expect(saveContent).toHaveBeenLastCalledWith({
      id: "doc-1",
      content: "# 第二次",
    });
  });

  it("clears search state without a request for blank queries", async () => {
    const search = vi.fn();
    const effect = createDocsEffect(createModel({ search }), notify);

    effect.search("   ");
    await vi.advanceTimersByTimeAsync(400);

    expect(search).not.toHaveBeenCalled();
    expect(useDocsStore.getState().searchQuery).toBe("");
  });

  it("enters the loading phase as soon as a query is typed", () => {
    const effect = createDocsEffect(createModel(), notify);

    effect.search("接口");

    expect(useDocsStore.getState().searchPhase).toBe("loading");
  });

  it("keeps the later document when an older response arrives last", async () => {
    const nodeA = { ...document, id: "doc-a", title: "文档 A" };
    const nodeB = { ...document, id: "doc-b", title: "文档 B" };

    let resolveA: (node: DocNode) => void = () => undefined;
    const slowA = new Promise<DocNode>((resolve) => {
      resolveA = resolve;
    });

    const effect = createDocsEffect(
      createModel({
        getNode: vi.fn((id: string) =>
          id === "doc-a" ? slowA : Promise.resolve(nodeB),
        ),
      }),
      notify,
    );

    const first = effect.openNode("doc-a");
    const second = effect.openNode("doc-b");
    await second;

    expect(useDocsStore.getState().activeNode?.id).toBe("doc-b");

    resolveA(nodeA);
    await first;

    expect(useDocsStore.getState().activeNode?.id).toBe("doc-b");
    expect(useDocsStore.getState().nodePhase).toBe("ready");
  });

  it("ignores a stale failure that arrives after a newer document loaded", async () => {
    const nodeB = { ...document, id: "doc-b", title: "文档 B" };

    let rejectA: (error: Error) => void = () => undefined;
    const failingA = new Promise<DocNode>((_resolve, reject) => {
      rejectA = reject;
    });

    const effect = createDocsEffect(
      createModel({
        getNode: vi.fn((id: string) =>
          id === "doc-a" ? failingA : Promise.resolve(nodeB),
        ),
      }),
      notify,
    );

    const first = effect.openNode("doc-a");
    await effect.openNode("doc-b");

    rejectA(new Error("文档不存在"));
    await first;

    expect(useDocsStore.getState().activeNode?.id).toBe("doc-b");
    expect(useDocsStore.getState().nodePhase).toBe("ready");
    expect(useDocsStore.getState().nodeErrorMessage).toBeNull();
  });

  it("marks the node phase as loading then ready", async () => {
    let resolveNode: (node: DocNode) => void = () => undefined;
    const pendingNode = new Promise<DocNode>((resolve) => {
      resolveNode = resolve;
    });

    const effect = createDocsEffect(
      createModel({ getNode: vi.fn(() => pendingNode) }),
      notify,
    );

    const pending = effect.openNode("doc-1");
    await Promise.resolve();
    await Promise.resolve();
    expect(useDocsStore.getState().nodePhase).toBe("loading");

    resolveNode(document);
    await pending;
    expect(useDocsStore.getState().nodePhase).toBe("ready");
  });
});
