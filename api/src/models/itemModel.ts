import { randomUUID } from "node:crypto";
import type {
  CreateItemInput,
  Item,
  UpdateItemInput,
} from "../../../src/models/contracts";
import type { LeeatDatabase } from "./db";

type ItemRow = {
  id: string;
  kind: Item["kind"];
  title: string;
  description: string | null;
  status: Item["status"];
  category_id: string | null;
  start_at: string | null;
  end_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
};

const mapItem = (row: ItemRow): Item => ({
  id: row.id,
  kind: row.kind,
  title: row.title,
  description: row.description ?? undefined,
  status: row.status,
  categoryId: row.category_id ?? undefined,
  startAt: row.start_at ?? undefined,
  endAt: row.end_at ?? undefined,
  completedAt: row.completed_at ?? undefined,
  createdAt: row.created_at,
  updatedAt: row.updated_at,
});

export function createItemModel(db: LeeatDatabase) {
  const get = (id: string): Item => {
    const row = db
      .prepare("SELECT * FROM items WHERE id = ?")
      .get(id) as ItemRow | undefined;

    if (!row) throw new Error("Item not found");
    return mapItem(row);
  };

  return {
    listToday(dayStart: string, dayEnd: string): Item[] {
      const rows = db
        .prepare(
          `SELECT * FROM items
           WHERE (start_at >= ? AND start_at < ?)
              OR (start_at IS NULL AND created_at >= ? AND created_at < ?)
           ORDER BY start_at IS NULL, start_at, created_at`,
        )
        .all(dayStart, dayEnd, dayStart, dayEnd) as ItemRow[];

      return rows.map(mapItem);
    },

    create(input: CreateItemInput): Item {
      const now = new Date().toISOString();
      const id = randomUUID();

      db.prepare(
        `INSERT INTO items (
          id, kind, title, status, category_id, start_at, end_at, created_at, updated_at
        ) VALUES (?, 'task', ?, ?, ?, ?, ?, ?, ?)`,
      ).run(
        id,
        input.title.trim(),
        input.startAt ? "planned" : "unplanned",
        input.categoryId ?? null,
        input.startAt ?? null,
        input.endAt ?? null,
        now,
        now,
      );

      return get(id);
    },

    update(input: UpdateItemInput): Item {
      const current = get(input.id);
      const now = new Date().toISOString();

      db.prepare(
        `UPDATE items
         SET title = ?, description = ?, category_id = ?, start_at = ?, end_at = ?, updated_at = ?
         WHERE id = ?`,
      ).run(
        input.title ?? current.title,
        input.description ?? current.description ?? null,
        input.categoryId ?? current.categoryId ?? null,
        input.startAt ?? current.startAt ?? null,
        input.endAt ?? current.endAt ?? null,
        now,
        input.id,
      );

      return get(input.id);
    },

    complete(id: string): Item {
      get(id);
      const now = new Date().toISOString();

      db.prepare(
        "UPDATE items SET status = 'done', completed_at = ?, updated_at = ? WHERE id = ?",
      ).run(now, now, id);

      return get(id);
    },

    uncomplete(id: string): Item {
      const current = get(id);
      const now = new Date().toISOString();

      db.prepare(
        "UPDATE items SET status = ?, completed_at = NULL, updated_at = ? WHERE id = ?",
      ).run(current.startAt ? "planned" : "unplanned", now, id);

      return get(id);
    },
  };
}
