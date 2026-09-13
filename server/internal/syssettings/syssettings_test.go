package syssettings_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 契约：系统设置为 admin（superuser）专属；键值存 DB；
// allowed_email_domains 是学校邮箱白名单（空/缺省 = 不限制）。
// 普通用户读 403、写 403；未认证 401。

func TestAdminCanReadWriteSettings(t *testing.T) {
	ts := testutil.New(t)
	ts.MakeSuperuser(t)

	resp := ts.Do(t, http.MethodPut, "/api/v1/admin/settings",
		mustJSON(t, map[string]any{"allowed_email_domains": []string{"hit.edu.cn", "*.hit.edu.cn"}}))
	require.Equal(t, 200, resp.StatusCode)

	resp = ts.Do(t, http.MethodGet, "/api/v1/admin/settings", nil)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	domains := m["data"].(map[string]any)["allowed_email_domains"].([]any)
	assert.Equal(t, "hit.edu.cn", domains[0])
}

func TestNormalUserForbidden(t *testing.T) {
	ts := testutil.New(t)

	assert.Equal(t, http.StatusForbidden, statusOf(t, ts, http.MethodGet, "/api/v1/admin/settings"))
	assert.Equal(t, http.StatusForbidden, statusOf(t, ts, http.MethodPut, "/api/v1/admin/settings"))
}

func statusOf(t *testing.T, ts *testutil.TestServer, method, path string) int {
	t.Helper()
	resp := ts.Do(t, method, path, mustJSON(t, map[string]any{}))
	return resp.StatusCode
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
