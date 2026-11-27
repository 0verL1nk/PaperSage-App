package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 中性测试邮箱：公开仓严禁出现真实学校域名。
const (
	superuserTestEmail = "ExampleUser@example.edu.cn"
	otherTestEmail     = "someone@example.edu.cn"
)

func newSuperuserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	return db
}

func newUserWithEmail(t *testing.T, db *gorm.DB, username, email string) store.User {
	t.Helper()
	u := store.User{Username: username, PasswordHash: "x", Email: &email}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func TestIsSuperuserEmail(t *testing.T) {
	cases := []struct {
		name  string
		list  string
		email string
		want  bool
	}{
		{"exact match", "admin@example.edu.cn", "admin@example.edu.cn", true},
		{"list has spaces and uppercase", " Admin@Example.EDU.CN , ops@example.com ", "admin@example.edu.cn", true},
		{"address mixed case", "admin@example.edu.cn", "Admin@Example.EDU.CN", true},
		{"not listed", "admin@example.edu.cn", "other@example.edu.cn", false},
		{"empty list", "", "admin@example.edu.cn", false},
		{"empty address", "admin@example.edu.cn", "", false},
		{"different local part is not a match", "admin@example.edu.cn", "admin2@example.edu.cn", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSuperuserEmail(tc.list, tc.email))
		})
	}
}

func TestMaybePromoteSuperuserMatchesFullEmailCaseInsensitively(t *testing.T) {
	db := newSuperuserTestDB(t)
	// .env 里写成大写、带空格，库里的 email 本地部分大小写混合。
	svc := NewService(db, &config.Settings{SuperuserEmails: " exampleuser@EXAMPLE.EDU.CN "}, nil)
	user := newUserWithEmail(t, db, "exampleuser", superuserTestEmail)

	svc.maybePromoteSuperuser(context.Background(), &user)

	assert.True(t, user.IsSuperuser)
	var stored store.User
	require.NoError(t, db.First(&stored, "id = ?", user.ID).Error)
	assert.True(t, stored.IsSuperuser, "提权必须落库")
}

// 重启后启动同步必须让「已登录用户的旧 token」立即拿到管理员权限，
// 无需退出重登（authCheck 每请求重读用户行）。
func TestSyncSuperusersPromotesExistingSessionWithoutRelogin(t *testing.T) {
	db := newSuperuserTestDB(t)
	user := newUserWithEmail(t, db, "exampleuser", superuserTestEmail)
	other := newUserWithEmail(t, db, "someone", otherTestEmail)

	// 旧配置（名单为空）下签发的 token = 当前已登录用户。
	svc := NewService(db, &config.Settings{SecretKey: testSecret, AccessTokenExpireMinutes: 30}, nil)
	token, err := CreateVersionedAccessToken(testSecret, user.ID.String(), user.AuthVersion, 30)
	require.NoError(t, err)

	// 运维改 SUPERUSER_EMAILS（大小写混合 + 空格）并重启 → 启动同步。
	promoted, err := SyncSuperusers(context.Background(), db, " EXAMPLEUSER@Example.EDU.CN ")
	require.NoError(t, err)
	assert.EqualValues(t, 1, promoted)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	Mount(v1, svc)
	v1.GET("/admin/ping", svc.ChainMiddleware(AdminRequired()), func(c *gin.Context) { c.Status(http.StatusOK) })
	srv := httptest.NewServer(api)
	defer srv.Close()

	me := getWithToken(t, srv.URL+"/api/v1/auth/me", token)
	require.Equal(t, http.StatusOK, me.StatusCode)
	assert.Equal(t, true, bodyJSON(t, me)["data"].(map[string]any)["isSuperuser"])

	admin := getWithToken(t, srv.URL+"/api/v1/admin/ping", token)
	assert.Equal(t, http.StatusOK, admin.StatusCode, "管理接口应无需重登即可用")

	// 幂等：再次启动同步不再写入。
	promoted, err = SyncSuperusers(context.Background(), db, "exampleuser@example.edu.cn")
	require.NoError(t, err)
	assert.EqualValues(t, 0, promoted)

	// 不在名单的账号不受影响。
	var storedOther store.User
	require.NoError(t, db.First(&storedOther, "id = ?", other.ID).Error)
	assert.False(t, storedOther.IsSuperuser)
}

// 只提权不降权：把邮箱移出名单不会回收已授予的管理员权限。
func TestSyncSuperusersDoesNotDemote(t *testing.T) {
	db := newSuperuserTestDB(t)
	user := newUserWithEmail(t, db, "exampleuser", superuserTestEmail)
	require.NoError(t, db.Model(&store.User{}).Where("id = ?", user.ID).Update("is_superuser", true).Error)

	promoted, err := SyncSuperusers(context.Background(), db, "")
	require.NoError(t, err)
	assert.EqualValues(t, 0, promoted)

	var stored store.User
	require.NoError(t, db.First(&stored, "id = ?", user.ID).Error)
	assert.True(t, stored.IsSuperuser)
}
