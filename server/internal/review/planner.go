package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

// 服务端确定性规划：校验用户选择的范围，拆成 (章节, 维度) 单元并锁定输入快照。
// 这里不做任何语义判断，只回答「检查哪些单元、每个单元看到什么材料」。

const (
	// ScopeVersion / ContextVersion 参与增量指纹：结构或上下文契约变化时自动重跑。
	ScopeVersion   = "review-scope-v1"
	ContextVersion = "review-context-v1"

	reviewChapterBytes    = 40 * 1024
	reviewCollectionBytes = 160 * 1024
	maxFindingsPerUnit    = 50
)

// CreateRequest 是 POST /projects/:id/review-runs 的请求体。
type CreateRequest struct {
	ClientRequestID string       `json:"clientRequestId"`
	Scope           ScopeRequest `json:"scope"`
}

// ScopeRequest 是用户勾选的检查范围。
type ScopeRequest struct {
	FilePaths   []string `json:"filePaths"`
	SectionIDs  []string `json:"sectionIds"`
	Dimensions  []string `json:"dimensions"`
	Incremental bool     `json:"incremental"`
}

// ScopeSnapshot 是锁定后的范围（写入 review_runs.scope_json，并进入报告 scope）。
type ScopeSnapshot struct {
	Dimensions  []string          `json:"dimensions"`
	Files       []string          `json:"files"`
	FileHashes  map[string]string `json:"fileHashes,omitempty"`
	SectionIDs  []string          `json:"sectionIds,omitempty"`
	Incremental bool              `json:"incremental"`
	Warnings    []string          `json:"warnings,omitempty"`
	Units       []ScopeUnit       `json:"units,omitempty"`
}

// ScopeUnit 是报告里对每个单元执行结果的概述。
type ScopeUnit struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Source      string `json:"source"` // fresh | reused
	Fingerprint string `json:"fingerprint"`
}

type projectContext struct {
	project      store.Project
	files        []latex.ProjectText
	fileByPath   map[string]string
	structure    latex.Structure
	units        []UnitOption
	plan         *research.LoadedChapterPlan
	norms        []norms.Entry
	normsOK      bool
	normsNote    string
	brief        writingstate.Brief
	requirements []writingstate.Requirement
	documents    []store.Document
	warnings     []string
}

type plannedUnit struct {
	Kind        string
	Label       string
	FilePaths   []string
	SectionIDs  []string
	Dimensions  []string
	Fingerprint string
	ContextJSON []byte
	Instruction string
	Truncated   bool
	Reused      *store.ReviewUnit
}

func (s *Service) loadProjectContext(ctx context.Context, userID, projectID store.UUID) (*projectContext, error) {
	var project store.Project
	if err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", projectID, userID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	if s.Files == nil {
		return nil, errors.New("文件服务不可用")
	}
	pairs, err := s.Files.LoadTextContents(ctx, projectID.UUID, 4)
	if err != nil {
		return nil, err
	}
	files := make([]latex.ProjectText, 0, len(pairs))
	byPath := map[string]string{}
	for _, pair := range pairs {
		p := latex.NormalizeTexPath(pair[0])
		if !isStructureFile(p) {
			continue
		}
		files = append(files, latex.ProjectText{Path: p, Content: pair[1]})
		byPath[p] = pair[1]
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	structure := latex.ParseStructure(files)
	plan, _, planWarnings := research.LoadAdoptedChapterPlan(ctx, s.DB, projectID)
	units, unitWarnings := BuildUnits(structure, plan)

	pc := &projectContext{
		project: project, files: files, fileByPath: byPath, structure: structure,
		units: units, plan: plan, normsNote: "未读取学术规范",
	}
	for _, w := range structure.Warnings {
		pc.warnings = append(pc.warnings, w.Detail)
	}
	pc.warnings = append(pc.warnings, planWarnings...)
	pc.warnings = append(pc.warnings, unitWarnings...)

	if s.Norms == nil {
		pc.normsNote = "学术规范库尚未接入，规范符合度本次只能标记为未检查"
	} else {
		entries, normsErr := s.Norms.AdoptedForProject(ctx, projectID)
		switch {
		case errors.Is(normsErr, norms.ErrUnavailable):
			pc.normsNote = "学术规范库尚未接入，规范符合度本次只能标记为未检查"
		case normsErr != nil:
			pc.normsNote = "读取已采用学术规范失败，规范符合度本次只能标记为未检查"
		default:
			pc.normsOK = true
			pc.norms = entries
			if len(entries) == 0 {
				pc.normsNote = "项目尚未采用任何学术规范，规范符合度本次只能标记为证据不足或未检查"
			}
		}
	}
	if !pc.normsOK && pc.normsNote != "" {
		pc.warnings = append(pc.warnings, pc.normsNote)
	}

	state, stateErr := writingstate.NewService(s.DB).GetForAgent(ctx, projectID.UUID, userID.UUID)
	if stateErr == nil {
		pc.brief = state.Brief
		pc.requirements = state.Requirements
	} else {
		pc.warnings = append(pc.warnings, "读取项目蓝图失败："+stateErr.Error())
	}
	if err := s.DB.WithContext(ctx).Where("project_id = ? AND status = ?", projectID, "uploaded").
		Order("created_at").Find(&pc.documents).Error; err != nil {
		return nil, err
	}
	if len(pc.documents) == 0 {
		pc.warnings = append(pc.warnings, "项目还没有已入库的参考文献；涉及外部证据的论断只能标记为证据不足")
	}
	return pc, nil
}

// Plan 校验范围并生成待执行单元；返回的报告 scope 已按实际单元文件收敛。
func (s *Service) Plan(ctx context.Context, pc *projectContext, model *agent.ResolvedModel, req CreateRequest) ([]plannedUnit, *ScopeSnapshot, error) {
	if strings.TrimSpace(req.ClientRequestID) == "" || len(req.ClientRequestID) > 64 {
		return nil, nil, httpapi.BadRequest("缺少有效的请求标识，请刷新页面后重试")
	}
	scope := req.Scope
	if len(scope.Dimensions) == 0 {
		return nil, nil, httpapi.BadRequest("请至少选择一个检查维度")
	}
	dims := dedupeStrings(scope.Dimensions)
	var chapterDims, collectionDims []string
	for _, key := range dims {
		dim, ok := LookupDimension(key)
		if !ok {
			return nil, nil, httpapi.BadRequest("不支持的检查维度：" + key)
		}
		if !DimensionAllowed(pc.project.Scenario, key) {
			return nil, nil, httpapi.BadRequest(fmt.Sprintf("维度「%s」不适用于当前项目场景", dim.Label))
		}
		if dim.Scope == "collection" {
			collectionDims = append(collectionDims, key)
		} else {
			chapterDims = append(chapterDims, key)
		}
	}
	if len(scope.FilePaths) == 0 {
		return nil, nil, httpapi.BadRequest("请至少选择一个章节")
	}
	selected := map[string]bool{}
	for _, raw := range scope.FilePaths {
		p := latex.NormalizeTexPath(raw)
		if !strings.HasSuffix(strings.ToLower(p), ".tex") {
			return nil, nil, httpapi.BadRequest("只能选择 .tex 章节文件：" + raw)
		}
		if _, ok := pc.fileByPath[p]; !ok {
			return nil, nil, httpapi.BadRequest("文件不存在：" + raw)
		}
		selected[p] = true
	}
	var chosen []UnitOption
	covered := map[string]bool{}
	for _, u := range pc.units {
		hit := false
		for _, f := range u.FilePaths {
			if selected[f] {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		chosen = append(chosen, u)
		for _, f := range u.FilePaths {
			covered[f] = true
		}
	}
	if len(chosen) == 0 {
		return nil, nil, httpapi.BadRequest("所选文件不属于任何可检查章节；请先在主文件中包含它，或把它加入章节计划")
	}
	for p := range selected {
		if !covered[p] {
			return nil, nil, httpapi.BadRequest("文件不在可检查章节中：" + p)
		}
	}
	if len(scope.SectionIDs) > 0 && pc.plan != nil {
		known := map[string]bool{}
		for _, sec := range pc.plan.Plan.Sections {
			known[sec.ID] = true
		}
		for _, id := range scope.SectionIDs {
			if !known[id] {
				return nil, nil, httpapi.BadRequest("章节不在当前章节计划中：" + id)
			}
		}
	}

	effectiveFiles := make([]string, 0, len(covered))
	for f := range covered {
		effectiveFiles = append(effectiveFiles, f)
	}
	sort.Strings(effectiveFiles)
	sectionIDs := make([]string, 0)
	for _, u := range chosen {
		sectionIDs = append(sectionIDs, u.SectionIDs...)
	}
	sectionIDs = dedupeStrings(sectionIDs)

	var planned []plannedUnit
	for _, u := range chosen {
		for _, key := range chapterDims {
			dim, _ := LookupDimension(key)
			ctxJSON, truncated := buildChapterUnitContext(pc, u, key)
			planned = append(planned, plannedUnit{
				Kind: "chapter", Label: u.Title + " · " + dim.Label,
				FilePaths: u.FilePaths, SectionIDs: u.SectionIDs, Dimensions: []string{key},
				Fingerprint: fingerprintUnit(pc, model, []string{key}, u.FilePaths, u.SectionIDs),
				ContextJSON: ctxJSON, Truncated: truncated,
			})
		}
	}
	for _, key := range collectionDims {
		dim, _ := LookupDimension(key)
		ctxJSON, truncated, err := buildCollectionUnitContext(pc, chosen, key)
		if err != nil {
			return nil, nil, err
		}
		planned = append(planned, plannedUnit{
			Kind: "collection", Label: dim.Label,
			FilePaths: effectiveFiles, SectionIDs: sectionIDs, Dimensions: []string{key},
			Fingerprint: fingerprintUnit(pc, model, []string{key}, effectiveFiles, sectionIDs),
			ContextJSON: ctxJSON, Truncated: truncated,
		})
	}
	if len(planned) == 0 {
		return nil, nil, httpapi.BadRequest("所选范围没有可执行的检查单元")
	}
	for i := range planned {
		planned[i].Instruction = buildUnitInstruction(planned[i], pc)
	}
	hashes := map[string]string{}
	for _, f := range effectiveFiles {
		sum := sha256.Sum256([]byte(pc.fileByPath[f]))
		hashes[f] = hex.EncodeToString(sum[:])
	}
	scopeOut := &ScopeSnapshot{
		Dimensions: dims, Files: effectiveFiles, FileHashes: hashes, SectionIDs: sectionIDs,
		Incremental: scope.Incremental, Warnings: pc.warnings,
	}
	return planned, scopeOut, nil
}

type unitChapterContext struct {
	FilePath  string `json:"filePath"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
	SizeBytes int    `json:"sizeBytes"`
}

type unitPlanContext struct {
	Status     string                    `json:"status"`
	ArtifactID string                    `json:"artifactId,omitempty"`
	Revision   int                       `json:"revision,omitempty"`
	Sections   []research.ChapterSection `json:"sections,omitempty"`
}

type unitDocumentContext struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"sizeBytes"`
}

type unitContextJSON struct {
	Unit struct {
		ID         string   `json:"id"`
		Label      string   `json:"label"`
		Kind       string   `json:"kind"`
		Dimensions []string `json:"dimensions"`
	} `json:"unit"`
	Project struct {
		Name          string `json:"name"`
		Description   string `json:"description,omitempty"`
		Scenario      string `json:"scenario"`
		ScenarioLabel string `json:"scenarioLabel"`
	} `json:"project"`
	Brief        writingstate.Brief         `json:"brief"`
	Requirements []writingstate.Requirement `json:"requirements"`
	Norms        struct {
		Available bool          `json:"available"`
		Note      string        `json:"note,omitempty"`
		Entries   []norms.Entry `json:"entries"`
	} `json:"norms"`
	Plan      *unitPlanContext      `json:"plan,omitempty"`
	Chapters  []unitChapterContext  `json:"chapters"`
	Documents []unitDocumentContext `json:"documents"`
	Warnings  []string              `json:"warnings"`
}

func baseUnitContext(pc *projectContext, unitID, label, kind string, dims []string) *unitContextJSON {
	out := &unitContextJSON{}
	out.Unit.ID, out.Unit.Label, out.Unit.Kind, out.Unit.Dimensions = unitID, label, kind, dims
	out.Project.Name = pc.project.Name
	out.Project.Scenario = pc.project.Scenario
	out.Project.ScenarioLabel = store.ScenarioLabel(pc.project.Scenario)
	if pc.project.Description != nil {
		out.Project.Description = *pc.project.Description
	}
	out.Brief = pc.brief
	out.Requirements = pc.requirements
	out.Norms.Available = pc.normsOK
	out.Norms.Note = pc.normsNote
	out.Norms.Entries = pc.norms
	out.Chapters = []unitChapterContext{}
	out.Documents = []unitDocumentContext{}
	if pc.plan != nil && pc.plan.Plan != nil {
		out.Plan = &unitPlanContext{Status: pc.plan.Status, ArtifactID: pc.plan.ArtifactID.String(), Revision: pc.plan.Revision}
	}
	out.Warnings = append(out.Warnings, pc.warnings...)
	for _, doc := range pc.documents {
		out.Documents = append(out.Documents, unitDocumentContext{ID: doc.ID.String(), Name: doc.Name, Type: doc.Type, SizeBytes: doc.Size})
	}
	return out
}

func buildChapterUnitContext(pc *projectContext, unit UnitOption, dimKey string) ([]byte, bool) {
	out := baseUnitContext(pc, unit.ID, unit.Title, "chapter", []string{dimKey})
	truncated := false
	for _, path := range unit.FilePaths {
		content := pc.fileByPath[path]
		chapter := unitChapterContext{FilePath: path, Title: unit.Title, Content: content, SizeBytes: len(content)}
		if len(chapter.Content) > reviewChapterBytes {
			chapter.Content = chapter.Content[:reviewChapterBytes]
			chapter.Truncated = true
			truncated = true
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s 较大，上下文只包含前 %d 字节；需要时用 read_latex_file 读取其余部分", path, reviewChapterBytes))
		}
		out.Chapters = append(out.Chapters, chapter)
	}
	out.Plan = planSectionsFor(out.Plan, pc, unit.SectionIDs)
	if out.Plan == nil || len(out.Plan.Sections) == 0 {
		out.Warnings = append(out.Warnings, "本单元没有对应的章节计划；目标—结论维度只能标记为证据不足")
	}
	encoded, _ := json.Marshal(out)
	return encoded, truncated
}

func buildCollectionUnitContext(pc *projectContext, units []UnitOption, dimKey string) ([]byte, bool, error) {
	out := baseUnitContext(pc, "collection", "跨章一致性", "collection", []string{dimKey})
	total := 0
	truncated := false
	seen := map[string]bool{}
	sectionIDs := make([]string, 0)
	for _, unit := range units {
		sectionIDs = append(sectionIDs, unit.SectionIDs...)
		for _, path := range unit.FilePaths {
			if seen[path] {
				continue
			}
			seen[path] = true
			content := pc.fileByPath[path]
			if total+len(content) > reviewCollectionBytes {
				return nil, false, httpapi.BadRequest("所选章节总长度超出单次跨章检查上限，请减少章节范围")
			}
			chapter := unitChapterContext{FilePath: path, Title: unit.Title, Content: content, SizeBytes: len(content)}
			if len(chapter.Content) > reviewChapterBytes {
				chapter.Content = chapter.Content[:reviewChapterBytes]
				chapter.Truncated = true
				truncated = true
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s 较大，上下文只包含前 %d 字节；需要时用 read_latex_file 读取其余部分", path, reviewChapterBytes))
			}
			total += len(chapter.Content)
			out.Chapters = append(out.Chapters, chapter)
		}
	}
	out.Plan = planSectionsFor(out.Plan, pc, dedupeStrings(sectionIDs))
	encoded, _ := json.Marshal(out)
	return encoded, truncated, nil
}

func planSectionsFor(base *unitPlanContext, pc *projectContext, sectionIDs []string) *unitPlanContext {
	if base == nil || pc.plan == nil || pc.plan.Plan == nil {
		return base
	}
	wanted := map[string]bool{}
	for _, id := range sectionIDs {
		wanted[id] = true
	}
	for _, sec := range pc.plan.Plan.Sections {
		if wanted[sec.ID] {
			base.Sections = append(base.Sections, sec)
		}
	}
	return base
}

func buildUnitInstruction(unit plannedUnit, pc *projectContext) string {
	dim, _ := LookupDimension(unit.Dimensions[0])
	var b strings.Builder
	b.WriteString("你是学术论文审稿助手。本次只检查一个维度，不要报告范围外的问题。\n\n")
	fmt.Fprintf(&b, "项目场景：%s\n", store.ScenarioLabel(pc.project.Scenario))
	fmt.Fprintf(&b, "检查维度：%s（%s）\n", dim.Label, dim.Description)
	if unit.Kind == "chapter" {
		fmt.Fprintf(&b, "检查范围：章节「%s」，文件 %s\n", unit.Label, strings.Join(unit.FilePaths, "、"))
	} else {
		fmt.Fprintf(&b, "检查范围：所选全部章节的跨章一致性，文件 %s\n", strings.Join(unit.FilePaths, "、"))
	}
	b.WriteString("第一步必须调用 get_review_unit_context 读取锁定上下文；需要核对原文时用 read_latex_file/read_document 真实读取。\n")
	b.WriteString("只报告能从已读原文中举证的问题；每条问题的 quote 必须逐字复制原文（保留标点和 LaTeX 命令），服务端会核对并计算行号。\n")
	b.WriteString("资料没读过就不能当依据；材料不足时把维度标为 insufficient 并写清缺什么，不要编造问题。\n")
	b.WriteString("不得输出「全部通过」、分数、通过率或对论文质量的总体保证；只陈述发现与依据。\n\n")
	b.WriteString("最后调用 submit_review_unit_report，report_json 结构：\n")
	b.WriteString(`{"dimensions":[{"key":"` + unit.Dimensions[0] + `","status":"checked|findings|insufficient|not_checked|failed","note":"非 checked/findings 时必填"}],`)
	b.WriteString(`"findings":[{"id":"f1","dimension":"` + unit.Dimensions[0] + `","severity":"high|medium|low","claimStatus":"overclaimed|orphaned（仅 evidence_support 必填）","sectionId":"可选","filePath":"单元内文件","quote":"逐字原文","problem":"问题是什么","basis":[{"kind":"text|plan|norm|source","ref":"...","quote":"逐字依据"}],"nextStep":"用户可以怎么改","confidence":"high|low"}]}`)
	b.WriteString("\n\n")
	switch unit.Dimensions[0] {
	case "evidence_support":
		b.WriteString("evidence_support 的每条问题必须给 claimStatus：overclaimed（过度断言，必须有对照依据）或 orphaned（无据孤立，项目有资料时必须先真实读取资料）。\n")
	case "cross_section":
		b.WriteString("cross_section 的每条问题必须有至少两条 text 依据，且来自不同文件（冲突双方原文）。\n")
	case "norm_compliance":
		if !pc.normsOK || len(pc.norms) == 0 {
			b.WriteString("本项目没有可用的已采用规范，本维度只能标记为 insufficient 或 not_checked，不得给出问题。\n")
		}
	}
	if pc.plan == nil {
		b.WriteString("本项目没有可用的章节计划；goal_conclusion 只能标记为 insufficient，不得给出问题。\n")
	}
	return b.String()
}

func fingerprintUnit(pc *projectContext, model *agent.ResolvedModel, dims, files, sectionIDs []string) string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
	}
	write(ScopeVersion, ContextVersion, agent.ReviewUnitPromptVersion, pc.project.Scenario)
	if model != nil {
		write(model.Name, model.Model)
	}
	write(dims...)
	for _, f := range files {
		sum := sha256.Sum256([]byte(pc.fileByPath[f]))
		write(f, hex.EncodeToString(sum[:]))
	}
	if pc.plan != nil {
		write(fmt.Sprintf("plan:%s:%d", pc.plan.ArtifactID.String(), pc.plan.Revision))
		if pc.plan.Plan != nil {
			for _, sec := range pc.plan.Plan.Sections {
				for _, id := range sectionIDs {
					if sec.ID == id {
						sum := sha256.Sum256([]byte(research.PlanSectionText(sec)))
						write(id, hex.EncodeToString(sum[:]))
					}
				}
			}
		}
	}
	for _, entry := range pc.norms {
		write(entry.ID, fmt.Sprintf("%d", entry.Version))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
