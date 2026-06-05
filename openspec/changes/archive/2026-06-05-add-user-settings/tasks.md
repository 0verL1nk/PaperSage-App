## 1. 数据模型

- [x] 1.1 创建 `backend/models/user_settings.py`：UserSettings 模型（user_id, category, settings JSONB, updated_at）
- [x] 1.2 更新 `backend/models/__init__.py` 导出 UserSettings
- [x] 1.3 创建 Alembic 迁移：`alembic revision --autogenerate -m "add user settings table"`

## 2. Schema 层

- [x] 2.1 创建 `backend/schemas/user_settings.py`：DEFAULT_SETTINGS 常量（ui、editor、agent 三个分类的默认值）
- [x] 2.2 创建每个分类的 Pydantic schema：UISettings、EditorSettings、AgentSettings（用于验证输入）
- [x] 2.3 创建请求/响应 schema：SettingsUpdateRequest、SettingsResponse、AllSettingsResponse
- [x] 2.4 更新 `backend/schemas/__init__.py` 导出新 schema

## 3. 服务层

- [x] 3.1 创建 `backend/services/settings_service.py`：SettingsService 类
- [x] 3.2 实现 `get_all_settings(user_id)` 方法：读取所有分类，与默认值合并
- [x] 3.3 实现 `get_category(user_id, category)` 方法：读取单分类，与默认值合并
- [x] 3.4 实现 `update_category(user_id, category, settings)` 方法：upsert，Pydantic 验证后合并存储
- [x] 3.5 实现 `replace_category(user_id, category, settings)` 方法：upsert，整体替换
- [x] 3.6 实现 `reset_category(user_id, category)` 方法：删除该分类行（读取时自动回退到默认值）

## 4. API 路由

- [x] 4.1 创建 `backend/api/routes/settings.py` 路由骨架，挂载到 /api/v1/settings
- [x] 4.2 实现 GET /settings — 返回所有分类设置
- [x] 4.3 实现 GET /settings/{category} — 返回单分类设置
- [x] 4.4 实现 PATCH /settings/{category} — 合并更新
- [x] 4.5 实现 PUT /settings/{category} — 整体替换
- [x] 4.6 实现 DELETE /settings/{category} — 重置为默认值
- [x] 4.7 更新 `backend/api/__init__.py` 注册 settings 路由

## 5. 前端服务层

- [x] 5.1 创建 `frontend/src/services/settings-api.ts`：settings API client（getSettings, getCategory, updateCategory, resetCategory）
- [x] 5.2 更新 `frontend/src/services/api-routes.ts` 添加 settings 路由常量

## 6. 前端 Store

- [x] 6.1 创建 `frontend/src/store/settings-store.ts`：Zustand store（缓存设置、按分类更新、乐观更新）

## 7. 前端设置页面

- [x] 7.1 创建 `frontend/src/views/settings/` 目录
- [x] 7.2 创建 `SettingsPage.tsx`：设置页面主组件（标签页布局：通用、编辑器、AI 助手）
- [x] 7.3 创建 `GeneralSettings.tsx`：UI 设置标签页（主题切换、语言选择、侧边栏折叠）
- [x] 7.4 创建 `EditorSettings.tsx`：编辑器设置标签页（字体大小、自动保存、自动换行等）
- [x] 7.5 创建 `AgentSettings.tsx`：AI 助手设置标签页（模型选择、温度、写作风格、记忆开关）
- [x] 7.6 更新 `frontend/src/app/router.tsx`：将 `/app/profile` 替换/重定向到设置页面，或新增 `/app/settings` 路由
- [x] 7.7 更新侧边栏导航：确保 Settings 链接指向正确路由

## 8. 测试

- [x] 8.1 创建 `backend/tests/test_settings_api.py`：测试 settings API 端点（CRUD、默认值合并、Pydantic 验证）
- [x] 8.2 创建 `backend/tests/test_settings_service.py`：测试 SettingsService 方法（upsert、合并、重置）
- [ ] 8.3 添加前端设置页面组件测试（可选）
