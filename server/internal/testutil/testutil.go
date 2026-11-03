// Package testutil 提供跨域 HTTP 契约测试的装配 helper。
package testutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/documents"
	"github.com/0verL1nk/GraduateProject/server/internal/email"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/projects"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/settings"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/store/query"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

// TestServer 一套装配好的 API + 预置用户。
type TestServer struct {
	*httptest.Server
	mailer          *swappableSender
	Engine          *gin.Engine // 调试路由表用
	CompileQueue    *latex.Queue
	ProjectService  *projects.Service
	DocumentService *documents.Service
	DB              *gorm.DB
	Token           string // 预置用户的 access token
	UID             uuid.UUID
}

// swappableSender 运行时可替换的邮件替身。
type swappableSender struct {
	mu    sync.Mutex
	inner email.Sender
}

func (s *swappableSender) Send(ctx context.Context, to, subject, html string) error {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	return inner.Send(ctx, to, subject, html)
}

// SetEmailSender 替换邮件实现（捕获器等）。
func (ts *TestServer) SetEmailSender(s email.Sender) { ts.mailer.Swap(s) }

func (s *swappableSender) Swap(inner email.Sender) {
	s.mu.Lock()
	s.inner = inner
	s.mu.Unlock()
}

// MakeSuperuser 把预置测试用户提升为 superuser（admin 路径测试用）。
func (ts *TestServer) MakeSuperuser(t *testing.T) {
	t.Helper()
	require.NoError(t, ts.DB.Model(&store.User{}).
		Where("id = ?", ts.UserID()).
		Update("is_superuser", true).Error)
}

// UserID 返回 store.UUID 形式的用户 ID。
func (ts *TestServer) UserID() store.UUID { return store.UUID{UUID: ts.UID} }

// New 装配全套域路由 + sqlite 内存库 + 预置用户（CLSI 指向不可达端口）。
func New(t *testing.T) *TestServer {
	t.Helper()
	return NewWithClsi(t, "http://127.0.0.1:9")
}

// NewWithClsi 指定 CLSI 基址（编译测试注入 httptest 假服务）。
func NewWithClsi(t *testing.T, clsiURL string) *TestServer {
	t.Helper()
	// :memory: 每个连接是独立空库；编译队列等并发路径会打开新连接，
	// 必须用共享内存库（file:...?mode=memory&cache=shared，测试间用唯一名隔离）
	dsn := fmt.Sprintf("file:papersage_test_%d_%d?mode=memory&cache=shared", os.Getpid(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	cfg := &config.Settings{SecretKey: "test", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7}
	// 可替换的邮件替身：注册制测试前 setEmailSender 换捕获器
	ts := &TestServer{}
	ts.mailer = &swappableSender{inner: email.Noop{}}
	mem := NewMemStorage()
	authSvc := auth.NewService(db, cfg, ts.mailer)
	latexFiles, err := latex.NewFilesService(db, mem)
	require.NoError(t, err)
	compileCfg := &config.Settings{
		SecretKey: "test", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7,
		LatexCLSIEnabled: true, LatexCLSITimeoutSeconds: 5,
		MaxConcurrentCompiles: 3, LatexCompileTimeout: 5,
		CompileJobMaxAgeHours: 24, CompileHistoryLimit: 20,
		CompileIncrementalSyncEnabled: true, S3ParallelFetchConcurrency: 8,
		S3PresignedURLTTLSeconds: 300,
	}
	compileSvc, err := latex.NewService(latex.NewClsiClient(clsiURL, compileCfg.LatexCLSITimeoutSeconds), compileCfg)
	require.NoError(t, err)
	compileQueue := latex.NewQueue(db, latexFiles, compileSvc, compileCfg)

	projSvc := projects.NewService(db, latex.CreateTemplate(latexFiles)) // 项目创建 → main.tex 模板
	ts.ProjectService = projSvc
	docSvc := documents.NewService(db, mem)
	ts.DocumentService = docSvc
	setSvc := settings.NewService(db)

	api := httpapi.New(nil)
	v1 := api.Group(httpapi.APIVersion)
	auth.Mount(v1, authSvc)
	projects.Mount(v1, authSvc.AuthRequired(), projSvc)
	writingstate.Mount(v1, authSvc.AuthRequired(), writingstate.NewService(db))
	documents.Mount(v1, authSvc.AuthRequired(), docSvc)
	settings.Mount(v1, authSvc.AuthRequired(), setSvc)
	syssettings.Mount(v1, authSvc.ChainMiddleware(auth.AdminRequired()), syssettings.NewService(db))
	ragStore, err2 := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	if err2 != nil {
		t.Fatalf("rag store: %v", err2)
	}
	query.SetDefault(db) // 生成 DAO 全局入口（与 bootstrap 对齐）
	agentSvc := agent.NewService(db, mem, latexFiles, cfg, ragStore)
	agent.Mount(v1, authSvc.AuthRequired(), agentSvc)
	latex.MountFiles(v1, authSvc.AuthRequired(), latexFiles)
	latex.MountCompile(v1, authSvc.AuthRequired(), compileSvc, compileQueue)

	uid := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	require.NoError(t, db.Create(&store.User{
		Username: "tester", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: store.UUID{UUID: uid}},
	}).Error)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	ts.Server = srv
	ts.Engine = api.Engine
	ts.CompileQueue = compileQueue
	ts.DB = db
	ts.Token = token
	ts.UID = uid
	return ts
}

// Do 发带认证的请求（body 非 nil 时按 JSON）。
func (ts *TestServer) Do(t *testing.T, method, path string, body []byte) *http.Response {
	return ts.DoWithHeaders(t, method, path, body, nil)
}

func (ts *TestServer) DoWithHeaders(t *testing.T, method, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// MemStorage 内存对象存储（测试替身）。
type MemStorage struct {
	objects map[string][]byte
}

// NewMemStorage creates an in-memory storage fake.
func NewMemStorage() *MemStorage { return &MemStorage{objects: map[string][]byte{}} }

// Put stores bytes.
func (m *MemStorage) Put(_ context.Context, key string, data []byte) error {
	m.objects[key] = append([]byte(nil), data...)
	return nil
}

// Get returns bytes.
func (m *MemStorage) Get(_ context.Context, key string) ([]byte, error) {
	d, ok := m.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return d, nil
}

// Delete removes a key.
func (m *MemStorage) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

// PresignGet fakes a URL.
func (m *MemStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://fake/" + key, nil
}

// List lists keys with prefix.
func (m *MemStorage) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for k := range m.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// Exists checks presence.
func (m *MemStorage) Exists(_ context.Context, key string) bool {
	_, ok := m.objects[key]
	return ok
}
