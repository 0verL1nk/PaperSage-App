// Command generate — GORM 代码生成（SQL-first）。
//
// 输入：schema/tables/*.sql（每表一份 DDL，真相源）。
// 流程：SCHEMA_DSN 目标库上建临时库 → 执行全部表 SQL →
// gorm.io/gen 从真实 schema（information_schema）生成模型 + 类型安全
// DAO 到 internal/store/query。
//
// 改表流程：改 schema/tables/<t>.sql（或先加迁移跑 cmd/dumpschema 刷新）
// → SCHEMA_DSN=... make generate-dao → go build ./...
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gen"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const tablesDir = "schema/tables" // 相对 server/；本命令开头 chdir 到 internal/store 后用 ../../

func main() {
	dsn := os.Getenv("SCHEMA_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "generate: SCHEMA_DSN required (mysql://..., 临时库在其上创建)")
		os.Exit(1)
	}
	// gen 的输出路径相对进程 cwd 解析；切到 internal/store 使
	// OutPath="query" 落在 server/internal/store/query（import 路径由
	// go.mod 推导，不受 cwd 影响）
	if err := os.Chdir("internal/store"); err != nil {
		fatal("chdir internal/store: %v", err)
	}
	const outDir = "query"
	if err := os.RemoveAll(outDir); err != nil {
		fatal("%v", err)
	}

	// 1) 临时库 + 逐表执行 SQL（schema 文件即生成的唯一输入）
	cfg, err := gomysql.ParseDSN(goDriverDSN(dsn))
	if err != nil {
		fatal("parse dsn: %v", err)
	}
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		fatal("open: %v", err)
	}
	snap := fmt.Sprintf("gen_snap_%d", os.Getpid())
	if _, err := admin.Exec("CREATE DATABASE IF NOT EXISTS " + snap); err != nil {
		fatal("create snap db: %v", err)
	}
	defer func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + snap) }()

	files, err := filepath.Glob(filepath.Join("..", "..", tablesDir, "*.sql"))
	if err != nil {
		fatal("glob: %v", err)
	}
	// 依赖序建表（外键引用的表先建）；不在表中的排最后（字母序稳定）
	order := map[string]int{
		"users": 0, "project_groups": 1, "projects": 2,
		"folders": 3, "documents": 4, "latex_files": 5,
		"compile_jobs": 6, "user_settings": 7,
	}
	sort.Slice(files, func(i, j int) bool {
		bi, bj := filepath.Base(files[i]), filepath.Base(files[j])
		oi, okI := order[strings.TrimSuffix(bi, ".sql")]
		oj, okJ := order[strings.TrimSuffix(bj, ".sql")]
		if okI && okJ {
			return oi < oj
		}
		if okI != okJ {
			return okI
		}
		return bi < bj
	})
	for _, f := range files {
		ddl := strings.TrimSpace(mustRead(f))
		if ddl == "" {
			continue
		}
		if _, err := admin.Exec("USE " + snap); err != nil {
			fatal("use snap: %v", err)
		}
		if _, err := admin.Exec(ddl); err != nil {
			fatal("exec %s: %v", filepath.Base(f), err)
		}
	}

	// 2) gen 从真实 schema 生成（模型 + DAO）
	snapCfg := *cfg
	snapCfg.DBName = snap
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{DSNConfig: &snapCfg}), &gorm.Config{})
	if err != nil {
		fatal("gorm open snap: %v", err)
	}
	g := gen.NewGenerator(gen.Config{
		OutPath: outDir,
		Mode:    gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	g.UseDB(gdb)

	// DAO 绑定到域模型（store.*：带 BeforeCreate/TableName/关联）。
	// schema 一致性由 SQL 建库保证：gdb 即从 schema/tables/*.sql 建出的
	// 真实库，gen 读取其表名/类型做字段解析——SQL 与模型漂移会在
	// 生成物里立即暴露（编译期可见）
	g.ApplyBasic(store.AllModels()...)
	g.Execute()

	fmt.Printf("generated from %d SQL files → %s\n", len(files), outDir)
	fmt.Println("next: go build ./... （generated 模型在 query/model）")
}

func mustRead(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		fatal("read %s: %v", p, err)
	}
	return string(b)
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

func fatal(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "generate: "+f+"\n", a...)
	os.Exit(1)
}
