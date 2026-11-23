/**
 * 跨章节检查的固定提示词。
 *
 * 语义判断全部交给真实模型；提示词只规定「读什么、怎么给依据、什么情况下
 * 必须承认证据不足」。不使用关键词/正则规则代替模型结论。
 */
export const CROSS_CHAPTER_REVIEW_PROMPT = `请对当前项目做一次跨章节检查。这是学术论证审阅，不是 LaTeX 格式检查；只报告你能从已读材料中证明的问题。

第一步：调用 get_chapter_review_context，读取项目背景与要求、章节计划、已采用学术规范、各章正文和资料清单。
- 计划状态为 candidate 时，在相关维度注明「计划尚未采用」。
- warnings 里的材料缺失必须如实反映到维度状态，不得忽略。
- 需要外部证据时，先用 read_document 或 read_source_version 真正读取资料；没有读过的资料不得作为依据。

检查四个维度（四个都必须返回，key 固定）：
1. goal_conclusion：章节计划的目标/职责/问题与正文实际结论是否一致。
2. evidence_support：正文与计划中的关键论断是否有已读资料或原文证据支撑；没有证据的论断要逐条指出。
3. cross_section：不同章节之间是否重复表述或相互矛盾（包括样本量、对象、方法、结论）。
4. norm_compliance：是否符合已采用学术规范。norms.available=false 时必须为 not_checked 并写明原因；norms.entries 为空但 available=true 时为 insufficient；不得在没有规范依据时声称已检查。

维度状态只能是：
- checked：完成且未发现问题；
- findings：完成并发现问题；
- insufficient：材料不足（必须写 note 说清缺什么）；
- not_checked：未检查（必须写 note 说明原因）；
- failed：读取或检查失败（必须写 note）。

面向用户的文字（dimension.note、problem、nextStep）必须用本科生能懂的中文：章节写标题（如「研究方法」）而不是 s1；不提工具名、字段名、内部 id、英文枚举；状态用中文（如「尚未核对」「材料不足」）；引用原文保持逐字不变。

问题条目要求：
- id 唯一；dimension、severity（high/medium/low）、problem、nextStep 必填。
- sectionId 必须来自章节计划；filePath 若给出必须与计划的 filePath 一致。定位不到原文时不要编造行号。
- quote 必须逐字复制原文（保留标点与 LaTeX 命令），不得改写、拼接、翻译；服务端会核对并计算行号。找不到逐字原文就不要写 quote，改用 plan 依据。
- basis 至少一条，kind 只能是：
  - text：ref=文件路径，quote=原文逐字摘录；
  - plan：ref=章节 id，quote=计划中的逐字内容（标题/职责/问题/论断）；
  - source：ref=本轮已读取的资料名称或 id，quote=资料中的逐字摘录；
  - norm：ref=已采用规范的 id 或标题。
- 材料不足时不要编造问题；把维度标为 insufficient 并在 note 里写清缺什么。

最后调用 create_research_artifact 保存报告，kind=chapter_review，payload_json 结构：
{
  "input": {},
  "dimensions": [
    {"key": "goal_conclusion", "status": "findings|checked|insufficient|not_checked|failed", "note": "非 checked/findings 时必填"},
    {"key": "evidence_support", "status": "..."},
    {"key": "cross_section", "status": "..."},
    {"key": "norm_compliance", "status": "..."}
  ],
  "findings": [
    {
      "id": "f1",
      "dimension": "goal_conclusion",
      "severity": "high",
      "sectionId": "s2",
      "sectionTitle": "研究方法",
      "filePath": "chapters/method.tex",
      "quote": "原文逐字摘录",
      "problem": "问题是什么",
      "basis": [{"kind": "text", "ref": "chapters/method.tex", "quote": "依据原文"}, {"kind": "plan", "ref": "s2", "quote": "计划原文"}],
      "nextStep": "用户下一步可以怎么改",
      "confidence": "high|low"
    }
  ],
  "sourcesRead": []
}
input 与 sourcesRead 由服务端按本轮实际读取覆盖，不要自行编造来源。服务端会核对 text/plan/source 的逐字引文与「资料是否真的读过」；核对失败会返回原因，请重新读取原文修正后重试，不要绕过核对，也不要因此改成没有依据的结论。

报告只陈述发现与依据，不要输出「全部通过」、分数或对论文质量的总体保证。`
