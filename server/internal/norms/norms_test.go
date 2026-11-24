package norms

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func setup(t *testing.T) (*Service, *gorm.DB, store.UUID, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/norms.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "norms-a", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "norms-b", PasswordHash: "x", BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}).Error)
	require.NoError(t, EnsureBuiltins(context.Background(), db))
	return NewService(db), db, uid, other, pid
}

func apiCode(t *testing.T, err error) int {
	t.Helper()
	var apiErr *httpapi.APIError
	if !errors.As(err, &apiErr) {
		return 0
	}
	return apiErr.Code
}

func mustUUID(t *testing.T, raw string) store.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	require.NoError(t, err)
	return store.UUID{UUID: id}
}

func builtinPackFor(t *testing.T, svc *Service, uid store.UUID, scenario string) packResponse {
	t.Helper()
	packs, err := svc.ListPacks(context.Background(), uid)
	require.NoError(t, err)
	for _, p := range packs {
		if p.IsBuiltin && p.Scenario == scenario {
			return p
		}
	}
	t.Fatalf("未找到场景 %s 的内置规范包", scenario)
	return packResponse{}
}

func entryReq(packID, code string) entryRequest {
	return entryRequest{
		PackID: packID, Code: code, Category: "参考文献", RuleKind: store.NormRuleKindOfficial,
		Binding: store.NormBindingMust, Scenario: store.NormScenarioUndergraduateThesis,
		Tags: []string{"参考文献"}, Title: "标题 " + code, RuleText: "规则说明",
		SourceRef: "GB/T 7714—2015", SourceQuote: "示例原文",
		ApplicableSections: []string{"参考文献"},
	}
}

func TestBuiltinSeedsAreDualScenarioAndSourced(t *testing.T) {
	packs, err := BuiltinPacks()
	require.NoError(t, err)
	require.Len(t, packs, 2)

	byScenario := map[string]builtinPack{}
	for _, pack := range packs {
		byScenario[pack.Scenario] = pack
		assert.NotEmpty(t, pack.SourceNote)
		assert.GreaterOrEqual(t, len(pack.Entries), 8, "内置包 %s 条目过少", pack.Key)
		for _, entry := range pack.Entries {
			assert.NotEmpty(t, entry.SourceRef, "条目 %s 缺少来源出处", entry.Code)
			assert.True(t, entry.SourceQuote != "" || entry.SourceURL != "",
				"条目 %s 的来源不可核验", entry.Code)
			assert.NotEmpty(t, entry.RuleText)
			assert.Equal(t, pack.Scenario, entry.Scenario)
		}
	}
	assert.Contains(t, byScenario, store.NormScenarioUndergraduateThesis)
	assert.Contains(t, byScenario, store.NormScenarioIntroPaper)
}

func TestEnsureBuiltinsIsIdempotent(t *testing.T) {
	svc, db, uid, _, _ := setup(t)
	ctx := context.Background()

	var firstPacks, firstEntries int64
	require.NoError(t, db.Model(&store.NormPack{}).Count(&firstPacks).Error)
	require.NoError(t, db.Model(&store.NormEntry{}).Count(&firstEntries).Error)
	require.NoError(t, EnsureBuiltins(ctx, db))
	require.NoError(t, EnsureBuiltins(ctx, db))

	var secondPacks, secondEntries int64
	require.NoError(t, db.Model(&store.NormPack{}).Count(&secondPacks).Error)
	require.NoError(t, db.Model(&store.NormEntry{}).Count(&secondEntries).Error)
	assert.Equal(t, firstPacks, secondPacks)
	assert.Equal(t, firstEntries, secondEntries)

	packs, err := svc.ListPacks(ctx, uid)
	require.NoError(t, err)
	require.Len(t, packs, 2)
	for _, p := range packs {
		assert.True(t, p.IsBuiltin)
		assert.Greater(t, p.EntryCount, int64(0))
	}
}

func TestPackCRUDAndVisibility(t *testing.T) {
	svc, _, uid, other, _ := setup(t)
	ctx := context.Background()

	created, err := svc.CreatePack(ctx, uid, packRequest{
		Key: "my-lab-rules", Name: "实验室写作规范", Authority: store.NormAuthorityUser,
		Scenario: store.NormScenarioUndergraduateThesis,
	})
	require.NoError(t, err)
	assert.False(t, created.IsBuiltin)

	mine, err := svc.ListPacks(ctx, uid)
	require.NoError(t, err)
	assert.Len(t, mine, 3) // 2 个内置 + 1 个自建

	theirs, err := svc.ListPacks(ctx, other)
	require.NoError(t, err)
	assert.Len(t, theirs, 2)

	_, err = svc.CreatePack(ctx, uid, packRequest{Key: "my-lab-rules", Name: "重复标识"})
	assert.Equal(t, 409, apiCode(t, err))

	_, err = svc.CreatePack(ctx, uid, packRequest{Key: "Bad Key", Name: "非法标识"})
	assert.Equal(t, 400, apiCode(t, err))

	_, err = svc.UpdatePack(ctx, other, mustUUID(t, created.ID), packPatch{Name: ptr("被别人改名")})
	assert.Equal(t, 404, apiCode(t, err))

	renamed, err := svc.UpdatePack(ctx, uid, mustUUID(t, created.ID), packPatch{Name: ptr("改名后的规范包")})
	require.NoError(t, err)
	assert.Equal(t, "改名后的规范包", renamed.Name)

	builtin := builtinPackFor(t, svc, uid, store.NormScenarioUndergraduateThesis)
	_, err = svc.UpdatePack(ctx, uid, mustUUID(t, builtin.ID), packPatch{Name: ptr("篡改内置包")})
	assert.Equal(t, 403, apiCode(t, err))
	assert.Equal(t, 403, apiCode(t, svc.DeletePack(ctx, uid, mustUUID(t, builtin.ID))))
}

func TestEntryVersioningAndBoundaries(t *testing.T) {
	svc, _, uid, _, _ := setup(t)
	ctx := context.Background()

	pack, err := svc.CreatePack(ctx, uid, packRequest{Key: "custom-pack", Name: "自建包"})
	require.NoError(t, err)

	created, err := svc.CreateEntry(ctx, uid, entryReq(pack.ID, "MY-1"))
	require.NoError(t, err)
	assert.Equal(t, 1, created.Version)
	assert.Len(t, created.Versions, 1)

	noSource := entryReq(pack.ID, "MY-2")
	noSource.SourceRef = ""
	_, err = svc.CreateEntry(ctx, uid, noSource)
	assert.Equal(t, 400, apiCode(t, err))

	duplicate := entryReq(pack.ID, "MY-1")
	_, err = svc.CreateEntry(ctx, uid, duplicate)
	assert.Equal(t, 409, apiCode(t, err))

	update := entryUpdateRequest{entryRequest: entryReq(pack.ID, "MY-1"), ExpectedVersion: 1}
	update.Title = "改后的标题"
	updated, err := svc.UpdateEntry(ctx, uid, mustUUID(t, created.ID), update)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
	assert.Equal(t, "改后的标题", updated.Title)
	assert.Len(t, updated.Versions, 2)

	_, err = svc.UpdateEntry(ctx, uid, mustUUID(t, created.ID), update)
	assert.Equal(t, 409, apiCode(t, err), "旧版本号必须被拒绝，防止覆盖他人修改")

	noVersion := entryUpdateRequest{entryRequest: entryReq(pack.ID, "MY-1")}
	_, err = svc.UpdateEntry(ctx, uid, mustUUID(t, created.ID), noVersion)
	assert.Equal(t, 400, apiCode(t, err))

	builtin := builtinPackFor(t, svc, uid, store.NormScenarioUndergraduateThesis)
	page, err := svc.SearchEntries(ctx, uid, entryFilter{PackID: builtin.ID, PageSize: 1})
	require.NoError(t, err)
	require.NotEmpty(t, page.Items)
	builtinEntry := page.Items[0]
	_, err = svc.UpdateEntry(ctx, uid, mustUUID(t, builtinEntry.ID), entryUpdateRequest{
		entryRequest: entryReq(builtin.ID, builtinEntry.Code), ExpectedVersion: builtinEntry.Version,
	})
	assert.Equal(t, 403, apiCode(t, err), "内置条目必须只读")
	assert.Equal(t, 403, apiCode(t, svc.DeleteEntry(ctx, uid, mustUUID(t, builtinEntry.ID))))

	require.NoError(t, svc.DeleteEntry(ctx, uid, mustUUID(t, created.ID)))
	_, err = svc.GetEntry(ctx, uid, mustUUID(t, created.ID))
	assert.Equal(t, 404, apiCode(t, err))
}

func TestSearchFiltersAndPagination(t *testing.T) {
	svc, _, uid, other, _ := setup(t)
	ctx := context.Background()

	all, err := svc.SearchEntries(ctx, uid, entryFilter{PageSize: 100})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, all.Total, int64(16))

	firstPage, err := svc.SearchEntries(ctx, uid, entryFilter{Page: 1, PageSize: 5})
	require.NoError(t, err)
	assert.Len(t, firstPage.Items, 5)
	secondPage, err := svc.SearchEntries(ctx, uid, entryFilter{Page: 2, PageSize: 5})
	require.NoError(t, err)
	assert.NotEqual(t, firstPage.Items[0].ID, secondPage.Items[0].ID)

	byText, err := svc.SearchEntries(ctx, uid, entryFilter{Query: "GB/T 7714"})
	require.NoError(t, err)
	assert.Greater(t, byText.Total, int64(0))

	byCategory, err := svc.SearchEntries(ctx, uid, entryFilter{Category: "学术诚信"})
	require.NoError(t, err)
	assert.Greater(t, byCategory.Total, int64(0))
	for _, item := range byCategory.Items {
		assert.Equal(t, "学术诚信", item.Category)
	}

	byBinding, err := svc.SearchEntries(ctx, uid, entryFilter{Binding: store.NormBindingMust})
	require.NoError(t, err)
	assert.Greater(t, byBinding.Total, int64(0))
	for _, item := range byBinding.Items {
		assert.Equal(t, store.NormBindingMust, item.Binding)
	}

	byTag, err := svc.SearchEntries(ctx, uid, entryFilter{Tag: "摘要"})
	require.NoError(t, err)
	assert.Greater(t, byTag.Total, int64(0))

	byScenario, err := svc.SearchEntries(ctx, uid, entryFilter{Scenario: store.NormScenarioIntroPaper})
	require.NoError(t, err)
	assert.Greater(t, byScenario.Total, int64(0))
	for _, item := range byScenario.Items {
		assert.Contains(t, []string{store.NormScenarioIntroPaper, store.NormScenarioAll}, item.Scenario)
	}

	// 私有包不进入他人检索
	private, err := svc.CreatePack(ctx, uid, packRequest{Key: "private-pack", Name: "私有包"})
	require.NoError(t, err)
	_, err = svc.CreateEntry(ctx, uid, entryReq(private.ID, "PRIV-1"))
	require.NoError(t, err)
	visibleToOther, err := svc.SearchEntries(ctx, other, entryFilter{Query: "PRIV-1"})
	require.NoError(t, err)
	assert.Zero(t, visibleToOther.Total)
}

func TestAdoptionLifecycle(t *testing.T) {
	svc, _, uid, _, pid := setup(t)
	ctx := context.Background()
	builtin := builtinPackFor(t, svc, uid, store.NormScenarioUndergraduateThesis)

	adopted, err := svc.Adopt(ctx, uid, pid, adoptRequest{PackID: builtin.ID})
	require.NoError(t, err)
	assert.Len(t, adopted, int(builtin.EntryCount))

	again, err := svc.Adopt(ctx, uid, pid, adoptRequest{PackID: builtin.ID})
	require.NoError(t, err)
	assert.Len(t, again, len(adopted), "重复采用必须幂等")

	entries, err := svc.AdoptedForProject(ctx, pid)
	require.NoError(t, err)
	assert.Len(t, entries, len(adopted))
	for _, e := range entries {
		assert.NotEmpty(t, e.Source, "注入 AI 的规范必须带出处")
		assert.NotEmpty(t, e.Rule)
	}

	target := mustUUID(t, adopted[0].EntryID)
	disabled := store.NormAdoptionDisabled
	updated, err := svc.UpdateAdoption(ctx, uid, pid, target, adoptionPatch{State: &disabled})
	require.NoError(t, err)
	assert.Equal(t, store.NormAdoptionDisabled, updated.State)
	entries, err = svc.AdoptedForProject(ctx, pid)
	require.NoError(t, err)
	assert.Len(t, entries, len(adopted)-1, "停用后不得再注入 AI")

	override := "本项目按学院补充要求执行"
	enabled := store.NormAdoptionEnabled
	_, err = svc.UpdateAdoption(ctx, uid, pid, target, adoptionPatch{State: &enabled, OverrideText: &override})
	require.NoError(t, err)
	entries, err = svc.AdoptedForProject(ctx, pid)
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if e.ID == target.String() {
			found = true
			assert.Equal(t, override, e.Rule)
		}
	}
	assert.True(t, found)

	require.NoError(t, svc.Unadopt(ctx, uid, pid, target))
	list, err := svc.ListProjectNorms(ctx, uid, pid)
	require.NoError(t, err)
	assert.Len(t, list, len(adopted)-1)
	assert.Equal(t, 404, apiCode(t, svc.Unadopt(ctx, uid, pid, target)))
}

func TestAdoptionLocksVersionAndBlocksDelete(t *testing.T) {
	svc, _, uid, _, pid := setup(t)
	ctx := context.Background()

	pack, err := svc.CreatePack(ctx, uid, packRequest{Key: "lock-pack", Name: "版本锁定包"})
	require.NoError(t, err)
	created, err := svc.CreateEntry(ctx, uid, entryReq(pack.ID, "LOCK-1"))
	require.NoError(t, err)

	_, err = svc.Adopt(ctx, uid, pid, adoptRequest{EntryIDs: []string{created.ID}})
	require.NoError(t, err)

	update := entryUpdateRequest{entryRequest: entryReq(pack.ID, "LOCK-1"), ExpectedVersion: 1}
	update.RuleText = "改后的规则说明"
	_, err = svc.UpdateEntry(ctx, uid, mustUUID(t, created.ID), update)
	require.NoError(t, err)

	list, err := svc.ListProjectNorms(ctx, uid, pid)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 1, list[0].Version, "已采用版本不得随条目更新静默变化")
	assert.Equal(t, 2, list[0].LatestVersion)
	assert.True(t, list[0].HasNewerVersion)

	entries, err := svc.AdoptedForProject(ctx, pid)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "规则说明", entries[0].Rule, "注入的仍是采用时锁定的版本")

	assert.Equal(t, 409, apiCode(t, svc.DeleteEntry(ctx, uid, mustUUID(t, created.ID))))
	assert.Equal(t, 409, apiCode(t, svc.DeletePack(ctx, uid, mustUUID(t, pack.ID))))
}

func TestAdoptionPermissions(t *testing.T) {
	svc, db, uid, other, pid := setup(t)
	ctx := context.Background()

	otherProject := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "别人的项目", OwnerID: other, BaseModel: store.BaseModel{ID: otherProject}}).Error)
	builtin := builtinPackFor(t, svc, uid, store.NormScenarioUndergraduateThesis)

	_, err := svc.Adopt(ctx, other, pid, adoptRequest{PackID: builtin.ID})
	assert.Equal(t, 404, apiCode(t, err), "不能往别人的项目里采用规范")
	_, err = svc.ListProjectNorms(ctx, other, pid)
	assert.Equal(t, 404, apiCode(t, err))

	otherPack, err := svc.CreatePack(ctx, other, packRequest{Key: "other-pack", Name: "别人的包"})
	require.NoError(t, err)
	otherEntry, err := svc.CreateEntry(ctx, other, entryReq(otherPack.ID, "OTHER-1"))
	require.NoError(t, err)
	_, err = svc.Adopt(ctx, uid, pid, adoptRequest{EntryIDs: []string{otherEntry.ID}})
	assert.Equal(t, 400, apiCode(t, err), "不能采用别人私有包里的条目")

	_, err = svc.Adopt(ctx, uid, pid, adoptRequest{})
	assert.Equal(t, 400, apiCode(t, err), "空请求必须被拒绝")
}

func TestProviderEmptyWhenNothingAdopted(t *testing.T) {
	svc, _, _, _, pid := setup(t)
	entries, err := svc.AdoptedForProject(context.Background(), pid)
	require.NoError(t, err)
	assert.NotNil(t, entries, "空切片表示「规范库可用但未采用」，不能是 nil")
	assert.Empty(t, entries)
}

func ptr[T any](v T) *T { return &v }
