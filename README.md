# AI Atlas

本地 AI 编程工具的项目、Token 用量、会话和存储管理工具。当前支持 **Codex**，后续可扩展 Claude Code 等数据源；本次更名不代表已支持其他工具。使用 **Wails 3.0.0-beta.27 + Vue 3 / TypeScript + Go 1.25+ + SQLite**，优先支持 macOS。

## 功能

- **项目看板**：按工作目录汇总输入、缓存输入、输出、总 Token；日期筛选、最近 30 个有记录日期的趋势、JSON 导出。
- **会话管理**：搜索标题/路径/ID，项目和状态筛选，按时间/体积/Token 排序，每页 50 条；预览对话、一键复制继续会话命令到系统剪贴板、归档/取消归档。复制反馈只在弹窗显示，10 秒后自动消失。
- **存储分析**：扫描 Codex Home 各条目，按逻辑文件大小排序，定位大体积会话。
- **临时文件**：扫描 Codex 临时目录、`/tmp`、系统 `TMPDIR`；关联工具命令/输出中的绝对路径，显示会话与项目线索，未知归属单列。
- **清理预览**：最多 50 项；检查路径、符号链接、最近修改及 `lsof` 占用，执行前再次校验。会话通过官方 `codex delete --force <UUID>` 删除；macOS 临时文件移入废纸篓。
- **历史用量留存**：独立 SQLite 索引，源日志移除后保留用量统计。扫描按文件大小与修改时间跳过未变化文件，变化文件流式重新解析。

无远程 API 调用，不读取认证文件内容、不上传日志。存储扫描只读取文件元数据；会话内容在打开预览时读取。

## 运行

需要 Go 1.25+、Node.js 22.12+、npm、Xcode Command Line Tools。清理/归档需要 Codex CLI；占用检查需要 `lsof`。

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
export PATH="$(go env GOPATH)/bin:$PATH"
# 在项目根目录运行
wails3 dev
```

构建并打包 Mac 应用：

```sh
wails3 package
open bin/ai-atlas.app
```

首次打开点击「开始扫描」。已有索引会直接展示，手动「刷新索引」同步变化。临时目录较大，单独点击「扫描临时文件」启动分析。扫描状态与错误会展示在页面顶部；不完整日志保留上一次成功索引。

### 数据位置

| 环境变量 | 默认值 | 用途 |
|---|---|---|
| `CODEX_HOME` | `~/.codex` | 会话及存储扫描根目录 |
| `AI_ATLAS_DB` | `~/.ai-atlas/atlas.sqlite` | Atlas 自己的 SQLite 数据库 |
| `AI_ATLAS_CODEX` | 自动发现 | Codex CLI 的绝对路径，可覆盖 PATH / Homebrew / NVM 查找 |

不改写 Codex 自身 SQLite。归档/删除委托 Codex CLI；首次使用清理前先在预览里核对完整路径。临时文件移入废纸篓后仍占用磁盘，需要用户清空废纸篓才真正释放空间。

### 无桌面窗口的本地模式

```sh
wails3 build
# SQLite 驱动使用 CGO，服务器模式也需要 CGO_ENABLED=1
CGO_ENABLED=1 go build -tags server -o bin/ai-atlas-server .
WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT=19427 ./bin/ai-atlas-server
```

浏览器访问 `http://127.0.0.1:19427`。此模式用于本机开发/验证，没有多用户认证，不应暴露到公网。远程服务器上的实例只能访问该服务器的文件。

## 测试

```sh
go test -race ./internal/atlas ./cmd/atlas-index
go vet ./...
# 生成绑定、构建前端和原生程序
wails3 build
# 隔离数据的端到端测试；使用本机 Google Chrome
python3 scripts/create-fixture.py
CGO_ENABLED=1 go build -tags server -o bin/ai-atlas-server .
cd frontend
npm run test:e2e
```

测试数据位于 `.test-data/`，不会读取或删除真实 Codex Home。端到端测试覆盖扫描、项目进入会话、消息预览、清理阻止、日期筛选、报表下载、窄屏和对话框键盘操作。Go 测试覆盖累计/重复/重置计数、分叉去重、历史留存、路径证据、长行/不完整 JSON、符号链接、清理计划和 CLI 删除（使用隔离的假 CLI）。

仅建立索引并导出报表，也可运行：

```sh
go run ./cmd/atlas-index -home "$HOME/.codex" -db "$HOME/.ai-atlas/atlas.sqlite" > report.json
```

## 统计口径与边界

1. 输入包含缓存输入，输出包含推理输出；不重复相加。总量保留日志的 `total_tokens` 增量，旧格式计数可能与分项存在差异。
2. 同一会话累计快照取增量，重复快照只算一次。计数器重置用最后请求用量续计；日志首条若含更早累计，只计最后请求并展示提示。
3. 跨会话以 `turn_id + 累计快照 + 最后请求用量` 去重；重复历史归给创建更早的会话。缺失父日志、缺失 turn ID 或旧版重写行为可能导致归属不完整。独立的压缩 `token_usage_record` 暂未单独纳入。它是本地统计，不是账单审计或额度监控。
4. Token 按请求上下文目录归类；会话数/体积按初始目录归类。默认精确目录，不合并 worktree 或相邻仓库。日期使用本机时区，筛选只作用于用量。
5. 同一 ID 的活动/归档副本只索引一份，优先活动日志；所有已移除会话仍保留历史索引，因此“会话数”含仅统计记录。
6. 文件体积是逻辑大小，不推断 APFS 共享块、压缩或实际可回收字节。不会跟随符号链接；权限错误明确显示。
7. 临时文件只通过工具参数/输出里的绝对路径建立关联，不把聊天中的提及视为证据。引用不等于创建证明；相对路径、shell 变量、未记录路径无法可靠追溯。目前针对 macOS 常见路径格式。
8. 临时文件清单在当前进程内缓存，重启需重扫。未知归属、扫描不完整、符号链接、最近 24 小时修改或无法确认空闲的项禁止清理；只处理扫描根目录的直接子项。
9. 清理计划 5 分钟有效且只能用一次。执行前再次检查大小/修改时间及占用；进程占用检查仍无法完全消除外部程序同时访问的竞争窗口。
10. 会话预览显示前 150 条用户/助手消息，每条最多 12,000 字符，不渲染原始 HTML，不解码图片。日志单行超过 32 MiB 会报告错误。

## 结构

```text
main.go                    Wails 桌面入口
internal/atlas/
  parser.go                流式 JSONL 解析与引用提取
  store.go                 独立 SQLite 表及去重视图
  service.go               扫描、项目汇总、会话查询
  temp.go                  临时目录和引用关联
  cleanup.go               预览、校验、清理及归档
  codex.go                 CLI 发现与启动
cmd/atlas-index/           只读源数据索引命令
frontend/src/              Vue 中文界面与类型适配
frontend/bindings/         Wails 自动生成绑定
frontend/e2e/              Playwright 端到端测试
```

Wails v3 为 Beta，Go 模块与前端 runtime 固定到同一个版本。默认桌面 UI 不需要另起 Web 服务。
