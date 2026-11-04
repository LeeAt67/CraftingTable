export type ItemKind = "task" | "event";
export type ItemStatus = "unplanned" | "planned" | "done";

export type Category = {
  id: string;
  name: string;
  color: string;
};

export type Item = {
  id: string;
  kind: ItemKind;
  title: string;
  description?: string;
  status: ItemStatus;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateItemInput = {
  title: string;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
};

export type UpdateItemInput = {
  id: string;
  title?: string;
  description?: string;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
};

export interface TodayModel {
  listToday(): Promise<Item[]>;
  listCategories(): Promise<Category[]>;
  createItem(input: CreateItemInput): Promise<Item>;
  updateItem(input: UpdateItemInput): Promise<Item>;
  completeItem(id: string): Promise<Item>;
  uncompleteItem(id: string): Promise<Item>;
}
