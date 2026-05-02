## MODIFIED Requirements

### Requirement: Real compilation flow
点击编译按钮 SHALL 提交异步编译任务到后端，获取 `job_id` 后通过轮询获取编译结果。编译过程不阻塞 UI 交互。

#### Scenario: Successful compilation
- **WHEN** 用户点击编译按钮
- **THEN** 前端 POST `/latex/compile` 获取 `job_id`，显示"排队中"状态，轮询状态直到编译成功，成功后通过 `build_id` 加载 PDF

#### Scenario: Compilation failure
- **WHEN** 轮询到编译状态为 `failed` 或 `timeout`
- **THEN** 在预览区域显示编译错误信息（从 status 响应的 `error` 和 `logs` 字段提取），保留上一次成功的 PDF（如有）

#### Scenario: Cancel compilation
- **WHEN** 编译进行中（`pending` 或 `running`）用户点击停止按钮
- **THEN** 前端调用 DELETE `/latex/compile` 取消任务，状态变为 `cancelled`，恢复就绪状态

#### Scenario: Compilation already in progress
- **WHEN** 当前项目已有活跃编译任务，用户点击编译按钮
- **THEN** 按钮为禁用状态，不发送新的编译请求
