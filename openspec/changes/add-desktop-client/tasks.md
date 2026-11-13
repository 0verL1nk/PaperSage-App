# add-desktop-client — 任务拆解

## 1. OpenSpec

- [x] 1.1 写 `proposal.md`（Why / What Changes / Capabilities / Impact / Out of Scope）
- [x] 1.2 写 `design.md`（Context / Goals / Decisions / Repository Layout / Rollout）
- [x] 1.3 写 `tasks.md`（本文件）
- [x] 1.4 写 `specs/desktop-shell/spec.md`
- [x] 1.5 写 `specs/desktop-native-integration/spec.md`
- [x] 1.6 写 `specs/desktop-local-workspace/spec.md`
- [x] 1.7 写 `specs/desktop-distribution/spec.md`
- [x] 1.8 `openspec validate add-desktop-client` 通过

## 2. 桌面模块骨架

- [x] 2.1 新建 `desktop/go.mod`（独立模块，Go 1.26+，依赖 wails v3.1.0）
- [x] 2.2 新建 `desktop/main.go`：`application.New` + 主窗口 + `Run`
- [x] 2.3 新建 `desktop/internal/assets/assets.go`：嵌入 `dist` 并提供 SPA 回退
- [x] 2.4 新建 `desktop/internal/app/app.go`：窗口/菜单/托盘装配入口
- [x] 2.5 `.gitignore` 忽略 `desktop/internal/assets/dist/*`，保留 `.gitkeep`
- [x] 2.6 `desktop/README.md` 记录环境要求（Go 1.26+、WebView2、wails3 CLI）

## 3. 构建流水线

- [x] 3.1 `desktop/Taskfile.yml`：`dev`、`build`、`package` 任务
- [x] 3.2 资源复制任务：`frontend/dist` → `desktop/internal/assets/dist`
- [x] 3.3 Makefile 新增 `desktop-dev` / `desktop-build`
- [x] 3.4 确认开发模式加载 Vite dev server；不可行则明确回退为静态构建
- [x] 3.5 构建产物落在 `desktop/bin/`，加入 `.gitignore`

## 4. 外壳能力

- [x] 4.1 单实例：`SingleInstanceOptions` + 第二次启动聚焦窗口
- [x] 4.2 窗口状态：尺寸/位置/最大化持久化与越界回退
- [x] 4.3 应用菜单：macOS 原生菜单；Windows 使用自定义标题栏菜单（全屏/检查更新/服务器/退出）
- [x] 4.4 托盘：显示/隐藏、退出；关闭窗口隐藏到托盘；托盘图标使用品牌 icon
- [x] 4.5 启动恢复：前端记录并在有效时恢复上次项目路由
- [x] 4.6 向前端暴露 `isDesktop`、版本、平台

## 5. 原生集成

- [x] 5.1 `DesktopService`：应用信息、通知、打开外部链接、文件/文件夹/保存对话框、在文件管理器中显示
- [x] 5.2 外部链接拦截：仅 `http/https/mailto`，其余拒绝
- [x] 5.3 深链解析：`papersage://project/<id>/<stage>` 白名单校验并路由（启动参数、二次启动与 NSIS 协议注册均已接入）
- [x] 5.4 编译完成/失败通知（窗口未聚焦时）
- [x] 5.5 Agent Run 完成/失败通知（窗口未聚焦时）
- [x] 5.6 拖放目标：写作页与参考资料页支持拖入文件，复用现上传流程

## 6. 本地工作区

- [x] 6.1 导入文件夹：原生选择器 + 类型过滤 + 复用文档上传 API
- [x] 6.2 导出项目：拉取 LaTeX 文件，`jszip` 打包后经原生保存对话框写出（参考资料尚未纳入）
- [x] 6.3 在文件管理器中显示当前项目/导出目录（导出成功提示可直接打开所在文件夹）
- [x] 6.4 API 目标持久化沿用现有切换器；“关于”显示当前生效地址
- [ ] 6.5 离线/不可达错误给出明确提示，不静默失败

## 7. 前端适配

- [x] 7.1 新增 `frontend/src/lib/desktop.ts`：桌面环境检测与原生能力的安全封装
- [x] 7.2 菜单触发的动作（新建/导入/导出）与页面内按钮复用同一服务函数
- [x] 7.3 设置页“关于”展示客户端版本、平台、API 地址
- [x] 7.4 浏览器环境保持现有行为，不出现桌面专属死按钮

## 8. 交付与自动更新

- [x] 8.1 从 `build/brand/appicon.svg` 生成分级 `appicon.png` / `.ico` / `.icns`，托盘用独立简化图形
- [x] 8.2 Windows 版本信息与安装包（NSIS）
- [x] 8.3 版本号通过 ldflags 注入，与发布标签一致
- [x] 8.4 接入 `app.Updater` + GitHub Releases provider（仓库 `0verL1nk/GraduateProject`）
- [x] 8.5 应用菜单“检查更新”；不在启动时自动弹窗打扰
- [ ] 8.6 下载进度、校验失败、重启提示的更新界面与事件
- [x] 8.7 Release 资产命名（`papersage-windows-amd64.exe` 等）与 `SHA256SUMS`
- [x] 8.8 确保更新器选中应用二进制而不是 `*-installer.exe`（Wails 默认匹配器显式排除 installer）
- [x] 8.9 可选：tag 触发的桌面构建与 Release 上传 workflow（Windows/macOS/Linux 矩阵）

## 9. 本地运行时

- [x] 9.1 构建时复制后端二进制到客户端 `backend/` 目录（随包分发，运行时不再解压）
- [x] 9.2 启动时定位同目录 `backend/papersage[.exe]`，缺失则明确报错
- [x] 9.3 端口选择：优先 5478，探测已有实例，必要时选择空闲端口并注入前端
- [x] 9.4 子进程生命周期：启动、日志落盘、优雅退出、异常退出退避重启
- [ ] 9.5 后端版本不匹配时拒绝本地模式并给出升级提示
- [x] 9.6 确认 WebView2 下现有 WASM 编译链路（Worker/WASM/资源请求）可用
  - 已验证（2026-11-08）：catalog 随安装包分发（`LATEX_RESOURCE_CATALOG_FILE`），资源经同源 `/texlive/*` 代理按需从 CTAN 镜像拉取；真实窗口内 ctexart + XeLaTeX 编译产出 1 页 PDF，网络请求仅 `wails.localhost`。
- [x] 9.7 明确不检测、不调用本机 TeX；编译只走自带 WASM（或已配置的 CLSI）
- [x] 9.8 本地数据库、日志目录的规范位置与清理策略（README「Runtime data」）

## 10. 本地目录同步

- [x] 10.1 项目绑定/解绑本地文件夹，持久化映射与上次同步哈希（引擎层）
- [x] 10.2 初始同步：空文件夹自动导出项目，非空文件夹导入本机新增/修改
- [x] 10.3 应用保存 → 防抖写入绑定目录（编辑器保存后写回绑定目录，冲突不覆盖）
- [x] 10.4 磁盘变更 → `fsnotify` 防抖 → 现有 LaTeX API 写回项目（fsnotify 监听后自动导入项目）
- [x] 10.5 冲突检测：两侧都相对上次同步变化时暂停并让用户选择，不自动覆盖（引擎层）
- [x] 10.6 删除/重命名只提示、不自动传播（引擎层）
- [x] 10.7 文件类型白名单与忽略规则（`.aux/.log/.out/.toc/.synctex*/.pdf` 等）
- [x] 10.8 路径安全：相对路径、拒绝 `..`/绝对路径/越界符号链接，限制文件大小与数量
- [x] 10.9 离线待同步队列与恢复连接后的顺序重试
  - 暂停或写入失败的项目保存按顺序进入持久化队列（localStorage），文件树标记「云端改动」；
    恢复监听、重新打开写作页或点「立即同步」时按入队顺序补写，成功出队、冲突保持待处理。
    真机验证：暂停后编辑产生队列与蓝色圆点，恢复监听后队列清空并写回磁盘。
- [x] 10.10 同步状态展示：已同步/本地改动/云端改动/冲突/暂停，支持手动全量比对
  - 文件树按需显示圆点（本机新增/修改/删除/重命名、云端待写入、冲突），常规状态不打扰；
    同步对话框新增「全量比对」，逐文件列出状态；真机验证：盘上改动出现「本机修改」圆点与比对行，暂停后状态转为「已暂停」。

## 11. 验收

- [x] 11.1 冷启动自动拉起本地后端，登录后进入项目列表
- [ ] 11.2 本地后端崩溃后自动恢复；退出客户端后无残留进程
- [x] 11.3 WASM 编译在 WebView2 下成功，失败日志可读（资源网关修复后已产出 1 页 PDF）
- [x] 11.4 本地目录双向同步：应用改动落盘、磁盘改动回写、冲突不覆盖（离线队列未做）
- [x] 11.5 Monaco 编辑、PDF 预览在 WebView2 下正常
- [x] 11.6 单实例、窗口状态、托盘、菜单快捷键逐项手测
- [ ] 11.7 导入/导出、通知、深链、外部链接、自动更新逐项手测
- [x] 11.8 `pnpm typecheck` / `pnpm lint` / `pnpm test` 通过
- [ ] 11.9 安装包在干净 Windows 用户环境可安装、启动、自动更新、卸载
