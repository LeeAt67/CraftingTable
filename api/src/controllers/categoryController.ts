import type { FastifyInstance } from "fastify";
import { createCategoryModel } from "../models/categoryModel";
import type { LeeatDatabase } from "../models/db";
import { ok } from "../views/jsonView";

export function registerCategoryController(
  app: FastifyInstance,
  db: LeeatDatabase,
) {
  app.get("/api/v1/categories", async () => {
    return ok(createCategoryModel(db).list());
  });
}
