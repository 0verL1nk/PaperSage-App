# rag-index-lifecycle Specification

## Purpose
TBD - created by archiving change data-integrity-refactor. Update Purpose after archive.
## Requirements
### Requirement: 检索索引代次
检索内容的每条记录 SHALL 归属一个索引代次；系统 SHALL 记录每个项目的代次及其状态（构建中 / 活动 / 已被取代 / 失败）、所用嵌入模型标识与维度、片段数量。同一项目同一时刻 MUST 至多存在一个活动代次。

#### Scenario: 建立新代次
- **WHEN** 项目开始一轮索引重建
- **THEN** 系统创建一个 `building` 状态的代次，并以该代次写入新产生的片段

#### Scenario: 校验后切换活动代次
- **WHEN** 新代次构建完成并通过校验
- **THEN** 该代次转为 `active`，原活动代次转为已被取代
- **AND** 同一项目同时只允许一个 `building` 代次，切换操作是幂等的

#### Scenario: 失败代次可重试
- **WHEN** 某代次构建失败
- **THEN** 该代次标记为失败并保留，可重新构建
- **AND** 失败不影响当前活动代次继续提供检索

#### Scenario: 旧代次可回查
- **WHEN** 需要查看某次检索结果对应的历史材料
- **THEN** 可依据片段所属代次回查已被取代的代次内容，并标注其对应的是旧版材料

### Requirement: 检索限定于活动代次
检索 SHALL 只命中当前活动代次的片段；MUST NOT 在同一结果集中混入不同代次的片段。

#### Scenario: 检索不跨代次
- **WHEN** 项目存在一个活动代次与若干已被取代或失败的代次
- **THEN** 检索结果只包含活动代次的片段

#### Scenario: 更换嵌入模型
- **WHEN** 嵌入模型或其维度发生变化
- **THEN** 系统以新代次重建向量，MUST NOT 在新旧向量混用的情况下宣称检索质量

