import type { FastifyInstance, FastifyReply } from "fastify";
import { z } from "zod";
import { createItemModel } from "../models/itemModel";
import type { LeeatDatabase } from "../models/db";
import { fail, ok } from "../views/jsonView";

const createSchema = z.object({
  title: z.string().trim().min(1).max(200),
  categoryId: z.string().min(1).optional(),
  startAt: z.string().datetime().optional(),
  endAt: z.string().datetime().optional(),
});

const updateSchema = z.object({
  id: z.string().min(1),
  title: z.string().trim().min(1).max(200).optional(),
  description: z.string().max(2000).optional(),
  categoryId: z.string().min(1).optional(),
  startAt: z.string().datetime().optional(),
  endAt: z.string().datetime().optional(),
});

const idSchema = z.object({
  id: z.string().min(1),
});

function validationError(reply: FastifyReply, message: string) {
  return reply.code(400).send(fail("VALIDATION_ERROR", message));
}

export function registerItemController(
  app: FastifyInstance,
  db: LeeatDatabase,
) {
  const model = () => createItemModel(db);

  app.post("/api/v1/item/create", async (request, reply) => {
    const parsed = createSchema.safeParse(request.body);
    if (!parsed.success) {
      return validationError(
        reply,
        parsed.error.issues[0]?.message ?? "参数错误",
      );
    }
    return ok(model().create(parsed.data));
  });

  app.post("/api/v1/item/update", async (request, reply) => {
    const parsed = updateSchema.safeParse(request.body);
    if (!parsed.success) {
      return validationError(
        reply,
        parsed.error.issues[0]?.message ?? "参数错误",
      );
    }
    return ok(model().update(parsed.data));
  });

  app.post("/api/v1/item/complete", async (request, reply) => {
    const parsed = idSchema.safeParse(request.body);
    if (!parsed.success) return validationError(reply, "缺少 id");
    return ok(model().complete(parsed.data.id));
  });

  app.post("/api/v1/item/uncomplete", async (request, reply) => {
    const parsed = idSchema.safeParse(request.body);
    if (!parsed.success) return validationError(reply, "缺少 id");
    return ok(model().uncomplete(parsed.data.id));
  });
}
