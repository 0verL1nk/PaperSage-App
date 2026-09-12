package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func newSettingsTestServer(t *testing.T) (*httptest.Server, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	cfg := &config.Settings{SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7}
	authSvc := auth.NewService(db, cfg)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	auth.Mount(v1, authSvc)
	Mount(v1, authSvc.AuthRequired(), NewService(db))

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
