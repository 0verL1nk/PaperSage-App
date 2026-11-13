# Security Policy

## 报告漏洞

请**不要**通过公开 issue 报告安全问题。

使用 GitHub 的私密漏洞报告：仓库页 **Security → Advisories → Report a vulnerability**。
报告里请包含复现步骤、影响范围和建议的修复方向。

## 范围

- 后端 API（`server/`）、鉴权与令牌轮换
- Agent 运行时（工具调用、建议应用、Run 事件）
- 前端（XSS、鉴权绕过、敏感信息泄露）
- 部署脚本（`deploy/`）

## 不在范围

- 需要受害者本机已被攻破的问题
- 依赖库自身的漏洞（请直接报给上游；Dependabot 会自动跟踪）
- 缺少速率限制之类的加固建议（可以提 issue，但不算漏洞）

## 处理方式

确认后会在修复版本中说明；修复发布前不会公开细节。
