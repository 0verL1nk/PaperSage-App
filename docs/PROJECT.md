# PaperSage 项目文档

更新：2026-09-19。课题：基于大模型的科研论文写作AI辅助系统设计与实现。

## 当前定位

面向科研新手的前后端分离 Web 系统，以本科毕业论文、入门级学术小论文为两类场景，支持写作准备、内容撰写、检查完善三阶段。四个核心模块是用户管理、分场景工作台、全流程 AI 辅助及学术规范资源库。

本项目以本科毕业设计可完成、可解释、可测试为范围。已有原型、拟完善方案和验证结果必须分开描述，不能把框架或接口存在当作完整业务已实现。

## 推荐阅读顺序

| 文档 | 用途与状态 |
|---|---|
| [课题简介](DEFINITION.md) | 课题定位和核心任务 |
| [毕设总体逻辑与决策台账](project/13-thesis-product-model.md) | 当前产品主线、范围分层、数据/Agent 边界与待确认问题；持续更新 |
| [Agent 技能与知识空间治理](project/14-agent-skills-and-knowledge-governance.md) | Agent 路由、自主入库、证据追溯、代码/实验材料与 SSH 扩展边界 |
| [事件驱动写作与多维审阅](project/15-event-driven-writing-and-review-model.md) | 开放式 Agent 触发、参考文献范式学习、Review Profile 与论文质量维度库 |
| [调研驱动的实现方案](project/12-ai-writing-design.md) | 当前后续实施基线：场景、证据、批注式修改确认、版本、测试 |
| [开发路线图](project/11-roadmap.md) | 任务书时间节点与分阶段验收 |
| [开题报告](proposal/交付/开题报告正文（含参考文献）.md) | 面向科研新手全流程写作辅助的研究计划；测试由开发者自行开展 |
| [文献来源与阅读边界](proposal/交付/文献来源与阅读边界.md) | 研究依据及适用范围；17 篇学术文献与 3 项产品官方资料分列 |
| [OpenSpec 流程](agents/openspec-workflow.md) | 后续能力变更和验收管理 |

## 当前代码基础

以下按代码静态核对，未声称本轮完成联调或性能测试。

| 层次 | 代码入口 | 当前基础 |
|---|---|---|
| 前端 | [frontend](../frontend/) | React 19、TypeScript、Vite；TanStack Router/Query；写作页采用 Monaco 与 PDF 预览 |
| 后端 | [主入口](../server/cmd/papersage/main.go)、[装配](../server/internal/bootstrap/) | Go、Gin；模块化服务；不是旧 FastAPI 主线 |
| 数据与文件 | [SQL 表定义](../server/schema/tables/)、[内部模块](../server/internal/) | 关系数据库、GORM 相关访问和 S3 兼容对象存储；数据库/向量能力以实际环境验证为准 |
| 写作与编译 | [WritingPage](../frontend/src/views/projects/WritingPage.tsx) | 文件编辑、保存与编译预览已有基础；真实 AI 面板交互尚需接通 |
| AI 与检索 | [Agent](../server/internal/agent/)、[RAG](../server/internal/rag/) | 模型配置、流式会话、工具、混合检索和 wiki 已有原型；不代表用户确认、来源定位或标准融合均已完成 |

优先缺口：补两场景三阶段的状态与提示模板；建立来源版本和证据定位；校正检索融合；让稿件建议先以锚定文本的批注呈现，候选修改由用户确认；补运行恢复与分层验收。细节及代码依据见调研驱动方案。

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

本轮已更新项目文档与开题报告并完成文档提交；没有修改业务代码、执行数据库迁移或变更 OpenSpec 历史记录。
