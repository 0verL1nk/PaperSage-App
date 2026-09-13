// Command dumpschema — 全表结构快照导出。
//
// 对目标库（DSOS 环境变量 SCHEMA_DSN，MySQL/TiDB 方言）执行：
// 建临时库 → 跑全部迁移 → SHOW CREATE TABLE 逐表输出。
// 产物提交到 server/schema/，作为「所有表」的权威记录；
// 真相源仍是 migrations/（改模型→加迁移→重跑本命令刷新快照）。
//
// 用法：
//
//	SCHEMA_DSN='mysql://user:pass@host:4000/test?tls=true' go run ./cmd/dumpschema > schema.mysql.sql
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

	var b strings.Builder
	fmt.Fprintf(&b, "-- PaperSage 全表结构快照（MySQL/TiDB 方言）\n")
	fmt.Fprintln(&b, "-- 由 cmd/dumpschema 生成：全部迁移（migrations/ 全量）执行后的终态。")
	fmt.Fprintf(&b, "-- 真相源是 migrations/；改模型请加迁移并重跑 make schema。\n\n")
	for _, t := range tables {
		var name, ddl string
		if err := admin.QueryRow("SHOW CREATE TABLE "+snap+"."+t).Scan(&name, &ddl); err != nil {
			fatal("show create %s: %v", t, err)
		}
		fmt.Fprintln(&b, ddl+";")
		fmt.Fprintln(&b)
	}
	fmt.Print(b.String())
}

// goDriverDSN: mysql://u:p@h:port/db?x → go-sql-driver 形式（复用 store 的
// 转换规则太重，这里精简实现）。
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
