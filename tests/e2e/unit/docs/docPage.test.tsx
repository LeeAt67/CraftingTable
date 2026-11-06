import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DocNode, DocsModel } from "@/models/docContracts";
import { initialDocsState, useDocsStore } from "@/controllers/stores/docsStore";

const folder: DocNode = {
  id: "folder-1",
  parentId: null,
  kind: "folder",
  title: "工作",
  sortOrder: 0,
  createdAt: "2026-11-06T00:00:00Z",
  updatedAt: "2026-11-06T00:00:00Z",
  content: "",
};

const document: DocNode = {
  ...folder,
  id: "doc-1",
  kind: "document",
  title: "接口设计",
  content: "# 接口设计",
};

const model: DocsModel = {
  listTree: vi.fn().mockResolvedValue([folder, document]),
  getNode: vi.fn().mockImplementation(async (id: string) =>
    id === folder.id ? folder : document,
  ),
  createNode: vi.fn().mockResolvedValue(document),
  renameNode: vi.fn().mockResolvedValue(document),
  moveNode: vi.fn().mockResolvedValue(document),
  saveContent: vi.fn().mockResolvedValue({
    id: document.id,
    updatedAt: "2026-11-06T00:00:09Z",
  }),
  deleteNode: vi.fn().mockResolvedValue({ deletedIds: [document.id] }),
  search: vi.fn().mockResolvedValue({ items: [], nextCursor: null }),
};

vi.mock("@/models/docsModel", () => ({ docsModel: model }));

const { default: DocPage } = await import("@/pages/docs/docPage");

function renderPage(nodeId: string) {
  return render(
    <MemoryRouter initialEntries={[`/docs/${nodeId}`]}>
      <Routes>
        <Route path="/docs/:nodeId" element={<DocPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useDocsStore.setState(initialDocsState, true);
  vi.clearAllMocks();
});

describe("DocPage", () => {
  it("renders the editor for a document", async () => {
    renderPage(document.id);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "接口设计" })).toBeInTheDocument();
    });
    expect(screen.getByLabelText("文档正文")).toBeInTheDocument();
  });

  it("renders a folder placeholder instead of the editor", async () => {
    renderPage(folder.id);

    await waitFor(() => {
      expect(screen.getByText("这是一个文件夹")).toBeInTheDocument();
    });
    expect(screen.queryByLabelText("文档正文")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "导出" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("文档标题")).toHaveValue("工作");
  });
});
