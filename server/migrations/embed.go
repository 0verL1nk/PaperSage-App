// Package migrations 将版本化 SQL 迁移文件嵌入二进制（golang-migrate iofs 源）。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// FS 导出为 golang-migrate 的 iofs 数据源：
// source, err := iofs.New(migrations.FS, ".")
