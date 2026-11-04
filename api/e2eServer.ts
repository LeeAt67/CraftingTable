import { mkdirSync, rmSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { createApp } from "./src/app";

const databasePath = resolve("data", "leeatE2e.sqlite");
mkdirSync(dirname(databasePath), { recursive: true });

for (const suffix of ["", "-shm", "-wal"]) {
  rmSync(`${databasePath}${suffix}`, { force: true });
}

const app = createApp({ databasePath, logger: false });

await app.listen({
  host: "127.0.0.1",
  port: Number(process.env.API_PORT ?? 3101),
});
