package agent_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
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
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 编译报错 AI 诊断：真模型桩（OpenAI 兼容端点），覆盖成功/失败/权限/限流，
// 并验证「应用修复」走现有 agent_change_proposals 决策路径真实写盘。

type diagnosisFixture struct {
	db        *gorm.DB
	stg       *testutil.MemStorage
	api       http.Handler
	token     string
	uid       store.UUID
	projectID store.UUID
	filePath  string
	s3Key     string
	content   string
}

func newDiagnosisFixture(t *testing.T, modelURL string, seed map[string]string) *diagnosisFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/diag.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	uid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{
		Username: "diagtester", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid},
	}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{
		Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid},
	}).Error)

	stg := testutil.NewMemStorage()
	f := &diagnosisFixture{db: db, stg: stg, uid: uid, projectID: pid, filePath: "main.tex"}
	for path, content := range seed {
		key := "projects/" + pid.String() + "/latex/" + path
		require.NoError(t, stg.Put(t.Context(), key, []byte(content)))
		hash := "h-" + path
		require.NoError(t, db.Create(&store.LatexFile{
			ProjectID: pid, FilePath: path, S3Key: &key,
			SizeBytes: int64(len(content)), ContentHash: &hash,
			SoftDeleteModel: store.SoftDeleteModel{ID: store.MustNewUUID()},
		}).Error)
		if path == "main.tex" {
			f.s3Key, f.content = key, content
		}
	}

	cfg := &config.Settings{SecretKey: "t", AgentAPIKey: "test-key", AgentBaseURL: modelURL, AgentModel: "test-model"}
	filesSvc, err := latex.NewFilesService(db, stg)
	require.NoError(t, err)
	svc := agent.NewService(db, stg, filesSvc, cfg, nil)
	authSvc := auth.NewService(db, cfg, nil)
	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), authSvc.AuthRequired(), svc)
	f.api = api
	f.token, err = auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	return f
}

func (f *diagnosisFixture) request(method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.api.ServeHTTP(rec, req)
	return rec
}

func (f *diagnosisFixture) diagnoseBody() []byte {
	return mustJSON(map[string]any{
		"filePath": f.filePath, "line": 3,
		"errorMessage": "Undefined control sequence.",
		"logExcerpt":   "--- pdflatex pass 1 ---\n! Undefined control sequence.\nl.3 \\badcommand",
		"compiler":     "pdflatex", "rootFile": "main.tex",
	})
}

func (f *diagnosisFixture) path(suffix string) string {
	return "/api/v1/projects/" + f.projectID.String() + "/agent/compile-diagnosis" + suffix
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

type diagnosisResponse struct {
	Code int    `json:"code"`
	Info string `json:"info"`
	Data struct {
		RootCause string  `json:"rootCause"`
		Notice    *string `json:"notice"`
		Patch     *struct {
			ProposalID string `json:"proposalId"`
			FilePath   string `json:"filePath"`
			OldText    string `json:"oldText"`
			NewText    string `json:"newText"`
			Note       string `json:"note"`
			Status     string `json:"status"`
		} `json:"patch"`
	} `json:"data"`
}

func decodeDiagnosis(t *testing.T, rec *httptest.ResponseRecorder) diagnosisResponse {
	t.Helper()
	var resp diagnosisResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	return resp
}

// fakeDiagnosisModel 返回固定的模型输出；status != 200 时模拟上游失败。
func fakeDiagnosisModel(t *testing.T, content string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = fmt.Fprint(w, `{"error":{"message":"upstream boom"}}`)
			return
		}
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			write := func(v any) {
				b, _ := json.Marshal(v)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
			write(map[string]any{"id": "c1", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": content}, "finish_reason": nil},
			}})
			write(map[string]any{"id": "c1", "object": "chat.completion.chunk", "choices": []any{
				map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"},
			}})
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1", "object": "chat.completion", "created": 1, "model": "test-model",
			"choices": []any{map[string]any{
				"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
}

func modelOutput(rootCause string, patch map[string]any) string {
	return string(mustJSON(map[string]any{"root_cause": rootCause, "patch": patch}))
}

func TestCompileDiagnosisHappyPathAndApply(t *testing.T) {
	content := "\\documentclass{article}\n\\begin{document}\n\\badcommand\n\\end{document}\n"
	model := fakeDiagnosisModel(t, modelOutput("未定义命令 \\badcommand，可能是拼写错误。", map[string]any{
		"file_path": "main.tex",
		"old_text":  "\\badcommand",
		"new_text":  "\\textbf{badcommand}",
		"note":      "换成 LaTeX 自带命令。",
	}), http.StatusOK)
	defer model.Close()

	f := newDiagnosisFixture(t, model.URL, map[string]string{"main.tex": content})
	rec := f.request(http.MethodPost, f.path(""), f.diagnoseBody())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	resp := decodeDiagnosis(t, rec)
	require.NotNil(t, resp.Data.Patch)
	assert.Contains(t, resp.Data.RootCause, "未定义命令")
	assert.Equal(t, "main.tex", resp.Data.Patch.FilePath)
	assert.Equal(t, "pending", resp.Data.Patch.Status)

	proposalUUID, err := uuid.Parse(resp.Data.Patch.ProposalID)
	require.NoError(t, err)
	proposalID := store.UUID{UUID: proposalUUID}
	var proposal store.AgentChangeProposal
	require.NoError(t, f.db.Where("id = ?", proposalID).First(&proposal).Error)
	assert.Equal(t, "pending", proposal.Status)
	assert.Nil(t, proposal.RunID)
	assert.Nil(t, proposal.ToolCallID)
	saved, err := f.stg.Get(t.Context(), f.s3Key)
	require.NoError(t, err)
	assert.Equal(t, content, string(saved), "诊断阶段不得改写稿件")

	// 走现有 apply 决策路径：重校验基准哈希后真实写盘。
	applyReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/agent/change-proposals/"+resp.Data.Patch.ProposalID+"/apply", bytes.NewReader([]byte("{}")))
	applyReq.Header.Set("Authorization", "Bearer "+f.token)
	applyRec := httptest.NewRecorder()
	f.api.ServeHTTP(applyRec, applyReq)
	require.Equal(t, http.StatusOK, applyRec.Code, applyRec.Body.String())

	saved, err = f.stg.Get(t.Context(), f.s3Key)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "\\textbf{badcommand}")
	require.NoError(t, f.db.Where("id = ?", proposalID).First(&proposal).Error)
	assert.Equal(t, "applied", proposal.Status)
}

func TestCompileDiagnosisRequiresModel(t *testing.T) {
	f := newDiagnosisFixture(t, "http://127.0.0.1:1", map[string]string{"main.tex": "x"})
	cfg := &config.Settings{SecretKey: "t"}
	authSvc := auth.NewService(f.db, cfg, nil)
	svc := agent.NewService(f.db, f.stg, nil, cfg, nil)
	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), authSvc.AuthRequired(), svc)
	req := httptest.NewRequest(http.MethodPost, f.path(""), bytes.NewReader(f.diagnoseBody()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, decodeDiagnosis(t, rec).Info, "尚未配置 AI 模型")
}

func TestCompileDiagnosisModelFailure(t *testing.T) {
	model := fakeDiagnosisModel(t, "", http.StatusInternalServerError)
	defer model.Close()
	f := newDiagnosisFixture(t, model.URL, map[string]string{"main.tex": "x"})
	rec := f.request(http.MethodPost, f.path(""), f.diagnoseBody())
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, decodeDiagnosis(t, rec).Info, "AI 诊断暂时不可用")
}

func TestCompileDiagnosisUnparsableOutput(t *testing.T) {
	model := fakeDiagnosisModel(t, "这不是 JSON，只是一段解释。", http.StatusOK)
	defer model.Close()
	f := newDiagnosisFixture(t, model.URL, map[string]string{"main.tex": "x"})
	rec := f.request(http.MethodPost, f.path(""), f.diagnoseBody())
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, decodeDiagnosis(t, rec).Info, "模型未返回可用的修复建议")

	var count int64
	require.NoError(t, f.db.Model(&store.AgentChangeProposal{}).Count(&count).Error)
	assert.Zero(t, count, "不可解析的输出不得落库")
}

func TestCompileDiagnosisAnchorMismatchDropsPatch(t *testing.T) {
	model := fakeDiagnosisModel(t, modelOutput("根因说明。", map[string]any{
		"file_path": "main.tex",
		"old_text":  "这段原文并不存在于稿件中",
		"new_text":  "替换",
		"note":      "说明",
	}), http.StatusOK)
	defer model.Close()
	f := newDiagnosisFixture(t, model.URL, map[string]string{"main.tex": "\\documentclass{article}\n"})
	rec := f.request(http.MethodPost, f.path(""), f.diagnoseBody())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	resp := decodeDiagnosis(t, rec)
	assert.Nil(t, resp.Data.Patch)
	require.NotNil(t, resp.Data.Notice)
	assert.Contains(t, *resp.Data.Notice, "不一致")

	var count int64
	require.NoError(t, f.db.Model(&store.AgentChangeProposal{}).Count(&count).Error)
	assert.Zero(t, count, "锚点不匹配时不得落库")
}

func TestCompileDiagnosisRejectsForeignProject(t *testing.T) {
	other := store.MustNewUUID()
	f := newDiagnosisFixture(t, "http://127.0.0.1:1", map[string]string{"main.tex": "x"})
	require.NoError(t, f.db.Create(&store.User{
		Username: "other", PasswordHash: "x", BaseModel: store.BaseModel{ID: other},
	}).Error)
	otherProject := store.MustNewUUID()
	require.NoError(t, f.db.Create(&store.Project{
		Name: "other", OwnerID: other, BaseModel: store.BaseModel{ID: otherProject},
	}).Error)

	body := mustJSON(map[string]any{
		"filePath": "main.tex", "line": 1, "errorMessage": "x",
		"logExcerpt": "", "compiler": "pdflatex", "rootFile": "main.tex",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+otherProject.String()+"/agent/compile-diagnosis", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.api.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCompileDiagnosisPayloadTooLarge(t *testing.T) {
	f := newDiagnosisFixture(t, "http://127.0.0.1:1", map[string]string{"main.tex": "x"})
	body := mustJSON(map[string]any{
		"filePath": "main.tex", "line": 1, "errorMessage": "x",
		"logExcerpt": strings.Repeat("a", 70<<10), "compiler": "pdflatex", "rootFile": "main.tex",
	})
	rec := f.request(http.MethodPost, f.path(""), body)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestCompileDiagnosisValidation(t *testing.T) {
	f := newDiagnosisFixture(t, "http://127.0.0.1:1", map[string]string{"main.tex": "x"})
	body := mustJSON(map[string]any{
		"filePath": "main.tex", "line": 1, "errorMessage": "x",
		"logExcerpt": "", "compiler": "lualatex", "rootFile": "main.tex",
	})
	rec := f.request(http.MethodPost, f.path(""), body)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestCompileDiagnosisRateLimit(t *testing.T) {
	model := fakeDiagnosisModel(t, modelOutput("根因。", nil), http.StatusOK)
	defer model.Close()
	f := newDiagnosisFixture(t, model.URL, map[string]string{"main.tex": "x"})

	for i := 0; i < 10; i++ {
		rec := f.request(http.MethodPost, f.path(""), f.diagnoseBody())
		require.Equal(t, http.StatusOK, rec.Code, "request %d: %s", i+1, rec.Body.String())
	}
	rec := f.request(http.MethodPost, f.path(""), f.diagnoseBody())
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, decodeDiagnosis(t, rec).Info, "太频繁")
}
