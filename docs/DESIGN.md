# PaperSage 技术设计文档

> 本文档定义 PaperSage 各核心模块的详细技术设计方案，结合调研结果与现有代码库，为实现提供具体指导。

---

## 一、核心设计原则

基于项目背景与调研结果，确定以下核心设计原则：

1. **RAG-First AI 架构**：AI 功能以检索增强生成为核心，文档解析 → 分块 → 向量索引 → LLM 生成是主干流水线。
2. **外部标注存储**：高亮/批注数据以独立 JSON 存储，不嵌入 PDF，以保证协作同步效率和搜索能力。
3. **来源可追溯**：每个 AI 生成结果必须能追溯到原文位置（页码 + 坐标），UI 需展示引用来源。
4. **渐进增强**：MVP 阶段聚焦单用户场景，协作与离线功能在 v1.1+ 迭代。
5. **模块化解耦**：解析服务、存储服务、检索服务、AI 编排服务独立部署或清晰分离。

---

## 二、文档解析与存储

### 2.1 解析流水线

```
用户上传参考文献（PDF/Word）
    ↓
存储原始文件 (MinIO)
    ↓
解析任务入队 (Celery / 后台队列)
    ↓
┌─────────────────────────────────┐
│  解析阶段 1: 布局检测             │
│  LayoutParser / PDF-Extract-Kit   │
│  识别多栏、表格、公式、图片区域     │
├─────────────────────────────────┤
│  解析阶段 2: 文本提取             │
│  PyMuPDF / PDFPlumber           │
│  提取纯文本 + HTML 文本 + 坐标     │
├─────────────────────────────────┤
│  解析阶段 3: 结构化提取            │
│  GROBID (元数据/参考文献)          │
│  Science Parse / LlamaParse      │
│  提取标题、作者、DOI、章节结构      │
├─────────────────────────────────┤
│  解析阶段 4: 内容分块             │
│  递归字符/标题分块，10-15% 重叠   │
│  按语义边界（段落/章节）切分       │
├─────────────────────────────────┤
│  解析阶段 5: 向量化索引            │
│  向量化存储 (向量数据库)           │
│  全文索引 (Elasticsearch)         │
└─────────────────────────────────┘
    ↓
元数据入库 (documents 表)
    ↓
任务完成通知 (WebSocket / SSE)
```

### 2.2 解析工具选型

| 阶段 | 工具 | 说明 |
|------|------|------|
| 布局检测 | LayoutParser (Detectron2) | 多栏/表格/公式区域检测 |
| 文本提取 | PyMuPDF + PDFPlumber | 文本 + 坐标 + 表格结构 |
| OCR | OCRFlux / PDF-Extract-Kit | 扫描 PDF 处理 |
| 元数据 | GROBID | 标题/作者/DOI/参考文献 |
| 结构化解析 | Science Parse / LlamaParse | 全文字段结构 JSON |
| 分块 | 自研（递归分块器） | 段落/标题边界感知 |

### 2.3 文档元数据模型

```typescript
interface DocumentRecord {
  id: string
  projectId: string
  name: string
  type: 'pdf' | 'word' | 'other'
  size: number
  storageKey: string          // MinIO 对象路径

  // 解析后的元数据
  metadata: {
    title?: string
    authors?: string[]
    doi?: string
    venue?: string
    year?: number
    pageCount?: number
  }

  // 结构化信息
  structure: {
    sections: Array<{
      id: string
      title: string
      level: number         // 1 = 一级标题
      pageRange: [number, number]
      chunkIds: string[]
    }>
    references: Array<{
      index: number
      text: string
      doi?: string
    }>
  }

  // 页尺寸映射（用于坐标归一化）
  pageMap: Array<{
    pageNumber: number
    width: number           // PDF 点单位
    height: number
  }>

  status: 'uploading' | 'processing' | 'ready' | 'error'
  errorMessage?: string

  createdAt: string
  updatedAt: string
}
```

### 2.4 内容分块策略

- **分块大小**：目标 500-800 tokens，段落为首选边界
- **重叠**：相邻块 10-15% 重叠，保持跨块上下文
- **特殊元素**：
  - 表格：以独立块存储，保留 table 标记
  - 公式：以 LaTeX 字符串存储
  - 图片：存储图片引用 + alt text
- **索引字段**：

```typescript
interface Chunk {
  id: string
  documentId: string
  chunkText: string
  chunkLevel: 'paragraph' | 'section' | 'figure' | 'table'
  tokenCount: number

  // 来源锚点（用于追溯）
  sourceAnchor: {
    pageNumber: number
    rect: { x: number; y: number; width: number; height: number }  // PDF 点坐标
    textOffsetStart: number
    textOffsetEnd: number
    sectionId?: string
  }

  // 元数据标签
  tags: {
    isFigure: boolean
    isTable: boolean
    isFormula: boolean
  }

  // 向量 ID（由向量数据库生成）
  vectorId?: string
}
```

---

## 三、标注系统

### 3.1 标注数据模型

```typescript
interface Highlight {
  id: string
  documentId: string
  userId: string

  // 位置锚点
  page: number
  position: {
    x: number       // PDF 点坐标
    y: number
    width: number
    height: number
  }
  // 文本偏移（作为备选锚点，支持不同渲染器）
  textOffsetStart: number
  textOffsetEnd: number
  highlightedText: string    // 选中的原文

  // 内容
  color: 'yellow' | 'green' | 'blue' | 'pink' | 'orange'
  note?: string             // 批注正文

  createdAt: string
  updatedAt: string
}

interface AnnotationThread {
  id: string
  highlightId: string       // 关联到某个高亮
  userId: string
  content: string           // 回复内容

  // 层级结构（支持讨论串）
  parentId?: string         // 父回复 ID
  depth: number             // 嵌套深度（限制 2 层）

  createdAt: string
  updatedAt: string
}
```

### 3.2 存储与同步

- **存储格式**：外部 JSON（Instant JSON 风格），不嵌入 PDF
- **同步策略**：
  - MVP 单用户：本地 IndexedDB 缓存 + 服务端持久化
  - v1.1+ 协作：基于事件溯源的增量同步（annotation delta）
- **导出**：支持导出为 XFDF 格式，供桌面 PDF 阅读器使用

### 3.3 PDF 渲染与标注锚定

- **前端渲染器**：PDF.js（Web 端）、react-pdf（React 组件封装）
- **坐标系统**：统一使用 PDF 点坐标（1 pt = 1/72 inch），渲染时换算为屏幕像素
- **文本选择**：利用 PDF.js 的 text content layer 获取文本范围
- **高亮渲染**：通过 SVG overlay 叠加层实现，不修改底层 PDF
- **换页/缩放**：监听 viewport change，重新计算坐标映射

### 3.4 标注与检索关联

每个 Highlight 存储 `highlightedText` 字段，可在全文检索中命中；
同时 `page + position` 可映射回 chunk，实现**基于高亮内容检索 AI 引用**。

---

## 四、AI 辅助功能

### 4.1 RAG 流水线架构

```
用户查询
    ↓
查询向量化 (embedding model)
    ↓
┌──────────────────────────────────────┐
│  检索阶段                              │
│  1. 向量相似度检索 (top-K chunks)      │
│  2. 可选：全文 BM25 召回              │
│  3. 可选：结构化关系查询               │
│  4. Rerank (可选)                     │
│  输出: 相关 chunk + 原文位置            │
└──────────────────────────────────────┘
    ↓
提示词组装 (Prompt Engineering)
    ↓
┌──────────────────────────────────────┐
│  LLM 生成                              │
│  - 输入: 用户查询 + 召回内容            │
│  - 要求: 引用来源（页码 + 原文片段）     │
│  - 输出: 带引用的回答                   │
└──────────────────────────────────────┘
    ↓
┌──────────────────────────────────────┐
│  可选: 验证阶段 (v0.5+)               │
│  - 用第二个 LLM 验证幻觉               │
│  - 标注置信度                          │
└──────────────────────────────────────┘
    ↓
返回结果（含引用溯源）
```

### 4.2 学术写作场景 Prompt 模板

#### 场景 1：文献摘要生成

```
你是一位学术写作助手。请根据以下文献片段，为这篇论文生成 200-300 字的中文摘要。

要求：
1. 包含研究目的、方法、主要发现和结论
2. 使用学术规范语言
3. 标注关键数据来源（页码）

--- 文献内容 ---
{retrieved_chunks}

--- 输出格式 ---
摘要：[内容]
关键词：[3-5个关键词]
```

#### 场景 2：大纲生成

```
你是一位计算机科学/[用户指定学科]领域的学术写作导师。

用户的研究主题：{user_topic}
用户已阅读的文献摘要：
{literature_summaries}

请生成一份毕业论文大纲，包含：
1. 摘要建议结构
2. 各章节标题和核心内容描述
3. 建议的研究方法

大纲应符合学术论文规范，适合本科毕业论文或入门级学术论文。
```

#### 场景 3：内容润色

```
请对以下论文片段进行学术润色：

原文：
{original_text}

要求：
1. 保持原意不变
2. 改善句式结构和表达
3. 使用规范的学术语言
4. 标注修改位置和原因
```

### 4.3 LLM 集成方案

- **模型选择**：支持 OpenAI GPT-4o / Claude Sonnet 4 / 本地模型切换
- **降级策略**：主模型失败时自动切换到轻量备用模型
- **成本控制**：
  - 缓存相同 query 的 embedding 结果（Redis）
  - 限制单次请求的最大 token 数
  - 启用流式输出（streaming）提升体感速度
- **监控**：记录每次 LLM 调用的 latency、cost、token 消耗

### 4.4 来源追溯设计

每个 AI 生成的回答中，每个论断必须附带溯源信息：

```typescript
interface Citation {
  chunkId: string
  documentId: string
  documentName: string
  pageNumber: number
  excerpt: string     // 引用的原文片段（30-50 字）
  confidence: number  // 0-1，可信度评分
}

interface AIResponse {
  answer: string      // 带 [1][2] 标注的回答
  citations: Citation[]
  metadata: {
    model: string
    latencyMs: number
    tokensUsed: number
    retrievedChunksCount: number
  }
}
```

---

## 五、搜索系统

### 5.1 混合搜索架构

```
用户搜索请求
    ↓
┌─────────────┐    ┌──────────────┐
│  全文检索    │    │  向量检索     │
│  (Elasticsearch) │  │ (向量数据库)  │
│  BM25 精确匹配 │  │ 语义相似度    │
└──────┬──────┘    └──────┬───────┘
       │                  │
       └────────┬─────────┘
                ↓
        Rerank (可选)
        (BGE-Reranker 等)
                ↓
        Top-K 结果
                ↓
        聚合文档级结果
        (去重 + 分组)
```

### 5.2 搜索索引设计

| 索引类型 | 引擎 | 内容 |
|----------|------|------|
| 全文索引 | Elasticsearch / Meilisearch | chunk 文本 + 文档元数据 |
| 向量索引 | Qdrant / ChromaDB / pgvector | chunk 向量 |
| 关系索引 | PostgreSQL | 文档/项目/用户关系 |

---

## 六、文件上传

### 6.1 上传类型定义

| 类型 | 用途 | 处理方式 |
|------|------|----------|
| **LaTeX 文档** | 用户的论文/文档源码 | 源码存储，触发编译 |
| **PDF 参考文献** | 阅读材料 | 解析 + 向量化 + 供 AI 引用 |
| **Word 文档** | 参考材料 | 转为 PDF 后处理 |

### 6.2 大文件分片上传

- **前端**：使用 `tus-js-client` 实现 tus 协议断点续传
- **后端**：MinIO + tusd（或自研分片合并端点）
- **分片大小**：5MB/片
- **进度追踪**：每个分片独立上传，后端维护上传状态

### 6.3 文件预览

- **LaTeX 源码**：内置编辑器，编译后预览 PDF
- **PDF 参考文献**：PDF.js 流式加载，支持分页 + 缩放
- **Word 文档**：后端转换为 PDF 再预览（LibreOffice headless）

---

## 七、前端路由与页面设计

### 7.1 路由结构

基于现有 `router.tsx`，各页面组件归属：

| 路由 | 组件归属 | 说明 |
|------|----------|------|
| `/app/documents` | `features/projects/ProjectListPage` | 项目列表页（当前占位） |
| `/app/trash` | `features/trash/TrashPage` | 回收站页（当前占位） |
| `/app/profile` | `features/profile/ProfilePage` | 个人中心（当前占位） |
| `/project/$projectId` | `features/workspace/WorkspacePage` | 工作区根页 |
| `/project/$projectId/uploading` | `features/uploading/UploadPage` | 上传模块 |
| `/project/$projectId/coding` | `features/coding/CodingPage` | 编码标注模块 |
| `/project/$projectId/classifying` | `features/classifying/ClassifyPage` | 分类模块 |
| `/project/$projectId/logic-chain` | `features/logic-chain/LogicChainPage` | 逻辑链模块 |
| `/project/$projectId/generating` | `features/generating/GeneratePage` | AI 生成模块 |
| `/project/$projectId/latex` | `features/latex/LatexEditorPage` | LaTeX 编辑模块 |

### 7.2 项目列表页设计

```
┌─────────────────────────────────────────┐
│  [搜索框]                [+ 新建项目]    │
├─────────────────────────────────────────┤
│  [全部分组 ▾] [排序: 最近更新 ▾]         │
├─────────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │ 项目 A  │  │ 项目 B  │  │ 项目 C  │ │
│  │ 5 篇文档 │  │ 12 篇文档│  │ 0 篇文档│ │
│  │ 3 天前   │  │ 1 周前   │  │ 刚刚   │ │
│  └─────────┘  └─────────┘  └─────────┘ │
└─────────────────────────────────────────┘
```

### 7.3 工作区布局设计

```
┌──────────────────────────────────────────────────────────────┐
│  项目名称 ▾                    [上传] [编码] [分类] [逻辑] [生成] │
├──────────────────────────────────────────────────────────────┤
│  ┌──────────────┐ ┌─────────────────────────────────────┐   │
│  │ 文件树面板    │ │                                      │   │
│  │              │ │                                      │   │
│  │ 📁 根目录    │ │         内容区域                      │   │
│  │  ├ 📄 doc1   │ │                                      │   │
│  │  ├ 📄 doc2   │ │    (PDF 预览 / 文件列表 / AI 对话)     │   │
│  │  └ 📁 子目录  │ │                                      │   │
│  │     └ 📄 doc3 │ │                                      │   │
│  │              │ │                                      │   │
│  ├──────────────┤ │                                      │   │
│  │ [高亮列表]    │ │                                      │   │
│  │ ┌──────────┐ │ │                                      │   │
│  │ │ 高亮 1   │ │ └─────────────────────────────────────┘   │
│  │ │ 高亮 2   │ │                                          │
│  │ └──────────┘ │                                          │
│  └──────────────┘                                          │
└──────────────────────────────────────────────────────────────┘
```

### 7.4 编码标注页设计

```
┌────────────────────────────────────────────────────────────┐
│  [文件选择下拉] [◀ 上一页] 第 3/15 页 [下一页 ▶] [🔍 搜索]   │
├────────────────────────────────────────────────────────────┤
│                    ┌────────────────────────────┐           │
│                    │                            │  ┌─────┐ │
│                    │   PDF 渲染区域              │  │高亮 │ │
│                    │   (SVG overlay 高亮叠加)   │  │面板 │ │
│                    │                            │  │     │ │
│                    │   ┌─ highlight ─────┐      │  │[黄] │ │
│                    │   │ 选中的文本高亮    │      │  │[绿] │ │
│                    │   └─────────────────┘      │  │[蓝] │ │
│                    │                            │  │     │ │
│                    └────────────────────────────┘  │ 笔记 │ │
│                                                    │ 内容 │ │
│  [高亮颜色选择] [添加批注] [导出高亮]               └─────┘ │
└────────────────────────────────────────────────────────────┘
```

---

## 八、数据流与模块交互

### 8.1 模块依赖关系

```
┌─────────────────────────────────────────────────────┐
│                    前端 (React 19)                   │
│  TanStack Query ←→ TanStack Router ←→ Zustand       │
└──────────────────────┬──────────────────────────────┘
                       │ HTTP/REST
┌──────────────────────▼──────────────────────────────┐
│                  API 网关层 (FastAPI)                │
│  Auth Route │ Projects Route │ Documents Route       │
│  Folders Route │ Highlights Route │ AI Route        │
└───────┬──────────┬──────────────┬─────────┬────────┘
        │          │              │         │
┌───────▼──┐ ┌─────▼─────┐ ┌────▼────┐ ┌──▼──────┐
│ Auth Svc │ │ Project   │ │Document │ │ AI Svc  │
│          │ │ Service   │ │ Service │ │          │
└────┬─────┘ └─────┬─────┘ └────┬────┘ └──┬──────┘
     │             │            │         │
┌────▼─────────────▼────────────▼─────────▼──────┐
│              数据访问层 (Repository Pattern)      │
└────┬─────────────┬─────────────┬──────────────┘
     │             │             │
┌────▼────┐ ┌──────▼──────┐ ┌───▼────────┐
│PostgreSQL│ │  MinIO/S3   │ │Redis Cache │
└──────────┘ └─────────────┘ └────────────┘
```

### 8.2 核心 API 端点

| 模块 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 认证 | POST | `/api/v1/auth/register` | 注册 |
| 认证 | POST | `/api/v1/auth/login` | 登录 |
| 认证 | POST | `/api/v1/auth/refresh` | 刷新 Token |
| 项目 | GET | `/api/v1/projects` | 项目列表 |
| 项目 | POST | `/api/v1/projects` | 创建项目 |
| 项目 | GET | `/api/v1/projects/{id}` | 项目详情 |
| 项目 | PUT | `/api/v1/projects/{id}` | 更新项目 |
| 项目 | DELETE | `/api/v1/projects/{id}` | 删除项目（软删除） |
| 分组 | GET | `/api/v1/groups` | 分组列表 |
| 分组 | POST | `/api/v1/groups` | 创建分组 |
| 文档 | GET | `/api/v1/projects/{id}/documents` | 文档列表 |
| 文档 | POST | `/api/v1/projects/{id}/documents` | 上传文档 |
| 文档 | GET | `/api/v1/documents/{id}` | 文档详情 |
| 文档 | GET | `/api/v1/documents/{id}/download` | 下载文件 |
| 文档 | DELETE | `/api/v1/documents/{id}` | 删除文档 |
| 文件夹 | POST | `/api/v1/projects/{id}/folders` | 创建文件夹 |
| 高亮 | GET | `/api/v1/documents/{id}/highlights` | 高亮列表 |
| 高亮 | POST | `/api/v1/documents/{id}/highlights` | 创建高亮 |
| 高亮 | PUT | `/api/v1/highlights/{id}` | 更新高亮/批注 |
| 高亮 | DELETE | `/api/v1/highlights/{id}` | 删除高亮 |
| AI | POST | `/api/v1/ai/summarize` | 生成摘要 |
| AI | POST | `/api/v1/ai/outline` | 生成大纲 |
| AI | POST | `/api/v1/ai/polish` | 润色内容 |
| AI | POST | `/api/v1/ai/query` | 文档问答 |
| 回收站 | GET | `/api/v1/trash` | 回收站列表 |
| 回收站 | POST | `/api/v1/trash/{type}/{id}/restore` | 恢复 |
| 回收站 | DELETE | `/api/v1/trash/{type}/{id}` | 永久删除 |

---

## 九、版本规划与实现顺序

基于 [11-roadmap.md](project/11-roadmap.md) 的 MVP 待办，按以下顺序实现：

### Phase 1: Projects + Folders (当前最适合启动)

- 后端：Projects/Groups CRUD API + 数据库模型
- 前端：项目列表页 + 创建/删除/分组功能
- 回收站基础 API

### Phase 2: Document Upload

- 文件上传（分片上传 + MinIO 集成）
- 文档列表页 + 文件夹管理

### Phase 3: Coding (PDF Annotation)

- PDF.js 集成
- 高亮 CRUD API
- 前端标注 UI

### Phase 4: AI Features

- LLM 集成（RAG 流水线）
- 摘要/大纲/润色 API
- 前端 AI 对话 UI

---

## 十一、LaTeX 渲染服务

### 11.1 设计目标

用户可以在 PaperSage 中编辑 LaTeX 文档，系统将其编译为 PDF 返回预览/下载，实现**"阅读文献 → AI 生成 → LaTeX 编辑 → PDF 输出"**的完整写作闭环。

### 11.2 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                      前端 (React 19)                        │
│  ┌─────────────────┐   ┌─────────────────────────────────┐  │
│  │  LaTeX 编辑器    │   │  PDF 预览组件                   │  │
│  │  (TipTap/Milkdown) │  │  (pdf.js)                     │  │
│  └────────┬────────┘   └───────────────┬─────────────────┘  │
│           │                            │                     │
│           │         HTTP/WebSocket      │                     │
└───────────┼────────────────────────────┼─────────────────────┘
            │                            │
            ▼                            │
┌───────────────────────────────────────────────────────────────┐
│                    后端 (FastAPI)                            │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐   │
│  │  TeX 路由    │   │ 编译任务队列  │   │  编译容器池     │   │
│  │ (/api/v1/tex) │   │ (Celery)     │   │ (Docker/本地)   │   │
│  └──────┬──────┘   └──────┬──────┘   └────────┬────────┘   │
│         │                 │                    │             │
│         └─────────────────┼────────────────────┘             │
│                           ▼                                  │
│                    ┌─────────────────┐                       │
│                    │  TeX Live 引擎   │                       │
│                    │  pdflatex + bibtex│                      │
│                    └────────┬────────┘                       │
│                           │                                  │
│                           ▼                                  │
│                    ┌─────────────────┐                       │
│                    │  MinIO (PDF 存储)│                       │
│                    └─────────────────┘                       │
└───────────────────────────────────────────────────────────────┘
```

### 11.3 功能范围

| 功能 | 说明 |
|------|------|
| LaTeX 编辑 | 富文本编辑器，支持公式插入（KaTeX 预览） |
| 实时预览 | 用户输入时增量编译（debounce 1-2s） |
| 完整编译 | 支持 `\include`、`\bibliography` |
| PDF 输出 | 预览 + 下载 |
| 版本历史 | 保留历史编译版本 |
| 模板支持 | 内置论文模板（本科毕业论文、学术小论文） |

### 11.4 数据模型

```typescript
interface TexDocument {
  id: string
  projectId: string
  name: string                    // 文档名称

  content: string                 // LaTeX 源码

  // 编译状态
  status: 'draft' | 'compiling' | 'success' | 'error'
  compiledPdfKey?: string         // MinIO 存储路径
  errorMessage?: string

  // 模板信息
  template?: 'thesis' | 'paper' | 'custom'

  // 版本
  version: number
  versions: Array<{
    version: number
    content: string
    compiledPdfKey?: string
    createdAt: string
  }>

  createdAt: string
  updatedAt: string
}
```

### 11.5 API 设计

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/projects/{id}/tex` | 创建 LaTeX 文档 |
| GET | `/api/v1/tex/{docId}` | 获取文档详情 |
| PUT | `/api/v1/tex/{docId}` | 更新源码（自动触发编译） |
| GET | `/api/v1/tex/{docId}/pdf` | 获取编译后的 PDF（流式） |
| GET | `/api/v1/tex/{docId}/versions` | 版本历史 |
| POST | `/api/v1/tex/{docId}/restore/{version}` | 恢复历史版本 |

### 11.6 编译流程

```
用户输入 LaTeX 源码
    ↓
debounce 1-2s（避免频繁编译）
    ↓
POST /tex/{id} 保存源码
    ↓
Celery 任务入队
    ↓
┌─────────────────────────────────────────┐
│  编译环境准备                            │
│  - 启动编译容器 / 使用本地 TeX Live      │
│  - 复制源码到工作目录                    │
├─────────────────────────────────────────┤
│  第一遍编译: pdflatex                   │
│  - 生成 .aux, .log                      │
├─────────────────────────────────────────┤
│  如果有参考文献:                        │
│  - bibtex 处理 .aux                    │
│  - pdflatex 第二遍                      │
│  - pdflatex 第三遍（交叉引用）          │
├─────────────────────────────────────────┤
│  输出 PDF                               │
│  - 存储到 MinIO                        │
│  - 更新数据库状态                       │
└─────────────────────────────────────────┘
    ↓
WebSocket / SSE 通知前端编译完成
    ↓
前端刷新 PDF 预览
```

### 11.7 错误处理

| 错误类型 | 处理方式 |
|----------|----------|
| 编译超时 | 5 分钟超时，终止并返回错误 |
| 语法错误 | 解析 .log 文件，提取错误行返回给前端 |
| 内存爆炸 | 限制编译容器内存（1GB） |
| 依赖缺失 | 提供基础 TeX Live 发行版，可扩展 |

### 11.8 技术选型

| 组件 | 选型 | 说明 |
|------|------|------|
| LaTeX 引擎 | TeX Live | 完整发行版，支持所有标准包 |
| 容器化 | Docker | 隔离编译环境，避免污染宿主机 |
| 任务队列 | Celery + Redis | 异步编译，避免阻塞主服务 |
| 前端编辑器 | Milkdown | 支持 LaTeX 公式的 Markdown 编辑器 |
| 公式渲染 | KaTeX | 行内/行间公式实时预览 |

### 11.9 与 AI 模块集成

```
AI 生成模块 → LaTeX 编辑器
      ↓
用户确认生成内容
      ↓
自动填充到 LaTeX 模板
      ↓
用户编辑 → 编译 → PDF
```

- AI 生成摘要/大纲后，可一键填入论文模板
- 支持生成章节草稿，用户在 LaTeX 编辑器中完善

---

## 十二、技术选型总结

| 类别 | 选型 | 备选 |
|------|------|------|
| 前端框架 | React 19 + TypeScript | - |
| 路由 | TanStack Router | - |
| 状态管理 | TanStack Query + Zustand | - |
| UI 组件 | shadcn/ui + Tailwind CSS 4 | - |
| PDF 渲染 | PDF.js | react-pdf |
| LaTeX 编辑 | Milkdown | TipTap |
| LaTeX 渲染 | TeX Live (Docker) | tinytex |
| 后端框架 | FastAPI | - |
| 数据库 | PostgreSQL (async SQLAlchemy 2.0) | MySQL / SQLite |
| 对象存储 | MinIO (S3 兼容) | AWS S3 |
| 缓存 | Redis | - |
| 全文检索 | Elasticsearch / Meilisearch | PostgreSQL full-text |
| 向量检索 | Qdrant / pgvector | ChromaDB |
| 文档解析 | GROBID + PyMuPDF + PDFPlumber | LlamaParse |
| LLM | OpenAI GPT-4o / Claude API | 本地模型 |
| 任务队列 | Celery + Redis | - |
| 标注格式 | Instant JSON | XFDF (导出) |
| 分片上传 | tus 协议 | - |

---

*本文档随实现进展持续更新。最后更新于 2026-04-12。*
