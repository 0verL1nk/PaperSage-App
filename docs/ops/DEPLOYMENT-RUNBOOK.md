# 生产部署运行手册

更新：2026-11-22。本文描述**当前实际部署**（拓扑、位置、发布与回滚、核对方式），与 `docs/backend/DEPLOYMENT.md` 的交付边界/首次安装说明互补；两者冲突时以本文的现网状态为准，并在变更后同步更新。

本文在公开仓库，因此用占位符代指服务器与 Vercel 上的实际路径和账号名：`<APP_DIR>`（版本目录根）、`<APP_STATE_DIR>`（运行状态目录）、`<ENV_FILE>`（环境文件）、`<APP_USER>` / `<DEPLOY_USER>`（运行用户 / 部署用户）、`<DEPLOY_HOME>`（部署用户主目录）、`<DEPLOY_BIN>`（发布命令安装目录）、`<VERCEL_TEAM>/<VERCEL_PROJECT>`。实际取值只在服务器与 Vercel 控制台内维护。

## 1. 拓扑

```text
GitHub 0verL1nk/PaperSage-App (main)
  ├─ push main ──► Server CI / Frontend CI / E2E / Docker / Desktop
  ├─ tag vX.Y.Z ─► Release 工作流
  │                  validate ─► goreleaser(linux amd64: papersage + migrate)
  │                  ─► release（GitHub Release 资产）
  │                  ─► deploy（SSH，仅 production-server environment 有凭据）
  │                        scp tar.gz ─► <DEPLOY_HOME>/uploads/
  │                        sudo <DEPLOY_BIN>/papersage-release activate <tag> <commit> <sha256>
  │                          stop ─► migrate ─► switch current ─► start ─► loopback /health 校验
  └─ 前端 ───────────► Vercel 自动构建（根目录 frontend，ignoreCommand 跳过无前端变更）

用户 ─► Cloudflare ─► api-papersage.overlink.top ─► Caddy（TLS/SSE）─► 127.0.0.1:5478
                                     │                              papersage.service（非 root）
                                     └─ 主库 / 迁移直连 / R2 / 模型 / 邮件凭据在 <ENV_FILE>

用户 ─► https://papersage.overlink.top（307）─► https://paper.overlink.top（Vercel 生产）
```

## 2. 组件与位置

| 组件 | 位置 | 说明 |
|---|---|---|
| 前端 | Vercel 项目 `<VERCEL_TEAM>/<VERCEL_PROJECT>` | 生产别名 `paper.overlink.top`（`papersage.overlink.top` 307 跳转）；构建根目录 `frontend/`，`frontend/vercel.json` 定义 install/build/输出与 SPA rewrite |
| API 二进制 | 服务器 `<APP_DIR>/releases/vX.Y.Z/` | root-owned 不可变版本目录（`papersage`、`migrate`、`manifest.json`）；`<APP_DIR>/current` 符号链接指向运行版本 |
| API 进程 | systemd `papersage.service` | 以 `<APP_USER>` 用户运行；`WorkingDirectory=<APP_STATE_DIR>`；环境文件 `<ENV_FILE>`（root-only） |
| 反向代理 | Caddy + `deploy/Caddyfile` | `api-papersage.overlink.top` 终止 TLS、刷新 SSE；仅回源 `127.0.0.1:5478` |
| 源站防护 | `papersage-cloudflare-guard.service` | nftables 仅放行 Cloudflare 网段到 80/443；Caddy 依赖该表启动 |
| 发布入口 | `<DEPLOY_BIN>/papersage-release` | 从 `deploy/papersage-release.py` 安装；只允许 `activate/rollback`，不接受任意 shell |
| 部署用户 | `<DEPLOY_USER>`（非 root） | 只能写 `<DEPLOY_HOME>/uploads/`，通过 sudoers 调发布命令 |
| 数据库 | 云端 PostgreSQL（主库） | 应用用池化 `DATABASE_URL`；迁移用直连 `DATABASE_MIGRATION_URL`；均只存在于服务器环境文件 |
| 对象存储 | Cloudflare R2（S3 兼容） | 上传/PDF/导出；`S3_*` 仅服务器持有 |
| 缓存 | 可选 Redis | 未配置时进程内内存实现 |
| CLSI / TeX | **不部署** | 生产 `LATEX_CLSI_ENABLED=false`，编译默认走浏览器 WASM；`papersage-clsi` 镜像只作为可选产物构建到 GHCR，服务器不运行 TeX |
| 桌面端 | GitHub Release 资产 | `papersage-windows-amd64-installer.exe` + `SHA256SUMS`，由桌面工作流打包，随同一次 Release 发布 |

## 3. 发布一个版本

1. 确认要发布的提交已经在 `main`，且该提交的 CI check-runs 为绿（`Server CI`、`Frontend CI`、`E2E`、`Docker` 等）。
   - **注意**：只改文档/OpenSpec 的提交不会触发这些工作流，因此没有 check-run；把标签打在它上面会让 validate 一直等到超时。
2. 打标签并推送：

```bash
git tag -a vX.Y.Z -m "vX.Y.Z: <变更摘要>" <commit>
git push origin vX.Y.Z
```

3. `Release` 工作流自动执行：
   - `validate`：标签格式、必须是 `main` 祖先、该提交的 GitHub Actions check-runs 全部成功；
   - `server-build` / `desktop`：goreleaser 产出 Linux amd64 归档与 `migrate`，桌面端产出 Windows 安装包；
   - `release`：校验校验和后创建 GitHub Release（已发布资产只读，不覆盖）；
   - `deploy`：在 `production-server` environment 中校验归档 checksum，`scp` 到上传目录，SSH 调用 `sudo -n <DEPLOY_BIN>/papersage-release activate`。
4. 服务器上的激活顺序（`deploy/papersage-release.py`）：
   - `systemctl stop papersage.service`（排空在途 AI 运行）；
   - 以 `<APP_USER>` 用户、独立 systemd 单元运行新版本 `migrate`（`RUN_DB_MIGRATIONS=false`，应用启动不迁移）；
   - 原子切换 `<APP_DIR>/current`，启动服务；
   - 轮询 `http://127.0.0.1:5478/health`（45s），要求 `status=healthy` 且 `version`、`commit` 与 manifest 完全一致；
   - 任一步失败：回到旧版本、`systemctl reset-failed`、重启旧版本并再次校验；首次发布失败则保持停止。

### 发布后核对（必须）

```bash
# 1) 部署任务日志应出现：
#    active: v0.1.17 <完整 commit>
gh run view <release-run-id> --repo 0verL1nk/PaperSage-App --log | grep "active:"

# 2) 公网只返回健康状态（按设计不暴露版本/提交）
curl -s https://api-papersage.overlink.top/health          # {"status":"healthy"}

# 3) 鉴权边界仍在
curl -s -o /dev/null -w '%{http_code}\n' https://api-papersage.overlink.top/api/v1/projects   # 401

# 4) 前端可用
curl -s -o /dev/null -w '%{http_code}\n' -L https://paper.overlink.top/
```

版本/提交只能通过服务器本机回环 `/health` 或部署日志核对；公网 `/health` 不返回版本信息是刻意设计。

## 4. 回滚

- 在 GitHub Actions 手动运行 `Release` 工作流，填写 `rollback_version`（例如 `v0.1.16`），执行 `<DEPLOY_BIN>/papersage-release rollback`。
- 只切换已安装的程序版本，**不执行数据库 down 迁移**。只有当旧版本与当前 schema 兼容时才能回退；破坏性迁移需先备份并单独评估。
- 服务器单实例，没有蓝绿双活；发布有短暂维护窗口。

## 5. 凭据与配置位置

| 位置 | 内容 |
|---|---|
| GitHub Environment `production-server` | 服务器连接信息与 SSH 私钥（仅 deploy/rollback job 可读） |
| 服务器环境文件 `<ENV_FILE>` | 数据库（池化 + 迁移直连）、R2、邮件、模型 provider 兜底、监听地址与端口、迁移与 CLSI 开关 |
| Vercel 项目环境变量 | `VITE_API_BASE_URL` 等前端公开配置；不含服务端密钥 |

仓库与日志中不出现密钥、连接串、服务器路径或账号名；发布 job 的 SSH 私钥写入临时文件并在步骤结束删除。具体路径与账号只在服务器与 GitHub Environment 内维护。

## 6. 常见故障

| 现象 | 原因与处理 |
|---|---|
| Release `validate` 一直 `waiting for CI` 后超时 | 标签指向的提交没有 CI check-run（通常只改了文档）。把标签移到有绿色 CI 的代码提交后重推；已发布的标签不要覆盖，先确认没有产生 Release |
| E2E 在安装浏览器阶段失败：`Playwright does not support chromium on ubuntu26.04-x64` | `ubuntu-latest` 已迁移到 26.04；`.github/workflows/e2e.yml` 已固定 `ubuntu-24.04`，勿改回 latest |
| 部署用户 SSH `Permission denied (publickey)` | 部署用户的 `~/.ssh` 目录 755、`authorized_keys` 644；不要 chmod 700/600（sshd 以目标用户身份读取，过严权限会让公钥认证全部失败） |
| 公网 `/health` 没有 version/commit | 预期行为；只有本机直连（不经 Caddy）才返回构建信息 |
| 发布后新接口 404 / 前端提示加载失败 | 前后端版本错位：先确认部署日志 `active:` 的版本与 commit，再补发版本；Vercel 会自动部署 main，但后端只在 Release 后更新 |
| 迁移失败导致激活回滚 | 查看 `journalctl -u papersage-migrate-*`；迁移在独立 systemd 单元中运行，有运行时限；不要执行 down 迁移掩盖差异 |
| 桌面自动更新下载失败 | Windows 桌面端问题，见桌面工作流/Release 资产；与 API 部署无关 |

## 7. 当前状态（2026-11-22）

- `main`：以 `git log -1 origin/main` 为准（随开发前进，不在本文写死）；线上后端对应的仍是 `cce81e1`（v0.1.17）。
- 服务端：**v0.1.17 @ `cce81e1`**。部署日志：`active: v0.1.17 cce81e1b321821d3117f8100ca0fb43cde4e71fc`（2026-11-22 08:55:28 UTC）；激活时执行了 0022（来源版本）与 0023（科研组织产物）迁移。
- 数据库：`schema_migrations` 版本 **23**；`research_artifacts`、`research_artifact_revisions`、`research_artifact_dependencies` 三表已存在（0 行）。发布时无需再补跑 0022/0023。
- 生产核对：`/health` 200 `{"status":"healthy"}`；匿名 `/api/v1/projects` 401；前端 `paper.overlink.top` 200。
- 科研组织产物路由已上线，正确路径是 `GET|POST /api/v1/projects/{project_id}/research-artifacts`；修订/采用/拒绝在 `/api/v1/research-artifacts/{artifact_id}/revisions`、`/adopt`、`/reject`，删除是 `DELETE /api/v1/research-artifacts/{artifact_id}`（无尾段）。**没有**裸 `GET /api/v1/research-artifacts`。
  - 探测时区分两种 404：`{"detail":"Not Found"}` 是路由不存在（路径拼错时出现）；带 token 请求正确路径返回信封 `{"code":404,"data":null,"info":"项目 not found"}`，说明路由存在、只是项目不存在。
- 前端：Vercel 跟随 `main` 自动构建；后端版本以部署日志 `active:` 为准，两者仅在 Release 后对齐。
- CLSI：未部署；`papersage-clsi` 镜像构建成功但不参与生产请求。
- 学校论文模板（HIT hithesis）仍是手工导入/桌面端使用；浏览器 WASM 资源目录覆盖不足时需走桌面端完整 TeX 资源。
