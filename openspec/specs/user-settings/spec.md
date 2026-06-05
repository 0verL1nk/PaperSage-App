# user-settings Specification

## Purpose
TBD - created by archiving change add-user-settings. Update Purpose after archive.
## Requirements
### Requirement: User settings storage
系统 SHALL 提供 `user_settings` 表，按 (user_id, category) 存储 JSONB 设置数据，支持灵活扩展无需加迁移。

#### Scenario: New user has no stored settings
- **WHEN** 新用户首次查询设置
- **THEN** 系统返回所有分类的默认值，不创建数据库行

#### Scenario: User updates a category
- **WHEN** 用户更新 `ui` 分类的 `theme` 字段为 `"dark"`
- **THEN** 系统在 `user_settings` 表 upsert 一行 `(user_id, "ui", {"theme": "dark"})`，其他字段保留默认值

### Requirement: Get all settings
系统 SHALL 提供端点 `GET /api/v1/settings` 返回当前用户所有分类设置。

#### Scenario: Get all settings with defaults merge
- **WHEN** 用户请求所有设置，数据库仅有 `ui` 分类行 `{"theme": "dark"}`
- **THEN** 系统返回 `{"ui": {"theme": "dark", "language": "zh-CN", "sidebar_collapsed": false}, "editor": {...默认值}, "agent": {...默认值}}`

### Requirement: Get single category
系统 SHALL 提供端点 `GET /api/v1/settings/{category}` 返回指定分类设置。

#### Scenario: Get existing category
- **WHEN** 用户请求 `editor` 分类，无存储数据
- **THEN** 系统返回 `{"font_size": 14, "tab_size": 2, "auto_save": true, "auto_save_interval": 30, "word_wrap": true}`

#### Scenario: Get unknown category
- **WHEN** 用户请求不存在的分类名如 `unknown`
- **THEN** 系统返回 HTTP 404

### Requirement: Patch update category (merge)
系统 SHALL 提供端点 `PATCH /api/v1/settings/{category}` 合并更新指定分类，只改传入字段。

#### Scenario: Patch single field
- **WHEN** 用户发送 `PATCH /settings/ui` body `{"theme": "dark"}`
- **THEN** 仅 `theme` 被更新为 `"dark"`，`language` 和 `sidebar_collapsed` 保留原值（或默认值）

#### Scenario: Patch with invalid field type
- **WHEN** 用户发送 `PATCH /settings/editor` body `{"font_size": "huge"}`
- **THEN** 系统返回 HTTP 422，Pydantic 验证错误

### Requirement: Put replace category (full replace)
系统 SHALL 提供端点 `PUT /api/v1/settings/{category}` 整体替换指定分类。

#### Scenario: Full replace
- **WHEN** 用户发送 `PUT /settings/ui` body `{"theme": "light", "language": "en"}`
- **THEN** 该分类存储变为 `{"theme": "light", "language": "en"}`，`sidebar_collapsed` 字段不再存在于存储中（读取时回退到默认值）

### Requirement: Reset category to defaults
系统 SHALL 提供端点 `DELETE /api/v1/settings/{category}` 重置指定分类为默认值。

#### Scenario: Reset category
- **WHEN** 用户发送 `DELETE /settings/ui`
- **THEN** 系统删除 `(user_id, "ui")` 行，后续读取返回默认值

### Requirement: Default settings constant
系统 SHALL 在 `schemas/user_settings.py` 中定义 `DEFAULT_SETTINGS` 字典，包含 `ui`、`editor`、`agent` 三个分类的默认值。

#### Scenario: New setting key added to defaults
- **WHEN** 开发者在 `DEFAULT_SETTINGS["ui"]` 中新增 `"compact_mode": false`
- **THEN** 所有用户读取设置时自动获得该默认值，无需数据库迁移

### Requirement: Pydantic validation per category
系统 SHALL 对每个分类定义 Pydantic schema，在 PATCH/PUT 时验证字段类型和范围。

#### Scenario: Editor font_size out of range
- **WHEN** 用户发送 `font_size: 1`（小于最小值）
- **THEN** 系统返回 HTTP 422 验证错误

#### Scenario: Agent temperature out of range
- **WHEN** 用户发送 `temperature: 3.0`（超出 [0, 2] 范围）
- **THEN** 系统返回 HTTP 422 验证错误

### Requirement: Frontend settings page
系统 SHALL 在前端提供设置页面，路由为 `/app/settings`，使用标签页组织三个分类（通用、编辑器、AI 助手）。

#### Scenario: Navigate to settings
- **WHEN** 用户点击侧边栏 Settings 链接
- **THEN** 导航到 `/app/settings`，显示通用设置标签页

#### Scenario: Toggle auto-save
- **WHEN** 用户在编辑器标签页关闭 auto_save 开关
- **THEN** 前端乐观更新 UI，后台 PATCH `/settings/editor` 发送 `{"auto_save": false}`

### Requirement: Agent config integration
系统 SHALL 让 DeerFlow agent 的模型选择通过 `user_settings` 的 `agent` 分类持久化。

#### Scenario: User selects model via agent API
- **WHEN** 用户调用 `PUT /api/v1/agent/models/deepseek`
- **THEN** 系统内部调用 settings service 写入 `agent.model_name = "deepseek"`

#### Scenario: User changes model via settings page
- **WHEN** 用户在设置页面 AI 助手标签选择不同模型
- **THEN** `agent.model_name` 被更新，下次对话使用新模型

