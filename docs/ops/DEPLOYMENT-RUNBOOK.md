# 生产部署运行手册（脱敏模板）

更新：2026-11-28。本文是公开仓库中的**脱敏部署模板**，描述通用部署拓扑、发布与回滚、核对方式，与 `docs/backend/DEPLOYMENT.md` 的交付边界/首次安装说明互补。

本文用占位符代指实际取值：`<API_DOMAIN>`（API 公网域名）、`<FRONTEND_DOMAIN>`（前端生产域名）、`<FRONTEND_ALIAS>`（前端别名/跳转域名）、`<APP_DIR>`（版本目录根）、`<APP_STATE_DIR>`（运行状态目录）、`<ENV_FILE>`（环境文件）、`<APP_USER>` / `<DEPLOY_USER>`（运行用户 / 部署用户）、`<DEPLOY_HOME>`（部署用户主目录）、`<DEPLOY_BIN>`（发布命令安装目录）、`<VERCEL_TEAM>/<VERCEL_PROJECT>`。实际取值只在服务器与 Vercel 控制台内维护，不写入本仓库。

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

用户 ─► Cloudflare ─► <API_DOMAIN> ─► Caddy（TLS/SSE）─► 127.0.0.1:5478
                                     │                              papersage.service（非 root）
                                     └─ 主库 / 迁移直连 / R2 / 模型 / 邮件凭据在 <ENV_FILE>

用户 ─► https://<FRONTEND_ALIAS>（307）─► https://<FRONTEND_DOMAIN>（Vercel 生产）
```

## 2. 组件与位置

| 组件 | 位置 | 说明 |
|---|---|---|
| 前端 | Vercel 项目 `<VERCEL_TEAM>/<VERCEL_PROJECT>` | 生产别名 `<FRONTEND_DOMAIN>`（`<FRONTEND_ALIAS>` 307 跳转）；构建根目录 `frontend/`，`frontend/vercel.json` 定义 install/build/输出与 SPA rewrite |
| API 二进制 | 服务器 `<APP_DIR>/releases/vX.Y.Z/` | root-owned 不可变版本目录（`papersage`、`migrate`、`manifest.json`）；`<APP_DIR>/current` 符号链接指向运行版本 |
| API 进程 | systemd `papersage.service` | 以 `<APP_USER>` 用户运行；`WorkingDirectory=<APP_STATE_DIR>`；环境文件 `<ENV_FILE>`（root-only） |
| 反向代理 | Caddy + `deploy/Caddyfile` | `<API_DOMAIN>` 终止 TLS、刷新 SSE；仅回源 `127.0.0.1:5478` |
| 源站防护 | `papersage-cloudflare-guard.service` | nftables 仅放行 Cloudflare 网段到 80/443；Caddy 依赖该表启动 |
| 发布入口 | `<DEPLOY_BIN>/papersage-release` | 从 `deploy/papersage-release.py` 安装；只允许 `activate/rollback`，不接受任意 shell |
| 部署用户 | `<DEPLOY_USER>`（非 root） | 只能写 `<DEPLOY_HOME>/uploads/`，通过 sudoers 调发布命令 |
| 数据库 | 云端 PostgreSQL（主库） | 应用用池化 `DATABASE_URL`；迁移用直连 `DATABASE_MIGRATION_URL`；均只存在于服务器环境文件 |
| 对象存储 | Cloudflare R2（S3 兼容） | 上传/PDF/导出；`S3_*` 仅服务器持有 |
| 缓存 | 可选 Redis | 未配置时进程内内存实现 |
| CLSI / TeX | **不部署** | 生产 `LATEX_CLSI_ENABLED=false`，编译默认走浏览器 WASM；随仓 CLSI/render 服务与 `papersage-clsi` 镜像已于 2026-11-26 删除（PR #109），**不再构建**，服务器不运行 TeX |
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
#    active: vX.Y.Z <完整 commit>
gh run view <release-run-id> --repo 0verL1nk/PaperSage-App --log | grep "active:"

# 2) 公网只返回健康状态（按设计不暴露版本/提交）
curl -s https://<API_DOMAIN>/health          # {"status":"healthy"}

# 3) 鉴权边界仍在
curl -s -o /dev/null -w '%{http_code}\n' https://<API_DOMAIN>/api/v1/projects   # 401

# 4) 前端可用
curl -s -o /dev/null -w '%{http_code}\n' -L https://<FRONTEND_DOMAIN>/
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

## 7. 现网状态

线上版本、提交与生产核对记录只在私有运维环境维护，不在公开仓库记录；核对方法与判据见第 3 节。
