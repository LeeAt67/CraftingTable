import fs from "node:fs";
import path from "node:path";
import { chromium } from "@playwright/test";

const directory = path.resolve("docs/architecture");
const chromePath = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe";
const browser = await chromium.launch({
  headless: true,
  executablePath: fs.existsSync(chromePath) ? chromePath : undefined,
});

try {
  const files = fs
    .readdirSync(directory)
    .filter((file) => file.startsWith("agent-") && file.endsWith(".svg"));

  for (const file of files) {
    const svgPath = path.join(directory, file);
    const pngPath = svgPath.replace(/\.svg$/, ".png");
    const svg = fs.readFileSync(svgPath, "utf8");
    const match = svg.match(/viewBox="0 0 (\d+) (\d+)"/);
    if (!match) {
      throw new Error(`Missing viewBox in ${file}`);
    }

    const width = Number(match[1]);
    const height = Number(match[2]);
    const page = await browser.newPage({
      viewport: { width, height },
      deviceScaleFactor: 2,
    });

    await page.setContent(
      `<html><body style="margin:0;background:#fff;overflow:hidden">${svg}</body></html>`,
      { waitUntil: "load" },
    );
    await page.screenshot({ path: pngPath, type: "png" });
    await page.close();
    console.log(`${file} -> ${path.basename(pngPath)} (${width}x${height} @2x)`);
  }
} finally {
  await browser.close();
}
