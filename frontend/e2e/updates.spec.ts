import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
const binding = readFileSync(
  new URL("../bindings/ai-atlas/internal/updates/service.ts", import.meta.url),
  "utf8",
);
const ids = Object.fromEntries(
  ["Status", "Check", "Download", "Restart"].map((name) => [
    name,
    Number(
      binding.match(new RegExp(`function ${name}\\([^]*?ByID\\((\\d+)`))![1],
    ),
  ]),
);

for (const scenario of ["available", "current", "error"] as const) {
  test(`version button handles ${scenario} release state`, async ({ page }) => {
    const calls: string[] = [];
    let state = {
      currentVersion: "0.1.1",
      latestVersion: "",
      phase: "idle",
      notes: "",
      releaseName: "",
      releaseURL: "https://github.com/wangle201210/ai-atlas/releases",
      checkedAt: "",
      written: 0,
      total: 2048,
      canInstall: true,
      error: "",
    };
    await page.route("**/wails/runtime", async (route) => {
      const req = route.request().postDataJSON();
      const method = Object.keys(ids).find(
        (k) => ids[k] === req?.args?.methodID,
      );
      if (req?.object !== 0 || !method) {
        await route.continue();
        return;
      }
      if (method === "Status") {
        await route.fulfill({ json: state });
        return;
      }
      calls.push(method);
      if (method === "Check")
        state = {
          ...state,
          phase: scenario,
          latestVersion: scenario === "available" ? "0.2.0" : "",
          notes: "增加更新功能\n<script>不可执行的发布说明</script>",
          error: scenario === "error" ? "网络暂不可用" : "",
        };
      if (method === "Download")
        state = { ...state, phase: "ready", written: 2048 };
      if (method === "Restart") state = { ...state, phase: "restarting" };
      await route.fulfill({ json: null });
    });
    await page.goto("/");
    const trigger = page.getByRole("button", {
      name: "检查 AI Atlas 更新",
      exact: true,
    });
    await expect(trigger).toContainText("v0.1.1");
    expect(calls).toEqual([]);
    await trigger.click();
    const dialog = page.getByRole("dialog", {
      name:
        scenario === "available"
          ? "发现新版本"
          : scenario === "current"
            ? "当前没有可用更新"
            : "更新未完成",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    expect(calls).toEqual(["Check"]);
    if (scenario === "available") {
      await expect(dialog.getByText("v0.2.0", { exact: true })).toBeVisible();
      await expect(dialog.locator("pre")).toContainText(
        "<script>不可执行的发布说明</script>",
      );
      await page.screenshot({ path: "test-results/update-available.png" });
      await dialog
        .getByRole("button", { name: "下载更新", exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: "更新已准备好", exact: true }),
      ).toBeVisible();
      expect(calls).toEqual(["Check", "Download"]);
      await page
        .getByRole("button", { name: "关闭更新窗口", exact: true })
        .click();
      await trigger.click();
      expect(calls).toEqual(["Check", "Download"]);
      await page
        .getByRole("button", { name: "重启并安装", exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: "正在重启安装", exact: true }),
      ).toBeVisible();
      expect(calls).toEqual(["Check", "Download", "Restart"]);
    } else if (scenario === "error") {
      await expect(dialog.getByRole("alert")).toContainText("网络暂不可用");
      await dialog
        .getByRole("button", { name: "重新检查", exact: true })
        .click();
      await expect.poll(() => [...calls]).toEqual(["Check", "Check"]);
    } else {
      await expect(
        dialog.getByText("GitHub Releases 暂未发布比当前版本更高的正式版本。"),
      ).toBeVisible();
    }
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).not.toBeVisible();
  });
}
