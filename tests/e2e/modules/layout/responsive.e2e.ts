import { expect, test } from "@playwright/test";
import { expectNoHorizontalOverflow } from "../../support/assertions";

test("the shell remains usable at 390, 768, 1024 and 1440px", async ({
  page,
}) => {
  const widths = [390, 768, 1024, 1440];

  for (const width of widths) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/#/");
    await expect(
      page.getByRole("complementary", { name: "今日上下文" }),
    ).toBeVisible();
    await expectNoHorizontalOverflow(page);

    const navWidth = await page
      .getByRole("complementary", { name: "主导航" })
      .evaluate((element) => element.getBoundingClientRect().width);
    expect(navWidth).toBeGreaterThanOrEqual(34);
    expect(navWidth).toBeLessThanOrEqual(56);
  }
});
