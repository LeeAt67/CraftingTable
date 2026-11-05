import type {
  CreateDocNodeInput,
  DocNode,
  DocNodeSummary,
  DocsModel,
  DocSearchPage,
  SaveDocContentResult,
} from "./docContracts";

type Envelope<T> =
  | { data: T }
  | { error: { code: string; message: string } };

const baseUrl = import.meta.env.VITE_API_BASE ?? "/api/v1";

async function readEnvelope<T>(response: Response): Promise<Envelope<T> | null> {
  const text = await response.text();
  if (!text.trim()) {
    return null;
  }
  try {
    return JSON.parse(text) as Envelope<T>;
  } catch {
    return null;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const body = await readEnvelope<T>(response);
  if (!body) {
    throw new Error(
      response.ok
        ? "服务返回了无效响应，请稍后重试"
        : "服务暂时不可用，请稍后重试",
    );
  }
  if (!response.ok || "error" in body) {
    throw new Error("error" in body ? body.error.message : "请求失败");
  }
  return body.data;
}

export const apiDocsModel: DocsModel = {
  listTree: () => request<DocNodeSummary[]>("/docs/tree"),
  getNode: (id: string) =>
    request<DocNode>(`/docs/item?id=${encodeURIComponent(id)}`),
  createNode: (input: CreateDocNodeInput) =>
    request<DocNode>("/docs/create", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  renameNode: (input) =>
    request<DocNodeSummary>("/docs/rename", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  moveNode: (input) =>
    request<DocNodeSummary>("/docs/move", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  saveContent: (input) =>
    request<SaveDocContentResult>("/docs/save", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  deleteNode: (id: string) =>
    request<{ deletedIds: string[] }>("/docs/delete", {
      method: "POST",
      body: JSON.stringify({ id }),
    }),
  search: ({ q, limit, cursor, signal }) => {
    const params = new URLSearchParams({ q });
    if (limit !== undefined) {
      params.set("limit", String(limit));
    }
    if (cursor) {
      params.set("cursor", cursor);
    }
    // URLSearchParams 把空格编码成 "+"，统一成 "%20" 便于代理与日志排查。
    const query = params.toString().replace(/\+/g, "%20");
    return request<DocSearchPage>(`/docs/search?${query}`, { signal });
  },
};
