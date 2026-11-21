// Package research 科研组织产物域：阅读记录、文献比较、章节计划与章节摘要。
// 契约来源：openspec/changes/research-organization-artifacts。
package research

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// ── 合同：四种 payload ──────────────────────────────────────────────

var fieldStatuses = map[string]bool{
	"reported": true, "not_read": true, "not_reported": true, "insufficient": true,
}

type EvidenceRef struct {
	SourceVersionID *store.UUID `json:"sourceVersionId,omitempty"`
	DocumentID      *store.UUID `json:"documentId,omitempty"`
	StartByte       *int64      `json:"startByte,omitempty"`
	EndByte         *int64      `json:"endByte,omitempty"`
	Quote           string      `json:"quote,omitempty"`
}

type Field struct {
	Text     string        `json:"text"`
	Status   string        `json:"status"`
	Scope    string        `json:"scope,omitempty"`
	Evidence []EvidenceRef `json:"evidence,omitempty"`
}

type LiteratureNote struct {
	Source      EvidenceRef `json:"source"`
	Title       string      `json:"title,omitempty"`
	Question    Field       `json:"question"`
	Method      Field       `json:"method"`
	Objects     Field       `json:"objects"`
	Conditions  Field       `json:"conditions"`
	Results     Field       `json:"results"`
	Limitations Field       `json:"limitations"`
}

type ComparisonGroup struct {
	SourceID store.UUID `json:"sourceId"`
	Label    string     `json:"label,omitempty"`
	Values   []Field    `json:"values"`
}

type LiteratureComparison struct {
	Question    string            `json:"question"`
	Axes        []string          `json:"axes"`
	Groups      []ComparisonGroup `json:"groups"`
	Commonality []Field           `json:"commonalities,omitempty"`
	Differences []Field           `json:"differences,omitempty"`
	Conflicts   []Field           `json:"conflicts,omitempty"`
	Missing     []Field           `json:"missing,omitempty"`
	Conditions  string            `json:"conditions,omitempty"`
}

type SectionClaim struct {
	Text     string        `json:"text"`
	Status   string        `json:"status"`
	Evidence []EvidenceRef `json:"evidence,omitempty"`
}

type ChapterSection struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Responsibility string         `json:"responsibility"`
	Questions      []string       `json:"questions,omitempty"`
	Claims         []SectionClaim `json:"claims,omitempty"`
	Dependencies   []string       `json:"dependencies,omitempty"`
	Order          int            `json:"order"`
	FilePath       *string        `json:"filePath,omitempty"`
	StartByte      *int64         `json:"startByte,omitempty"`
	EndByte        *int64         `json:"endByte,omitempty"`
}

type ChapterPlan struct {
	Sections []ChapterSection `json:"sections"`
}

type DigestInput struct {
	LatexFileID store.UUID `json:"latexFileId"`
	Revision    *string    `json:"revision,omitempty"`
}

type ChapterDigest struct {
	Input     DigestInput `json:"input"`
	SectionID string      `json:"sectionId"`
	Content   string      `json:"content"`
	Terms     []string    `json:"terms,omitempty"`
	Coverage  []string    `json:"coverage"`
	Status    string      `json:"status"`
}

// ── 请求/响应 ───────────────────────────────────────────────────────

type dependencyInput struct {
	Kind string     `json:"kind"`
	ID   store.UUID `json:"id"`
}

type createRequest struct {
	Kind            string            `json:"kind"`
	Payload         json.RawMessage   `json:"payload"`
	Origin          string            `json:"origin"`
	RunID           *store.UUID       `json:"runId"`
	ClientRequestID *string           `json:"clientRequestId"`
	Dependencies    []dependencyInput `json:"dependencies"`
}

type revisionRequest struct {
	ExpectedRevision int               `json:"expectedRevision"`
	Payload          json.RawMessage   `json:"payload"`
	Origin           string            `json:"origin"`
	RunID            *store.UUID       `json:"runId"`
	Dependencies     []dependencyInput `json:"dependencies"`
}

type decisionRequest struct {
	Revision         int `json:"revision"`
	ExpectedRevision int `json:"expectedRevision"`
}

type revisionResponse struct {
	ID        uuid.UUID       `json:"id"`
	Revision  int             `json:"revision"`
	Status    string          `json:"status"`
	Origin    string          `json:"origin"`
	Payload   json.RawMessage `json:"payload"`
	RunID     *uuid.UUID      `json:"runId"`
	CreatedAt dto.JSONTime    `json:"createdAt"`
}

type artifactResponse struct {
	ID              uuid.UUID          `json:"id"`
	ProjectID       uuid.UUID          `json:"projectId"`
	Kind            string             `json:"kind"`
	State           string             `json:"state"`
	CurrentRevision int                `json:"currentRevision"`
	AdoptedRevision *int               `json:"adoptedRevision"`
	CreatedAt       dto.JSONTime       `json:"createdAt"`
	UpdatedAt       dto.JSONTime       `json:"updatedAt"`
	Current         *revisionResponse  `json:"current,omitempty"`
	Adopted         *revisionResponse  `json:"adopted,omitempty"`
	Revisions       []revisionResponse `json:"revisions,omitempty"`
}

// ── 写入路径 ────────────────────────────────────────────────────────

func validKind(kind string) bool {
	switch kind {
	case store.ResearchArtifactLiteratureNote, store.ResearchArtifactLiteratureComparison,
		store.ResearchArtifactChapterPlan, store.ResearchArtifactChapterDigest:
		return true
	}
	return false
}

func (s *Service) ownsProject(ctx context.Context, uid, projectID store.UUID) error {
	var count int64
	if err := s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", projectID, uid).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return httpapi.NotFound("项目")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, uid, projectID store.UUID, req createRequest) (*artifactResponse, error) {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return nil, err
	}
	if !validKind(req.Kind) {
		return nil, httpapi.BadRequest("不支持的产物类型")
	}
	if req.ClientRequestID != nil {
		var existing store.ResearchArtifact
		err := s.DB.WithContext(ctx).Where("project_id = ? AND client_request_id = ?", projectID, *req.ClientRequestID).
			First(&existing).Error
		if err == nil {
			return s.loadArtifact(ctx, existing.ID)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	payload, deps, err := s.validatePayload(ctx, projectID, req.Kind, req.Payload, req.Dependencies)
	if err != nil {
		return nil, err
	}
	origin := req.Origin
	if origin != "agent" {
		origin = "user"
	}
	var artifact store.ResearchArtifact
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		artifact = store.ResearchArtifact{
			ProjectID: projectID, Kind: req.Kind, State: "active",
			CurrentRevision: 1, ClientRequestID: req.ClientRequestID, CreatedBy: uid,
		}
		if err := tx.Create(&artifact).Error; err != nil {
			return err
		}
		rev := store.ResearchArtifactRevision{
			ArtifactID: artifact.ID, Revision: 1, PayloadJSON: payload,
			Origin: origin, Status: store.ResearchArtifactRevisionCandidate,
			RunID: req.RunID, CreatedBy: uid,
		}
		if err := tx.Create(&rev).Error; err != nil {
			return err
		}
		return createDependencies(tx, projectID, artifact.ID, 1, deps)
	})
	if err != nil {
		return nil, err
	}
	return s.loadArtifact(ctx, artifact.ID)
}

func (s *Service) AddRevision(ctx context.Context, uid, projectID, artifactID store.UUID, req revisionRequest) (*artifactResponse, error) {
	artifact, err := s.loadOwnedArtifact(ctx, uid, projectID, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact.CurrentRevision != req.ExpectedRevision {
		return nil, conflict("产物已在其他位置更新，请基于最新版本重试")
	}
	payload, deps, err := s.validatePayload(ctx, projectID, artifact.Kind, req.Payload, req.Dependencies)
	if err != nil {
		return nil, err
	}
	origin := req.Origin
	if origin != "agent" {
		origin = "user"
	}
	next := artifact.CurrentRevision + 1
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&store.ResearchArtifact{}).
			Where("id = ? AND current_revision = ?", artifact.ID, artifact.CurrentRevision).
			Update("current_revision", next)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return conflict("产物已在其他位置更新，请基于最新版本重试")
		}
		rev := store.ResearchArtifactRevision{
			ArtifactID: artifact.ID, Revision: next, PayloadJSON: payload,
			Origin: origin, Status: store.ResearchArtifactRevisionCandidate,
			RunID: req.RunID, CreatedBy: uid,
		}
		if err := tx.Create(&rev).Error; err != nil {
			return err
		}
		return createDependencies(tx, projectID, artifact.ID, next, deps)
	})
	if err != nil {
		return nil, err
	}
	return s.loadArtifact(ctx, artifact.ID)
}

func (s *Service) Adopt(ctx context.Context, uid, projectID, artifactID store.UUID, req decisionRequest) (*artifactResponse, error) {
	artifact, err := s.loadOwnedArtifact(ctx, uid, projectID, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact.CurrentRevision != req.ExpectedRevision {
		return nil, conflict("产物已在其他位置更新，请基于最新版本重试")
	}
	if req.Revision <= 0 || req.Revision > artifact.CurrentRevision {
		return nil, httpapi.BadRequest("修订号不存在")
	}
	var rev store.ResearchArtifactRevision
	if err := s.DB.WithContext(ctx).Where("artifact_id = ? AND revision = ?", artifact.ID, req.Revision).First(&rev).Error; err != nil {
		return nil, httpapi.NotFound("版本")
	}
	if rev.Status == store.ResearchArtifactRevisionRejected {
		return nil, httpapi.BadRequest("已拒绝的修订不能采用")
	}
	if rev.Status == store.ResearchArtifactRevisionAdopted {
		return s.loadArtifact(ctx, artifact.ID)
	}
	if err := s.validateDependencies(ctx, projectID, artifact.ID, req.Revision); err != nil {
		return nil, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&store.ResearchArtifact{}).
			Where("id = ? AND current_revision = ?", artifact.ID, req.ExpectedRevision).
			Update("adopted_revision", req.Revision)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return conflict("产物已在其他位置更新，请基于最新版本重试")
		}
		if artifact.AdoptedRevision != nil && *artifact.AdoptedRevision != req.Revision {
			if err := tx.Model(&store.ResearchArtifactRevision{}).
				Where("artifact_id = ? AND revision = ?", artifact.ID, *artifact.AdoptedRevision).
				Update("status", store.ResearchArtifactRevisionSuperseded).Error; err != nil {
				return err
			}
		}
		return tx.Model(&store.ResearchArtifactRevision{}).
			Where("artifact_id = ? AND revision = ?", artifact.ID, req.Revision).
			Update("status", store.ResearchArtifactRevisionAdopted).Error
	})
	if err != nil {
		return nil, err
	}
	return s.loadArtifact(ctx, artifact.ID)
}

func (s *Service) Reject(ctx context.Context, uid, projectID, artifactID store.UUID, req decisionRequest) (*artifactResponse, error) {
	artifact, err := s.loadOwnedArtifact(ctx, uid, projectID, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact.CurrentRevision != req.ExpectedRevision {
		return nil, conflict("产物已在其他位置更新，请基于最新版本重试")
	}
	if req.Revision <= 0 || req.Revision > artifact.CurrentRevision {
		return nil, httpapi.BadRequest("修订号不存在")
	}
	if artifact.AdoptedRevision != nil && *artifact.AdoptedRevision == req.Revision {
		return nil, httpapi.BadRequest("已采用的修订不能拒绝")
	}
	err = s.DB.WithContext(ctx).Model(&store.ResearchArtifactRevision{}).
		Where("artifact_id = ? AND revision = ? AND status = ?", artifact.ID, req.Revision, store.ResearchArtifactRevisionCandidate).
		Update("status", store.ResearchArtifactRevisionRejected).Error
	if err != nil {
		return nil, err
	}
	return s.loadArtifact(ctx, artifact.ID)
}

func (s *Service) Delete(ctx context.Context, uid, projectID, artifactID store.UUID) error {
	artifact, err := s.loadOwnedArtifact(ctx, uid, projectID, artifactID)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Delete(&store.ResearchArtifact{SoftDeleteModel: store.SoftDeleteModel{ID: artifact.ID}}).Error
}

// ── 读取路径 ────────────────────────────────────────────────────────

func (s *Service) List(ctx context.Context, uid, projectID store.UUID, kind string) ([]artifactResponse, error) {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return nil, err
	}
	q := s.DB.WithContext(ctx).Model(&store.ResearchArtifact{}).Where("project_id = ?", projectID)
	if kind != "" {
		if !validKind(kind) {
			return nil, httpapi.BadRequest("不支持的产物类型")
		}
		q = q.Where("kind = ?", kind)
	}
	var artifacts []store.ResearchArtifact
	if err := q.Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, err
	}
	out := make([]artifactResponse, 0, len(artifacts))
	for i := range artifacts {
		item, err := s.loadArtifact(ctx, artifacts[i].ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, uid, projectID, artifactID store.UUID) (*artifactResponse, error) {
	artifact, err := s.loadOwnedArtifact(ctx, uid, projectID, artifactID)
	if err != nil {
		return nil, err
	}
	return s.loadArtifact(ctx, artifact.ID)
}

func (s *Service) GetRevision(ctx context.Context, uid, projectID, artifactID store.UUID, revision int) (*revisionResponse, error) {
	artifact, err := s.loadOwnedArtifact(ctx, uid, projectID, artifactID)
	if err != nil {
		return nil, err
	}
	var rev store.ResearchArtifactRevision
	if err := s.DB.WithContext(ctx).Where("artifact_id = ? AND revision = ?", artifact.ID, revision).First(&rev).Error; err != nil {
		return nil, httpapi.NotFound("版本")
	}
	r := toRevisionResponse(&rev)
	return &r, nil
}

func (s *Service) loadOwnedArtifact(ctx context.Context, uid, projectID, artifactID store.UUID) (*store.ResearchArtifact, error) {
	var artifact store.ResearchArtifact
	err := s.DB.WithContext(ctx).
		Joins("JOIN projects ON projects.id = research_artifacts.project_id").
		Where("research_artifacts.id = ? AND research_artifacts.project_id = ? AND projects.owner_id = ?",
			artifactID, projectID, uid).
		First(&artifact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpapi.NotFound("产物")
	}
	return &artifact, err
}

func (s *Service) loadArtifact(ctx context.Context, artifactID store.UUID) (*artifactResponse, error) {
	var artifact store.ResearchArtifact
	if err := s.DB.WithContext(ctx).First(&artifact, "id = ?", artifactID).Error; err != nil {
		return nil, err
	}
	var revisions []store.ResearchArtifactRevision
	if err := s.DB.WithContext(ctx).Where("artifact_id = ?", artifact.ID).
		Order("revision").Find(&revisions).Error; err != nil {
		return nil, err
	}
	resp := artifactResponse{
		ID: artifact.ID.UUID, ProjectID: artifact.ProjectID.UUID, Kind: artifact.Kind,
		State: artifact.State, CurrentRevision: artifact.CurrentRevision,
		AdoptedRevision: artifact.AdoptedRevision,
		CreatedAt:       dto.JSONTime{Time: artifact.CreatedAt},
		UpdatedAt:       dto.JSONTime{Time: artifact.UpdatedAt},
	}
	for i := range revisions {
		rr := toRevisionResponse(&revisions[i])
		resp.Revisions = append(resp.Revisions, rr)
		if revisions[i].Revision == artifact.CurrentRevision {
			r := rr
			resp.Current = &r
		}
		if artifact.AdoptedRevision != nil && revisions[i].Revision == *artifact.AdoptedRevision {
			r := rr
			resp.Adopted = &r
		}
	}
	return &resp, nil
}

func toRevisionResponse(rev *store.ResearchArtifactRevision) revisionResponse {
	r := revisionResponse{
		ID: rev.ID.UUID, Revision: rev.Revision, Status: rev.Status, Origin: rev.Origin,
		Payload: json.RawMessage(rev.PayloadJSON), CreatedAt: dto.JSONTime{Time: rev.CreatedAt},
	}
	if rev.RunID != nil {
		id := rev.RunID.UUID
		r.RunID = &id
	}
	return r
}

// ── 校验与依赖 ──────────────────────────────────────────────────────

func (s *Service) validatePayload(ctx context.Context, projectID store.UUID, kind string, raw json.RawMessage, deps []dependencyInput) (datatypes.JSON, []dependencyInput, error) {
	if len(raw) == 0 {
		return nil, nil, httpapi.BadRequest("缺少产物内容")
	}
	var allDeps []dependencyInput
	addEvidence := func(refs []EvidenceRef) {
		for _, ref := range refs {
			if ref.SourceVersionID != nil {
				allDeps = append(allDeps, dependencyInput{Kind: store.ResearchDependencySourceVersion, ID: *ref.SourceVersionID})
			}
			if ref.DocumentID != nil {
				allDeps = append(allDeps, dependencyInput{Kind: store.ResearchDependencySourceVersion, ID: *ref.DocumentID})
			}
		}
	}
	validateRef := func(ref EvidenceRef) error {
		if ref.SourceVersionID != nil {
			return s.checkSourceVersion(ctx, projectID, *ref.SourceVersionID)
		}
		if ref.DocumentID != nil {
			var count int64
			if err := s.DB.WithContext(ctx).Model(&store.Document{}).
				Where("id = ? AND project_id = ?", *ref.DocumentID, projectID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return httpapi.BadRequest("引用的资料不属于该项目")
			}
		}
		return nil
	}
	validateField := func(f Field) error {
		if !fieldStatuses[f.Status] {
			return httpapi.BadRequest("字段状态不合法")
		}
		if f.Status == "reported" {
			if strings.TrimSpace(f.Text) == "" {
				return httpapi.BadRequest("已报告字段必须包含内容")
			}
			if len(f.Evidence) == 0 {
				return httpapi.BadRequest("已报告字段必须附带原文依据")
			}
		}
		for _, ref := range f.Evidence {
			if err := validateRef(ref); err != nil {
				return err
			}
		}
		addEvidence(f.Evidence)
		return nil
	}

	switch kind {
	case store.ResearchArtifactLiteratureNote:
		var p LiteratureNote
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, httpapi.BadRequest("阅读记录结构不合法")
		}
		if err := validateRef(p.Source); err != nil {
			return nil, nil, err
		}
		addEvidence([]EvidenceRef{p.Source})
		for _, f := range []Field{p.Question, p.Method, p.Objects, p.Conditions, p.Results, p.Limitations} {
			if err := validateField(f); err != nil {
				return nil, nil, err
			}
		}
	case store.ResearchArtifactLiteratureComparison:
		var p LiteratureComparison
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, httpapi.BadRequest("文献比较结构不合法")
		}
		if strings.TrimSpace(p.Question) == "" || len(p.Axes) == 0 || len(p.Groups) == 0 {
			return nil, nil, httpapi.BadRequest("比较必须包含问题、轴与分组")
		}
		for _, g := range p.Groups {
			for _, f := range g.Values {
				if err := validateField(f); err != nil {
					return nil, nil, err
				}
			}
		}
		for _, list := range [][]Field{p.Commonality, p.Differences, p.Conflicts, p.Missing} {
			for _, f := range list {
				if err := validateField(f); err != nil {
					return nil, nil, err
				}
			}
		}
	case store.ResearchArtifactChapterPlan:
		var p ChapterPlan
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, httpapi.BadRequest("章节计划结构不合法")
		}
		if len(p.Sections) == 0 {
			return nil, nil, httpapi.BadRequest("章节计划不能为空")
		}
		ids := map[string]bool{}
		for _, sec := range p.Sections {
			if strings.TrimSpace(sec.ID) == "" || strings.TrimSpace(sec.Title) == "" || strings.TrimSpace(sec.Responsibility) == "" {
				return nil, nil, httpapi.BadRequest("章节必须包含 ID、标题与职责")
			}
			if ids[sec.ID] {
				return nil, nil, httpapi.BadRequest("章节 ID 重复")
			}
			ids[sec.ID] = true
			for _, claim := range sec.Claims {
				if err := validateField(Field{Text: claim.Text, Status: claim.Status, Evidence: claim.Evidence}); err != nil {
					return nil, nil, err
				}
			}
		}
		for _, sec := range p.Sections {
			for _, dep := range sec.Dependencies {
				if !ids[dep] {
					return nil, nil, httpapi.BadRequest("章节依赖指向不存在的章节")
				}
			}
		}
	case store.ResearchArtifactChapterDigest:
		var p ChapterDigest
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, httpapi.BadRequest("章节摘要结构不合法")
		}
		if p.Input.LatexFileID.UUID == uuid.Nil || strings.TrimSpace(p.SectionID) == "" || strings.TrimSpace(p.Content) == "" || len(p.Coverage) == 0 {
			return nil, nil, httpapi.BadRequest("摘要必须包含稿件、章节、内容与覆盖范围")
		}
		var count int64
		if err := s.DB.WithContext(ctx).Model(&store.LatexFile{}).
			Where("id = ? AND project_id = ?", p.Input.LatexFileID, projectID).Count(&count).Error; err != nil {
			return nil, nil, err
		}
		if count == 0 {
			return nil, nil, httpapi.BadRequest("摘要引用的稿件不属于该项目")
		}
	}
	allDeps = append(allDeps, deps...)
	return datatypes.JSON(raw), allDeps, nil
}

func (s *Service) checkSourceVersion(ctx context.Context, projectID, versionID store.UUID) error {
	var count int64
	err := s.DB.WithContext(ctx).Model(&store.SourceVersion{}).
		Joins("JOIN project_sources ON project_sources.id = source_versions.source_id").
		Where("source_versions.id = ? AND project_sources.project_id = ?", versionID, projectID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return httpapi.BadRequest("引用的来源版本不属于该项目")
	}
	return nil
}

var dependencyKinds = map[string]bool{
	store.ResearchDependencySourceVersion: true,
	store.ResearchDependencySourceSegment: true,
	store.ResearchDependencyArtifact:      true,
	store.ResearchDependencyRun:           true,
}

func createDependencies(tx *gorm.DB, projectID, artifactID store.UUID, revision int, deps []dependencyInput) error {
	seen := map[string]bool{}
	for _, dep := range deps {
		key := dep.Kind + ":" + dep.ID.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := tx.Create(&store.ResearchArtifactDependency{
			ProjectID: projectID, ArtifactID: artifactID, Revision: revision,
			Kind: dep.Kind, RefID: dep.ID,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validateDependencies(ctx context.Context, projectID, artifactID store.UUID, revision int) error {
	var deps []store.ResearchArtifactDependency
	if err := s.DB.WithContext(ctx).Where("artifact_id = ? AND revision = ?", artifactID, revision).Find(&deps).Error; err != nil {
		return err
	}
	for _, dep := range deps {
		switch dep.Kind {
		case store.ResearchDependencySourceVersion:
			if err := s.checkSourceVersion(ctx, projectID, dep.RefID); err != nil {
				return err
			}
		case store.ResearchDependencySourceSegment:
			var count int64
			err := s.DB.WithContext(ctx).Model(&store.SourceSegment{}).
				Joins("JOIN source_extractions ON source_extractions.id = source_segments.extraction_id").
				Joins("JOIN source_versions ON source_versions.id = source_extractions.source_version_id").
				Joins("JOIN project_sources ON project_sources.id = source_versions.source_id").
				Where("source_segments.id = ? AND project_sources.project_id = ?", dep.RefID, projectID).Count(&count).Error
			if err != nil {
				return err
			}
			if count == 0 {
				return httpapi.BadRequest("引用的原文段落已失效")
			}
		case store.ResearchDependencyArtifact:
			var count int64
			if err := s.DB.WithContext(ctx).Model(&store.ResearchArtifact{}).
				Where("id = ? AND project_id = ?", dep.RefID, projectID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return httpapi.BadRequest("引用的产物不属于该项目")
			}
		case store.ResearchDependencyRun:
			var count int64
			err := s.DB.WithContext(ctx).Model(&store.AgentRun{}).
				Joins("JOIN agent_conversations ON agent_conversations.id = agent_runs.conversation_id").
				Where("agent_runs.id = ? AND agent_conversations.project_id = ?", dep.RefID, projectID).Count(&count).Error
			if err != nil {
				return err
			}
			if count == 0 {
				return httpapi.BadRequest("引用的运行不属于该项目")
			}
		default:
			return httpapi.BadRequest("依赖类型不合法")
		}
	}
	return nil
}

func conflict(info string) error { return &httpapi.APIError{Code: http.StatusConflict, Info: info} }

// ── HTTP ────────────────────────────────────────────────────────────

func parseIDs(c *gin.Context) (store.UUID, store.UUID, bool) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return store.UUID{}, store.UUID{}, false
	}
	aid, err := uuid.Parse(c.Param("artifact_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("产物"))
		return store.UUID{}, store.UUID{}, false
	}
	return store.UUID{UUID: pid}, store.UUID{UUID: aid}, true
}

func (s *Service) list(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return
	}
	items, err := s.List(c.Request.Context(), auth.CurrentUser(c).ID, store.UUID{UUID: pid}, c.Query("kind"))
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(items, ""))
}

func (s *Service) create(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.Create(c.Request.Context(), auth.CurrentUser(c).ID, store.UUID{UUID: pid}, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(item, ""))
}

func (s *Service) addRevision(c *gin.Context) {
	pid, aid, ok := parseIDs(c)
	if !ok {
		return
	}
	var req revisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.AddRevision(c.Request.Context(), auth.CurrentUser(c).ID, pid, aid, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(item, ""))
}

func (s *Service) adopt(c *gin.Context) {
	pid, aid, ok := parseIDs(c)
	if !ok {
		return
	}
	var req decisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.Adopt(c.Request.Context(), auth.CurrentUser(c).ID, pid, aid, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(item, ""))
}

func (s *Service) reject(c *gin.Context) {
	pid, aid, ok := parseIDs(c)
	if !ok {
		return
	}
	var req decisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.Reject(c.Request.Context(), auth.CurrentUser(c).ID, pid, aid, req)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(item, ""))
}

func (s *Service) getRevision(c *gin.Context) {
	pid, aid, ok := parseIDs(c)
	if !ok {
		return
	}
	revision, err := strconv.Atoi(c.Param("revision"))
	if err != nil || revision <= 0 {
		httpapi.WriteError(c.Writer, httpapi.NotFound("版本"))
		return
	}
	item, err := s.GetRevision(c.Request.Context(), auth.CurrentUser(c).ID, pid, aid, revision)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(item, ""))
}

func (s *Service) deleteArtifact(c *gin.Context) {
	pid, aid, ok := parseIDs(c)
	if !ok {
		return
	}
	if err := s.Delete(c.Request.Context(), auth.CurrentUser(c).ID, pid, aid); err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"deleted": true}, ""))
}

func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	p := rg.Group("/projects", authMw)
	p.GET("/:project_id/research-artifacts", s.list)
	p.POST("/:project_id/research-artifacts", s.create)

	a := rg.Group("/research-artifacts", authMw)
	a.GET("/:artifact_id/revisions/:revision", s.getRevision)
	a.POST("/:artifact_id/revisions", s.addRevision)
	a.POST("/:artifact_id/adopt", s.adopt)
	a.POST("/:artifact_id/reject", s.reject)
	a.DELETE("/:artifact_id", s.deleteArtifact)
}
