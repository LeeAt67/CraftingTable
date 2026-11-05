import { Download, Upload } from "lucide-react";
import { useEffect, useRef } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { DocEditor } from "@/components/docs/docEditor";
import {
  documentTitleFromFilename,
  downloadMarkdown,
  importMarkdownError,
  readMarkdownFile,
} from "@/components/docs/docFileTransfer";
import { DocTitleInput } from "@/components/docs/docTitleInput";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocsEffect } from "@/controllers/effects/useDocsEffect";
import { useDocsStore } from "@/controllers/stores/docsStore";

export default function DocPage() {
  const { nodeId } = useParams();
  const navigate = useNavigate();
  const effect = useDocsEffect();
  const fileInput = useRef<HTMLInputElement>(null);

  const activeNode = useDocsStore((state) => state.activeNode);
  const draftTitle = useDocsStore((state) => state.draftTitle);
  const saveStatus = useDocsStore((state) => state.saveStatus);
  const phase = useDocsStore((state) => state.phase);
  const errorMessage = useDocsStore((state) => state.errorMessage);

  useEffect(() => {
    if (nodeId) {
      void effect.openNode(nodeId);
    }
  }, [effect, nodeId]);

  useEffect(() => {
    const flush = () => {
      void effect.flushSave();
    };
    window.addEventListener("beforeunload", flush);
    return () => {
      window.removeEventListener("beforeunload", flush);
      flush();
    };
  }, [effect]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        void effect.flushSave();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [effect]);

  const importFile = async (file: File) => {
    let content: string;
    try {
      content = await readMarkdownFile(file);
    } catch {
      toast.error("文件读取失败，请重试");
      return;
    }
    const invalid = importMarkdownError(content);
    if (invalid) {
      toast.error(invalid);
      return;
    }

    const node = await effect.createNode(
      "document",
      activeNode?.parentId ?? null,
    );
    if (!node) {
      return;
    }

    const title = documentTitleFromFilename(file.name);
    const renamed = await effect.renameNode(node.id, title, { silent: true });
    effect.applyContentChange(node.id, content);
    await effect.flushSave();
    if (useDocsStore.getState().saveStatus === "error") {
      return;
    }

    await effect.openNode(node.id);
    navigate(`/docs/${node.id}`);
    if (useDocsStore.getState().activeNodeId === node.id) {
      toast.success(`已导入「${renamed?.title ?? title}」`);
    }
  };

  const exportMarkdown = async () => {
    await effect.flushSave();
    if (useDocsStore.getState().saveStatus === "error") {
      toast.error("文档尚未保存成功，暂时无法导出");
      return;
    }
    const state = useDocsStore.getState();
    downloadMarkdown(state.draftTitle, state.draftContent);
  };

  if (phase === "loading" && !activeNode) {
    return (
      <div className="mx-auto w-full max-w-3xl px-6 py-10">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="mt-6 h-4 w-full" />
        <Skeleton className="mt-3 h-4 w-5/6" />
      </div>
    );
  }

  if (errorMessage && !activeNode) {
    return (
      <div className="mx-auto w-full max-w-3xl px-6 py-10">
        <p className="text-sm text-textPrimary">{errorMessage}</p>
        <Button
          className="mt-4"
          variant="outline"
          onClick={() => nodeId && void effect.openNode(nodeId)}
        >
          重试
        </Button>
      </div>
    );
  }

  if (!activeNode) {
    return null;
  }

  return (
    <section className="flex h-full min-w-0 flex-col bg-surface">
      <header className="flex flex-wrap items-center gap-2 border-b border-border px-6 py-4">
        <div className="min-w-0 flex-1">
          <DocTitleInput
            value={draftTitle}
            status={saveStatus}
            onChange={(title) => effect.applyTitleChange(activeNode.id, title)}
            onRetry={() => void effect.flushSave()}
          />
        </div>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => fileInput.current?.click()}
          >
            <Upload aria-hidden="true" size={14} />
            导入
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => void exportMarkdown()}
          >
            <Download aria-hidden="true" size={14} />
            导出
          </Button>
        </div>
        <input
          ref={fileInput}
          type="file"
          accept=".md,.markdown,.txt,text/markdown"
          aria-label="导入 Markdown"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file) {
              void importFile(file);
            }
          }}
        />
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-6">
        <div className="mx-auto w-full max-w-3xl">
          <DocEditor
            documentId={activeNode.id}
            initialMarkdown={activeNode.content}
            onChange={(markdown) =>
              effect.applyContentChange(activeNode.id, markdown)
            }
          />
        </div>
      </div>
    </section>
  );
}
