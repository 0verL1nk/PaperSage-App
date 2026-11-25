package norms

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func (s *Service) ListProjectNorms(ctx context.Context, uid, projectID store.UUID) ([]adoptionResponse, error) {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return nil, err
	}
	var adoptions []store.ProjectNormAdoption
	if err := s.DB.WithContext(ctx).Where("project_id = ?", projectID).
		Order("created_at ASC, id ASC").Find(&adoptions).Error; err != nil {
		return nil, err
	}
	return s.adoptionResponses(ctx, uid, adoptions)
}

func (s *Service) adoptionResponses(ctx context.Context, uid store.UUID, adoptions []store.ProjectNormAdoption) ([]adoptionResponse, error) {
	ids := make([]store.UUID, 0, len(adoptions))
	for _, a := range adoptions {
		ids = append(ids, a.EntryID)
	}
	rows, err := s.loadEntryRows(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	tags, err := s.loadTags(ctx, ids)
	if err != nil {
		return nil, err
	}
	// 采用响应的内容必须来自被锁定的版本；loadEntryRows 给的是当前版本，
	// 直接把当前内容展示成「本项目生效的规范」会让用户误以为改稿已生效。
	lockedVersions, err := s.loadVersions(ctx, adoptions)
	if err != nil {
		return nil, err
	}
	out := make([]adoptionResponse, 0, len(adoptions))
	for _, a := range adoptions {
		row, ok := rows[a.EntryID]
		if !ok {
			continue
		}
		if locked, ok := lockedVersions[a.EntryID]; ok {
			row.Version = locked
		}
		resp := adoptionResponse{
			EntryID: a.EntryID.String(), Version: a.Version, State: a.State,
			LatestVersion:   row.Entry.CurrentVersion,
			HasNewerVersion: row.Entry.CurrentVersion > a.Version,
			CreatedAt:       dto.JSONTime{Time: a.CreatedAt}, UpdatedAt: dto.JSONTime{Time: a.UpdatedAt},
			Entry: toEntrySummary(row, tags[a.EntryID]),
		}
		if a.OverrideText != nil {
			resp.OverrideText = *a.OverrideText
		}
		out = append(out, resp)
	}
	return out, nil
}

// loadVersions 按每条采用记录锁定的版本号精确取内容，供采用列表展示。
func (s *Service) loadVersions(ctx context.Context, adoptions []store.ProjectNormAdoption) (map[store.UUID]store.NormVersion, error) {
	out := map[store.UUID]store.NormVersion{}
	if len(adoptions) == 0 {
		return out, nil
	}
	ids := make([]store.UUID, 0, len(adoptions))
	versions := make([]int, 0, len(adoptions))
	pinned := map[store.UUID]int{}
	for _, a := range adoptions {
		ids = append(ids, a.EntryID)
		versions = append(versions, a.Version)
		pinned[a.EntryID] = a.Version
	}
	var rows []store.NormVersion
	if err := s.DB.WithContext(ctx).Where("entry_id IN ? AND version IN ?", ids, versions).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, v := range rows {
		if pinned[v.EntryID] == v.Version {
			out[v.EntryID] = v
		}
	}
	return out, nil
}

func (s *Service) findAdoption(ctx context.Context, projectID, entryID store.UUID) (*store.ProjectNormAdoption, error) {
	var adoption store.ProjectNormAdoption
	err := s.DB.WithContext(ctx).Where("project_id = ? AND entry_id = ?", projectID, entryID).First(&adoption).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpapi.NotFound("已采用的规范")
	}
	if err != nil {
		return nil, err
	}
	return &adoption, nil
}

// Adopt 批量采用：可指定条目集合或整个规范包。已采用的条目跳过而不是重复插入；
// 已停用的条目会被重新启用（用户点「采用」的意图就是让它生效）。
func (s *Service) Adopt(ctx context.Context, uid, projectID store.UUID, req adoptRequest) ([]adoptionResponse, error) {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return nil, err
	}
	targets := []store.UUID{}
	if strings.TrimSpace(req.PackID) != "" {
		packID, ok := parseUUID(req.PackID)
		if !ok {
			return nil, httpapi.BadRequest("规范包不存在")
		}
		pack, err := s.loadVisiblePack(ctx, uid, packID)
		if err != nil {
			return nil, err
		}
		if pack.State != store.NormStateActive {
			return nil, httpapi.BadRequest("规范包已归档，无法采用")
		}
		if err := s.DB.WithContext(ctx).Model(&store.NormEntry{}).
			Where("pack_id = ? AND state = ?", pack.ID, store.NormStateActive).
			Order("sort_order ASC, code ASC").Pluck("id", &targets).Error; err != nil {
			return nil, err
		}
	} else {
		if len(req.EntryIDs) == 0 {
			return nil, httpapi.BadRequest("请选择要采用的规范条目或规范包")
		}
		if len(req.EntryIDs) > maxAdoptBatch {
			return nil, httpapi.BadRequest("一次最多采用 200 条规范")
		}
		seen := map[store.UUID]bool{}
		for _, raw := range req.EntryIDs {
			id, ok := parseUUID(raw)
			if !ok {
				return nil, httpapi.BadRequest("规范条目不存在")
			}
			if !seen[id] {
				seen[id] = true
				targets = append(targets, id)
			}
		}
	}
	if len(targets) == 0 {
		return nil, httpapi.BadRequest("该规范包没有可采用的条目")
	}
	rows, err := s.loadEntryRows(ctx, uid, targets)
	if err != nil {
		return nil, err
	}
	for _, id := range targets {
		row, ok := rows[id]
		if !ok {
			return nil, httpapi.BadRequest("部分规范条目不存在或无权访问")
		}
		if row.Entry.State != store.NormStateActive {
			return nil, httpapi.BadRequest("规范条目「" + row.Version.Title + "」已归档，无法采用")
		}
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range targets {
			var existing store.ProjectNormAdoption
			err := tx.Where("project_id = ? AND entry_id = ?", projectID, id).First(&existing).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				adoption := store.ProjectNormAdoption{
					ProjectID: projectID, EntryID: id, Version: rows[id].Entry.CurrentVersion,
					State: store.NormAdoptionEnabled, AdoptedBy: uid,
				}
				res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&adoption)
				if res.Error != nil {
					return res.Error
				}
			case err != nil:
				return err
			case existing.State != store.NormAdoptionEnabled:
				if err := tx.Model(&store.ProjectNormAdoption{}).Where("id = ?", existing.ID).
					Update("state", store.NormAdoptionEnabled).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListProjectNorms(ctx, uid, projectID)
}

func (s *Service) UpdateAdoption(ctx context.Context, uid, projectID, entryID store.UUID, req adoptionPatch) (*adoptionResponse, error) {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return nil, err
	}
	adoption, err := s.findAdoption(ctx, projectID, entryID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if req.State != nil {
		if !adoptionStates[*req.State] {
			return nil, httpapi.BadRequest("规范启用状态不合法")
		}
		updates["state"] = *req.State
	}
	if req.OverrideText != nil {
		text := strings.TrimSpace(*req.OverrideText)
		if len([]rune(text)) > 2000 {
			return nil, httpapi.BadRequest("本项目改写的规范不能超过 2000 个字符")
		}
		if text == "" {
			updates["override_text"] = nil
			updates["override_updated_at"] = nil
		} else {
			updates["override_text"] = text
			updates["override_updated_at"] = time.Now().UTC()
		}
	}
	if len(updates) > 0 {
		if err := s.DB.WithContext(ctx).Model(&store.ProjectNormAdoption{}).
			Where("id = ?", adoption.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	fresh, err := s.findAdoption(ctx, projectID, entryID)
	if err != nil {
		return nil, err
	}
	list, err := s.adoptionResponses(ctx, uid, []store.ProjectNormAdoption{*fresh})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, httpapi.NotFound("已采用的规范")
	}
	return &list[0], nil
}

func (s *Service) Unadopt(ctx context.Context, uid, projectID, entryID store.UUID) error {
	if err := s.ownsProject(ctx, uid, projectID); err != nil {
		return err
	}
	res := s.DB.WithContext(ctx).
		Where("project_id = ? AND entry_id = ?", projectID, entryID).
		Delete(&store.ProjectNormAdoption{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpapi.NotFound("已采用的规范")
	}
	return nil
}
