## Why

WritingPage 当前使用硬编码的 LaTeX 内容、假的 setTimeout 编译和静态文件树。后端 CLSI LaTeX 编译服务已就绪（33/33 任务完成），但前端写作页面未接入任何真实后端 API。需要打通从源文件存储 → 编辑器编辑 → CLSI 编译 → PDF 预览的完整链路。

## What Changes

- 新增 `LatexSourceFile` 后端模型和 CRUD API，用于存储项目下的 `.tex`/`.bib`/`.cls` 等源文件内容（存储在 S3 对象存储，数据库记录元数据）
- 新增 `latex-files-api.ts` 前端服务层，封装源文件的 CRUD 和 TanStack Query hooks
- 改造 `WritingPage.tsx`：从硬编码改为从后端加载文件列表和文件内容，编辑后自动保存
- 改造编译流程：收集所有源文件 → POST `/latex/compile` → 拿到 buildId → GET `/output/{buildId}/output.zip` → 解压提取 PDF → react-pdf 渲染
- 文件树从静态 JSX 改为动态渲染，支持文件切换、新建、重命名、删除
- 编译错误显示在编辑器下方或侧边面板

## Capabilities

### New Capabilities
- `latex-source-storage`: LaTeX 源文件的存储模型、S3 对象存储集成和 CRUD API（后端）
- `writing-page-integration`: WritingPage 前端接入真实后端 API（文件加载、编译、预览、文件树）

### Modified Capabilities

## Impact

- **后端**: 新增 `models/latex_file.py`、`api/routes/latex_files.py`、`services/latex/file_storage.py`；新增 Alembic migration；需要 S3/MinIO 配置
- **前端**: 改造 `WritingPage.tsx`；新增 `services/latex-api.ts`；可能需要新的 Zustand store 管理编辑器状态
- **依赖**: 新增 `boto3`（S3 客户端）后端依赖；前端无需新依赖（已有 axios、react-pdf、monaco-editor）
- **API**: 新增 `/api/v1/projects/{projectId}/latex-files/*` 系列端点
