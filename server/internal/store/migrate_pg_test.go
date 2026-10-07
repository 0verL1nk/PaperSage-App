//go:build integration

// 需要真实 PostgreSQL：CI integration job（TEST_PG_DSN）跑；
// 本地 `go test -tags=integration ./internal/store/` + DSN 亦可。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

func pgDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		dsn = "postgres://papersage:papersage_dev_password@localhost:5432/papersage_test"
	}
	return dsn
}

func pgSchemaDSN(dsn, schema string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "search_path=" + schema
}

// 全新库：MigrateUp 应建出全部 8 张表（真实校验 baseline SQL 语法）
func TestMigrateUpFreshDatabase(t *testing.T) {
	dsn := pgDSN(t)
	db, err := sql.Open("pgx/v5", dsn)
	require.NoError(t, err)
	defer db.Close()

	// 独立 schema 隔离
	schema := fmt.Sprintf("mig_test_%d", os.Getpid())
	_, err = db.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE") })

	// 同连接串指向该 schema：pgx 原生支持 search_path 运行时参数，
	// 与 migrate 的 pgx5 驱动行为一致（options=-c 在两条路径上解析不一致）
	schemaDSN := pgSchemaDSN(dsn, schema)
	require.NoError(t, MigrateUp(context.Background(), schemaDSN))

	// 期望值跟随嵌入迁移数自动演进：每张表数 = 基线 8 + 基线后新增表
	//（0002 refresh_tokens +1、0012 rag_index_generations +1、0013 agent 运行时 +9、0014 writing state +1、0015 import receipts +1；0016 仅加字段）。
	// 从迁移 FS 数版本数太绕，直接数业务表。
	var count int
	require.NoError(t, db.QueryRow(
		fmt.Sprintf("SELECT count(*) FROM information_schema.tables WHERE table_schema = '%s' AND table_name != 'schema_migrations'", schema)).Scan(&count))
	assert.Equal(t, 27, count, "业务表总数（两方言一致；新增迁移时同步更新）")

	// 幂等：再跑一次不报错（ErrNoChange 路径）
	require.NoError(t, MigrateUp(context.Background(), schemaDSN))
}

// Alembic 交接：预置业务表 + alembic_version → stamp 后接管
func TestMigrateUpAlembicHandover(t *testing.T) {
	dsn := pgDSN(t)
	db, err := sql.Open("pgx/v5", dsn)
	require.NoError(t, err)
	defer db.Close()

	schema := fmt.Sprintf("mig_handover_%d", os.Getpid())
	_, err = db.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP SCHEMA " + schema + " CASCADE") })

	// 模拟真实 Python create_all 遗留：应用完整 baseline 结构，再留下 alembic_version。
	// 不能用只有 id/name 的最小 projects 表，否则这不是可兼容的既有 schema，
	// 且 fresh baseline 的索引会在缺失 group_id 时失败。
	schemaDSN := pgSchemaDSN(dsn, schema)
	fixtureDB, err := sql.Open("pgx/v5", schemaDSN)
	require.NoError(t, err)
	fixtureDB.SetMaxOpenConns(1)
	defer fixtureDB.Close()
	baselineFS, baselineDir, err := migrations.FSFor("postgres")
	require.NoError(t, err)
	baselineSQL, err := baselineFS.ReadFile(path.Join(baselineDir, "0001_baseline.up.sql"))
	require.NoError(t, err)
	_, err = fixtureDB.Exec(string(baselineSQL))
	require.NoError(t, err)
	_, err = fixtureDB.Exec(`CREATE TABLE alembic_version (version_num varchar(32) NOT NULL)`)
	require.NoError(t, err)

	require.NoError(t, MigrateUp(context.Background(), schemaDSN))

	var version int64
	require.NoError(t, db.QueryRow(fmt.Sprintf(
		"SELECT version FROM %s.schema_migrations", schema)).Scan(&version))
	// 交接 stamp 在 baseline(1)，随后 Up 补齐基线后迁移 → 版本 = 最新
	//（0002 后为 2；写死会随迁移演进腐化，此处锚定「>baseline 且 ≤ 最新」）
	var maxVersion int64
	require.NoError(t, db.QueryRow(fmt.Sprintf(
		"SELECT max(version) FROM %s.schema_migrations", schema)).Scan(&maxVersion))
	assert.Equal(t, maxVersion, version, "交接后应推进到最新版本（stamp baseline + 补齐后续迁移）")
	assert.Greater(t, version, int64(1), "0002 之后版本应 > baseline")
}
