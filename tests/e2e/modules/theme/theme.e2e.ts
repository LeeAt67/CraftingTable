import { expect, test } from "@playwright/test";

test("light and dark themes persist across reload", async ({ page }) => {
  await page.goto("/#/");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await page.getByRole("button", { name: "切换到深色主题" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.getByRole("button", { name: "切换到浅色主题" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});
