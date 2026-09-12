package store

import (
	"io/fs"
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

// baseline 迁移必须已嵌入且命名可被 golang-migrate 解析（NNNN_name.up.sql）
func TestEmbeddedMigrationsPresent(t *testing.T) {
	up, err := fs.ReadFile(migrations.FS, "0001_baseline.up.sql")
	require.NoError(t, err)
	assert.Contains(t, string(up), "CREATE TABLE IF NOT EXISTS users")

	entries, err := fs.ReadDir(migrations.FS, ".")
	require.NoError(t, err)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "0001_baseline.up.sql")
	assert.Contains(t, names, "0001_baseline.down.sql")
}
