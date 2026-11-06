import { expect, test } from "@playwright/test";

test.skip(
  ({ viewport }) => Boolean(viewport && viewport.width < 768),
  "文档工作区只在桌面宽度下断言",
);

test.beforeEach(async ({ page }) => {
  await page.goto("/#/docs");
  await expect(
    page.getByRole("complementary", { name: "文档目录" }),
  ).toBeVisible();
});

test("creates a document, edits it, and keeps content after reload", async ({
  page,
}) => {
  await page.getByRole("button", { name: "新建文档" }).click();

  const title = page.getByLabel("文档标题");
  await expect(title).toBeVisible();
  await title.fill("接口设计");

  const editor = page.getByLabel("文档正文");
  await editor.click();
  await page.keyboard.type("# ");
  await page.keyboard.type("接口设计");
  await page.keyboard.press("Enter");
  await page.keyboard.type("自动保存验证");

  await expect(
    page.getByRole("heading", { level: 1, name: "接口设计" }),
  ).toBeVisible();
  await expect(page.getByText("自动保存验证")).toBeVisible();
  await expect(page.getByRole("status")).toHaveText("已保存", {
    timeout: 15_000,
  });

  await page.reload();

  await expect(page.getByLabel("文档标题")).toHaveValue("接口设计");
  await expect(
    page.getByRole("heading", { level: 1, name: "接口设计" }),
  ).toBeVisible();
  await expect(page.getByText("自动保存验证")).toBeVisible();
});

test("groups a document into a folder and searches it", async ({ page }) => {
  const tree = page.getByRole("navigation", { name: "文档树" });

  await page.getByRole("button", { name: "新建文件夹" }).click();
  await expect(tree.getByText("未命名文件夹", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "操作 未命名文件夹" }).click();
  await page.getByRole("button", { name: "重命名", exact: true }).click();
  await page.getByLabel("节点标题").fill("工作");
  await page.getByRole("button", { name: "保存", exact: true }).click();

  await expect(page.getByRole("button", { name: "操作 工作" })).toBeVisible();

  await page.getByRole("button", { name: "新建文档" }).click();
  await page.getByLabel("文档标题").fill("接口说明");
  await expect(
    page.getByRole("button", { name: "操作 接口说明" }),
  ).toBeVisible();

  await page.getByLabel("搜索文档").fill("接口");

  const results = page.getByRole("list", { name: "搜索结果" });
  await expect(results).toBeVisible();
  await expect(
    results.getByRole("button", { name: "接口说明", exact: true }),
  ).toBeVisible();
});
