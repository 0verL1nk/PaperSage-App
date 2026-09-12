package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // 注册 postgres:// 驱动
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

// baselineVersion 是 baseline 迁移的版本号（golang-migrate 从 0001 起）。
const baselineVersion uint = 1

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

// MigrateUp 在 PostgreSQL 上执行启动迁移：探测交接状态 → 按需 stamp → up。
// dsn 为 Go 驱动格式（postgres://...）。
func MigrateUp(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx/v5", dsn)
	if err != nil {
		return fmt.Errorf("open for migration: %w", err)
	}
	defer func() { _ = db.Close() }()

	st, err := probeState(ctx, db)
	if err != nil {
		return err
	}
	if decideHandover(st) == actionStampBaseline {
		if err := stampBaseline(ctx, db); err != nil {
			return err
		}
	}
	return runMigrations(dsn)
}

// runMigrations 执行 golang-migrate up（需要真实 PG，由 CI 集成/E2E 覆盖）。
func runMigrations(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer func() { _ = src.Close() }()

	// pgx/v5 数据库驱动注册的 URL scheme 是 pgx5://（"postgres" scheme 属于
	// lib/pq 驱动）；database/sql 的 "pgx/v5" 驱动名与此无关
	migrateDSN := dsn
	if strings.HasPrefix(migrateDSN, "postgres://") {
		migrateDSN = "pgx5://" + strings.TrimPrefix(migrateDSN, "postgres://")
	} else if strings.HasPrefix(migrateDSN, "postgresql://") {
		migrateDSN = "pgx5://" + strings.TrimPrefix(migrateDSN, "postgresql://")
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

func probeState(ctx context.Context, db *sql.DB) (state, error) {
	var s state
	var err error
	if s.HasAlembic, err = tableExists(ctx, db, "alembic_version"); err != nil {
		return s, err
	}
	if s.HasSchemaMigrations, err = tableExists(ctx, db, "schema_migrations"); err != nil {
		return s, err
	}
	if s.HasUsers, err = tableExists(ctx, db, "users"); err != nil {
		return s, err
	}
	return s, nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
		name,
	).Scan(&exists)
	return exists, err
}

// stampBaseline 按 golang-migrate 的版本表形态记录当前版本（dirty=false）。
func stampBaseline(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint not null primary key, dirty boolean not null)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false) ON CONFLICT (version) DO NOTHING`, int64(baselineVersion))
	if err != nil {
		return fmt.Errorf("stamp baseline: %w", err)
	}
	return nil
}
