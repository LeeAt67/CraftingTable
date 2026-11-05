import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { DocSidebar, buildTree } from "@/components/docs/docSidebar";
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
  parentId: "folder-1",
};

beforeEach(() => {
  useDocsStore.setState(initialDocsState, true);
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

  it("filters the tree by the search query", async () => {
    useDocsStore.getState().setTree([folder, document]);
    const user = userEvent.setup();

    render(
      <MemoryRouter>
        <DocSidebar />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText("搜索文档"), "接口");

    await waitFor(() => {
      expect(screen.getByText("接口设计")).toBeInTheDocument();
      expect(screen.queryByText("工作")).not.toBeInTheDocument();
    });
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
});
