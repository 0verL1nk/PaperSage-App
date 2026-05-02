## Context

PaperSage 后端使用 FastAPI + SQLAlchemy async + PostgreSQL。User 模型仅有账号字段（username、email、password_hash），无偏好设置。前端 `/app/profile` 路由存在但是空占位符。项目没有任何用户级别的配置持久化机制。

DeerFlow agent 集成需要存储每用户的模型偏好（如选择 deepseek 还是其他模型），这也需要用户设置系统支撑。

## Goals / Non-Goals

**Goals:**
- 提供统一的用户设置存储（后端表 + API）
- 支持按分类组织设置（ui、editor、agent 等）
- 前端设置页面可读写所有分类
- JSONB 存储允许灵活扩展，无需频繁加迁移

**Non-Goals:**
- 管理员级全局设置（仅用户级）
- 设置版本历史/审计
- 设置的细粒度权限控制（用户只能改自己的）

## Decisions

### D1: 存储结构 — 分类 JSONB

**选择**: 单表 `user_settings`，按 (user_id, category) 存储分类设置 JSONB。

```sql
user_settings (
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category  VARCHAR(100) NOT NULL,
    settings  JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, category)
)
```

**设置分类和默认值**:

| category | 默认值 | 说明 |
|---|---|---|
| `ui` | `{"theme": "system", "language": "zh-CN", "sidebar_collapsed": false}` | UI 偏好 |
| `editor` | `{"font_size": 14, "tab_size": 2, "auto_save": true, "auto_save_interval": 30, "word_wrap": true}` | 编辑器设置 |
| `agent` | `{"model_name": "deepseek", "temperature": 0.7, "writing_style": "academic", "memory_enabled": true}` | AI 助手设置 |

**替代方案**:
- 在 User 模型加 `preferences JSONB` 列 → 单列存所有设置，无法按分类隔离更新
- 独立的 `user_setting` 表（每行一个 key-value）→ 查询复杂，JSONB 更适合嵌套结构

**理由**: 按分类隔离使得 PATCH 更新只影响一个分类，前端可按需加载。JSONB 允许任意嵌套结构，新增设置项不需要改表结构。

### D2: API 设计

**端点**:

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/settings` | 获取当前用户所有分类设置 |
| GET | `/api/v1/settings/{category}` | 获取指定分类设置 |
| PATCH | `/api/v1/settings/{category}` | 合并更新指定分类（只改传入的字段） |
| PUT | `/api/v1/settings/{category}` | 替换指定分类（整体覆盖） |
| DELETE | `/api/v1/settings/{category}` | 重置指定分类为默认值 |

**PATCH vs PUT**:
- `PATCH` 合并更新：`{"theme": "dark"}` 只改 theme，保留其他字段
- `PUT` 整体替换：传入完整设置，覆盖该分类所有字段

**理由**: PATCH 是前端设置页最常用的操作（改一个选项），PUT 用于"恢复默认"前的全量保存。

### D3: 默认值管理

**选择**: 在 `schemas/user_settings.py` 中定义 `DEFAULT_SETTINGS` 常量，API 层读取时与默认值合并。

```python
DEFAULT_SETTINGS: dict[str, dict[str, Any]] = {
    "ui": {"theme": "system", "language": "zh-CN", "sidebar_collapsed": False},
    "editor": {"font_size": 14, "tab_size": 2, "auto_save": True, "auto_save_interval": 30, "word_wrap": True},
    "agent": {"model_name": "deepseek", "temperature": 0.7, "writing_style": "academic", "memory_enabled": True},
}
```

**理由**: 默认值在代码中集中管理，不存数据库。读取时 `defaults | stored` 合并，确保新增设置项自动有默认值。

### D4: 前端设置页面

**选择**: 替换现有 `/app/profile` 占位符为设置页面，使用标签页组织分类。

**页面结构**:
- 路由：`/app/settings`（新建）或复用 `/app/profile`（改为 redirect）
- 标签页：通用设置（ui）、编辑器设置（editor）、AI 助手（agent）
- 每个设置项用 shadcn/ui 的 Switch/Select/Slider 组件
- 使用 Zustand store + TanStack Query 管理状态

**理由**: 复用现有导航结构（侧边栏已有 Settings 图标链接），按分类标签页组织清晰。

### D5: 与 Agent 集成的关系

**选择**: agent 集成变更的 `PUT /models/{model_name}` 端点内部调用 settings API 写入 `agent.model_name`。

**流程**:
```
PUT /api/v1/agent/models/deepseek
  → settings_service.update(user_id, "agent", {"model_name": "deepseek"})
  → AgentService 刷新该用户的模型配置
```

**理由**: 设置是单一数据源，agent 配置端点只是 settings API 的语义化封装。

## Risks / Trade-offs

- **[JSONB 无 schema 约束]** → 前端可能写入任意字段。缓解：API 层用 Pydantic schema 验证每个分类的输入。
- **[分类数量有限]** → 初始 3 个分类足够，后续可扩展。分类名在代码中常量化管理。
- **[并发写入]** → 同一用户同时改多个分类不存在冲突（行级锁），同一分类的并发 PATCH 需要乐观锁或 last-write-wins。缓解：初始版本用 last-write-wins。
