package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// refresh token family / 宽限 / 重放契约。
// 一次登录 = 一个 family；窗口内并发刷新平滑续签，窗口外重放整族吊销。

func refreshWith(t *testing.T, base, refresh string) *http.Response {
	t.Helper()
	return postJSON(t, base+"/api/v1/auth/refresh", map[string]string{"refresh_token": refresh})
}

func refreshData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	require.Equal(t, 200, resp.StatusCode)
	return bodyJSON(t, resp)["data"].(map[string]any)
}

func findRefreshRow(t *testing.T, srv *authServer, token string) store.RefreshToken {
	t.Helper()
	var row store.RefreshToken
	require.NoError(t, srv.db.Where("token_hash = ?", tokenHash(token)).First(&row).Error)
	return row
}

// backdateRevokedAt 把已轮换行的吊销时间拨到窗口外，不 sleep。
func backdateRevokedAt(t *testing.T, srv *authServer, token string, age time.Duration) {
	t.Helper()
	require.NoError(t, srv.db.Model(&store.RefreshToken{}).
		Where("token_hash = ?", tokenHash(token)).
		Update("revoked_at", time.Now().UTC().Add(-age)).Error)
}

func TestRefreshSameTokenWithinGraceWindowRenewsBothTabs(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	oldRefresh := loginOK(t, srv.URL)["refreshToken"].(string)

	// 两个标签页同时拿着同一个旧 token 刷新：第二个必须平滑续签，不能 401。
	tabA := refreshData(t, refreshWith(t, srv.URL, oldRefresh))
	tabB := refreshData(t, refreshWith(t, srv.URL, oldRefresh))

	oldRow := findRefreshRow(t, srv, oldRefresh)
	require.NotNil(t, oldRow.FamilyID, "轮换后旧行必须归属一个 family")
	require.NotNil(t, oldRow.ReplacedBy)

	rowA := findRefreshRow(t, srv, tabA["refreshToken"].(string))
	rowB := findRefreshRow(t, srv, tabB["refreshToken"].(string))
	require.NotNil(t, rowA.FamilyID)
	require.NotNil(t, rowB.FamilyID)
	assert.Equal(t, *oldRow.FamilyID, *rowA.FamilyID, "轮换继承同一 family")
	assert.Equal(t, *oldRow.FamilyID, *rowB.FamilyID, "宽限续签不得另起 family")

	// 两条分叉都保持可用（family 吊销兜底，而不是靠单行）。
	refreshData(t, refreshWith(t, srv.URL, tabA["refreshToken"].(string)))
	refreshData(t, refreshWith(t, srv.URL, tabB["refreshToken"].(string)))
}

func TestRefreshReplayOutsideGraceWindowRevokesFamily(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	oldRefresh := loginOK(t, srv.URL)["refreshToken"].(string)

	rotated := refreshData(t, refreshWith(t, srv.URL, oldRefresh))
	newRefresh := rotated["refreshToken"].(string)

	// 窗口外重放已轮换的旧 token：整族吊销。
	backdateRevokedAt(t, srv, oldRefresh, refreshGraceWindow+time.Second)
	resp := refreshWith(t, srv.URL, oldRefresh)
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "Refresh token has been revoked", bodyJSON(t, resp)["info"])

	// 该 family 下最新 token 也被吊销，不能再刷新。
	resp = refreshWith(t, srv.URL, newRefresh)
	assert.Equal(t, 401, resp.StatusCode, "重放后整族失效，包括最新 token")

	// 另一次登录（另一台设备）是独立 family，不受影响。
	other := loginOK(t, srv.URL)["refreshToken"].(string)
	refreshData(t, refreshWith(t, srv.URL, other))
}

func TestLogoutRevokesWholeFamily(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	oldRefresh := loginOK(t, srv.URL)["refreshToken"].(string)
	newRefresh := refreshData(t, refreshWith(t, srv.URL, oldRefresh))["refreshToken"].(string)

	resp := postJSON(t, srv.URL+"/api/v1/auth/logout", map[string]string{"refresh_token": newRefresh})
	require.Equal(t, 200, resp.StatusCode)

	resp = refreshWith(t, srv.URL, newRefresh)
	assert.Equal(t, 401, resp.StatusCode, "登出后当前 token 失效")
	// 同一 family 的兄弟 token 不得靠宽限期复活会话。
	resp = refreshWith(t, srv.URL, oldRefresh)
	assert.Equal(t, 401, resp.StatusCode, "登出后宽限期内也不能复活")
}

func TestRefreshSlidingTTLUsesConfiguredDays(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	srv.svc.Cfg.RefreshTokenExpireDays = 30
	register(t, srv.URL)
	oldRefresh := loginOK(t, srv.URL)["refreshToken"].(string)

	before := time.Now().UTC()
	newRefresh := refreshData(t, refreshWith(t, srv.URL, oldRefresh))["refreshToken"].(string)
	row := findRefreshRow(t, srv, newRefresh)

	want := before.Add(30 * 24 * time.Hour)
	assert.WithinDuration(t, want, row.ExpiresAt, 2*time.Minute, "每次刷新按配置重签全量 TTL")
}

func TestExpiredRefreshRowsAreCleanedUpLazily(t *testing.T) {
	srv := newAuthTestServer(t)
	defer srv.Close()
	register(t, srv.URL)
	var user store.User
	require.NoError(t, srv.db.Where("username = ?", "alice").First(&user).Error)

	stale := store.RefreshToken{
		TokenHash: tokenHash("stale-token"),
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}
	require.NoError(t, srv.db.Create(&stale).Error)

	// 登录路径触发惰性清理（每小时最多一次）。
	loginOK(t, srv.URL)

	var count int64
	require.NoError(t, srv.db.Model(&store.RefreshToken{}).
		Where("token_hash = ?", stale.TokenHash).Count(&count).Error)
	assert.Zero(t, count, "过期行应被清理，避免 30 天 TTL 下无界增长")
}
