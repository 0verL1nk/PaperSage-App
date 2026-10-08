# agentic-writing-preparation Specification

## Purpose
Help researchers develop and maintain a verified research plan through a user-confirmed Agent conversation.
## Requirements
### Requirement: 对话式研究准备
写作准备阶段 SHALL 以 Agent 对话作为主要整理方式，用户 MUST NOT 被要求逐字段填写空白研究蓝图。Agent SHALL 读取现有项目研究状态，围绕用户输入逐步澄清研究主题、问题、目标、对象、方法、课题要求与材料；一次聚焦一个关键缺口。

#### Scenario: 从模糊兴趣开始
- **WHEN** 用户表示尚未确定选题并描述兴趣方向
- **THEN** Agent SHALL 通过追问帮助用户缩小范围，并将已有事实、假设和待验证事项分别说明

#### Scenario: 复用已有计划
- **WHEN** 用户进入写作准备并开始讨论计划
- **THEN** Agent SHALL 读取当前研究蓝图，保留已有内容，不要求用户重复填写

#### Scenario: 用户确认后保存
- **WHEN** Agent 根据对话形成研究蓝图草案
- **THEN** Agent SHALL 先向用户展示拟保存的研究蓝图与课题要求；只有用户明确确认后才保存，且保存 SHALL 使用当前 revision；未讨论要求时 SHALL 保留已有要求

#### Scenario: 并发修改冲突
- **WHEN** Agent 保存时提交的 revision 已过期
- **THEN** 系统 SHALL 拒绝覆盖并发更新并要求重新读取最新状态

#### Scenario: 游客尝试使用研究顾问
- **WHEN** 游客打开写作准备
- **THEN** 页面 SHALL 展示本机状态摘要并说明需要登录账号才能使用服务端 Agent，不得展示伪造的 AI 回复
