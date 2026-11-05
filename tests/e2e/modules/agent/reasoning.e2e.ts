import { expect, test } from "@playwright/test";
import { uniquePrompt } from "../../support/data";

test("keeps reasoning above the answer across reloads", async ({ page }) => {
  const prompt = uniquePrompt("请确认收到这条消息");
  await page.goto("/#/agent");
  await page.getByRole("button", { name: "新建会话" }).click();
  await expect(page).toHaveURL(/#\/agent\/.+/);

  const composer = page.getByRole("textbox", { name: "发送给 Agent" });
  await composer.fill(prompt);
  await page.getByRole("button", { name: "发送" }).click();

  const toggle = page.getByRole("button", { name: "Reasoning" });
  await expect(toggle).toBeVisible({ timeout: 30_000 });
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  const reasoning = page.getByLabel("Reasoning 内容");
  await expect(reasoning).toHaveText(/\S/, { timeout: 30_000 });

  const messages = page.getByRole("log", { name: "会话消息" });
  const answer = messages.getByLabel("Agent 回复正文");
  await expect(answer).toHaveText(/\S/, { timeout: 60_000 });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(reasoning).toHaveCount(0);

  const toggleBox = await toggle.boundingBox();
  const answerBox = await answer.boundingBox();
  expect(toggleBox).not.toBeNull();
  expect(answerBox).not.toBeNull();
  expect(toggleBox!.y).toBeLessThan(answerBox!.y);

  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(page.getByLabel("Reasoning 内容")).toHaveText(/\S/);

  await page.reload();
  const restored = page.getByRole("button", { name: "Reasoning" });
  await expect(restored).toBeVisible({ timeout: 30_000 });
  await expect(restored).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByLabel("Reasoning 内容")).toHaveCount(0);
  await restored.click();
  await expect(page.getByLabel("Reasoning 内容")).toHaveText(/\S/);
});
