import { expect, test } from "@playwright/test";

test("creates a session and completes a message", async ({ page }) => {
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/.+/);

  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("整理今天的重点");
  await page.getByRole("button", { name: "发送" }).click();

  await expect(page.getByText("整理今天的重点")).toBeVisible();
  await expect(page.getByText(/收到你的请求/)).toBeVisible();
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
  const sessionHref = await page
    .getByRole("link", { name: "新会话" })
    .getAttribute("href");
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

  await expect(page.getByRole("button", { name: "重试回复" })).toBeVisible();
  await expect(page.getByLabel("你的消息")).toHaveCount(1);
  await page.getByRole("button", { name: "重试回复" }).click();
  await expect(page.getByRole("button", { name: "停止" })).toBeVisible();
  await expect(page.getByRole("button", { name: "发送" })).toBeEnabled({
    timeout: 10_000,
  });
  await expect(page.getByLabel("你的消息")).toHaveCount(1);
  await expect(page.getByText(/收到你的请求/)).toBeVisible();
});
