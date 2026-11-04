import Fastify from "fastify";
import { openDatabase } from "./models/db";
import { registerRoutes } from "./routes";
import { fail } from "./views/jsonView";

type Options = {
  databasePath: string;
  logger?: boolean;
};

export function createApp({ databasePath, logger = false }: Options) {
  const app = Fastify({ logger });
  const db = openDatabase(databasePath);

  registerRoutes(app, db);

  app.setErrorHandler((error, _request, reply) => {
    const message = error instanceof Error ? error.message : "请求失败";

    if (message === "Item not found") {
      return reply.code(404).send(fail("NOT_FOUND", message));
    }

    const statusCodeValue =
      typeof error === "object" &&
      error !== null &&
      "statusCode" in error &&
      typeof error.statusCode === "number"
        ? error.statusCode
        : 500;
    const statusCode =
      statusCodeValue >= 400
        ? statusCodeValue
        : 500;
    return reply
      .code(statusCode)
      .send(fail("INTERNAL_ERROR", message));
  });

  app.addHook("onClose", async () => {
    db.close();
  });

  return app;
}
