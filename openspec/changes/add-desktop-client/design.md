# add-desktop-client — 技术设计

## Context

- 前端是 React 19 SPA，核心能力依赖 Web 生态：Monaco/CodeMirror、PDF.js、Radix、TanStack Router/Query。
- 后端是 API-only Go 单二进制，CORS 为 `AllowAllOrigins`，鉴权用双 JWT。
- 前端已有远程/本地 API 切换（`VITE_API_BASE_URL` + `VITE_LOCAL_API_BASE_URL` + localStorage 覆盖）。
- 现有业务 API 已覆盖项目、文档上传/预览、LaTeX 文件读写、编译与 Agent 运行，桌面端不需要新增后端接口。

## Goals

1. 让 PaperSage 以原生桌面应用形态运行，并具备客户端应有的行为，而不是“去掉浏览器地址栏的网页”。
2. 复用现有前端与后端契约，避免第二套业务实现。
3. 构建、运行、排障路径清晰，仓库结构符合 Wails v3 与 Go 项目惯例。
4. 原生能力有明确的安全边界。

## Non-Goals

- 不重写 UI，不引入第二套设计系统。
- 不在桌面端实现离线数据库或本地优先同步。
- 不承诺移动端。

## Decisions

### D1. 桌面框架选 Wails v3.1

- 复用现有 Web 前端，改造成本最低。
- 相比 MyGo（2026-09 建仓、0.x）生态与稳定性更好；相比 GPUI 不需要重写 Monaco/PDF/Radix；相比 Tauri 保持纯 Go 栈。
- 代价：引入 Rust 之外的 Wails 工具链；WebView2 行为需逐项验证。

### D2. 独立 Go 模块

`desktop/` 使用独立 `go.mod`，不把 Wails 依赖带进 `server/`。桌面端只消费 HTTP API，不 import 服务端内部包。

### D3. 前端复用与资源嵌入

- 生产：`frontend` 执行 `pnpm build`，产物复制到 `desktop/internal/assets/dist`，由 `//go:embed` 嵌入并作为 `AssetServer` 根目录。
- 复制而不是相对路径引用：`go:embed` 不能跨模块/父目录，Windows 下也不依赖符号链接。
- `desktop/internal/assets/dist` 是生成物，加入 `.gitignore`，仅保留 `.gitkeep`。
- 开发：Wails 加载 Vite dev server；若代理配置不稳定，回退为“构建后加载本地资源”。以实际跑通为准，不引入第二套前端构建。

### D4. API 目标

沿用现有机制，不新增配置系统：

- 构建时 `VITE_API_BASE_URL` 指向线上 API。
- 运行时用户可在既有切换器里选“远程/本地”，localStorage 覆盖优先。
- 桌面端在“关于”里显示当前生效的 API 地址，便于排障。

### D5. 原生能力通过小型 DesktopService 暴露

Go 侧只提供前端无法完成的能力：

- 应用信息（版本、平台、是否为桌面端）
- 打开系统文件/文件夹选择器、保存对话框
- 在文件管理器中显示路径
- 发送系统通知
- 用系统浏览器打开外部链接

业务操作（上传、导出内容生成）仍走现有前端服务层，避免在 Go 侧复制业务逻辑。

### D6. 窗口与托盘行为

- 单实例：第二次启动唤醒并聚焦已有窗口，参数交给已有实例。
- 窗口状态：尺寸、位置、最大化写入系统配置目录的 JSON；显示器变化导致位置越界时回退居中。
- 关闭按钮隐藏到托盘；退出由托盘菜单或应用菜单触发。
- 托盘菜单：显示/隐藏、退出。长任务状态通过通知而非托盘气泡表达，减少平台差异。

### D7. 通知

- 仅在窗口未聚焦时发送，避免前台重复打扰。
- 触发点：编译完成/失败、Agent Run 完成/失败。
- 同一任务去重，失败与成功各一次。

### D8. 导入与导出

- 导入文件夹：前端用原生文件夹选择器拿到路径，遍历允许的文件类型，复用现有文档上传 API 与进度 UI。
- 导出项目：前端通过现有 API 拉取 LaTeX 文件与参考资料预览，使用已有 `jszip` 打包，再通过原生保存对话框落盘。
- 不新增后端导出接口；不在 Go 侧解析文件内容。

### D9. 安全边界

- 外部链接仅允许 `http`、`https`、`mailto`；其他协议拒绝并提示。
- 深链只接受 `papersage://project/<id>[/<stage>]`，ID 与阶段做白名单校验；非法输入忽略。
- 单实例传递的参数与拖入的文件路径都按不可信输入处理，路径只用于用户明确发起的操作。
- 不在 WebView 中启用 Node/文件系统直通；所有本地能力经 Go 服务显式暴露。

### D10. 菜单与快捷键

- 文件：新建项目、导入文件夹、导出项目、退出。
- 编辑：撤销/重做/剪切/复制/粘贴/全选（交给 WebView）。
- 视图：重新加载、显示/隐藏窗口、开发者工具（仅开发构建）。
- 帮助：关于、打开文档。
- Windows 用 Ctrl，macOS 用 Cmd；不与编辑器已有快捷键冲突。

### D11. 自动更新走 GitHub Releases

- 使用 Wails v3 内置 `app.Updater` 与 `pkg/updater/providers/github`，仓库为 `0verL1nk/GraduateProject`。
- 启动后延迟检查一次，帮助菜单提供“检查更新”；检查失败不阻塞启动、不反复打扰。
- Release 资产按 `papersage-windows-amd64.exe`、`papersage-darwin-arm64`、`papersage-linux-amd64` 命名；安装包命名为 `*-installer.exe`，Wails 默认匹配器会自动排除安装包，避免用安装器替换正在运行的程序。
- 同时发布 `SHA256SUMS`，更新器下载后校验摘要；校验失败中止更新并保留当前版本。
- 更新由用户确认后下载，下载完成提示重启；重启由更新器原子替换并拉起新版本。不静默安装。
- 版本号通过 ldflags 注入，正式发布只从干净工作区按标签构建。

### D12. 品牌图标与分级导出

- 品牌源文件是 `desktop/build/brand/appicon.svg`（深色底“纸→笔尖”）；`appicon-small.svg` 是 ≤32px 的去细节版本；`tray.svg` 是独立托盘图形。
- `frontend/public/favicon.svg` 同步为同一个 `appicon.svg`，保证 Web 与客户端一致。
- Windows `.ico` 分级导出：16/20/24/32 用简化版，40/48/64/128/256 用完整版；由 `build/brand/gen-icons.py` 组装。
- 托盘只导出 16/20，去掉文字条、折角、泄气孔与细描边，保留奶油色纸页、金色笔尖和 1px 靛蓝外轮廓。
- 不再在构建时用单一 PNG 重新生成 `.ico`，避免覆盖分级结果；图标作为已提交产物，源 SVG 变更时手动重跑脚本。

### D13. 本地后端进程由客户端托管

- 安装包附带与客户端同版本的 `papersage` 后端二进制；首次启动定位或提取到应用数据目录。
- 启动顺序：单实例锁 → 检查版本 → 选择端口 → 启动子进程 → 轮询 `/health` → 前端可用。
- 端口优先 `5478`；被占用时先探测是否为本项目已运行的后端，是则复用，否则选择空闲端口并把实际地址注入前端。
- 子进程固定使用应用数据目录下的 SQLite 数据库与日志目录；`LATEX_CLSI_ENABLED=false`，只监听 `127.0.0.1`。
- 客户端退出时优雅停止子进程（超时后强制结束）；子进程异常退出时有限次数退避重启，超过上限提示用户并回退远程模式。
- 后端版本与客户端不一致时拒绝启动本地模式，避免 schema 漂移；提示用户更新。
- 前端 API 地址来源优先级：用户显式自定义地址 > 用户显式选择远程 > 客户端托管的本地后端。

### D14. 编译继续使用自带 WASM，不依赖本机 TeX

- 不假设用户安装了 TeX，也不检测或调用 `latexmk`、TeX Live、MiKTeX。用户本机装了什么、宏包是否齐全都无法保证，把编译正确性押在用户环境上不可接受。
- v1 的编译路径保持不变：浏览器 WASM 引擎为主，CLSI 仍是可选的服务端编译器。
- 自带完整 TeX 发行版会显著增加安装包体积（数百 MB 到 GB），并带来许可证、更新和平台差异问题；v1 不捆绑。未来若需要，应作为独立可选组件下载，而不是默认依赖。
- 因此本地编译不需要额外的 Go 编译服务；桌面端只负责让 WASM 在 WebView2 中可靠运行。

### D15. 本地项目目录双向同步

- 每个项目可选绑定一个本地文件夹；绑定关系与同步元数据持久化在应用数据目录。
- v1 同步范围只包含 LaTeX 源文件：`.tex`、`.bib`、`.cls`、`.sty`、`.bst` 和项目内引用的图片资源。参考文献 PDF 与二进制附件不同步，避免体积和二进制冲突。
- 初始同步：比较文件夹与项目文件集合，用户选择“以项目为准导出”“以文件夹为准导入”或“逐个确认”，不默认覆盖任何一侧。
- 应用 → 磁盘：LaTeX 文件保存成功后防抖写入绑定目录。
- 磁盘 → 应用：`fsnotify` 监听绑定目录，防抖后通过现有 LaTeX 文件 API 写回项目。
- 冲突判定：记录上次同步的内容哈希；若两侧哈希都相对上次变化，则暂停该文件并提示用户选择保留哪一侧或打开对比，不自动覆盖。
- 删除不自动传播：任一侧删除只产生提示，由用户确认后再执行另一侧删除，避免误删。
- 忽略规则：`.aux`、`.log`、`.out`、`.toc`、`.synctex*`、`.pdf` 等编译产物与临时文件不参与同步。
- 路径安全：只接受项目内相对路径；拒绝绝对路径、`..` 穿越和指向根目录外的符号链接；限制单文件大小与文件数量。
- 离线：磁盘改动进入待同步队列，恢复连接后按顺序写回；冲突仍按上述规则处理。
- 状态展示：每个绑定显示“已同步 / 本地有改动 / 云端有改动 / 冲突 / 暂停”，并允许手动触发一次全量比对。

### D16. 发布去符号与密钥边界

- 桌面壳与内置后端的 release 构建使用 `-trimpath` 与 `-ldflags "-s -w"` 去符号；前端生产构建关闭 source map 并保留默认 minify。
- `garble` 仅作为手动 opt-in：实测 v0.18 的产物（`-tiny`、`-literals`、默认模式）在 Windows Defender 下链接阶段即被判为病毒，因此默认发布链不启用混淆，避免用户安装即被杀软隔离。
- 内置后端固定放在 `bin/backend/papersage[.exe]`，安装后为 `$INSTDIR\backend\`，避免与桌面壳同名。
- 长期密钥（模型、嵌入、JWT、S3）只存在于服务端环境或用户自己的设置中；本地后端从用户设置/应用数据目录读取，不接受前端传入的密钥。
- 完整性依赖自动更新的 `SHA256SUMS` 与后续代码签名；不引入 anti-debug、反虚拟机或多层加壳。
- 发布前对桌面壳、内置后端和前端产物做一次字符串审计，确认无密钥和内网地址。

## Repository Layout

```text
desktop/
├── go.mod                       # 独立模块：github.com/0verL1nk/GraduateProject/desktop
├── main.go                      # 进程入口：装配 application 并 Run
├── internal/
│   ├── app/                     # 窗口、菜单、托盘、单实例、生命周期
│   ├── assets/                  # embed frontend/dist（生成物，git 忽略）
│   ├── backend/                 # 本地后端定位、启动、健康检查、退出
│   ├── foldersync/              # 本地目录绑定、监听、冲突与离线队列
│   ├── deeplink/                # papersage:// 解析与校验
│   ├── services/                # DesktopService：对话框、通知、外部链接、应用信息
│   └── windowstate/             # 窗口尺寸/位置持久化
├── bin/                         # 构建产物（git 忽略），含后端二进制副本
│   └── papersage.exe            # 由构建任务从 server 复制，随安装包分发
├── build/
│   ├── brand/
│   │   ├── appicon.svg          # 品牌源：深色底“纸→笔尖”
│   │   ├── appicon-small.svg    # ≤32px 去细节版
│   │   ├── tray.svg             # 独立托盘图形
│   │   └── gen-icons.py         # 组装分级 .ico 与托盘 PNG
│   ├── appicon.png              # 1024 源图
│   └── windows/                 # icon.ico、info.json、manifest
├── Taskfile.yml                 # Wails v3 构建任务
└── README.md                    # 开发、构建、排障说明
```

约束：

- 业务逻辑不进入 `desktop/`；它只做壳与原生桥接。
- 前端改动集中在 `frontend/src/lib/desktop.ts`、设置页“关于”和少量事件触发点。
- 新增文件按职责分层，不建“utils.ts”万能文件。

## Frontend Integration Points

已核对当前代码，桌面能力按以下位置接入：

| 能力 | 接入点 |
|---|---|
| 编译终态与通知 | `frontend/src/hooks/use-compile-polling.ts:42`（终态判定）、`frontend/src/views/projects/WritingPage.tsx:230-256`（成功/失败处理） |
| Agent Run 终态与通知 | `frontend/src/store/chat-store.ts:168-191`（error/abort/finish） |
| 设置页“关于/客户端” | `frontend/src/views/settings/SettingsPage.tsx:30-45`（Tabs） |
| 启动恢复路由 | `frontend/src/app/router.tsx:46/262/282`；`frontend/src/routes/__root.tsx` 是未引用的死代码，不在此处实现 |
| 文件拖放挂载点 | `frontend/src/views/uploading/UploadDropzone.tsx:173`、`UploadingPage.tsx:224-252`（当前没有窗口级 drop handler） |
| 本地目录同步 | LaTeX 保存入口 `frontend/src/hooks/use-latex.ts`、`frontend/src/services/latex-api.ts:91/97/209`；状态 UI 挂在项目页或设置页 |
| 导出数据来源 | `frontend/src/services/latex-api.ts:91/97/209/230`、`frontend/src/services/files-api.ts:9/29`；服务端没有项目导出接口，导出在客户端用 `jszip` 完成 |
| API 目标切换 | `frontend/src/components/shared/api-target-toggle.tsx:40`、`frontend/src/lib/env.ts:32/40/74/80` |
| 桌面环境检测 | 当前不存在；新增 `frontend/src/lib/desktop.ts` 作为唯一入口 |

## Rollout

1. 骨架：应用启动、加载前端、完成登录与项目列表。
2. 外壳：单实例、窗口状态、菜单、托盘。
3. 本地运行时：本地后端托管、WASM 编译路径确认、本地目录双向同步。
4. 原生集成：对话框、拖放、通知、深链、外部链接。
5. 本地工作区：文件夹导入、项目导出、文件管理器定位。
6. 交付：Windows 安装包、版本信息、GitHub Releases 自动更新、CI 构建。

每阶段都可独立验收，不要求一次性全部落地。

## 后续能力路线（非 v1 验收）

以下能力方向已确认可行，按阶段排入；每项在独立变更中落地，不阻塞桌面客户端首发。

### v1.1（体验补齐）

- 本地缓存与崩溃恢复：项目元数据、最近文档、编辑器草稿本地落盘，进程异常退出后可恢复。
- 本地版本快照：定期快照 LaTeX 源文件，支持回看与恢复。
- Token 存系统凭据库：Windows Credential Manager / macOS Keychain，替代 localStorage。
- 任务栏进度与最近项目：编译/Agent 状态、Jump List / macOS Recents。
- 系统打印与分享：调用系统打印对话框打印 PDF，调用系统分享面板导出。
- 一键诊断：导出日志、版本、WebView 版本、API 地址。
- 启动性能：窗口先出现、内容后加载，缩短冷启动感知时间。

### v2（深度集成）

- 多窗口：一个项目一个窗口、PDF 独立窗口、分屏与专注模式。
- 全局快捷键与快速捕获：任意界面唤起窗口、新建项目或速记。
- 剪贴板增强：粘贴表格清洗、图片粘贴、公式复制为 LaTeX/图片。
- 文件关联：双击 `.tex` / `.bib` / `.papersage` 用客户端打开。
- 系统主题、强调色、高对比度与缩放跟随。
- 代理与自定义 CA，适配校园网/企业网。
- 更新通道（stable/beta）与回滚。
- 本地全文搜索与最近项目索引。

### 明确不做

- 移动端（Wails v3 的 iOS/Android 为实验性，iOS 需要 macOS）。
- 自建同步协议（服务端仍是真相源）。
- 原生重写编辑器（Monaco 已满足需求）。
- OCR/截图识别公式（收益低、复杂度高）。
- 自带完整 TeX 发行版（体积与维护成本过高，v1 继续用 WASM）。

## Open Questions

- 生产构建默认指向线上 API 还是 `localhost:5478`（建议线上，首次启动允许用户切换）。
- 关闭到托盘是否需要在设置页提供开关（建议二期，v1 固定行为并在托盘菜单提供退出）。
- 导出是否包含最近一次编译产物 PDF（建议默认不包含，避免体积和一致性争议）。
- 更新渠道是否只取正式版（建议只取 stable，预发布版本不推送给普通用户）。
