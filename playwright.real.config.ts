import { defineConfig, devices } from "@playwright/test";

const apiPort = process.env.API_PORT ?? "3101";

export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: "**/modules/agent/reasoning.e2e.ts",
  fullyParallel: false,
  workers: 1,
  timeout: 120_000,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4174",
    trace: "on-first-retry",
  },
  webServer: [
    {
      command: "pnpm e2e:api:real",
      url: `http://127.0.0.1:${apiPort}/api/v1/categories`,
      timeout: 120_000,
      reuseExistingServer: false,
    },
    {
      command: "pnpm e2e:web",
      url: "http://127.0.0.1:4174",
      timeout: 120_000,
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
