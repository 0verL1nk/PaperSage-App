package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 持久化契约（用户需求：重启不丢登录态）：
// refresh token 注册表必须落在 DB 而非进程内存——服务重建（重启模拟）
// 后 refresh 仍有效、轮换语义仍成立。

// newAuthTestServerOnDB：在既有 DB 上重建整套服务（内存缓存为全新实例，
// 等价于进程重启后的状态）。
func newAuthTestServerOnDB(t *testing.T, db *gorm.DB) *httptest.Server {
	t.Helper()
	svc := NewService(db, &config.Settings{
		SecretKey:                testSecret,
		AccessTokenExpireMinutes: 30,
		RefreshTokenExpireDays:   7,
	}, nil)
	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	Mount(v1, svc)
	return httptest.NewServer(api)
}

func TestRefreshSurvivesRestart(t *testing.T) {
	// 显式文件库：:memory: 库在两个 gorm.DB 间不共享
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/restart.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	// Windows：TempDir 清理会因 sqlite 句柄未释放而失败，显式关库
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	srv1 := newAuthTestServerOnDB(t, db)
	register(t, srv1.URL)
	data := loginOK(t, srv1.URL)
	oldRefresh := data["refreshToken"].(string)
	srv1.Close()

	// 「重启」：同库重建服务（NewService 无内存依赖）
	srv2 := newAuthTestServerOnDB(t, db)
	defer srv2.Close()

	resp := postJSON(t, srv2.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": oldRefresh})
	require.Equal(t, 200, resp.StatusCode, "重启后 refresh token 应仍然有效")
	newData := bodyJSON(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, newData["accessToken"])

	// 轮换语义跨重启保持：旧 token 已吊销
	resp = postJSON(t, srv2.URL+"/api/v1/auth/refresh", map[string]string{"refresh_token": oldRefresh})
	assert.Equal(t, 401, resp.StatusCode, "旧 refresh token 应已吊销")
}
