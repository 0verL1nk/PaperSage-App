package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"  // 注册 mysql:// 驱动（TiDB 兼容）
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // 注册 postgres:// 驱动
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

// baselineVersion 是 baseline 迁移的版本号（golang-migrate 从 0001 起）。
const baselineVersion uint = 1

// dialect 由 DSN scheme 推导（pg/postgres → postgres；mysql/tidb → mysql）。
func dialectOf(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return "postgres"
	case strings.HasPrefix(dsn, "mysql://"), strings.HasPrefix(dsn, "tidb://"):
		return "mysql"
	default:
		return ""
	}
}

// state 描述启动时数据库的交接状态（由 MigrateUp 探测）。
type state struct {
	HasAlembic          bool // alembic_version 表存在（Python 迁移管理过）
	HasSchemaMigrations bool // schema_migrations 表存在（golang-migrate 已接管）
	HasUsers            bool // 业务表已存在（alembic 或 create_all 建的）
}

type handoverAction int

const (
	actionFreshInstall  handoverAction = iota // 空库：直接 migrate up
	actionStampBaseline                       // 既有业务库：先记版本再 up（schema 不变）
	actionNormal                              // 已接管：直接 up（增量）
)

// decideHandover 决定接管既有数据库的策略。
// 说明：真实部署的 PG 库 schema 与模型一致（Python 侧靠 create_all 兜底建表，
// Alembic 链含 SQLite 形状迁移、无法在 PG 干净重放），因此存在业务表即视为
// baseline 已就位，无需数据迁移。
func decideHandover(s state) handoverAction {
	switch {
	case s.HasSchemaMigrations:
		return actionNormal
	case s.HasUsers:
		return actionStampBaseline
	default:
		return actionFreshInstall
	}
}

// MigrateUp 在 SQL 数据库（PostgreSQL / MySQL·TiDB）上执行启动迁移：
// 探测交接状态 → 按需 stamp → up。dsn 为 Go 驱动格式。
func MigrateUp(ctx context.Context, dsn string) error {
	dialect := dialectOf(dsn)
	if dialect == "" {
		return fmt.Errorf("migrate: unsupported DSN scheme: %s", dsn)
	}
	driverName := "pgx/v5"
	if dialect == "mysql" {
		driverName = "mysql"
	}
	db, err := sql.Open(driverName, toDriverDSN(dsn))
	if err != nil {
		return fmt.Errorf("open for migration: %w", err)
	}
	defer func() { _ = db.Close() }()

	st, err := probeState(ctx, db, dialect)
	if err != nil {
		return err
	}
	if decideHandover(st) == actionStampBaseline {
		if err := stampBaseline(ctx, db, dialect); err != nil {
			return err
		}
	}
	return runMigrations(dsn, dialect)
}

// toDriverDSN 转成 database/sql 驱动可用的形式。
// MySQL 系：golang-migrate 用 mysql:// scheme；database/sql 用
// go-sql-driver 的 DSN（host:port/user/pass?params），并强制
// parseTime/UTC/utf8mb4。tidb:// 视同 mysql://。
func toDriverDSN(dsn string) string {
	if dialectOf(dsn) != "mysql" {
		return dsn
	}
	rest := dsn
	for _, p := range []string{"mysql://", "tidb://"} {
		rest = strings.TrimPrefix(rest, p)
	}
	// mysql://user:pass@host:port/db?tls=... → go-sql-driver 格式
	// （user:pass@tcp(host:port)/db?...）
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		userinfo, hostpart := rest[:i], rest[i+1:]
		sep := "/"
		if j := strings.Index(hostpart, "/"); j >= 0 {
			hostonly, dbpart := hostpart[:j], hostpart[j+1:]
			hostpart = "tcp(" + hostonly + ")/" + dbpart
			_ = sep
		}
		rest = userinfo + "@" + hostpart
	}
	// 必需参数（幂等追加）。tidb_skip_isolation_level_check：golang-migrate
	// 默认设 SERIALIZABLE 隔离级，TiDB 报 8048——官方指引用该会话变量跳过
	//（MySQL 8 忽略未知系统变量，不破坏兼容）。
	if !strings.Contains(rest, "parseTime") {
		if strings.Contains(rest, "?") {
			rest += "&parseTime=true&loc=UTC&charset=utf8mb4&tidb_skip_isolation_level_check=1"
		} else {
			rest += "?parseTime=true&loc=UTC&charset=utf8mb4&tidb_skip_isolation_level_check=1"
		}
	}
	return rest
}

// runMigrations 执行 golang-migrate up（需要真实数据库，由 CI 集成/E2E 覆盖）。
func runMigrations(dsn, dialect string) error {
	fs, subdir, err := migrations.FSFor(dialect)
	if err != nil {
		return err
	}
	src, err := iofs.New(fs, subdir)
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer func() { _ = src.Close() }()

	migrateDSN := dsn
	switch {
	case strings.HasPrefix(migrateDSN, "postgres://"):
		// pgx/v5 数据库驱动注册的 URL scheme 是 pgx5://（"postgres" scheme
		// 属于 lib/pq 驱动）；database/sql 的 "pgx/v5" 驱动名与此无关
		migrateDSN = "pgx5://" + strings.TrimPrefix(migrateDSN, "postgres://")
	case strings.HasPrefix(migrateDSN, "postgresql://"):
		migrateDSN = "pgx5://" + strings.TrimPrefix(migrateDSN, "postgresql://")
	case strings.HasPrefix(migrateDSN, "mysql://"), strings.HasPrefix(migrateDSN, "tidb://"):
		// golang-migrate 的 mysql 驱动认 mysql:// scheme，但地址部分须为
		// go-sql-driver 格式（tcp(host:port)）；复用 toDriverDSN 的转换
		for _, p := range []string{"mysql://", "tidb://"} {
			migrateDSN = strings.TrimPrefix(migrateDSN, p)
		}
		migrateDSN = "mysql://" + toDriverDSN("mysql://"+migrateDSN)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateDSN)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func probeState(ctx context.Context, db *sql.DB, dialect string) (state, error) {
	var s state
	var err error
	if s.HasAlembic, err = tableExists(ctx, db, dialect, "alembic_version"); err != nil {
		return s, err
	}
	if s.HasSchemaMigrations, err = tableExists(ctx, db, dialect, "schema_migrations"); err != nil {
		return s, err
	}
	if s.HasUsers, err = tableExists(ctx, db, dialect, "users"); err != nil {
		return s, err
	}
	return s, nil
}

func tableExists(ctx context.Context, db *sql.DB, dialect, name string) (bool, error) {
	var exists bool
	var err error
	if dialect == "mysql" {
		// MySQL 无 schema 概念上的 public；table_schema 取当前库
		err = db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)`,
			name,
		).Scan(&exists)
	} else {
		err = db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
			name,
		).Scan(&exists)
	}
	return exists, err
}

// stampBaseline 按 golang-migrate 的版本表形态记录当前版本（dirty=false）。
func stampBaseline(ctx context.Context, db *sql.DB, dialect string) error {
	if dialect == "mysql" {
		if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint not null primary key, dirty boolean not null)`); err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}
		_, err := db.ExecContext(ctx,
			`INSERT IGNORE INTO schema_migrations (version, dirty) VALUES (?, false)`, int64(baselineVersion))
		if err != nil {
			return fmt.Errorf("stamp baseline: %w", err)
		}
		return nil
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint not null primary key, dirty boolean not null)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false) ON CONFLICT (version) DO NOTHING`, int64(baselineVersion))
	if err != nil {
		return fmt.Errorf("stamp baseline: %w", err)
	}
	return nil
}
