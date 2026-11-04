import { expect, test } from "@playwright/test";

test("enters the Agent workspace from primary navigation", async ({ page }) => {
  await page.goto("/#/");
  await page.getByRole("link", { name: "Agent" }).click();

  await expect(page).toHaveURL(/#\/agent$/);
  await expect(page.getByRole("heading", { name: "Agent" })).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Agent 会话" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Agent" })).toHaveAttribute(
    "aria-current",
    "page",
  );
});
