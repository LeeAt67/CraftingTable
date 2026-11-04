import type { FastifyInstance } from "fastify";
import { registerCategoryController } from "./controllers/categoryController";
import { registerItemController } from "./controllers/itemController";
import { registerTodayController } from "./controllers/todayController";
import type { LeeatDatabase } from "./models/db";

export function registerRoutes(app: FastifyInstance, db: LeeatDatabase) {
  registerTodayController(app, db);
  registerCategoryController(app, db);
  registerItemController(app, db);
}
