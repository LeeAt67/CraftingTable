import type { TodayModel } from "./contracts";

const unavailable = () => Promise.reject(new Error("ApiModel is not configured"));

export const apiModel: TodayModel = {
  listToday: unavailable,
  listCategories: unavailable,
  createItem: unavailable,
  updateItem: unavailable,
  completeItem: unavailable,
  uncompleteItem: unavailable,
};
