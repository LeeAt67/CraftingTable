import type {
  Category,
  CreateItemInput,
  Item,
  TodayModel,
  UpdateItemInput,
} from "./contracts";

const categories: Category[] = [
  { id: "work", name: "工作", color: "#5468FF" },
  { id: "study", name: "学习", color: "#00B42A" },
  { id: "life", name: "生活", color: "#F59E0B" },
];

const now = () => new Date().toISOString();

export function createMockModel(): TodayModel {
  let items: Item[] = [
    {
      id: "item-1",
      kind: "task",
      title: "整理本周工作记录",
      status: "planned",
      categoryId: "work",
      startAt: new Date().toISOString(),
      createdAt: now(),
      updatedAt: now(),
    },
    {
      id: "item-2",
      kind: "event",
      title: "产品方案同步",
      status: "planned",
      categoryId: "work",
      startAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      endAt: new Date(Date.now() + 90 * 60 * 1000).toISOString(),
      createdAt: now(),
      updatedAt: now(),
    },
  ];

  return {
    async listToday() {
      return items;
    },
    async listCategories() {
      return categories;
    },
    async createItem(input: CreateItemInput) {
      const item: Item = {
        id: crypto.randomUUID(),
        kind: "task",
        title: input.title.trim(),
        status: input.startAt ? "planned" : "unplanned",
        categoryId: input.categoryId,
        startAt: input.startAt,
        endAt: input.endAt,
        createdAt: now(),
        updatedAt: now(),
      };
      items = [item, ...items];
      return item;
    },
    async updateItem(input: UpdateItemInput) {
      const current = items.find((item) => item.id === input.id);
      if (!current) throw new Error("Item not found");
      const item = { ...current, ...input, updatedAt: now() };
      items = items.map((value) => (value.id === item.id ? item : value));
      return item;
    },
    async completeItem(id: string) {
      const current = items.find((item) => item.id === id);
      if (!current) throw new Error("Item not found");
      const item = {
        ...current,
        status: "done" as const,
        completedAt: now(),
        updatedAt: now(),
      };
      items = items.map((value) => (value.id === id ? item : value));
      return item;
    },
    async uncompleteItem(id: string) {
      const current = items.find((item) => item.id === id);
      if (!current) throw new Error("Item not found");
      const item = {
        ...current,
        status: current.startAt ? ("planned" as const) : ("unplanned" as const),
        completedAt: undefined,
        updatedAt: now(),
      };
      items = items.map((value) => (value.id === id ? item : value));
      return item;
    },
  };
}

export const mockModel = createMockModel();
