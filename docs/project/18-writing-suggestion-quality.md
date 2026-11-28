# 动态写作建议质量契约（HELE-69）

撰写过程中的动态建议必须满足「一条建议 = 一个可核验的原文锚点 + 一条可回查的依据 + 一个可执行的 LaTeX Diff」。
本文记录实现契约、状态机、触发节奏与评测口径；语义判断只由真实模型 + 三源上下文完成，
服务端只做确定性结构校验，**不得**用关键词/正则/模板冒充智能。

## 1. 数据与状态

| 位置 | 字段 | 说明 |
|---|---|---|
| `agent_conversations.kind` | `suggestion` | 建议 Run 的隐藏会话，不出现在会话列表 |
| `agent_runs.trigger` | `chat/auto/manual` | 触发来源；限额与打断率按此统计 |
| `agent_runs.focus_json` | `{filePath,sectionPath,paragraphText,paragraphHash}` | 焦点快照，重试复用 |
| `agent_change_proposals.basis` | `[{type,stance,note,...}]` | 服务端解析后的三源依据，模型无法伪造 |
| `agent_change_proposals.confidence` | `high/medium/low` | 动态建议只接受 `high` |
| `agent_change_proposals.paragraph_hash/anchor_hash` | SHA-256 | 段落级去重与审计 |
| `agent_change_proposals.origin` | `chat/auto/manual` | 建议来源 |
| `agent_change_proposals.drop_reason` | 见下表 | 结构门禁丢弃原因（仅审计，不推送） |
| `agent_change_proposals.reject_reason/reject_note` | 可选 | 用户忽略原因，用于持续评测 |

建议状态：`pending → applied → reverted`；`pending → rejected`；稿件变化时 `pending → stale`；
未通过门禁的建议落 `dropped` 审计行，默认不出现在列表里（`?status=dropped` 才返回）。

## 2. 建议契约（`propose_edit`）

模型必须二选一：调用 `propose_edit`，或调用 `skip_suggestion` 说明本段暂无建议。
`propose_edit` 的关键入参：

```json
{
  "path": "body/ch1.tex",
  "old_text": "逐字取自焦点段落的原文",
  "new_text": "替换后的文本",
  "reason": "这条改动解决的具体问题",
  "suggestion_kind": "evidence_gap|logic_gap|norm|citation|language|structure",
  "confidence": "high",
  "confidence_reason": "为什么整体依据足够",
  "basis": [
    {"type": "evidence", "evidence_id": "本次 search_evidence 返回的 id", "stance": "supports", "note": "…"},
    {"type": "norm", "cite": "N-GBT7714@v1", "stance": "supports", "note": "…"},
    {"type": "plan", "section_id": "ch1.1", "gap": "该段未交代研究问题的来源", "stance": "supports", "note": "…"}
  ]
}
```

服务端确定性校验（不做语义判断）：

1. `old_text` 必须逐字存在于文件，且（动态建议）必须位于焦点段落内。
2. `new_text != old_text`；`reason` 非空。
3. 依据必须是本次运行真实可回查的对象：`evidence_id` 属于本 Run；规范引用号命中项目已采用集合；
   章节计划必须是**已采用**修订且 `section_id + gap` 存在。
4. 动态建议：`confidence=high`、至少一条 `stance=supports`、不得全部 `uncertain`。
5. `suggestion_kind=evidence_gap` 要求本 Run 真实调用过 `search_evidence` 且 0 命中。
6. LaTeX 保护：不得删除 `\cite/\citep/\citet/\ref/\eqref/\pageref/\autoref/\label`；
   数学定界符与公式环境开闭数量保持一致。应用时再兜底校验一次。
7. 去重：同段 pending、同段同类 30 分钟、同段任意类 10 分钟、同 anchor 60 分钟、同段同类已被拒绝
   （手动重检可绕过）→ 落 `dropped` 审计行，不推送。

丢弃原因：`duplicate_pending`、`duplicate_recent`、`rejected_recent`。

## 3. 触发与不打扰

- 默认「专注写作」：**不自动建 Run**，唯一默认入口是编辑器底栏「请导师批注本段」。
- Opt-In 自动检查默认关闭（localStorage `papersage.writing-suggestion.auto`）；开启后：
  - 段落完成（光标离开刚编辑的段落 / 空行 / 标题）→ **3 秒**后检查该段；
  - 仍停在本段继续写 → 停止输入 **8 秒**后才检查。
- 一次停顿最多一个当前段落 Run；同段可见建议由去重窗口收敛；自动触发不弹窗、不切 tab、不抢焦点、不 toast。
- 服务端限额：自动触发间隔 45s，每项目每小时 20 条（auto+manual），同类建议运行期间跳过。
- 呈现形态：正文 Monaco 行内锚点（按类型微弱区分）；≥1280 编辑器右侧可折叠批注栏，进入/切换文件按 pending 决定展开，停留期间新建议只更新底栏计数；<1280 点锚点弹出浮层，不挤压编辑区。AI 助手面板不再承载动态建议。
- 应用后卡片保留依据与「撤销」；撤销要求 `new_text` 仍恰好出现一次，否则 409 并提示手动回退。

## 4. API

| 方法/路径 | 说明 |
|---|---|
| `POST /projects/:id/agent/suggestions` | body `{filePath,paragraphText,sectionPath,trigger,clientRequestId}`；返回 `{runId,streamUrl,conversationId}` 或 `{skipped}` |
| `GET /projects/:id/agent/proposals` | 默认排除 `dropped`；payload 含 `basis/suggestionKind/confidence/sectionPath/origin/createdAt` |
| `POST /agent/change-proposals/:id/reject` | body `{reasonCode,note?}`，`reasonCode ∈ too_vague/out_of_context/no_change_needed/other` |
| `POST /agent/change-proposals/:id/undo` | 仅 `applied`；`new_text` 唯一存在才恢复 |

## 5. 评测指标

自动化（PostgreSQL 示例；`pid`、`since` 由评测脚本传入）：

```sql
-- 建议漏斗（排除 dropped 审计行）
SELECT status, count(*) FROM agent_change_proposals
WHERE project_id = :pid AND created_at >= :since AND status <> 'dropped'
GROUP BY status;

-- 可落地率 = applied / (applied + rejected + stale + reverted)，目标 ≥60%
SELECT round(100.0 * sum((status = 'applied')::int)
  / nullif(sum((status IN ('applied','rejected','stale','reverted'))::int), 0), 1)
FROM agent_change_proposals
WHERE project_id = :pid AND created_at >= :since AND status <> 'dropped';

-- 重复率 = duplicate 丢弃 / 全部建议尝试，目标 ≤10%
SELECT round(100.0 * sum((status = 'dropped' AND drop_reason LIKE 'duplicate%')::int)
  / nullif(count(*), 0), 1)
FROM agent_change_proposals
WHERE project_id = :pid AND created_at >= :since;

-- 结构锚点率（应恒为 100%，作为回归门禁）
SELECT round(100.0 * sum((basis <> '[]'::jsonb)::int) / nullif(count(*), 0), 1)
FROM agent_change_proposals
WHERE project_id = :pid AND created_at >= :since AND status <> 'dropped';

-- 低置信动态建议展示数（结构门禁下恒为 0）
SELECT count(*) FROM agent_change_proposals
WHERE project_id = :pid AND created_at >= :since AND origin IN ('auto','manual')
  AND status <> 'dropped' AND confidence <> 'high';

-- 打断率：自动触发期间编辑器焦点/内容被改动 = 0（E2E 断言；库侧看自动 Run 的条数）
SELECT count(*) FROM agent_runs r
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE c.project_id = :pid AND c.kind = 'suggestion'
  AND r.trigger = 'auto' AND r.created_at >= :since;
```

人工抽样：

- 固定案例：本科毕业论文 + 学术小论文两场景；准备/撰写/检查三阶段；撰写阶段每场景 ≥30 条真实建议
  （真实模型、真实材料、已采用 ≥2 条规范、已采用章节计划）。
- 三档口径：高价值（点名当前段落具体句/术语 + 可直接落地 + 依据可回查）、低价值（方向对但需用户自行翻译成动作）、
  噪音（与当前段落/材料/规范/计划无关或重复）。
- 每条额外记录两个布尔：依据是否真的支持建议；是否与同段已有建议重复。
- 阈值：可落地率 ≥60%；依据锚点率 ≥80%（人工核验；结构锚点率必须 100%）；同段 10 分钟重复率 ≤10%；打断率 = 0。
- 真实论文段落、截图与日志只进私有仓 `GraduateProject` 的验证记录；公开仓只放匿名化聚合与口径。
