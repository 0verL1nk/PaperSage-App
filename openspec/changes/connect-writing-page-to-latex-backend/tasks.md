## 1. 后端：LaTeX 源文件存储模型与 Migration

- [x] 1.1 创建 `backend/models/latex_file.py`，定义 `LatexFile` 模型（project_id, file_path, content_type, size_bytes, s3_key, last_saved_at），关联 Project
- [x] 1.2 创建 Alembic migration，添加 `latex_files` 表
- [x] 1.3 扩展 `backend/app/config.py`，添加 S3 配置项（S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET, S3_REGION）
- [x] 1.4 更新 `backend/.env.example`，补充 S3 相关配置说明

## 2. 后端：S3 存储服务

- [x] 2.1 创建 `backend/services/storage/__init__.py` 和 `backend/services/storage/s3_client.py`，封装 boto3 S3 操作（put_object, get_object, delete_object, list_objects）
- [x] 2.2 添加 `boto3` 到后端依赖
- [x] 2.3 为 S3 客户端编写单元测试（使用 moto mock）

## 3. 后端：源文件 CRUD API

- [x] 3.1 创建 `backend/schemas/latex_file.py`，定义文件列表、创建、更新、内容响应的 Pydantic 模型
- [x] 3.2 创建 `backend/services/latex/file_service.py`，实现文件 CRUD 业务逻辑（验证路径、操作 S3、维护元数据）
- [x] 3.3 创建 `backend/api/routes/latex_files.py`，实现 GET（列表/内容）、POST（创建）、PUT（更新）、DELETE 路由
- [x] 3.4 在 `backend/api/__init__.py` 注册新路由
- [x] 3.5 为源文件 API 编写测试：路径验证、CRUD 操作、权限检查

## 4. 后端：项目创建时初始化模板

- [x] 4.1 修改项目创建逻辑，创建项目时自动生成默认 `main.tex` 模板并存入 S3
- [x] 4.2 编写测试验证项目创建后自动有 `main.tex`

## 5. 前端：LaTeX API 服务层

- [x] 5.1 创建 `frontend/src/services/latex-api.ts`，封装源文件 CRUD 和编译相关 API 调用
- [x] 5.2 创建 TanStack Query hooks：`useLatexFiles(projectId)`, `useLatexFileContent(projectId, path)`, `useLatexCompile()`, `useSaveLatexFile()`, `useCreateLatexFile()`, `useDeleteLatexFile()`
- [x] 5.3 添加 `jszip` 前端依赖，用于解压 output.zip

## 6. 前端：WritingPage 改造

- [x] 6.1 改造 `WritingPage.tsx`，从路由获取 `projectId`，挂载时加载文件列表和主文件内容
- [x] 6.2 改造文件树为动态渲染：从后端文件列表构建树形结构，支持目录折叠
- [x] 6.3 实现文件切换：点击文件树项加载对应文件内容到编辑器
- [x] 6.4 实现真实编译流程：收集文件 → POST compile → GET output.zip → JSZip 解压 → Blob URL → react-pdf 渲染
- [x] 6.5 实现编译错误显示：编译失败时在预览区显示错误信息
- [x] 6.6 实现 debounced 自动保存：编辑器内容变更 1.5s 后 PUT 保存
- [x] 6.7 实现新建文件（"+" 按钮 → 输入文件名 → POST → 刷新文件树）
- [x] 6.8 实现删除文件（右键菜单 → 确认 → DELETE → 刷新文件树）
- [x] 6.9 添加编译器选择下拉（pdflatex / xelatex / lualatex）

## 7. 端到端验证

- [ ] 7.1 启动全栈（make dev），验证创建项目后 WritingPage 自动有 main.tex 模板
- [ ] 7.2 验证编辑 → 编译 → PDF 预览完整流程
- [ ] 7.3 验证多文件工程：新建 sections/intro.tex，\input 引用，编译成功
- [ ] 7.4 验证编译错误场景：故意写错误 LaTeX，确认错误信息显示
