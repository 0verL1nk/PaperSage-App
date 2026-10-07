## Why

账号目前只能通过登录或注册使用，现有验证邮箱流程没有找回密码入口。用户忘记密码时无法安全恢复账号。

## What Changes

- 增加防账号枚举的密码重置申请、邮件一次性链接和设置新密码流程。
- 重置 token 使用高熵随机值，只存哈希，短时有效、单次消费并限速；复用现有一次性验证 token 表并用 purpose 隔离用途。
- 重置成功后撤销 refresh token 并增加账号认证版本，使旧 access token 失效；要求用户重新登录。
- 增加一致的找回/重置页面、邮件通知、测试与运行文档。

## Capabilities

### New Capabilities
- **email-password-recovery**: 用户通过已验证邮箱安全重置密码。

### Modified Capabilities
- **authentication**: 密码重置后使旧会话失效，并要求重新登录。

## Impact

- 后端认证域、用户与一次性 token 数据模型、MySQL/TiDB 和 PostgreSQL 迁移。
- 前端认证 API、路由、登录入口和两张恢复流程页面。
- OpenSpec、项目认证文档与自动化测试。
