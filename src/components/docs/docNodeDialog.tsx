import { useEffect, useMemo, useState } from "react";
import { collectSubtreeIds } from "@/components/docs/docTree";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import type { DocNodeSummary } from "@/models/docContracts";

export type DocDialogState =
  | { type: "none" }
  | { type: "rename"; node: DocNodeSummary }
  | { type: "move"; node: DocNodeSummary }
  | { type: "delete"; node: DocNodeSummary };

type Props = {
  state: DocDialogState;
  nodes: DocNodeSummary[];
  onClose: () => void;
  onRename: (id: string, title: string) => Promise<unknown>;
  onMove: (id: string, parentId: string | null) => Promise<unknown>;
  onDelete: (id: string) => Promise<unknown>;
};

const TITLES: Record<Exclude<DocDialogState["type"], "none">, string> = {
  rename: "重命名",
  move: "移动到",
  delete: "删除确认",
};

export function DocNodeDialog({
  state,
  nodes,
  onClose,
  onRename,
  onMove,
  onDelete,
}: Props) {
  const [title, setTitle] = useState("");
  const [parentId, setParentId] = useState<string>("");
  const [pending, setPending] = useState(false);
  const open = state.type !== "none";
  const node = state.type === "none" ? null : state.node;

  useEffect(() => {
    if (!node) {
      setTitle("");
      setParentId("");
      return;
    }
    setTitle(state.type === "rename" ? node.title : "");
    setParentId(state.type === "move" ? (node.parentId ?? "") : "");
  }, [node, state.type]);

  const folders = useMemo(() => {
    if (!node) {
      return [];
    }
    const blocked = collectSubtreeIds(nodes, node.id);
    return nodes.filter(
      (candidate) => candidate.kind === "folder" && !blocked.has(candidate.id),
    );
  }, [node, nodes]);

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setTitle("");
      setParentId("");
      onClose();
    }
  };

  const submit = async () => {
    if (state.type === "none") {
      return;
    }
    setPending(true);
    try {
      if (state.type === "rename") {
        await onRename(state.node.id, title.trim() || state.node.title);
      }
      if (state.type === "move") {
        await onMove(state.node.id, parentId || null);
      }
      if (state.type === "delete") {
        await onDelete(state.node.id);
      }
      setTitle("");
      setParentId("");
      onClose();
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{open ? TITLES[state.type] : null}</DialogTitle>
          <DialogDescription>
            {state.type === "rename" && "修改节点标题。"}
            {state.type === "move" && "选择目标文件夹，留空表示移到根目录。"}
            {state.type === "delete" &&
              `确定删除「${state.node.title}」吗？删除后不可在界面恢复，文件夹内的内容会一并删除。`}
          </DialogDescription>
        </DialogHeader>

        {state.type === "rename" && (
          <Input
            aria-label="节点标题"
            autoFocus
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            className="mt-4"
          />
        )}

        {state.type === "move" && (
          <select
            aria-label="目标文件夹"
            value={parentId}
            onChange={(event) => setParentId(event.target.value)}
            className="mt-4 h-9 w-full rounded-md border border-border bg-surface px-3 text-sm text-textPrimary"
          >
            <option value="">根目录</option>
            {folders.map((folder) => (
              <option key={folder.id} value={folder.id}>
                {folder.title}
              </option>
            ))}
          </select>
        )}

        <DialogFooter className="mt-4">
          <Button
            type="button"
            variant="ghost"
            onClick={() => handleOpenChange(false)}
          >
            取消
          </Button>
          <Button
            type="button"
            variant={state.type === "delete" ? "destructive" : "default"}
            disabled={pending}
            onClick={() => void submit()}
          >
            {pending ? "处理中" : state.type === "delete" ? "删除" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
