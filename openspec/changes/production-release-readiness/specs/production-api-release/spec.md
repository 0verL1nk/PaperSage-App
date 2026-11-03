## ADDED Requirements

### Requirement: 发布版本与运行实例可追踪

系统 SHALL 通过既有CI门禁和不可变版本产物发布API，校验同次构建的checksum，并在运行health中返回真实version、完整commit、构建时间和Go版本；MUST NOT 用HTTP 200代替版本一致性。

#### Scenario: 正式发布成功
- **WHEN** 一个已在main且门禁通过的稳定版本被发布
- **THEN** 服务器运行对应的API-only二进制，health与发布元数据一致，前端仍独立部署

#### Scenario: 激活失败
- **WHEN** 新版本启动或health校验失败
- **THEN** 恢复已验证兼容的旧程序；首次无旧版时明确失败并停止，且不执行schema down

### Requirement: 配置与迁移预检不破坏现有数据

发布 SHALL 预检已有schema、真实连接和权限；数据库领先于发布包、dirty或向量认证失败时 MUST 报告具体阻塞，MUST NOT 猜密码、force版本、seed生产库或另建项目掩盖问题。

#### Scenario: 迁移历史缺失
- **WHEN** 云库已有版本无法由发布包迁移历史识别
- **THEN** 停止激活并核对历史与实际结构，而不修改数据库版本标记

#### Scenario: 向量服务不可用
- **WHEN** 认证、嵌入或向量查询失败
- **THEN** 显示实际可用模式并保留失败证据，不将哈希或词法替身算作完整语义检索通过

### Requirement: 生产切换有顺序和回退边界

系统运维 SHALL 在新实例回环健康和业务兼容验证后执行获批DNS/HTTPS切换，并记录旧API入口的写入状态；MUST NOT 在未受控双写时激活版本化稿件存储。

#### Scenario: 旧API仍可写
- **WHEN** 旧Vercel部署域名仍能原位覆盖共享对象
- **THEN** 后续CAS启用保持受阻，直到该入口停写或迁移到同一版本契约

### Requirement: 真实业务验收与替身分开

上线验收 SHALL 包括获授权账号、模型SSE、向量/嵌入、R2实际读写、浏览器编译与邮件实际收件，并保存失败和未执行记录；MUST NOT 将模拟流、HEAD、provider接受或入队成功替代对应终态。

#### Scenario: provider接受但没有收件证据
- **WHEN** 邮件服务已接受发送而未取得测试收件箱或用户实际收件确认
- **THEN** 发送接口可标通过，实际收件任务仍未完成

#### Scenario: 外部条件不足
- **WHEN** 缺少授权模型、凭据、收件箱或域名操作条件
- **THEN** 仅阻塞依赖该条件的验收，保留本地实现/测试进展且不伪造成功
