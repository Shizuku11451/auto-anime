# 参与开发

Auto-Anime 使用 Go 后端与 Vue 前端，面向 NAS 和 Docker 部署。请按现有职责拆分修改，避免在功能修改中夹带无关重构。

当前适配目标为飞牛 fnOS、Linux 和 Windows。飞牛 NAS 为主要维护与实机验收重点，Linux 提供服务端部署，Windows 10/11 x64 为次要适配并保留现有回归与打包检查。平台定位不要求删除其他已有代码，也不代表其他 NAS 品牌已验收；完整边界见 [平台适配范围](README.md#平台适配范围)。

## 仓库目录

| 路径 | 用途 | 约定 |
| --- | --- | --- |
| `backend/` | Go 模块、服务入口、业务协调与集成测试 | 在此目录运行 Go 命令 |
| `backend/internal/filter/` | 关键词与简繁等价匹配 | 使用内嵌字典，不依赖网络，纯逻辑测试随包维护 |
| `backend/internal/title/` | 标题、季集、搜索季数与元数据名称匹配 | 共用解析规则，不依赖数据库和网络，纯逻辑测试随包维护 |
| `backend/static/` | 随服务交付的前端构建产物 | 由 `web/` 构建生成并入库，不手工修改 |
| `web/` | Vue、TypeScript、样式与前端测试 | Node.js 仅用于前端开发与构建 |
| `deploy/fnos/` | 飞牛 Compose、安装配置页与 FPK 模板 | 安装配置页由前端构建生成，不手工修改 |
| `scripts/` | 开发启动、容器入口与发布验证脚本 | 可复用脚本随源码入库，不放私人凭据 |
| `tools/` | 打包、制品规范化与相关测试 | Python 为构建辅助工具，不是服务端运行依赖 |
| `docs/` | 面向用户的部署、配置与平台说明 | 从[文档索引](docs/README.md)进入 |
| `local/` | 本地临时脚本与内部资料，不入库 | 评估和验收报告放在 `local/reports/`，不存放正式构建输入 |
| `.github/workflows/` | 持续检查与发布工作流 | 修改发布路径时同步检查脚本引用 |
| 根目录 Docker / Compose 文件 | 通用部署与源码镜像构建 | 默认 Compose 拉取发布镜像，源码构建使用覆盖文件 |

Go 测试与实现文件相邻，前端测试与对应组件或模块相邻。后端逐步将独立能力提取到 `internal/`；数据库、下载与归档协调暂留 `package main`，不为视觉整齐强行拆包。

以下内容是本机工作资料，不属于公开源码：数据目录、`dist/`、缓存、可执行文件、日志、`.env` 及其本地变体、私钥、`local/`、`promo/`、`AGENTS.md`、本地协作历史和带日期的内部评估/验收记录。忽略规则不会删除这些文件，也不会自动移除已经提交的文件或历史。

`promo/` 是独立的本地宣传工程，不参与应用构建。若以后需要公开宣传素材，请另行检查授权、内容与输出，并明确维护方式。

### 文件放置规则

- `scripts/` 放开发启动、容器运行与发布流程入口；`tools/` 放由这些流程调用的制品处理工具及其测试，不按脚本语言划分职责。
- 新的本地评估、验收报告放在 `local/reports/`；协作历史保留在 `docs/AGENTS_HISTORY.md`，两者都不参与发布。
- 临时 Go 工具如需使用后端模块依赖，放在 `backend/tmp_<用途>/`，并为该具体目录添加忽略规则；不要加入正式构建与发布输入。
- `data/` 是运行数据，`dist/` 是打包输出，`.gocache/` 是构建缓存。根目录与 `backend/` 下的数据可能属于不同实例，不能直接合并、移动或删除；先核实实例配置。
- `backend/static/` 与 `deploy/fnos/install.html` 是需要交付的生成产物，必须通过前端构建更新，不能当作临时文件清理。

## 开发与验证

需要 Go 1.23+，建议 Node.js 22.12+。源码启动步骤见本节。`scripts/start-dev.bat` 是带固定本机路径及 Aria2 配置的旧开发脚本，不是通用开箱启动器或隔离测试命令。不要直接运行它来验证现有实例。

前端资源在 Go 构建时通过 `go:embed` 嵌入，运行程序不依赖当前目录下的 `static/`。修改前端后先构建，再重新编译后端；不能只替换已运行 exe 旁的 HTML 来更新界面。

首次开发在 `web/` 执行 `npm ci`、`npm run build`。随后在 `backend/` 用独立目录启动（PowerShell）：

```powershell
$env:DB_PATH = Join-Path $PWD 'data/auto-anime.db'
$env:DOWNLOAD_DIR = Join-Path $PWD 'data/downloads'
$env:COVERS_DIR = Join-Path $PWD 'data/covers'
$env:DOWNLOAD_MODE = 'direct'
$env:UPDATE_CHECK = '0'
go run .
```

Linux 可在 `backend/` 执行：

```bash
DB_PATH="$PWD/data/auto-anime.db" DOWNLOAD_DIR="$PWD/data/downloads" COVERS_DIR="$PWD/data/covers" DOWNLOAD_MODE=direct UPDATE_CHECK=0 go run .
```

第二终端在 `web/` 运行 `npm run dev`，访问 `http://127.0.0.1:5173`，API 默认转发到 8000。确认端口未被已有实例使用；这只是开发启动，不是隔离自动化测试。运行测试只使用临时数据库和模拟服务。

后端修改先对修改的文件逐个执行 `gofmt -w 文件路径`，然后在 `backend/` 执行：

```bash
CGO_ENABLED=0 go test ./... -count=1 -timeout=240s
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go build ./...
```

PowerShell 中先执行 `$env:CGO_ENABLED = '0'`，再运行对应 Go 命令。

前端修改在 `web/` 执行；首次运行或锁文件变化后先 `npm ci`：

```powershell
$frontendTests = @(rg --files src -g '*.test.mjs')
node --test @frontendTests
npm run build
```

Linux 可使用 `node --test $(find src -name '*.test.mjs')`。前端没有 `npm test` 脚本。提交前端修改时同时交付生成的 `backend/static/`；安装配置页变更还需交付生成的 `deploy/fnos/install.html`。

辅助工具测试在仓库根目录运行：

```bash
python -m unittest discover -s tools -p 'test_*.py'
```

容器配置修改使用 `docker compose config` 检查。界面修改应验证桌面、手机、亮暗主题、中文及加载/空/失败状态。只能报告本次实际执行的验证；编译通过不等于容器或 NAS 实机验收。

## 数据与安全

- 自动化测试使用临时数据库、临时目录、独立端口与本地模拟服务，不连接真实下载器或操作现有媒体。
- 不提交数据库、媒体、真实 RSS 令牌、下载器密码、Webhook 凭据或代理凭据；截图、日志和 JSON 备份也需要检查。
- 文本使用 UTF-8 无 BOM，保留已有修改，生成产物通过构建更新。
- 功能、配置或默认值发生变化时同步更新 README 与相关用户文档。
- 提交前检查 `git status --short`、`git diff --check` 与实际差异。忽略规则不是凭据扫描器；仓库转公开前还应检查 Git 历史、Release 附件和工作流日志。

请仅使用有权访问、保存和分发的测试资源。第三方代码、界面或素材须遵守相应授权。

## 源码镜像构建

完整仓库根目录执行：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

构建使用已生成的 `backend/static/`，修改前端后先执行前端构建。需要获取基础镜像、Go 依赖和 Alpine 软件包；运行镜像不需要 Node.js 或 Python。默认版本为 `dev`，不支持一键更新，更新源码后重新构建。

根 `.dockerignore` 仅允许 Dockerfile 所需输入进入上下文。发布流程另建 `image-context/`；变更构建输入须同步调整对应规则。发布步骤见[维护者发布说明](docs/RELEASING.md)。
