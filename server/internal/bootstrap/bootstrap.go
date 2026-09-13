// Package bootstrap 组装应用：一条 Build 函数完成「配置→存储→域服务→
// HTTP 引擎」的全部装配。cmd/papersage 只保留进程生命周期（信号/优雅
// 停机），组件装配全部收进这里——main.go 不再 import 一堆域包。
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/documents"
	"github.com/0verL1nk/GraduateProject/server/internal/email"
	"github.com/0verL1nk/GraduateProject/server/internal/extract"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/projects"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/settings"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
)

// App 装配产物。调用方只拿它需要的东西（Engine 起服务、Closer 收尾）。
type App struct {
	API *httpapi.API // 挂载完毕的 gin 引擎（含全部域路由）
	DB  *gorm.DB
}

// Build 按依赖序装配整个应用。
func Build(ctx context.Context, cfg *config.Settings) (*App, error) {
	// ── 存储 ──
	db, err := store.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	s3, err := storage.NewS3(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("s3 init: %w", err)
	}

	// ── 邮件（启动默认；运行时可被管理后台 email 组覆盖）──
	mailer := defaultMailer(cfg)

	// ── 域服务 ──
	authSvc := auth.NewService(db, cfg, mailer)
	latexFiles, err := latex.NewFilesService(db, s3)
	if err != nil {
		return nil, fmt.Errorf("latex files init: %w", err)
	}
	projSvc := projects.NewService(db, latex.CreateTemplate(latexFiles))

	docSvc := documents.NewService(db, s3)

	setSvc := settings.NewService(db)

	compileSvc, err := latex.NewService(latex.NewClsiClient(cfg.LatexCLSIBaseURL, cfg.LatexCLSITimeoutSeconds), cfg)
	if err != nil {
		return nil, fmt.Errorf("latex compile init: %w", err)
	}
	compileQueue := latex.NewQueue(db, latexFiles, compileSvc, cfg)
	compileQueue.RecoverOnStartup(ctx) // 对应 Python lifespan 的孤儿任务恢复

	// ── RAG（嵌入端点：管理后台 embed 组 → env；都未配置哈希兜底）──
	embedder := defaultEmbedder(cfg, db)
	ragStore, err := rag.NewStore(db, embedder, "")
	if err != nil {
		return nil, fmt.Errorf("rag store init: %w", err)
	}

	// 文献入库 → 索引钩子（documents 不 import rag：回调闭包解耦）
	docSvc.Indexer = &documents.DocIndexer{
		IndexDocument: indexDocumentHook(s3, ragStore),
	}

	agentSvc := agent.NewService(db, s3, compileQueue, cfg, ragStore)

	// ── HTTP ──
	engine := httpapi.New(nil)
	v1 := engine.Group(httpapi.APIVersion)
	adminMw := authSvc.ChainMiddleware(auth.AdminRequired())

	auth.Mount(v1, authSvc)
	projects.Mount(v1, authSvc.AuthRequired(), projSvc)
	documents.Mount(v1, authSvc.AuthRequired(), docSvc)
	settings.Mount(v1, authSvc.AuthRequired(), setSvc)
	syssettings.Mount(v1, adminMw, syssettings.NewService(db))
	latex.MountFiles(v1, authSvc.AuthRequired(), latexFiles)
	latex.MountCompile(v1, authSvc.AuthRequired(), compileSvc, compileQueue)
	agent.Mount(v1, authSvc.AuthRequired(), agentSvc)

	return &App{API: engine, DB: db}, nil
}

// defaultMailer 启动时的邮件 Provider（EMAIL_PROVIDER=resend/smtp/noop）。
func defaultMailer(cfg *config.Settings) email.Sender {
	switch cfg.EmailProvider {
	case "resend":
		if cfg.ResendAPIKey != "" {
			from := "PaperSage <noreply@" + cfg.ResendDomain + ">"
			return email.NewResend(cfg.ResendAPIKey, from, "https://api.resend.com")
		}
	case "smtp":
		if cfg.SMTPHost != "" {
			var a smtp.Auth
			if cfg.SMTPUser != "" {
				a = smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, hostOnly(cfg.SMTPHost))
			}
			return &email.SMTP{Host: cfg.SMTPHost, From: cfg.SMTPUser, Auth: a}
		}
	}
	return email.Noop{}
}

// defaultEmbedder 管理后台 embed 组优先，env 其次，哈希兜底。
func defaultEmbedder(cfg *config.Settings, db *gorm.DB) rag.Embedder {
	if e := syssettings.GetEmbed(db); e.APIKey != "" && e.BaseURL != "" {
		return &rag.HTTPEmbedder{BaseURL: e.BaseURL, APIKey: e.APIKey, Model: e.Model}
	}
	if cfg.EmbedAPIKey != "" && cfg.EmbedBaseURL != "" {
		return &rag.HTTPEmbedder{BaseURL: cfg.EmbedBaseURL, APIKey: cfg.EmbedAPIKey, Model: cfg.EmbedModel}
	}
	return rag.HashEmbedder{}
}

// indexDocumentHook 文献入库后的异步索引（抽取→索引，失败仅日志）。
func indexDocumentHook(s3 *storage.S3, ragStore *rag.Store) documents.IndexDocFn {
	return func(projectID, docID store.UUID, name, s3Key, contentType string) {
		ctx := context.Background()
		raw, err := s3.Get(ctx, s3Key)
		if err != nil {
			slog.Warn("rag: fetch doc failed", "key", s3Key, "err", err)
			return
		}
		text, err := extract.Text(ctx, raw, name, contentType, 0)
		if err != nil {
			slog.Warn("rag: extract failed", "name", name, "err", err)
			return
		}
		if _, err := ragStore.IndexSource(ctx, projectID, "document", docID, name, text); err != nil {
			slog.Warn("rag: index failed", "name", name, "err", err)
		}
	}
}

func hostOnly(hostport string) string {
	if i := strings.LastIndex(hostport, ":"); i > 0 {
		return hostport[:i]
	}
	return hostport
}
