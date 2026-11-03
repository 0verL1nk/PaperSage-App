# 后端部署配置

## 交付边界

前端独立部署在 Vercel；Go 只交付 API，不构建、复制或 embed `frontend/dist`，未知前端路径返回 JSON 404。小服务器运行一个 Go 进程及 HTTPS 反代；数据库和对象存储继续使用云端服务。

正式发布采用版本化二进制 + systemd，不在服务器安装 Go、Node、Docker、本地数据库、Redis 或 TeX Live。浏览器编译沿用前端 WASM；外置 CLSI 不属于本机的默认部署。

## 构建与版本

- Go 版本以 `server/go.mod` 为准，CI 使用 `actions/setup-go` 读取该文件。
- `make build-server` 仅构建 API；前端由 Vercel 单独构建。
- `.goreleaser.yml` 构建 `papersage` 与一次性迁移命令 `migrate`，归档和 `checksums.txt` 发布到 GitHub Release。
- 正式版本来自 `vMAJOR.MINOR.PATCH` 标签；标签指向的提交必须已在 `main` 中。不要重打已发布标签或覆盖同版本产物。
- `internal/buildinfo` 的 version、commit、build time 通过链接参数注入。公网 `/health` 只返回 `{"status":"healthy"}`；version/commit/buildTime/goVersion 仅在本机直连（不经过 Caddy）时返回，供发布脚本校验，避免公网指纹信息泄露。

```bash
make build-server
```

## CI/CD

`.github/workflows/release.yml` 的标签发布顺序：

1. 检查版本格式与主线提交归属。
2. 复用 Server CI、Frontend CI、E2E；任何门禁失败都不发布。
3. 用固定版本 GoReleaser 构建 API/migrate 及校验和，不进行前端打包。
4. 将同一次运行的 Linux amd64 产物交给生产部署 job，先验证 checksum。
5. 通过专用受限 SSH 用户上传并调用服务器上的 root-owned 发布命令。
6. 在本机直连的 `/health` 上核验 version 和完整 commit（公网代理请求只返回健康状态），不只检查 HTTP 200。

Server CI 当前覆盖率回归门槛为48%，80%是逐步提升目标；发布不得降低门槛或跳过失败检查。正式发布串行执行，普通分支与 PR 不访问生产部署凭据。

GitHub Environment `production-server` 所需配置：

| 类型 | 名称 | 用途 |
|---|---|---|
| Secret | `SERVER_SSH_KEY` | 本项目专用部署私钥，不使用个人 root 私钥 |
| Secret | `SERVER_KNOWN_HOSTS` | 经人工核对的 SSH 主机公钥，不在部署时无条件信任扫描结果 |
| Variable | `SERVER_HOST` | 服务器地址 |
| Variable | `SERVER_USER` | 受限用户 `papersage-deploy`，不使用 root |

数据库、R2、邮件和模型运行凭据只保存在服务器，不作为发布 job 的环境输入。SSH 私钥只写入临时 runner 文件，步骤结束即删除。

## 服务器布局与权限

| 路径 | 用途与权限 |
|---|---|
| `/opt/papersage/releases/vX.Y.Z/` | root-owned 不可变版本目录，包含 API、migrate 与 manifest |
| `/opt/papersage/current` | root-owned 当前版本符号链接 |
| `/usr/local/sbin/papersage-release` | 从 `deploy/papersage-release.py` 安装的受限 root 发布命令 |
| `/var/lib/papersage-deploy/uploads/` | 仅部署用户可写的归档上传目录 |
| `/etc/papersage/papersage.env` | root-only 运行配置，由 systemd 读取并传给进程 |
| `/var/lib/papersage/` | 应用用户 `papersage` 的可写工作目录 |

参考模板：`deploy/papersage.service`、`deploy/papersage-deploy.sudoers`、`deploy/Caddyfile`。首次安装必须检查实际主机、现有服务和发行版软件包事务；不要覆盖其他应用的配置、已有 SSH 公钥或运行凭据。

日常部署用户只能上传归档并通过 sudo 调用经审查的发布命令，不授予任意 shell、systemctl 或环境变量注入权限。应用以非 root 账号运行。

## 运行配置

从当前生产环境安全迁移配置，不直接复制可能过期的 worktree `.env`。至少确认主库连接、直接迁移连接、认证签名密钥、R2、邮件和模型设置；迁移时保持认证密钥一致，避免使现有登录态及数据失效。不要将值打印到日志或提交到仓库。

```dotenv
HOST=127.0.0.1
PORT=5478
DEBUG=false
RUN_DB_MIGRATIONS=false
LATEX_CLSI_ENABLED=false
DATA_DIR=/var/lib/papersage
```

保留 `DB_MAX_OPEN_CONNS`、`DB_MAX_IDLE_CONNS` 等已有连接池配置，基于数据库与内存实测调整，不按请求创建连接池。不要为了预热而持续查询云数据库，造成不必要的运行费用。

`AGENT_DOCUMENT_READ_LIMIT_BYTES` 默认为 0，不限制单次文献分段大小；部署者可明确设置正数字节数作为可选单次预算。超过预算会明确报错，可使用较小分段续读，不截断或限制完整文档的总长度。未指定 `max_bytes` 或其值为 0 时仍只返回读取信息，不自动向模型发送全文。
`AGENT_MAX_ITERATIONS` 默认为 20，是 Eino DeepAgent 的显式迭代上限（框架不接受 0 表示无限）；达到上限时界面会提示可发送“继续”接着处理，部署者可按模型成本调整。

`DATABASE_MIGRATION_URL` 可使用直接数据库连接，`DATABASE_URL` 可使用池化连接。发布命令通过独立受限进程运行 `migrate`；服务启动不执行迁移。版本发布前必须审查待执行 DDL，破坏性变更应先备份并单独确认。

发布前应只读检查 `schema_migrations.version/dirty`，确认当前版本对应的原始迁移已被发布包收录。若数据库领先于主线，先核对实际列、索引和约束并恢复遗漏的迁移历史；不要通过 `force`、重写版本号或执行 down 来掩盖差异。迁移 0019–0021 的恢复仅对齐既有 schema，不启用历史分支的执行租约、项目清理或对象清理业务。

## HTTPS、SSE 与首次切换

`deploy/Caddyfile` 为 `api.paper.overlink.top` 提供 HTTPS，反代到 `127.0.0.1:5478`，及时刷新 SSE，不缓存整条 AI 回复。

首次切换顺序：

1. 完成 API-only 构建、服务配置、数据库/R2连通与本机回环健康验证。
2. 验证新二进制的版本、提交和取消/失败语义。
3. 确认安全组允许必要的80/443端口，再将 API 域名指向新服务器并验证证书。
4. 从 Vercel 页面验证真实登录、配置读取、SSE和 local/online 环境选择。

域名和证书切换不是每次 CD 的一部分。配置未确认、DNS未切换或浏览器验收未完成时，不得宣称生产迁移成功。真实对话测试只使用获授权账号，不把模拟流当成真实模型结果。

## 激活与回滚

发布 helper 只接受稳定版标签、完整 commit 和 SHA-256；对上传归档做私有快照与 checksum 验证，拒绝路径穿越、符号/硬链接及多余文件。相同版本只有 commit 与 checksum 完全一致时才能重试，不覆盖已安装的二进制。

激活时先停止接收新运行并排空在途任务，执行新版本迁移，然后原子切换当前版本并启动探活。新版本启动/健康校验失败时恢复之前的程序和链接；首次发布没有旧版本时明确失败并保持停止。

在 GitHub Actions 的 Release 工作流选择手动运行，填写已安装的 `rollback_version` 可回滚。回滚只切换已存在的程序版本，**不执行数据库 down 迁移**。只有已确认与当前 schema 兼容的旧版本才能回退。

小服务器不运行双实例蓝绿部署，发布会有短维护窗口。正常关机等待本实例 AI 运行完成；超出排空预算时取消并落明确终态。进程强杀/崩溃不等于具备自动断点续跑能力。

## 验证与性能基线

发布脚本在隔离临时目录运行回归测试，覆盖无效参数、坏 checksum、危险归档、健康失败恢复、首次失败、同版本重试与回滚不迁移。

```bash
python -m unittest discover -s deploy -p 'test_*.py' -v
```

```bash
go -C server test ./internal/... -count=1
```

2026-10-28 从部署服务器对主目录当前配置进行只读测量（不含真实业务负载）：

| 测量 | 样本与结果 |
|---|---|
| 主库池化连接 `SELECT 1` | 20次串行，P50 55.1ms、P95 57.9ms，最大109.9ms |
| 主库首次连接+查询 | 1.03s；新连接3次中位数0.82s |
| 向量库 | TCP可达，但数据库认证失败，SQLSTATE 28P01；未取得SQL时延 |

这些数据仅是网络/连接基线，不是接口P95、模型首字或并发性能结论。上线前还需记录空闲与单对话的进程RSS、CPU、首字延迟、事件写入次数及业务接口P50/P95。若向量库仍认证失败，应显式记录RAG未验收，而不是静默降级后称其正常。
