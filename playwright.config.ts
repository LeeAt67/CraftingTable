import { defineConfig, devices } from "@playwright/test";

const apiPort = process.env.API_PORT ?? "3101";

export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: "**/*.e2e.ts",
  fullyParallel: false,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4174",
    trace: "on-first-retry",
  },
  webServer: [
    {
      command: "node scripts/fakeAgentProvider.mjs",
      url: "http://127.0.0.1:3201/healthz",
      reuseExistingServer: false,
    },
    {
      command: "pnpm e2e:api",
      url: `http://127.0.0.1:${apiPort}/api/v1/categories`,
      reuseExistingServer: false,
    },
    {
      command: "pnpm e2e:web",
      url: "http://127.0.0.1:4174",
      reuseExistingServer: false,
    },
  ],
  projects: [
    {
      name: "desktop",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1440, height: 900 },
      },
    },
    {
      name: "mobile",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 390, height: 844 },
      },
    },
  ],
});
