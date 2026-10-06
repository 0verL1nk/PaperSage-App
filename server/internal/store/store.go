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

	db, err := gorm.Open(sqlite.Open(cfg.DSN()), gormConfig(logLevel))
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return nil, err
	}
	if err := sqliteRepair(db); err != nil {
		return nil, err
	}
	return db, nil
}

// sqliteRepair 补上 AutoMigrate 覆盖不到的两件事（PG/MySQL 由迁移 SQL 处理）：
//
//  1. 新增列不会被回填：`deleted_seq` 以 DEFAULT 0 加上去，升级前就已软删的行
//     仍是 0，会和新行一起占住唯一键。
//  2. 索引按名字存在就不会重建：AutoMigrate 只判断 HasIndex(name)，而新旧唯一索引
//     同名（uq_project_file_path），于是老的两列索引会永久留存——删除后重建同名文件
//     依旧撞唯一键，正是本次要修的那个 bug。
//
// 因此这里显式回填并重建索引。幂等：已回填/已重建时为空操作。
func sqliteRepair(db *gorm.DB) error {
	if err := db.Exec(`UPDATE latex_files
		SET deleted_seq = CAST(strftime('%s', deleted_at) AS INTEGER) * 1000000
		WHERE deleted_at IS NOT NULL AND deleted_seq = 0`).Error; err != nil {
		return err
	}

	var cols int
	if err := db.Raw(`SELECT count(*) FROM pragma_index_info('uq_project_file_path')`).
		Scan(&cols).Error; err != nil {
		return err
	}
	if cols == 0 || cols >= 3 {
		return nil // 不存在（全新库交给 AutoMigrate）或已经是三列
	}
	if err := db.Exec(`DROP INDEX IF EXISTS uq_project_file_path`).Error; err != nil {
		return err
	}
	return db.Exec(`CREATE UNIQUE INDEX uq_project_file_path
		ON latex_files (project_id, file_path, deleted_seq)`).Error
}

// openSQL SQL 后端公共装配：先迁移（含交接探测）再开 GORM，池参数统一。
func openSQL(ctx context.Context, cfg *config.Settings, logLevel logger.LogLevel, dialect string, dialector gorm.Dialector) (*gorm.DB, error) {
	if err := MigrateUp(ctx, cfg.DSN()); err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, gormConfig(logLevel))
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

// gormConfig 统一 GORM 配置。
//
// NowFunc 固定为 UTC：GORM 的 autoCreateTime / autoUpdateTime 默认取 time.Now()
// （本地时区），而本项目约定业务时间一律以 UTC 写入（决策 D3）；不统一会让
// created_at / updated_at 的语义随部署机器的时区变化。
func gormConfig(logLevel logger.LogLevel) *gorm.Config {
	return &gorm.Config{
		Logger:  logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func isPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// isMySQLDSN：mysql:// 或 tidb://（TiDB Cloud；两 scheme 等价处理）。
func isMySQLDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "mysql://") || strings.HasPrefix(dsn, "tidb://")
}
