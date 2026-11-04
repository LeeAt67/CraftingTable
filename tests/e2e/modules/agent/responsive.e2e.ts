import { expect, test } from "@playwright/test";
import { expectNoHorizontalOverflow } from "../../support/assertions";

test("Agent keeps its session sidebar at every target width", async ({
  page,
}) => {
  for (const width of [390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/#/agent");
    await expect(
      page.getByRole("complementary", { name: "Agent 会话" }),
    ).toBeVisible();
    await expectNoHorizontalOverflow(page);
  }
});

test("Agent stays within the mobile viewport while a reply is active", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill("这是一条用于检查移动端横向溢出的消息");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.getByRole("button", { name: "停止" })).toBeVisible();

  await expectNoHorizontalOverflow(page);
  await expect(page.getByRole("button", { name: "发送" })).toBeEnabled({
    timeout: 10_000,
  });
  await expectNoHorizontalOverflow(page);
});
