import type { Category } from "../../../src/models/contracts";
import type { LeeatDatabase } from "./db";

type CategoryRow = {
  id: string;
  name: string;
  color: string;
};

export function createCategoryModel(db: LeeatDatabase) {
  return {
    list(): Category[] {
      const rows = db
        .prepare("SELECT id, name, color FROM categories ORDER BY name")
        .all() as CategoryRow[];

      return rows.map((row) => ({
        id: row.id,
        name: row.name,
        color: row.color,
      }));
    },
  };
}
