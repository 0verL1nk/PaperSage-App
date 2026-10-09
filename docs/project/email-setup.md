# 邮件链路配置（Resend + Cloudflare）

注册邮箱验证邮件通过 [Resend](https://resend.com) 发送，发信域托管在 Cloudflare DNS。
配置完成后整个「注册 → 收验证邮件 → 点击验证 → 登录」链路即为真实生产路径。

## 一次性配置（约 10 分钟）

### 1. Resend 添加发信域

```bash
# 登录（浏览器授权，凭据不经过终端）
resend login

# 添加域：推荐子域（如 mail.example.com），Resend 最佳实践
resend domains create --name mail.<你的域名> --region ap-northeast-1

# 查看需要添加的 DNS 记录（DKIM/SPF/Return-Path MX）
resend domains get <domain_id>
```

### 2. Cloudflare 添加 DNS 记录

把 `resend domains get` 输出的三条记录加到 Cloudflare DNS：

| 类型 | 名称 | 内容 | 代理 |
|---|---|---|---|
| TXT (SPF) | `mail` | `v=spf1 include:amazonses.com ~all` | — |
| TXT (DKIM) | `resend._domainkey.mail` | Resend 给的公钥（很长） | **仅 DNS（灰云！）** |
| MX | `send.mail` | Resend 给的地址，优先级 10 | 仅 DNS |

> **注意**：DKIM 记录必须关橙云（DNS only），开着代理会验证失败。
> 加 DMARC（可选但推荐）：TXT `_dmarc` → `v=DMARC1; p=none;`

### 3. 验证域名

```bash
resend domains verify <domain_id>   # DNS 生效后（通常几分钟）
resend domains list                 # 状态应为 verified
```

### 4. 创建 API Key 并写入 .env

- Resend 控制台 → API Keys → **Create API Key**（Sending access 即可）
- `server/.env`：

```env
APP_URL=http://localhost:5173      # 或线上地址
EMAIL_PROVIDER=resend
RESEND_API_KEY=re_xxxxxxxx         # 你的 key
RESEND_DOMAIN=mail.example.com     # 已验证的发信域
SUPERUSER_EMAILS=you@example.com   # 管理员邮箱（可选）
```

### 5. 重启后端并端到端验证

```bash
# 后端日志确认 Resend 生效（不再是 noop）
# 注册页填 <你的邮箱>，收到真实邮件 → 点链接 → 登录成功
```

## 故障排查

- 邮件进垃圾箱：检查 SPF/DKIM/DMARC 三条记录是否齐（`resend domains get`）
- `resend: status 403`：API key 无 Sending 权限或域名未 verify
- 域名验证超时：DKIM 记录橙云没关；`resend domains verify` 重试
- API 返回 `503 Verification email could not be sent`：检查服务端 Resend 配置及 API 日志；注册账号已保留，重试注册会重新发送。
- Resend 的邮件事件为 `delivered` 但用户未看到：收件方服务器已接受邮件，继续检查垃圾邮件、学校邮箱隔离区、收件规则和转发设置。
- 发送日志：Resend 控制台的 Emails/Events 页面；不要在日志或截图中暴露 API Key、邮件 token。

## 生产 API Key 轮换

生产凭据轮换按“新旧并行—切换部署—验证—撤销旧值”执行，避免邮件发送在切换中断：

1. 在 Resend 创建新的 `sending_access` key；如发信域已验证，限制到 PaperSage 发信域。新 key 仅在创建时显示一次，立即安全保存，不写入仓库、命令历史、日志或聊天内容。
2. 在 Vercel 的 `papersage-api` 项目中，只更新确实使用该凭据的环境（至少 Production；Preview 仅在确实使用生产发信域时更新），变量名为 `RESEND_API_KEY`。不要把生产 key 放入浏览器可见变量或 Preview 的不相关环境。
3. 重新部署后，先确认部署成功，再用已授权的测试邮箱走一遍验证/重置邮件链路，并检查 Resend 发送事件和应用日志；`delivered` 只表示收件方邮件服务器已接受。
4. 确认新部署发信正常后，在 Resend 撤销旧 key；旧部署仍使用旧环境快照，必要时一并重部署。
5. 不把 key、完整环境变量导出文件或含 token 的邮件链接提交到 Git；本地临时材料放在忽略目录并在验证后删除。

不能只替换 Vercel 变量后立即删旧 key：Vercel 环境变量变更只对新部署生效。详见 [Vercel 安全轮换说明](https://vercel.com/docs/environment-variables/rotating-secrets)。
