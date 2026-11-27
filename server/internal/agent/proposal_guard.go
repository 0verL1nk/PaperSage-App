package agent

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 动态写作建议的质量门禁。
//
// 边界：这里只做**确定性结构校验**——锚点是否逐字存在、依据是否真实可回查、
// 引用与公式是否被破坏、置信度声明是否自洽。判断「这句话是否空泛」「这条依据
// 是否真的支持建议」属于语义问题，只能由模型与人工抽样负责；用关键词/正则去
// 判语义会触犯项目「严禁伪智能」红线。

const (
	BasisEvidence = "evidence"
	BasisNorm     = "norm"
	BasisPlan     = "plan"

	StanceSupports  = "supports"
	StanceUncertain = "uncertain"

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

var suggestionKinds = map[string]bool{
	"evidence_gap": true,
	"logic_gap":    true,
	"norm":         true,
	"citation":     true,
	"language":     true,
	"structure":    true,
}

// proposalBasisInput 是模型提交的依据声明。除 note/stance 外，其余字段都只是
// 「指向某个真实对象」的引用号，由服务端解析成可回查的元数据。
type proposalBasisInput struct {
	Type       string `json:"type" jsonschema:"description=evidence/norm/plan"`
	Stance     string `json:"stance,omitempty" jsonschema:"description=supports 或 uncertain；依据与建议的关系由你判断，不得把不确定说成支持"`
	Note       string `json:"note,omitempty" jsonschema:"description=这条依据如何支持本建议（一句话）"`
	EvidenceID string `json:"evidence_id,omitempty" jsonschema:"description=search_evidence 返回的 evidence_id；必须是本次运行检索到的"`
	Cite       string `json:"cite,omitempty" jsonschema:"description=已采用规范的引用号，如 N-GBT7714@v1"`
	SectionID  string `json:"section_id,omitempty" jsonschema:"description=已采用章节计划中的章节 id"`
	Gap        string `json:"gap,omitempty" jsonschema:"description=该段相对章节计划的具体缺口"`
}

// proposalBasis 是落库/推给前端的依据：所有来源信息由服务端补全。
type proposalBasis struct {
	Type           string `json:"type"`
	Stance         string `json:"stance"`
	Note           string `json:"note,omitempty"`
	EvidenceID     string `json:"evidenceId,omitempty"`
	SourceName     string `json:"sourceName,omitempty"`
	SourceVersion  string `json:"sourceVersion,omitempty"`
	Locator        string `json:"locator,omitempty"`
	Quote          string `json:"quote,omitempty"`
	Cite           string `json:"cite,omitempty"`
	NormVersion    int    `json:"normVersion,omitempty"`
	NormTitle      string `json:"normTitle,omitempty"`
	NormRule       string `json:"normRule,omitempty"`
	NormSource     string `json:"normSource,omitempty"`
	SectionID      string `json:"sectionId,omitempty"`
	SectionTitle   string `json:"sectionTitle,omitempty"`
	Gap            string `json:"gap,omitempty"`
	PlanArtifactID string `json:"planArtifactId,omitempty"`
	PlanRevision   int    `json:"planRevision,omitempty"`
}

// proposeEditArgs 是 propose_edit 的完整入参（聊天与动态建议共用）。
type proposeEditArgs struct {
	Path    string `json:"path" jsonschema:"description=文件路径，如 main.tex"`
	OldText string `json:"old_text" jsonschema:"description=要被替换的原文片段（必须与文件当前内容逐字一致）"`
	NewText string `json:"new_text" jsonschema:"description=替换后的新文"`
	Reason  string `json:"reason" jsonschema:"description=这条改动解决的具体问题；空泛点评会被拒绝"`
	// 动态建议的质量契约；聊天模式下可省略。
	SuggestionKind   string               `json:"suggestion_kind,omitempty" jsonschema:"description=evidence_gap/logic_gap/norm/citation/language/structure"`
	Confidence       string               `json:"confidence,omitempty" jsonschema:"description=high/medium/low；动态建议只接受 high"`
	ConfidenceReason string               `json:"confidence_reason,omitempty" jsonschema:"description=为什么整体依据足够或不足"`
	Basis            []proposalBasisInput `json:"basis,omitempty" jsonschema:"description=三源依据数组；动态建议必须至少一条真实依据"`
	// NormRefs 是旧聊天契约；保留兼容，服务端会折算成 basis。
	NormRefs []string `json:"norm_refs,omitempty" jsonschema:"description=兼容字段；新调用请使用 basis（type=norm）"`
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// md5HexOf 沿用 base_content_hash 既有的 MD5 口径：应用建议时按同一算法重校验。
func md5HexOf(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// resolveProposalBasis 把模型给的引用号解析成本次运行里真实存在的依据。
// 任一条无法回查就整条拒绝：宁可让模型重试，也不落一条无法核对的依据。
func resolveProposalBasis(ctx context.Context, db *gorm.DB, provider norms.Provider,
	projectID, runID store.UUID, inputs []proposalBasisInput, legacyNormRefs []string) ([]proposalBasis, error) {

	var normCites []string
	for _, in := range inputs {
		if in.Type == BasisNorm && strings.TrimSpace(in.Cite) != "" {
			normCites = append(normCites, strings.TrimSpace(in.Cite))
		}
	}
	normCites = append(normCites, legacyNormRefs...)
	normMeta := map[string]normRefMeta{}
	if len(normCites) > 0 {
		encoded, err := resolveNormRefs(ctx, provider, projectID, normCites)
		if err != nil {
			return nil, err
		}
		var metas []normRefMeta
		if err := json.Unmarshal(encoded, &metas); err != nil {
			return nil, err
		}
		for _, m := range metas {
			normMeta[m.Cite] = m
		}
	}

	var plan *research.LoadedChapterPlan
	if planInputs(inputs) {
		loaded, _, warnings := research.LoadAdoptedChapterPlan(ctx, db, projectID)
		if loaded == nil || loaded.Status != "adopted" {
			msg := "项目没有已采用的章节计划，不能把章节计划作为建议依据"
			if len(warnings) > 0 {
				msg += "（" + strings.Join(warnings, "；") + "）"
			}
			return nil, fmt.Errorf("%s", msg)
		}
		plan = loaded
	}

	out := make([]proposalBasis, 0, len(inputs)+len(legacyNormRefs))
	for _, in := range inputs {
		stance := strings.TrimSpace(in.Stance)
		if stance == "" {
			stance = StanceSupports
		}
		if stance != StanceSupports && stance != StanceUncertain {
			return nil, fmt.Errorf("basis.stance 只能是 supports 或 uncertain，收到 %q", in.Stance)
		}
		switch in.Type {
		case BasisEvidence:
			id, err := uuid.Parse(strings.TrimSpace(in.EvidenceID))
			if err != nil {
				return nil, fmt.Errorf("basis.type=evidence 需要合法的 evidence_id")
			}
			var row store.AgentEvidence
			if err := db.WithContext(ctx).
				Where("id = ? AND run_id = ?", store.UUID{UUID: id}, runID).First(&row).Error; err != nil {
				return nil, fmt.Errorf("evidence_id %s 不属于本次运行：只能引用 search_evidence 本次真实检索到的证据", in.EvidenceID)
			}
			item := proposalBasis{
				Type: BasisEvidence, Stance: stance, Note: strings.TrimSpace(in.Note),
				EvidenceID: row.ID.String(), SourceName: row.SourceName,
			}
			if row.SourceVersion != nil {
				item.SourceVersion = *row.SourceVersion
			}
			if row.Locator != nil {
				item.Locator = *row.Locator
			}
			if row.Quote != nil {
				item.Quote = truncateRunes(*row.Quote, 500)
			}
			out = append(out, item)
		case BasisNorm:
			cite := strings.TrimSpace(in.Cite)
			meta, ok := normMeta[cite]
			if !ok {
				return nil, fmt.Errorf("引用号 %s 不在本项目生效的规范里", cite)
			}
			out = append(out, proposalBasis{
				Type: BasisNorm, Stance: stance, Note: strings.TrimSpace(in.Note), Cite: meta.Cite,
				NormVersion: meta.Version, NormTitle: meta.Title, NormRule: meta.Rule, NormSource: meta.Source,
			})
		case BasisPlan:
			sectionID := strings.TrimSpace(in.SectionID)
			gap := strings.TrimSpace(in.Gap)
			if sectionID == "" || gap == "" {
				return nil, fmt.Errorf("basis.type=plan 需要 section_id 与非空 gap")
			}
			var section *research.ChapterSection
			for i := range plan.Plan.Sections {
				if plan.Plan.Sections[i].ID == sectionID {
					section = &plan.Plan.Sections[i]
					break
				}
			}
			if section == nil {
				return nil, fmt.Errorf("章节 %s 不在已采用的章节计划中", sectionID)
			}
			out = append(out, proposalBasis{
				Type: BasisPlan, Stance: stance, Note: strings.TrimSpace(in.Note), SectionID: section.ID,
				SectionTitle: section.Title, Gap: gap,
				PlanArtifactID: plan.ArtifactID.String(), PlanRevision: plan.Revision,
			})
		default:
			return nil, fmt.Errorf("未知的 basis.type %q：只能是 evidence/norm/plan", in.Type)
		}
	}
	// 旧聊天契约的规范引用折算为 basis，避免前端两套字段并存。
	for _, cite := range legacyNormRefs {
		meta, ok := normMeta[strings.TrimSpace(cite)]
		if !ok {
			continue
		}
		seen := false
		for _, b := range out {
			if b.Type == BasisNorm && b.Cite == meta.Cite {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		out = append(out, proposalBasis{
			Type: BasisNorm, Stance: StanceSupports, Cite: meta.Cite,
			NormVersion: meta.Version, NormTitle: meta.Title, NormRule: meta.Rule, NormSource: meta.Source,
		})
	}
	return out, nil
}

func planInputs(inputs []proposalBasisInput) bool {
	for _, in := range inputs {
		if in.Type == BasisPlan {
			return true
		}
	}
	return false
}

// validateSuggestionQuality 是动态建议的结构门禁。
func validateSuggestionQuality(in proposeEditArgs, basis []proposalBasis) error {
	if !suggestionKinds[in.SuggestionKind] {
		return fmt.Errorf("suggestion_kind 必须是 evidence_gap/logic_gap/norm/citation/language/structure 之一")
	}
	switch in.Confidence {
	case ConfidenceHigh:
	case ConfidenceMedium, ConfidenceLow:
		return fmt.Errorf("动态建议只接受 confidence=high；当前为 %s，请补充可回查依据或改用 skip_suggestion", in.Confidence)
	default:
		return fmt.Errorf("动态建议必须给出 confidence=high 并说明理由")
	}
	if len(basis) == 0 {
		return fmt.Errorf("动态建议必须至少给出一条依据：材料原文（evidence）、已采用规范（norm）或章节计划缺口（plan）")
	}
	hasSupport := false
	for _, b := range basis {
		if b.Stance == StanceSupports {
			hasSupport = true
			break
		}
	}
	if !hasSupport {
		return fmt.Errorf("所有依据都标记为 uncertain，不能作为动态建议推送；请补充支持性依据或改用 skip_suggestion")
	}
	return nil
}

// ── LaTeX 完整性保护 ────────────────────────────────────────────────

var protectedLatexCommand = regexp.MustCompile(`\\(?:cite|citep|citet|ref|eqref|pageref|autoref|label)\*?(?:\[[^\]]*\])?\{[^}]*\}`)
var mathEnvironment = regexp.MustCompile(`\\(begin|end)\{([a-zA-Z*]+)\}`)

// latexIntegrityError 阻止建议删除引用、标签或破坏公式定界符。
// 只做结构比较，不判断文字好坏。
func latexIntegrityError(oldText, newText string) error {
	for _, cmd := range protectedLatexCommand.FindAllString(oldText, -1) {
		if strings.Count(newText, cmd) < strings.Count(oldText, cmd) {
			return fmt.Errorf("新文删除了受保护的引用命令 %s；建议不得删除 \\cite/\\ref/\\label，如需调整请保留原命令", cmd)
		}
	}

	envs := map[string][2]int{}
	for _, m := range mathEnvironment.FindAllStringSubmatch(oldText, -1) {
		pair := envs[m[2]]
		if m[1] == "begin" {
			pair[0]++
		} else {
			pair[1]++
		}
		envs[m[2]] = pair
	}
	for env, pair := range envs {
		begin, end := strings.Count(newText, `\begin{`+env+`}`), strings.Count(newText, `\end{`+env+`}`)
		if begin != pair[0] || end != pair[1] {
			return fmt.Errorf("新文改变了 %s 环境的开闭数量（原文 %d/%d，新文 %d/%d）；公式环境必须原样保留", env, pair[0], pair[1], begin, end)
		}
	}

	oldCounts := mathDelimiters(oldText)
	newCounts := mathDelimiters(newText)
	for _, name := range []string{"$$", "$", `\[`, `\]`, `\(`, `\)`} {
		if oldCounts[name] != newCounts[name] {
			return fmt.Errorf("新文改变了数学定界符 %s 的数量（原文 %d，新文 %d）；公式必须原样保留", name, oldCounts[name], newCounts[name])
		}
	}
	return nil
}

func mathDelimiters(s string) map[string]int {
	s = strings.ReplaceAll(s, `\$`, "")
	dd := strings.Count(s, "$$")
	return map[string]int{
		"$$": dd,
		"$":  strings.Count(s, "$") - 2*dd,
		`\[`: strings.Count(s, `\[`),
		`\]`: strings.Count(s, `\]`),
		`\(`: strings.Count(s, `\(`),
		`\)`: strings.Count(s, `\)`),
	}
}

// ── 去重 ────────────────────────────────────────────────────────────

const (
	dedupeSameKindWindow  = 30 * time.Minute
	dedupeParagraphWindow = 10 * time.Minute
	dedupeAnchorWindow    = 60 * time.Minute
)

// duplicateDropReason 返回命中的丢弃原因；空字符串表示允许落库。
// 时间窗口与同段 pending 是权威去重（服务端判断，不信任客户端）。
func duplicateDropReason(ctx context.Context, db *gorm.DB, projectID store.UUID,
	filePath, paragraphHash, kind, anchorHash string, bypassRejected bool) string {

	now := time.Now().UTC()
	count := func(query string, args ...any) int64 {
		var n int64
		_ = db.WithContext(ctx).Model(&store.AgentChangeProposal{}).
			Where("project_id = ? AND file_path = ? AND paragraph_hash = ?", projectID, filePath, paragraphHash).
			Where(query, args...).Count(&n).Error
		return n
	}
	if count("status = ?", ProposalPending) > 0 {
		return "duplicate_pending"
	}
	if count("status <> ? AND suggestion_kind = ? AND created_at >= ?", "dropped", kind, now.Add(-dedupeSameKindWindow)) > 0 {
		return "duplicate_recent"
	}
	if count("status <> ? AND created_at >= ?", "dropped", now.Add(-dedupeParagraphWindow)) > 0 {
		return "duplicate_recent"
	}
	var n int64
	_ = db.WithContext(ctx).Model(&store.AgentChangeProposal{}).
		Where("project_id = ? AND file_path = ? AND anchor_hash = ? AND status <> ? AND created_at >= ?",
			projectID, filePath, anchorHash, "dropped", now.Add(-dedupeAnchorWindow)).Count(&n).Error
	if n > 0 {
		return "duplicate_recent"
	}
	if !bypassRejected && count("status = ? AND suggestion_kind = ?", ProposalRejected, kind) > 0 {
		return "rejected_recent"
	}
	return ""
}

// recordDroppedProposal 落一条审计行：不进入 pending、不推给用户，
// 但保留可查的丢弃原因与依据，供质量评测。
func recordDroppedProposal(ctx context.Context, db *gorm.DB, p *store.AgentChangeProposal, reason string) {
	p.Status = "dropped"
	p.DropReason = reason
	_ = db.WithContext(ctx).Create(p).Error
}

// createProposal 是 propose_edit 的唯一实现路径：聊天与动态建议共用。
// strict 非空时执行动态建议的锚点/依据/置信度/去重门禁。
func createProposal(ctx context.Context, db *gorm.DB, stg storage.Storage, provider norms.Provider,
	projectID, runID store.UUID, in proposeEditArgs, strict *SuggestionSpec) (string, error) {

	if err := validateAgentPath(in.Path); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.OldText) == "" {
		return "", fmt.Errorf("old_text 不能为空")
	}
	if in.OldText == in.NewText {
		return "", fmt.Errorf("new_text 与 old_text 相同，没有可应用的改动；无需修改时请调用 skip_suggestion")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return "", fmt.Errorf("reason 不能为空：请说明这条改动解决的具体问题")
	}
	if strict != nil && in.Path != strict.FilePath {
		return "", fmt.Errorf("只能修改当前焦点文件 %s", strict.FilePath)
	}
	_, content, err := findLatex(ctx, db, stg, projectID, in.Path)
	if err != nil {
		return "", err
	}
	if !strings.Contains(content, in.OldText) {
		return "", fmt.Errorf("old_text 在 %s 的当前内容中找不到。请先 read_latex_file 取回原文，再给出逐字一致的片段", in.Path)
	}
	if strict != nil && !strings.Contains(strict.ParagraphText, in.OldText) {
		return "", fmt.Errorf("old_text 必须逐字取自当前焦点段落；不要修改段落之外的文字")
	}
	if err := latexIntegrityError(in.OldText, in.NewText); err != nil {
		return "", err
	}

	basis, err := resolveProposalBasis(ctx, db, provider, projectID, runID, in.Basis, in.NormRefs)
	if err != nil {
		return "", err
	}
	basisJSON, err := json.Marshal(basis)
	if err != nil {
		return "", err
	}

	origin := "chat"
	paragraphHash := ""
	anchorHash := sha256Hex(in.OldText)
	sectionPath := ""
	if strict != nil {
		if err := validateSuggestionQuality(in, basis); err != nil {
			return "", err
		}
		if in.SuggestionKind == "evidence_gap" && !strict.EvidenceGapSupported() {
			return "", fmt.Errorf("声称缺少材料依据前必须先调用 search_evidence 并得到 0 条命中；本次运行没有这样的检索记录")
		}
		origin = strict.Trigger
		paragraphHash = strict.ParagraphHash
		sectionPath = strict.SectionPath
	}

	reason := strings.TrimSpace(in.Reason)
	p := &store.AgentChangeProposal{
		ID: store.MustNewUUID(), ProjectID: projectID, RunID: &runID,
		FilePath: in.Path, BaseContentHash: md5HexOf(content),
		OldText: in.OldText, NewText: in.NewText, Reason: &reason,
		NormRefs: datatypes.JSON([]byte("[]")), Basis: datatypes.JSON(basisJSON),
		SuggestionKind: in.SuggestionKind, Confidence: in.Confidence,
		AnchorHash: anchorHash, ParagraphHash: paragraphHash, SectionPath: sectionPath,
		Origin: origin, Status: ProposalPending,
	}
	if strings.TrimSpace(in.ConfidenceReason) != "" {
		cr := strings.TrimSpace(in.ConfidenceReason)
		p.ConfidenceReason = &cr
	}
	if len(in.NormRefs) > 0 {
		// norm_refs 列保留给旧读端；内容仍由服务端解析，模型无法伪造。
		if encoded, err := resolveNormRefs(ctx, provider, projectID, in.NormRefs); err == nil {
			p.NormRefs = encoded
		}
	}

	if strict != nil {
		if drop := duplicateDropReason(ctx, db, projectID, in.Path, paragraphHash,
			in.SuggestionKind, anchorHash, strict.Trigger == SuggestionTriggerManual); drop != "" {
			recordDroppedProposal(ctx, db, p, drop)
			return "", fmt.Errorf("该建议未通过去重门禁（%s），已记录但不会推送给用户；不要重复提交同类建议", drop)
		}
	}

	if err := db.WithContext(ctx).Create(p).Error; err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"已生成修改建议（%s，建议 id %s），等待用户确认。不要声称已修改文件——用户确认后才会应用。",
		in.Path, p.ID.String()), nil
}
