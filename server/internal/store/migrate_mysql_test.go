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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// 数业务表（与 PG 对齐：0012 后 16 张 + 0013 的 agent 运行时 9 张 = 25，
	// 去掉 schema_migrations）
	var count int
	require.NoError(t, db.QueryRow(
		fmt.Sprintf("SELECT count(*) FROM information_schema.tables WHERE table_schema = '%s' AND table_name != 'schema_migrations'", schema)).Scan(&count))
	assert.Equal(t, 25, count, "两方言业务表数量必须一致")

	// 幂等
	require.NoError(t, MigrateUp(context.Background(), schemaDSN))
}

// 交接：预置业务表（无 alembic）→ stamp baseline 后接管
func TestMySQLMigrateUpHandover(t *testing.T) {
	dsn := mysqlDSN(t)
	db, err := sql.Open("mysql", toDriverDSN(dsn))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := fmt.Sprintf("mig_handover_%d", os.Getpid())
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS " + schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE IF EXISTS " + schema) })
	// 模拟既有业务库（Python create_all 时代的最小集：0004/0005 的
	// 外键会引用 users 与 projects，交接测试需预置两者）
	_, err = db.Exec(fmt.Sprintf(
		"CREATE TABLE %s.users (id char(32) PRIMARY KEY, username varchar(50) NOT NULL)", schema))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(
		"CREATE TABLE %s.projects (id char(32) PRIMARY KEY, name varchar(100) NOT NULL, owner_id char(32) NOT NULL)", schema))
	require.NoError(t, err)

	schemaDSN := rebindMySQLDatabase(dsn, schema)
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
