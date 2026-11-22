package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

// 跨章节检查的上下文与落库前核对。
//
// 分工是刻意划死的：语义判断（目标—结论是否一致、论断有没有证据、章节间是否
// 重复/矛盾、是否符合已采用规范）全部由模型完成；这里只做「把真实材料交给
// 模型」和「拒绝模型编造的引文/未读过的依据」两类确定性工作，不产生任何
// 语义结论，也不冒充 AI。

const (
	reviewChapterBytes    = 40 * 1024
	reviewContextBytes    = 160 * 1024
	reviewContextSrcLimit = 200
)

type reviewReads struct {
	mu      sync.Mutex
	state   *reviewContextState
	sources []research.ReviewSource
	seen    map[string]bool
}

func newReviewReads() *reviewReads {
	return &reviewReads{seen: map[string]bool{}}
}

func (r *reviewReads) record(kind, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := kind + "\x00" + name
	if r.seen[key] || len(r.sources) >= reviewContextSrcLimit {
		return
	}
	r.seen[key] = true
	r.sources = append(r.sources, research.ReviewSource{Kind: kind, Name: name})
}

func (r *reviewReads) has(kind, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[kind+"\x00"+name]
}

func (r *reviewReads) snapshot() []research.ReviewSource {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]research.ReviewSource, len(r.sources))
	copy(out, r.sources)
	return out
}

func (r *reviewReads) setContext(state *reviewContextState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
}

func (r *reviewReads) contextState() *reviewContextState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

type reviewContextState struct {
	planArtifactID *store.UUID
	planRevision   *int
	planStatus     string
	normsAvailable bool
	normsNote      string
	norms          []norms.Entry
	plan           *research.ChapterPlan
	files          map[string]string
}

type reviewContextOutput struct {
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
	Plan *reviewPlanContext `json:"plan"`
	// Chapters 是计划里已映射到稿件文件的章节正文；模型无需再逐个读文件。
	Chapters        []reviewChapter  `json:"chapters"`
	OtherLatexFiles []reviewFile     `json:"otherLatexFiles"`
	Documents       []reviewDocument `json:"documents"`
	Warnings        []string         `json:"warnings"`
}

type reviewPlanContext struct {
	Status     string                    `json:"status"`
	ArtifactID string                    `json:"artifactId"`
	Revision   int                       `json:"revision"`
	Sections   []research.ChapterSection `json:"sections"`
}

type reviewChapter struct {
	SectionID    string `json:"sectionId"`
	SectionTitle string `json:"sectionTitle"`
	FilePath     string `json:"filePath"`
	Content      string `json:"content"`
	Truncated    bool   `json:"truncated,omitempty"`
	SizeBytes    int    `json:"sizeBytes"`
}

type reviewFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Mapped    bool   `json:"mapped"`
}

type reviewDocument struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"sizeBytes"`
}

// chapterReviewContextTool 装配三源上下文：已采用规范（HELE-25 接口）+
// 项目蓝图/要求 + 章节计划与各章正文/资料清单。
func chapterReviewContextTool(db *gorm.DB, stg storage.Storage, provider norms.Provider,
	projectID, ownerID store.UUID, verify func(context.Context) error, reads *reviewReads) (tool.BaseTool, error) {

	return utils.InferTool(
		"get_chapter_review_context",
		"装配跨章节检查所需的三源上下文：项目蓝图与要求、章节计划、已采用学术规范（未接入时会明确标注不可用）、各章正文与资料清单。运行跨章节检查前必须先调用本工具；报告只能基于这里返回的上下文与随后真实读取的资料，不得编造。",
		func(ctx context.Context, _ struct{}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			out, state, sources, err := buildChapterReviewContext(ctx, db, stg, provider, projectID, ownerID)
			if err != nil {
				return "", err
			}
			reads.setContext(state)
			for _, src := range sources {
				reads.record(src.Kind, src.Name)
			}
			encoded, err := json.Marshal(out)
			return string(encoded), err
		})
}

func buildChapterReviewContext(ctx context.Context, db *gorm.DB, stg storage.Storage,
	provider norms.Provider, projectID, ownerID store.UUID) (*reviewContextOutput, *reviewContextState, []research.ReviewSource, error) {

	var project store.Project
	if err := db.WithContext(ctx).Where("id = ? AND owner_id = ?", projectID, ownerID).First(&project).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("project not accessible")
	}
	state, err := writingstate.NewService(db).GetForAgent(ctx, projectID.UUID, ownerID.UUID)
	if err != nil {
		return nil, nil, nil, err
	}

	out := &reviewContextOutput{Brief: state.Brief, Requirements: state.Requirements}
	out.Project.Name = project.Name
	out.Project.Scenario = project.Scenario
	out.Project.ScenarioLabel = scenarioLabel(project.Scenario)
	if project.Description != nil {
		out.Project.Description = *project.Description
	}

	reviewState := &reviewContextState{normsNote: "未读取学术规范", files: map[string]string{}}
	if provider == nil {
		reviewState.normsNote = "学术规范库尚未接入，规范符合度本次只能标记为未检查"
		out.Warnings = append(out.Warnings, reviewState.normsNote)
	} else {
		entries, normsErr := provider.AdoptedForProject(ctx, projectID)
		switch {
		case errors.Is(normsErr, norms.ErrUnavailable):
			reviewState.normsNote = "学术规范库尚未接入，规范符合度本次只能标记为未检查"
			out.Warnings = append(out.Warnings, reviewState.normsNote)
		case normsErr != nil:
			slog.Warn("read adopted norms failed", "project_id", projectID.String(), "err", normsErr)
			reviewState.normsNote = "读取已采用学术规范失败，规范符合度本次只能标记为未检查"
			out.Warnings = append(out.Warnings, reviewState.normsNote)
		default:
			reviewState.normsAvailable = true
			reviewState.norms = entries
			if len(entries) == 0 {
				out.Warnings = append(out.Warnings, "项目尚未采用任何学术规范，规范符合度本次只能标记为证据不足或未检查")
			}
		}
	}
	out.Norms.Available = reviewState.normsAvailable
	out.Norms.Note = reviewState.normsNote
	out.Norms.Entries = reviewState.norms

	sources := []research.ReviewSource{}
	plan, planSources, planWarnings := loadAdoptedReviewPlan(ctx, db, projectID)
	if plan == nil {
		reviewState.planStatus = "missing"
		out.Warnings = append(out.Warnings, "项目还没有可用的章节计划，目标—结论与章节间对照只能标记为证据不足")
	} else {
		reviewState.plan = plan.plan
		reviewState.planStatus = plan.status
		artifactID := plan.artifactID
		revision := plan.revision
		reviewState.planArtifactID = &artifactID
		reviewState.planRevision = &revision
		out.Plan = &reviewPlanContext{
			Status: plan.status, ArtifactID: artifactID.String(), Revision: revision, Sections: plan.plan.Sections,
		}
		sources = append(sources, planSources...)
		if plan.status == "candidate" {
			out.Warnings = append(out.Warnings, "章节计划尚未采用，本次以当前候选修订为参考")
		}
	}
	out.Warnings = append(out.Warnings, planWarnings...)

	allFiles := []store.LatexFile{}
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Order("file_path").Find(&allFiles).Error; err != nil {
		return nil, nil, nil, err
	}
	mappedFiles := map[string]bool{}
	total := 0
	if plan != nil {
		for _, sec := range plan.plan.Sections {
			if sec.FilePath == nil || strings.TrimSpace(*sec.FilePath) == "" {
				continue
			}
			path := *sec.FilePath
			mappedFiles[path] = true
			_, content, readErr := findLatex(ctx, db, stg, projectID, path)
			if readErr != nil {
				out.Warnings = append(out.Warnings, fmt.Sprintf("章节「%s」对应的文件 %s 读取失败：%v", sec.Title, path, readErr))
				continue
			}
			reviewState.files[path] = content
			chapter := reviewChapter{SectionID: sec.ID, SectionTitle: sec.Title, FilePath: path, Content: content, SizeBytes: len(content)}
			if len(chapter.Content) > reviewChapterBytes {
				chapter.Content = chapter.Content[:reviewChapterBytes]
				chapter.Truncated = true
				out.Warnings = append(out.Warnings, fmt.Sprintf("章节文件 %s 较大，上下文只包含前 %d 字节；需要时用 read_latex_file 读取其余部分", path, reviewChapterBytes))
			}
			if total+len(chapter.Content) > reviewContextBytes {
				chapter.Content = ""
				chapter.Truncated = true
				out.Warnings = append(out.Warnings, fmt.Sprintf("上下文总量达到上限，章节 %s 未内联；请用 read_latex_file 读取", path))
				continue
			}
			total += len(chapter.Content)
			out.Chapters = append(out.Chapters, chapter)
			sources = append(sources, research.ReviewSource{Kind: "latex", Name: path})
		}
	}
	for _, file := range allFiles {
		out.OtherLatexFiles = append(out.OtherLatexFiles, reviewFile{
			Path: file.FilePath, SizeBytes: file.SizeBytes, Mapped: mappedFiles[file.FilePath],
		})
	}

	var docs []store.Document
	if err := db.WithContext(ctx).Where("project_id = ? AND status = ?", projectID, "uploaded").Order("created_at").Find(&docs).Error; err != nil {
		return nil, nil, nil, err
	}
	for _, doc := range docs {
		out.Documents = append(out.Documents, reviewDocument{ID: doc.ID.String(), Name: doc.Name, Type: doc.Type, SizeBytes: doc.Size})
	}
	if len(docs) == 0 {
		out.Warnings = append(out.Warnings, "项目还没有已入库的参考文献；涉及外部证据的论断只能标记为证据不足")
	}
	return out, reviewState, sources, nil
}

type loadedReviewPlan struct {
	artifactID store.UUID
	revision   int
	status     string
	plan       *research.ChapterPlan
}

func loadAdoptedReviewPlan(ctx context.Context, db *gorm.DB, projectID store.UUID) (*loadedReviewPlan, []research.ReviewSource, []string) {
	var artifacts []store.ResearchArtifact
	if err := db.WithContext(ctx).Where("project_id = ? AND kind = ?", projectID, store.ResearchArtifactChapterPlan).
		Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, nil, []string{"读取章节计划失败：" + err.Error()}
	}
	var chosen *store.ResearchArtifact
	status := ""
	for i := range artifacts {
		if artifacts[i].AdoptedRevision != nil {
			chosen, status = &artifacts[i], "adopted"
			break
		}
	}
	if chosen == nil && len(artifacts) > 0 {
		chosen, status = &artifacts[0], "candidate"
	}
	if chosen == nil {
		return nil, nil, nil
	}
	revision := chosen.CurrentRevision
	if chosen.AdoptedRevision != nil {
		revision = *chosen.AdoptedRevision
	}
	if revision <= 0 {
		return nil, nil, []string{"章节计划没有可用修订"}
	}
	var rev store.ResearchArtifactRevision
	if err := db.WithContext(ctx).Where("artifact_id = ? AND revision = ?", chosen.ID, revision).First(&rev).Error; err != nil {
		return nil, nil, []string{"读取章节计划修订失败：" + err.Error()}
	}
	var plan research.ChapterPlan
	if err := json.Unmarshal(rev.PayloadJSON, &plan); err != nil || len(plan.Sections) == 0 {
		return nil, nil, []string{"章节计划内容不可用"}
	}
	name := fmt.Sprintf("章节计划 v%d（%s）", revision, map[string]string{"adopted": "已采用", "candidate": "候选未采用"}[status])
	return &loadedReviewPlan{artifactID: chosen.ID, revision: revision, status: status, plan: &plan},
		[]research.ReviewSource{{Kind: "plan", Name: name}}, nil
}

// verifyChapterReviewPayload 在报告落库前做确定性核对：
//   - 报告必须基于本次上下文（否则模型可能在凭空写）；
//   - 每条引文必须能在对应文件中逐字定位，行号由服务端计算；
//   - 章节/规范/资料依据必须真实存在且确实被读取过；
//   - sourcesRead 用实际读取记录覆盖，模型自报的来源不采信。
func verifyChapterReviewPayload(ctx context.Context, db *gorm.DB, stg storage.Storage,
	projectID store.UUID, payload []byte, reads *reviewReads) ([]byte, error) {

	var report research.ChapterReview
	if err := json.Unmarshal(payload, &report); err != nil {
		return nil, fmt.Errorf("检查报告结构不合法")
	}
	state := reads.contextState()
	if state == nil {
		return nil, fmt.Errorf("请先调用 get_chapter_review_context 读取章节计划与各章正文，再提交检查报告")
	}

	report.Input.PlanArtifactID = state.planArtifactID
	report.Input.PlanRevision = state.planRevision
	report.Input.PlanStatus = state.planStatus
	report.Input.NormsAvailable = state.normsAvailable
	report.Input.NormsNote = state.normsNote
	report.Input.Norms = state.norms
	if state.planStatus == "missing" {
		report.Input.PlanArtifactID = nil
		report.Input.PlanRevision = nil
	}

	sectionByID := map[string]research.ChapterSection{}
	if state.plan != nil {
		for _, sec := range state.plan.Sections {
			sectionByID[sec.ID] = sec
		}
	}
	fileContent := func(path string) (string, error) {
		if content, ok := state.files[path]; ok {
			return content, nil
		}
		_, content, err := findLatex(ctx, db, stg, projectID, path)
		return content, err
	}

	for i := range report.Findings {
		f := &report.Findings[i]
		if f.SectionID != "" {
			sec, ok := sectionByID[f.SectionID]
			if !ok {
				return nil, fmt.Errorf("检查问题 %s 指向的章节 %s 不在本次章节计划中", f.ID, f.SectionID)
			}
			if strings.TrimSpace(f.SectionTitle) == "" {
				f.SectionTitle = sec.Title
			}
			if f.FilePath == "" && sec.FilePath != nil {
				f.FilePath = *sec.FilePath
			}
		}
		content := ""
		if f.FilePath != "" {
			var err error
			content, err = fileContent(f.FilePath)
			if err != nil {
				return nil, fmt.Errorf("检查问题 %s 定位的文件 %s 不存在或无法读取", f.ID, f.FilePath)
			}
			if strings.TrimSpace(f.Quote) != "" {
				startLine, endLine, ok := locateQuote(content, f.Quote)
				if !ok {
					return nil, fmt.Errorf("检查问题 %s 的引文在 %s 中找不到。请先读取文件并逐字引用原文，不要改写或拼接", f.ID, f.FilePath)
				}
				f.LineStart, f.LineEnd = startLine, endLine
			} else if strings.TrimSpace(content) != "" && f.Dimension != "norm_compliance" {
				return nil, fmt.Errorf("检查问题 %s 必须给出 %s 中的原文引文，便于用户核对", f.ID, f.FilePath)
			}
		}
		for j := range f.Basis {
			b := &f.Basis[j]
			switch b.Kind {
			case "text":
				path := b.FilePath
				if path == "" {
					path = b.Ref
				}
				b.FilePath = path
				text, err := fileContent(path)
				if err != nil {
					return nil, fmt.Errorf("检查问题 %s 的文字依据 %s 不存在或无法读取", f.ID, path)
				}
				startLine, endLine, ok := locateQuote(text, b.Quote)
				if !ok {
					return nil, fmt.Errorf("检查问题 %s 的文字依据在 %s 中找不到；依据必须逐字来自原文", f.ID, path)
				}
				b.LineStart, b.LineEnd = startLine, endLine
			case "plan":
				sec, ok := sectionByID[b.Ref]
				if !ok {
					return nil, fmt.Errorf("检查问题 %s 的计划依据指向不存在的章节 %s", f.ID, b.Ref)
				}
				if _, _, ok := locateQuote(planSectionText(sec), b.Quote); !ok {
					return nil, fmt.Errorf("检查问题 %s 的计划依据在章节计划中找不到；依据必须逐字来自计划", f.ID)
				}
			case "norm":
				found := false
				for _, entry := range state.norms {
					if entry.ID == b.Ref || entry.Title == b.Ref {
						found = true
						b.Ref = entry.ID
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("检查问题 %s 的规范依据不在本次已采用规范中", f.ID)
				}
			case "source":
				if err := verifyReviewSourceBasis(ctx, db, projectID, b, reads); err != nil {
					return nil, fmt.Errorf("检查问题 %s 的资料依据不可用：%v", f.ID, err)
				}
			}
		}
		f.AnchorVerified = true
	}
	report.SourcesRead = reads.snapshot()
	return json.Marshal(report)
}

func verifyReviewSourceBasis(ctx context.Context, db *gorm.DB, projectID store.UUID, b *research.ReviewBasis, reads *reviewReads) error {
	var doc store.Document
	err := db.WithContext(ctx).Where("project_id = ? AND (id = ? OR name = ?)", projectID, b.Ref, b.Ref).First(&doc).Error
	if err == nil {
		if !reads.has("document", doc.Name) {
			return fmt.Errorf("资料 %s 尚未在本轮检查中读取", doc.Name)
		}
		b.Ref = doc.Name
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var version store.SourceVersion
	err = db.WithContext(ctx).Model(&store.SourceVersion{}).
		Joins("JOIN project_sources ON project_sources.id = source_versions.source_id").
		Where("source_versions.id = ? AND project_sources.project_id = ?", b.Ref, projectID).
		First(&version).Error
	if err != nil {
		return fmt.Errorf("引用的资料不属于该项目")
	}
	if !reads.has("source", version.ID.String()) {
		return fmt.Errorf("来源版本 %s 尚未在本轮检查中读取", b.Ref)
	}
	return nil
}

// locateQuote 在原文中定位引文（空白归一化），返回 1 起始的行号区间。
func locateQuote(content, quote string) (int, int, bool) {
	fields := strings.Fields(quote)
	if len(fields) == 0 {
		return 0, 0, false
	}
	parts := make([]string, len(fields))
	for i, field := range fields {
		parts[i] = regexp.QuoteMeta(field)
	}
	re, err := regexp.Compile(strings.Join(parts, `\s+`))
	if err != nil {
		return 0, 0, false
	}
	loc := re.FindStringIndex(content)
	if loc == nil {
		return 0, 0, false
	}
	startLine := 1 + strings.Count(content[:loc[0]], "\n")
	endLine := startLine + strings.Count(content[loc[0]:loc[1]], "\n")
	return startLine, endLine, true
}

func planSectionText(sec research.ChapterSection) string {
	parts := []string{sec.Title, sec.Responsibility}
	parts = append(parts, sec.Questions...)
	for _, claim := range sec.Claims {
		parts = append(parts, claim.Text)
		for _, evidence := range claim.Evidence {
			parts = append(parts, evidence.Quote)
		}
	}
	return strings.Join(parts, "\n")
}
