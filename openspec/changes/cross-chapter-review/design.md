## 分工边界

- **模型**：四个语义维度的全部结论（目标—结论一致、证据支撑、跨章节重复/矛盾、规范符合度）。
- **确定性代码**：装配真实上下文、校验 payload 结构、核对逐字引文与行号、核对资料是否真的读过、覆盖历史与对比。确定性代码不产生「有没有问题」的语义判断。
- **规范库**：`server/internal/norms.Provider` 是唯一输入边界。HELE-25 未落地时注入 `Unavailable`，上下文里返回 `available=false`，报告只能把规范符合度标为 `not_checked`；服务端拒绝「规范不可用却声称已检查」。

## 数据契约

payload `chapter_review`（存于 `research_artifact_revisions.payload_json`）：

```json
{
  "input": {"planArtifactId": "<uuid>", "planRevision": 1, "planStatus": "adopted|candidate|missing", "normsAvailable": false, "normsNote": "…", "norms": []},
  "dimensions": [{"key": "goal_conclusion|evidence_support|cross_section|norm_compliance", "status": "checked|findings|insufficient|not_checked|failed", "note": "非 checked/findings 必填"}],
  "findings": [{
    "id": "f1", "dimension": "goal_conclusion", "severity": "high|medium|low",
    "sectionId": "s2", "sectionTitle": "研究方法", "filePath": "chapters/method.tex",
    "quote": "逐字原文", "lineStart": 2, "lineEnd": 2,
    "problem": "…", "basis": [{"kind": "text|plan|source|norm", "ref": "…", "quote": "…"}],
    "nextStep": "…", "confidence": "high|low", "anchorVerified": true
  }],
  "sourcesRead": [{"kind": "latex|plan|document|source|norm", "name": "…"}]
}
```

- `input`、`sourcesRead`、`anchorVerified`、`lineStart/lineEnd` 由工具层按本轮真实读取与定位覆盖；模型自报值不采信。
- findings 上限 50；维度必须四个齐全；`insufficient/not_checked/failed` 必填 `note`。
- `filePath` 必须是项目现有稿件的相对路径；`sectionId` 必须属于引用的章节计划修订；`plan` 依据的 `ref` 必须是计划章节 id。

## 上下文装配

`get_chapter_review_context` 一次返回：

- 项目名称/描述/场景、研究蓝图与课题要求；
- 已采用章节计划（无已采用则用当前候选并明确警告）；
- 已采用规范条目或显式不可用原因；
- 计划映射到的各章正文（单文件 40KB、总量 160KB 上限，超出标注 `truncated` 并提示用 `read_latex_file`）；
- 其余 LaTeX 文件与已入库资料清单；
- 材料缺失 warnings。

需要外部证据时模型必须调用 `read_document` / `read_source_version`；`basis.kind=source` 会校验该资料在本轮确实被读过。

## 展示

- 基础结构检查：现有 5 项本地规则，标注「本地确定性检查」。
- 跨章节检查：四个维度状态 + 问题列表（严重度、章节/文件/行号、原文引文、依据可展开、下一步、引文已核对）+ 读取材料 + 历史对比。
- 覆盖不完整时显示原因，绝不显示「全部通过」；模型未配置时显示「去设置模型」，游客显示登录提示。
- 修复前可能已存在「维度状态与问题列表不一致」的历史报告：前端按同一一致性规则识别并单列「已隔离」提示，不进入历史选择与报告展示，也不改写用户数据。

## 已知边界

- 模型输出不保证逐字可复现；报告快照与 `run_id` 记录当次结论，确定性部分与固定案例可复现。
- 单次上下文有上限，超出章节需模型主动续读；未读到的内容必须在维度 `note` 中如实标注。
- 不做查重、AI 检测、完整规范库与自动改稿。
