import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DocSidebar, buildTree } from "@/components/docs/docSidebar";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";
import { docsModel } from "@/models/docsModel";

vi.mock("@/models/docsModel", () => ({
  docsModel: {
    listTree: vi.fn(),
    getNode: vi.fn(),
    createNode: vi.fn(),
    renameNode: vi.fn(),
    moveNode: vi.fn(),
    saveContent: vi.fn(),
    deleteNode: vi.fn(),
    search: vi.fn(),
  },
}));

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
  parentId: "folder-1",
};

const searchHit = {
  id: "doc-1",
  kind: "document" as const,
  title: "接口设计",
  parentId: "folder-1",
  path: ["工作"],
  updatedAt: "2026-11-06T00:00:00Z",
};

beforeEach(() => {
  useDocsStore.setState(initialDocsState, true);
  vi.mocked(docsModel.listTree).mockResolvedValue([folder, document]);
  vi.mocked(docsModel.getNode).mockResolvedValue({ ...document, content: "" });
  vi.mocked(docsModel.createNode).mockResolvedValue({
    ...document,
    content: "",
  });
  vi.mocked(docsModel.renameNode).mockResolvedValue(document);
  vi.mocked(docsModel.moveNode).mockResolvedValue(document);
  vi.mocked(docsModel.saveContent).mockResolvedValue({
    id: document.id,
    updatedAt: "2026-11-06T00:00:09Z",
  });
  vi.mocked(docsModel.deleteNode).mockResolvedValue({
    deletedIds: [document.id],
  });
  vi.mocked(docsModel.search).mockResolvedValue({
    items: [searchHit],
    nextCursor: null,
  });
});

describe("buildTree", () => {
  it("groups children under their folder", () => {
    const tree = buildTree([folder, document]);

    expect(tree.roots).toHaveLength(1);
    expect(tree.roots[0].id).toBe("folder-1");
    expect(tree.childrenOf["folder-1"].map((node) => node.id)).toEqual([
      "doc-1",
    ]);
  });
});

describe("DocSidebar", () => {
  it("renders folders and their documents", async () => {
    useDocsStore.getState().setTree([folder, document]);

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("complementary", { name: "文档目录" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText("工作")).toBeInTheDocument();
    });
    expect(screen.getByText("接口设计")).toBeInTheDocument();
  });

  it("renders server search results with their path and hides the tree", async () => {
    useDocsStore.getState().setTree([folder, document]);
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText("搜索文档"), "接口");

    await waitFor(
      () => {
        expect(screen.getByText("接口设计")).toBeInTheDocument();
      },
      { timeout: 1500 },
    );

    expect(docsModel.search).toHaveBeenCalledWith({ q: "接口", limit: 20 });
    expect(screen.getByRole("list", { name: "搜索结果" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "操作 接口设计" }),
    ).not.toBeInTheDocument();
    // 文件夹行已经隐藏，仍能看到“工作”说明渲染的是服务端返回的路径。
    expect(screen.getByText("工作")).toBeInTheDocument();
  });

  it("shows no match when the server returns no hits even if a title matches", async () => {
    useDocsStore.getState().setTree([folder, document]);
    vi.mocked(docsModel.search).mockResolvedValue({
      items: [],
      nextCursor: null,
    });
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText("搜索文档"), "接口");

    await waitFor(
      () => {
        expect(docsModel.search).toHaveBeenCalledWith({ q: "接口", limit: 20 });
        expect(screen.getByText("没有匹配的文档")).toBeInTheDocument();
      },
      { timeout: 1500 },
    );

    expect(screen.getByRole("list", { name: "搜索结果" })).toBeInTheDocument();
    expect(screen.queryByText("接口设计")).not.toBeInTheDocument();
  });

  it("shows an inline error with retry when the search request fails", async () => {
    useDocsStore.getState().setTree([folder, document]);
    vi.mocked(docsModel.search)
      .mockRejectedValueOnce(new Error("网络异常"))
      .mockResolvedValueOnce({ items: [searchHit], nextCursor: null });
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText("搜索文档"), "接口");

    await waitFor(
      () => {
        expect(screen.getByRole("alert")).toHaveTextContent("搜索失败");
      },
      { timeout: 1500 },
    );

    const callsBeforeRetry = vi.mocked(docsModel.search).mock.calls.length;
    await user.click(screen.getByRole("button", { name: "重试" }));

    await waitFor(
      () => {
        expect(screen.getByText("接口设计")).toBeInTheDocument();
      },
      { timeout: 1500 },
    );

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(vi.mocked(docsModel.search).mock.calls.length).toBe(
      callsBeforeRetry + 1,
    );
    expect(docsModel.search).toHaveBeenLastCalledWith({ q: "接口", limit: 20 });
  });

  it("offers rename, move and delete from the row action menu", async () => {
    useDocsStore.getState().setTree([folder, document]);
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole("button", { name: "操作 接口设计" }));

    expect(screen.getByRole("button", { name: "重命名" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "移动到" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "删除" })).toBeInTheDocument();
  });

  it("asks for confirmation before deleting and warns about folder contents", async () => {
    useDocsStore.getState().setTree([folder, document]);
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole("button", { name: "操作 工作" }));
    await user.click(screen.getByRole("button", { name: "删除" }));

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("删除确认")).toBeInTheDocument();
    expect(
      screen.getByText(/文件夹内的内容会一并删除/),
    ).toBeInTheDocument();
  });

  it("shows a Chinese error with retry when the tree fails to load", async () => {
    vi.mocked(docsModel.listTree).mockRejectedValueOnce(new Error("Failed to fetch"));
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    expect(await screen.findByText("加载文档目录失败")).toBeInTheDocument();
    expect(
      screen.getByText("网络异常，请检查服务是否可用后重试"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Failed to fetch")).not.toBeInTheDocument();
    expect(screen.queryByText("接口设计")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "重试" }));

    await waitFor(
      () => {
        expect(screen.getByText("接口设计")).toBeInTheDocument();
      },
      { timeout: 1500 },
    );
    expect(screen.queryByText("加载文档目录失败")).not.toBeInTheDocument();
  });

  it("retries the tree automatically after remounting from an error", async () => {
    vi.mocked(docsModel.listTree).mockClear();
    useDocsStore.setState({ phase: "error", errorMessage: "网络异常" });

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await waitFor(
      () => {
        expect(screen.getByText("工作")).toBeInTheDocument();
      },
      { timeout: 1500 },
    );
    expect(vi.mocked(docsModel.listTree)).toHaveBeenCalledTimes(1);
  });
});
