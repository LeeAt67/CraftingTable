import { spawn } from "node:child_process";

const server = spawn(
  "go",
  [
    "run",
    "./server/cmd/craftingtabled",
    "-env-file=.env.local",
    "-reset-database",
  ],
  {
    stdio: "inherit",
    env: {
      ...process.env,
      API_PORT: process.env.API_PORT ?? "3101",
      CRAFTINGTABLE_DATABASE_PATH:
        process.env.CRAFTINGTABLE_DATABASE_PATH ??
        "./data/craftingtable-e2e-real.sqlite",
    },
  },
);

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => {
    server.kill(signal);
  });
}

server.once("error", (error) => {
  console.error("failed to start real Agent E2E API", error);
  process.exitCode = 1;
});

server.once("exit", (code) => {
  process.exit(code ?? 1);
});
