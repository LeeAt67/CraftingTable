import { describe, expect, it } from "vitest";
import {
  documentTitleFromFilename,
  importMarkdownError,
  MAX_IMPORT_RUNES,
  sanitizeFileName,
} from "@/components/docs/docFileTransfer";

describe("docFileTransfer", () => {
  it("derives a title from a markdown filename", () => {
    expect(documentTitleFromFilename("接口设计.md")).toBe("接口设计");
    expect(documentTitleFromFilename("notes.markdown")).toBe("notes");
    expect(documentTitleFromFilename("无扩展名")).toBe("无扩展名");
  });

  it("replaces characters that are illegal in filenames", () => {
    expect(sanitizeFileName('a/b\\c:d*e?f"g<h>i|j')).toBe("a-b-c-d-e-f-g-h-i-j");
  });

  it("falls back to a default title for empty names", () => {
    expect(documentTitleFromFilename(".md")).toBe("无标题文档");
  });

  it("rejects empty and oversized imports before creating a document", () => {
    expect(importMarkdownError("  \n ")).toBe("无法导入空文件");
    expect(importMarkdownError("a".repeat(MAX_IMPORT_RUNES))).toBeNull();
    expect(importMarkdownError("a".repeat(MAX_IMPORT_RUNES + 1))).toBe(
      `文档超过 ${MAX_IMPORT_RUNES} 个字符，无法导入`,
    );
    expect(importMarkdownError("# 标题")).toBeNull();
    expect(importMarkdownError("a\u0000b")).toBe("仅支持导入文本文件");
  });
});
