## Context

论文撰写页使用 react-arborist 作为文件树底层库，自定义了 `NodeRenderer`、`FolderNodeRow` 和 `InlineInput` 三个渲染组件。当前实现功能上可用（拖拽、重命名、创建、删除），但存在多个问题：视觉粗糙、数据转换算法脆弱（依赖文件列表排序）、错误处理缺失、删除后无自动选中、外部拖拽仅支持根目录、无搜索过滤。

项目使用 Tailwind CSS + shadcn/ui 作为设计系统，主题色为 `primary`（已在 CSS variables 中定义）。文件树需要与学术主题 PaperSage 的整体风格保持一致。

## Goals / Non-Goals

**Goals:**
- 文件树视觉质量达到 VS Code 文件浏览器水平
- 文件图标细粒度按类型区分（9 类 LaTeX 文件类型）
- 搜索过滤功能可快速定位文件（纯过滤，不支持操作）
- 拖拽反馈清晰，包含目标文件夹高亮和放置预览
- 外部文件拖拽支持拖到指定文件夹
- 文件夹展开/折叠有平滑过渡动画
- 文件夹展开/折叠状态持久化
- 删除当前活跃文件后自动选中下一个
- 修复数据转换算法的脆弱性
- 补全错误处理（依赖全局 toast）
- 所有交互符合 PaperSage 学术主题风格
- 完整测试覆盖（单元 + 集成 + E2E）

**Non-Goals:**
- 不替换 react-arborist 库（继续使用 v3.5.0）
- 不增加多选、批量操作等高级功能
- 不实现文件内容预览（已有 PDF 预览区）
- 不修改后端 API（schema 修复已完成）
- 不改动 AI Panel（后续独立实现）
- 不实现多入口编译（拆为独立 change）
- 搜索过滤仅用于定位，不支持在过滤结果上执行操作

## Decisions

### 1. 文件图标方案：Lucide 图标 + 细粒度颜色映射（9 类）

**选择：** 用已有的 lucide-react 图标，按扩展名映射 9 种类型。

| 扩展名 | 图标 | 颜色 |
|--------|------|------|
| `.tex` | FileText | 蓝 (primary) |
| `.bib` | BookOpen | 琥珀色 |
| `.cls` | FileCode | 紫色 |
| `.sty` | FileCode | 靛蓝色 |
| `.bst` | FileCode | 粉色 |
| `.tikz` | FileCode | 青色 |
| `.png/.jpg/.jpeg/.eps` | FileImage | 绿色 |
| `.pdf` | FileText | 红色 |
| 其他 | File | 灰色 (muted) |

**替代方案：** 引入 `vscode-icons` npm 包。
**理由：** 避免新依赖。Lucide 的图标 + 颜色区分已足够覆盖 LaTeX 项目所有常见文件类型。

### 2. 搜索过滤：react-arborist 内置 searchTerm（纯定位）

**选择：** 利用 react-arborist 原生 `searchTerm` prop + `searchMatch` 函数，顶部加搜索输入框。过滤后仅用于定位，不能执行创建/重命名/移动操作。

**理由：** 简单可靠。LaTeX 项目文件少，搜索主要用来快速定位，找到后切回完整树操作。

### 3. 动画方案：CSS transition

**选择：** chevron 用 `transform: rotate` + CSS transition 150ms。

**理由：** 不引入动画库，纯 CSS 即可实现流畅过渡。

### 4. 组件结构：拆分为 4 个子组件文件

**选择：**
- `writing-file-tree.tsx` — 主组件，数据转换和事件处理
- `file-tree-node.tsx` — 文件/文件夹节点渲染器
- `file-tree-icons.tsx` — 文件图标映射（9 类）
- `file-tree-search.tsx` — 搜索过滤栏

**理由：** 当前单文件 439 行，拆分后每个文件控制在 200 行以内。

### 5. 数据转换算法：两遍扫描

**选择：** 改 `toArboristData` 为两遍扫描：第一遍建所有节点，第二遍统一链接父子关系。

**理由：** 当前算法依赖文件列表的插入顺序，如果后端返回乱序会导致孤儿节点。两遍扫描彻底消除此问题。

### 6. 错误处理：handler 加 catch，依赖全局 toast

**选择：** 文件树 handler 里加 `.catch(() => {})` 吞掉 unhandled rejection。用户提示由 `client.ts` 全局 toast 处理，状态回滚由 TanStack Query invalidation 处理。

**理由：** 不在文件树层重复错误提示逻辑。全局已有完善的错误展示机制。

### 7. 外部拖拽：支持指定文件夹

**选择：** 检测外部文件拖拽悬停在文件夹节点上时，记录目标文件夹路径，drop 时上传到该文件夹。

**理由：** 当前只能拖到根目录，用户需要手动移动文件到目标文件夹。

### 8. 删除后自动选中

**选择：** 删除当前活跃文件时，优先选中同目录下一个文件，其次回退 `main.tex`，最后清空编辑器。

**理由：** 当前删除活跃文件后编辑器保留已删除文件内容，用户困惑。

### 9. 折叠状态持久化

**选择：** 使用 react-arborist 的 `initialOpenState` prop，将展开状态存入 React state，可通过 localStorage 或组件生命周期保持。

**理由：** 避免每次刷新/切换项目后所有文件夹都展开。

### 10. Edit Toolbar 移除

**选择：** 直接删除 WritingPage.tsx 中无功能的 Edit Toolbar 组件。

**理由：** B/I/U/cite/ref 按钮无 onClick handler，纯占位，移除后不影响任何功能。

### 11. 键盘快捷键

**选择：** 添加 Delete 删除选中文件、F2 重命名、Escape 取消操作三个快捷键。

**理由：** VS Code 风格文件树的标准交互，用户期望自然存在。实现简单（react-arborist 已暴露 onRename/Delete 回调），无副作用。

### 12. 文件夹递归删除

**选择：** 新增后端 `DELETE /api/latex-files/{project_id}/folder` 端点，接收文件夹路径，递归删除该文件夹下所有文件。前端在删除文件夹前弹出确认对话框，提示"该文件夹下所有文件及子文件夹将被永久删除"。

**替代方案：** 前端遍历文件夹下所有文件逐一调用删除 API。
**理由：** 逐一删除在文件多时请求量大、不可靠（部分失败无法回滚）。后端批量删除更安全、更高效。

### 13. InlineInput onBlur 行为

**选择：** `InlineInput` 的 `onBlur` 从自动提交改为取消编辑。

**理由：** 用户点击其他区域通常是"我不想改了"而非"我要确认"。与 VS Code 行为一致（Esc 取消、Enter 确认、点击外部取消）。

### 14. console.error 替换

**选择：** 将 `BinaryFilePlaceholder` 中的 `console.error` 替换为 toast 提示。

**理由：** `console.error` 对用户不可见，错误应通过全局 toast 展示。项目已有 `useToast` hook。

## Risks / Trade-offs

- **[react-arborist 维护状态]** → 最后更新 2025 年初，但有 3.5k stars 和 351k 周下载。如果出现 React 19 兼容问题，可 fork 修复。
- **[搜索性能]** → 对于 LaTeX 项目通常 <100 文件，性能无问题。超大项目可后续加 debounce。
- **[外部拖拽到文件夹]** → 需要精确检测拖拽悬停的文件夹节点，react-arborist 的外部拖拽 API 较弱，可能需要自行实现 drop target 检测。
- **[样式回归]** → 需要在改完后验证现有的拖拽、重命名、创建文件等所有交互流程。
