## Why

现有AI候选和文件结构检查尚不能提供像导师批注一样的版本化写作反馈，也不能检查目标—实现—测试—结论或学校规范。需要在内容/组织产物底座上形成有定位、依据、覆盖范围和复查状态的审阅，而不是增加不透明论文分数或一组互不相通按钮。

## What Changes

- 基于固定稿件/资料/采用计划快照生成局部或文档级批注，可解释、暂缓、拒绝、手动解决或复查；候选差异仍需另行确认。
- 记录实际文本、蓝图字段、计划、规范及搜索范围依赖，分别维护运行终态、问题处理状态和适用性。
- 显式请求、保存/章节/资料/编译变化产生可合并机会；默认手动，用户开启后才按策略/预算执行，不每键触发、不拍死字数/轮数阈值。
- 提供有来源、版本、适用范围和规则类型的规范条目/包与可选择的ReviewProfile。
- 在现有检查页展示规则/模型审阅、跨章节两端定位、冲突/材料不足/未检查/不适用/失败和覆盖范围。

## Capabilities

### New Capabilities
- `writing-feedback`: 快照锚定批注、候选差异、实际依赖复查与受控事件机会。
- `academic-norms-and-review`: 版本化规范/ReviewProfile、多维及跨章节审阅与诚实覆盖报告。

### Modified Capabilities
- `writing-workbench`: 将检查页从仅骨架/结构检查扩展为保留诚实边界的真实审阅入口。

## Impact

- 依赖 `versioned-content-and-evidence` 与 `research-organization-artifacts`；复用DeepAgent、Run、审批/建议、现有检查函数与UI组件。
- 新增 `server/internal/review` 与必要反馈/规范/依赖数据及项目API；SQL/迁移/模型同步。
- `WritingPage`/`AiPanel`/`ReviewPage`增加批注、diff、规则选择和复查；不自动改稿、不运行实验、不宣称替代导师或保证引用真实。
