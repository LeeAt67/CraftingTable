import { expect, test } from "@playwright/test";
import { uniquePrompt } from "../../support/data";

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

test("agent history survives a direct URL reload", async ({ page }) => {
  const prompt = uniquePrompt("持久化测试");
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/[0-9a-f-]+/);

  await page.getByRole("textbox", { name: "发送给 Agent" }).fill(prompt);
  await page.getByRole("button", { name: "发送" }).click();
  await expect(
    page.getByRole("log", { name: "会话消息" }).getByText(prompt, {
      exact: true,
    }),
  ).toBeVisible();

  const url = page.url();
  await page.reload();

  await expect(page).toHaveURL(url);
  await expect(
    page.getByRole("log", { name: "会话消息" }).getByText(prompt, {
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByText("会话不存在或已被删除")).toHaveCount(0);
});
