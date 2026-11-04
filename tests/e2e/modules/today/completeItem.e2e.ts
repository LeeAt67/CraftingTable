import { expect, test } from "@playwright/test";
import { uniqueTitle } from "../../support/data";

test("a task can be completed and restored", async ({ page }) => {
  const title = uniqueTitle("E2E 完成");
  await page.goto("/#/");

  await page.getByRole("button", { name: "新建待办" }).click();
  await page.getByRole("textbox", { name: "待办标题" }).fill(title);
  await page.getByRole("button", { name: "保存" }).click();

  const complete = page.getByRole("button", { name: `完成 ${title}` });
  await expect(complete).toBeVisible();
  await complete.click();

  const restore = page.getByRole("button", { name: `取消完成 ${title}` });
  await expect(restore).toHaveAttribute("aria-pressed", "true");
  await restore.click();
  await expect(
    page.getByRole("button", { name: `完成 ${title}` }),
  ).toHaveAttribute("aria-pressed", "false");
});
