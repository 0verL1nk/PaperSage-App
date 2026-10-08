## MODIFIED Requirements

### Requirement: LaTeX source file model
新建项目 SHALL 自动创建默认 `main.tex` 模板并写入元数据。若模板初始化失败，项目创建请求 MUST 返回失败并清理项目记录；系统 MUST NOT 报告一个缺少默认模板的项目创建成功。

#### Scenario: Create project with default template
- **WHEN** 用户创建一个新项目且模板初始化成功
- **THEN** 系统自动在对象存储创建默认 `main.tex` 模板文件，并在 `latex_files` 表插入元数据记录

#### Scenario: Default template initialization fails
- **WHEN** 用户创建项目时默认 LaTeX 模板无法写入对象存储或元数据
- **THEN** 项目创建请求返回失败，且项目记录被清理

### Requirement: Source file CRUD API
项目文件树即使为空也 SHALL 支持创建文件和文件夹。创建失败 SHALL 保留当前文件树状态并显示失败原因，MUST NOT 静默吞掉错误。

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

#### Scenario: Create a file in an empty project
- **WHEN** 用户在没有任何 LaTeX 文件的项目中点击新建文件
- **THEN** 文件树进入新文件命名状态，提交后创建文件并在树中显示

#### Scenario: Create a folder in an empty project
- **WHEN** 用户在没有任何 LaTeX 文件的项目中点击新建文件夹
- **THEN** 文件树进入文件夹命名状态，提交后创建文件夹并在树中显示

#### Scenario: File creation fails
- **WHEN** 新文件或文件夹的创建请求失败
- **THEN** 页面显示失败原因且不显示创建成功的虚假节点
