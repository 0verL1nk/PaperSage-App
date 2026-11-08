## Context

参考仓库：`https://github.com/deepseek-ai/deepseek-harness`（本地参考副本 `/tmp/deepseek-harness`）。

可借鉴的层：
- `packages/client/ui-chat/src/client/chat/AssistantMarkdown.tsx`：Assistant 正文按 block 渲染，全宽、14/24、streaming 状态、代码/表格/图片独立处理。
- `packages/client/ui-chat/src/client/chat/ChatView.tsx`：消息列宽、滚动容器、工具行与正文分层。
- `packages/client/ui-primitives/src/markdown/MarkdownText.tsx`：增量解析、冻结已渲染块、脚注/引用、KaTeX、代码高亮。

不照搬：其 mdast 增量解析器、KaTeX、高亮、文件提及/图片解析依赖大量内部包，PaperSage 不需要。用 `react-markdown` + `remark-gfm` 做安全、够用的 GFM 渲染。

## Goals / Non-Goals

**Goals**
- Markdown 正确渲染，长回答可读。
- 视觉接近成熟 AI 对话产品：全宽 Assistant 正文、右对齐 User 气泡、清晰层级、舒适行高。
- 代码块可复制、表格可滚动、链接安全。
- 保留现有流式、工具、建议、依据、取消、错误、重试行为。

**Non-Goals**
- 不复制 deepseek-harness 的完整渲染管线、KaTeX、图片提及、增量冻结。
- 不做富文本编辑器、不做消息引用/分支、不做主题系统重写。
- 不改变后端事件协议或数据形状。

## Decisions

### D1 Markdown 渲染器

使用 `react-markdown` + `remark-gfm`。默认不启用 `rehype-raw`，模型输出的 HTML 不执行（防 XSS）。自定义 `components` 映射：
- `h1..h4`：紧凑标题层级，不抢正文；
- `p`：14px/24px，段间距 12px；
- `ul/ol/li`：列表缩进与 marker；
- `blockquote`：左侧细线 + 次要色；
- `pre/code`：代码块深色底、等宽字体、语言标签、复制按钮；行内代码浅底；
- `table/thead/tbody/th/td`：边框、表头底色、横向滚动容器；
- `a`：主色、下划线、`target="_blank"`、`rel="noopener noreferrer"`；
- `hr`：细分隔线。

不启用语法高亮，避免新增高亮依赖；语言标签仅展示。

### D2 消息排版

- Assistant：无气泡，全宽正文，左侧小头像/角色标签可选；正文最大宽度不设硬上限（面板宽度内自适应）。
- User：右对齐、浅主色气泡、最大宽度 85%，保留换行。
- 消息间距 24px；同一消息内 block 间距 12–16px。
- 流式时正文末尾显示细光标/“正在生成”，完成后消失。
- 工具调用保持紧凑行/卡，默认折叠；建议卡、依据、错误在正文下方，不与 Markdown 正文混排成纯文本。

### D3 状态与可访问性

- Markdown 容器 `aria-live="polite"`（仅流式消息），避免整段重复播报。
- 复制按钮有 `aria-label` 与成功反馈。
- 代码块/表格在窄屏可横向滚动，不撑破面板；长单词/URL 可断行。
- 暗色模式使用语义 token，不写死颜色。

### D4 依赖与安全

新增 `react-markdown`、`remark-gfm`（均为常用、无原生依赖）。不启用 raw HTML；外链统一安全属性。若模型输出无法解析的 Markdown，退化为纯文本而不是白屏。

## Risks

- Markdown 渲染增加每次流式 chunk 的解析成本；用 `memo` 按 part 文本缓存，避免整棵消息树重渲染。
- 模型输出恶意链接/HTML：禁用 raw HTML，链接加安全属性。
- 既有测试断言纯文本；需要更新为语义节点断言，避免只测字符串。
- 代码块复制在非安全上下文（http）可能不可用；失败时给出提示，不静默。
