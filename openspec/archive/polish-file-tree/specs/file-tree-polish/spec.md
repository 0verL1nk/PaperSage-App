## ADDED Requirements

### Requirement: File tree displays fine-grained type-specific icons
文件树 SHALL 根据文件扩展名显示 9 种不同图标和颜色，使 LaTeX 相关文件类型可被快速识别。

#### Scenario: LaTeX source files
- **WHEN** 文件树中存在 `.tex` 文件
- **THEN** 该文件显示 FileText 图标，颜色为 primary 蓝

#### Scenario: Bibliography files
- **WHEN** 文件树中存在 `.bib` 文件
- **THEN** 该文件显示 BookOpen 图标，颜色为琥珀色

#### Scenario: Document class files
- **WHEN** 文件树中存在 `.cls` 文件
- **THEN** 该文件显示 FileCode 图标，颜色为紫色

#### Scenario: Style package files
- **WHEN** 文件树中存在 `.sty` 文件
- **THEN** 该文件显示 FileCode 图标，颜色为靛蓝色

#### Scenario: Bibliography style files
- **WHEN** 文件树中存在 `.bst` 文件
- **THEN** 该文件显示 FileCode 图标，颜色为粉色

#### Scenario: TikZ graphics files
- **WHEN** 文件树中存在 `.tikz` 文件
- **THEN** 该文件显示 FileCode 图标，颜色为青色

#### Scenario: Image files
- **WHEN** 文件树中存在 `.png`、`.jpg`、`.jpeg`、`.eps` 文件
- **THEN** 该文件显示 FileImage 图标，颜色为绿色

#### Scenario: PDF files
- **WHEN** 文件树中存在 `.pdf` 文件
- **THEN** 该文件显示 FileText 图标，颜色为红色

#### Scenario: Unknown file types
- **WHEN** 文件树中存在未识别扩展名的文件
- **THEN** 该文件显示 File 图标，颜色为 muted 灰色

### Requirement: File tree supports search filtering for locating files
文件树 SHALL 提供搜索输入框，用户可按文件名过滤文件树用于快速定位。过滤状态下不支持创建/重命名/移动操作。

#### Scenario: Filter by filename
- **WHEN** 用户在搜索框输入 "intro"
- **THEN** 文件树仅显示文件名包含 "intro" 的文件及其父目录

#### Scenario: Clear search
- **WHEN** 用户清空搜索框
- **THEN** 文件树恢复显示所有文件

#### Scenario: No matches
- **WHEN** 搜索词不匹配任何文件
- **THEN** 显示"无匹配文件"提示

### Requirement: File tree provides VS Code style visual feedback
文件树 SHALL 提供精细的视觉反馈，包括选中高亮、hover 效果、拖拽目标指示和活动文件标记。

#### Scenario: Active file indicator
- **WHEN** 用户选中某个文件
- **THEN** 该文件行左侧显示 primary 色竖条指示器，行背景使用 primary/10 色

#### Scenario: Hover effect
- **WHEN** 鼠标悬停在文件或文件夹上
- **THEN** 行背景变为 muted 色，文字颜色变亮

#### Scenario: Drag target highlight
- **WHEN** 拖拽文件悬停在某个文件夹上
- **THEN** 目标文件夹显示 primary 色边框和浅色背景

#### Scenario: Dragging source opacity
- **WHEN** 文件正在被拖拽
- **THEN** 源文件行显示半透明效果

### Requirement: Folder expand/collapse has smooth animation
文件夹展开和折叠 SHALL 有平滑的视觉过渡动画。

#### Scenario: Chevron rotation
- **WHEN** 用户点击文件夹展开或折叠
- **THEN** chevron 箭头在 150ms 内平滑旋转 90 度

#### Scenario: Folder icon transition
- **WHEN** 文件夹在展开和折叠之间切换
- **THEN** 文件夹图标从关闭状态平滑过渡到打开状态

### Requirement: Folder expand/collapse state persists
文件夹展开/折叠状态 SHALL 在组件重新渲染时保持。

#### Scenario: State preserved across re-renders
- **WHEN** 用户折叠某个文件夹后触发文件列表刷新
- **THEN** 该文件夹保持折叠状态

#### Scenario: State preserved across project switches
- **WHEN** 用户切换到另一个项目再切回来
- **THEN** 之前的展开/折叠状态被恢复

### Requirement: Data transform algorithm is order-independent
`toArboristData` 数据转换 SHALL 不依赖文件列表的输入顺序，使用两遍扫描算法。

#### Scenario: Unsorted file list
- **WHEN** 后端返回的文件列表不是按路径深度排序的
- **THEN** 文件树仍能正确构建完整的目录层级，不出现孤儿节点

### Requirement: Error handling does not cause unhandled rejections
文件树的所有操作 handler SHALL 捕获 API 错误，不产生 unhandled promise rejection。

#### Scenario: Create file fails
- **WHEN** 创建文件的 API 调用失败
- **THEN** 全局 toast 显示错误提示，文件列表通过 query invalidation 恢复正确状态

#### Scenario: Rename file fails
- **WHEN** 重命名文件的 API 调用失败
- **THEN** 全局 toast 显示错误提示，文件名恢复为原值

### Requirement: External file drop supports specific folders
从操作系统拖拽文件到文件树 SHALL 支持拖到指定文件夹，不仅限于根目录。

#### Scenario: Drop external file onto folder
- **WHEN** 用户从操作系统拖拽一个文件到文件树的某个文件夹上
- **THEN** 文件上传到该文件夹内

#### Scenario: Drop external file onto root area
- **WHEN** 用户从操作系统拖拽一个文件到文件树的空白区域
- **THEN** 文件上传到项目根目录

### Requirement: File tree component structure is modular
文件树组件 SHALL 拆分为多个子组件文件，每个文件不超过 200 行。

#### Scenario: Component files are under 200 lines
- **WHEN** 检查文件树相关组件文件
- **THEN** 每个文件不超过 200 行代码
- **AND** 组件按职责拆分为：主组件、节点渲染器、图标映射、搜索栏

### Requirement: File tree has comprehensive test coverage
文件树 SHALL 有完整的单元测试、集成测试和 E2E 测试覆盖。

#### Scenario: Unit tests cover core logic
- **WHEN** 运行 `pnpm test`
- **THEN** 数据转换、图标映射、事件 handler 等核心逻辑有独立单元测试

#### Scenario: Integration tests cover component interactions
- **WHEN** 运行集成测试
- **THEN** 文件选择、创建、重命名、删除、拖拽移动等组件交互有测试覆盖

#### Scenario: E2E tests cover user workflows
- **WHEN** 运行 Playwright E2E 测试
- **THEN** 创建文件、编辑、编译、重命名、删除等端到端用户流程有测试覆盖

### Requirement: File tree supports keyboard shortcuts
文件树 SHALL 支持键盘快捷键进行常用操作。

#### Scenario: Delete file with Delete key
- **WHEN** 用户选中一个文件后按下 Delete 键
- **THEN** 触发删除确认流程

#### Scenario: Rename file with F2 key
- **WHEN** 用户选中一个文件后按下 F2 键
- **THEN** 进入内联重命名模式

#### Scenario: Cancel with Escape key
- **WHEN** 用户在重命名或创建模式下按下 Escape 键
- **THEN** 取消当前操作，恢复原状态

### Requirement: InlineInput cancels on blur
文件树中 InlineInput 组件的 `onBlur` 事件 SHALL 取消编辑，而非自动提交。

#### Scenario: Click outside during rename
- **WHEN** 用户正在重命名文件时点击文件树外部区域
- **THEN** 取消重命名操作，文件名恢复为原值

#### Scenario: Enter confirms rename
- **WHEN** 用户在重命名模式下按下 Enter 键
- **THEN** 确认提交新文件名

### Requirement: Empty folders are visible in file tree
空文件夹 SHALL 在文件树中可见并显示文件夹图标。

#### Scenario: Display empty folder
- **WHEN** 后端返回的文件列表中包含某个目录路径
- **THEN** 该目录在文件树中显示为可展开的文件夹节点，即使其下无文件

### Requirement: Folder delete has recursive confirmation
删除文件夹 SHALL 需要用户确认，并提示将删除所有子内容。

#### Scenario: Delete folder with confirmation
- **WHEN** 用户请求删除一个非空文件夹
- **THEN** 弹出确认对话框，提示"该文件夹下所有文件及子文件夹将被永久删除"
- **AND** 用户确认后，后端递归删除文件夹下所有文件
