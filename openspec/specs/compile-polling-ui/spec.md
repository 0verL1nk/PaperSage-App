## ADDED Requirements

### Requirement: Compile status indicator
WritingPage SHALL 展示当前编译状态指示器，包含四种视觉状态：空闲、排队中、编译中、完成（成功/失败）。

#### Scenario: Idle state
- **WHEN** 没有活跃的编译任务
- **THEN** 编译按钮显示为可点击状态，无额外状态指示

#### Scenario: Queued state
- **WHEN** 编译任务提交成功，状态为 `pending`
- **THEN** 编译按钮变为禁用状态，显示"排队中"文字和排队动画，按钮旁显示排队位置

#### Scenario: Compiling state
- **WHEN** 编译任务状态变为 `running`
- **THEN** 状态指示器显示"编译中"动画（旋转图标），显示已用时间

#### Scenario: Success state
- **WHEN** 编译任务状态变为 `success`
- **THEN** 状态指示器短暂显示成功标记，然后自动加载 PDF

#### Scenario: Failed state
- **WHEN** 编译任务状态变为 `failed` 或 `timeout`
- **THEN** 状态指示器显示失败标记，展开错误日志面板

### Requirement: Async compile submission
前端提交编译 SHALL 立即返回，不阻塞 UI 交互。

#### Scenario: Submit and continue editing
- **WHEN** 用户点击编译按钮
- **THEN** 前端 POST 提交编译请求，获取 `job_id` 后立即启用轮询，用户可继续编辑文件

#### Scenario: Duplicate submission prevention
- **WHEN** 当前项目已有 `pending` 或 `running` 状态的编译任务
- **THEN** 编译按钮显示为禁用状态，用户无法重复提交

### Requirement: Polling with auto-stop
前端 SHALL 使用 TanStack Query 的 `refetchInterval` 轮询编译状态，完成后自动停止。

#### Scenario: Polling starts after submit
- **WHEN** 编译任务提交成功，返回 `job_id`
- **THEN** 前端开始每 1 秒轮询一次状态

#### Scenario: Polling interval changes
- **WHEN** 编译状态从 `pending` 变为 `running`
- **THEN** 轮询间隔从 1 秒调整为 2 秒

#### Scenario: Auto-stop on completion
- **WHEN** 编译状态变为终态（`success` / `failed` / `timeout` / `cancelled`）
- **THEN** 轮询自动停止

### Requirement: Error log display
编译失败时 SHALL 展示错误日志，支持折叠/展开。

#### Scenario: Show error summary
- **WHEN** 编译状态变为 `failed`
- **THEN** 在 PDF 预览区域显示错误摘要（前 5 行错误）

#### Scenario: Expand full logs
- **WHEN** 用户点击"查看完整日志"
- **THEN** 展开显示完整编译日志，支持滚动查看

#### Scenario: Timeout error message
- **WHEN** 编译状态变为 `timeout`
- **THEN** 显示"编译超时，请检查文档复杂度或联系管理员"提示
