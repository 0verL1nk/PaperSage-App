# OpenSpec 变更管理流程

> 状态：中央速查手册。详细 AI 指引见 `.codex/skills/openspec-*`。

## 1. 适用与不适用

| 场景 | 走 OpenSpec | 备注 |
|------|------------|------|
| 新增能力（新表、新路由、新页面、新工作流）| ✅ | 必走 |
| 跨多个模块的能力修改 | ✅ | 必须有 capability 影响面 |
| 破坏性 API 变更 | ✅ | 旧 spec 要 `# MODIFIED` 标注 |
| 依赖关系重构 | ✅ | |
| 单一 bug 修复、文案、配置微调 | ❌ | 直接 commit |
| 纯重构（无行为变化）| ❌ | commit 即可 |
| 文档、CSS、样式调整 | ❌ | |

## 2. 目录与生命周期

```text
openspec/
├── config.yaml                     # 项目级配置
├── changes/                        # 进行中的变更
│   ├── <change-name>/             # 提案
│   │   ├── proposal.md            # Why / What Changes / Capabilities
│   │   ├── design.md              # 技术设计（可选）
│   │   ├── tasks.md               # 拆解任务 + 进度勾选
│   │   └── specs/                 # 变更引入的增量 spec
│   │       └── <capability>/spec.md
│   └── archive/                   # 已完成的变更（按日期归档）
│       └── YYYY-MM-DD-<change-name>/
└── specs/                          # 已稳定的正式 spec（按 capability 聚合）
    └── <capability>/spec.md
```

**生命周期**：

```text
propose → apply → tasks 100% → validate → archive
                              ↑
                  spec 同步合并到 specs/<capability>/
```

## 3. 提案文件骨架

### `proposal.md`（必填）

```markdown
## Why
<问题 / 现状>

## What Changes
- <要点 1>
- <要点 2>

## Capabilities
### New Capabilities
- **<capability-id>**: <一句话描述>

### Modified Capabilities
- **<capability-id>**: <影响面>

## Impact
- 后端/前端/数据库/依赖 各写一句
```

### `tasks.md`（必填）

- 拆成 1-2 小时内的可勾选任务
- 按依赖顺序编号：1.x 数据模型 → 2.x schema → 3.x 服务 → 4.x API → ...
- `[x]` 完成的打勾，不要删
- 末尾可保留"可选"项（如 8.3）

### `specs/<capability>/spec.md`（每个新 capability 一份）

```markdown
## ADDED Requirements

### Requirement: <一句话陈述>
The system SHALL ...

#### Scenario: <场景 A>
- **WHEN** ...
- **THEN** ...

#### Scenario: <场景 B>
- ...
```

> 用 SHALL / MUST / SHOULD 等 RFC 2119 关键词，不要用"会"、"可以"。

## 4. 常用命令

```bash
# 列出进行中的变更
openspec list

# 查看一个变更的状态（哪些 artifact 缺）
openspec status add-user-settings

# 校验 proposal/spec/tasks 是否合规
openspec validate add-user-settings

# 完成后归档（移动到 archive/，合并 spec 到 specs/）
openspec archive add-user-settings --yes

# 启动交互式 dashboard
openspec view
```

> `--skip-specs` 用于纯 infra/工具/文档类变更，跳过 spec 合并。`--no-validate` 不推荐。

## 5. 验收前 Checklist

- [ ] `openspec validate <name>` 通过
- [ ] `tasks.md` 所有非"可选"任务都打勾
- [ ] 代码与 spec 的 Requirement/Scenario 一一对应（每条都有测试或代码位置支撑）
- [ ] PR 引用 change 名称
- [ ] 跑 `openspec archive <name> --yes`，确认 `openspec/list` 不再显示该 change

## 6. 反模式

- ❌ **change 留着不 archive**：超过 2 周未归档的 change 要么拆小要么 close
- ❌ **spec 写得过于具体**：spec 是契约，不是实现细节；不要把内部字段都写进 Scenario
- ❌ **proposal 只写 What 不写 Why**：未来读者不知道这个 change 解决什么问题
- ❌ **tasks.md 全空勾**：未做的也打勾，archive 时会被 `--no-validate` 警告但允许——别骗自己
- ❌ **改 archive 里的历史变更**：那是历史记录；要做新工作就开新 change
