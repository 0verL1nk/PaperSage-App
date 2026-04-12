## Why

系统需要发送邮件（未来用于邮箱验证、密码重置、通知等），但当前缺少邮件服务层。Resend SMTP 已配置，需要构建通用邮件服务抽象。

## What Changes

- 实现通用的 SMTP 邮件服务抽象层
- 支持多种 SMTP 后端（Resend、SendGrid、自建 SMTP 等）
- 提供简单的邮件发送接口
- 配置化切换不同邮件服务提供商

## Capabilities

### New Capabilities

- `email-service`: 通用邮件服务抽象层，支持多种 SMTP 后端

### Modified Capabilities

- （无）

## Impact

- **Backend**: 新增 `core/email/` 模块
- **Config**: 新增邮件服务配置项
- **Future**: 邮箱验证、密码重置等功能基于此模块构建
