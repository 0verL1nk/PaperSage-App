## Why

论文撰写页的文件树基于 react-arborist 但自定义渲染器过于简陋：样式粗糙、拖拽反馈不清晰、缺少搜索过滤、文件图标单一、文件夹展开无动画，且存在移动到根目录等 bug。数据转换算法脆弱、错误处理缺失、删除后无自动选中逻辑、外部拖拽仅支持根目录。需要将其打磨到接近 VS Code 文件浏览器的视觉和交互质量，同时符合项目 PaperSage 的学术主题风格。

## What Changes

- 重写 `writing-file-tree.tsx` 的 NodeRenderer，实现 VS Code 风格的精细样式（选中高亮、hover、拖拽反馈、活动指示条）
- 增加细粒度 LaTeX 文件类型图标（9 类：.tex=蓝, .bib=琥珀, .cls=紫, .sty=靛蓝, .bst=粉, .tikz=青, 图片=绿, .pdf=红, 其他=灰）
- 增加文件夹展开/折叠 chevron 动画（CSS transition 150ms）
- 增加顶部搜索过滤栏（纯定位过滤，使用 react-arborist 内置 searchTerm）
- 修复 `toArboristData` 脆弱算法（改为两遍扫描，不依赖文件列表排序）
- handler 加 `.catch` 吞掉 unhandled rejection，依赖全局 toast + query invalidation 处理错误
- 外部文件拖拽支持拖到指定文件夹（不仅限于根目录）
- 删除当前活跃文件后自动选中下一个文件或回退到 main.tex
- 文件夹展开/折叠状态持久化（initialOpenState）
- 删除 WritingPage.tsx 中无功能的 Edit Toolbar（B/I/U/cite/ref 占位按钮）
- 修复移动文件到根目录时 `new_dir` 为空字符串导致 422 的 bug（已修复 schema）

## Capabilities

### New Capabilities
- `file-tree-polish`: 打磨文件树组件的视觉样式、交互反馈、文件图标、搜索过滤、健壮性修复和测试覆盖

### Modified Capabilities
- `writing-page-integration`: 文件树拖拽增强（外部拖到指定文件夹）、删除后自动选中、Edit Toolbar 移除、根目录移动 bug 修复

## Impact

- `frontend/src/views/projects/writing-file-tree.tsx` — 重构，拆分为多个子组件
- `frontend/src/views/projects/file-tree-node.tsx` — 新增，节点渲染器
- `frontend/src/views/projects/file-tree-icons.tsx` — 新增，文件图标映射
- `frontend/src/views/projects/file-tree-search.tsx` — 新增，搜索过滤栏
- `frontend/src/views/projects/WritingPage.tsx` — 移除 Edit Toolbar，调整 FileTree props
- `frontend/src/views/projects/__tests__/` — 新增完整单元测试
- `frontend/tests/e2e/` — 新增 E2E 测试
- `backend/schemas/latex_file.py` — `LatexFileMoveRequest.new_dir` 已允许空字符串
- `backend/api/routes/latex_files.py` — 新增文件夹递归删除端点（含确认提示）
- `backend/services/latex/file_service.py` — 新增文件夹递归删除服务方法
- 修复 `InlineInput` onBlur 时自动提交改为取消编辑
- 修复 `BinaryFilePlaceholder` 中 `console.error` 替换为 toast 提示
- 新增键盘快捷键（Delete 删除、F2 重命名、Escape 取消）
- 无新增依赖，继续使用 react-arborist

## Out of Scope

- AI Panel 保持原样不动，后续独立实现
- 多入口编译（选中任意 .tex 编译）拆为独立 change
- 编辑器工具栏（B/I/U/cite/ref）直接删除，不做功能实现
- 移动端文件树体验不在本次范围
- 文件上传不限制类型和大小
- 空文件夹在文件树中可见（后端返回目录结构即可显示），不支持在文件树中创建空文件夹
- 右键菜单保持现有功能，不做新增
- 文件内容预览不在本次范围（已有 PDF 预览区）
