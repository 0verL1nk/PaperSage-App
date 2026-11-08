## ADDED Requirements

### Requirement: 对话内容 Markdown 渲染
系统 SHALL 将 Assistant 的文本内容按 GitHub Flavored Markdown 渲染为语义化元素，包括标题、段落、列表、引用、代码块、行内代码、表格、链接与分隔线。系统 MUST NOT 执行模型输出中的原始 HTML；外链 MUST 使用新窗口与 `noopener noreferrer`。无法解析的内容 SHALL 退化为纯文本，MUST NOT 导致消息区白屏。

#### Scenario: 长回答包含标题与列表
- **WHEN** 模型返回带 `##` 标题和有序/无序列表的回答
- **THEN** 消息区显示对应标题层级与列表项，而不是原样显示 `##` 或 `-`

#### Scenario: 代码块
- **WHEN** 模型返回 fenced code block
- **THEN** 以等宽字体、代码底色和语言标签展示
- **AND** 提供复制按钮，复制失败时给出可见提示

#### Scenario: 表格
- **WHEN** 模型返回 Markdown 表格
- **THEN** 以带表头与边框的表格展示
- **AND** 窄屏下可横向滚动，不撑破面板

#### Scenario: 不安全 HTML
- **WHEN** 模型输出包含 `<script>` 或事件属性
- **THEN** 不执行、不注入为可执行 HTML

#### Scenario: 解析失败
- **WHEN** 文本不是合法 Markdown
- **THEN** 仍以可读纯文本展示，不出现空白消息区

### Requirement: 对话消息排版
Assistant 消息 SHALL 使用全宽正文排版，正文基准字号不小于 14px、行高约 24px；User 消息 SHALL 使用右对齐气泡并限制最大宽度。消息之间与块之间 SHALL 保持稳定间距。流式生成时 SHALL 显示生成状态或光标，完成后消失。

#### Scenario: 长回答阅读
- **WHEN** Assistant 返回多段长回答
- **THEN** 正文按段落/列表/代码块分层展示，行距舒适，不显示为 12px 纯文本墙

#### Scenario: 流式生成
- **WHEN** 回答仍在流式输出
- **THEN** 末尾显示生成状态或光标
- **AND** 完成后状态消失，正文不重复

### Requirement: 工具、建议与错误在同一排版体系内
工具调用、修改建议、依据与错误 SHALL 与 Markdown 正文使用一致的间距与层级；工具卡默认紧凑、可展开，建议卡保留 diff 与应用/拒绝，错误可见且不重复。

#### Scenario: 工具调用后继续正文
- **WHEN** 一次回答包含工具调用与后续文本
- **THEN** 工具行与正文清晰分层，正文仍按 Markdown 渲染

#### Scenario: 建议卡
- **WHEN** 助手产出修改建议
- **THEN** 建议卡显示 diff、状态与应用/拒绝按钮，不混入纯文本
