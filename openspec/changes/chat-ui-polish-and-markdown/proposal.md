## Why

当前 AI 对话面板把模型输出当纯文本渲染（`whitespace-pre-wrap`、12px），标题、列表、代码块、表格、链接全部原样显示，长回答可读性差，视觉上也与成熟 AI 产品差距明显。用户明确要求参考 `deepseek-ai/deepseek-harness` 的对话 UI，Markdown 必须渲染。

## What Changes

- 引入 Markdown 渲染（GFM）：标题、段落、列表、引用、代码块、行内代码、表格、链接、分隔线、加粗/斜体。
- 代码块提供语言标识与复制按钮；表格横向滚动；链接新窗口打开且 `rel="noopener noreferrer"`。
- 重做消息排版：Assistant 使用全宽正文（不再是小字号纯文本），User 使用右对齐气泡；统一 14/24 字号行高与消息间距。
- 流式输出显示光标/生成状态；工具调用、建议卡、错误、依据在正文中保持清晰层级。
- 空状态、加载、断线、取消、失败状态与 Markdown 正文使用同一套视觉语言。
- 暗色模式与移动端（390px 起）可用；长代码/表格不撑破面板。

## Capabilities

### Modified Capabilities

- **ai-writing-assistant**：对话内容的 Markdown 渲染、消息排版、代码块/表格/链接呈现与流式可读性。

## Impact

- 前端：`views/projects/ai-panel.tsx`、新增 Markdown 渲染组件、样式与测试；新增 `react-markdown`、`remark-gfm` 依赖。
- 后端：无接口变更。
- 不引入 AI Elements/Next.js；不复制 deepseek-harness 的完整 mdast 管线，只借鉴其排版与组件分层。
