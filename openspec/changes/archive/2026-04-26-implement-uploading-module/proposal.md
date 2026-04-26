## Why

PaperSage 是一个学术研究辅助平台。uploading 模块是项目工作区内上传和管理**参考文献**的核心入口，用户可以在此上传 PDF、DOCX、TXT 格式的研究资料，并通过文件夹组织这些文献。这些上传的文献将在后续阶段作为**知识库**的基础数据源，支撑内容分类、逻辑链条提取、论文撰写等功能。

> **注意**：不支持 DOC 格式（旧版 Word 二进制），后端上传时直接拒绝。

当前状态：
- 前端路由 `/project/$projectId/uploading` 仅显示一个 placeholder 页面
- 前端数据层已就绪：`files-api.ts`（7 个 TanStack Query hooks）、`folders-api.ts`（3 个 hooks）、`types/files.ts`（Zod schemas）、`upload-store.ts`（Zustand store）
- **后端完全缺失**：`Document` 和 `Folder` 数据库模型不存在，`documents.py` 路由文件中所有端点返回 HTTP 501 占位符
- 需要前后端同步建设

## What Changes

### 后端（新建）
- 创建 `Folder` 数据模型（自引用 parent_id 支持多层嵌套）和 Alembic 迁移
- 创建 `Document` 数据模型（关联 project、folder、S3 存储）和迁移
- 创建 Pydantic request/response schemas
- 创建 `FolderService` 和 `DocumentService` 业务逻辑层
- 实现完整的文件夹 CRUD API（list、create、delete）
- 实现完整的文档管理 API（list by project、list by folder、upload multipart、preview、move、remark、delete）
- S3 存储集成：`projects/{id}/documents/{uuid}/{filename}` key 命名空间

### 前端（新建）
- 创建 `UploadingPage` 两栏布局（数据表格 + 预览面板），使用 `@tanstack/react-table` + `react-resizable-panels` v4
- 创建带筛选标签的 `DataTable`（排序、筛选、分页、行选择、批量操作）
- 创建 `FileListPanel` 文件列表表格，支持单选/多选和行操作
- 创建 `UploadDropzone` 拖拽上传组件（基于 shadcn-dropzone）
- 创建 `DocumentPreviewPanel` 文档预览面板（基于 react-pdf + pdfjs-dist）
- 创建操作对话框：CreateFolder、MoveDocument、EditRemark
- 替换路由 placeholder 为真实页面组件

## Capabilities

### New Capabilities
- `project-file-management`: 在项目工作区内管理文件夹和参考文献（上传、移动、删除、备注），支持文件夹层级组织
- `document-preview`: 在前端直接预览 PDF 和纯文本文档，无需下载

### Modified Capabilities

## Impact

- **后端**: 新增 2 个数据库表（folders、documents）、2 个 service 模块、2 个路由模块、1 个 Alembic 迁移。S3 存储增加 `documents/` 命名空间。
- **前端**: 新增 `views/uploading/` 目录（约 9 个组件文件），安装 shadcn-dropzone 和 checkbox 组件。修改 `router.tsx` 替换 placeholder。
- **依赖**: 前端新增 `shadcn-dropzone`（shadcn registry 组件，源码拷贝入项目）
- **未来影响**: Document 模型的 `status` 字段和 S3 存储结构设计需考虑知识库阶段的文本提取和向量化需求
