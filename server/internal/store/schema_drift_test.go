package store_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 漂移检查：逐表比对 Go 模型与 schema/tables/*.sql（SQL-first 真相源）的列集合。
//
// 为什么需要它：agent_wiki_pages 曾出现三方不一致——Go 模型声明并使用 meta，
// 而迁移与 schema 都没有该列（wiki_write 在生产必失败）；schema 又声明了
// 模型与迁移都没有的 deleted_at。这类问题在测试里看不见，因为测试直接按
// Go 模型 AutoMigrate，把与迁移的差异掩盖了。
var columnLine = regexp.MustCompile("^\\s*`([a-z0-9_]+)`\\s")

func sqlColumns(t *testing.T, table string) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "schema", "tables", table+".sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取真相源 %s 失败: %v", path, err)
	}
	cols := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if m := columnLine.FindStringSubmatch(line); m != nil {
			cols[m[1]] = true
		}
	}
	if len(cols) == 0 {
		t.Fatalf("%s 未解析出任何列，解析逻辑可能已失效", path)
	}
	return cols
}

func modelColumns(t *testing.T, model any) (string, map[string]bool) {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析模型 %T 失败: %v", model, err)
	}
	cols := map[string]bool{}
	for _, f := range s.Fields {
		// 关系字段不是列，GORM 不给它们 DBName。
		// 注意：不能按 Kind() == Struct 过滤——store.UUID 与 time.Time 本身就是
		// struct，那样会把 id / created_at 一并误杀。
		if f.DBName == "" || f.IgnoreMigration {
			continue
		}
		cols[f.DBName] = true
	}
	return s.Table, cols
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func TestSchemaDrift(t *testing.T) {
	// 空白导入确保 rag 注册其模型（store.RegisterModels）。
	_ = rag.WikiPage{}

	models := store.AllModels()
	if len(models) == 0 {
		t.Fatal("AllModels() 为空")
	}

	for _, m := range models {
		table, modelCols := modelColumns(t, m)
		sqlCols := sqlColumns(t, table)

		onlyModel := diff(modelCols, sqlCols)
		onlySQL := diff(sqlCols, modelCols)

		if len(onlyModel) > 0 {
			t.Errorf("表 %s：模型有而真相源没有的列 %v —— 模型写入该列会在 PG/MySQL 上报「列不存在」", table, onlyModel)
		}
		if len(onlySQL) > 0 {
			t.Errorf("表 %s：真相源有而模型没有的列 %v", table, onlySQL)
		}
	}
}

// SQLite 开发/E2E 路径只 AutoMigrate AllModels()。rag 包经 RegisterModels
// 注册自己的模型，缺这次注册的后果是 rag_chunks / agent_wiki_pages
// 根本不被创建，而索引失败仅记日志——RAG 静默失效且无人察觉。
func TestAllModelsCoversRagTables(t *testing.T) {
	_ = rag.WikiPage{} // 触发 rag 的 init() 注册

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/allmodels.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	for _, table := range []string{"rag_chunks", "agent_wiki_pages", "rag_index_generations"} {
		assert.Truef(t, db.Migrator().HasTable(table),
			"全新 SQLite 库应建立 %s（检查 rag 包是否仍注册其模型）", table)
	}
}
