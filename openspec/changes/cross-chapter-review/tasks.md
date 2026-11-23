## 1. 数据与契约

- [x] 1.1 新增 `chapter_review` 产物 kind 与 payload 类型（input/dimensions/findings/sourcesRead）。
- [x] 1.2 `validatePayload` 校验四维齐全、状态枚举、证据不足必填原因、finding 定位/依据/下一步、上限与项目归属。
- [x] 1.3 规范库输入边界 `norms.Provider` + `Unavailable`；规范不可用时拒绝声称完成规范符合度检查。
- [x] 1.4 Go 表驱动测试覆盖非法枚举、缺 note、悬空 filePath、跨项目计划与伪通过。

## 2. 上下文与防编造

- [x] 2.1 `get_chapter_review_context` 装配项目背景/要求、已采用章节计划、规范条目、各章正文、资料清单与 warnings。
- [x] 2.2 `create_research_artifact` / `add_research_artifact_revision` 在 `chapter_review` 落库前逐字核对 text/plan 引文并计算行号。
- [x] 2.3 `basis.kind=source` 校验资料存在且本轮确实读过；`sourcesRead` 用实际读取记录覆盖。
- [x] 2.4 未调用上下文工具时拒绝保存报告；测试覆盖编造引文与缺上下文。

## 3. 前端

- [x] 3.1 Review 页分区：基础结构检查（本地确定性）+ 跨章节 AI 审阅。
- [x] 3.2 报告展示维度状态、问题定位、原文引文、可展开依据、下一步与读取材料。
- [x] 3.3 覆盖不完整 / 证据不足 / 模型未配置 / 游客 / 加载 / 出错状态均有明确呈现，不显示未经验证的「全部通过」。
- [x] 3.4 运行按钮复用 `chat.send`，支持进度、取消与失败重试；报告完成后自动刷新。
- [x] 3.5 报告历史回看与上一份对比（新增/已消失/仍存在）。
- [x] 3.6 计划编辑器支持选择关联稿件文件；`chapter_review` 不混入资料组织列表。
- [x] 3.7 Vitest 覆盖纯函数、页面状态与计划编辑器回归。

## 4. 固定案例与验收

- [x] 4.1 `make seed` 幂等写入本科毕设固定案例（4 章正文、样本说明资料、研究蓝图、已采用章节计划）。
- [x] 4.2 植入三类可演示问题：方法/结论样本量矛盾、引言/结果跨章重复、结果章论断缺证据；规范符合度因规范库未接入标未检查。
- [x] 4.3 2026-11-23 本地真实模型（MiniMax-M3）+ 本地 S3 跑通固定路径：登录 → 固定案例 → 检查完善 → 运行跨章节检查，生成 11 条问题（目标—结论 4、证据支撑 4、章节间 3，规范符合度 `not_checked` 并写明规范库未接入）；每条含章节/文件/行号、逐字引文与「引文已核对」；刷新后报告仍在；第二次运行生成新报告并与上一份对比（新增 1 / 已消失 1 / 仍存在 10）。截图与报告 JSON 已附 HELE-11 评论。
- [x] 4.4 真实运行观察：模型首轮曾先保存一份 0 finding、三个语义维度均为 `findings` 的中间报告。已在 `validateChapterReview` 增加一致性校验（`findings` 维度至少一条问题；`checked` 维度不得有问题）并补测试；修复后 10:13 运行直接保存完整 11 条报告。另有跨维度重叠（样本量矛盾同时出现在目标—结论与章节间）与引用句重复，已记录为后续提示词调优输入。

## 5. 验证命令

- [x] `cd server && go test ./internal/research/ ./internal/agent/`
- [x] `cd frontend && pnpm typecheck`
- [x] `cd frontend && pnpm test -- ReviewPage cross-chapter-review ResearchArtifactsPanel`
- [ ] `make check`（合并前由验收官/CI 复核）
