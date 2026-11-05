import { FilePlus2, FolderPlus, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
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

type SearchHitView = {
  id: string;
  title: string;
  path: string[];
};

export function DocSidebar() {
  const navigate = useNavigate();
  const { nodeId } = useParams();
  const effect = useDocsEffect();
  const nodes = useDocsStore((state) => state.nodes);
  const searchQuery = useDocsStore((state) => state.searchQuery);
  const searchResults = useDocsStore((state) => state.searchResults);
  const [dialog, setDialog] = useState<DocDialogState>({ type: "none" });

  useEffect(() => {
    // 目录树只拉取一次：store 已有数据时不再覆盖，避免重复请求。
    if (useDocsStore.getState().phase === "idle") {
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

  // 服务端检索是异步的，先在本地树上过滤让输入立即可见，
  // 服务端结果返回后按 id 去重合并。
  const hits = useMemo<SearchHitView[]>(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) {
      return [];
    }
    const byId = new Map(nodes.map((node) => [node.id, node]));
    const pathOf = (node: DocNodeSummary) => {
      const path: string[] = [];
      const visited = new Set<string>([node.id]);
      let parent = node.parentId ? byId.get(node.parentId) : undefined;
      while (parent && !visited.has(parent.id)) {
        visited.add(parent.id);
        path.unshift(parent.title);
        parent = parent.parentId ? byId.get(parent.parentId) : undefined;
      }
      return path;
    };
    const local = nodes
      .filter((node) => node.title.toLowerCase().includes(query))
      .map((node) => ({ id: node.id, title: node.title, path: pathOf(node) }));
    const seen = new Set(local.map((hit) => hit.id));
    const remote = searchResults
      .filter((hit) => !seen.has(hit.id))
      .map((hit) => ({ id: hit.id, title: hit.title, path: hit.path }));
    return [...local, ...remote];
  }, [nodes, searchQuery, searchResults]);

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
            {hits.map((hit) => (
              <li key={hit.id}>
                <button
                  type="button"
                  onClick={() => navigate(`/docs/${hit.id}`)}
                  title={hit.path.length > 0 ? hit.path.join(" / ") : undefined}
                  className="flex w-full flex-col rounded-md px-2 py-1.5 text-left hover:bg-surface"
                >
                  <span className="truncate text-sm text-textPrimary">
                    {hit.title}
                  </span>
                </button>
              </li>
            ))}
            {hits.length === 0 && (
              <li className="px-2 py-2 text-xs text-textSecondary">
                没有匹配的文档
              </li>
            )}
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
