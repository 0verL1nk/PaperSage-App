// Package migrations 将版本化 SQL 迁移文件嵌入二进制（golang-migrate iofs 源）。
// 按方言分目录：postgres/ 与 mysql/（TiDB 兼容 MySQL 方言）；
// SQLite 走 GORM AutoMigrate（无 SQL 迁移）。
package migrations

import (
	"embed"
	"fmt"
)

//go:embed postgres/*.sql
var postgresFS embed.FS

//go:embed mysql/*.sql
var mysqlFS embed.FS

// FS 导出为 golang-migrate 的 iofs 数据源（方言子目录）：
// source, err := iofs.New(migrations.FSFor("postgres"), "postgres")
func FSFor(dialect string) (embed.FS, string, error) {
	switch dialect {
	case "postgres":
		return postgresFS, "postgres", nil
	case "mysql":
		return mysqlFS, "mysql", nil
	default:
		return embed.FS{}, "", fmt.Errorf("migrations: unsupported dialect %q", dialect)
	}
}
