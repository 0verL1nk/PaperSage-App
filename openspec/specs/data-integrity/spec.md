# data-integrity Specification

## Purpose
TBD - created by archiving change data-integrity-refactor. Update Purpose after archive.
## Requirements
### Requirement: 逻辑删除资源的同名重建
对采用逻辑删除的资源，系统 SHALL 允许在删除后重新创建同名资源；唯一性约束 SHALL 只约束活跃记录，MUST NOT 因存在已删除的同名记录而拒绝创建。系统 SHALL 支持同一名称反复删除与重建。

#### Scenario: 删除后重建同名 LaTeX 文件
- **WHEN** 用户删除项目中的 `main.tex`，随后再次创建 `main.tex`
- **THEN** 创建成功
- **AND** 列表与文件读取返回新建的文件，不返回已删除的旧文件

#### Scenario: 反复删除与重建
- **WHEN** 同一路径被删除并重建多次
- **THEN** 每次创建均成功，历史删除记录不影响后续创建

#### Scenario: 活跃记录的唯一性仍然生效
- **WHEN** 用户创建的项目中已存在未删除的 `main.tex`
- **THEN** 再次创建同名文件被拒绝，错误语义与既有契约一致

### Requirement: 删除语义分层
系统 SHALL 按数据性质区分删除语义并保持一致：用户资产（项目 / 分组 / 文件夹 / 文档 / LaTeX 文件）采用逻辑删除以支持恢复；会话与消息采用逻辑删除；会话删除时，其运行记录 SHALL 保留，MUST NOT 被销毁。删除操作在表结构、数据模型与业务代码三层的语义 MUST 一致，MUST NOT 出现「表声明逻辑删除而代码执行物理删除」的情况。

#### Scenario: 删除会话保留运行记录
- **WHEN** 用户删除一个已产生执行记录的会话
- **THEN** 该会话与其消息不再对用户可见
- **AND** 其运行记录（模型配置、用量、耗时、工具调用）仍然保留可供审计与统计

#### Scenario: 删除会话不依赖外键级联
- **WHEN** 用户删除会话
- **THEN** 该会话的消息在同一事务内一并不可见
- **AND** 不依赖物理删除的外键级联行为达成该结果

#### Scenario: 已删除资源不出现在查询中
- **WHEN** 用户列出项目、文件或会话
- **THEN** 逻辑删除的记录不出现在结果中

### Requirement: 跨方言时间戳一致性
系统 SHALL 由应用层以 UTC 写入所有业务时间戳；MUST NOT 依赖某一数据库方言特有的时间函数。数据库默认值与自动更新子句仅作兜底，MUST NOT 作为业务时钟。时间戳的对外序列化格式 MUST 保持既有契约不变。

#### Scenario: 更新记录的时间戳确实变化
- **WHEN** 用户对一条记录执行更新
- **THEN** 该记录的更新时间被更新为当前 UTC 时间
- **AND** 在 SQLite、PostgreSQL、MySQL 三种方言下行为一致

#### Scenario: 按更新时间排序
- **WHEN** 系统按最近活动排序会话列表
- **THEN** 排序结果反映真实的最近更新时间

### Requirement: 与查询模式匹配的索引
系统 SHALL 为主要列表查询提供与查询条件匹配的复合索引：会话列表 SHALL 支持按（项目、用户、更新时间）高效检索；消息历史 SHALL 支持按（会话、创建时间）高效检索。

#### Scenario: 会话列表检索
- **WHEN** 系统查询某项目下某用户的会话并按更新时间倒序取前若干条
- **THEN** 该查询可由复合索引覆盖，不产生全表扫描

