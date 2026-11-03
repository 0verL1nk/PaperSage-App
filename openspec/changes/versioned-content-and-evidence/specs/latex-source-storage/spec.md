## MODIFIED Requirements

### Requirement: LaTeX source file model
系统 SHALL 使用现有`latex_files`保留项目文件身份与元数据，并将当前内容关联到稳定项目来源及不可变来源修订；元数据、当前指针和建议应用状态 MUST 在同一主库事务内保持一致。

#### Scenario: Create project with default template
- **WHEN** 用户创建项目且默认模板初始化成功
- **THEN** 创建默认`main.tex`、其初始不可变内容修订和一致元数据

#### Scenario: Default template initialization fails
- **WHEN** 默认模板对象或数据库提交失败
- **THEN** 项目创建失败并清理未完成项目记录；未引用对象只在引用复核后回收，不宣称模板已初始化

### Requirement: Source file CRUD API
系统 SHALL 保留项目源文件列表、获取、创建、更新和删除API，并向内容读写调用方提供修订身份；已有文件更新 MUST 携带有效基准修订，且失败 MUST NOT 覆盖旧稿或返回成功。

#### Scenario: List all source files
- **WHEN** 请求项目源文件列表
- **THEN** 按file_path返回可见文件元数据及当前修订，不暴露其他项目

#### Scenario: Get file content
- **WHEN** 获取当前文件或获授权历史修订
- **THEN** 返回对应不可变内容、修订/ETag及正确内容类型，不悄悄混合不同版本

#### Scenario: Update file content
- **WHEN** PUT携带content及与当前一致的expectedRevision或强If-Match
- **THEN** 准备新对象后以CAS提交当前指针与元数据，返回新修订；缓存只在提交后更新

#### Scenario: Stale or missing write precondition
- **WHEN** 已激活版本化写入的已有文件更新缺少前置条件或基准过期
- **THEN** 缺条件返回428、基准冲突返回409并给出恢复提示；当前稿件不变，客户端本地输入保留

#### Scenario: Create new file
- **WHEN** 请求创建合法路径的文件
- **THEN** 创建初始修订并返回201；并发同路径不生成两个活跃文件，重复幂等请求返回同一结果

#### Scenario: Delete file
- **WHEN** 用户删除其项目中的文件
- **THEN** 文件退出当前工作区及新分析范围；历史内容按既定删除/恢复权限处理，仍被引用的对象不被直接删除

#### Scenario: Create a file in an empty project
- **WHEN** 用户在空项目点击新建文件并命名提交
- **THEN** 创建文件和初始修订后出现在文件树中

#### Scenario: Create a folder in an empty project
- **WHEN** 用户在空项目点击新建文件夹并命名提交
- **THEN** 文件夹创建成功并在树中显示，不要求先有文件

#### Scenario: File creation fails
- **WHEN** 文件或文件夹创建失败
- **THEN** 展示原因且不显示虚假成功节点，不丢已有内容

### Requirement: S3 storage integration
系统 SHALL 继续使用配置的S3兼容存储（含R2/MinIO），以项目/来源/修订限定的新对象键保存不可变内容；存量`projects/{project_id}/latex/{file_path}`只作为待受控回填的旧内容来源，MUST NOT 在版本化激活后继续原位覆盖。

#### Scenario: Configure S3 backend
- **WHEN** 配置S3兼容存储
- **THEN** 所有新内容修订使用不可变对象，凭据仅在服务端，旧读取在回填期保持可用

#### Scenario: Local dev with MinIO
- **WHEN** 开发环境使用本地MinIO或项目现有S3替身
- **THEN** 应用相同版本/故障/权限契约，不依赖真实AWS凭据

## ADDED Requirements

### Requirement: 历史恢复和建议应用具有同一版本保证

系统 SHALL 允许获授权用户查看历史并以当前修订为基准恢复为一个新修订；AI建议应用 SHALL 在同一事务内校验权限、证据用途、审批、幂等和基准，再提交全部目标修订及建议终态。

#### Scenario: Restore a previous revision
- **WHEN** 用户确认恢复历史内容且当前基准仍一致
- **THEN** 创建新的恢复修订并保留原历史；重复幂等请求不额外增加修订

#### Scenario: Partial object or database failure
- **WHEN** 多文件建议准备对象或提交任一CAS失败
- **THEN** 所有当前指针和建议状态保持未应用，旧稿仍可读，未引用对象可补偿清理

#### Scenario: Evidence changes after proposal generation
- **WHEN** 正文未变但候选依赖的来源权限、采用计划或规范已失效
- **THEN** 拒绝旧候选应用并要求复查/新候选与重新确认，不借旧确认继续写入

### Requirement: 版本化写入切换不得保留旁路

系统 SHALL 先升级全部写入调用方并验证旧API停写，再在受控窗口回填基线与激活强制CAS；MUST NOT 由当前内容推造丢失历史或将无条件写兼容作为永久后门。

#### Scenario: Legacy writer remains active
- **WHEN** 旧客户端或旧部署仍可覆盖共享旧对象
- **THEN** 版本化切换不得放行，且不宣称历史已经不可变
