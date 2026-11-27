// Package research 科研组织产物域：阅读记录、文献比较、章节计划与章节摘要。
// 契约来源：openspec/changes/research-organization-artifacts。
package research

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/recheck"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type Service struct {
	DB *gorm.DB
	// Norms 是校验规范依据的权威来源：模型写下的规范条目必须能在项目已采用集合里
	// 找到，不能只跟模型自己填的 input.norms 自洽。
	Norms norms.Provider
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

// ── 跨章节检查报告（chapter_review）─────────────────────────────────
//
// 四个语义维度只能由模型基于三源上下文推理得出；本文件只校验结构、
// 归属与诚实边界（例如规范库不可用时不得声称完成规范符合度检查）。

const (
	reviewMaxFindings       = 50
	reviewMaxScopedFindings = 200
)

var (
	// 旧四维整书报告（scope 缺省）允许的维度集合。
	reviewLegacyDimensionSet = map[string]bool{"goal_conclusion": true, "evidence_support": true, "cross_section": true, "norm_compliance": true}
	// 范围化报告允许的全部维度（新增术语一致与双场景闭环）。
	reviewDimensionSet = map[string]bool{
		"goal_conclusion": true, "evidence_support": true, "cross_section": true, "norm_compliance": true,
		"terminology_consistency": true, "thesis_engineering_closure": true, "paper_contribution_closure": true,
	}
	reviewClaimStatuses = map[string]bool{"overclaimed": true, "orphaned": true}
	reviewStatuses      = map[string]bool{"checked": true, "findings": true, "insufficient": true, "not_checked": true, "failed": true}
	reviewSeverities    = map[string]bool{"high": true, "medium": true, "low": true}
	reviewBasisKinds    = map[string]bool{"norm": true, "plan": true, "text": true, "source": true}
	reviewSourceKinds   = map[string]bool{"latex": true, "plan": true, "document": true, "source": true, "norm": true}
	reviewPlanStatuses  = map[string]bool{"adopted": true, "candidate": true, "missing": true}
)

type ReviewPlanSection struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ChapterReviewInput struct {
	PlanArtifactID *store.UUID         `json:"planArtifactId,omitempty"`
	PlanRevision   *int                `json:"planRevision,omitempty"`
	PlanStatus     string              `json:"planStatus,omitempty"`
	PlanSections   []ReviewPlanSection `json:"planSections,omitempty"`
	NormsAvailable bool                `json:"normsAvailable"`
	NormsNote      string              `json:"normsNote,omitempty"`
	Norms          []norms.Entry       `json:"norms,omitempty"`
}

type ReviewDimension struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

type ReviewBasis struct {
	Kind      string `json:"kind"`
	Ref       string `json:"ref"`
	FilePath  string `json:"filePath,omitempty"`
	Quote     string `json:"quote,omitempty"`
	LineStart int    `json:"lineStart,omitempty"`
	LineEnd   int    `json:"lineEnd,omitempty"`
}

type ReviewFinding struct {
	ID           string        `json:"id"`
	Dimension    string        `json:"dimension"`
	Severity     string        `json:"severity"`
	SectionID    string        `json:"sectionId,omitempty"`
	SectionTitle string        `json:"sectionTitle,omitempty"`
	FilePath     string        `json:"filePath,omitempty"`
	Quote        string        `json:"quote,omitempty"`
	LineStart    int           `json:"lineStart,omitempty"`
	LineEnd      int           `json:"lineEnd,omitempty"`
	Problem      string        `json:"problem"`
	Basis        []ReviewBasis `json:"basis"`
	NextStep     string        `json:"nextStep"`
	// ClaimStatus 仅在 evidence_support 维度使用：overclaimed / orphaned。
	ClaimStatus string `json:"claimStatus,omitempty"`
	// Fingerprint 由服务端计算，用于跨报告收敛与处置账本。
	Fingerprint    string `json:"fingerprint,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	AnchorVerified bool   `json:"anchorVerified,omitempty"`
}

// ChapterReviewScope 是范围化检查报告的范围快照；缺省表示旧的四维整书报告。
type ChapterReviewScope struct {
	Dimensions  []string          `json:"dimensions"`
	Files       []string          `json:"files"`
	SectionIDs  []string          `json:"sectionIds,omitempty"`
	Incremental bool              `json:"incremental"`
	Warnings    []string          `json:"warnings,omitempty"`
	Units       []ReviewScopeUnit `json:"units,omitempty"`
}

// ReviewScopeUnit 是报告中每个检查单元的执行概述。
type ReviewScopeUnit struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	Fingerprint string `json:"fingerprint"`
}

type ReviewSource struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type ChapterReview struct {
	Input       ChapterReviewInput  `json:"input"`
	Scope       *ChapterReviewScope `json:"scope,omitempty"`
	Dimensions  []ReviewDimension   `json:"dimensions"`
	Findings    []ReviewFinding     `json:"findings"`
	SourcesRead []ReviewSource      `json:"sourcesRead"`
}

// reviewPlanRef 记录检查报告实际依据的计划产物与修订号（未引用计划时为空）。
type reviewPlanRef struct {
	ArtifactID *store.UUID
	Revision   int
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

type CreateInput = createRequest
type RevisionInput = revisionRequest
type DecisionInput = decisionRequest

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
	// 依赖资料变更后未处理的复查原因；处理完即消失。
	NeedsRecheck   bool     `json:"needsRecheck"`
	RecheckReasons []string `json:"recheckReasons,omitempty"`
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
	// 产物级聚合：当前修订与已采用修订的并集，旧候选上的标记不给整个产物打标。
	NeedsRecheck   bool     `json:"needsRecheck"`
	RecheckReasons []string `json:"recheckReasons,omitempty"`
}

// ── 写入路径 ────────────────────────────────────────────────────────

func validKind(kind string) bool {
	switch kind {
	case store.ResearchArtifactLiteratureNote, store.ResearchArtifactLiteratureComparison,
		store.ResearchArtifactChapterPlan, store.ResearchArtifactChapterDigest,
		store.ResearchArtifactChapterReview:
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
		return httpapi.NotFound("Project")
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
		return nil, httpapi.NotFound("Revision")
	}
	if rev.Status == store.ResearchArtifactRevisionRejected {
		return nil, httpapi.BadRequest("已拒绝的修订不能采用")
	}
	if rev.Status == store.ResearchArtifactRevisionAdopted {
		return s.loadArtifact(ctx, artifact.ID)
	}
	// 依赖资料已变更的修订必须先复查：用户确认「已复查 / 已重建 / 忽略」后才能采用。
	reasons, err := recheck.OpenReasons(ctx, s.DB, store.RecheckTargetArtifactRevision, rev.ID)
	if err != nil {
		return nil, err
	}
	if len(reasons) > 0 {
		return nil, conflict("依赖资料已变更，请先复查或重建：" + strings.Join(reasons, "；"))
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
		return nil, httpapi.NotFound("Revision")
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
		return nil, httpapi.NotFound("Artifact")
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
	revisionIDs := make([]store.UUID, 0, len(revisions))
	for i := range revisions {
		revisionIDs = append(revisionIDs, revisions[i].ID)
	}
	markReasons, err := recheck.OpenRevisionMarks(ctx, s.DB, revisionIDs)
	if err != nil {
		return nil, err
	}
	for i := range revisions {
		rr := toRevisionResponse(&revisions[i])
		if reasons := markReasons[revisions[i].ID]; len(reasons) > 0 {
			rr.NeedsRecheck = true
			rr.RecheckReasons = reasons
		}
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
	for _, target := range []*revisionResponse{resp.Current, resp.Adopted} {
		if target == nil || !target.NeedsRecheck {
			continue
		}
		resp.NeedsRecheck = true
		for _, reason := range target.RecheckReasons {
			if !slices.Contains(resp.RecheckReasons, reason) {
				resp.RecheckReasons = append(resp.RecheckReasons, reason)
			}
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
	addEvidence := func(refs []EvidenceRef) error {
		for _, ref := range refs {
			if ref.SourceVersionID != nil {
				allDeps = append(allDeps, dependencyInput{Kind: store.ResearchDependencySourceVersion, ID: *ref.SourceVersionID})
			}
			if ref.DocumentID != nil {
				// 记录实际依赖：能解析到文档当前来源版本就记版本，否则记文档本身。
				// 曾把文档 ID 直接记成 source_version，adopt 时按版本表校验必然 400。
				versionID, ok, err := s.resolveDocumentVersion(ctx, *ref.DocumentID)
				if err != nil {
					return err
				}
				if ok {
					allDeps = append(allDeps, dependencyInput{Kind: store.ResearchDependencySourceVersion, ID: versionID})
				} else {
					allDeps = append(allDeps, dependencyInput{Kind: store.ResearchDependencyDocument, ID: *ref.DocumentID})
				}
			}
		}
		return nil
	}
	validateRef := func(ref EvidenceRef) error {
		if ref.SourceVersionID != nil {
			return s.checkSourceVersionForCreate(ctx, projectID, *ref.SourceVersionID)
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
		return addEvidence(f.Evidence)
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
		if err := addEvidence([]EvidenceRef{p.Source}); err != nil {
			return nil, nil, err
		}
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
	case store.ResearchArtifactChapterReview:
		var p ChapterReview
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, httpapi.BadRequest("跨章节检查报告结构不合法")
		}
		var plan reviewPlanRef
		if err := s.validateChapterReview(ctx, projectID, p, &plan); err != nil {
			return nil, nil, err
		}
		if plan.ArtifactID != nil {
			// 记录「检查的是哪一版计划」并落依赖行：资料变更时只标记受影响的报告。
			if p.Input.PlanRevision == nil || *p.Input.PlanRevision != plan.Revision {
				p.Input.PlanRevision = &plan.Revision
				updated, err := json.Marshal(p)
				if err != nil {
					return nil, nil, err
				}
				raw = updated
			}
			allDeps = append(allDeps, dependencyInput{Kind: store.ResearchDependencyArtifact, ID: *plan.ArtifactID})
		}
	}
	for _, dep := range deps {
		if !dependencyKinds[dep.Kind] {
			return nil, nil, httpapi.BadRequest("依赖类型不合法")
		}
	}
	allDeps = append(allDeps, deps...)
	return datatypes.JSON(raw), allDeps, nil
}

// validateNormRefs 把报告的规范依据与「项目已采用集合」对齐。
//
// 只和模型自己填的 input.norms 比对是自证：模型少列几条，就能既声称用了规范、
// 又引用未采用的条款。这里以服务端 Provider（采用时锁定的版本）为准。
func (s *Service) validateNormRefs(ctx context.Context, projectID store.UUID, p ChapterReview) error {
	claimsNorms := p.Input.NormsAvailable || len(p.Input.Norms) > 0 || hasNormBasis(p.Findings)
	if s.Norms == nil {
		if claimsNorms {
			return httpapi.BadRequest("学术规范库未接入，检查报告不能声称使用了规范依据")
		}
		return nil
	}
	entries, err := s.Norms.AdoptedForProject(ctx, projectID)
	if errors.Is(err, norms.ErrUnavailable) {
		if claimsNorms {
			return httpapi.BadRequest("学术规范库不可用，检查报告不能声称使用了规范依据")
		}
		return nil
	}
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(entries)*3)
	for _, entry := range entries {
		known[entry.Cite()] = true
		known[entry.ID] = true
		if entry.Title != "" {
			known[entry.Title] = true
		}
	}
	for _, entry := range p.Input.Norms {
		if !known[entry.Cite()] && !known[entry.ID] && (entry.Title == "" || !known[entry.Title]) {
			return httpapi.BadRequest("检查报告列出的规范条目不在本项目已采用的规范中：" + entry.Title)
		}
	}
	for _, finding := range p.Findings {
		for _, basis := range finding.Basis {
			if basis.Kind != "norm" {
				continue
			}
			if !known[strings.TrimSpace(basis.Ref)] {
				return httpapi.BadRequest("依据引用的规范条目不在本项目已采用的规范中：" + basis.Ref)
			}
		}
	}
	return nil
}

func hasNormBasis(findings []ReviewFinding) bool {
	for _, finding := range findings {
		for _, basis := range finding.Basis {
			if basis.Kind == "norm" {
				return true
			}
		}
	}
	return false
}

// validateChapterReview 只校验结构与诚实边界；语义判断由模型完成。
// 这里刻意拒绝「规范库不可用却声称已完成规范符合度检查」这类伪通过。
func (s *Service) validateChapterReview(ctx context.Context, projectID store.UUID, p ChapterReview, plan *reviewPlanRef) error {
	if err := validateReviewDimensionSet(p); err != nil {
		return err
	}
	statusByKey := map[string]string{}
	for _, d := range p.Dimensions {
		if !reviewDimensionSet[d.Key] {
			return httpapi.BadRequest("检查维度不合法")
		}
		if statusByKey[d.Key] != "" {
			return httpapi.BadRequest("检查维度重复")
		}
		if !reviewStatuses[d.Status] {
			return httpapi.BadRequest("检查状态不合法")
		}
		if (d.Status == "insufficient" || d.Status == "not_checked" || d.Status == "failed") && strings.TrimSpace(d.Note) == "" {
			return httpapi.BadRequest("未覆盖或证据不足的维度必须写明原因")
		}
		statusByKey[d.Key] = d.Status
	}
	if !p.Input.NormsAvailable && len(p.Input.Norms) > 0 {
		return httpapi.BadRequest("规范库不可用时不能附带规范条目")
	}
	if p.Input.NormsAvailable && len(p.Input.Norms) == 0 && (statusByKey["norm_compliance"] == "checked" || statusByKey["norm_compliance"] == "findings") {
		return httpapi.BadRequest("没有已采用的规范条目，规范符合度只能标记为证据不足或未检查")
	}
	if !p.Input.NormsAvailable && (statusByKey["norm_compliance"] == "checked" || statusByKey["norm_compliance"] == "findings") {
		return httpapi.BadRequest("规范库不可用时规范符合度只能标记为证据不足或未检查")
	}
	if len(p.Input.Norms) > 100 {
		return httpapi.BadRequest("规范条目过多")
	}
	for _, entry := range p.Input.Norms {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.Rule) == "" {
			return httpapi.BadRequest("规范条目必须包含 ID、标题与规则说明")
		}
	}
	if p.Input.PlanStatus != "" && !reviewPlanStatuses[p.Input.PlanStatus] {
		return httpapi.BadRequest("章节计划状态不合法")
	}
	if p.Input.PlanArtifactID == nil {
		if p.Input.PlanRevision != nil || (p.Input.PlanStatus != "" && p.Input.PlanStatus != "missing") {
			return httpapi.BadRequest("章节计划引用不完整")
		}
	} else if p.Input.PlanStatus == "missing" {
		return httpapi.BadRequest("章节计划状态与引用不一致")
	}
	maxFindings := reviewMaxFindings
	if p.Scope != nil {
		maxFindings = reviewMaxScopedFindings
	}
	if len(p.Findings) > maxFindings {
		return httpapi.BadRequest("检查问题数量超过上限")
	}
	if len(p.SourcesRead) > 200 {
		return httpapi.BadRequest("阅读来源数量超过上限")
	}
	for _, src := range p.SourcesRead {
		if !reviewSourceKinds[src.Kind] || strings.TrimSpace(src.Name) == "" {
			return httpapi.BadRequest("阅读来源记录不合法")
		}
	}

	var planSections map[string]ChapterSection
	if p.Input.PlanArtifactID != nil {
		if plan != nil {
			plan.ArtifactID = p.Input.PlanArtifactID
		}
		var artifact store.ResearchArtifact
		err := s.DB.WithContext(ctx).Where("id = ? AND project_id = ? AND kind = ?",
			*p.Input.PlanArtifactID, projectID, store.ResearchArtifactChapterPlan).First(&artifact).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpapi.BadRequest("检查报告引用的章节计划不属于该项目")
		}
		if err != nil {
			return err
		}
		revision := artifact.CurrentRevision
		if artifact.AdoptedRevision != nil {
			revision = *artifact.AdoptedRevision
		}
		if p.Input.PlanRevision != nil {
			revision = *p.Input.PlanRevision
		}
		if revision <= 0 || revision > artifact.CurrentRevision {
			return httpapi.BadRequest("章节计划修订号不存在")
		}
		if plan != nil {
			plan.Revision = revision
		}
		var rev store.ResearchArtifactRevision
		if err := s.DB.WithContext(ctx).Where("artifact_id = ? AND revision = ?", artifact.ID, revision).First(&rev).Error; err != nil {
			return httpapi.BadRequest("章节计划修订号不存在")
		}
		var plan ChapterPlan
		if err := json.Unmarshal(rev.PayloadJSON, &plan); err != nil {
			return httpapi.BadRequest("章节计划内容不合法")
		}
		planSections = map[string]ChapterSection{}
		for _, sec := range plan.Sections {
			planSections[sec.ID] = sec
		}
	}

	checkedFiles := map[string]bool{}
	checkFile := func(path string) error {
		if path == "" || checkedFiles[path] {
			return nil
		}
		var count int64
		if err := s.DB.WithContext(ctx).Model(&store.LatexFile{}).
			Where("project_id = ? AND file_path = ?", projectID, path).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return httpapi.BadRequest("检查定位的稿件文件不存在：" + path)
		}
		checkedFiles[path] = true
		return nil
	}

	scopeFiles := map[string]bool{}
	scopeDims := map[string]bool{}
	scopeSections := map[string]bool{}
	if p.Scope != nil {
		for _, f := range p.Scope.Files {
			if err := checkFile(f); err != nil {
				return err
			}
			scopeFiles[f] = true
		}
		for _, d := range p.Scope.Dimensions {
			scopeDims[d] = true
		}
		for _, s := range p.Scope.SectionIDs {
			scopeSections[s] = true
		}
	}

	findingIDs := map[string]bool{}
	for _, f := range p.Findings {
		if strings.TrimSpace(f.ID) == "" || findingIDs[f.ID] {
			return httpapi.BadRequest("检查问题必须包含唯一 ID")
		}
		findingIDs[f.ID] = true
		if !reviewDimensionSet[f.Dimension] {
			return httpapi.BadRequest("检查问题维度不合法")
		}
		if p.Scope != nil {
			if !scopeDims[f.Dimension] {
				return httpapi.BadRequest("检查问题维度超出本次范围")
			}
			if f.FilePath == "" || !scopeFiles[f.FilePath] {
				return httpapi.BadRequest("检查问题定位超出本次范围：" + f.FilePath)
			}
			if strings.TrimSpace(f.Fingerprint) == "" {
				return httpapi.BadRequest("检查问题缺少服务端指纹")
			}
			if f.SectionID != "" && !scopeSections[f.SectionID] {
				return httpapi.BadRequest("检查问题章节超出本次范围")
			}
			if err := validateClaimEvidence(f); err != nil {
				return err
			}
		}
		if !reviewSeverities[f.Severity] {
			return httpapi.BadRequest("检查问题严重程度不合法")
		}
		if strings.TrimSpace(f.Problem) == "" || strings.TrimSpace(f.NextStep) == "" {
			return httpapi.BadRequest("检查问题必须写清问题与下一步")
		}
		if f.Confidence != "" && f.Confidence != "high" && f.Confidence != "low" {
			return httpapi.BadRequest("检查问题置信度不合法")
		}
		if f.SectionID == "" && f.FilePath == "" {
			return httpapi.BadRequest("检查问题必须定位到章节或文件")
		}
		if f.LineStart < 0 || f.LineEnd < 0 || (f.LineEnd > 0 && f.LineEnd < f.LineStart) {
			return httpapi.BadRequest("检查问题行号不合法")
		}
		if len(f.Basis) == 0 {
			return httpapi.BadRequest("检查问题必须给出依据")
		}
		effectivePath := f.FilePath
		if f.SectionID != "" {
			if planSections == nil {
				return httpapi.BadRequest("章节定位必须对应章节计划")
			}
			sec, ok := planSections[f.SectionID]
			if !ok {
				return httpapi.BadRequest("检查问题指向的章节不在章节计划中")
			}
			if sec.FilePath != nil {
				if f.FilePath != "" && f.FilePath != *sec.FilePath {
					return httpapi.BadRequest("检查问题定位文件与章节计划不一致")
				}
				effectivePath = *sec.FilePath
			}
		}
		if err := checkFile(effectivePath); err != nil {
			return err
		}
		for _, b := range f.Basis {
			if !reviewBasisKinds[b.Kind] || strings.TrimSpace(b.Ref) == "" {
				return httpapi.BadRequest("检查依据不合法")
			}
			if b.LineStart < 0 || b.LineEnd < 0 || (b.LineEnd > 0 && b.LineEnd < b.LineStart) {
				return httpapi.BadRequest("检查依据行号不合法")
			}
			if (b.Kind == "text" || b.Kind == "plan" || b.Kind == "source") && strings.TrimSpace(b.Quote) == "" {
				return httpapi.BadRequest("检查依据必须给出原文摘录")
			}
			switch b.Kind {
			case "text":
				path := b.FilePath
				if path == "" {
					path = b.Ref
				}
				if p.Scope != nil && !scopeFiles[path] {
					return httpapi.BadRequest("检查依据超出本次范围：" + path)
				}
				if err := checkFile(path); err != nil {
					return err
				}
			case "plan":
				if planSections == nil {
					return httpapi.BadRequest("章节计划依据不可用")
				}
				if _, ok := planSections[b.Ref]; !ok {
					return httpapi.BadRequest("依据引用的章节不在章节计划中")
				}
			case "norm":
				// 统一由 validateNormRefs 按项目已采用集合校验。
			}
		}
	}
	if err := s.validateNormRefs(ctx, projectID, p); err != nil {
		return err
	}
	findingsPerDimension := map[string]int{}
	for _, f := range p.Findings {
		findingsPerDimension[f.Dimension]++
	}
	for _, d := range p.Dimensions {
		switch d.Status {
		case "findings":
			if findingsPerDimension[d.Key] == 0 {
				return httpapi.BadRequest("维度标记为发现问题时必须至少给出一条问题")
			}
		case "checked":
			if findingsPerDimension[d.Key] > 0 {
				return httpapi.BadRequest("维度标记为已检查时不能包含问题")
			}
		}
	}
	return nil
}

// validateReviewDimensionSet 区分旧的四维整书报告与范围化报告：
// scope 缺省时必须恰好四维；scope 存在时维度必须与 scope 完全一致。
func validateReviewDimensionSet(p ChapterReview) error {
	if p.Scope == nil {
		if len(p.Dimensions) != len(reviewLegacyDimensionSet) {
			return httpapi.BadRequest("检查报告必须覆盖四个语义维度")
		}
		seen := map[string]bool{}
		for _, d := range p.Dimensions {
			if !reviewLegacyDimensionSet[d.Key] || seen[d.Key] {
				return httpapi.BadRequest("检查维度不合法")
			}
			seen[d.Key] = true
		}
		return nil
	}
	if len(p.Scope.Dimensions) == 0 || len(p.Scope.Files) == 0 {
		return httpapi.BadRequest("范围化检查报告必须包含维度与文件范围")
	}
	allowed := map[string]bool{}
	for _, key := range p.Scope.Dimensions {
		if !reviewDimensionSet[key] || allowed[key] {
			return httpapi.BadRequest("检查维度不合法")
		}
		allowed[key] = true
	}
	if len(p.Dimensions) != len(allowed) {
		return httpapi.BadRequest("检查报告的维度必须与范围一致")
	}
	for _, d := range p.Dimensions {
		if !allowed[d.Key] {
			return httpapi.BadRequest("检查维度超出本次范围")
		}
	}
	if len(p.Scope.Units) == 0 {
		return httpapi.BadRequest("范围化检查报告必须包含单元执行记录")
	}
	for _, u := range p.Scope.Units {
		if strings.TrimSpace(u.ID) == "" || strings.TrimSpace(u.Label) == "" || strings.TrimSpace(u.Fingerprint) == "" {
			return httpapi.BadRequest("检查单元执行记录不完整")
		}
		if u.Kind != "chapter" && u.Kind != "collection" {
			return httpapi.BadRequest("检查单元类型不合法")
		}
	}
	return nil
}

// validateClaimEvidence 落实 Claim–Evidence 的硬约束：证据支撑必须标注状态，
// 过度断言必须有对照依据，跨章矛盾必须给出双方原文。
func validateClaimEvidence(f ReviewFinding) error {
	switch f.Dimension {
	case "evidence_support":
		if !reviewClaimStatuses[f.ClaimStatus] {
			return httpapi.BadRequest("证据支撑问题必须标注过度断言或无据孤立")
		}
		if f.ClaimStatus == "overclaimed" && len(f.Basis) == 0 {
			return httpapi.BadRequest("过度断言必须给出对照依据")
		}
	case "cross_section":
		files := map[string]bool{}
		for _, b := range f.Basis {
			if b.Kind == "text" {
				p := b.FilePath
				if p == "" {
					p = b.Ref
				}
				files[p] = true
			}
		}
		if len(files) < 2 {
			return httpapi.BadRequest("跨章矛盾必须给出冲突双方原文")
		}
	}
	return nil
}

// resolveDocumentVersion 解析文档当前的来源版本（documents.source_id → 最大 revision）。
// 文档未走版本化上传或来源尚无版本时返回 false，由调用方改记 document 依赖。
func (s *Service) resolveDocumentVersion(ctx context.Context, documentID store.UUID) (store.UUID, bool, error) {
	var doc store.Document
	if err := s.DB.WithContext(ctx).Select("id", "source_id").First(&doc, "id = ?", documentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.UUID{}, false, nil
		}
		return store.UUID{}, false, err
	}
	if doc.SourceID == nil {
		return store.UUID{}, false, nil
	}
	var version store.SourceVersion
	err := s.DB.WithContext(ctx).Where("source_id = ?", *doc.SourceID).
		Order("revision DESC").First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.UUID{}, false, nil
	}
	if err != nil {
		return store.UUID{}, false, err
	}
	return version.ID, true, nil
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

// checkSourceVersionForCreate 在创建/新增修订路径上额外要求：文档来源必须仍有
// 未删除文档引用，防止删除资料后用旧 source_version 造出新产物绕开复查。
// Adopt 路径不走这里（用户复查后仍应能采用旧修订）。
func (s *Service) checkSourceVersionForCreate(ctx context.Context, projectID, versionID store.UUID) error {
	if err := s.checkSourceVersion(ctx, projectID, versionID); err != nil {
		return err
	}
	var row struct {
		Kind     string     `gorm:"column:kind"`
		SourceID store.UUID `gorm:"column:source_id"`
	}
	err := s.DB.WithContext(ctx).Table("source_versions AS v").
		Select("ps.kind AS kind, ps.id AS source_id").
		Joins("JOIN project_sources AS ps ON ps.id = v.source_id").
		Where("v.id = ?", versionID).Scan(&row).Error
	if err != nil {
		return err
	}
	if row.Kind != "document" {
		return nil
	}
	var total, active int64
	if err := s.DB.WithContext(ctx).Unscoped().Model(&store.Document{}).
		Where("project_id = ? AND source_id = ?", projectID, row.SourceID).Count(&total).Error; err != nil {
		return err
	}
	if err := s.DB.WithContext(ctx).Model(&store.Document{}).
		Where("project_id = ? AND source_id = ?", projectID, row.SourceID).Count(&active).Error; err != nil {
		return err
	}
	// 只有「曾有文档引用该来源、但现在全被删除」才拒绝；没有文档行的历史/服务级来源不误伤。
	if total > 0 && active == 0 {
		return httpapi.BadRequest("引用的资料已被删除，无法再基于它生成或修改产物")
	}
	return nil
}

var dependencyKinds = map[string]bool{
	store.ResearchDependencySourceVersion: true,
	store.ResearchDependencySourceSegment: true,
	store.ResearchDependencyArtifact:      true,
	store.ResearchDependencyRun:           true,
	store.ResearchDependencyDocument:      true,
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
		case store.ResearchDependencyDocument:
			var count int64
			if err := s.DB.WithContext(ctx).Model(&store.Document{}).
				Where("id = ? AND project_id = ?", dep.RefID, projectID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return httpapi.BadRequest("引用的资料已不存在或不属于该项目")
			}
		default:
			return httpapi.BadRequest("依赖类型不合法")
		}
	}
	return nil
}

func conflict(info string) error { return &httpapi.APIError{Code: http.StatusConflict, Info: info} }

// ── HTTP ────────────────────────────────────────────────────────────

// 产物路由的 URL 只带 artifact_id（前端契约），project_id 由产物反查并校验归属。
func (s *Service) parseIDs(c *gin.Context) (store.UUID, store.UUID, bool) {
	aid, err := uuid.Parse(c.Param("artifact_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("Artifact"))
		return store.UUID{}, store.UUID{}, false
	}
	artifactID := store.UUID{UUID: aid}
	var artifact store.ResearchArtifact
	err = s.DB.WithContext(c.Request.Context()).
		Joins("JOIN projects ON projects.id = research_artifacts.project_id").
		Where("research_artifacts.id = ? AND projects.owner_id = ?", artifactID, auth.CurrentUser(c).ID).
		First(&artifact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpapi.WriteError(c.Writer, httpapi.NotFound("Artifact"))
		return store.UUID{}, store.UUID{}, false
	}
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return store.UUID{}, store.UUID{}, false
	}
	return artifact.ProjectID, artifactID, true
}

func (s *Service) list(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("Project"))
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
		httpapi.WriteError(c.Writer, httpapi.NotFound("Project"))
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
	pid, aid, ok := s.parseIDs(c)
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
	pid, aid, ok := s.parseIDs(c)
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
	pid, aid, ok := s.parseIDs(c)
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
	pid, aid, ok := s.parseIDs(c)
	if !ok {
		return
	}
	revision, err := strconv.Atoi(c.Param("revision"))
	if err != nil || revision <= 0 {
		httpapi.WriteError(c.Writer, httpapi.NotFound("Revision"))
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
	pid, aid, ok := s.parseIDs(c)
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
