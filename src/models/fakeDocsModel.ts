import type {
  CreateDocNodeInput,
  DocNode,
  DocNodeSummary,
  DocsModel,
  DocSearchPage,
} from "./docContracts";

function now() {
  return new Date().toISOString();
}

function toSummary(node: DocNode): DocNodeSummary {
  const { content, ...summary } = node;
  void content;
  return summary;
}

export function createFakeDocsModel(): DocsModel {
  const nodes = new Map<string, DocNode>();
  const requests = new Map<string, string>();

  const listTree = async () =>
    [...nodes.values()]
      .map(toSummary)
      .sort((left, right) => {
        if (left.kind !== right.kind) {
          return left.kind === "folder" ? -1 : 1;
        }
        return right.updatedAt.localeCompare(left.updatedAt);
      });

  const getNode = async (id: string) => {
    const node = nodes.get(id);
    if (!node) {
      throw new Error("文档不存在或已被删除");
    }
    return node;
  };

  const createNode = async (input: CreateDocNodeInput) => {
    const existing = input.clientRequestId
      ? requests.get(input.clientRequestId)
      : undefined;
    if (existing) {
      return getNode(existing);
    }
    const id = `fake-${nodes.size + 1}`;
    const timestamp = now();
    const node: DocNode = {
      id,
      parentId: input.parentId ?? null,
      kind: input.kind,
      title:
        input.title?.trim() ||
        (input.kind === "folder" ? "未命名文件夹" : "无标题文档"),
      sortOrder: 0,
      createdAt: timestamp,
      updatedAt: timestamp,
      content: "",
    };
    nodes.set(id, node);
    if (input.clientRequestId) {
      requests.set(input.clientRequestId, id);
    }
    return node;
  };

  const renameNode = async ({ id, title }: { id: string; title: string }) => {
    const node = await getNode(id);
    node.title = title.trim() || node.title;
    node.updatedAt = now();
    return toSummary(node);
  };

  const moveNode = async ({
    id,
    parentId,
  }: {
    id: string;
    parentId: string | null;
  }) => {
    const node = await getNode(id);
    node.parentId = parentId;
    node.updatedAt = now();
    return toSummary(node);
  };

  const saveContent = async ({
    id,
    content,
  }: {
    id: string;
    content: string;
  }) => {
    const node = await getNode(id);
    node.content = content;
    node.updatedAt = now();
    return { id, updatedAt: node.updatedAt };
  };

  const deleteNode = async (id: string) => {
    const deleted: string[] = [];
    const collect = (target: string) => {
      const node = nodes.get(target);
      if (!node) {
        return;
      }
      deleted.push(target);
      nodes.delete(target);
      for (const child of [...nodes.values()]) {
        if (child.parentId === target) {
          collect(child.id);
        }
      }
    };
    collect(id);
    if (deleted.length === 0) {
      throw new Error("文档不存在或已被删除");
    }
    return { deletedIds: deleted };
  };

  const search = async ({
    q,
    limit = 20,
  }: {
    q: string;
    limit?: number;
    cursor?: string | null;
    signal?: AbortSignal;
  }): Promise<DocSearchPage> => {
    const query = q.trim().toLowerCase();
    if (!query) {
      return { items: [], nextCursor: null };
    }
    const hits = [...nodes.values()]
      .filter((node) => node.title.toLowerCase().includes(query))
      .sort((left, right) => left.title.localeCompare(right.title))
      .slice(0, limit)
      .map((node) => {
        const path: string[] = [];
        let parentId = node.parentId;
        while (parentId) {
          const parent = nodes.get(parentId);
          if (!parent) {
            break;
          }
          path.unshift(parent.title);
          parentId = parent.parentId;
        }
        return {
          id: node.id,
          kind: node.kind,
          title: node.title,
          parentId: node.parentId,
          path,
          updatedAt: node.updatedAt,
        };
      });
    return { items: hits, nextCursor: null };
  };

  return {
    listTree,
    getNode,
    createNode,
    renameNode,
    moveNode,
    saveContent,
    deleteNode,
    search,
  };
}

export const fakeDocsModel = createFakeDocsModel();
