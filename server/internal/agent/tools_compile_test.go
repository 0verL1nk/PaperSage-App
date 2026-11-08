package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// #79：返回值必须以机器标记开头（前端正则依赖），并在其后说明模型读不到编译结果。
func TestCompileProjectReturnsBrowserRequestMarkerWithExplanation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/compile.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	owner, project := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: owner},
		Username: "compile-user", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&store.Project{BaseModel: store.BaseModel{ID: project},
		Name: "Compile", OwnerID: owner}).Error)
	s3Key := "projects/" + project.String() + "/latex/main.tex"
	require.NoError(t, db.Create(&store.LatexFile{
		ProjectID: project, FilePath: "main.tex", S3Key: &s3Key, SizeBytes: 5,
		SoftDeleteModel: store.SoftDeleteModel{ID: store.MustNewUUID()},
	}).Error)

	mem := testutil.NewMemStorage()
	require.NoError(t, mem.Put(context.Background(), s3Key, []byte("hello")))

	tools, err := agent.ProjectTools(db, mem, project, owner, store.MustNewUUID(), 0)
	require.NoError(t, err)
	var compile tool.InvokableTool
	for _, candidate := range tools {
		info, ierr := candidate.Info(context.Background())
		require.NoError(t, ierr)
		if info.Name == "compile_project" {
			compile, _ = candidate.(tool.InvokableTool)
		}
	}
	require.NotNil(t, compile, "compile_project tool is missing")

	out, err := compile.InvokableRun(context.Background(), "{}")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, "browser_compile_request:"), out)

	lines := strings.SplitN(out, "\n", 2)
	require.Len(t, lines, 2, "标记后应有给模型的人话说明")
	var req struct {
		RootFile string `json:"root_file"`
	}
	require.NoError(t, json.Unmarshal(
		[]byte(strings.TrimPrefix(lines[0], "browser_compile_request:")), &req))
	require.Equal(t, "main.tex", req.RootFile)
	require.Contains(t, lines[1], "写作页")
	require.Contains(t, lines[1], "无法读取")
}
