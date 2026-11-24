// Package norms 学术规范资源库：规范包与条目的增删改查、项目采用，以及
// 「项目已采用规范」的只读 Provider（供 AI 写作建议/跨章节检查注入）。
//
// 红线：本包只做存储、结构校验与归属校验，不做任何「内容是否符合规范」的
// 语义判断——那是真实大模型的工作。任何以关键词/正则推断规范符合度的代码
// 都不允许出现在这里。
package norms

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// ── 校验集合 ────────────────────────────────────────────────────────

var (
	packKeyPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,79}$`)
	entryCodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,59}$`)

	authorities = map[string]bool{
		store.NormAuthoritySchool: true, store.NormAuthorityJournal: true,
		store.NormAuthorityStandard: true, store.NormAuthorityGeneral: true,
		store.NormAuthorityUser: true,
	}
	scenarios = map[string]bool{
		store.NormScenarioAll: true, store.NormScenarioUndergraduateThesis: true,
		store.NormScenarioIntroPaper: true,
	}
	ruleKinds = map[string]bool{
		store.NormRuleKindOfficial: true, store.NormRuleKindGeneral: true,
		store.NormRuleKindUser: true, store.NormRuleKindModel: true,
	}
	bindings = map[string]bool{
		store.NormBindingMust: true, store.NormBindingShould: true,
		store.NormBindingRecommended: true,
	}
	packStates     = map[string]bool{store.NormStateActive: true, store.NormStateArchived: true}
	adoptionStates = map[string]bool{store.NormAdoptionEnabled: true, store.NormAdoptionDisabled: true}
	exampleKinds   = map[string]bool{"good": true, "bad": true}
)

const (
	maxTagsPerEntry     = 20
	maxExamplesPerEntry = 20
	maxSectionsPerEntry = 20
	maxAdoptBatch       = 200
	defaultPageSize     = 20
	maxPageSize         = 100
)

func conflict(info string) error { return &httpapi.APIError{Code: http.StatusConflict, Info: info} }

// ── 请求/响应契约（camelCase，与前端既有 DTO 一致）──────────────────

type Example struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Note string `json:"note,omitempty"`
}

type packRequest struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	Authority  string `json:"authority"`
	Edition    string `json:"edition"`
	SourceURL  string `json:"sourceUrl"`
	SourceNote string `json:"sourceNote"`
	Scenario   string `json:"scenario"`
}

type packPatch struct {
	Name       *string `json:"name"`
	Authority  *string `json:"authority"`
	Edition    *string `json:"edition"`
	SourceURL  *string `json:"sourceUrl"`
	SourceNote *string `json:"sourceNote"`
	Scenario   *string `json:"scenario"`
	State      *string `json:"state"`
}

type entryRequest struct {
	PackID             string    `json:"packId"`
	Code               string    `json:"code"`
	Category           string    `json:"category"`
	RuleKind           string    `json:"ruleKind"`
	Binding            string    `json:"binding"`
	Scenario           string    `json:"scenario"`
	SortOrder          int       `json:"sortOrder"`
	Tags               []string  `json:"tags"`
	Title              string    `json:"title"`
	RuleText           string    `json:"ruleText"`
	Rationale          string    `json:"rationale"`
	Examples           []Example `json:"examples"`
	ApplicableSections []string  `json:"applicableSections"`
	SourceRef          string    `json:"sourceRef"`
	SourceQuote        string    `json:"sourceQuote"`
	SourceURL          string    `json:"sourceUrl"`
}

type entryUpdateRequest struct {
	entryRequest
	ExpectedVersion int     `json:"expectedVersion"`
	State           *string `json:"state"`
}

type adoptRequest struct {
	EntryIDs []string `json:"entryIds"`
	PackID   string   `json:"packId"`
}

type adoptionPatch struct {
	State        *string `json:"state"`
	OverrideText *string `json:"overrideText"`
}

type packResponse struct {
	ID         string       `json:"id"`
	Key        string       `json:"key"`
	Name       string       `json:"name"`
	Authority  string       `json:"authority"`
	Edition    string       `json:"edition"`
	SourceURL  string       `json:"sourceUrl,omitempty"`
	SourceNote string       `json:"sourceNote,omitempty"`
	Scenario   string       `json:"scenario"`
	IsBuiltin  bool         `json:"isBuiltin"`
	State      string       `json:"state"`
	EntryCount int64        `json:"entryCount"`
	CreatedAt  dto.JSONTime `json:"createdAt"`
	UpdatedAt  dto.JSONTime `json:"updatedAt"`
}

type entrySummary struct {
	ID                 string       `json:"id"`
	PackID             string       `json:"packId"`
	PackName           string       `json:"packName"`
	Code               string       `json:"code"`
	Category           string       `json:"category"`
	RuleKind           string       `json:"ruleKind"`
	Binding            string       `json:"binding"`
	Scenario           string       `json:"scenario"`
	Version            int          `json:"version"`
	SortOrder          int          `json:"sortOrder"`
	State              string       `json:"state"`
	Title              string       `json:"title"`
	RuleText           string       `json:"ruleText"`
	SourceRef          string       `json:"sourceRef"`
	SourceQuote        string       `json:"sourceQuote,omitempty"`
	SourceURL          string       `json:"sourceUrl,omitempty"`
	ApplicableSections []string     `json:"applicableSections"`
	Tags               []string     `json:"tags"`
	IsBuiltin          bool         `json:"isBuiltin"`
	UpdatedAt          dto.JSONTime `json:"updatedAt"`
}

type versionResponse struct {
	Version            int          `json:"version"`
	Title              string       `json:"title"`
	RuleText           string       `json:"ruleText"`
	Rationale          string       `json:"rationale,omitempty"`
	Examples           []Example    `json:"examples"`
	ApplicableSections []string     `json:"applicableSections"`
	SourceRef          string       `json:"sourceRef"`
	SourceQuote        string       `json:"sourceQuote,omitempty"`
	SourceURL          string       `json:"sourceUrl,omitempty"`
	Origin             string       `json:"origin"`
	ContentHash        string       `json:"contentHash"`
	CreatedAt          dto.JSONTime `json:"createdAt"`
}

type entryDetailResponse struct {
	entrySummary
	Rationale string            `json:"rationale,omitempty"`
	Examples  []Example         `json:"examples"`
	Versions  []versionResponse `json:"versions"`
}

type entryPage struct {
	Items    []entrySummary `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

type adoptionResponse struct {
	EntryID         string       `json:"entryId"`
	Version         int          `json:"version"`
	State           string       `json:"state"`
	OverrideText    string       `json:"overrideText,omitempty"`
	HasNewerVersion bool         `json:"hasNewerVersion"`
	LatestVersion   int          `json:"latestVersion"`
	CreatedAt       dto.JSONTime `json:"createdAt"`
	UpdatedAt       dto.JSONTime `json:"updatedAt"`
	Entry           entrySummary `json:"entry"`
}

// ── 通用查询 ────────────────────────────────────────────────────────

// visiblePacks 限定可见范围：内置包全局可读，用户包仅 owner 可见。
func visiblePacks(db *gorm.DB, uid store.UUID) *gorm.DB {
	return db.Where("(is_builtin = ? OR owner_id = ?)", true, uid)
}

func (s *Service) loadVisiblePack(ctx context.Context, uid, packID store.UUID) (*store.NormPack, error) {
	var pack store.NormPack
	err := visiblePacks(s.DB.WithContext(ctx), uid).Where("id = ?", packID).First(&pack).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpapi.NotFound("规范包")
	}
	if err != nil {
		return nil, err
	}
	return &pack, nil
}

// loadWritablePack：内置包只读，别人的包按不存在处理（不泄露存在性）。
func (s *Service) loadWritablePack(ctx context.Context, uid, packID store.UUID) (*store.NormPack, error) {
	pack, err := s.loadVisiblePack(ctx, uid, packID)
	if err != nil {
		return nil, err
	}
	if pack.IsBuiltin {
		return nil, httpapi.Forbidden("内置规范包只读，请复制为自建包后再修改")
	}
	if pack.OwnerID == nil || *pack.OwnerID != uid {
		return nil, httpapi.NotFound("规范包")
	}
	return pack, nil
}

type entryRow struct {
	Entry   store.NormEntry
	Version store.NormVersion
	Pack    store.NormPack
}

// loadEntryRows 按 entry id 批量装配 entry+当前版本+包，避免 N+1。
func (s *Service) loadEntryRows(ctx context.Context, uid store.UUID, entryIDs []store.UUID) (map[store.UUID]entryRow, error) {
	out := map[store.UUID]entryRow{}
	if len(entryIDs) == 0 {
		return out, nil
	}
	var entries []store.NormEntry
	if err := s.DB.WithContext(ctx).Where("id IN ?", entryIDs).Find(&entries).Error; err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return out, nil
	}
	packIDs := make([]store.UUID, 0, len(entries))
	versions := make([]int, 0, len(entries))
	for _, e := range entries {
		packIDs = append(packIDs, e.PackID)
		versions = append(versions, e.CurrentVersion)
	}
	var packs []store.NormPack
	if err := visiblePacks(s.DB.WithContext(ctx), uid).Where("id IN ?", packIDs).Find(&packs).Error; err != nil {
		return nil, err
	}
	packByID := map[store.UUID]store.NormPack{}
	for _, p := range packs {
		packByID[p.ID] = p
	}
	var vs []store.NormVersion
	if err := s.DB.WithContext(ctx).Where("entry_id IN ? AND version IN ?", entryIDs, versions).Find(&vs).Error; err != nil {
		return nil, err
	}
	versionByKey := map[string]store.NormVersion{}
	for _, v := range vs {
		versionByKey[v.EntryID.String()+"@"+strconv.Itoa(v.Version)] = v
	}
	for _, e := range entries {
		pack, ok := packByID[e.PackID]
		if !ok {
			continue // 包不可见（越权或已删）
		}
		v, ok := versionByKey[e.ID.String()+"@"+strconv.Itoa(e.CurrentVersion)]
		if !ok {
			continue
		}
		out[e.ID] = entryRow{Entry: e, Version: v, Pack: pack}
	}
	return out, nil
}

func (s *Service) loadTags(ctx context.Context, entryIDs []store.UUID) (map[store.UUID][]string, error) {
	out := map[store.UUID][]string{}
	if len(entryIDs) == 0 {
		return out, nil
	}
	var tags []store.NormEntryTag
	if err := s.DB.WithContext(ctx).Where("entry_id IN ?", entryIDs).Order("tag").Find(&tags).Error; err != nil {
		return nil, err
	}
	for _, t := range tags {
		out[t.EntryID] = append(out[t.EntryID], t.Tag)
	}
	return out, nil
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

// ── 版本内容 ────────────────────────────────────────────────────────

func decodeExamples(raw datatypes.JSON) []Example {
	out := []Example{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = []Example{}
	}
	return out
}

func decodeSections(raw datatypes.JSON) []string {
	out := []string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func normalizeList(items []string, limit int, field string) ([]string, error) {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if len([]rune(v)) > 32 {
			return nil, httpapi.BadRequest(field + "单项不能超过 32 个字符")
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) > limit {
		return nil, httpapi.BadRequest(field + "数量超过上限")
	}
	return out, nil
}

// versionContent 是参与 content_hash 的字段集合；元数据（分类/强制程度）不在内。
type versionContent struct {
	Title              string    `json:"title"`
	RuleText           string    `json:"ruleText"`
	Rationale          string    `json:"rationale"`
	Examples           []Example `json:"examples"`
	ApplicableSections []string  `json:"applicableSections"`
	SourceRef          string    `json:"sourceRef"`
	SourceQuote        string    `json:"sourceQuote"`
	SourceURL          string    `json:"sourceUrl"`
}

func (c versionContent) hash() string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// buildVersion 校验内容并生成一条不可变版本。
func buildVersion(req entryRequest, version int, origin string, createdBy *store.UUID) (store.NormVersion, error) {
	title := strings.TrimSpace(req.Title)
	ruleText := strings.TrimSpace(req.RuleText)
	sourceRef := strings.TrimSpace(req.SourceRef)
	if title == "" {
		return store.NormVersion{}, httpapi.BadRequest("规范条目必须填写标题")
	}
	if len([]rune(title)) > 200 {
		return store.NormVersion{}, httpapi.BadRequest("标题不能超过 200 个字符")
	}
	if ruleText == "" {
		return store.NormVersion{}, httpapi.BadRequest("规范条目必须填写规则说明")
	}
	if sourceRef == "" {
		return store.NormVersion{}, httpapi.BadRequest("规范条目必须填写来源出处")
	}
	if len([]rune(sourceRef)) > 300 {
		return store.NormVersion{}, httpapi.BadRequest("来源出处不能超过 300 个字符")
	}
	if len(req.Examples) > maxExamplesPerEntry {
		return store.NormVersion{}, httpapi.BadRequest("示例数量超过上限")
	}
	examples := make([]Example, 0, len(req.Examples))
	for _, ex := range req.Examples {
		if !exampleKinds[ex.Kind] {
			return store.NormVersion{}, httpapi.BadRequest("示例类型只能是 good 或 bad")
		}
		text := strings.TrimSpace(ex.Text)
		if text == "" {
			return store.NormVersion{}, httpapi.BadRequest("示例内容不能为空")
		}
		examples = append(examples, Example{Kind: ex.Kind, Text: text, Note: strings.TrimSpace(ex.Note)})
	}
	sections, err := normalizeList(req.ApplicableSections, maxSectionsPerEntry, "适用章节")
	if err != nil {
		return store.NormVersion{}, err
	}
	content := versionContent{
		Title: title, RuleText: ruleText, Rationale: strings.TrimSpace(req.Rationale),
		Examples: examples, ApplicableSections: sections,
		SourceRef: sourceRef, SourceQuote: strings.TrimSpace(req.SourceQuote),
		SourceURL: strings.TrimSpace(req.SourceURL),
	}
	if len(content.SourceURL) > 500 {
		return store.NormVersion{}, httpapi.BadRequest("来源链接不能超过 500 个字符")
	}
	rawExamples, _ := json.Marshal(content.Examples)
	rawSections, _ := json.Marshal(content.ApplicableSections)
	return store.NormVersion{
		Version: version, Title: content.Title,
		RuleText: content.RuleText, Rationale: content.Rationale,
		Examples: datatypes.JSON(rawExamples), ApplicableSections: datatypes.JSON(rawSections),
		SourceRef: content.SourceRef, SourceQuote: content.SourceQuote, SourceURL: content.SourceURL,
		ContentHash: content.hash(), Origin: origin, CreatedBy: createdBy,
	}, nil
}

// ── 响应装配 ────────────────────────────────────────────────────────

func toPackResponse(p store.NormPack, entryCount int64) packResponse {
	resp := packResponse{
		ID: p.ID.String(), Key: p.Key, Name: p.Name, Authority: p.Authority,
		Edition: p.Edition, Scenario: p.Scenario, IsBuiltin: p.IsBuiltin,
		State: p.State, EntryCount: entryCount,
		CreatedAt: dto.JSONTime{Time: p.CreatedAt}, UpdatedAt: dto.JSONTime{Time: p.UpdatedAt},
	}
	if p.SourceURL != nil {
		resp.SourceURL = *p.SourceURL
	}
	if p.SourceNote != nil {
		resp.SourceNote = *p.SourceNote
	}
	return resp
}

func toEntrySummary(row entryRow, tags []string) entrySummary {
	if tags == nil {
		tags = []string{}
	}
	return entrySummary{
		ID: row.Entry.ID.String(), PackID: row.Pack.ID.String(), PackName: row.Pack.Name,
		Code: row.Entry.Code, Category: row.Entry.Category, RuleKind: row.Entry.RuleKind,
		Binding: row.Entry.Binding, Scenario: row.Entry.Scenario, Version: row.Version.Version,
		SortOrder: row.Entry.SortOrder, State: row.Entry.State,
		Title: row.Version.Title, RuleText: row.Version.RuleText,
		SourceRef: row.Version.SourceRef, SourceQuote: row.Version.SourceQuote, SourceURL: row.Version.SourceURL,
		ApplicableSections: decodeSections(row.Version.ApplicableSections),
		Tags:               tags, IsBuiltin: row.Pack.IsBuiltin,
		UpdatedAt: dto.JSONTime{Time: row.Entry.UpdatedAt},
	}
}

func toVersionResponse(v store.NormVersion) versionResponse {
	return versionResponse{
		Version: v.Version, Title: v.Title, RuleText: v.RuleText, Rationale: v.Rationale,
		Examples: decodeExamples(v.Examples), ApplicableSections: decodeSections(v.ApplicableSections),
		SourceRef: v.SourceRef, SourceQuote: v.SourceQuote, SourceURL: v.SourceURL,
		Origin: v.Origin, ContentHash: v.ContentHash, CreatedAt: dto.JSONTime{Time: v.CreatedAt},
	}
}

// sortEntries 保持注入顺序稳定：先分类，再标题，最后 ID。
func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		if entries[i].Title != entries[j].Title {
			return entries[i].Title < entries[j].Title
		}
		return entries[i].ID < entries[j].ID
	})
}

// AdoptedForProject 实现 Provider：返回项目已启用且锁定的规范条目。
// 空切片表示「规范库可用，但项目未采用任何条目」，与 ErrUnavailable 语义不同。
func (s *Service) AdoptedForProject(ctx context.Context, projectID store.UUID) ([]Entry, error) {
	var adoptions []store.ProjectNormAdoption
	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND state = ?", projectID, store.NormAdoptionEnabled).
		Find(&adoptions).Error; err != nil {
		return nil, err
	}
	if len(adoptions) == 0 {
		return []Entry{}, nil
	}
	entryIDs := make([]store.UUID, 0, len(adoptions))
	versions := make([]int, 0, len(adoptions))
	for _, a := range adoptions {
		entryIDs = append(entryIDs, a.EntryID)
		versions = append(versions, a.Version)
	}
	var entries []store.NormEntry
	if err := s.DB.WithContext(ctx).Where("id IN ? AND state = ?", entryIDs, store.NormStateActive).
		Find(&entries).Error; err != nil {
		return nil, err
	}
	var rows []store.NormVersion
	if err := s.DB.WithContext(ctx).Where("entry_id IN ? AND version IN ?", entryIDs, versions).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	versionByKey := map[string]store.NormVersion{}
	for _, v := range rows {
		versionByKey[v.EntryID.String()+"@"+strconv.Itoa(v.Version)] = v
	}
	entryByID := map[store.UUID]store.NormEntry{}
	for _, e := range entries {
		entryByID[e.ID] = e
	}
	out := make([]Entry, 0, len(adoptions))
	for _, a := range adoptions {
		entry, ok := entryByID[a.EntryID]
		if !ok {
			continue
		}
		v, ok := versionByKey[a.EntryID.String()+"@"+strconv.Itoa(a.Version)]
		if !ok {
			continue
		}
		rule := v.RuleText
		if a.OverrideText != nil && strings.TrimSpace(*a.OverrideText) != "" {
			rule = *a.OverrideText
		}
		out = append(out, Entry{
			ID: entry.ID.String(), Title: v.Title, Rule: rule,
			Category: entry.Category, Source: v.SourceRef, Scope: entry.Scenario,
		})
	}
	sortEntries(out)
	return out, nil
}
