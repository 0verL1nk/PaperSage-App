## ADDED Requirements

### Requirement: 密码哈希工具
应用 SHALL 提供密码哈希和验证工具。

#### Scenario: 密码哈希
- **WHEN** 调用 hash_password(plain_password)
- **THEN** 返回 bcrypt 哈希后的密码

#### Scenario: 密码验证
- **WHEN** 调用 verify_password(plain, hashed)
- **THEN** 正确返回 True/False

### Requirement: JWT Token 工具
应用 SHALL 提供 JWT Token 生成和验证工具。

#### Scenario: Access Token 生成
- **WHEN** 调用 create_access_token(data)
- **THEN** 返回签名的 JWT Token，有效期 30 分钟

#### Scenario: Refresh Token 生成
- **WHEN** 调用 create_refresh_token(data)
- **THEN** 返回签名的 JWT Token，有效期 7 天

#### Scenario: Token 验证
- **WHEN** 调用 verify_token(token, "access")
- **THEN** 验证成功返回 payload，失败抛出异常

### Requirement: 异常处理
应用 SHALL 提供全局异常处理。

#### Scenario: HTTP 异常
- **WHEN** 路由抛出 HTTPException
- **THEN** 返回统一的 {code, data, info} 格式

#### Scenario: 验证异常
- **WHEN** 请求参数验证失败
- **THEN** 返回 422 状态码和详细错误信息