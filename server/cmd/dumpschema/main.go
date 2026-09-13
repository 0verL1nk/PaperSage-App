// Command dumpschema — 逐表结构导出（schema/tables/*.sql）。
//
// 对目标库执行：建临时库 → 跑全部迁移 → 逐表 SHOW CREATE TABLE
// → 每表一个 SQL 写入 schema/tables/。这些文件是表结构的权威记录，
// 也是 cmd/generate 生成 DAO 的输入（SQL-first）。
// migrations/ 仍是演进通道：改表 = 加迁移 + 跑本命令刷新表文件 +
// 重新生成 DAO。
//
// 用法：
//
//	SCHEMA_DSN='mysql://user:pass@host:4000/test?tls=true' go run ./cmd/dumpschema
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func main() {
	dsn := os.Getenv("SCHEMA_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "dumpschema: SCHEMA_DSN required (mysql://...)")
		os.Exit(1)
	}

	// 解析 DSN → 换库名为临时快照库
	cfg, err := gomysql.ParseDSN(goDriverDSN(dsn))
	if err != nil {
		fatal("parse dsn: %v", err)
	}
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		fatal("open: %v", err)
	}
	snap := fmt.Sprintf("schema_snap_%d", os.Getpid())
	if _, err := admin.Exec("CREATE DATABASE IF NOT EXISTS " + snap); err != nil {
		fatal("create snap db: %v", err)
	}
	defer func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + snap) }()

	// 迁移到快照库
	cfg.DBName = snap
	snapDSN := rebind(dsn, snap)
	if err := store.MigrateUp(context.Background(), snapDSN); err != nil {
		fatal("migrate: %v", err)
	}

	// 逐表 SHOW CREATE TABLE
	rows, err := admin.Query(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name", snap)
	if err != nil {
		fatal("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var t string
		_ = rows.Scan(&t)
		tables = append(tables, t)
	}
	_ = rows.Close()

	// 逐表写入 schema/tables/<table>.sql（真相源；cmd/generate 基于它们生成 DAO）
	outDir := "schema/tables"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal("mkdir: %v", err)
	}
	written := 0
	for _, t := range tables {
		if t == "schema_migrations" {
			continue // golang-migrate 版本表，非业务表
		}
		var name, ddl string
		if err := admin.QueryRow("SHOW CREATE TABLE "+snap+"."+t).Scan(&name, &ddl); err != nil {
			fatal("show create %s: %v", t, err)
		}
		hdr := "-- " + t + " 表结构（MySQL/TiDB 方言，真相源）\n" +
			"-- cmd/generate 基于这些 SQL 建库并生成 DAO；改表先改这里，\n" +
			"-- 再 `make generate-dao SCHEMA_DSN=...` 重新生成。\n\n"
		if err := os.WriteFile(outDir+"/"+t+".sql",
			[]byte(hdr+ddl+";\n"), 0o644); err != nil {
			fatal("write %s: %v", t, err)
		}
		written++
	}
	fmt.Printf("wrote %d table files → %s\n", written, outDir)
}

// goDriverDSN: mysql://u:p@h:port/db?x → go-sql-driver 形式。
func goDriverDSN(dsn string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(dsn, "mysql://"), "tidb://")
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		userinfo, hostpart := rest[:i], rest[i+1:]
		if j := strings.Index(hostpart, "/"); j >= 0 {
			return userinfo + "@tcp(" + hostpart[:j] + ")/" + hostpart[j+1:]
		}
	}
	return rest
}

// rebind: 原 DSN 的库名替换为 snap。
func rebind(dsn, snap string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return dsn
	}
	rest := dsn[at+1:]
	if j := strings.Index(rest, "/"); j >= 0 {
		tail := rest[j+1:]
		q := strings.Index(tail, "?")
		if q >= 0 {
			return dsn[:at+1] + rest[:j] + "/" + snap + tail[q:]
		}
		return dsn[:at+1] + rest[:j] + "/" + snap
	}
	return dsn
}

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "dumpschema: "+f+"\n", a...)
	os.Exit(1)
}
