import type { FastifyInstance } from "fastify";
import { createItemModel } from "../models/itemModel";
import type { LeeatDatabase } from "../models/db";
import { ok } from "../views/jsonView";

function dayRange() {
  const now = new Date();
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  const end = new Date(start);
  end.setDate(end.getDate() + 1);
  return { start: start.toISOString(), end: end.toISOString() };
}

export function registerTodayController(
  app: FastifyInstance,
  db: LeeatDatabase,
) {
  app.get("/api/v1/today", async () => {
    const range = dayRange();
    return ok(createItemModel(db).listToday(range.start, range.end));
  });
}
