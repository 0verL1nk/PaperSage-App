// Command generate — GORM 代码生成（gorm.io/gen）。
//
// 以 sqlite 内存库 AutoMigrate 出全部模型为真相源，生成类型安全
// DAO 到 internal/store/query。CRUD 统一走生成代码；改动模型后
// 重新运行：cd server && go run ./cmd/generate && go build ./...
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gen"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func main() {
	outDir := "internal/store/query"
	// 输出目录须先清空（gen 不覆盖旧产物，残留会编译失败）
	if err := os.RemoveAll(outDir); err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}

	// 内存库 AutoMigrate = 模型真相源（免外部依赖，可重复执行）
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fatal(err)
	}
	if err := db.AutoMigrate(store.AllModels()...); err != nil {
		fatal(err)
	}

	g := gen.NewGenerator(gen.Config{
		OutPath: outDir,
		Mode:    gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	g.UseDB(db)

	// 自定义类型映射：store.UUID 实现 Valuer/Scanner，按字段类型透传
	g.WithOpts(gen.FieldType("ID", "field.String"))

	// 全模型生成（表名来自 TableName()）
	g.ApplyBasic(store.AllModels()...)
	g.Execute()

	// 生成包缺少对本模块 store 包的 import 修正：gen 输出引用 model
	// 路径为生成时的包路径，校验一次编译即可发现；此命令最后打印提示
	abs, _ := filepath.Abs(outDir)
	fmt.Printf("generated DAO → %s\n", abs)
	fmt.Println("next: cd server && go build ./... （首次需修正 import 路径）")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "generate:", err)
	os.Exit(1)
}
