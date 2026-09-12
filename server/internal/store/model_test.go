package store

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestDB 建内存 SQLite 并建全量表（对应 Python 测试的 sqlite+create_all 路径）
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(AllModels()...))
	return db
}

func tables(t *testing.T, db *gorm.DB) map[string]bool {
	t.Helper()
	var names []string
	require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type='table'").Scan(&names).Error)
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

func columns(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	type row struct{ Name string }
	var rows []row
	require.NoError(t, db.Raw("PRAGMA table_info("+table+")").Scan(&rows).Error)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

// 8 张表：users、user_settings、project_groups、projects、folders、
// documents、latex_files、compile_jobs
func TestAllTablesExist(t *testing.T) {
	got := tables(t, openTestDB(t))
	for _, want := range []string{
		"users", "user_settings", "project_groups", "projects",
		"folders", "documents", "latex_files", "compile_jobs",
	} {
		assert.True(t, got[want], "missing table %s (have %v)", want, got)
	}
}

func TestUsersColumns(t *testing.T) {
	got := columns(t, openTestDB(t), "users")
	for _, want := range []string{
		"id", "username", "password_hash", "email", "is_active", "is_superuser", "created_at", "updated_at",
	} {
		assert.Contains(t, got, want)
	}
}

func TestUsersUsernameUnique(t *testing.T) {
	db := openTestDB(t)
	u := User{Username: "dup", PasswordHash: "x"}
	require.NoError(t, db.Create(&u).Error)

	err := db.Create(&User{Username: "dup", PasswordHash: "y"}).Error
	require.Error(t, err, "username 必须唯一（对应 ix_users_username unique）")
}

// UUID 主键以 32 位无横线 hex 落库（Python TypeDecorator 的 SQLite 形态）
func TestUUIDStoredAsHexWithoutDashes(t *testing.T) {
	db := openTestDB(t)
	u := User{Username: "hex", PasswordHash: "x", BaseModel: BaseModel{ID: UUID{UUID: uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a7b")}}}
	require.NoError(t, db.Create(&u).Error)

	var raw string
	require.NoError(t, db.Raw("SELECT id FROM users WHERE username='hex'").Scan(&raw).Error)
	assert.Equal(t, "a0eebc999c0b4ef8bb6d6bb9bd380a7b", raw)

	var got User
	require.NoError(t, db.Where("username = ?", "hex").First(&got).Error)
	assert.Equal(t, u.ID, got.ID)
}

// created_at/updated_at 自动填充；更新时 updated_at 前进（对应 TimestampMixin default/onupdate）
func TestAutoTimestamps(t *testing.T) {
	db := openTestDB(t)
	u := User{Username: "ts", PasswordHash: "x"}
	require.NoError(t, db.Create(&u).Error)
	require.False(t, u.CreatedAt.IsZero())
	created := u.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	u.PasswordHash = "y"
	require.NoError(t, db.Save(&u).Error)
	assert.True(t, u.UpdatedAt.After(created), "updated_at 应在更新时前进")
}

// (project_id, file_path) 唯一约束：uq_project_file_path
func TestLatexFilesUniqueProjectPath(t *testing.T) {
	db := openTestDB(t)
	owner := User{Username: "o1", PasswordHash: "x"}
	require.NoError(t, db.Create(&owner).Error)
	p := Project{Name: "p", OwnerID: owner.ID}
	require.NoError(t, db.Create(&p).Error)

	f := LatexFile{ProjectID: p.ID, FilePath: "main.tex"}
	require.NoError(t, db.Create(&f).Error)
	err := db.Create(&LatexFile{ProjectID: p.ID, FilePath: "main.tex"}).Error
	require.Error(t, err, "同项目下 file_path 必须唯一")
}

// 外键级联语义：documents.folder_id SET NULL，其余 CASCADE
func TestForeignKeys(t *testing.T) {
	db := openTestDB(t)

	type fk struct {
		Table string
		From  string `gorm:"column:from"`
		To    string `gorm:"column:to"`
		OnDel string `gorm:"column:on_delete"`
	}
	var fks []fk
	require.NoError(t, db.Raw("PRAGMA foreign_key_list(documents)").Scan(&fks).Error)
	set := map[string]string{}
	for _, f := range fks {
		set[f.From] = f.OnDel
	}
	assert.Equal(t, "SET NULL", set["folder_id"], "folder_id 删除策略为 SET NULL")
	assert.Equal(t, "CASCADE", set["project_id"])
	assert.Equal(t, "CASCADE", set["owner_id"])

	require.NoError(t, db.Raw("PRAGMA foreign_key_list(projects)").Scan(&fks).Error)
	set = map[string]string{}
	for _, f := range fks {
		set[f.From] = f.OnDel
	}
	assert.Equal(t, "SET NULL", set["group_id"], "group_id 删除策略为 SET NULL")
	assert.Equal(t, "CASCADE", set["owner_id"])
}

// 编译任务状态枚举语义（CompileJobStatus.is_terminal 的 Go 对应）
func TestCompileJobStatusIsTerminal(t *testing.T) {
	assert.False(t, JobPending.IsTerminal())
	assert.False(t, JobRunning.IsTerminal())
	for _, s := range []JobStatus{JobSuccess, JobFailed, JobTimeout, JobCancelled} {
		assert.True(t, s.IsTerminal(), "%s 应为终态", s)
	}
	assert.Equal(t, "pending", JobPending.String())
}
