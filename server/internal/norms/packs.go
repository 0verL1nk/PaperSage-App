package norms

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func (s *Service) ListPacks(ctx context.Context, uid store.UUID) ([]packResponse, error) {
	var packs []store.NormPack
	if err := visiblePacks(s.DB.WithContext(ctx), uid).
		Order("is_builtin DESC, name ASC").Find(&packs).Error; err != nil {
		return nil, err
	}
	counts := map[store.UUID]int64{}
	if len(packs) > 0 {
		ids := make([]store.UUID, 0, len(packs))
		for _, p := range packs {
			ids = append(ids, p.ID)
		}
		var rows []struct {
			PackID store.UUID `gorm:"column:pack_id"`
			Total  int64      `gorm:"column:total"`
		}
		if err := s.DB.WithContext(ctx).Model(&store.NormEntry{}).
			Select("pack_id, count(*) AS total").Where("pack_id IN ?", ids).
			Group("pack_id").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			counts[r.PackID] = r.Total
		}
	}
	out := make([]packResponse, 0, len(packs))
	for _, p := range packs {
		out = append(out, toPackResponse(p, counts[p.ID]))
	}
	return out, nil
}

func (s *Service) CreatePack(ctx context.Context, uid store.UUID, req packRequest) (*packResponse, error) {
	key := strings.TrimSpace(req.Key)
	if !packKeyPattern.MatchString(key) {
		return nil, httpapi.BadRequest("规范包标识只能用小写字母、数字和连字符，3—80 位且以字母或数字开头")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, httpapi.BadRequest("规范包必须填写名称")
	}
	if len([]rune(name)) > 120 {
		return nil, httpapi.BadRequest("规范包名称不能超过 120 个字符")
	}
	authority := strings.TrimSpace(req.Authority)
	if authority == "" {
		authority = store.NormAuthorityUser
	}
	if !authorities[authority] {
		return nil, httpapi.BadRequest("规范包来源类型不合法")
	}
	scenario := strings.TrimSpace(req.Scenario)
	if scenario == "" {
		scenario = store.NormScenarioAll
	}
	if !scenarios[scenario] {
		return nil, httpapi.BadRequest("适用场景不合法")
	}
	edition := strings.TrimSpace(req.Edition)
	if len([]rune(edition)) > 40 {
		return nil, httpapi.BadRequest("版本说明不能超过 40 个字符")
	}
	sourceURL := strings.TrimSpace(req.SourceURL)
	if len(sourceURL) > 500 {
		return nil, httpapi.BadRequest("来源链接不能超过 500 个字符")
	}
	sourceNote := strings.TrimSpace(req.SourceNote)

	owner := uid
	pack := store.NormPack{
		Key: key, Name: name, Authority: authority, Edition: edition,
		Scenario: scenario, IsBuiltin: false, OwnerID: &owner, State: store.NormStateActive,
	}
	if sourceURL != "" {
		pack.SourceURL = &sourceURL
	}
	if sourceNote != "" {
		pack.SourceNote = &sourceNote
	}
	res := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&pack)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, conflict("规范包标识已被占用，请换一个")
	}
	resp := toPackResponse(pack, 0)
	return &resp, nil
}

func (s *Service) UpdatePack(ctx context.Context, uid, packID store.UUID, req packPatch) (*packResponse, error) {
	pack, err := s.loadWritablePack(ctx, uid, packID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, httpapi.BadRequest("规范包必须填写名称")
		}
		if len([]rune(name)) > 120 {
			return nil, httpapi.BadRequest("规范包名称不能超过 120 个字符")
		}
		updates["name"] = name
	}
	if req.Authority != nil {
		if !authorities[*req.Authority] {
			return nil, httpapi.BadRequest("规范包来源类型不合法")
		}
		updates["authority"] = *req.Authority
	}
	if req.Scenario != nil {
		if !scenarios[*req.Scenario] {
			return nil, httpapi.BadRequest("适用场景不合法")
		}
		updates["scenario"] = *req.Scenario
	}
	if req.State != nil {
		if !packStates[*req.State] {
			return nil, httpapi.BadRequest("规范包状态不合法")
		}
		updates["state"] = *req.State
	}
	if req.Edition != nil {
		edition := strings.TrimSpace(*req.Edition)
		if len([]rune(edition)) > 40 {
			return nil, httpapi.BadRequest("版本说明不能超过 40 个字符")
		}
		updates["edition"] = edition
	}
	if req.SourceURL != nil {
		v := strings.TrimSpace(*req.SourceURL)
		if len(v) > 500 {
			return nil, httpapi.BadRequest("来源链接不能超过 500 个字符")
		}
		if v == "" {
			updates["source_url"] = nil
		} else {
			updates["source_url"] = v
		}
	}
	if req.SourceNote != nil {
		v := strings.TrimSpace(*req.SourceNote)
		if v == "" {
			updates["source_note"] = nil
		} else {
			updates["source_note"] = v
		}
	}
	if len(updates) > 0 {
		if err := s.DB.WithContext(ctx).Model(&store.NormPack{}).
			Where("id = ?", pack.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	var entryCount int64
	if err := s.DB.WithContext(ctx).Model(&store.NormEntry{}).
		Where("pack_id = ?", pack.ID).Count(&entryCount).Error; err != nil {
		return nil, err
	}
	fresh, err := s.loadVisiblePack(ctx, uid, pack.ID)
	if err != nil {
		return nil, err
	}
	resp := toPackResponse(*fresh, entryCount)
	return &resp, nil
}

// DeletePack 软删包并显式软删其条目（软删不触发级联）。
// 有任何条目被项目采用时拒绝删除，避免项目里的约束凭空消失。
func (s *Service) DeletePack(ctx context.Context, uid, packID store.UUID) error {
	pack, err := s.loadWritablePack(ctx, uid, packID)
	if err != nil {
		return err
	}
	var entryIDs []store.UUID
	if err := s.DB.WithContext(ctx).Model(&store.NormEntry{}).
		Where("pack_id = ?", pack.ID).Pluck("id", &entryIDs).Error; err != nil {
		return err
	}
	if len(entryIDs) > 0 {
		names, err := s.projectsUsingEntries(ctx, entryIDs)
		if err != nil {
			return err
		}
		if len(names) > 0 {
			return conflict(inUseMessage("规范包", names))
		}
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(entryIDs) > 0 {
			if err := tx.Where("pack_id = ?", pack.ID).Delete(&store.NormEntry{}).Error; err != nil {
				return err
			}
			if err := tx.Where("entry_id IN ?", entryIDs).Delete(&store.NormEntryTag{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(pack).Error
	})
}

// projectsUsingEntries 返回正在采用这些条目的项目名（含已进回收站的项目，
// 否则删除后项目里会留下孤立采用）。
func (s *Service) projectsUsingEntries(ctx context.Context, entryIDs []store.UUID) ([]string, error) {
	names := []string{}
	if len(entryIDs) == 0 {
		return names, nil
	}
	err := s.DB.WithContext(ctx).Table("projects").
		Joins("JOIN project_norm_adoptions ON project_norm_adoptions.project_id = projects.id").
		Where("project_norm_adoptions.entry_id IN ? AND project_norm_adoptions.deleted_at IS NULL", entryIDs).
		Distinct().Order("projects.name").Limit(3).Pluck("projects.name", &names).Error
	return names, err
}

func inUseMessage(what string, names []string) string {
	return "该" + what + "已被项目「" + strings.Join(names, "」「") + "」采用，请先在项目中取消采用"
}
