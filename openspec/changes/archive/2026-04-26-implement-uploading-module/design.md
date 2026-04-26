## Context

PaperSage 的 uploading 模块是项目内上传和管理**参考文献**的入口。这是一个**参考文献中心**，不是代码文件管理器。用户在此浏览、筛选、预览研究资料（PDF、DOC、DOCX、TXT），这些文献后续将用于构建知识库。

### 现状分析

| 层面 | 状态 | 说明 |
|------|------|------|
| 前端 API hooks | ✅ 已完成 | `files-api.ts`(7 hooks), `folders-api.ts`(3 hooks) |
| 前端 Zod schemas | ✅ 已完成 | `types/files.ts` |
| 前端 Zustand store | ✅ 已完成 | `store/upload-store.ts` |
| 前端路由 | ❌ Placeholder | `router.tsx:174` |
| 前端 UI 组件 | ❌ 不存在 | `views/uploading/` 未创建 |
| 后端 Document 模型 | ❌ 不存在 | 无数据库表 |
| 后端 Folder 模型 | ❌ 不存在 | 无数据库表 |
| 后端 API 端点 | ❌ 全部 501 | 无真实实现 |

### 可复用的现有模式

1. **PDF 预览**: `react-pdf` v10 + `pdfjs-dist` v5 + range-transport（已安装）
2. **可调整面板**: `react-resizable-panels` v4（已安装，WritingPage 已用）
3. **服务层模式**: `services/latex/file_service.py` 的 S3 上传 + 项目权限验证
4. **对话框**: `ConfirmDialog`、`Dialog` 等可复用

## Goals / Non-Goals

**Goals:**
- 后端：建立 Document/Folder 数据模型、服务层、API 端点
- 前端：构建**全宽数据表格 + 筛选标签 + 侧边预览**的参考文献管理界面
- 表格支持：排序、筛选、行选择、批量操作、分页
- 文件上传：拖拽上传 + 进度追踪
- 文档预览：PDF（react-pdf）、DOCX（docx-preview）、TXT（原生渲染）
- 移动端响应式
- 为知识库阶段预留扩展点

**Non-Goals:**
- 不实现 OCR/文本提取（属于知识库阶段）
- 不实现文档版本控制
- 不实现文件分享或权限管理

## Layout Design

```
┌─────────────────────────────────────────────────────────────┐
│ Uploading Header: 项目名称 + 上传按钮                        │
├─────────────────────────────────────────┬───────────────────┤
│                                         │                   │
│  筛选标签栏                              │                   │
│  [全部] [PDF] [DOC] [TXT] [未分类]       │                   │
│  文件夹: [全部 ▾] [搜索...]              │    文档预览        │
│                                         │                   │
│  ┌──┬──────────┬────────┬──────┬───┐    │   PDF / TXT       │
│  │☐ │ Name     │ Size   │ Type │ ⋮ │    │   内嵌渲染         │
│  ├──┼──────────┼────────┼──────┼───┤    │                   │
│  │☐ │ paper1   │ 2.1 MB │ PDF  │ ⋮ │    │                   │
│  │☐ │ notes    │ 45 KB  │ TXT  │ ⋮ │    │                   │
│  │☐ │ thesis   │ 5.3 MB │ PDF  │ ⋮ │    │                   │
│  └──┴──────────┴────────┴──────┴───┘    │                   │
│                                         │                   │
│  批量操作栏 (选中时出现)                   │                   │
│  [3 selected] [移动] [备注] [删除]        │                   │
│                                         │                   │
│  分页: < 1 2 3 >                         │                   │
├─────────────────────────────────────────┴───────────────────┤
│ Upload Dropzone (可折叠/拖拽触发)                            │
└─────────────────────────────────────────────────────────────┘
```

**两栏布局**：左侧数据表格（主区域，约 60-65%），右侧预览面板（约 35-40%，可折叠）。文件夹和文件类型通过**顶部筛选标签**和**下拉选择器**过滤，而非独立左栏树。

**移动端**：预览面板折叠，点击文件后全屏预览（Sheet 组件）。

## Decisions

### 成熟软件包选型

| 需求 | 包 | 版本 | 成熟度 | 安装方式 |
|------|-----|------|--------|----------|
| 数据表格 | `@tanstack/react-table` | v8.21.3 | shadcn 官方 data table 基础，headless | `pnpm add @tanstack/react-table` |
| 文件上传 | `react-dropzone` | v15.0.0 | 最成熟的 React 拖拽上传库 | `pnpm add react-dropzone` |
| PDF 预览 | `react-pdf` + `pdfjs-dist` | v10 / v5 | 已安装，含 range transport | 无需操作 |
| 面板布局 | `react-resizable-panels` | v4.10.0 | 已安装 | 无需操作 |
| 复选框 | shadcn Checkbox | — | shadcn 官方 | `npx shadcn@latest add checkbox` |
| 选择器 | shadcn Select | — | shadcn 官方 | `npx shadcn@latest add select` |

### 文档预览方案（按文件类型）

| 文件类型 | 渲染包 | 版本 | 原理 | 安装方式 |
|----------|--------|------|------|----------|
| **PDF** | `react-pdf` + `pdfjs-dist` | v10 / v5（已安装） | Canvas 渲染，Web Worker 加速，支持 Range 请求 | 无需操作 |
| **DOCX** | `docx-preview` | v0.3.x | DOCX → HTML 转换，纯浏览器端，支持分页/字体/样式 | `pnpm add docx-preview` |
| **TXT** | 原生 `<pre>` | — | 直接文本渲染，等宽字体，自动换行 | 无需操作 |
| ~~DOC~~ | **不支持** | — | 旧版 Word 二进制格式，无成熟浏览器方案，后端上传时拒绝 | — |

```
文档预览面板路由逻辑:

         document.type
              │
    ┌─────────┼──────────┐
    ▼         ▼          ▼
  PDF       DOCX        TXT
    │         │          │
    ▼         ▼          ▼
 react-pdf  docx-      <pre>
 Document   preview    等宽字体
 + Page     renderAsync 自动换行
 Web Worker 分页/样式
```

### 为什么选 docx-preview

- 最成熟的 DOCX 浏览器端渲染方案（npm 周下载 20万+）
- 纯前端：将 DOCX 解析为 HTML，无需后端服务
- 支持分页、字体、样式、页边距、图片内嵌
- API 简洁：`renderAsync(blob, container)` 一行调用
- 替代方案对比：
  - `mammoth.js` — 只做简单 HTML 转换，丢失样式/分页，不适合预览
  - `react-docxodus-viewer` — 基于 WASM，生态小，不成熟
  - `@nutrient-sdk/viewer` — 商业付费，不适用

### 为什么不支持 DOC 格式

DOC（旧版 Word 97-2003 二进制格式）没有成熟的浏览器端渲染方案。对于学术论文场景：
- 绝大多数论文以 PDF 或 DOCX 发布，DOC 使用量极低
- 后端上传时直接拒绝 `.doc` 文件，仅接受 `.pdf`、`.docx`、`.txt`
- 避免引入后端 LibreOffice 转换服务的运维复杂度

### 为什么选 @tanstack/react-table

- shadcn/ui 官方 data table 就是基于它构建的
- headless 设计，完全控制 UI（配合 shadcn Table 组件）
- 内置：排序、筛选、分页、行选择、列可见性
- shadcn.io 提供 8 种 data table 示例代码可直接参考
- 活跃维护，v9 roadmap 已公布（向后兼容）

### 为什么选 react-dropzone（而非 shadcn-dropzone）

- react-dropzone v15 是最成熟的方案（npm 周下载 400万+）
- shadcn-dropzone 只是 react-dropzone 的 thin wrapper，增加了一层依赖
- 直接用 react-dropzone + shadcn Card/Input 自定义 UI 更灵活
- 支持 `useDropzone` hook，可完全控制渲染逻辑

### 为什么改为纯表格+筛选标签（而非三栏）

1. **场景匹配**：参考文献中心的核心操作是**浏览和筛选**，不是文件树导航
2. **信息密度**：表格展示文献信息（名称、大小、类型、备注、日期）更高效
3. **操作效率**：筛选标签 + 批量操作 > 逐层展开文件夹树
4. **参考对象**：Google Drive 列表视图、Zotero、Mendeley 等文献管理工具都是这个模式
5. **成熟组件**：@tanstack/react-table 提供开箱即用的排序、筛选、分页、行选择

文件夹功能**保留但不作为主要导航方式**，而是通过顶部的文件夹下拉选择器来过滤。

### 后端架构

| 决策 | 选择 | 原因 |
|------|------|------|
| Folder 层级结构 | 自引用 FK (`parent_id → folders.id`) | 支持任意深度嵌套 |
| 文件夹删除策略 | `SET NULL` | 不丢失数据 |
| S3 key 设计 | `projects/{id}/documents/{uuid}/{filename}` | UUID 防重名 |
| Document 状态字段 | `status` default="uploaded" | 为知识库阶段预留 |
| 预览接口 | 后端 streaming response | 不暴露 S3 凭据 |

## Risks / Trade-offs

- **Risk**: 大文件上传超时
  - **Mitigation**: react-dropzone + Axios `onUploadProgress` + Zustand 进度追踪
- **Risk**: PDF 预览大文件性能
  - **Mitigation**: pdf.js Web Worker + 按需渲染
- **Trade-off**: 去掉文件夹树后，深层嵌套文件夹需要通过下拉选择器导航
  - **Acceptable**: 参考文献场景文件夹层级通常很浅（1-2 级），下拉选择器足够

## API Endpoint Mapping

| 前端常量 | 方法 | 端点 | 后端路由 |
|----------|------|------|---------|
| `DOCUMENTS.listByProject` | GET | `/api/v1/projects/{pid}/documents` | `projects.py` |
| `DOCUMENTS.listByFolder` | GET | `/api/v1/folders/{fid}/documents` | `folders.py` |
| `DOCUMENTS.upload` | POST | `/api/v1/documents/upload` | `documents.py` |
| `DOCUMENTS.preview` | GET | `/api/v1/documents/{id}/preview` | `documents.py` |
| `DOCUMENTS.move` | PUT | `/api/v1/documents/{id}/move` | `documents.py` |
| `DOCUMENTS.remark` | PUT | `/api/v1/documents/{id}/remark` | `documents.py` |
| `DOCUMENTS.delete` | DELETE | `/api/v1/documents/{id}` | `documents.py` |
| `FOLDERS.listByProject` | GET | `/api/v1/projects/{pid}/folders` | `projects.py` |
| `FOLDERS.create` | POST | `/api/v1/folders` | `folders.py` |
| `FOLDERS.delete` | DELETE | `/api/v1/folders/{id}` | `folders.py` |

## Data Model

```
┌──────────────┐       ┌──────────────┐
│   projects   │       │    users     │
│──────────────│       │──────────────│
│ id (PK)      │◄──┐   │ id (PK)      │◄──┐
│ name         │   │   │ email        │   │
│ owner_id (FK)│───┤   └──────────────┘   │
└──────────────┘   │                      │
                   │                      │
┌──────────────┐   │   ┌──────────────┐   │
│   folders    │   │   │  documents   │   │
│──────────────│   │   │──────────────│   │
│ id (PK)      │   │   │ id (PK)      │   │
│ name         │   │   │ name         │   │
│ parent_id(FK)│───┤   │ type (MIME)  │   │
│ project_id   │───┘   │ size         │   │
│ owner_id     │───────│ status       │   │
└──────────────┘       │ project_id   │───┘
       ▲               │ folder_id    │───┐ (SET NULL)
       │               │ remark       │   │
       └───────────────│ owner_id     │   │
                       │ s3_key       │   │
                       └──────────────┘   │
                                          │
                                  ┌───────┘
                                  │
                       ┌──────────────┐
                       │  S3 (R2)     │
                       │──────────────│
                       │ projects/    │
                       │   {pid}/     │
                       │   documents/ │
                       │     {uuid}/  │
                       │       {file} │
                       └──────────────┘
```
