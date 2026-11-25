package recheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	defaultListLimit = 100
	maxListLimit     = 200
)

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// MarkResponse 是前端可直接展示的标记视图；目标摘要由服务端确定性派生。
type MarkResponse struct {
	ID            uuid.UUID     `json:"id"`
	ProjectID     uuid.UUID     `json:"projectId"`
	TargetType    string        `json:"targetType"`
	TargetID      uuid.UUID     `json:"targetId"`
	ArtifactID    *uuid.UUID    `json:"artifactId,omitempty"`
	Revision      *int          `json:"revision,omitempty"`
	TargetTitle   string        `json:"targetTitle"`
	TargetDetail  string        `json:"targetDetail,omitempty"`
	TargetMissing bool          `json:"targetMissing"`
	TriggerKind   string        `json:"triggerKind"`
	TriggerLabel  string        `json:"triggerLabel"`
	Reason        string        `json:"reason"`
	CreatedAt     dto.JSONTime  `json:"createdAt"`
	ResolvedAt    *dto.JSONTime `json:"resolvedAt,omitempty"`
	Resolution    *string       `json:"resolution,omitempty"`
}

// List 返回项目下 open / resolved 的待复查标记。
func (s *Service) List(ctx context.Context, uid, projectID store.UUID, status string, limit int) ([]MarkResponse, error) {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return nil, err
	}
	if status == "" {
		status = "open"
	}
	if status != "open" && status != "resolved" {
		return nil, httpapi.BadRequest("状态参数只能是 open 或 resolved")
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	q := s.DB.WithContext(ctx).Model(&store.ContentRecheckMark{}).Where("project_id = ?", projectID)
	if status == "resolved" {
		q = q.Where("resolved_seq > 0").Order("resolved_at DESC")
	} else {
		q = q.Where("resolved_seq = 0").Order("created_at DESC")
	}
	var marks []store.ContentRecheckMark
	if err := q.Limit(limit).Find(&marks).Error; err != nil {
		return nil, err
	}
	return s.summarize(ctx, marks)
}

// Resolve 记录用户对一条待复查标的处理。幂等：同 resolution 返回现状，不同返回 409。
// 「已重建」只是用户声明；服务端不自动生成修订。
func (s *Service) Resolve(ctx context.Context, uid, projectID, markID store.UUID, resolution string) (*MarkResponse, error) {
	switch resolution {
	case store.RecheckResolutionRechecked, store.RecheckResolutionRebuilt, store.RecheckResolutionDismissed:
	default:
		return nil, httpapi.BadRequest("处理方式只能是已复查、已重建或忽略")
	}
	mark, err := s.loadOwnedMark(ctx, uid, projectID, markID)
	if err != nil {
		return nil, err
	}
	if mark.ResolvedSeq != 0 {
		if mark.Resolution != nil && *mark.Resolution == resolution {
			items, err := s.summarize(ctx, []store.ContentRecheckMark{*mark})
			if err != nil {
				return nil, err
			}
			return &items[0], nil
		}
		return nil, conflict("该待复查项已按其他方式处理")
	}

	now := time.Now().UTC()
	res := s.DB.WithContext(ctx).Model(&store.ContentRecheckMark{}).
		Where("id = ? AND resolved_seq = 0", mark.ID).
		Updates(map[string]any{
			"resolved_at": now, "resolved_by": uid,
			"resolution": resolution, "resolved_seq": now.UnixMicro(),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, conflict("该待复查项已被处理，请刷新后重试")
	}
	if mark.TargetType == store.RecheckTargetWikiPage {
		if err := s.clearWikiStaleIfSettled(ctx, mark.TargetID); err != nil {
			return nil, err
		}
	}

	updated, err := s.loadOwnedMark(ctx, uid, projectID, markID)
	if err != nil {
		return nil, err
	}
	items, err := s.summarize(ctx, []store.ContentRecheckMark{*updated})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (s *Service) clearWikiStaleIfSettled(ctx context.Context, pageID store.UUID) error {
	var open int64
	if err := s.DB.WithContext(ctx).Model(&store.ContentRecheckMark{}).
		Where("target_type = ? AND target_id = ? AND resolved_seq = 0",
			store.RecheckTargetWikiPage, pageID).Count(&open).Error; err != nil {
		return err
	}
	if open > 0 {
		return nil
	}
	// agent_wiki_pages 无软删列，可用 Table。
	return s.DB.WithContext(ctx).Table("agent_wiki_pages").
		Where("id = ?", pageID).Update("stale_at", nil).Error
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

func (s *Service) loadOwnedMark(ctx context.Context, uid, projectID, markID store.UUID) (*store.ContentRecheckMark, error) {
	var mark store.ContentRecheckMark
	err := s.DB.WithContext(ctx).
		Joins("JOIN projects ON projects.id = content_recheck_marks.project_id").
		Where("content_recheck_marks.id = ? AND content_recheck_marks.project_id = ? AND projects.owner_id = ?",
			markID, projectID, uid).
		First(&mark).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &httpapi.APIError{Code: http.StatusNotFound, Info: "待复查项不存在"}
	}
	return &mark, err
}

// summarize 批量派生目标摘要；目标已被删除时降级为「目标已不存在」，不报错。
func (s *Service) summarize(ctx context.Context, marks []store.ContentRecheckMark) ([]MarkResponse, error) {
	revIDs := make([]store.UUID, 0, len(marks))
	evidenceIDs := make([]store.UUID, 0, len(marks))
	pageIDs := make([]store.UUID, 0, len(marks))
	for _, mark := range marks {
		switch mark.TargetType {
		case store.RecheckTargetArtifactRevision:
			revIDs = append(revIDs, mark.TargetID)
		case store.RecheckTargetEvidence:
			evidenceIDs = append(evidenceIDs, mark.TargetID)
		case store.RecheckTargetWikiPage:
			pageIDs = append(pageIDs, mark.TargetID)
		}
	}

	revisions := map[store.UUID]store.ResearchArtifactRevision{}
	if len(revIDs) > 0 {
		var rows []store.ResearchArtifactRevision
		if err := s.DB.WithContext(ctx).Where("id IN ?", revIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			revisions[row.ID] = row
		}
	}
	artifactIDs := make([]store.UUID, 0, len(revisions))
	for _, rev := range revisions {
		artifactIDs = append(artifactIDs, rev.ArtifactID)
	}
	artifacts := map[store.UUID]store.ResearchArtifact{}
	if len(artifactIDs) > 0 {
		var rows []store.ResearchArtifact
		if err := s.DB.WithContext(ctx).Where("id IN ?", artifactIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			artifacts[row.ID] = row
		}
	}
	evidence := map[store.UUID]store.AgentEvidence{}
	if len(evidenceIDs) > 0 {
		var rows []store.AgentEvidence
		if err := s.DB.WithContext(ctx).Where("id IN ?", evidenceIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			evidence[row.ID] = row
		}
	}
	pages := map[store.UUID]wikiSummary{}
	if len(pageIDs) > 0 {
		var rows []struct {
			ID      store.UUID `gorm:"column:id"`
			Title   string     `gorm:"column:title"`
			StaleAt *time.Time `gorm:"column:stale_at"`
		}
		if err := s.DB.WithContext(ctx).Table("agent_wiki_pages").
			Select("id, title, stale_at").Where("id IN ?", pageIDs).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			pages[row.ID] = wikiSummary{Title: row.Title, StaleAt: row.StaleAt}
		}
	}

	out := make([]MarkResponse, 0, len(marks))
	for _, mark := range marks {
		item := MarkResponse{
			ID: mark.ID.UUID, ProjectID: mark.ProjectID.UUID,
			TargetType: mark.TargetType, TargetID: mark.TargetID.UUID,
			TriggerKind: mark.TriggerKind, TriggerLabel: triggerLabel(mark.TriggerKind),
			Reason: mark.Reason, CreatedAt: dto.JSONTime{Time: mark.CreatedAt},
			Resolution: mark.Resolution,
		}
		if mark.ArtifactID != nil {
			id := mark.ArtifactID.UUID
			item.ArtifactID = &id
		}
		item.Revision = mark.Revision
		if mark.ResolvedAt != nil {
			at := dto.JSONTime{Time: *mark.ResolvedAt}
			item.ResolvedAt = &at
		}
		describeTarget(&item, mark, revisions, artifacts, evidence, pages)
		out = append(out, item)
	}
	return out, nil
}

type wikiSummary struct {
	Title   string
	StaleAt *time.Time
}

func describeTarget(item *MarkResponse, mark store.ContentRecheckMark,
	revisions map[store.UUID]store.ResearchArtifactRevision,
	artifacts map[store.UUID]store.ResearchArtifact,
	evidence map[store.UUID]store.AgentEvidence,
	pages map[store.UUID]wikiSummary) {
	switch mark.TargetType {
	case store.RecheckTargetArtifactRevision:
		rev, ok := revisions[mark.TargetID]
		artifact, artifactOK := artifacts[rev.ArtifactID]
		if !ok || !artifactOK {
			markMissing(item)
			return
		}
		item.TargetTitle = artifactTargetTitle(artifact.Kind, rev.PayloadJSON)
		item.TargetDetail = fmt.Sprintf("修订 v%d", rev.Revision)
	case store.RecheckTargetEvidence:
		row, ok := evidence[mark.TargetID]
		if !ok {
			markMissing(item)
			return
		}
		item.TargetTitle = row.SourceName
		if item.TargetTitle == "" {
			item.TargetTitle = "资料证据"
		}
		detail := truncateText(stringValue(row.Quote), 120)
		if row.Locator != nil && *row.Locator != "" {
			detail = strings.TrimSpace(detail + "（" + *row.Locator + "）")
		}
		item.TargetDetail = detail
	case store.RecheckTargetWikiPage:
		page, ok := pages[mark.TargetID]
		if !ok {
			markMissing(item)
			return
		}
		item.TargetTitle = page.Title
		item.TargetDetail = "项目知识库页面"
	default:
		markMissing(item)
	}
}

func markMissing(item *MarkResponse) {
	item.TargetMissing = true
	item.TargetTitle = "目标已不存在"
	item.TargetDetail = ""
}

// artifactTargetTitle 与前端展示规则一致的服务端确定性摘要（不共享代码）。
func artifactTargetTitle(kind string, payload datatypes.JSON) string {
	var parsed struct {
		Title     string `json:"title"`
		Question  string `json:"question"`
		SectionID string `json:"sectionId"`
		Sections  []struct {
			ID string `json:"id"`
		} `json:"sections"`
	}
	_ = json.Unmarshal(payload, &parsed)
	switch kind {
	case store.ResearchArtifactLiteratureNote:
		if title := strings.TrimSpace(parsed.Title); title != "" {
			return title
		}
		return "阅读记录"
	case store.ResearchArtifactLiteratureComparison:
		if question := strings.TrimSpace(parsed.Question); question != "" {
			return question
		}
		return "文献比较"
	case store.ResearchArtifactChapterPlan:
		if len(parsed.Sections) > 0 {
			return fmt.Sprintf("章节计划（%d 节）", len(parsed.Sections))
		}
		return "章节计划"
	case store.ResearchArtifactChapterDigest:
		if id := strings.TrimSpace(parsed.SectionID); id != "" {
			return "章节摘要 · " + id
		}
		return "章节摘要"
	case store.ResearchArtifactChapterReview:
		return "跨章节检查报告"
	}
	return kind
}

func triggerLabel(kind string) string {
	switch kind {
	case store.RecheckTriggerSourceDeleted:
		return "资料已删除"
	case store.RecheckTriggerSourceNewVersion:
		return "资料出了新版本"
	}
	return kind
}

func truncateText(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func conflict(info string) error { return &httpapi.APIError{Code: http.StatusConflict, Info: info} }

// ── HTTP ────────────────────────────────────────────────────────────

func (s *Service) list(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := s.List(c.Request.Context(), auth.CurrentUser(c).ID, store.UUID{UUID: pid}, c.Query("status"), limit)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(items, ""))
}

func (s *Service) resolve(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return
	}
	mid, err := uuid.Parse(c.Param("mark_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusNotFound, Info: "待复查项不存在"})
		return
	}
	var req struct {
		Resolution string `json:"resolution"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	item, err := s.Resolve(c.Request.Context(), auth.CurrentUser(c).ID,
		store.UUID{UUID: pid}, store.UUID{UUID: mid}, req.Resolution)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(item, ""))
}

func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	p := rg.Group("/projects", authMw)
	p.GET("/:project_id/recheck-marks", s.list)
	p.POST("/:project_id/recheck-marks/:mark_id/resolve", s.resolve)
}
