package store

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

// 契约来源：design.md D3（Alembic 交接策略）
func TestDecideHandover(t *testing.T) {
	cases := []struct {
		name string
		state
		want handoverAction
	}{
		{"全新空库", state{}, actionFreshInstall},
		{"Alembic 管理的既有库", state{HasAlembic: true, HasUsers: true}, actionStampBaseline},
		{"create_all 建的库（无 alembic_version）", state{HasUsers: true}, actionStampBaseline},
		{"已由 golang-migrate 接管", state{HasSchemaMigrations: true}, actionNormal},
		{"schema_migrations 与业务表并存", state{HasSchemaMigrations: true, HasUsers: true, HasAlembic: true}, actionNormal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, decideHandover(tc.state))
		})
	}
}

// 迁移必须已嵌入且命名可被 golang-migrate 解析（NNNN_name.up.sql）。
// 两方言版本号必须一一对齐（0001..000N），否则某方言升级会跳版本。
func TestEmbeddedMigrationsPresent(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql"} {
		f, subdir, err := migrations.FSFor(dialect)
		require.NoError(t, err)

		up, err := fs.ReadFile(f, subdir+"/0001_baseline.up.sql")
		require.NoError(t, err, dialect)
		if dialect == "postgres" {
			assert.Contains(t, string(up), "CREATE TABLE IF NOT EXISTS users")
		} else {
			assert.Contains(t, string(up), "CREATE TABLE users")
		}

		entries, err := fs.ReadDir(f, subdir)
		require.NoError(t, err)
		names := map[string]bool{}
		for _, e := range entries {
			names[e.Name()] = true
		}
		assert.Contains(t, names, "0001_baseline.up.sql")
		assert.Contains(t, names, "0001_baseline.down.sql")
	}

	// 版本对齐：两方言的 up 版本集合一致
	versions := func(f interface {
		ReadDir(string) ([]fs.DirEntry, error)
	}, dir string) map[string]bool {
		out := map[string]bool{}
		entries, _ := f.ReadDir(dir)
		for _, e := range entries {
			n := e.Name()
			if strings.HasSuffix(n, ".up.sql") {
				out[n[:4]] = true
			}
		}
		return out
	}
	pgFS, pgDir, _ := migrations.FSFor("postgres")
	myFS, myDir, _ := migrations.FSFor("mysql")
	assert.Equal(t, versions(pgFS, pgDir), versions(myFS, myDir),
		"postgres 与 mysql 迁移版本必须一一对齐")
}
