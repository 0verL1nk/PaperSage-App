# 交付与协作规范

更新：2026-11-08。适用于功能、可靠性与体验类变更；与 [OpenSpec 流程](openspec-workflow.md) 和 [Issue 跟踪](issue-tracker.md) 配套。

## 1. 角色与职责

| 角色 | 负责 | 交付物 |
|---|---|---|
| 产品经理（Lead） | 范围、优先级、验收口径、跨角色集成 | OpenSpec 变更、GitHub 总控 Issue、验收矩阵 |
| 后端 | 数据/接口/运行时、迁移、契约测试 | 可运行的后端改动与测试 |
| 前端 | 动线、状态机、UI/UX、可访问性 | 可运行的前端改动与测试 |
| 测试 | 单元/契约/E2E、真实环境验收 | 测试用例、失败记录、证据 |
| 运营/交付 | 部署、文档、发布、线上核验 | 部署记录、验收报告、归档 |

Lead 负责拆解、集成和最终验收，不把所有实现包办；并行任务必须文件域隔离。

## 2. 需求管理：GitHub Issues + Milestone + Project

- 每个可独立交付的工作流建一个 Issue；一个变更建一个总控 Issue 链接全部子 Issue。
- 标签至少包含：`area:*`、`role:product|backend|frontend|test|ops`、`priority:P0|P1|P2`。
- 每个交付批次建一个 Milestone，写明截止日期；Issue 必须挂到 Milestone。
- 有 `project` scope 时，把总控与子 Issue 加入 GitHub Project，用 Status/角色字段跟踪；没有 scope 时先维护 Issue/Milestone，授权后补录。
- Issue 的验收标准必须可观察；不要把“代码已写”当作完成。

## 3. OpenSpec 与 Issue 的对应

- 跨模块、新接口、新表、新流程走 OpenSpec；单一 bug/文案/样式直接提交。
- 变更目录：`openspec/changes/<name>/`，包含 `proposal.md`、`design.md`、`tasks.md`、`specs/`。
- 提交前运行 `openspec validate <name> --strict`。
- `tasks.md` 的分节与 GitHub 子 Issue 一一对应；勾选任务时同步 Issue checklist。
- 规格写实际可执行的契约；不得把没有写入点的状态、未接线的配置写进 spec。

## 4. 分支、PR 与合并

- 从 `origin/main` 建分支；分支名 `feat|fix|docs/<scope>`。
- 不直接推 main；通过 PR 合并，使用 squash merge。
- PR 必须通过 Server CI、Frontend CI、Playwright E2E 与依赖检查；失败先修，不绕过门禁。
- 工作树里未完成的实验性改动不得混入 PR；提交前逐项核对 `git status`。
- PR 标题用 `type(scope): 简述`；描述包含 Summary、Test plan、未验收/风险。

## 5. 并行协作

- 并行 agent 必须有明确文件所有权；同一包/同一文件不要多人同时改。
- 后端与前端按契约并行：先冻结接口字段，再各自实现。
- 测试可以在契约冻结后并行写；真实运行放到实现落地后。
- 集成前由 Lead 检查 diff、跑全量测试，并对用户可见路径做实际体验。

## 6. 验收与“实际体验”

- 模拟 SSE、mock API、单元测试只能证明实现符合契约，不能证明产品可用。
- 功能完成后必须实际启动应用，用浏览器走完主路径：登录 → 配置模型 → 对话 → 工具 → 建议 → 取消 → 刷新 → 断网 → 重连 → 切会话。
- 真实模型验收必须使用真实 provider；记录请求、响应、截图或日志，并脱敏。
- 失败、未执行、受限条件如实记录；不得用旧成功截图代替当前版本证据。

## 7. PR 清理策略

- 绿色且范围清晰的依赖/CI 升级可以合并；合并前确认没有跨文件冲突。
- 被取代的 PR（如旧部署方案）关闭并写明替代路径。
- major 依赖升级需要单独迁移与回归，不在功能 PR 中顺带处理；关闭或转为独立 Issue。
- 长期挂起的 dependabot PR 要给出处置理由，避免仓库噪音。

## 8. 沉淀

- 每次交付把新规则写回本文件或对应领域文档，并在会话记忆中保留跨会话要点。
- 不重复已有全文；交接优先给文件路径与增量结论。
