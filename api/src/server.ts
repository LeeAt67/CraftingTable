import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { createApp } from "./app";

const databasePath = resolve(
  process.env.LEEAT_DATABASE_PATH ?? resolve("data", "leeat.sqlite"),
);
mkdirSync(dirname(databasePath), { recursive: true });

const agentConfigured =
  process.env.AGENT_API_BASE_URL &&
  process.env.AGENT_API_KEY &&
  process.env.AGENT_MODEL;
const app = createApp({
  databasePath,
  logger: true,
  agent: agentConfigured
    ? {
        baseUrl: process.env.AGENT_API_BASE_URL!,
        apiKey: process.env.AGENT_API_KEY!,
        model: process.env.AGENT_MODEL!,
      }
    : undefined,
});

await app.listen({
  host: "127.0.0.1",
  port: Number(process.env.API_PORT ?? 3001),
});
