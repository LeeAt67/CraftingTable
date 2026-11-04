import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { createApp } from "./app";

const databasePath = resolve(
  process.env.LEEAT_DATABASE_PATH ?? resolve("data", "leeat.sqlite"),
);
mkdirSync(dirname(databasePath), { recursive: true });

const app = createApp({ databasePath, logger: true });

await app.listen({
  host: "127.0.0.1",
  port: Number(process.env.API_PORT ?? 3001),
});
