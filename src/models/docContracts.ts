export type DocNodeKind = "folder" | "document";

/** 与后端 server/internal/docs/node.go 的 MaxContentRunes 保持一致。 */
export const MAX_CONTENT_RUNES = 200000;

export const CONTENT_TOO_LONG_MESSAGE = `文档内容过长（上限 ${MAX_CONTENT_RUNES} 字符）`;

export type DocNodeSummary = {
  id: string;
  parentId: string | null;
  kind: DocNodeKind;
  title: string;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

export type DocNode = DocNodeSummary & {
  content: string;
};

export type DocSearchHit = {
  id: string;
  kind: DocNodeKind;
  title: string;
  parentId: string | null;
  path: string[];
  updatedAt: string;
};

export type DocSearchPage = {
  items: DocSearchHit[];
  nextCursor: string | null;
};

export type CreateDocNodeInput = {
  kind: DocNodeKind;
  parentId?: string | null;
  title?: string;
  clientRequestId: string;
};

export type SaveDocContentResult = {
  id: string;
  updatedAt: string;
};

export interface DocsModel {
  listTree(): Promise<DocNodeSummary[]>;
  getNode(id: string): Promise<DocNode>;
  createNode(input: CreateDocNodeInput): Promise<DocNode>;
  renameNode(input: { id: string; title: string }): Promise<DocNodeSummary>;
  moveNode(input: {
    id: string;
    parentId: string | null;
  }): Promise<DocNodeSummary>;
  saveContent(input: {
    id: string;
    content: string;
  }): Promise<SaveDocContentResult>;
  deleteNode(id: string): Promise<{ deletedIds: string[] }>;
  search(input: {
    q: string;
    limit?: number;
    cursor?: string | null;
    signal?: AbortSignal;
  }): Promise<DocSearchPage>;
}
