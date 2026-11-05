import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DocEditor } from "@/components/docs/docEditor";

describe("DocEditor", () => {
  it("renders markdown as editable content", async () => {
    render(
      <DocEditor
        documentId="doc-1"
        initialMarkdown={"# 标题\n\n- 一\n- 二"}
        onChange={() => undefined}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "标题" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("一")).toBeInTheDocument();
  });

  it("emits markdown when the document changes", async () => {
    const onChange = vi.fn();
    render(
      <DocEditor
        documentId="doc-1"
        initialMarkdown="# 标题"
        onChange={onChange}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "标题" })).toBeInTheDocument();
    });

    const editable = screen.getByLabelText("文档正文");
    editable.textContent = "改写后的内容";
    editable.dispatchEvent(new Event("input", { bubbles: true }));

    await waitFor(() => {
      expect(onChange).toHaveBeenCalled();
    });
    expect(onChange.mock.calls.at(-1)?.[0]).toContain("改写后的内容");
  });
});
