import type {
  Category,
  CreateItemInput,
  Item,
  TodayModel,
  UpdateItemInput,
} from "./contracts";

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

export const apiModel: TodayModel = {
  listToday: () => request<Item[]>("/today"),
  listCategories: () => request<Category[]>("/categories"),
  createItem: (input: CreateItemInput) =>
    request<Item>("/item/create", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateItem: (input: UpdateItemInput) =>
    request<Item>("/item/update", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  completeItem: (id: string) =>
    request<Item>("/item/complete", {
      method: "POST",
      body: JSON.stringify({ id }),
    }),
  uncompleteItem: (id: string) =>
    request<Item>("/item/uncomplete", {
      method: "POST",
      body: JSON.stringify({ id }),
    }),
};
