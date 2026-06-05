## 1. 组件拆分与文件图标

- [x] 1.1 创建 `file-tree-icons.tsx`：实现 9 类细粒度文件图标映射（.tex=蓝/FileText, .bib=琥珀/BookOpen, .cls=紫/FileCode, .sty=靛蓝/FileCode, .bst=粉/FileCode, .tikz=青/FileCode, .png/.jpg/.jpeg/.eps=绿/FileImage, .pdf=红/FileText, 其他=灰/File）
- [x] 1.2 创建 `file-tree-node.tsx`：提取 `NodeRenderer`、`FolderNodeRow`、`InlineInput` 组件，引入新图标组件，保持原有 props 接口不变

## 2. 视觉样式打磨

- [x] 2.1 优化文件节点样式：选中时左侧 primary 竖条 + primary/10 背景，hover 时 muted 背景 + 亮文字，拖拽中半透明
- [x] 2.2 优化文件夹节点样式：拖拽目标高亮（primary 边框 + 浅色背景），hover 效果
- [x] 2.3 添加文件夹 chevron 旋转动画（CSS transition 150ms）和文件夹图标切换过渡

## 3. 搜索过滤功能

- [x] 3.1 创建 `file-tree-search.tsx`：搜索输入框组件，带清空按钮，使用 Search 图标
- [x] 3.2 在主组件中集成 `searchTerm` state，传入 react-arborist 的 `searchTerm` + `searchMatch` props（纯定位过滤）
- [x] 3.3 处理搜索无匹配时的"无匹配文件"空状态显示

## 4. 数据转换与健壮性修复

- [x] 4.1 重写 `toArboristData` 为两遍扫描算法：第一遍建所有节点，第二遍统一链接父子关系
- [x] 4.2 在所有 handler（create/rename/move）中加 `.catch(() => {})` 吞掉 unhandled rejection
- [x] 4.3 实现文件夹展开/折叠状态持久化（使用 react-arborist `initialOpenState`）
- [x] 4.4 修复 `InlineInput` onBlur：从自动提交改为取消编辑（与 VS Code 行为一致）
- [x] 4.5 修复 `BinaryFilePlaceholder` 中 `console.error` 替换为 toast 提示

## 5. 交互增强

- [x] 5.1 实现外部文件拖拽到指定文件夹：检测拖拽悬停的文件夹节点，drop 时上传到该文件夹
- [x] 5.2 实现删除活跃文件后自动选中下一个文件（同目录优先 → main.tex 兜底 → 空状态）
- [x] 5.3 移除 WritingPage.tsx 中无功能的 Edit Toolbar（B/I/U/cite/ref 占位按钮）
- [x] 5.4 添加键盘快捷键：Delete 删除选中文件、F2 重命名、Escape 取消操作
- [x] 5.5 实现文件夹递归删除：前端确认对话框 + 后端批量删除端点

## 6. 后端 API 扩展

- [x] 6.1 新增文件夹递归删除端点 `DELETE /api/latex-files/{project_id}/folder`：接收 folder_path 参数
- [x] 6.2 在 `file_service.py` 中实现递归删除：查找指定前缀下所有文件，逐一删除，返回删除数量
- [x] 6.3 新增 Pydantic schema `FolderDeleteRequest`：验证 folder_path 非空且不以 / 结尾
- [x] 6.4 编写后端单元测试：覆盖空文件夹删除、含文件文件夹删除、路径验证

## 7. 主组件重构

- [x] 7.1 重写 `writing-file-tree.tsx`：导入拆分后的子组件，删除内联的 NodeRenderer/InlineInput/FolderNodeRow，保持 `FileTreeProps` 和 `FileTreeHandle` 接口不变
- [x] 7.2 在 `WritingPage.tsx` 中调整：删除 Edit Toolbar，传入 onDeleteFile 回调处理自动选中逻辑
- [x] 7.3 验证所有交互：拖拽移动、重命名、创建文件/文件夹、删除、右键菜单、外部文件拖入（含指定文件夹）、键盘快捷键

## 8. 单元测试

- [x] 8.1 编写 `file-tree-icons.test.tsx`：覆盖全部 9 种文件类型的图标和颜色映射
- [x] 8.2 编写 `toArboristData` 单元测试：覆盖正常排序、乱序输入、深层嵌套、孤儿节点防护
- [x] 8.3 编写 `file-tree-node.test.tsx`：覆盖选中高亮、hover、拖拽反馈、内联输入
- [x] 8.4 编写 `file-tree-search.test.tsx`：覆盖过滤、清空、无匹配空状态
- [x] 8.5 编写 handler 错误处理测试：覆盖 API 失败时的 catch 和状态恢复
- [x] 8.6 编写 InlineInput onBlur 测试：覆盖取消行为（不提交）
- [x] 8.7 编写键盘快捷键测试：覆盖 Delete/F2/Escape

## 9. 集成测试

- [x] 9.1 编写 WritingFileTree 集成测试：覆盖完整创建→编辑→重命名→删除流程
- [x] 9.2 编写拖拽移动集成测试：覆盖内部拖拽 + 外部文件拖到指定文件夹
- [x] 9.3 编写删除活跃文件集成测试：覆盖自动选中下一个、main.tex 兜底、空状态
- [x] 9.4 编写折叠状态持久化集成测试：覆盖刷新后状态保持
- [x] 9.5 编写文件夹递归删除集成测试：覆盖确认对话框 + 后端调用

## 10. E2E 测试

- [ ] 10.1 编写 Playwright E2E 测试：创建文件 → 编辑 → 编译 → 查看预览完整流程
- [ ] 10.2 编写文件操作 E2E 测试：创建文件夹 → 拖拽文件 → 重命名 → 删除
- [x] 10.3 编写搜索过滤 E2E 测试：搜索文件 → 点击定位 → 清空搜索（`tests/e2e/features/file-tree.spec.ts` — 2026-06）
- [x] 10.4 编写键盘快捷键 E2E 测试：F2 重命名 → Delete 删除 → Escape 取消（`tests/e2e/features/file-tree.spec.ts` — 2026-06；Delete 路径走 confirm-dialog，详见 `frontend/tests/AGENTS.md`）

## 11. 最终验证

- [x] 11.1 运行 `pnpm typecheck` 确认无类型错误
- [x] 11.2 运行 `pnpm test` 确认所有单元 + 集成测试通过
- [ ] 11.3 运行 Playwright E2E 测试确认通过
- [ ] 11.4 运行 `make dev` 在浏览器中手动验证所有交互和视觉效果
