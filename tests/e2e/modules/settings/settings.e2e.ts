import { expect, test } from "@playwright/test";

test("settings is reachable from the primary navigation", async ({ page }) => {
  await page.goto("/#/");
  await page.getByRole("link", { name: "设置" }).click();

  await expect(page).toHaveURL(/#\/settings$/);
  await expect(page.getByRole("heading", { name: "设置" })).toBeVisible();
  await page.getByRole("link", { name: "返回今日" }).click();
  await expect(page.getByRole("heading", { name: "今日" })).toBeVisible();
});
