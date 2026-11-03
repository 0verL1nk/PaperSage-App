# PaperSage 发布预检记录

记录日期：2026-10-30（本地日期；GitHub操作时间以下注明UTC）。这是阶段性证据，不是上线完成声明。

## 版本与授权

- 迁移修复：[0verL1nk/GraduateProject#40](https://github.com/0verL1nk/GraduateProject/pull/40)。
- 合并前核对head `8f66f1db14576c28a91faa7b5d9cd7173baa02bf`：12项通过、1项跳过、无失败/待定，mergeable且CLEAN。
- 用户单独确认合并后于 `2026-10-29T15:58:21Z` squash进入main，合并提交 `6c7f2a814d381732bff58c31459ced54bd5bf10c`。
- 本轮生产预检获用户明确授权：仅`SELECT 1`/`schema_migrations`、模型`GET /models`及R2专用前缀一个对象的写/读/清理；不查用户内容、不发邮件、不付费生成、不切DNS。

## 配置一致性

以主目录 `server/.env` 为权威源，与服务器 `/etc/papersage/papersage.env` 比较相关值：主库、迁移直连、向量、Agent/嵌入配置、R2及认证签名密钥相同。比较使用进程内一次性摘要，仅输出相等布尔值，未输出任何凭据、摘要或完整DSN。

服务器配置为root-only；本轮未修改配置、数据库角色或密码。

## 最小检查结果

| 检查 | 方法 | 结果与边界 |
|---|---|---|
| 主库迁移状态 | 服务器发起HTTPS SQL，只读`schema_migrations` | version=21，dirty=false；不代表已运行发行物的原生PG迁移协议 |
| 已配置向量库 | 服务器发起HTTPS SQL，`SELECT 1` | HTTP400，SASL/SCRAM认证失败；未猜密码或改建项目 |
| 服务端env模型 | 使用现有key执行`GET /models` | HTTP401；只说明env兜底配置鉴权失败，不等于用户私有模型配置也失败 |
| 嵌入服务 | 使用现有key执行`GET /models` | HTTP200，配置模型在列表；未付费执行真实embedding，因此不证明语义检索效果 |
| R2专用对象 | 创建前HEAD→PUT→GET比对→DELETE→HEAD | 404→200→200且内容一致→204→404；测试对象已清理 |

R2唯一对象：`acceptance/papersage-20261030/cloud-preflight.txt`。只在确认该键原先不存在时创建；读回内容与本次写入一致后才删除，未触碰用户对象。

## 向量endpoint定位

当前配置的 `ep-snowy-pond-b3tpe8oa` 未出现在当前授权账户可见的匹配区域项目endpoint元数据中，也不属于已确认的PaperSage主库endpoint。仅查元数据，没有读取其他项目业务数据或连接串，也没有尝试其他项目密码。

因此目前不能从既有管理授权自动修复该连接，需要正确的原项目/分支连接信息或用户更新权威 `.env`。不能悄悄改用主库、创建替代项目或重置共享角色。

## 2026-11-01 首次发行与服务器激活

- `v0.1.0` 的发布因后来新增的前端依赖安全公告在 audit 门禁失败；release/deploy 作业跳过，没有在服务器激活。依赖安全补丁经 [0verL1nk/GraduateProject#41](https://github.com/0verL1nk/GraduateProject/pull/41) 合并，未覆盖旧 tag。
- [v0.1.1 Release](https://github.com/0verL1nk/GraduateProject/releases/tag/v0.1.1) 对应提交 `5d118a15ca5f8f006f0729aef24615408c9e31a7`；[Release 工作流](https://github.com/0verL1nk/GraduateProject/actions/runs/41272083111)中的 release 与 deploy 均成功。该次运行按当时配置生成多平台归档；linux/amd64-only是后续工作树变更，尚未发布。
- 服务器 `papersage.service` 为 active/running；本机 `/health` 返回 `healthy`、版本 `0.1.1`、上述完整提交及 `go1.27.3`。未知前端路径返回 JSON 404，匿名访问项目API返回401；未通过此证据推断登录或AI对话成功。
- LaTeX 资源网关 `GET /texlive/.../package-maps?packages=latex`：首次请求返回204耗约11.5秒，热请求204耗约18毫秒，旧Vercel API同请求也返回204；这证明catalog可加载，不证明实际模板已编译通过。进程RSS在首次读取前约37MiB，读取后一次观察约447MiB；尚无分配分析或优化后验证。
- 用户选择新一级域名 `api-papersage.overlink.top` 并建立 Cloudflare 橙云记录；公开 DNS 返回 Cloudflare 地址，边缘证书验证通过。旧 `api.paper.overlink.top` 保持 Vercel 作为回退。
- Caddy及专属 Cloudflare ingress guard 已运行并设为开机启动；源站公信证书已签发，本机SNI请求`/health`返回0.1.1。独立SSH连接仍成功；独立网络命名空间中实际验证IPv4/IPv6的非CF来源HTTP(S)被拒、SSH可用及CF来源HTTP(S)可达。工作流/持久模板见 [0verL1nk/GraduateProject#43](https://github.com/0verL1nk/GraduateProject/pull/43)，尚未合并。
- 公开新域名此前返回重定向到自身的308；2026-11-01 再次检查时，HTTPS `/health` 返回200，响应版本 `0.1.1`、提交 `5d118a15ca5f8f006f0729aef24615408c9e31a7`，未授权访问 `/api/v1/projects` 返回401。以生产前端 Origin `https://paper.overlink.top` 发送含 Authorization、Content-Type 和导入标识头的 POST 预检，返回204并允许这些方法和请求头。Cloudflare 回源 SSL 模式未能读取，不据此断言 Strict 已启用。
- 后续工作树已将公网 `/health` 收敛为仅 `{"status":"healthy"}`，version/commit/build/go 版本只在本机直连时返回；该变更随 v0.1.2 发布，线上公网 `/health` 已只返回状态。

## 2026-11-03 v0.1.2 发布与部署

- [PR #44](https://github.com/0verL1nk/GraduateProject/pull/44) 全绿（15 通过、1 跳过、0 失败）后 squash 合并为 `27078e0`。
- [v0.1.2 Release](https://github.com/0verL1nk/GraduateProject/releases/tag/v0.1.2) 由 tag 触发的 Release 工作流完成：validate、Server CI、Frontend CI、Playwright E2E、goreleaser、deploy 均成功；部署产物为 `papersage_0.1.2_linux_amd64.tar.gz`。
- 线上 `https://api-papersage.overlink.top/health` 返回 `{"status":"healthy"}`（HTTP 200），不再暴露 version/commit/buildTime/goVersion；匿名 `/api/v1/projects` 返回 401；以生产前端 Origin 发起的 CORS 预检返回 204。
- 后续 PR 将 goreleaser 构建收敛为 linux/amd64-only；v0.1.2 仍按历史配置产出多平台归档。
- 本次未验证登录后的真实模型对话、R2 业务读写与 LaTeX 编译闭环。

## 未完成 / 阻塞

- 向量认证仍失败；服务端env模型返回401。用户私有模型/管理后台配置未读取、未验收。
- 新域名健康检查已通过，但 Cloudflare 至源站是否为 Strict 未验证；前端生产 API 仍指向原Vercel入口，尚未切换。源站guard已启用，但公网完整链路和SSH长期回归仍需验证。
- Cloudflare 地址表目前只在启动或手工操作时刷新；Caddy 启动前只检查专属 nft 表是否存在，不校验具体规则，运行中其他服务的全局 `flush ruleset` 也可能移除该表。需设计并验证安全刷新、故障告警和独立上游防护，不能声称已持续自动保护。
- 没有真实模型生成/embedding、邮件实际收件、应用层上传/保存/编译闭环和业务负载性能结果。
- R2本轮证明的是存储原语及清理，不是整个应用业务已通过。

后续先解决有权限/凭据条件的任务，其余本地实现继续；相关OpenSpec任务保持未完成，禁止用本记录替代最终上线验收。
