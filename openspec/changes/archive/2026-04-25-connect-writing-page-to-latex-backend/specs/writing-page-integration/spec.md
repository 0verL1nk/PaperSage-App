## ADDED Requirements

### Requirement: Load project files on mount
WritingPage SHALL 在组件挂载时通过 `projectId` 路由参数加载项目的 LaTeX 源文件列表和默认主文件内容。

#### Scenario: Open writing page
- **WHEN** 用户导航到 `/project/{projectId}/writing`
- **THEN** 页面从后端加载文件列表（GET `/latex-files`），并加载 `main.tex` 内容到 Monaco 编辑器

#### Scenario: Empty project
- **WHEN** 项目没有 LaTeX 文件
- **THEN** 显示空状态提示，提供"创建 main.tex"按钮

### Requirement: Dynamic file tree
文件树 SHALL 从后端加载的文件列表动态渲染，支持目录折叠、文件切换、新建和删除。

#### Scenario: Render file tree
- **WHEN** 后端返回文件列表 `["main.tex", "sections/intro.tex", "references.bib"]`
- **THEN** 文件树按目录结构渲染，`sections/` 为可折叠文件夹

#### Scenario: Switch active file
- **WHEN** 用户点击文件树中的 `references.bib`
- **THEN** 编辑器切换显示 `references.bib` 的内容（从后端加载），当前未保存内容提示保存

#### Scenario: Create new file
- **WHEN** 用户点击 "+" 按钮并输入文件名 `sections/method.tex`
- **THEN** 前端 POST 创建文件，文件树刷新，编辑器打开新文件

#### Scenario: Delete file
- **WHEN** 用户右键文件选择删除并确认
- **THEN** 前端 DELETE 文件，文件树刷新，如删除的是当前文件则切换到 main.tex

### Requirement: Real compilation flow
点击编译按钮 SHALL 调用真实 CLSI 编译 API，收集所有源文件内容发送到后端。

#### Scenario: Successful compilation
- **WHEN** 用户点击编译按钮
- **THEN** 前端收集所有文件内容，POST `/latex/compile`，显示编译中状态，编译成功后获取 output.zip 并渲染 PDF

#### Scenario: Compilation failure
- **WHEN** CLSI 编译返回 failure 状态
- **THEN** 在预览区域显示编译错误信息（从 response.error 提取），保留上一次成功的 PDF（如有）

#### Scenario: Cancel compilation
- **WHEN** 编译进行中用户点击停止按钮
- **THEN** 前端调用 DELETE `/latex/compile`，恢复就绪状态

### Requirement: PDF preview from output.zip
编译成功后 SHALL 从 output.zip 提取 PDF 并用 react-pdf 渲染。

#### Scenario: Extract and render PDF
- **WHEN** 编译返回 buildId
- **THEN** 前端 GET `/latex/output/{buildId}/output.zip`，用 JSZip 解压，找到 `.pdf` 文件，创建 Blob URL 传递给 react-pdf 渲染

### Requirement: Auto-save on edit
编辑器内容变更 SHALL 在防抖延迟后自动保存到后端。

#### Scenario: Debounced save
- **WHEN** 用户编辑文件内容，停止输入 1.5 秒后
- **THEN** 前端 PUT 更新文件内容到后端，保存状态在 UI 上有视觉反馈（如标题栏 indicator）

### Requirement: Compiler selection
编译面板 SHALL 支持选择编译器（pdflatex / xelatex / lualatex）。

#### Scenario: Select xelatex
- **WHEN** 用户从编译器下拉选择 xelatex
- **THEN** 后续编译请求使用 `compiler: "xelatex"` 参数
