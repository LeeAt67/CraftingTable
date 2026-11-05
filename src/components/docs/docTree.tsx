import { ChevronRight, FileText, Folder, FolderOpen } from "lucide-react";
import { useEffect, useState } from "react";
import { cn } from "@/components/utils";
import type { DocNodeSummary } from "@/models/docContracts";

export type DocTreeIndex = {
  roots: DocNodeSummary[];
  childrenOf: Record<string, DocNodeSummary[]>;
};

export type DocNodeAction = "rename" | "move" | "delete";

const sortNodes = (items: DocNodeSummary[]) =>
  [...items].sort((left, right) => {
    if (left.kind !== right.kind) {
      return left.kind === "folder" ? -1 : 1;
    }
    if (left.sortOrder !== right.sortOrder) {
      return left.sortOrder - right.sortOrder;
    }
    return right.updatedAt.localeCompare(left.updatedAt);
  });

export const buildTree = (nodes: DocNodeSummary[]): DocTreeIndex => {
  const known = new Set(nodes.map((node) => node.id));
  const childrenOf: Record<string, DocNodeSummary[]> = {};
  const roots: DocNodeSummary[] = [];

  for (const node of nodes) {
    const parentId =
      node.parentId && known.has(node.parentId) ? node.parentId : null;
    if (!parentId) {
      roots.push(node);
      continue;
    }
    childrenOf[parentId] = [...(childrenOf[parentId] ?? []), node];
  }

  return {
    roots: sortNodes(roots),
    childrenOf: Object.fromEntries(
      Object.entries(childrenOf).map(([key, value]) => [key, sortNodes(value)]),
    ),
  };
};

/** 节点自身及其全部后代 id，用于避免把文件夹移动到自己的子树里。 */
export const collectSubtreeIds = (nodes: DocNodeSummary[], id: string) => {
  const ids = new Set<string>([id]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const node of nodes) {
      if (node.parentId && ids.has(node.parentId) && !ids.has(node.id)) {
        ids.add(node.id);
        grew = true;
      }
    }
  }
  return ids;
};

type Props = {
  nodes: DocNodeSummary[];
  activeNodeId: string | null;
  onSelect: (node: DocNodeSummary) => void;
  onAction: (action: DocNodeAction, node: DocNodeSummary) => void;
};

export function DocTree({ nodes, activeNodeId, onSelect, onAction }: Props) {
  const { roots, childrenOf } = buildTree(nodes);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [menuNodeId, setMenuNodeId] = useState<string | null>(null);

  useEffect(() => {
    if (!menuNodeId) {
      return;
    }
    const closeOnOutside = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.closest("[data-doc-menu]")) {
        return;
      }
      setMenuNodeId(null);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMenuNodeId(null);
      }
    };
    document.addEventListener("mousedown", closeOnOutside);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("mousedown", closeOnOutside);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [menuNodeId]);

  const toggle = (id: string) =>
    setCollapsed((current) => ({ ...current, [id]: !current[id] }));

  const choose = (action: DocNodeAction, node: DocNodeSummary) => {
    setMenuNodeId(null);
    onAction(action, node);
  };

  const renderRow = (node: DocNodeSummary, depth: number) => {
    const isFolder = node.kind === "folder";
    const isCollapsed = Boolean(collapsed[node.id]);
    const children = childrenOf[node.id] ?? [];
    const isActive = node.id === activeNodeId;
    const menuOpen = menuNodeId === node.id;

    return (
      <li key={node.id}>
        <div
          className={cn(
            "relative flex items-center gap-1 rounded-md pr-1",
            isActive ? "bg-brandSoft text-brand" : "text-textBody hover:bg-surface",
          )}
          style={{ paddingLeft: depth === 0 ? 4 : 18 }}
        >
          {isFolder ? (
            <button
              type="button"
              aria-label={isCollapsed ? `展开 ${node.title}` : `折叠 ${node.title}`}
              onClick={() => toggle(node.id)}
              className="grid size-5 shrink-0 place-items-center text-textSecondary"
            >
              <ChevronRight
                aria-hidden="true"
                size={13}
                className={cn("transition-transform", !isCollapsed && "rotate-90")}
              />
            </button>
          ) : (
            <span className="size-5 shrink-0" />
          )}
          <button
            type="button"
            onClick={() => onSelect(node)}
            className="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-left text-sm"
          >
            {isFolder ? (
              isCollapsed ? (
                <Folder aria-hidden="true" size={14} className="shrink-0 opacity-70" />
              ) : (
                <FolderOpen aria-hidden="true" size={14} className="shrink-0 opacity-70" />
              )
            ) : (
              <FileText aria-hidden="true" size={14} className="shrink-0 opacity-70" />
            )}
            <span className="truncate">{node.title}</span>
          </button>
          <button
            type="button"
            aria-label={`操作 ${node.title}`}
            aria-expanded={menuOpen}
            onClick={() => setMenuNodeId(menuOpen ? null : node.id)}
            className={cn(
              "shrink-0 rounded px-1 text-xs text-textSecondary opacity-60 hover:text-textPrimary hover:opacity-100 focus-visible:opacity-100",
              menuOpen && "opacity-100",
            )}
          >
            ⋯
          </button>
          {menuOpen && (
            <div
              role="group"
              aria-label={`${node.title} 的更多操作`}
              data-doc-menu="true"
              className="absolute right-0 top-full z-20 mt-1 w-32 rounded-md border border-border bg-surface p-1 shadow-lg"
            >
              <button
                type="button"
                onClick={() => choose("rename", node)}
                className="block w-full rounded px-2 py-1.5 text-left text-xs text-textPrimary hover:bg-surfaceSubtle"
              >
                重命名
              </button>
              <button
                type="button"
                onClick={() => choose("move", node)}
                className="block w-full rounded px-2 py-1.5 text-left text-xs text-textPrimary hover:bg-surfaceSubtle"
              >
                移动到
              </button>
              <button
                type="button"
                onClick={() => choose("delete", node)}
                className="block w-full rounded px-2 py-1.5 text-left text-xs text-destructive hover:bg-surfaceSubtle"
              >
                删除
              </button>
            </div>
          )}
        </div>
        {isFolder && !isCollapsed && children.length > 0 && (
          <ul>{children.map((child) => renderRow(child, depth + 1))}</ul>
        )}
      </li>
    );
  };

  return (
    <ul className="space-y-0.5">
      {roots.map((root) => renderRow(root, 0))}
    </ul>
  );
}
