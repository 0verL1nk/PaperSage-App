## Context

WritingPage 是 PaperSage 论文撰写的核心页面，当前处于纯 UI 壳状态：硬编码 LaTeX 内容、假编译（setTimeout + 远程 PDF URL）、静态文件树。后端 CLSI 编译服务已完成集成（compile/wordcount/synctex/output.zip 端点全部就绪），但前端未接入。

当前数据模型中 `Document` 是参考文献/知识库用途，不适合存储 LaTeX 源文件。需要独立的存储方案。

用户确认 LaTeX 源文件存储使用 S3 兼容对象存储（MinIO 本地 / 云端 S3）。

## Goals / Non-Goals

**Goals:**
- 打通 WritingPage 的完整数据流：源文件 CRUD → 编辑 → 编译 → PDF 预览
- 文件树动态化，支持多文件 LaTeX 工程（主文件 + \input 子文件）
- 编译结果实时显示，编译错误可查看
- S3 对象存储源文件，数据库仅存元数据

**Non-Goals:**
- 实时协作编辑（多人 OT/CRDT）— 远期目标
- SyncTeX 双向跳转 — 本阶段仅接入编译和预览，SyncTeX 预留但不在前端实现
- AI 助手面板 — 保持现有 UI 壳，不做功能
- 自动保存 / 版本历史 — 仅做手动保存触发
- 文件上传（图片等二进制资源）— 第二阶段

## Decisions

### D1: 源文件存储 — S3 对象存储 + 数据库元数据

**选择**: 文件内容存 S3（MinIO 本地 / 云端 S3），`latex_files` 表存路径、大小、最后修改时间等元数据。

**理由**: 源文件可能较大（尤其是包含大量图片引用的工程），S3 天然支持大文件，且与后续文件上传（图片资源）统一存储后端。数据库存全文会膨胀 pg 体积。

**替代方案**:
- 纯数据库 TEXT 字段：简单，但不适合大文件和二进制资源扩展
- 纯文件系统：不利于多实例部署和备份

### D2: 文件 Key 结构

**选择**: `projects/{project_id}/latex/{file_path}` 作为 S3 object key。

**理由**: 与项目关联清晰，file_path 保留嵌套目录结构（如 `sections/intro.tex`），便于直接映射到 CLSI resources。

### D3: 编译流程 — 前端驱动

**选择**: 前端收集所有文件内容 → POST compile（files 数组带 content） → 轮询或等待返回 → GET output.zip → 前端解压取 PDF。

**理由**: CLSI compile 是同步阻塞请求（等编译完成才返回），无需额外轮询。output.zip 是 CLSI 唯一的输出端点，前端用 JSZip 解压提取 PDF。

### D4: 前端编辑器状态管理

**选择**: 组件内 useState + debounced 自动保存（不改 Zustand）。

**理由**: 编辑器状态（当前文件内容）是 WritingPage 局部状态，不需要全局共享。debounce 1-2s 后触发 PUT 保存到后端。

### D5: PDF 预览 — 从 output.zip 提取

**选择**: 编译成功后，前端 GET output.zip → JSZip 解压 → 找到 output.pdf → 转 Blob URL → react-pdf 渲染。

**理由**: CLSI 不提供单文件输出端点，只提供 output.zip。JSZip 轻量（~30KB gzip），前端解压比后端额外处理简单。

## Risks / Trade-offs

- **[S3 配置复杂度]** → 本地开发用 MinIO Docker，生产用云 S3；.env 配置统一管理
- **[大工程编译慢]** → CLSI timeout 已设 120s；前端显示编译进度和 cancel 按钮
- **[并发编辑冲突]** → 本阶段单用户，无冲突问题；后续引入 OT 时需重构
- **[output.zip 解压开销]** → 对于只有几页的论文，ZIP 很小（<1MB），前端解压开销可忽略
