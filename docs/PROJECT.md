# PaperSage 项目文档

更新：2026-10-07。课题：基于大模型的科研论文写作AI辅助系统设计与实现。

## 当前定位

面向科研新手的前后端分离 Web 系统，以本科毕业论文、入门级学术小论文为两类场景，支持写作准备、内容撰写、检查完善三阶段。四个核心模块是用户管理、分场景工作台、全流程 AI 辅助及学术规范资源库。

本项目以本科毕业设计可完成、可解释、可测试为范围。已有原型、拟完善方案和验证结果必须分开描述，不能把框架或接口存在当作完整业务已实现。

后续设计主线为：**澄清研究任务 → 整理和比较材料 → 形成可编辑的章节论证计划 → 按章节提供有据写作建议 → 用户确认修改 → 跨章节检查 → 返回补材料或修订。** AI 阅读记录、比较和章节摘要可自动保存为派生分析，正式研究事实和稿件变更由用户确认。用户可以从已有材料或正文直接开始，系统不要求先填完准备表格。

## 推荐阅读顺序

| 文档 | 用途与状态 |
|---|---|
| [课题简介](DEFINITION.md) | 课题定位和核心任务 |
| [毕设总体逻辑与决策台账](project/13-thesis-product-model.md) | 当前产品主线、范围分层、数据/Agent 边界与待确认问题；持续更新 |
| [Agent 技能与知识空间治理](project/14-agent-skills-and-knowledge-governance.md) | Agent 路由、自主入库、证据追溯、代码/实验材料与 SSH 扩展边界 |
| [写作准备：研究顾问对话](frontend/writing-preparation-agent.md) | 准备阶段对话、用户确认保存、游客边界与模板初始化 |
| [事件驱动写作与多维审阅](project/15-event-driven-writing-and-review-model.md) | 开放式 Agent 触发、参考文献范式学习、Review Profile 与论文质量维度库 |
| [调研驱动的实现方案](project/12-ai-writing-design.md) | 后续实施基线：具体任务契约、组织产物、证据、修改确认、依赖与分层评价 |
| [开发路线图](project/11-roadmap.md) | 课题时间节点与材料组织—章节写作—整合检查的分阶段验收 |
| [学术写作研究设计评审与采纳记录](project/16-literature-informed-design-review.md) | 文献借鉴边界、原始评审与2026-10-04的文档采纳记录 |
| [开题报告](proposal/交付/开题报告正文（含参考文献）.md) | 面向科研新手全流程写作辅助的研究计划；测试由开发者自行开展 |
| [文献来源与阅读边界](proposal/交付/文献来源与阅读边界.md) | 研究依据及适用范围；17 篇学术文献与 3 项产品官方资料分列 |
| [OpenSpec 流程](agents/openspec-workflow.md) | 后续能力变更和验收管理 |
| [Vercel Go 后端部署调研（2026-10-07）](backend/2026-10-07-vercel-go-backend-assessment.md) | Go/Gin 的 Vercel Go Runtime 配置与当前部署验证状态；生产 `/health` 已通过，业务链路仍待验证 |

## 已记录的代码基础

以下沿用2026-09-16至09-19的静态核对记录；本次只同步设计，未重新评估代码完成程度。代码可能继续变化，进入实现前须核对当前入口和行为，不能把此表当作实时验收结果。

| 层次 | 代码入口 | 当前基础 |
|---|---|---|
| 前端 | [frontend](../frontend/) | React 19、TypeScript、Vite；TanStack Router/Query；写作页采用 Monaco 与 PDF 预览 |
| 后端 | [主入口](../server/cmd/papersage/main.go)、[装配](../server/internal/bootstrap/) | Go、Gin；模块化服务；不是旧 FastAPI 主线 |
| 数据与文件 | [SQL 表定义](../server/schema/tables/)、[内部模块](../server/internal/) | 关系数据库、GORM 相关访问和 S3 兼容对象存储；数据库/向量能力以实际环境验证为准 |
| 写作与编译 | [WritingPage](../frontend/src/views/projects/WritingPage.tsx) | 文件编辑、保存与编译预览已有基础；真实 AI 面板交互尚需接通 |
| AI 与检索 | [Agent](../server/internal/agent/)、[RAG](../server/internal/rag/) | 模型配置、流式会话、工具、混合检索和 wiki 已有原型；不代表用户确认、来源定位或标准融合均已完成 |

后续实施工作：补两场景三阶段的状态与提示；建立阅读记录、文献比较、章节计划和摘要；补来源定位、论断支持评估及实际依赖记录；校正检索融合；以批注呈现建议并确认候选修改；实现跨章节检查、运行恢复与分层验收。提示评价沿用正式报告约30个固定任务，检索与流程评价另行记录。细节见调研驱动方案；这些均是计划，不是本次交付的运行功能。

## 2026-10-07 实现与验证快照

本轮 worktree 已实现双场景项目创建、写作准备—撰写—检查三阶段导航与持久化状态、准备页和检查页；`docs/agents/handoff-2026-10-07.md` 记录本次收尾状态。游客入口部署在正式网站，但游客项目、写作状态、资料和 LaTeX 源文件只保存在本浏览器的 IndexedDB；不创建服务端匿名账号，也不在游客编辑时上传项目内容。注册或登录后，用户可预览并确认将本地项目导入当前账号，失败时保留本地副本。跨设备恢复、清除浏览器站点数据后的找回不可用；AI 生成和在线编译需登录。存储、导入与隐私边界见[本地游客工作区文档](frontend/local-guest-workspace.md)；设计提案见 `openspec/changes/online-guest-workspace/`。

登录/注册页采用缓慢漂移的 Aurora 渐变背景并遵循 `prefers-reduced-motion`；登录卡内的游客按钮直接初始化浏览器存储并进入工作台，不再经过单独说明页的二次点击。认证页和账号工作台顶部均明确显示“后端 在线/本地”切换，手机端也保留可读的文字状态。手机体验以 320px 起验：认证和游客入口 E2E 覆盖 320×640、390×844、768×1024、1024×768、1440×900，检查横向溢出、表单与主操作的视口可达性和 44px 触控高度；工作台另检查固定导航遮挡、编辑器宽度、移动标签和键盘操作。Vercel 前端使用 `frontend/vercel.json` 将 SPA 深链接回退至 `index.html`，支持刷新任意前端子路由。E2E 使用 mock API 验证布局与状态流，认证页面也经浏览器视口实测。全量验证由 `go vet ./...`、Go 单元测试、前端 typecheck/lint/Vitest、响应式 Playwright 及隔离 PostgreSQL/MySQL 迁移集成测试组成。外部邮件实际送达、生产模型/CLSI/对象存储配置和真实部署环境仍需单独验收，不因本地测试通过而视为已验证。

认证新增邮箱密码找回：通用申请响应避免账号枚举，已验证邮箱通过 15 分钟一次性 token 重置密码；重置会撤销旧会话。实现依据、接口、字段迁移和限速的多实例边界见[密码找回安全说明](backend/password-recovery.md)与[前端认证流程](frontend/auth-flow.md)。生产 schema 需在部署服务启动迁移后生效，真实邮件投递仍需实际收件箱验收。

## 开发入口

前端：

```bash
cd frontend
pnpm install
pnpm dev
```

后端（先按实际环境配置数据库、对象存储、编译与可选模型服务；这里只说明入口，不代表无需配置即可运行）：

```bash
cd server
go run ./cmd/papersage
```

测试入口：前端 `pnpm test`、`pnpm test:e2e`、`pnpm typecheck`；后端在 `server` 下 `go test ./...`。类型检查不等于功能测试；测试所需外部依赖、凭据和网络条件需另行准备。

## 历史文档边界

[早期总设计](DESIGN.md)、[旧需求](project/02-requirements.md)、[旧架构](project/03-architecture.md)、[旧后端设计](backend/DESIGN.md)、[旧测试设计](project/09-testing.md) 已添加历史状态提示。其他早期章节：[背景](project/01-background.md)、[前端](project/04-frontend.md)、[后端](project/05-backend.md)、[数据库](project/06-database.md)、[API](project/07-api.md)、[业务流程](project/08-workflows.md)、[部署](project/10-deployment.md) 也须结合现有代码核对，不能整套当作当前契约。

旧文档保留以追踪设计演变，本轮未逐项重写其全部示例。涉及 FastAPI、SQLAlchemy、Celery、旧目录、未经验证的部署兼容性及过期排期时，以当前代码事实和新的明确计划为准。设计计划与当前代码不一致之处意味着待实施，不意味着代码已改变。

本轮同步项目总览及11—16号设计文档。正式报告、PPT、业务代码、数据库迁移与历史 OpenSpec 状态不在本次修改范围；新增接口和跨模块行为进入实现时另立变更提案。
