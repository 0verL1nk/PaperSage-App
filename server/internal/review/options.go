package review

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 检查页的范围选择数据：主文件、章节单元、文件清单、可用维度与文本快照。
// 结构由服务端确定性解析；这里不产生任何语义结论。

const (
	// maxSnapshotBytes 是随 options 一次性返回的文本快照上限。
	maxSnapshotBytes = 2 << 20
	// maxSnapshotFiles 是快照文件数上限（与 StructureMaxTexFiles 一致）。
	maxSnapshotFiles = latex.StructureMaxTexFiles
)

// ErrProjectNotFound 表示项目不存在或不属于当前用户。
var ErrProjectNotFound = errors.New("project not found")

// Service 是检查域服务。引擎（review runs）落地前，先提供确定性的项目结构入口。
type Service struct {
	DB    *gorm.DB
	Files *latex.FilesService
}

// NewService 构造检查域服务。
func NewService(db *gorm.DB, files *latex.FilesService) *Service {
	return &Service{DB: db, Files: files}
}

// Limits 告诉前端一次检查的硬上限，超限时由用户缩小范围而不是静默截断。
type Limits struct {
	MaxFiles         int `json:"maxFiles"`
	MaxUnits         int `json:"maxUnits"`
	MaxSnapshotBytes int `json:"maxSnapshotBytes"`
}

// UnitOption 是一个可勾选的章节单元。
type UnitOption struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind"` // chapter
	FilePaths  []string `json:"filePaths"`
	SectionIDs []string `json:"sectionIds,omitempty"`
	Order      int      `json:"order"`
	SizeBytes  int      `json:"sizeBytes"`
	Empty      bool     `json:"empty"`
}

// PlanSectionOption 是章节计划中可对照的一节。
type PlanSectionOption struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"filePath,omitempty"`
}

// PlanOption 描述本次检查可用的章节计划（没有时为 missing，不阻塞检查）。
type PlanOption struct {
	Status     string              `json:"status"` // adopted | candidate | missing
	ArtifactID string              `json:"artifactId,omitempty"`
	Revision   int                 `json:"revision,omitempty"`
	Sections   []PlanSectionOption `json:"sections,omitempty"`
}

// SnapshotFile 是前端秒级结构检查所需的逐字文本。
type SnapshotFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Options 是 GET /projects/:id/review/options 的响应。
type Options struct {
	Scenario          string                `json:"scenario"`
	ScenarioLabel     string                `json:"scenarioLabel"`
	Roots             []latex.StructureRoot `json:"roots"`
	Units             []UnitOption          `json:"units"`
	Files             []latex.StructureFile `json:"files"`
	Dimensions        []Dimension           `json:"dimensions"`
	Plan              PlanOption            `json:"plan"`
	Limits            Limits                `json:"limits"`
	Snapshot          []SnapshotFile        `json:"snapshot"`
	SnapshotTruncated bool                  `json:"snapshotTruncated"`
	Warnings          []string              `json:"warnings"`
}

// Options 读取项目结构、计划与可用维度。只读，不改任何状态。
func (s *Service) Options(ctx context.Context, userID, projectID store.UUID) (*Options, error) {
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
	for _, pair := range pairs {
		if !isStructureFile(pair[0]) {
			continue
		}
		files = append(files, latex.ProjectText{Path: pair[0], Content: pair[1]})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	structure := latex.ParseStructure(files)
	plan, _, planWarnings := research.LoadAdoptedChapterPlan(ctx, s.DB, projectID)
	units, unitWarnings := BuildUnits(structure, plan)
	snapshot, truncated := BuildSnapshot(files)

	out := &Options{
		Scenario:          project.Scenario,
		ScenarioLabel:     store.ScenarioLabel(project.Scenario),
		Roots:             structure.Roots,
		Units:             units,
		Files:             structure.Files,
		Dimensions:        DimensionsFor(project.Scenario),
		Plan:              buildPlanOption(plan),
		Limits:            Limits{MaxFiles: latex.StructureMaxTexFiles, MaxUnits: latex.StructureMaxUnits, MaxSnapshotBytes: maxSnapshotBytes},
		Snapshot:          snapshot,
		SnapshotTruncated: truncated,
		Warnings:          []string{},
	}
	for _, w := range structure.Warnings {
		out.Warnings = append(out.Warnings, w.Detail)
	}
	out.Warnings = append(out.Warnings, planWarnings...)
	out.Warnings = append(out.Warnings, unitWarnings...)
	if truncated {
		out.Warnings = append(out.Warnings, fmt.Sprintf("项目文本超过 %d MB，页面只拿到部分快照；完整结构仍以服务端识别结果为准。", maxSnapshotBytes/(1<<20)))
	}
	return out, nil
}

func buildPlanOption(plan *research.LoadedChapterPlan) PlanOption {
	if plan == nil || plan.Plan == nil {
		return PlanOption{Status: "missing"}
	}
	out := PlanOption{Status: plan.Status, ArtifactID: plan.ArtifactID.String(), Revision: plan.Revision}
	for _, sec := range plan.Plan.Sections {
		item := PlanSectionOption{ID: sec.ID, Title: sec.Title}
		if sec.FilePath != nil {
			item.FilePath = latex.NormalizeTexPath(*sec.FilePath)
		}
		out.Sections = append(out.Sections, item)
	}
	return out
}

// BuildUnits 把结构单元与章节计划合并：计划的 filePath 决定 sectionIds 归属；
// 计划中未出现在主文件包含树里的章节文件，补成独立单元（仍可单独检查）。
func BuildUnits(structure latex.Structure, plan *research.LoadedChapterPlan) ([]UnitOption, []string) {
	units := make([]UnitOption, 0, len(structure.Units))
	warnings := []string{}
	for _, u := range structure.Units {
		units = append(units, UnitOption{
			ID: u.ID, Title: u.Title, Kind: "chapter", FilePaths: u.FilePaths,
			Order: u.Order, SizeBytes: u.SizeBytes, Empty: u.Empty,
		})
	}
	if plan == nil || plan.Plan == nil {
		return units, warnings
	}

	sectionsByFile := map[string][]PlanSectionOption{}
	covered := map[string]bool{}
	for _, sec := range plan.Plan.Sections {
		if sec.FilePath == nil || strings.TrimSpace(*sec.FilePath) == "" {
			warnings = append(warnings, fmt.Sprintf("章节计划中的「%s」没有绑定稿件文件，无法按章节检查。", sec.Title))
			continue
		}
		file := latex.NormalizeTexPath(*sec.FilePath)
		item := PlanSectionOption{ID: sec.ID, Title: sec.Title, FilePath: file}
		sectionsByFile[file] = append(sectionsByFile[file], item)
	}
	fileExists := map[string]bool{}
	for _, f := range structure.Files {
		if f.Kind == "tex" {
			fileExists[f.Path] = true
		}
	}
	for i := range units {
		for _, file := range units[i].FilePaths {
			sections := sectionsByFile[file]
			for _, sec := range sections {
				units[i].SectionIDs = append(units[i].SectionIDs, sec.ID)
				covered[sec.ID] = true
			}
			if len(sections) == 1 && filenameFallback(units[i].Title, file) {
				units[i].Title = sections[0].Title
			}
		}
	}

	// 计划里有文件、但主文件没有包含它：补成独立单元，避免「计划里有却检查不到」。
	for _, sec := range plan.Plan.Sections {
		if sec.FilePath == nil || strings.TrimSpace(*sec.FilePath) == "" || covered[sec.ID] {
			continue
		}
		file := latex.NormalizeTexPath(*sec.FilePath)
		if !fileExists[file] {
			warnings = append(warnings, fmt.Sprintf("章节计划中的「%s」指向的文件 %s 不存在。", sec.Title, file))
			continue
		}
		id := fmt.Sprintf("p%d", len(units)+1)
		units = append(units, UnitOption{
			ID: id, Title: sec.Title, Kind: "chapter", FilePaths: []string{file},
			SectionIDs: []string{sec.ID}, Order: len(units) + 1,
			SizeBytes: structureFileSize(structure, file),
		})
		covered[sec.ID] = true
	}
	if len(units) > latex.StructureMaxUnits {
		warnings = append(warnings, fmt.Sprintf("可检查的章节单元有 %d 个，超过单次上限 %d 个；请缩小检查范围。", len(units), latex.StructureMaxUnits))
	}
	return units, warnings
}

// filenameFallback 判断标题是否是「没有章节命令时退回的文件名」。
func filenameFallback(title, file string) bool {
	base := path.Base(file)
	return title == strings.TrimSuffix(base, path.Ext(base))
}

func structureFileSize(structure latex.Structure, file string) int {
	for _, f := range structure.Files {
		if f.Path == file {
			return f.SizeBytes
		}
	}
	return 0
}

// BuildSnapshot 生成前端秒级检查所需的文本快照（.tex/.bib），超限显式标记。
func BuildSnapshot(files []latex.ProjectText) ([]SnapshotFile, bool) {
	out := make([]SnapshotFile, 0, len(files))
	total := 0
	for _, f := range files {
		if !isStructureFile(f.Path) {
			continue
		}
		if len(out) >= maxSnapshotFiles || total+len(f.Content) > maxSnapshotBytes {
			return out, true
		}
		out = append(out, SnapshotFile{Path: latex.NormalizeTexPath(f.Path), Content: f.Content})
		total += len(f.Content)
	}
	return out, false
}

func isStructureFile(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".tex", ".bib":
		return true
	}
	return false
}
