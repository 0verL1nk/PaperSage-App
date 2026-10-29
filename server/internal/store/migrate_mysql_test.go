//go:build integration

// 需要真实 MySQL（TiDB 兼容）：CI integration job（TEST_MYSQL_DSN）跑；
// 本地 `go test -tags=integration ./internal/store/ -run TestMySQLMigrate` + DSN。
// 与 migrate_pg_test.go 对偶，验证 MySQL 方言迁移全链路。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

func mysqlDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = "mysql://root:root@localhost:3306/papersage_test"
	}
	return dsn
}

// 全新库：MigrateUp 应建出全部业务表（真实校验 MySQL 方言 SQL 语法）
func TestMySQLMigrateUpFreshDatabase(t *testing.T) {
	dsn := mysqlDSN(t)
	db, err := sql.Open("mysql", toDriverDSN(dsn))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// 独立 schema（database）隔离
	schema := fmt.Sprintf("mig_test_%d", os.Getpid())
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS " + schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE IF EXISTS " + schema) })

	schemaDSN := rebindMySQLDatabase(dsn, schema)
	require.NoError(t, MigrateUp(context.Background(), schemaDSN), "MySQL 方言迁移应全部执行成功")

	// 0019 与 0021 各新增一张表；0020 只增加运行元数据列。
	var count int
	require.NoError(t, db.QueryRow(
		fmt.Sprintf("SELECT count(*) FROM information_schema.tables WHERE table_schema = '%s' AND table_name != 'schema_migrations'", schema)).Scan(&count))
	assert.Equal(t, 29, count, "两方言业务表数量必须一致")

	var version int64
	var dirty bool
	require.NoError(t, db.QueryRow(fmt.Sprintf("SELECT version, dirty FROM %s.schema_migrations", schema)).Scan(&version, &dirty))
	assert.Equal(t, int64(21), version)
	assert.False(t, dirty)

	// 已在部署版本上的重复发布不能重放 DDL。
	require.NoError(t, MigrateUp(context.Background(), schemaDSN))
}

// 交接：完整 baseline schema（无 alembic）→ stamp baseline 后接管。
func TestMySQLMigrateUpHandover(t *testing.T) {
	dsn := mysqlDSN(t)
	db, err := sql.Open("mysql", toDriverDSN(dsn))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := fmt.Sprintf("mig_handover_%d", os.Getpid())
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS " + schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE IF EXISTS " + schema) })
	schemaDSN := rebindMySQLDatabase(dsn, schema)
	fixtureDB, err := sql.Open("mysql", toDriverDSN(schemaDSN+"?multiStatements=true"))
	require.NoError(t, err)
	fixtureDB.SetMaxOpenConns(1)
	defer fixtureDB.Close()
	baselineFS, baselineDir, err := migrations.FSFor("mysql")
	require.NoError(t, err)
	baselineSQL, err := baselineFS.ReadFile(path.Join(baselineDir, "0001_baseline.up.sql"))
	require.NoError(t, err)
	_, err = fixtureDB.Exec(string(baselineSQL))
	require.NoError(t, err)

	require.NoError(t, MigrateUp(context.Background(), schemaDSN))

	var version int64
	require.NoError(t, db.QueryRow(fmt.Sprintf(
		"SELECT version FROM %s.schema_migrations", schema)).Scan(&version))
	assert.Greater(t, version, int64(1), "交接后应推进到最新版本")
}

// rebindMySQLDatabase 把 DSN 的库名替换为隔离库（mysql://u:p@h:port/db?x → .../schema?x）。
func rebindMySQLDatabase(dsn, schema string) string {
	// 找 host:port 之后第一个 '/' 之前的部分
	i := indexMySQLDBSeparator(dsn)
	if i < 0 {
		return dsn + "/" + schema
	}
	rest := dsn[i+1:]
	q := 0
	for j := 0; j < len(rest); j++ {
		if rest[j] == '?' {
			q = j
			break
		}
	}
	if q == 0 {
		q = len(rest)
	}
	return dsn[:i+1] + schema + rest[q:]
}

func indexMySQLDBSeparator(dsn string) int {
	at := -1
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == '@' {
			at = i
		}
	}
	if at < 0 {
		return -1
	}
	for i := at; i < len(dsn); i++ {
		if dsn[i] == '/' {
			return i
		}
	}
	return -1
}
