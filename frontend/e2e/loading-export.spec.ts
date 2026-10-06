import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
const binding = readFileSync(new URL('../bindings/ai-atlas/internal/atlas/service.ts', import.meta.url), 'utf8');
const id = (name: string) => Number(binding.match(new RegExp(`function ${name}\\([^]*?ByID\\((\\d+)`))![1]);

test('initial load shows progress, fails visibly, and can retry', async ({page}) => {
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let fail = true;
  await page.route('**/wails/runtime', async route => {
    if (route.request().postDataJSON()?.args?.methodID === id('Overview') && fail) {
      await gate;
      return route.fulfill({status:500, body:'fixture unavailable'});
    }
    await route.continue();
  });
  await page.goto('/');
  await expect(page.getByRole('heading', {name:'正在读取本地索引'})).toBeVisible();
  await expect(page.getByText('刷新索引后显示存储占用')).toHaveCount(0);
  release();
  await expect(page.getByRole('heading', {name:'数据加载失败'})).toBeVisible();
  fail = false;
  await page.getByRole('button', {name:'重新加载', exact:true}).click();
  await expect(page.getByRole('heading', {name:'项目分布'})).toBeVisible();
  await expect(page.getByRole('heading', {name:'数据加载失败'})).toHaveCount(0);
});

test('native report export handles cancel, failure and successful save honestly', async ({page}) => {
  let outcome = 'cancel';
  await page.route('**/wails/runtime', async route => {
    const request = route.request().postDataJSON();
    if (request?.args?.methodID === id('NativeFileDialogs')) return route.fulfill({contentType:'application/json',json:true});
    if (request?.args?.methodID === id('ExportReport')) {
      expect(request.args.args[0].sessions).toBeGreaterThan(0);
      if (outcome === 'failure') return route.fulfill({status:500,body:'fixture write failed'});
      return route.fulfill({contentType:'application/json',json:outcome === 'cancel' ? '' : '/fixture/report.json'});
    }
    await route.continue();
  });
  await page.goto('/');
  const exportButton=page.getByRole('button',{name:'导出报表',exact:true});
  await expect(exportButton).toBeEnabled();
  await exportButton.click();
  await expect(exportButton).toBeEnabled();
  await expect(page.getByText('报表已保存至：',{exact:false})).toHaveCount(0);
  outcome='failure';
  await exportButton.click();
  await expect(page.getByRole('alert').filter({hasText:'fixture write failed'})).toBeVisible();
  await expect(page.getByText('报表已保存至：',{exact:false})).toHaveCount(0);
  outcome='success';
  await exportButton.click();
  await expect(page.getByText('报表已保存至：/fixture/report.json')).toBeVisible();
});
