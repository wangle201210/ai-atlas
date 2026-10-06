import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
const source=readFileSync(new URL('../bindings/ai-atlas/internal/atlas/service.ts',import.meta.url),'utf8');
const id=(name:string)=>Number(source.match(new RegExp(`function ${name}\\([^]*?ByID\\((\\d+)`))![1]);
test('force temp preview requires a new plan and explicit confirmation',async({page})=>{
 const path='/tmp/atlas-force-fixture';
 let executed=false;
 await page.route('**/wails/runtime',async route=>{
  const req=route.request().postDataJSON();const method=req?.args?.methodID;
  if(method===id('TempFiles')) return route.fulfill({json:[{path,name:'atlas-force-fixture',bytes:1024,modified:'2026-01-01T00:00:00Z',mtime:0,isDir:true,link:false,shared:true,confidence:'reference',cleanupBlocked:'多项目引用或工具共享目录，禁止整体清理',error:'',evidence:[{sessionId:'fixture',project:'/projects/atlas',kind:'命令引用',path}]}]});
  if(method===id('PreviewClean')||method===id('PreviewForceTempClean')){
   const force=method===id('PreviewForceTempClean');
   return route.fulfill({json:{token:force?'forced':'normal',kind:'temp',force,backup:true,expires:'2099-01-01T00:00:00Z',bytes:force?1024:0,items:[{id:path,path,bytes:1024,mtime:0,blocked:force?'':'多项目引用或工具共享目录，禁止整体清理'}]}});
  }
  if(method===id('ExecuteClean')){
   expect(req.args.args).toEqual(['forced','确认强制清理']);executed=true;
   return route.fulfill({json:[{id:path,success:true,message:'fixture moved to trash'}]});
  }
  await route.continue();
 });
 await page.goto('/');
 await page.getByRole('heading',{name:'项目分布'}).waitFor();
 await page.getByRole('button',{name:'临时文件',exact:true}).click();
 await page.getByRole('checkbox',{name:'选择 atlas-force-fixture',exact:true}).check();
 await page.getByRole('button',{name:'预览移入废纸篓',exact:true}).click();
 await expect(page.getByRole('button',{name:'执行清理',exact:true})).toBeDisabled();
 await page.getByRole('checkbox',{name:'强制删除（无需标记归属）',exact:true}).check();
 await expect(page.getByText('可清理',{exact:true})).toBeVisible();
 await page.locator('#confirmation').fill('确认清理');
 await expect(page.getByRole('button',{name:'执行清理',exact:true})).toBeDisabled();
 expect(executed).toBe(false);
 await page.locator('#confirmation').fill('确认强制清理');
 await page.getByRole('button',{name:'执行清理',exact:true}).click();
 await expect(page.getByText('fixture moved to trash')).toBeVisible();
 expect(executed).toBe(true);
});
