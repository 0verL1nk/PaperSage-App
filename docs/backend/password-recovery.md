# 邮箱密码找回

## 方案

按 OWASP Forgot Password 与 Email Validation 指南采用已验证邮箱的短时一次性链接：申请响应不区分邮箱是否存在，token 使用 CSPRNG 生成并单独哈希存储，重置操作不自动建立会话。链接有效期 15 分钟，使用后立即失效。邮件服务故障不会向调用方暴露账号状态。

重置 token 复用 `verification_tokens` 表，以 `purpose=email_verify` 和 `purpose=password_reset` 区分；此次迁移不添加表。token 通过 fragment 传递，浏览器读取后移除，站点配置 `no-referrer`。

重置成功在一个数据库事务中完成：更新 bcrypt 密码哈希、递增 `users.auth_version`、作废该用户其他重置 token、撤销该用户 refresh tokens。Access/refresh token 都携带认证版本，Go 认证中间件和刷新端点与数据库版本比较，确保旧会话不能继续使用。

## 限速与隐私

- 每 IP 每分钟最多 10 次申请；每规范化邮箱每分钟最多发送 1 封。
- 不存在、未验证、停用或无邮箱的账号都得到相同的 HTTP 200 提示，不发送邮件。
- 邮件发送在响应后异步进行；失败日志仅包含脱敏邮箱和错误，不写入 token 或完整链接。
- 限速器当前为单进程内存实现。水平扩容前应替换为 Redis/网关共享限速，避免跨实例绕过限制。

## 数据迁移

`0016_email_password_recovery` 为 PostgreSQL 和 MySQL/TiDB 增加 `users.auth_version` 与 `verification_tokens.purpose` 两个字段，分别默认 0、`email_verify`。旧验证 token 可继续使用，历史 access/refresh token 对未发生重置的账号按版本 0 兼容。本地 SQLite 测试由 GORM AutoMigrate 创建字段。

Go 服务启动时自动应用待执行迁移；使用 `server/.env` 启动本地服务也会直接迁移该文件连接的数据库。本次 0016 尚未应用到生产。本次功能的生产迁移和真实邮件送达均尚未验收；Resend 域名 API 的只读授权检查不能替代邮件收件箱闭环。

## 验证

认证测试覆盖账号枚举一致响应、未验证邮箱不发信、速率限制、token 只存哈希、15 分钟过期、用途隔离、重复消费、密码更新、refresh 撤销和 access token 版本失效。浏览器 E2E 覆盖找回/重置页面及 320–1440px 响应式布局。

## References

- [OWASP Forgot Password Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)
- [OWASP Email Validation and Verification Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Email_Validation_and_Verification_Cheat_Sheet.html)
