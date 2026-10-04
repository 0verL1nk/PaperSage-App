## 1. 数据与迁移

- [ ] 1.1 `server/schema/tables/projects.sql` 增 `scenario` 列；新增 `server/schema/tables/project_writing_states.sql`（真相源）
- [ ] 1.2 `server/migrations/{postgres,mysql}/0011_writing_workbench.{up,down}.sql`：up 加列建表 / down 反向；登记 PG 集成测试
- [ ] 1.3 store：`ProjectWritingState` 模型 + `AllModels()` 注册（SQLite AutoMigrate）；`make generate-dao` 生成 DAO

## 2. 后端：项目场景

- [ ] 2.1 projects 域创建/详情/列表 DTO 增 `scenario`（默认值 + 枚举校验 422）+ 单测

## 3. 后端：写作状态域

- [ ] 3.1 GET：默认合成 + 旧数据逐键合并 + 单测（缺省 / revision=0）
- [ ] 3.2 PUT：upsert + revision 乐观锁 + 409 附最新状态 + 单测
- [ ] 3.3 校验（枚举/体积 422、归属 404）、bootstrap 装配 + 路由契约测试

## 4. 前端服务层

- [ ] 4.1 `api-routes.ts` + `writing-state-api.ts`（查询/保存 hooks + 单测）
- [ ] 4.2 `types/projects.ts` 增 scenario；CreateProjectDialog 场景选择 + 测试更新

## 5. 前端信息架构与导航

- [ ] 5.1 WorkspaceLayout 阶段导航（准备/撰写/检查 + 资料库）+ i18n 词条
- [ ] 5.2 路由：新增 prepare/review；移除占位路由并重定向；根重定向改 prepare；同步 e2e 断言

## 6. 写作准备页

- [ ] 6.1 页面布局：左侧检查清单/待补项 + 右侧编辑卡片（响应式）
- [ ] 6.2 研究蓝图编辑 + 自动保存（防抖 + 保存状态）
- [ ] 6.3 课题要求条目编辑
- [ ] 6.4 材料概览（只读汇总 + 资料库入口）
- [ ] 6.5 待补项派生提示
- [ ] 6.6 409 冲突横幅：加载最新 / 覆盖保存

## 7. 检查完善页

- [ ] 7.1 骨架 + 阶段状态维护 + 边界说明（不伪造检查结果）

## 8. 测试与验收

- [ ] 8.1 组件/导航测试：保存、待补项、冲突、重定向
- [ ] 8.2 e2e：创建选场景 → 准备编辑刷新持久 → 交叉跳转 → 检查骨架
- [ ] 8.3 `make check` + `make test` 全绿；覆盖率门槛达标

## 9. 文档同步

- [ ] 9.1 `CLAUDE.md` 架构/命令与 `docs/PROJECT.md` 状态段落更新
