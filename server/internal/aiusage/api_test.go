package aiusage_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Info string          `json:"info"`
}

func getRaw(t *testing.T, ts *testutil.TestServer, path, token string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, raw
}

func get(t *testing.T, ts *testutil.TestServer, path, token string) (*http.Response, envelope) {
	t.Helper()
	resp, raw := getRaw(t, ts, path, token)
	var env envelope
	require.NoError(t, json.Unmarshal(raw, &env), "响应不是信封 JSON: %s", raw)
	return resp, env
}

func seedCall(t *testing.T, ts *testutil.TestServer, userID *store.UUID) {
	t.Helper()
	row := &store.AIModelCall{
		ID: store.MustNewUUID(), UserID: userID,
		FeatureKind: "chat", Provider: "provider-a", ProviderSource: "server-env", Model: "model-a",
		Status: "success", LatencyMs: 100, CreatedDay: "2026-11-27",
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	require.NoError(t, ts.DB.Create(row).Error)
}

type reportData struct {
	Totals struct {
		Calls int64 `json:"calls"`
	} `json:"totals"`
}

func TestPersonalUsageOnlySeesOwnCalls(t *testing.T) {
	ts := testutil.New(t)
	own := ts.UserID()
	other := store.UUID{UUID: uuid.MustParse("88888888-8888-4888-8888-888888888888")}
	seedCall(t, ts, &own)
	seedCall(t, ts, &own)
	seedCall(t, ts, &other)

	resp, env := get(t, ts, "/api/v1/ai-usage", ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var data reportData
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, int64(2), data.Totals.Calls, "个人接口只能看到自己的调用")

	// 接口不接受用户参数：显式传别人的 user_id 也无效。
	_, env = get(t, ts, "/api/v1/ai-usage?user_id="+other.String(), ts.Token)
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, int64(2), data.Totals.Calls)

	resp, _ = get(t, ts, "/api/v1/ai-usage", "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAdminUsageRequiresSuperuser(t *testing.T) {
	ts := testutil.New(t)
	own := ts.UserID()
	seedCall(t, ts, &own)

	resp, _ := get(t, ts, "/api/v1/admin/ai-usage", ts.Token)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "非 superuser 必须 403")

	ts.MakeSuperuser(t)
	resp, env := get(t, ts, "/api/v1/admin/ai-usage", ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var data reportData
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, int64(1), data.Totals.Calls)
}

func TestAdminExportCSVAndJSON(t *testing.T) {
	ts := testutil.New(t)
	ts.MakeSuperuser(t)
	own := ts.UserID()
	seedCall(t, ts, &own)
	seedCall(t, ts, &own)

	resp, raw := getRaw(t, ts, "/api/v1/admin/ai-usage/export?format=csv", ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
	body := string(raw)
	assert.Contains(t, body, "day,provider,model,feature_kind")
	assert.NotContains(t, body, "user_id", "导出不得包含个人维度")
	assert.NotContains(t, body, own.String())

	resp, env := get(t, ts, "/api/v1/admin/ai-usage/export?format=json", ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(env.Data), "user_id")
	assert.Contains(t, string(env.Data), `"model":"model-a"`)
}

func TestUsageFilterValidation(t *testing.T) {
	ts := testutil.New(t)
	ts.MakeSuperuser(t)

	cases := []string{
		"/api/v1/admin/ai-usage?from=not-a-date",
		"/api/v1/admin/ai-usage?from=2026-01-01&to=2026-06-01", // 超过 90 天
		"/api/v1/admin/ai-usage/export?format=xml",
	}
	for _, path := range cases {
		resp, _ := get(t, ts, path, ts.Token)
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, path)
	}
}

func TestAdminExportDoesNotLeakUsernames(t *testing.T) {
	ts := testutil.New(t)
	ts.MakeSuperuser(t)
	own := ts.UserID()
	seedCall(t, ts, &own)
	_, env := get(t, ts, "/api/v1/admin/ai-usage/export?format=json", ts.Token)
	assert.False(t, strings.Contains(string(env.Data), "tester"), "导出不得包含用户名")
}
