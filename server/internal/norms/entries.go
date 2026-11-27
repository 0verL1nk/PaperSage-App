package norms

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type entryFilter struct {
	Query    string
	Category string
	Scenario string
	Tag      string
	PackID   string
	Binding  string
	State    string
	Page     int
	PageSize int
}

func parseUUID(raw string) (store.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return store.UUID{}, false
	}
	return store.UUID{UUID: id}, true
}

// entryQuery 每次重新构造：Count 与分页查询各用一份，避免 Statement 互相污染。
func (s *Service) entryQuery(ctx context.Context, uid store.UUID, f entryFilter) *gorm.DB {
	q := s.DB.WithContext(ctx).Model(&store.NormEntry{}).
		Joins("JOIN norm_packs ON norm_packs.id = norm_entries.pack_id AND norm_packs.deleted_at IS NULL").
		Joins("JOIN norm_versions ON norm_versions.entry_id = norm_entries.id AND norm_versions.version = norm_entries.current_version").
		Where("(norm_packs.is_builtin = ? OR norm_packs.owner_id = ?)", true, uid).
		Where("norm_packs.state = ?", store.NormStateActive)
	state := f.State
	if state == "" {
		state = store.NormStateActive
	}
	q = q.Where("norm_entries.state = ?", state)
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where(`(norm_entries.code LIKE ? OR norm_versions.title LIKE ?
			OR norm_versions.rule_text LIKE ? OR norm_versions.source_ref LIKE ?)`,
			like, like, like, like)
	}
	if f.Category != "" {
		q = q.Where("norm_entries.category = ?", f.Category)
	}
	if f.Scenario != "" {
		q = q.Where("(norm_entries.scenario = ? OR norm_entries.scenario = ?)", f.Scenario, store.NormScenarioAll)
	}
	if f.Binding != "" {
		q = q.Where("norm_entries.binding = ?", f.Binding)
	}
	if f.PackID != "" {
		if id, ok := parseUUID(f.PackID); ok {
			q = q.Where("norm_entries.pack_id = ?", id)
		} else {
			q = q.Where("1 = 0")
		}
	}
	if f.Tag != "" {
		q = q.Where(`EXISTS (SELECT 1 FROM norm_entry_tags
			WHERE norm_entry_tags.entry_id = norm_entries.id AND norm_entry_tags.tag = ?)`, f.Tag)
	}
	return q
}

func (s *Service) SearchEntries(ctx context.Context, uid store.UUID, f entryFilter) (*entryPage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = defaultPageSize
	}
	if f.PageSize > maxPageSize {
		f.PageSize = maxPageSize
	}
	var total int64
	if err := s.entryQuery(ctx, uid, f).Count(&total).Error; err != nil {
		return nil, err
	}
	ids := []store.UUID{}
	if total > 0 {
		if err := s.entryQuery(ctx, uid, f).
			Order("norm_entries.sort_order ASC, norm_entries.code ASC").
			Offset((f.Page-1)*f.PageSize).Limit(f.PageSize).
			Pluck("norm_entries.id", &ids).Error; err != nil {
			return nil, err
		}
	}
	rows, err := s.loadEntryRows(ctx, uid, ids)
	if err != nil {
		return nil, err
	}
	tags, err := s.loadTags(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]entrySummary, 0, len(ids))
	for _, id := range ids {
		row, ok := rows[id]
		if !ok {
			continue
		}
		items = append(items, toEntrySummary(row, tags[id]))
	}
	return &entryPage{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

func (s *Service) GetEntry(ctx context.Context, uid, entryID store.UUID) (*entryDetailResponse, error) {
	rows, err := s.loadEntryRows(ctx, uid, []store.UUID{entryID})
	if err != nil {
		return nil, err
	}
	row, ok := rows[entryID]
	if !ok {
		return nil, httpapi.NotFound("Norm entry")
	}
	tags, err := s.loadTags(ctx, []store.UUID{entryID})
	if err != nil {
		return nil, err
	}
	var versions []store.NormVersion
	if err := s.DB.WithContext(ctx).Where("entry_id = ?", entryID).Order("version ASC").Find(&versions).Error; err != nil {
		return nil, err
	}
	resp := &entryDetailResponse{
		entrySummary: toEntrySummary(row, tags[entryID]),
		Rationale:    row.Version.Rationale,
		Examples:     decodeExamples(row.Version.Examples),
		Versions:     make([]versionResponse, 0, len(versions)),
	}
	for _, v := range versions {
		resp.Versions = append(resp.Versions, toVersionResponse(v))
	}
	return resp, nil
}

type entryMeta struct {
	Category string
	RuleKind string
	Binding  string
	Scenario string
}

func normalizeEntryMeta(req entryRequest, fallbackScenario string) (entryMeta, error) {
	meta := entryMeta{
		Category: strings.TrimSpace(req.Category),
		RuleKind: strings.TrimSpace(req.RuleKind),
		Binding:  strings.TrimSpace(req.Binding),
		Scenario: strings.TrimSpace(req.Scenario),
	}
	if meta.Category == "" {
		return entryMeta{}, httpapi.BadRequest("规范条目必须填写分类")
	}
	if len([]rune(meta.Category)) > 32 {
		return entryMeta{}, httpapi.BadRequest("分类不能超过 32 个字符")
	}
	if meta.RuleKind == "" {
		meta.RuleKind = store.NormRuleKindGeneral
	}
	if !ruleKinds[meta.RuleKind] {
		return entryMeta{}, httpapi.BadRequest("规范类型不合法")
	}
	if meta.Binding == "" {
		meta.Binding = store.NormBindingShould
	}
	if !bindings[meta.Binding] {
		return entryMeta{}, httpapi.BadRequest("强制程度不合法")
	}
	if meta.Scenario == "" {
		meta.Scenario = fallbackScenario
	}
	if !scenarios[meta.Scenario] {
		return entryMeta{}, httpapi.BadRequest("适用场景不合法")
	}
	return meta, nil
}

func createTags(tx *gorm.DB, entryID store.UUID, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	rows := make([]store.NormEntryTag, 0, len(tags))
	for _, tag := range tags {
		rows = append(rows, store.NormEntryTag{EntryID: entryID, Tag: tag})
	}
	return tx.Create(&rows).Error
}

func (s *Service) CreateEntry(ctx context.Context, uid store.UUID, req entryRequest) (*entryDetailResponse, error) {
	packID, ok := parseUUID(req.PackID)
	if !ok {
		return nil, httpapi.BadRequest("请选择要新建条目的规范包")
	}
	pack, err := s.loadWritablePack(ctx, uid, packID)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if !entryCodePattern.MatchString(code) {
		return nil, httpapi.BadRequest("条目编号只能用字母、数字、点、下划线和连字符，2—60 位且以字母或数字开头")
	}
	meta, err := normalizeEntryMeta(req, pack.Scenario)
	if err != nil {
		return nil, err
	}
	tags, err := normalizeList(req.Tags, maxTagsPerEntry, "标签")
	if err != nil {
		return nil, err
	}
	version, err := buildVersion(req, 1, store.NormOriginUser, &uid)
	if err != nil {
		return nil, err
	}
	entry := store.NormEntry{
		PackID: pack.ID, Code: code, Category: meta.Category, RuleKind: meta.RuleKind,
		Binding: meta.Binding, Scenario: meta.Scenario, SortOrder: req.SortOrder,
		CurrentVersion: 1, State: store.NormStateActive,
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return conflict("该规范包内已存在同编号条目，请换一个编号")
		}
		version.EntryID = entry.ID
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		return createTags(tx, entry.ID, tags)
	})
	if err != nil {
		return nil, err
	}
	return s.GetEntry(ctx, uid, entry.ID)
}

func (s *Service) UpdateEntry(ctx context.Context, uid, entryID store.UUID, req entryUpdateRequest) (*entryDetailResponse, error) {
	row, err := s.loadEntryForWrite(ctx, uid, entryID)
	if err != nil {
		return nil, err
	}
	if req.ExpectedVersion <= 0 {
		return nil, httpapi.BadRequest("请提供当前版本号（expectedVersion），以避免覆盖他人的修改")
	}
	if req.ExpectedVersion != row.Entry.CurrentVersion {
		return nil, conflict("该条目已被修改，请刷新后重试")
	}
	meta, err := normalizeEntryMeta(req.entryRequest, row.Entry.Scenario)
	if err != nil {
		return nil, err
	}
	tags, err := normalizeList(req.Tags, maxTagsPerEntry, "标签")
	if err != nil {
		return nil, err
	}
	state := row.Entry.State
	if req.State != nil {
		if !packStates[*req.State] {
			return nil, httpapi.BadRequest("规范条目状态不合法")
		}
		state = *req.State
	}
	version, err := buildVersion(req.entryRequest, req.ExpectedVersion+1, store.NormOriginUser, &uid)
	if err != nil {
		return nil, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&store.NormEntry{}).
			Where("id = ? AND current_version = ?", row.Entry.ID, req.ExpectedVersion).
			Updates(map[string]any{
				"current_version": req.ExpectedVersion + 1,
				"category":        meta.Category,
				"rule_kind":       meta.RuleKind,
				"binding":         meta.Binding,
				"scenario":        meta.Scenario,
				"sort_order":      req.SortOrder,
				"state":           state,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return conflict("该条目已被修改，请刷新后重试")
		}
		version.EntryID = row.Entry.ID
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		// 标签是派生的，随内容重建；先删后建避免主键冲突。
		if err := tx.Where("entry_id = ?", row.Entry.ID).Delete(&store.NormEntryTag{}).Error; err != nil {
			return err
		}
		return createTags(tx, row.Entry.ID, tags)
	})
	if err != nil {
		return nil, err
	}
	return s.GetEntry(ctx, uid, row.Entry.ID)
}

func (s *Service) DeleteEntry(ctx context.Context, uid, entryID store.UUID) error {
	row, err := s.loadEntryForWrite(ctx, uid, entryID)
	if err != nil {
		return err
	}
	names, err := s.projectsUsingEntries(ctx, []store.UUID{row.Entry.ID})
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return conflict(inUseMessage("规范条目", names))
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&store.NormEntry{SoftDeleteModel: store.SoftDeleteModel{ID: row.Entry.ID}}).Error; err != nil {
			return err
		}
		return tx.Where("entry_id = ?", row.Entry.ID).Delete(&store.NormEntryTag{}).Error
	})
}

func (s *Service) loadEntryForWrite(ctx context.Context, uid, entryID store.UUID) (entryRow, error) {
	rows, err := s.loadEntryRows(ctx, uid, []store.UUID{entryID})
	if err != nil {
		return entryRow{}, err
	}
	row, ok := rows[entryID]
	if !ok {
		return entryRow{}, httpapi.NotFound("Norm entry")
	}
	if row.Pack.IsBuiltin {
		return entryRow{}, httpapi.Forbidden("内置规范条目只读，请复制为自建包后再修改")
	}
	if row.Pack.OwnerID == nil || *row.Pack.OwnerID != uid {
		return entryRow{}, httpapi.NotFound("Norm entry")
	}
	return row, nil
}
