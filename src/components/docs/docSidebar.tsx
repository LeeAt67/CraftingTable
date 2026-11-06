import { FilePlus2, FolderPlus, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  DocNodeDialog,
  type DocDialogState,
} from "@/components/docs/docNodeDialog";
import {
  DocTree,
  buildTree,
  type DocNodeAction,
} from "@/components/docs/docTree";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useDocsEffect } from "@/controllers/effects/useDocsEffect";
import { useDocsStore } from "@/controllers/stores/docsStore";
import type { DocNodeSummary } from "@/models/docContracts";

export { buildTree };

export function DocSidebar() {
  const navigate = useNavigate();
  const { nodeId } = useParams();
  const effect = useDocsEffect();
  const nodes = useDocsStore((state) => state.nodes);
  const treePhase = useDocsStore((state) => state.phase);
  const treeError = useDocsStore((state) => state.errorMessage);
  const searchQuery = useDocsStore((state) => state.searchQuery);
  const searchResults = useDocsStore((state) => state.searchResults);
  const searchPhase = useDocsStore((state) => state.searchPhase);
  const [dialog, setDialog] = useState<DocDialogState>({ type: "none" });

  useEffect(() => {
    // 已有数据时不重复请求；加载失败后允许在重新挂载时自动重试。
    const { phase } = useDocsStore.getState();
    if (phase === "idle" || phase === "error") {
      void effect.loadTree();
    }
  }, [effect]);

  const create = async (kind: "folder" | "document") => {
    const node = await effect.createNode(kind, null);
    if (node) {
      void effect.loadTree();
      navigate(`/docs/${node.id}`);
    }
  };

  const filtering = searchQuery.trim().length > 0;

  const handleAction = (action: DocNodeAction, node: DocNodeSummary) => {
    setDialog({ type: action, node });
  };

  return (
    <aside
      aria-label="文档目录"
      className="flex h-full w-[88px] shrink-0 flex-col border-r border-border bg-surfaceSubtle px-2 py-4 min-[461px]:w-[104px] min-[768px]:w-[220px] min-[768px]:px-3 min-[1280px]:w-[260px]"
    >
      <div className="relative hidden min-[768px]:block">
        <Search
          aria-hidden="true"
          size={14}
          className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-textSecondary"
        />
        <Input
          aria-label="搜索文档"
          value={searchQuery}
          onChange={(event) => effect.search(event.target.value)}
          placeholder="搜索文档"
          className="h-8 pl-8 text-sm"
        />
      </div>

      <div className="mt-3 flex items-center gap-1 min-[768px]:justify-start">
        <Button
          type="button"
          variant="ghost"
          aria-label="新建文档"
          title="新建文档"
          onClick={() => void create("document")}
          className="h-8 flex-1 justify-center gap-1.5 px-1 text-xs text-textPrimary hover:bg-surface min-[768px]:justify-start min-[768px]:px-2"
        >
          <FilePlus2 aria-hidden="true" size={14} />
          <span className="hidden min-[768px]:inline">新建文档</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          aria-label="新建文件夹"
          title="新建文件夹"
          onClick={() => void create("folder")}
          className="h-8 flex-1 justify-center gap-1.5 px-1 text-xs text-textPrimary hover:bg-surface min-[768px]:justify-start min-[768px]:px-2"
        >
          <FolderPlus aria-hidden="true" size={14} />
          <span className="hidden min-[768px]:inline">新建文件夹</span>
        </Button>
      </div>

      <nav aria-label="文档树" className="mt-4 min-h-0 flex-1 overflow-y-auto">
        {filtering ? (
          <ul className="space-y-0.5" aria-label="搜索结果">
            {searchPhase === "error" && (
              <li
                role="alert"
                className="flex items-center justify-between gap-2 rounded-md px-2 py-1.5"
              >
                <span className="text-xs text-destructive">搜索失败</span>
                <button
                  type="button"
                  onClick={() => effect.search(searchQuery)}
                  className="rounded px-1 text-xs text-brand underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                >
                  重试
                </button>
              </li>
            )}
            {searchResults.map((hit) => (
              <li key={hit.id}>
                <button
                  type="button"
                  onClick={() => navigate(`/docs/${hit.id}`)}
                  className="flex w-full flex-col rounded-md px-2 py-1.5 text-left hover:bg-surface"
                >
                  <span className="truncate text-sm text-textPrimary">
                    {hit.title}
                  </span>
                  {hit.path.length > 0 && (
                    <span className="truncate text-[11px] text-textSecondary">
                      {hit.path.join(" / ")}
                    </span>
                  )}
                </button>
              </li>
            ))}
            {searchResults.length === 0 &&
              searchPhase !== "error" &&
              searchPhase !== "loading" && (
                <li className="px-2 py-2 text-xs text-textSecondary">
                  没有匹配的文档
                </li>
              )}
            {searchResults.length === 0 && searchPhase === "loading" && (
              <li className="px-2 py-2 text-xs text-textSecondary">搜索中…</li>
            )}
          </ul>
        ) : treePhase === "error" ? (
          <div
            role="alert"
            className="rounded-md border border-border bg-surface px-2 py-2"
          >
            <p className="text-xs text-destructive">加载文档目录失败</p>
            {treeError && (
              <p className="mt-1 text-[11px] text-textSecondary">{treeError}</p>
            )}
            <button
              type="button"
              onClick={() => void effect.loadTree()}
              className="mt-2 w-full rounded border border-border px-2 py-1 text-xs text-textPrimary hover:bg-surfaceSubtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            >
              重试
            </button>
          </div>
        ) : treePhase === "loading" ? (
          <ul aria-label="目录加载中" className="space-y-1 px-1">
            {[0, 1, 2].map((index) => (
              <li
                key={index}
                className="h-6 animate-pulse rounded bg-surface"
                aria-hidden="true"
              />
            ))}
            <li className="sr-only">目录加载中…</li>
          </ul>
        ) : (
          <DocTree
            nodes={nodes}
            activeNodeId={nodeId ?? null}
            onSelect={(node) => navigate(`/docs/${node.id}`)}
            onAction={handleAction}
          />
        )}
      </nav>

      <DocNodeDialog
        state={dialog}
        nodes={nodes}
        onClose={() => setDialog({ type: "none" })}
        onRename={effect.renameNode}
        onMove={effect.moveNode}
        onDelete={effect.deleteNode}
      />
    </aside>
  );
}
