package agent_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 建议应用是一条安全敏感路径：必须由服务端重新校验基准版本，
// 过期建议不得用旧基准覆盖新内容，且不得重复应用。
func TestProposalApplyReject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/prop.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})

	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "prop-a", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "prop-b", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "PP", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)

	cfg := &config.Settings{SecretKey: "t"}
	stg := testutil.NewMemStorage()
	files, ferr := latex.NewFilesService(db, stg)
	require.NoError(t, ferr)
	_, err = files.CreateFile(context.Background(), pid.UUID, "main.tex", "ABC DEF", uid.UUID)
	require.NoError(t, err)

	rgStore, rerr := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, rerr)
	svc := agent.NewService(db, stg, files, cfg, rgStore)

	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	otherToken, err := auth.CreateAccessToken(cfg.SecretKey, other.String(), 30)
	require.NoError(t, err)

	do := func(tok, method, path, body string) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	// 基准哈希取当下内容（与工具产出时一致）
	baseHash := md5Hex("ABC DEF")
	newProposal := func(oldText, newText, base string) store.AgentChangeProposal {
		p := store.AgentChangeProposal{
			ID: store.MustNewUUID(), ProjectID: pid, FilePath: "main.tex",
			BaseContentHash: base, OldText: oldText, NewText: newText, Status: "pending",
		}
		require.NoError(t, db.Create(&p).Error)
		return p
	}
	content := func() string {
		c, _, e := files.GetFileRaw(context.Background(), pid.UUID, "main.tex", uid.UUID)
		require.NoError(t, e)
		return string(c)
	}

	p1 := newProposal("ABC", "XYZ", baseHash)

	// 越权：他人不得操作该建议
	rec := do(otherToken, http.MethodPost,
		"/api/v1/agent/change-proposals/"+p1.ID.String()+"/apply", "")
	assert.Equal(t, 404, rec.Code, "越权应用应 404")

	// 正常应用
	rec = do(token, http.MethodPost, "/api/v1/agent/change-proposals/"+p1.ID.String()+"/apply", "")
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Equal(t, "XYZ DEF", content(), "建议应被应用")

	// 幂等：重复应用被拒
	rec = do(token, http.MethodPost, "/api/v1/agent/change-proposals/"+p1.ID.String()+"/apply", "")
	assert.Equal(t, 409, rec.Code, "重复应用应被拒")
	assert.Equal(t, "XYZ DEF", content())

	// 基准版本漂移：稿件已变 → 不得用旧基准覆盖新内容
	p2 := newProposal("XYZ", "QQQ", baseHash) // 基准仍是旧内容的哈希
	rec = do(token, http.MethodPost, "/api/v1/agent/change-proposals/"+p2.ID.String()+"/apply", "")
	assert.Equal(t, 409, rec.Code, "基准版本不匹配应 409")
	assert.Equal(t, "XYZ DEF", content(), "稿件不得被修改")
	var reloaded store.AgentChangeProposal
	require.NoError(t, db.Where("id = ?", p2.ID).First(&reloaded).Error)
	assert.Equal(t, "stale", reloaded.Status, "旧建议应标记为过期")

	// 驳回：稿件不变
	p3 := newProposal("XYZ", "ZZZ", md5Hex("XYZ DEF"))
	rec = do(token, http.MethodPost, "/api/v1/agent/change-proposals/"+p3.ID.String()+"/reject", "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "XYZ DEF", content(), "驳回不得改稿")

	// 列表：能按状态查到
	rec = do(token, http.MethodGet, "/api/v1/projects/"+pid.String()+"/agent/proposals?status=stale", "")
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "stale")
}

// P0 要求：助手不得直接改稿——工具集里不能有整文件覆盖的写盘工具。
func TestProjectToolsHasNoWriteTool(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/tools.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})
	uid, pid := store.MustNewUUID(), store.MustNewUUID()

	tools, err := agent.ProjectTools(db, testutil.NewMemStorage(),
		pid, uid, store.MustNewUUID())
	require.NoError(t, err)

	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		info, ierr := tl.Info(context.Background())
		require.NoError(t, ierr)
		names = append(names, info.Name)
	}
	assert.NotContains(t, names, "write_latex_file", "写盘工具必须已摘除")
	assert.Contains(t, names, "propose_edit", "应改为产出建议")
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
