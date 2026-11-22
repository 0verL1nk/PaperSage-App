package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/extract"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	extractorVersion = "text-v1"
	segmentBytes     = 4096
	maxReadBytes     = 1 << 20
)

type Service struct {
	DB      *gorm.DB
	Storage storage.Storage
}

func NewService(db *gorm.DB, stg storage.Storage) *Service { return &Service{DB: db, Storage: stg} }

func (s *Service) ownedVersion(ctx context.Context, uid, projectID, versionID store.UUID) (*store.SourceVersion, error) {
	var ver store.SourceVersion
	err := s.DB.WithContext(ctx).Model(&store.SourceVersion{}).
		Joins("JOIN project_sources ON project_sources.id = source_versions.source_id").
		Joins("JOIN projects ON projects.id = project_sources.project_id").
		Where("source_versions.id = ? AND project_sources.project_id = ? AND projects.owner_id = ?",
			versionID, projectID, uid).
		First(&ver).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpapi.NotFound("来源版本")
	}
	return &ver, err
}

// ExtractVersion 抽取并保存不可变文本与段落定位；重复调用会重建该版本的抽取结果。
func (s *Service) ExtractVersion(ctx context.Context, projectID, versionID store.UUID) error {
	var ver store.SourceVersion
	err := s.DB.WithContext(ctx).Model(&store.SourceVersion{}).
		Joins("JOIN project_sources ON project_sources.id = source_versions.source_id").
		Where("source_versions.id = ? AND project_sources.project_id = ?", versionID, projectID).
		First(&ver).Error
	if err != nil {
		return err
	}
	name, contentType := "source", "application/octet-stream"
	var doc store.Document
	if err := s.DB.WithContext(ctx).Where("source_id = ?", ver.SourceID).First(&doc).Error; err == nil {
		name, contentType = doc.Name, doc.Type
	}
	raw, err := s.Storage.Get(ctx, ver.S3Key)
	if err != nil {
		s.markFailed(ctx, ver.ID, err)
		return err
	}
	text, err := extract.Text(ctx, raw, name, contentType, 0)
	if err != nil {
		s.markFailed(ctx, ver.ID, err)
		return err
	}
	textBytes := []byte(text)
	textKey := ver.S3Key + ".txt"
	if err := s.Storage.Put(ctx, textKey, textBytes); err != nil {
		s.markFailed(ctx, ver.ID, err)
		return err
	}
	sum := sha256.Sum256(textBytes)
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ext := store.SourceExtraction{
			SourceVersionID: ver.ID, ExtractorVersion: extractorVersion,
			State: "ready", TextS3Key: &textKey, TextSHA256: ptr(hex.EncodeToString(sum[:])),
			TextBytes: int64(len(textBytes)),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "source_version_id"}, {Name: "extractor_version"}},
			DoUpdates: clause.AssignmentColumns([]string{"state", "text_s3_key", "text_sha256", "text_bytes", "error_json", "updated_at"}),
		}).Create(&ext).Error; err != nil {
			return err
		}
		if err := tx.Where("source_version_id = ? AND extractor_version = ?", ver.ID, extractorVersion).
			First(&ext).Error; err != nil {
			return err
		}
		if err := tx.Where("extraction_id = ?", ext.ID).Delete(&store.SourceSegment{}).Error; err != nil {
			return err
		}
		segments := make([]store.SourceSegment, 0, len(textBytes)/segmentBytes+1)
		for start := 0; start < len(textBytes); start += segmentBytes {
			end := start + segmentBytes
			if end > len(textBytes) {
				end = len(textBytes)
			}
			for end < len(textBytes) && !utf8.RuneStart(textBytes[end]) {
				end++
			}
			segments = append(segments, store.SourceSegment{
				ExtractionID: ext.ID, Ordinal: len(segments),
				StartByte: int64(start), EndByte: int64(end), LocatorKind: "text_range",
			})
		}
		if len(segments) > 0 {
			if err := tx.CreateInBatches(&segments, 200).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) markFailed(ctx context.Context, versionID store.UUID, cause error) {
	payload, _ := json.Marshal(map[string]string{"message": cause.Error()})
	ext := store.SourceExtraction{
		SourceVersionID: versionID, ExtractorVersion: extractorVersion,
		State: "failed", ErrorJSON: datatypes.JSON(payload),
	}
	_ = s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source_version_id"}, {Name: "extractor_version"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "error_json", "updated_at"}),
	}).Create(&ext).Error
}

type extractionResponse struct {
	State     string `json:"state"`
	TextBytes int64  `json:"textBytes"`
	Error     string `json:"error,omitempty"`
}

type versionResponse struct {
	ID         uuid.UUID           `json:"id"`
	SourceID   uuid.UUID           `json:"sourceId"`
	Revision   int64               `json:"revision"`
	SizeBytes  int64               `json:"sizeBytes"`
	ContentSHA string              `json:"contentSha256"`
	CreatedAt  string              `json:"createdAt"`
	Extraction *extractionResponse `json:"extraction,omitempty"`
}

func (s *Service) versionInfo(ctx context.Context, ver *store.SourceVersion) versionResponse {
	resp := versionResponse{
		ID: ver.ID.UUID, SourceID: ver.SourceID.UUID, Revision: ver.Revision,
		SizeBytes: ver.SizeBytes, ContentSHA: ver.ContentSHA256,
		CreatedAt: ver.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	var ext store.SourceExtraction
	if err := s.DB.WithContext(ctx).Where("source_version_id = ? AND extractor_version = ?", ver.ID, extractorVersion).
		First(&ext).Error; err == nil {
		e := extractionResponse{State: ext.State, TextBytes: ext.TextBytes}
		if len(ext.ErrorJSON) > 0 {
			var payload map[string]string
			if json.Unmarshal(ext.ErrorJSON, &payload) == nil {
				e.Error = payload["message"]
			}
		}
		resp.Extraction = &e
	}
	return resp
}

// ReadText 返回指定字节区间；offset 会对齐到 UTF-8 字符边界，limit 有硬上限。
func (s *Service) ReadText(ctx context.Context, uid, projectID, versionID store.UUID, offset int64, limit int) (string, int64, bool, error) {
	ver, err := s.ownedVersion(ctx, uid, projectID, versionID)
	if err != nil {
		return "", 0, false, err
	}
	var ext store.SourceExtraction
	if err := s.DB.WithContext(ctx).Where("source_version_id = ? AND extractor_version = ?", ver.ID, extractorVersion).
		First(&ext).Error; err != nil || ext.State != "ready" || ext.TextS3Key == nil {
		return "", 0, false, httpapi.BadRequest("该来源尚未完成文本抽取")
	}
	if limit <= 0 || limit > maxReadBytes {
		limit = maxReadBytes
	}
	text, err := s.Storage.Get(ctx, *ext.TextS3Key)
	if err != nil {
		return "", 0, false, err
	}
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(text)) {
		offset = int64(len(text))
	}
	for offset < int64(len(text)) && !utf8.RuneStart(text[offset]) {
		offset--
	}
	end := offset + int64(limit)
	if end > int64(len(text)) {
		end = int64(len(text))
	}
	for end < int64(len(text)) && !utf8.RuneStart(text[end]) {
		end++
	}
	return string(text[offset:end]), end, end == int64(len(text)), nil
}

func ptr[T any](v T) *T { return &v }

// ── HTTP ────────────────────────────────────────────────────────────

func (s *Service) getVersion(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return
	}
	vid, err := uuid.Parse(c.Param("version_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("来源版本"))
		return
	}
	ver, err := s.ownedVersion(c.Request.Context(), auth.CurrentUser(c).ID, store.UUID{UUID: pid}, store.UUID{UUID: vid})
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(s.versionInfo(c.Request.Context(), ver), ""))
}

func (s *Service) readText(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("项目"))
		return
	}
	vid, err := uuid.Parse(c.Param("version_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("来源版本"))
		return
	}
	offset, _ := strconv.ParseInt(c.Query("offset"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	text, next, done, err := s.ReadText(c.Request.Context(), auth.CurrentUser(c).ID,
		store.UUID{UUID: pid}, store.UUID{UUID: vid}, offset, limit)
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{
		"text": text, "nextOffset": next, "done": done,
	}, ""))
}

func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	p := rg.Group("/projects", authMw)
	p.GET("/:project_id/source-versions/:version_id", s.getVersion)
	p.GET("/:project_id/source-versions/:version_id/text", s.readText)
}
