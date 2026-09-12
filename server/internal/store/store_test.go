package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// SQLite 路径：Open 即建表（对应 Python create_all 兜底）
func TestOpenSQLiteCreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	t.Setenv("DATABASE_URL", "sqlite:///"+dbPath)
	cfg, err := config.Load()
	require.NoError(t, err)

	db, err := Open(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { // Windows 下必须显式关闭句柄，否则 TempDir 清理失败
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	got := tables(t, db)
	assert.True(t, got["users"], "Open 后 users 表应存在")
}

// DSN 驱动判定
func TestIsPostgresDSN(t *testing.T) {
	assert.True(t, isPostgresDSN("postgres://u:p@h/db"))
	assert.True(t, isPostgresDSN("postgresql://u:p@h/db"))
	assert.False(t, isPostgresDSN("./e2e.db"))
}
