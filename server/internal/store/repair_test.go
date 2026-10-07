package store

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 回归：升级前建好的 SQLite 库上，AutoMigrate 既不回填 deleted_seq，也不重建同名的
// 唯一索引（只判 HasIndex(name)），于是「删除后重建同名文件」照旧撞唯一键——
// 也就是本次要修的那个 bug 在老开发库上根本没被修好。sqliteRepair 必须补上这两件事。
func TestSQLiteRepairFixesLegacyUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/legacy.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})

	// 旧 schema：两列唯一索引
	require.NoError(t, db.Exec(`CREATE TABLE latex_files (
		id TEXT PRIMARY KEY, project_id TEXT NOT NULL, file_path TEXT NOT NULL,
		deleted_at datetime, deleted_seq bigint NOT NULL DEFAULT 0)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX uq_project_file_path
		ON latex_files (project_id, file_path)`).Error)
	// 升级前就软删掉的一行：新列以 DEFAULT 0 加上来，仍占着 file_path
	require.NoError(t, db.Exec(`INSERT INTO latex_files (id, project_id, file_path, deleted_at, deleted_seq)
		VALUES ('old', 'p1', 'main.tex', '2026-01-01 00:00:00', 0)`).Error)

	require.NoError(t, sqliteRepair(db))

	var seq int64
	require.NoError(t, db.Raw(`SELECT deleted_seq FROM latex_files WHERE id = 'old'`).Scan(&seq).Error)
	assert.NotZero(t, seq, "升级前已软删的行必须回填 deleted_seq")

	var cols int
	require.NoError(t, db.Raw(`SELECT count(*) FROM pragma_index_info('uq_project_file_path')`).Scan(&cols).Error)
	assert.Equal(t, 3, cols, "唯一索引应重建为 (project_id, file_path, deleted_seq)")

	// 关键断言：现在能重建同名文件——修复前这一步会撞唯一键
	require.NoError(t, db.Exec(`INSERT INTO latex_files (id, project_id, file_path, deleted_seq)
		VALUES ('new', 'p1', 'main.tex', 0)`).Error, "修复后应能重建同名文件")

	// 幂等：再跑一次不报错、不改变结论
	require.NoError(t, sqliteRepair(db))
}
