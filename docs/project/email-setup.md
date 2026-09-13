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
- 发送日志：`resend logs list`
