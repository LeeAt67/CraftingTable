import { MAX_CONTENT_RUNES } from "@/models/docContracts";

const DEFAULT_TITLE = "无标题文档";

export function documentTitleFromFilename(name: string): string {
  const trimmed = name.trim();
  const withoutExtension = trimmed.replace(/\.(md|markdown|txt)$/i, "");
  const title = withoutExtension.trim();
  return title || DEFAULT_TITLE;
}

export function sanitizeFileName(value: string): string {
  return value.replace(/[\\/:*?"<>|]/g, "-").trim() || DEFAULT_TITLE;
}

export function importMarkdownError(content: string): string | null {
  if (content.trim().length === 0) {
    return "无法导入空文件";
  }
  if (content.includes("\u0000")) {
    return "仅支持导入文本文件";
  }
  if ([...content].length > MAX_CONTENT_RUNES) {
    return `文档超过 ${MAX_CONTENT_RUNES} 个字符，无法导入`;
  }
  return null;
}

export function downloadMarkdown(title: string, content: string): void {
  const blob = new Blob([content], { type: "text/markdown;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${sanitizeFileName(title)}.md`;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export function readMarkdownFile(file: File): Promise<string> {
  return file.text();
}
