import { test, expect } from "@playwright/test";
test("indexes fixtures, drills into sessions, previews without deleting, filters and exports", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "项目概览", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "刷新索引", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "刷新索引", exact: true }),
  ).toBeEnabled({ timeout: 30000 });
  await expect(page.getByRole("heading", { name: "项目分布" })).toBeVisible({
    timeout: 15000,
  });
  await page.screenshot({ path: "test-results/overview.png", fullPage: true });
  await page
    .getByRole("button", { name: "查看 atlas 会话", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "实现项目用量看板", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "实现项目用量看板", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByText("已经完成，可以查看测试结果。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "关闭对话框" }).click();
  await page
    .getByRole("checkbox", { name: "选择 实现项目用量看板", exact: true })
    .check();
  await page.getByRole("button", { name: "预览清理", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "清理预览", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("最近 24 小时有改动，暂不清理", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "执行清理", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("button", { name: "关闭对话框", exact: true })).toBeEnabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: "存储空间", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "目录占用", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "临时文件", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "扫描临时文件", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "项目概览", exact: true }).click();
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出报表", exact: true }).click();
  expect((await download).suggestedFilename()).toContain("ai-atlas-");
  await page.getByLabel("开始日期", { exact: true }).fill("2027-01-01");
  await expect(
    page.getByText("该时间范围没有用量记录", { exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
test("small viewport remains usable and settings can be dismissed by keyboard", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "项目概览", exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({ path: "test-results/mobile.png", fullPage: true });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page
    .getByRole("button", { name: "数据源与统计口径", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭对话框", exact: true })).toBeEnabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).not.toBeVisible();
});

test("resume command copies to clipboard and feedback expires after the latest click", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await page
    .getByRole("button", { name: "查看 atlas 会话", exact: true })
    .click();
  await page
    .getByRole("button", { name: "实现项目用量看板", exact: true })
    .click();
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  const copy = page.getByRole("button", {
    name: "复制继续会话命令",
    exact: true,
  });
  const feedback = page.getByRole("dialog").getByRole("status");
  await copy.click();
  await expect(copy).toBeEnabled();
  await expect(feedback).toHaveText("命令已复制到剪贴板，可直接粘贴到终端。");
  const command = await page.locator(".command").innerText();
  expect(command).toContain(
    "codex -C '/projects/atlas' resume '00000000-0000-0000-0000-000000000001'",
  );
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    command,
  );
  await page.clock.runFor(9000);
  await expect(feedback).toBeVisible();
  await copy.click();
  await expect(copy).toBeEnabled();
  await page.clock.runFor(9000);
  await expect(feedback).toBeVisible();
  await page.clock.runFor(1001);
  await expect(feedback).toHaveCount(0);
  await copy.click();
  await expect(copy).toBeEnabled();
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();
  await page.getByRole("button", { name: "存储空间", exact: true }).click();
  await expect(
    page.getByText("命令已复制到剪贴板，可直接粘贴到终端。", { exact: true }),
  ).toHaveCount(0);
});
