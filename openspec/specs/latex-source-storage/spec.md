## ADDED Requirements

### Requirement: LaTeX source file model
系统 SHALL 提供 `latex_files` 表存储项目下 LaTeX 源文件的元数据，包含 project_id、file_path、content_type、size_bytes、s3_key、last_saved_at。

#### Scenario: Create project with default template
- **WHEN** 用户创建一个新项目
- **THEN** 系统自动在 S3 创建默认 `main.tex` 模板文件，并在 `latex_files` 表插入元数据记录

### Requirement: Source file CRUD API
系统 SHALL 提供 `/api/v1/projects/{projectId}/latex-files` 端点，支持列出、获取、创建、更新、删除源文件。

#### Scenario: List all source files
- **WHEN** GET `/api/v1/projects/{projectId}/latex-files` 请求
- **THEN** 返回该项目所有源文件的元数据列表，按 file_path 排序

#### Scenario: Get file content
- **WHEN** GET `/api/v1/projects/{projectId}/latex-files/{filePath}` 请求
- **THEN** 从 S3 读取文件内容并返回，Content-Type 根据 file extension 确定

#### Scenario: Update file content
- **WHEN** PUT `/api/v1/projects/{projectId}/latex-files/{filePath}` 请求带 body `{ content: "..." }`
- **THEN** 系统将新内容写入 S3，更新 `latex_files` 元数据的 size_bytes 和 last_saved_at

#### Scenario: Create new file
- **WHEN** POST `/api/v1/projects/{projectId}/latex-files` 请求带 body `{ path: "sections/intro.tex", content: "..." }`
- **THEN** 系统在 S3 创建文件，插入 `latex_files` 记录，返回 201

#### Scenario: Delete file
- **WHEN** DELETE `/api/v1/projects/{projectId}/latex-files/{filePath}` 请求
- **THEN** 系统从 S3 删除对象，删除 `latex_files` 记录

### Requirement: S3 storage integration
系统 SHALL 使用 S3 兼容对象存储（MinIO 或 AWS S3）存储 LaTeX 源文件内容，object key 格式为 `projects/{project_id}/latex/{file_path}`。

#### Scenario: Configure S3 backend
- **WHEN** 环境变量 `S3_ENDPOINT`、`S3_ACCESS_KEY`、`S3_SECRET_KEY`、`S3_BUCKET` 已配置
- **THEN** 系统使用 boto3 连接 S3，自动创建 bucket（如不存在）

#### Scenario: Local dev with MinIO
- **WHEN** `S3_ENDPOINT` 指向本地 MinIO（如 `http://localhost:9000`）
- **THEN** 所有文件操作通过 MinIO 完成，无需 AWS 凭证

### Requirement: File path validation
系统 SHALL 验证文件路径安全：禁止路径遍历（`../`）、必须以 `.tex`/`.bib`/`.cls`/`.sty`/`.bst` 扩展名结尾。

#### Scenario: Reject path traversal
- **WHEN** 创建文件请求的 path 包含 `../`
- **THEN** 返回 400 错误

#### Scenario: Reject invalid extension
- **WHEN** 创建文件请求的 path 扩展名不在允许列表中
- **THEN** 返回 400 错误

### Requirement: Project access control
所有 LaTeX 源文件 API SHALL 验证当前用户是项目 owner，非 owner 请求返回 403。

#### Scenario: Non-owner access
- **WHEN** 用户请求非自己拥有的项目的 LaTeX 文件 API
- **THEN** 返回 403 Forbidden
