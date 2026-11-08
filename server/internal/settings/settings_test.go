package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/tests/test_settings_api.py 语义 + routes/settings.py

func newSettingsTestServer(t *testing.T, cfgs ...*config.Settings) (*httptest.Server, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	cfg := &config.Settings{SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7}
	if len(cfgs) > 0 && cfgs[0] != nil {
		cfg = cfgs[0]
	}
	authSvc := auth.NewService(db, cfg, nil)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	auth.Mount(v1, authSvc)
	Mount(v1, authSvc.AuthRequired(), NewService(db, cfg))

	// 预置固定 ID 用户
	require.NoError(t, db.Create(&store.User{
		Username: "u1", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: store.UUID{UUID: seededUID}},
	}).Error)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return srv, db
}

var seededUID = uuid.MustParse("33333333-3333-3333-3333-333333333333")

var testCfg = &config.Settings{SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7}

// mintToken 直接铸造 access token（测试用户密码非 bcrypt，不走登录）
func mintToken(t *testing.T, uid uuid.UUID) string {
	t.Helper()
	tok, err := auth.CreateAccessToken(testCfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	return tok
}

func authedGet(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx // test
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func authedJSON(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, bytes.NewReader(b)) //nolint:noctx // test
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	return m
}

func TestConcurrentPatchesDoNotLoseSuccessfulChanges(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new category"
		if existing {
			name = "existing category"
		}
		t.Run(name, func(t *testing.T) { testConcurrentPatches(t, existing) })
	}
}

func testConcurrentPatches(t *testing.T, existing bool) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/concurrent.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&store.UserSettings{}))
	uid := uuid.New()
	svc := NewService(db, nil)
	if existing {
		_, initialErr := svc.UpdateCategory(context.Background(), uid, "agent", map[string]any{})
		require.Nil(t, initialErr)
	}
	type marker struct{}
	read, release := make(chan struct{}), make(chan struct{})
	var pause sync.Once
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:pause-read", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_settings" && tx.Statement.Context.Value(marker{}) != nil {
			pause.Do(func() { close(read); <-release })
		}
	}))
	defer func() { require.NoError(t, db.Callback().Query().Remove("test:pause-read")) }()
	first, second := make(chan *ValidationError, 1), make(chan *ValidationError, 1)
	go func() {
		_, e := svc.UpdateCategory(context.WithValue(context.Background(), marker{}, true), uid, "agent", map[string]any{"temperature": 1.0})
		first <- e
	}()
	select {
	case <-read:
	case <-time.After(3 * time.Second):
		t.Fatal("first patch did not read settings")
	}
	go func() {
		_, e := svc.UpdateCategory(context.Background(), uid, "agent", map[string]any{"writing_style": "formal"})
		second <- e
	}()
	var secondResult *ValidationError
	var secondFinished bool
	select {
	case secondResult = <-second:
		secondFinished = true
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.Nil(t, <-first)
	if !secondFinished {
		secondResult = <-second
	}
	require.Nil(t, secondResult)
	var row store.UserSettings
	require.NoError(t, db.Where("user_id = ? AND category = ?", store.UUID{UUID: uid}, "agent").First(&row).Error)
	var result map[string]any
	require.NoError(t, json.Unmarshal(row.Settings, &result))
	assert.Equal(t, float64(1), result["temperature"])
	assert.Equal(t, "formal", result["writing_style"])
}

func TestModelSelectionPatch(t *testing.T) {
	srv, db := newSettingsTestServer(t)
	url, token := srv.URL+"/api/v1/settings/agent", mintToken(t, seededUID)
	providers := []any{
		map[string]any{"id": "a", "api_key": "secret-a", "models": []any{"a1", "a2"}, "active_model": "a1"},
		map[string]any{"id": "b", "api_key": "secret-b", "models": []any{"b1"}, "active_model": "b1"},
	}
	resp := authedJSON(t, http.MethodPatch, url, token, map[string]any{"settings": map[string]any{"providers": providers}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = authedJSON(t, http.MethodPatch, url, token, map[string]any{"settings": map[string]any{
		"model_selection": map[string]any{"provider_id": "a", "model": "a2"},
	}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	result := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	assert.Equal(t, "a", result["active_provider_id"])
	assert.NotContains(t, result, "model_selection")
	got := result["providers"].([]any)
	assert.Equal(t, "a2", got[0].(map[string]any)["active_model"])
	assert.Equal(t, "secret-a", got[0].(map[string]any)["api_key"])
	assert.Equal(t, "secret-b", got[1].(map[string]any)["api_key"])
	var row store.UserSettings
	require.NoError(t, db.Where("user_id = ? AND category = ?", store.UUID{UUID: seededUID}, "agent").First(&row).Error)
	assert.NotContains(t, string(row.Settings), "model_selection")

	invalid := []map[string]any{
		{"model_selection": nil},
		{"model_selection": "bad"},
		{"model_selection": map[string]any{"provider_id": "a"}},
		{"model_selection": map[string]any{"provider_id": "a", "model": ""}},
		{"model_selection": map[string]any{"provider_id": "missing", "model": "a2"}},
		{"model_selection": map[string]any{"provider_id": "a", "model": "missing"}},
		{"model_selection": map[string]any{"provider_id": "a", "model": "a2", "api_key": "secret-marker"}},
		{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}, "providers": providers},
		{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}, "active_provider_id": "b"},
		{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}, "temperature": 1.2},
	}
	for _, patch := range invalid {
		resp := authedJSON(t, http.MethodPatch, url, token, map[string]any{"settings": patch})
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "%v", patch)
		body := decode(t, resp)
		assert.NotContains(t, body["info"], "secret-marker")
		assert.NotContains(t, body["info"], "secret-a")
	}
	resp = authedGet(t, url, token)
	result = decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	assert.Equal(t, "a", result["active_provider_id"])
	assert.Equal(t, "a2", result["providers"].([]any)[0].(map[string]any)["active_model"])
	resp = authedJSON(t, http.MethodPut, url, token, map[string]any{"settings": map[string]any{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}}})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	resp = authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/ui", token, map[string]any{"settings": map[string]any{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}}})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	duplicate, err := json.Marshal(map[string]any{"providers": []any{providers[0], providers[0]}})
	require.NoError(t, err)
	require.NoError(t, db.Model(&row).Update("settings", duplicate).Error)
	resp = authedJSON(t, http.MethodPatch, url, token, map[string]any{"settings": map[string]any{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}}})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Contains(t, decode(t, resp)["info"], "unique")
	require.NoError(t, db.Where("user_id = ? AND category = ?", store.UUID{UUID: seededUID}, "agent").First(&row).Error)
	assert.JSONEq(t, string(duplicate), string(row.Settings))
}

func TestModelSelectionPreservesLegacyEmptyActiveModel(t *testing.T) {
	srv, db := newSettingsTestServer(t)
	legacy := `{"providers":[{"id":"a","models":["a1"],"active_model":"","api_key":"synthetic-a"},{"id":"b","models":["b1"],"active_model":"","api_key":"synthetic-b"}],"active_provider_id":""}`
	require.NoError(t, db.Create(&store.UserSettings{UserID: store.UUID{UUID: seededUID}, Category: "agent", Settings: []byte(legacy)}).Error)
	url, token := srv.URL+"/api/v1/settings/agent", mintToken(t, seededUID)
	for _, tc := range []struct{ id, model, other string }{{"a", "a1", "b"}, {"b", "b1", "a"}} {
		resp := authedJSON(t, http.MethodPatch, url, token, map[string]any{"settings": map[string]any{"model_selection": map[string]any{"provider_id": tc.id, "model": tc.model}}})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		result := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
		assert.Equal(t, tc.id, result["active_provider_id"])
		for _, raw := range result["providers"].([]any) {
			p := raw.(map[string]any)
			if p["id"] == tc.id {
				assert.Equal(t, tc.model, p["active_model"])
			} else if tc.id == "a" && p["id"] == tc.other {
				assert.Equal(t, map[string]any{"id": "b", "models": []any{"b1"}, "active_model": "", "api_key": "synthetic-b"}, p)
			}
		}
	}
}

func TestProviderIDsAreUnique(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	url, token := srv.URL+"/api/v1/settings/agent", mintToken(t, seededUID)
	for _, invalid := range [][]any{
		{map[string]any{"id": "same"}, map[string]any{"id": "same"}},
		{map[string]any{"id": " \t "}},
	} {
		for _, method := range []string{http.MethodPatch, http.MethodPut} {
			resp := authedJSON(t, method, url, token, map[string]any{"settings": map[string]any{"providers": invalid}})
			assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
		}
	}
	providers := make([]any, 100)
	for i := range providers {
		providers[i] = map[string]any{"name": "synthetic"}
	}
	resp := authedJSON(t, http.MethodPatch, url, token, map[string]any{"settings": map[string]any{"providers": providers}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	result := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	ids := map[string]bool{}
	for _, item := range result["providers"].([]any) {
		id := item.(map[string]any)["id"].(string)
		assert.Regexp(t, `^[0-9a-f]{12}$`, id)
		assert.False(t, ids[id], "generated IDs must be unique")
		ids[id] = true
	}
}

func TestModelSelectionInterleavesWithProviderPatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/selection.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&store.UserSettings{}))
	uid, svc := uuid.New(), NewService(db, nil)
	providers := []any{
		map[string]any{"id": "a", "api_key": "secret-a", "models": []any{"a1", "a2"}, "active_model": "a1"},
		map[string]any{"id": "b", "api_key": "old-b", "models": []any{"b1"}, "active_model": "b1"},
	}
	_, validation := svc.UpdateCategory(context.Background(), uid, "agent", map[string]any{"providers": providers})
	require.Nil(t, validation)
	providers[1].(map[string]any)["api_key"] = "new-b"
	type marker struct{}
	read, release, secondStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var pause sync.Once
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:selection-started", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_settings" && tx.Statement.Context.Value(marker{}) == "selection" {
			close(secondStarted)
		}
	}))
	defer func() { require.NoError(t, db.Callback().Create().Remove("test:selection-started")) }()
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:selection-pause", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_settings" && tx.Statement.Context.Value(marker{}) == "provider" {
			pause.Do(func() { close(read); <-release })
		}
	}))
	defer func() { require.NoError(t, db.Callback().Query().Remove("test:selection-pause")) }()
	first, second := make(chan *ValidationError, 1), make(chan *ValidationError, 1)
	go func() {
		_, e := svc.UpdateCategory(context.WithValue(context.Background(), marker{}, "provider"), uid, "agent", map[string]any{"providers": providers, "temperature": 1.2})
		first <- e
	}()
	select {
	case <-read:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("provider patch did not read settings")
	}
	go func() {
		_, e := svc.UpdateCategory(context.WithValue(context.Background(), marker{}, "selection"), uid, "agent", map[string]any{"model_selection": map[string]any{"provider_id": "a", "model": "a2"}})
		second <- e
	}()
	select {
	case <-secondStarted:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("selection patch did not start")
	}
	close(release)
	require.Nil(t, <-first)
	require.Nil(t, <-second)
	result, err := svc.GetCategory(context.Background(), uid, "agent")
	require.NoError(t, err)
	got := result["providers"].([]any)
	assert.Equal(t, "a", result["active_provider_id"])
	assert.Equal(t, "a2", got[0].(map[string]any)["active_model"])
	assert.Equal(t, "secret-a", got[0].(map[string]any)["api_key"])
	assert.Equal(t, "new-b", got[1].(map[string]any)["api_key"])
	assert.Equal(t, 1.2, result["temperature"])
}

func TestInvalidPatchRollsBackNewSettingsRow(t *testing.T) {
	srv, db := newSettingsTestServer(t)
	resp := authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/agent", mintToken(t, seededUID), map[string]any{
		"settings": map[string]any{"providers": []any{map[string]any{"models": []any{map[string]any{"bad": true}}}}},
	})
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var count int64
	require.NoError(t, db.Model(&store.UserSettings{}).Where("user_id = ? AND category = ?", store.UUID{UUID: seededUID}, "agent").Count(&count).Error)
	require.Zero(t, count)
}

func TestSettingsReadFailureIsNotReportedAsDefaults(t *testing.T) {
	srv, db := newSettingsTestServer(t)
	require.NoError(t, db.Migrator().DropTable(&store.UserSettings{}))
	for _, path := range []string{"/api/v1/settings", "/api/v1/settings/agent"} {
		resp := authedGet(t, srv.URL+path, mintToken(t, seededUID))
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.NotContains(t, decode(t, resp)["info"], "user_settings")
	}
}

func TestFailedSettingsWriteDoesNotLogAPIKey(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.New(log.New(&logs, "", 0), logger.Config{LogLevel: logger.Info})})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&store.UserSettings{}))
	uid := uuid.New()
	require.NoError(t, db.Create(&store.UserSettings{
		UserID: store.UUID{UUID: uid}, Category: "agent",
		Settings: []byte(`{"providers":[{"id":"p","api_key":"synthetic-update-secret","models":["a","b"],"active_model":"a"}],"active_provider_id":"p"}`),
	}).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_secret BEFORE UPDATE ON user_settings BEGIN SELECT RAISE(FAIL, 'write failed'); END`).Error)
	logs.Reset()
	_, saveErr := NewService(db, nil).UpdateCategory(context.Background(), uid, "agent", map[string]any{
		"model_selection": map[string]any{"provider_id": "p", "model": "b"},
	})
	require.NotNil(t, saveErr)
	assert.True(t, saveErr.internal)
	assert.NotContains(t, logs.String(), "synthetic-update-secret")
}

func TestAgentSettingsWriteFailureIsReported(t *testing.T) {
	srv, db := newSettingsTestServer(t)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_settings BEFORE INSERT ON user_settings BEGIN SELECT RAISE(FAIL, 'private database detail'); END`).Error)
	resp := authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/agent", mintToken(t, seededUID), map[string]any{"settings": map[string]any{"active_provider_id": "p"}})
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body := decode(t, resp)
	assert.NotContains(t, body["info"], "private database detail")
}

func TestGetAllDefaults(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	token := mintToken(t, seededUID)

	resp := authedGet(t, srv.URL+"/api/v1/settings", token)
	require.Equal(t, 200, resp.StatusCode)

	data := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	ui := data["ui"].(map[string]any)
	assert.Equal(t, "system", ui["theme"])
	assert.Equal(t, "zh-CN", ui["language"])
	editor := data["editor"].(map[string]any)
	assert.Equal(t, float64(14), editor["font_size"])
	agent := data["agent"].(map[string]any)
	assert.Equal(t, float64(0.7), agent["temperature"])
	assert.Equal(t, []any{}, agent["providers"])
}

func TestUnknownCategory404(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	token := mintToken(t, seededUID)

	resp := authedGet(t, srv.URL+"/api/v1/settings/unknown", token)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "Unknown settings category: unknown", decode(t, resp)["info"])
}

func TestPatchMergeAndValidation(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	token := mintToken(t, seededUID)

	// PATCH 单字段 → 合并返回
	resp := authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/ui", token, map[string]any{"settings": map[string]any{"theme": "dark"}})
	require.Equal(t, 200, resp.StatusCode)
	settings := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	assert.Equal(t, "dark", settings["theme"])
	assert.Equal(t, "zh-CN", settings["language"], "未传字段保留默认")

	// 非法值 → 422
	resp = authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/editor", token, map[string]any{"settings": map[string]any{"font_size": "huge"}})
	assert.Equal(t, 422, resp.StatusCode)

	resp = authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/editor", token, map[string]any{"settings": map[string]any{"font_size": 99}})
	assert.Equal(t, 422, resp.StatusCode)

	// 未知字段被忽略（Pydantic extra=ignore）
	resp = authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/ui", token, map[string]any{"settings": map[string]any{"nope": 1}})
	require.Equal(t, 200, resp.StatusCode)
}

func TestPutReplaceFillsDefaults(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	token := mintToken(t, seededUID)

	resp := authedJSON(t, http.MethodPut, srv.URL+"/api/v1/settings/ui", token, map[string]any{"settings": map[string]any{"theme": "light"}})
	require.Equal(t, 200, resp.StatusCode)
	settings := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	assert.Equal(t, "light", settings["theme"])
	assert.Equal(t, false, settings["sidebar_collapsed"], "PUT 后其余字段回到默认（model_dump 填充）")
}

func TestDeleteResets(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	token := mintToken(t, seededUID)

	resp := authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/ui", token, map[string]any{"settings": map[string]any{"theme": "dark"}})
	require.Equal(t, 200, resp.StatusCode)

	resp = authedJSON(t, http.MethodDelete, srv.URL+"/api/v1/settings/ui", token, nil)
	require.Equal(t, 200, resp.StatusCode)
	settings := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	assert.Equal(t, "system", settings["theme"], "重置后回到默认")
}

// agent.providers 无 id 时自动生成 12 位 hex（model_validator 语义）
func TestAgentProviderIDAutogen(t *testing.T) {
	out, err := NormalizeCategory("agent", map[string]any{
		"providers": []any{map[string]any{"name": "deepseek", "type": "anthropic"}},
	})
	require.NoError(t, err)
	providers := out["providers"].([]any)
	p := providers[0].(map[string]any)
	assert.Regexp(t, `^[0-9a-f]{12}$`, p["id"])
	assert.Equal(t, "anthropic", p["type"])
	assert.Equal(t, "", p["active_model"])
}

func TestUnauthenticatedBlocked(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/settings") //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, 401, resp.StatusCode)
}
