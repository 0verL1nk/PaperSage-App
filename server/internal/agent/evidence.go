package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// persistRunEvidence records the exact chunks returned to the model. A chunk's
// version is its text digest, not a claim about the full source document.
func persistRunEvidence(ctx context.Context, db *gorm.DB, runID store.UUID, hits []rag.Hit) ([]store.AgentEvidence, error) {
	if len(hits) == 0 {
		return []store.AgentEvidence{}, nil
	}

	rows := make([]store.AgentEvidence, 0, len(hits))
	seen := make(map[store.UUID]struct{}, len(hits))
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, hit := range hits {
			sourceUUID, err := uuid.Parse(hit.SourceID)
			if err != nil {
				return fmt.Errorf("invalid evidence source id: %w", err)
			}
			if strings.TrimSpace(hit.SourceType) == "" || strings.TrimSpace(hit.ChunkID) == "" {
				return fmt.Errorf("evidence source type and chunk id are required")
			}

			digest := sha256.Sum256([]byte(hit.Content))
			version := hex.EncodeToString(digest[:])
			chunkID := hit.ChunkID
			locator := chunkLocator(chunkID)
			evidence := store.AgentEvidence{
				ID: store.MustNewUUID(), RunID: runID,
				SourceType: hit.SourceType, SourceID: store.UUID{UUID: sourceUUID},
				SourceVersion: &version, ChunkID: &chunkID, Locator: &locator, Quote: &hit.Content,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "run_id"}, {Name: "chunk_id"}, {Name: "source_version"}},
				DoNothing: true,
			}).Create(&evidence).Error; err != nil {
				return fmt.Errorf("persist run evidence: %w", err)
			}

			var saved store.AgentEvidence
			if err := tx.Where("run_id = ? AND chunk_id = ? AND source_version = ? AND source_type = ? AND source_id = ?",
				runID, chunkID, version, hit.SourceType, store.UUID{UUID: sourceUUID}).First(&saved).Error; err != nil {
				return fmt.Errorf("read persisted run evidence: %w", err)
			}
			if _, ok := seen[saved.ID]; !ok {
				rows = append(rows, saved)
				seen[saved.ID] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func chunkLocator(chunkID string) string {
	last := strings.LastIndexByte(chunkID, ':')
	if last >= 0 {
		if index, err := strconv.Atoi(chunkID[last+1:]); err == nil && index >= 0 {
			return fmt.Sprintf("chunk:%d", index)
		}
	}
	return "chunk:" + chunkID
}

type runEvidenceDTO struct {
	ID            string `json:"id"`
	SourceType    string `json:"sourceType"`
	SourceID      string `json:"sourceId"`
	SourceVersion string `json:"sourceVersion"`
	ChunkID       string `json:"chunkId"`
	Locator       string `json:"locator"`
	Quote         string `json:"quote"`
}

// listRunEvidence GET /agent/runs/:run_id/evidence. Evidence is only visible
// through the same conversation ownership boundary used by Run event queries.
func (s *Service) listRunEvidence(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	rid, err := parseUUIDParam(c.Param("run_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	runID := store.UUID{UUID: rid}
	_, projectID, err := s.ownedRun(ctx, runID, uid)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Run not found"})
		return
	}
	var projectCount int64
	if err := s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", projectID, uid).Count(&projectCount).Error; err != nil || projectCount == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Run not found"})
		return
	}

	var evidence []store.AgentEvidence
	if err := s.DB.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("created_at ASC, source_type ASC, source_id ASC, chunk_id ASC, source_version ASC").
		Find(&evidence).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	out := make([]runEvidenceDTO, 0, len(evidence))
	for _, row := range evidence {
		item := runEvidenceDTO{
			ID: row.ID.String(), SourceType: row.SourceType, SourceID: row.SourceID.String(),
		}
		if row.SourceVersion != nil {
			item.SourceVersion = *row.SourceVersion
		}
		if row.ChunkID != nil {
			item.ChunkID = *row.ChunkID
		}
		if row.Locator != nil {
			item.Locator = *row.Locator
		}
		if row.Quote != nil {
			item.Quote = *row.Quote
		}
		out = append(out, item)
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}
