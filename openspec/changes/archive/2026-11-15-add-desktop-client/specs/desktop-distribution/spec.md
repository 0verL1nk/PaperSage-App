## ADDED Requirements

### Requirement: 可重复构建
项目 SHALL 提供单一命令完成桌面客户端构建：构建前端产物、复制到桌面模块、编译并输出安装包。构建过程 SHALL NOT 依赖手工复制文件或修改生成物；重复执行应得到等价结果。

#### Scenario: 干净环境构建
- **WHEN** 开发者在具备 Go、Node/pnpm、WebView2 的干净环境中执行桌面构建命令
- **THEN** 命令一次完成前端构建、资源嵌入、Go 编译与安装包输出

#### Scenario: 前端变更后重建
- **WHEN** 开发者修改前端并再次执行构建命令
- **THEN** 新前端产物被打包进客户端，无需手工清理旧 `dist`

### Requirement: 版本元数据
客户端 SHALL 在构建时注入版本号，并在“关于”界面展示版本、平台与构建时间。安装包和可执行文件的版本信息 SHALL 与发布标签一致。

#### Scenario: 关于页显示版本
- **WHEN** 用户打开帮助菜单中的“关于”
- **THEN** 显示客户端版本、平台和当前 API 地址

#### Scenario: 发布标签一致
- **WHEN** 使用标签 `v0.2.0` 构建发布包
- **THEN** 可执行文件与安装包的版本信息均为 `0.2.0`

### Requirement: Windows 安装包
项目 SHALL 产出 Windows 安装包，包含应用图标、开始菜单入口和卸载程序。安装与卸载 SHALL 不依赖管理员权限以外的额外手工步骤。

#### Scenario: 安装并启动
- **WHEN** 用户在干净 Windows 环境运行安装包并按默认选项安装
- **THEN** 安装完成，开始菜单出现入口，启动后进入应用

#### Scenario: 卸载
- **WHEN** 用户从系统“应用和功能”卸载客户端
- **THEN** 程序文件被移除，用户数据目录按约定保留或提示清理

### Requirement: GitHub Releases 自动更新
客户端 SHALL 通过 GitHub Releases 检查、下载并安装新版本。检查 SHALL 在启动后延迟执行且不阻塞使用，帮助菜单 SHALL 提供手动“检查更新”。发现新版本后 SHALL 展示版本与发行说明，由用户确认后下载；下载完成 SHALL 校验摘要并在用户确认后重启应用。客户端 SHALL NOT 静默安装或未经确认重启。

#### Scenario: 启动时发现新版本
- **WHEN** 客户端启动且 GitHub 上存在高于当前版本的正式发布
- **THEN** 非阻断地提示新版本，用户可查看说明、下载或忽略

#### Scenario: 用户确认下载并重启
- **WHEN** 用户确认下载更新，下载与校验完成
- **THEN** 提示重启，用户确认后替换当前版本并启动新版本

#### Scenario: 校验失败
- **WHEN** 下载文件的摘要与发布资产中的 `SHA256SUMS` 不一致
- **THEN** 中止更新，保留当前版本继续运行并提示失败原因

#### Scenario: 检查失败不影响使用
- **WHEN** 更新检查遇到网络错误或 GitHub 不可达
- **THEN** 不阻塞启动，不反复弹窗，用户可在帮助菜单手动重试

#### Scenario: 不把安装包当作应用二进制
- **WHEN** Release 同时包含 `papersage-windows-amd64.exe` 与 `papersage-windows-amd64-installer.exe`
- **THEN** 更新器选择应用二进制，忽略安装包

### Requirement: 发布产物去符号与密钥边界
发布构建 SHALL 对桌面壳和内置后端使用 `-trimpath -ldflags "-s -w"` 去符号；前端生产构建 SHALL 关闭 source map 并保留 minify。发布产物 SHALL NOT 包含 `AGENT_API_KEY`、`EMBED_API_KEY`、`SECRET_KEY`、S3 凭据或其他长期密钥；本地后端 SHALL 从用户设置/环境读取配置，不得接受前端传入的密钥。`garble` 混淆 MAY 作为手动构建选项，但 SHALL NOT 作为 Windows 默认发布路径：实测其产物会被 Windows Defender 误判为病毒。

#### Scenario: 发布构建去符号
- **WHEN** 维护者按标签构建桌面产物
- **THEN** 桌面壳与 `backend/` 下的后端二进制均使用 `-trimpath` 与 `-s -w`

#### Scenario: 前端无 source map
- **WHEN** 构建前端生产产物
- **THEN** 产物目录不包含 `.map` 文件或 `sourcesContent`

#### Scenario: 密钥扫描
- **WHEN** 发布前对桌面壳、内置后端和前端产物执行字符串审计
- **THEN** 不出现 `sk-`、`api_key`、`secret` 等长期密钥或内网地址

#### Scenario: 混淆不作为 Windows 默认
- **WHEN** CI 构建 Windows 发布产物
- **THEN** 默认不调用 garble；需要混淆时由维护者手动开启并接受杀软误报风险

### Requirement: 发布资产命名与校验
每个正式发布 SHALL 提供按平台和架构命名的应用二进制、Windows 安装包和 `SHA256SUMS`。版本号、资产文件名和校验文件 SHALL 来自同一次构建。

#### Scenario: 发布资产完整
- **WHEN** 维护者发布标签 `v0.2.0`
- **THEN** Release 包含对应平台的二进制、安装包与 `SHA256SUMS`，版本信息均为 `0.2.0`

### Requirement: 发布构建可追踪
桌面发布 SHALL 与源码版本可对应。发布产物 SHALL 记录所用提交或标签，禁止用未提交的本地改动产出正式安装包。

#### Scenario: 正式发布
- **WHEN** 维护者从干净工作区按标签执行发布构建
- **THEN** 产物版本信息与标签一致，构建记录包含对应提交
