## 1. 配置层

- [x] 1.1 在 `app/config.py` 中添加 `EmailSettings` 类
- [x] 1.2 添加 `email` 配置到 `Settings`
- [x] 1.3 更新 `.env.example` 添加邮件配置项

## 2. Email Backend 接口定义

- [x] 2.1 创建 `core/email/__init__.py`
- [x] 2.2 创建 `core/email/base.py`，定义 `EmailBackend` 协议

## 3. Resend Backend 实现

- [x] 3.1 创建 `core/email/resend.py`
- [x] 3.2 实现 `ResendBackend` 类
- [x] 3.3 添加 Resend SDK 依赖到 `pyproject.toml`

## 4. Generic SMTP Backend 实现

- [x] 4.1 创建 `core/email/smtp.py`
- [x] 4.2 实现 `SMTPBackend` 类
- [x] 4.3 处理连接池和超时

## 5. Email Service 封装

- [x] 5.1 创建 `core/email/service.py`
- [x] 5.2 实现 `EmailService` 类
- [x] 5.3 实现 `send_template()` 模板方法

## 6. 集成与导出

- [x] 6.1 更新 `core/email/__init__.py` 导出公共接口
- [x] 6.2 实现 `get_backend()` 工厂函数
- [x] 6.3 添加单元测试

## 7. 文档

- [x] 7.1 更新 `docs/backend/SECURITY.md` 添加邮件配置说明
- [x] 7.2 验证 OpenAPI 文档可正常访问
