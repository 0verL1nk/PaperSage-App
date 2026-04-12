## Context

系统需要发送邮件用于通知和验证场景。Resend SMTP 已配置，需构建通用邮件服务抽象层，支持未来切换不同的邮件服务提供商。

## Goals / Non-Goals

**Goals:**
- 通用邮件发送接口
- 支持多种 SMTP 后端配置
- 邮件内容模板支持（简单占位符替换）
- 配置化切换邮件服务

**Non-Goals:**
- 不实现邮箱验证功能（为未来预留）
- 不实现异步队列（V1 同步发送）
- 不实现复杂邮件模板引擎

## Decisions

### 1. 抽象层设计：接口 + 多后端实现

**选择理由：**
- 符合"依赖倒置"原则
- 便于测试时替换实现
- 支持运行时切换后端

**接口定义：**
```python
class EmailBackend(Protocol):
    async def send(self, to: str, subject: str, body: str) -> bool: ...
```

**内置实现：**
- `ResendBackend` - Resend SMTP
- `SMTPBackend` - 标准 Python SMTP（通用）

### 2. 配置结构

```python
class EmailSettings(BaseModel):
    provider: Literal["resend", "smtp"] = "resend"
    from_email: str
    # Resend 专用
    resend_api_key: str | None = None
    resend_domain: str | None = None
    # 通用 SMTP 专用
    smtp_host: str | None = None
    smtp_port: int | None = None
    smtp_user: str | None = None
    smtp_password: str | None = None
```

### 3. 文件结构

```
backend/core/email/
├── __init__.py          # 导出 EmailBackend, get_backend
├── base.py              # EmailBackend 协议定义
├── resend.py            # Resend 后端实现
├── smtp.py              # 通用 SMTP 后端实现
└── service.py           # 邮件服务类
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| 邮件发送失败 | 记录日志，返回 False |
| SMTP 连接超时 | 设置合理超时时间（10s） |
