## ADDED Requirements

### Requirement: Critical user paths covered by E2E
项目 SHALL 在 `frontend/tests/e2e/` 提供 9 个 Playwright spec,覆盖关键用户路径,所有 spec 在 CI 上 PASS。

#### Scenario: Auth roundtrip
- **WHEN** 运行 `auth/auth-flow.spec.ts`
- **THEN** 包含:register → 自动跳 login → login → 进 /app/projects → logout → 再 login → 401 拦截

#### Scenario: Project golden path
- **WHEN** 运行 `projects/project-lifecycle.spec.ts`
- **THEN** 包含:create project → upload doc → write file → compile trigger → delete → restore from trash

#### Scenario: Project group filter
- **WHEN** 运行 `projects/group-filter.spec.ts`
- **THEN** 包含:建两个 group,在每个下建项目,filter chip 切换可见列表

#### Scenario: Document upload
- **WHEN** 运行 `uploading/document-upload.spec.ts`
- **THEN** 包含:真 multipart 上传 sample.pdf → 出现在 list → 拖到 folder → preview 按钮可用

#### Scenario: Compile polling
- **WHEN** 运行 `writing/compile-polling.spec.ts`
- **THEN** 包含:点击 compile → 看到 "pending" 状态 → poll 转到 "success" → 看到 PDF 链接

#### Scenario: Folder operations
- **WHEN** 运行 `writing/folder-ops.spec.ts`
- **THEN** 包含:create folder → rename via F2 → recursive delete via confirm dialog

#### Scenario: Compile trigger
- **WHEN** 运行 `writing/compile-trigger.spec.ts`
- **THEN** 包含:点击 compile 按钮 → 看到 toast 错误(LATEX_CLSI_ENABLED=false 模拟 503)

#### Scenario: File tree interaction
- **WHEN** 运行 `writing/file-tree.spec.ts`(从现有 file-tree.spec.ts 迁入)
- **THEN** 包含:search filter → 无匹配空状态 → clear button → F2 rename → Delete 键删除

#### Scenario: Settings persistence
- **WHEN** 运行 `settings/settings-persistence.spec.ts`
- **THEN** 包含:改 theme 到 dark → reload page → 仍为 dark;改 editor font_size → API 持久化

#### Scenario: Cross-route navigation
- **WHEN** 运行 `navigation/cross-route.spec.ts`
- **THEN** 包含:projects → writing → settings → trash → 回 projects 路径正确;未登录直链 /app/projects 跳 /auth

#### Scenario: 401 redirect
- **WHEN** 运行 `error-states/401-redirect.spec.ts`
- **THEN** 包含:注入坏 access_token → 任何 API 调用返回 401 → axios 拦截器清 session → 跳 /auth

### Requirement: Spec file structure organized by domain
`frontend/tests/e2e/` SHALL 按用户域组织,每个 spec 文件归属唯一目录。

#### Scenario: 7 domain directories
- **WHEN** 列 `frontend/tests/e2e/`
- **THEN** 包含子目录:`auth/`、`projects/`、`uploading/`、`writing/`、`settings/`、`navigation/`、`error-states/`,以及 `helpers.ts` 和 `fixtures/`

#### Scenario: Each spec under one domain
- **WHEN** 列任意 spec 文件
- **THEN** 文件名以域目录为前缀,如 `auth/auth-flow.spec.ts`,不存在散落的 spec

### Requirement: Shared helpers for common patterns
`tests/e2e/helpers.ts` SHALL 提供 `loginAs`、`logout`、`seedAuthTokens`、`uploadFixture`、`apiMock` 五个工厂函数,供所有 spec 复用。

#### Scenario: loginAs walks real form
- **WHEN** 调 `loginAs(page, 'e2euser', 'e2epass1')`
- **THEN** 填表 + 提交,等 `/app/projects` URL 出现,不直接塞 localStorage

#### Scenario: uploadFixture uses setInputFiles
- **WHEN** 调 `uploadFixture(page, 'sample.pdf')`
- **THEN** 找到 `input[type=file]`,调 `setInputFiles('frontend/tests/e2e/fixtures/sample.pdf')`,等待上传完成

#### Scenario: apiMock factory
- **WHEN** 调 `apiMock(page, '**/api/v1/auth/login', 200, { ... })`
- **THEN** 注册一个 route handler,返回指定 status + JSON body,持续到 page 关闭
