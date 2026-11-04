import { expect, test } from "@playwright/test";
import { uniqueTitle } from "../../support/data";

test("quick capture creates an item that survives reload", async ({ page }) => {
  const title = uniqueTitle("E2E 新建");
  await page.goto("/#/");

  await page.getByRole("button", { name: "新建待办" }).click();
  await expect(
    page.getByRole("dialog", { name: "新建待办" }),
  ).toBeVisible();
  await page.getByRole("textbox", { name: "待办标题" }).fill(title);
  await page.getByRole("button", { name: "保存" }).click();

  await expect(page.getByText(title)).toBeVisible();
  await page.reload();
  await expect(page.getByText(title)).toBeVisible();
});
