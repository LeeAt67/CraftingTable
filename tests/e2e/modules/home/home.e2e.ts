import { expect, test } from "@playwright/test";
import { expectNoHorizontalOverflow } from "../../support/assertions";

test("home renders the workbench with a visible context sidebar", async ({
  page,
}, testInfo) => {
  await page.goto("/#/");

  await expect(page.getByRole("heading", { name: "今日" })).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "今日上下文" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "首页" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expectNoHorizontalOverflow(page);

  const screenshot = await page.screenshot({
    path: testInfo.outputPath(`leeat-home-${testInfo.project.name}.png`),
    fullPage: true,
  });
  expect(screenshot.byteLength).toBeGreaterThan(10_000);
});
