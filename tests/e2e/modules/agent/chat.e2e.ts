import { expect, test } from "@playwright/test";

test("creates a session and completes a message", async ({ page }) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/.+/);

  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("整理今天的重点");
  await page.getByRole("button", { name: "发送" }).click();

  const messages = page.getByRole("log", { name: "会话消息" });
  await expect(
    messages.getByText("整理今天的重点", { exact: true }),
  ).toBeVisible();
  await expect(messages.getByText(/已收到：整理今天的重点/)).toBeVisible();
  await expect(
    page.getByRole("link", { name: "整理今天的重点" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "发送" })).toBeEnabled();
});

test("blocks a second session while the first session is streaming", async ({
  page,
}) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/[0-9a-f-]+/);
  const sessionHref = new URL(page.url()).hash;
  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("会话 A 正在生成");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.getByRole("button", { name: "停止" })).toBeVisible();

  await page.getByRole("button", { name: "新建会话" }).click();

  await expect(composer).toBeDisabled();
  await expect(page.getByRole("button", { name: "发送" })).toBeDisabled();
  await page.locator(`a[href="${sessionHref}"]`).click();
  await page.getByRole("button", { name: "停止" }).click();
  await expect(page.getByRole("button", { name: "发送" })).toBeEnabled();
});

test("stops and retries a reply without duplicating the user message", async ({
  page,
}) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("停止后重试");
  await page.getByRole("button", { name: "发送" }).click();
  await page.getByRole("button", { name: "停止" }).click();

  await expect(page.getByRole("button", { name: "重试" })).toBeVisible();
  await expect(page.getByLabel("你的消息")).toHaveCount(1);
  await page.getByRole("button", { name: "重试" }).click();
  await expect(page.getByRole("button", { name: "停止" })).toBeVisible();
  await expect(page.getByRole("button", { name: "发送" })).toBeEnabled({
    timeout: 10_000,
  });
  await expect(page.getByLabel("你的消息")).toHaveCount(1);
  await expect(
    page
      .getByRole("log", { name: "会话消息" })
      .getByText(/已收到：停止后重试/),
  ).toBeVisible();
});

test("agent continues after a page reload during generation", async ({
  page,
}) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await page
    .getByRole("textbox", { name: "发送给 Agent" })
    .fill("慢速恢复测试");
  await page.getByRole("button", { name: "发送" }).click();

  const messages = page.getByRole("log", { name: "会话消息" });
  await expect(messages.getByText("第一段", { exact: false })).toBeVisible({
    timeout: 10_000,
  });
  await page.reload();

  await expect(messages.getByText("第一段", { exact: false })).toBeVisible();
  await expect(
    messages.getByText("第一段第二段", { exact: true }),
  ).toBeVisible({ timeout: 10_000 });
});
