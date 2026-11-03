## Why

API-only 发布代码已经合并，但正式服务器尚未激活；向量认证、真实模型/SSE、云端写入和邮件收件不能由CI替身证明。先交付一个可追踪、可恢复的现有功能基线，为后续版本化内容变更提供可信部署环境。

## What Changes

- 完成PR40迁移历史对齐并核对实际schema/对象存储；既有CI通过后以明确SemVer发布，不手工绕过Release/CD。
- 使用现有Go二进制、systemd、Caddy和云库/R2，完成回环健康、HTTPS、DNS、真实前端环境切换、模型与编译验证。
- 修复现有向量连接，确认真实嵌入与降级可见性；只使用现有授权项目/角色，不猜密码、不另建项目替代失败认证。
- 建立账号、邮件收件、文件上传保存、SSE、取消/历史与失败恢复证据；秘密留在运行配置，不进入报告。
- 明确旧Vercel API的过渡/停写策略，供后续CAS激活使用；不盲目删除线上服务或仓库部署PR。

## Capabilities

### New Capabilities
- `production-api-release`: 小服务器的可验证版本发布、外部配置与真实业务上线验收。

### Modified Capabilities
- 无；复用既有交付与业务契约，不在本变更新增科研产物。

## Impact

- 主要涉及 `deploy/`、`.github/workflows/release.yml`、`docs/backend/DEPLOYMENT.md`、运行配置及验证记录；只有发现真实缺陷时修改对应共享实现。
- 与 `complete-research-writing-workflow` 关联；阶段B不得在旧写入口未受控时开启版本化存储。
- 生产数据库不执行down/force/seed；域名和旧API状态变更在部署健康及明确切换审批后实施。
