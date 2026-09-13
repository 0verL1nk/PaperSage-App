package store

import (
	"context"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// Open 打开数据库并对齐 schema（工厂：按 DSN scheme 分派方言）：
//   - PostgreSQL（自托管生产）：golang-migrate（含 Alembic 交接）+ 连接池
//   - MySQL / TiDB（云端生产，TiDB Cloud 走 mysql:// DSN）：同上，MySQL 方言迁移
//   - SQLite（开发/测试/E2E）：GORM AutoMigrate（create_all 兜底路径）
//
// 业务代码经 GORM + store.UUID 自定义类型隔离方言差异，不感知后端。
func Open(ctx context.Context, cfg *config.Settings) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.Debug {
		logLevel = logger.Info
	}

	switch {
	case isPostgresDSN(cfg.DSN()):
		return openSQL(ctx, cfg, logLevel, "postgres", postgres.Open(cfg.DSN()))
	case isMySQLDSN(cfg.DSN()):
		return openSQL(ctx, cfg, logLevel, "mysql", gormmysql.Open(toDriverDSN(cfg.DSN())))
	}

	db, err := gorm.Open(sqlite.Open(cfg.DSN()), &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return nil, err
	}
	return db, nil
}

// openSQL SQL 后端公共装配：先迁移（含交接探测）再开 GORM，池参数统一。
func openSQL(ctx context.Context, cfg *config.Settings, logLevel logger.LogLevel, dialect string, dialector gorm.Dialector) (*gorm.DB, error) {
	if err := MigrateUp(ctx, cfg.DSN()); err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 池参数对齐 Python 侧（pool 20 + overflow 30，recycle 1h）
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(20)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

func isPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// isMySQLDSN：mysql:// 或 tidb://（TiDB Cloud；两 scheme 等价处理）。
func isMySQLDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "mysql://") || strings.HasPrefix(dsn, "tidb://")
}
