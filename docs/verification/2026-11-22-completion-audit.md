# 学术规范资源库完成度审计（HELE-25）

> 按验收标准逐条记录**实际验证结果**，不以「接口存在」代替「业务可用」。
> 最近更新：2026-11-25（第 1 批后端 + 第 2 批前端 + 固定版本展示修复）

## 一、验收标准对照

| 验收项 | 状态 | 证据 |
|---|---|---|
| 数据库迁移、条目 CRUD API、项目采用/取消 API，`go test` 通过 | **已完成** | 迁移 `0024_academic_norm_library`（PG+MySQL up/down）；`server/internal/norms/`；`go test ./internal/...` 全绿；`postgres migration integration` 在 PR #77 CI 通过 |
| 本科毕业论文与学术小论文两套默认种子，每条带来源出处 | **已完成** | `server/internal/norms/builtin/*.json`：15 条 + 12 条；测试断言 `sourceRef` 非空且 `sourceQuote`/`sourceUrl` 至少其一；无出处时启动失败 |
| 前端闭环：查看/新增/编辑/删除条目 → 项目采用 → 写作/检查侧查看出处追溯 | **部分完成** | 规范库管理页 `/app/norms` 与准备页「学术规范」卡片已可用；**写作建议 `normRefs` 与检查报告 `findings[].basis` 的规范出处展示未实现** |
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

- 统一上下文装配 `internal/contextpack`：目前仅由 `norms.Provider` 把已采用规范交给跨章节检查工具，写作建议链路尚未注入规范。
- 写作建议 `propose_edit` 的 `normRefs` 与建议卡规范 chip。
- 检查报告 `chapter_review` 的 `findings[].basis` 规范引用与页面出处展开。
- 外部模型服务的端到端验证：本机与本地验收环境未配置模型，检查页如实显示「跨章节检查不可用」。
- `GET /projects/:id/norms/effective`（含上下文预算截断）。
- 条目导入（JSON/CSV + dry-run）。
- 主操作 ≤3 秒的实测数据。

## 五、边界

不做外部网页爬虫抓取、不做跨用户公共规范市场、不做多级审批流；内置种子不含任何学校专属条款，学校要求由用户自建规范包补充。
