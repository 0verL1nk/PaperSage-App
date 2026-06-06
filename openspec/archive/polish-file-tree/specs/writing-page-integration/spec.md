## MODIFIED Requirements

### Requirement: File move operation supports root directory
移动文件操作 SHALL 支持目标目录为根目录（空字符串），不应返回验证错误。

#### Scenario: Move file to root directory
- **WHEN** 用户拖拽文件到文件树的根目录区域
- **THEN** 前端发送 `new_dir` 为空字符串的移动请求
- **AND** 后端接受空字符串作为合法的根目录路径
- **AND** 文件被移动到项目根目录

#### Scenario: Move file to subdirectory
- **WHEN** 用户拖拽文件到某个子文件夹
- **THEN** 前端发送 `new_dir` 为该文件夹路径的移动请求
- **AND** 文件被移动到目标文件夹

## ADDED Requirements

### Requirement: Deleting active file auto-selects next file
删除当前正在编辑的活跃文件 SHALL 自动选中下一个可用文件。

#### Scenario: Select sibling after delete
- **WHEN** 用户删除当前正在编辑的文件，且同目录存在其他文件
- **THEN** 编辑器自动切换到同目录的下一个文件

#### Scenario: Fallback to main.tex after delete
- **WHEN** 用户删除当前正在编辑的文件，且同目录无其他文件，但项目存在 main.tex
- **THEN** 编辑器自动切换到 main.tex

#### Scenario: Empty state after delete
- **WHEN** 用户删除项目中的最后一个文件
- **THEN** 编辑器显示空状态

### Requirement: Edit toolbar is removed
WritingPage 中的无功能 Edit Toolbar（B/I/U/cite/ref 占位按钮）SHALL 被移除。

#### Scenario: No edit toolbar in writing page
- **WHEN** 用户打开论文撰写页
- **THEN** 编辑器上方不显示 B/I/U/cite/ref 工具栏按钮

### Requirement: Binary file error shows toast instead of console.error
BinaryFilePlaceholder 中遇到错误时 SHALL 使用 toast 提示用户，而非 console.error。

#### Scenario: Open binary file shows toast
- **WHEN** 用户打开一个二进制文件（如图片、PDF）
- **AND** 文件加载失败
- **THEN** 显示 toast 错误提示，不输出 console.error

### Requirement: Folder recursive delete via backend API
删除文件夹 SHALL 通过后端 API 递归删除文件夹下所有文件和子文件夹。

#### Scenario: Delete non-empty folder
- **WHEN** 用户删除一个包含文件的文件夹
- **THEN** 前端弹出确认对话框，提示将删除所有子内容
- **AND** 用户确认后调用后端文件夹删除端点
- **AND** 后端递归删除该文件夹下所有文件

#### Scenario: Backend folder delete endpoint
- **WHEN** 前端发送 `DELETE /api/latex-files/{project_id}/folder?path={folder_path}`
- **THEN** 后端查找该文件夹下所有文件并逐一删除
- **AND** 返回删除的文件数量
