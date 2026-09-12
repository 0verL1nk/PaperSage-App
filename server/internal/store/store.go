package store

import (
	"context"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// Open 打开数据库并对齐 schema（工厂：按 DSN 选择驱动）：
//   - PostgreSQL（生产）：先执行 golang-migrate（含 Alembic 交接，见 MigrateUp），
//     连接池对齐 Python（pool_size 20 + max_overflow 30，recycle 1h）
//   - SQLite（测试/E2E）：GORM AutoMigrate，对应 Python 的 create_all 兜底路径
func Open(ctx context.Context, cfg *config.Settings) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.Debug {
		logLevel = logger.Info
	}

	if isPostgresDSN(cfg.DSN()) {
		dsn := cfg.DSN()
		if err := MigrateUp(ctx, dsn); err != nil {
			return nil, err
		}
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
		if err != nil {
			return nil, err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(50)
		sqlDB.SetMaxIdleConns(20)
		sqlDB.SetConnMaxLifetime(time.Hour)
		return db, nil
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

func isPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}
