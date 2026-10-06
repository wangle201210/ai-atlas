import { test, expect } from "@playwright/test";
import { mkdirSync, writeFileSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
const source = resolve("../.test-data/home"),
  extra = resolve("../.test-data/extra-temp");
const id = "00000000-0000-0000-0000-000000000099";
test.beforeAll(() => {
  mkdirSync(extra, { recursive: true });
  mkdirSync(source + "/sessions", { recursive: true });
  const rows: any[] = [
    {
      type: "session_meta",
      payload: {
        id,
        cwd: "/projects/paging",
        timestamp: "2026-09-01T00:00:00Z",
      },
    },
  ];
  for (let i = 0; i < 130; i++)
    rows.push({
      type: "response_item",
      timestamp: "2026-09-01T00:01:00Z",
      payload: {
        type: "message",
        role: "user",
        content: [
          {
            type: "input_text",
            text: i === 0 ? "长会话分页测试" : `消息编号 ${i} 搜索标记-${i}`,
          },
        ],
      },
    });
  writeFileSync(
    source + "/sessions/" + id + ".jsonl",
    rows.map((r) => JSON.stringify(r)).join("\n") + "\n",
  );
});
test("settings persist and long sessions show latest messages with search", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "数据源与统计口径", exact: true })
    .click();
  await expect(
    page.getByLabel("清理会话时先压缩备份，保留恢复能力"),
  ).toBeChecked();
  await expect(
    page.getByLabel("扫描系统临时目录（/tmp 和 TMPDIR）"),
  ).toBeChecked();
  await expect(page.locator(".cli-path")).toContainText("当前生效路径（已保存配置）");
  await page.getByLabel("额外临时目录（每行一个）").fill(extra);
  await page.getByRole("button", { name: "保存设置", exact: true }).click();
  await expect(
    page.getByText("设置已保存。刷新索引后读取新数据源。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();
  await page.getByRole("button", { name: "刷新索引", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "刷新索引", exact: true }),
  ).toBeEnabled({ timeout: 30000 });
  await page
    .getByRole("button", { name: "查看 paging 会话", exact: true })
    .click();
  await page
    .getByRole("button", { name: "长会话分页测试", exact: true })
    .click();
  await expect(
    page.getByText("消息编号 129 搜索标记-129", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("消息编号 1 搜索标记-1", { exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("会话内搜索").fill("搜索标记-42");
  await page.getByRole("button", { name: "搜索", exact: true }).click();
  await expect(
    page.getByText("消息编号 42 搜索标记-42", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("消息编号 129 搜索标记-129", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();
  await page
    .getByRole("button", { name: "数据源与统计口径", exact: true })
    .click();
  await expect(page.getByLabel("额外临时目录（每行一个）")).toHaveValue(extra);
  await page.screenshot({ path: "test-results/preferences.png" });
});
test("recovery requires explicit confirmation before sending a restore operation", async ({
  page,
}) => {
  const binding = readFileSync(
    resolve("bindings/ai-atlas/internal/atlas/service.ts"),
    "utf8",
  );
  const methodID = (name: string) =>
    Number(
      binding.match(new RegExp(`function ${name}\\([^]*?ByID\\((\\d+)`))![1],
    );
  const historyID = methodID("Recoveries"),
    restoreID = methodID("RestoreRecovery");
  let restored = false;
  const record = {
    id: "fixture-recovery",
    kind: "temp",
    source: "/fixture/original",
    stored: "/fixture/trash",
    sessionId: "",
    home: "",
    created: "2026-10-01T00:00:00Z",
    state: "ready",
    error: "",
    restoredAt: "",
    size: 100,
    storedBytes: 100,
    digest: "",
    wasArchived: false,
  };
  await page.route("**/wails/runtime", async (route) => {
    const req = route.request().postDataJSON();
    if (req?.args?.methodID === historyID) {
      await route.fulfill({
        json: [{ ...record, state: restored ? "restored" : "ready" }],
      });
      return;
    }
    if (req?.args?.methodID === restoreID) {
      expect(req.args.args).toEqual(["fixture-recovery", "确认恢复"]);
      restored = true;
      await route.fulfill({ json: null });
      return;
    }
    await route.continue();
  });
  await page.goto("/");
  await page.getByRole("button", { name: "清理历史", exact: true }).click();
  await page.getByRole("button", { name: "恢复…", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "执行恢复", exact: true }),
  ).toBeDisabled();
  expect(restored).toBe(false);
  await page.getByLabel("输入「确认恢复」").fill("确认恢复");
  await page.getByRole("button", { name: "执行恢复", exact: true }).click();
  await expect(page.getByText("已恢复到原路径", { exact: true })).toBeVisible();
  expect(restored).toBe(true);
});
