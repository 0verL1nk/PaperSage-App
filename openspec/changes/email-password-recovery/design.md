## Security design

- 申请接口对存在、不存在、未验证和无邮箱的账号都返回相同 HTTP 200 和消息；仅给已验证邮箱的账号排队发送邮件。
- 以 32 字节 CSPRNG opaque token 作为邮件凭据，复用现有 `verification_tokens` 表并用 `purpose=password_reset` 与注册验证 token 隔离；数据库仅保存 SHA-256 哈希。token 15 分钟到期，成功消费时在事务内更新密码、标记该用户所有重置 token 已用、增加 `auth_version` 并撤销全部 refresh token。
- 用户认证中间件和 refresh 流程都比较 token 中的认证版本与用户行版本，因此密码重置立即吊销旧 access/refresh token。历史版本字段缺省为 0，旧 token 在密码未重置的账号上兼容。
- 对来源 IP 与规范化邮箱限速。重置申请邮件在提交通用响应后异步发送，避免邮件服务延迟暴露账号是否存在；日志仅记录脱敏邮箱和错误，不记录 token 或完整链接。
- 邮件链接 token 放在 URL fragment 中，避免发到 Web 服务器日志和 Referer。SPA 读取后立即用 `history.replaceState` 移除地址栏凭据；重置页的文档策略设为 `no-referrer`。
- 新密码仍沿用现有前端 SHA-256 传输契约并由后端 bcrypt 存储；成功后不自动登录，用户走正常登录流程。

## Delivery

使用 SQL-first schema 与 0016 双方言迁移；新增字段，不新建 token 表。SQLite 测试由 GORM AutoMigrate 覆盖。认证测试验证统一响应、未验证邮箱不发信、token 哈希存储、过期/重放、刷新 token 撤销和旧 access token 失效。
