import { afterEach, describe, expect, it, vi } from "vitest";
import { apiDocsModel } from "@/models/apiDocsModel";

function mockFetch(body: unknown, status = 200) {
  const spy = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("apiDocsModel", () => {
  it("loads the tree without content", async () => {
    const spy = mockFetch({
      data: [
        {
          id: "node-1",
          parentId: null,
          kind: "folder",
          title: "工作",
          sortOrder: 0,
          createdAt: "2026-11-06T00:00:00Z",
          updatedAt: "2026-11-06T00:00:00Z",
        },
      ],
    });

    const nodes = await apiDocsModel.listTree();

    expect(spy).toHaveBeenCalledWith("/api/v1/docs/tree", expect.any(Object));
    expect(nodes[0].title).toBe("工作");
  });

  it("encodes the search query", async () => {
    const spy = mockFetch({ data: { items: [], nextCursor: null } });

    await apiDocsModel.search({ q: "接口 设计", limit: 20 });

    expect(spy.mock.calls[0][0]).toBe(
      "/api/v1/docs/search?q=%E6%8E%A5%E5%8F%A3%20%E8%AE%BE%E8%AE%A1&limit=20",
    );
  });

  it("posts the save payload", async () => {
    const spy = mockFetch({
      data: { id: "node-1", updatedAt: "2026-11-06T00:00:01Z" },
    });

    const result = await apiDocsModel.saveContent({
      id: "node-1",
      content: "# 标题",
    });

    expect(result.updatedAt).toBe("2026-11-06T00:00:01Z");
    expect(JSON.parse(spy.mock.calls[0][1].body as string)).toEqual({
      id: "node-1",
      content: "# 标题",
    });
  });

  it("surfaces the server error message", async () => {
    mockFetch(
      {
        error: {
          code: "VALIDATION_ERROR",
          kind: "validation",
          message: "不能移动到自身或子节点",
          retryable: false,
        },
      },
      400,
    );

    await expect(
      apiDocsModel.moveNode({ id: "a", parentId: "a" }),
    ).rejects.toThrow("不能移动到自身或子节点");
  });
});
