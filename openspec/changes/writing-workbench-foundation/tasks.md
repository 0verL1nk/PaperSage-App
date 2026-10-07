## 1. 数据与迁移

- [x] 1.1 `projects.scenario` SQL 真相源、`project_writing_states` 表结构与 Go 模型已加入；`AllModels()` 覆盖 SQLite AutoMigrate。
- [x] 1.2 PostgreSQL/MySQL `0014_writing_workbench` up/down 已加入；PostgreSQL 与 MySQL 8.4 fresh/handoff migration integration 实跑通过（四项均绿，最终业务表数 27）。
- [x] 1.3 SQLite 模型建表与 schema drift 已由后端测试覆盖。该域直接使用 GORM 查询/事务，不新增 `gorm.io/gen` DAO，不需要 `make generate-dao`。

## 2. 后端：项目场景

- [x] 2.1 create/list/detail DTO 返回 `scenario`；默认值与枚举校验（422）由 projects 契约测试覆盖。
- [x] 2.2 线上游客账号的项目数量受 `GUEST_MAX_PROJECTS` 限制（默认 3）；超限返回 429，普通用户不受该限制。测试覆盖 guest 与普通账号。

## 3. 后端：写作状态域

- [x] 3.1 GET 合成 `revision=0` 默认状态；兼容逐键合并旧 JSON，包括保留默认数组应对旧字段 `null`。
- [x] 3.2 PUT 首次插入 / revision 递增 / 乐观锁冲突 409 最新状态均有契约测试。
- [x] 3.3 枚举、字段长度、请求体体积校验（422）、项目归属（404）及 bootstrap 路由装配已实现并有 SQLite API 测试。

## 4. 前端服务层

- [x] 4.1 `api-routes.ts`、查询/保存 hooks、409 状态读取与 service 测试已完成。
- [x] 4.2 项目场景类型及创建选择已完成；创建对话框测试验证选定场景随请求提交。

## 5. 前端信息架构与导航

- [x] 5.1 WorkspaceLayout 阶段导航为准备、撰写、检查与资料库；中英文 i18n 已更新，移动端导航按钮满足 44px 最小高度并暴露 `aria-current`。
- [x] 5.2 新增 prepare/review 路由，项目根落地 prepare；旧 classifying / logic-chain / generating URL 兼容重定向至 prepare，并更新路由 manifest 与 E2E 断言。

## 6. 写作准备页

- [x] 6.1 左侧准备清单/待补项、右侧研究内容卡片采用响应式单列/双列布局。
- [x] 6.2 研究蓝图防抖自动保存、修订号与保存状态已完成。
- [x] 6.3 课题要求支持新增、编辑、删除。
- [x] 6.4 材料数量/最近更新时间只读汇总，并提供资料库入口。
- [x] 6.5 待补项从主题、问题、目标、方法、要求与已确认材料即时派生，不阻断阶段跳转。
- [x] 6.6 revision 冲突保留本地输入，支持加载最新版本或以最新 revision 覆盖保存。

## 7. 检查完善页

- [x] 7.1 页面展示并允许用户循环更新三阶段状态；清楚说明不自动判定论文质量、不展示伪造的检查结论。

## 8. 测试与验收

- [x] 8.1 Prepare/Review/API 与项目场景组件测试覆盖待补项、自动保存、409、本地阶段更新和场景提交。
- [x] 8.2 Playwright route-mock 测试覆盖项目根/旧路由重定向、准备内容保存刷新、工作台及资料库/LaTeX 页跨阶段浏览；mock 未命中的 API 请求会中止并断言为空，不访问开发数据库。
- [x] 8.3 响应式 Playwright 检查手机/平板/桌面 viewport：320×640、390×844、768×1024、1024×768、1440×900；覆盖 PreparePage、ReviewPage、参考资料库、LaTeX 写作页的横向溢出、主内容与 header/mobile tabs 遮挡、编辑器宽度、移动导航触控高度与键盘 Enter/Tab/`aria-current`。
- [x] 8.4 PostgreSQL/MySQL 真实 migration integration（fresh install + baseline handoff）已在隔离服务实跑通过。
- [ ] 8.5 最终整库验证已在 2026-10-07 复跑：`go vet ./...`、`go test ./...`、前端 typecheck/lint、Vitest（214 passed、1 skipped）、build 均通过；响应式/游客/密码找回 Playwright 7 passed。Server CI 的 Linux/Windows race+shuffle 与 PG/MySQL migration job 通过；Linux 覆盖率门槛实际失败（48.1%，要求 80%），同机对合并前 `origin/main` 测得 43.5%。本分支已提升基线但仍未达到门槛，不能记为通过。CI 发现的 5 项 lint 问题已在后续提交修复，本地 golangci-lint v2.14.0 为 0 issues；需要在新提交上复跑云端 workflow。整库覆盖率目标仍待达到或经维护者单独调整策略。

## 9. 文档同步

- [x] 9.1 `CLAUDE.md` 已同步三阶段路由、写作状态服务契约与线上游客会话入口。
- [x] 9.2 `docs/PROJECT.md` 已同步线上浏览器本地游客、用户确认导入、写作工作台阶段、响应式范围与尚未完成的生产验收边界。
