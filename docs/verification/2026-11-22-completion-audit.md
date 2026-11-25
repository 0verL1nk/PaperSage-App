# 学术规范资源库完成度审计（HELE-25）

> 按验收标准逐条记录**实际验证结果**，不以「接口存在」代替「业务可用」。
> 最近更新：2026-11-25（第 1–3 批全部交付；写作建议规范引用随 PR #83 合入）

## 一、验收标准对照

| 验收项 | 状态 | 证据 |
|---|---|---|
| 数据库迁移、条目 CRUD API、项目采用/取消 API，`go test` 通过 | **已完成** | 迁移 `0024_academic_norm_library`（PG+MySQL up/down）；`server/internal/norms/`；`go test ./internal/...` 全绿；`postgres migration integration` 在 PR #77 CI 通过 |
| 本科毕业论文与学术小论文两套默认种子，每条带来源出处 | **已完成** | `server/internal/norms/builtin/*.json`：15 条 + 12 条；测试断言 `sourceRef` 非空且 `sourceQuote`/`sourceUrl` 至少其一；无出处时启动失败 |
| 前端闭环：查看/新增/编辑/删除条目 → 项目采用 → 写作/检查侧查看出处追溯 | **已完成（待 PR #83 合并）** | 规范库管理页 `/app/norms`、准备页采用抽屉、检查报告「本次生效的规范依据」展开区（PR #82）；写作建议卡引用号 chip + 条款/出处展开（PR #83）。建议侧引用号由服务端按本次注入集合解析，未命中即拒绝，无正则/关键词判断 |
| 刷新/重启后采用状态不丢；主操作 ≤3 秒 | **持久化已验证；性能未测** | 2026-11-25 黑盒：刷新与重启后端后项目仍显示已采用/已启用；未采集 ≤3 秒响应时间数据 |
| 同步 `docs/PROJECT.md` 与本文档 | **已完成** | 本文件；`docs/PROJECT.md` 已更新学术规范库现状 |

## 二、已修复缺陷

- **采用列表展示最新修订而非生效版本**（2026-11-25 黑盒发现，阻断级）
  - 现象：条目采用时为 v1，编辑出 v2 后，准备页「管理」里显示 v2 的标题与规则，同时提示「按采用时的版本生效」，用户会误以为 v2 已在项目中生效。
  - 原因：`server/internal/norms/adoptions.go` 的采用响应复用了「当前版本」的条目摘要，只把 `Version` 标为采用版本。
  - 修复：按采用记录锁定的版本号精确取 `norm_versions` 内容；回归测试断言采用后改条目，列表仍返回 v1 内容，同时 `latestVersion=2`、`hasNewerVersion=true`。
  - 前端配合：徽标显示「已启用 v1」，并明确「这里是目录里的最新 v2；本项目仍按 v1 生效」。

## 三、已执行的验证命令

```text
# 后端
cd server && gofmt -l .                                   # 空
cd server && go vet ./...                                 # 通过
cd server && go test ./internal/... -count=1 -p 2         # 全部 ok
cd server && go test ./internal/norms/ -cover             # coverage: 73.0%

# 前端
cd frontend && pnpm typecheck                             # 通过
cd frontend && pnpm lint                                  # 0 error（2 条既有 warning）
cd frontend && pnpm exec vitest run src/views/norms \
  src/services/norms-api.test.ts \
  src/components/domain/NormLibraryPanel.test.tsx         # 3 文件 / 12 测试通过

# 真实 Postgres 迁移
GitHub Actions: server-ci / postgres migration integration  # passed（PR #77）
```

## 四、明确未完成（不勾选）

- 统一上下文装配 `internal/contextpack`：目前用 `norms.Provider` 分别把已采用规范交给写作提示（`systemPromptFor`）与跨章节检查上下文，尚未抽出统一装配器。
- 检查报告 `findings[].basis` 里规范引用号的逐条校验（当前由 `input.norms` 整体承载依据）。
- 外部模型服务的端到端验证：本机与本地验收环境未配置模型，检查页如实显示「跨章节检查不可用」。
- `GET /projects/:id/norms/effective`（含上下文预算截断）。
- 条目导入（JSON/CSV + dry-run）。
- 主操作 ≤3 秒的实测数据。

## 四之二、本地验收环境怎么进得去

本地后端默认 `EMAIL_PROVIDER=noop`，走「注册」会创建账号但发不出验证邮件，
而邮箱注册用户在未验证时登录返回 403（`Please verify your email before logging in`）。
本地验收请任选其一：

1. **用种子账号**：`cd server && go run ./cmd/seed`，账号 `e2euser` / 口令见 `server/cmd/seed/main.go`（E2E 同款，无需邮箱验证）；
2. **不填邮箱注册**：`POST /api/v1/auth/register` 只带 `account` 与 `password`，该路径不进入邮箱验证门槛，注册后可直接登录；
3. 需要走邮件流程时，配置真实 `EMAIL_PROVIDER`（SMTP/Resend）并完成验证链接。

真实模型链路的验收需要配置 `AGENT_API_KEY` / `AGENT_BASE_URL` / `AGENT_MODEL`（或用户设置里的服务商），否则检查页会如实提示「模型审阅不可用」。

## 五、边界

不做外部网页爬虫抓取、不做跨用户公共规范市场、不做多级审批流；内置种子不含任何学校专属条款，学校要求由用户自建规范包补充。
