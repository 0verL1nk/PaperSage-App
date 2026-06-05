## Why

PaperSage 目前没有任何用户偏好系统 — User 模型只有账号信息，没有设置列；前端 `/app/profile` 是空占位符。用户无法自定义编辑器行为、UI 偏好、AI 助手配置等。DeerFlow agent 集成也需要持久化每用户的模型选择和写作偏好。需要一个统一的用户设置系统，覆盖 PaperSage 和 Agent 的所有可配置项。

## What Changes

- 新增 `user_settings` 数据库表：按分类存储用户偏好（JSONB），通用 key-value 结构
- 新增 `/api/v1/settings/*` REST API 路由组：读取、更新、重置用户设置
- 新增前端设置页面：替换 `/app/profile` 占位符，分类标签页展示各项设置
- Agent 集成变更中的 `PUT /models/{model_name}` 改为写入 `user_settings` 表

## Capabilities

### New Capabilities

- `user-settings`: 用户设置管理 — 按分类读写设置、重置为默认值、批量导入导出

### Modified Capabilities

- `agent-config`: 每用户模型选择等偏好通过 `user-settings` 能力持久化，不再依赖内存状态

## Impact

- **后端代码**：新增 `models/user_settings.py`、`schemas/user_settings.py`、`api/routes/settings.py`
- **数据库**：新增 `user_settings` 表（1 张表，JSONB 存储）
- **API**：新增 `/api/v1/settings/*` 路由前缀，需要认证
- **前端**：新增设置页面组件、Zustand settings store、API service 层
- **依赖**：agent 集成变更依赖此变更的 settings API 来持久化 agent 偏好
