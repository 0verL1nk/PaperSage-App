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
	"github.com/0verL1nk/GraduateProject/server/internal/store/query"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

// App 装配产物。调用方只拿它需要的东西（Engine 起服务、Closer 收尾）。
type App struct {
	API *httpapi.API // 挂载完毕的 gin 引擎（含全部域路由）
	DB  *gorm.DB
	// Close drains AI work; the process closes DB only after HTTP and AI return.
	Close func(context.Context) error
}

// Build 按依赖序装配整个应用。
func Build(ctx context.Context, cfg *config.Settings) (*App, error) {
	// ── 存储 ──
	db, err := store.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	// 生成 DAO（gorm.io/gen）的全局入口；域代码可用 query.Q.Documents 等
	query.SetDefault(db)

	s3, err := storage.NewS3(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("s3 init: %w", err)
	}

	// ── 邮件（启动默认；运行时可被管理后台 email 组覆盖）──
	mailer := defaultMailer(cfg)
	if degradedMailer(syssettings.GetEmail(db), cfg) {
		slog.Warn("email: Resend/SMTP not configured; account emails (verification, password reset) will not be delivered")
	}

	// ── 域服务 ──
	authSvc := auth.NewService(db, cfg, mailer)
	latexFiles, err := latex.NewFilesService(db, s3)
	if err != nil {
		return nil, fmt.Errorf("latex files init: %w", err)
	}
	projSvc := projects.NewService(db, latex.CreateTemplate(latexFiles))

	docSvc := documents.NewService(db, s3)

	setSvc := settings.NewService(db, cfg)

	compileSvc, err := latex.NewService(latex.NewClsiClient(cfg.LatexCLSIBaseURL, cfg.LatexCLSITimeoutSeconds), cfg)
	if err != nil {
		return nil, fmt.Errorf("latex compile init: %w", err)
	}
	compileQueue := latex.NewQueue(db, latexFiles, compileSvc, cfg)
	// Legacy CLSI jobs remain in the database for possible later reactivation,
	// but must not resume while browser compilation is the only supported path.
	// compileQueue.RecoverOnStartup(ctx)

	// ── RAG（嵌入端点：管理后台 embed 组 → env；都未配置哈希兜底）──
	embedder := defaultEmbedder(syssettings.GetEmbed(db), cfg)
	if embedderDegraded(embedder) {
		slog.Warn("embedding: no admin embed config or EMBED_* env; semantic retrieval degraded to bag-of-words (hash) fallback")
	}
	ragStore, err := rag.NewStore(db, embedder, "")
	if err != nil {
		return nil, fmt.Errorf("rag store init: %w", err)
	}
	// Neon(pgvector)：配置了 NEON_DATABASE_URL 时启用，向量检索下推 PG
	if cfg.NeonDatabaseURL != "" {
		if err := ragStore.AttachNeon(ctx, cfg.NeonDatabaseURL); err != nil {
			// 拉不起不阻断启动：降级主库路径，日志提示
			slog.Warn("rag: neon attach failed, falling back to primary DB", "err", err)
		}
	}

	// 文献入库 → 索引钩子（documents 不 import rag：回调闭包解耦）
	docSvc.Indexer = &documents.DocIndexer{
		IndexDocument: indexDocumentHook(s3, ragStore),
		DeleteDocument: func(projectID, docID store.UUID) {
			if err := ragStore.DeleteSource(context.Background(), projectID, "document", docID); err != nil {
				slog.Warn("rag: delete doc index failed", "doc", docID.String(), "err", err)
			}
		},
	}
	// 永久删除项目后清理对象存储与派生索引（软删不触发级联，必须显式处理）。
	projSvc.OnPurge = purgeProjectHook(db, s3, ragStore)

	agentSvc := agent.NewService(db, s3, latexFiles, cfg, ragStore)

	// ── HTTP ──
	engine := httpapi.New(nil)
	if err := latex.MountResources(engine.Engine, s3, cfg.LatexResourceProfile, cfg.LatexResourceRepository); err != nil {
		return nil, fmt.Errorf("latex resource resolver init: %w", err)
	}
	v1 := engine.Group(httpapi.APIVersion)
	adminMw := authSvc.ChainMiddleware(auth.AdminRequired())

	auth.Mount(v1, authSvc)
	projects.Mount(v1, authSvc.AuthRequired(), projSvc)
	writingstate.Mount(v1, authSvc.AuthRequired(), writingstate.NewService(db))
	documents.Mount(v1, authSvc.AuthRequired(), docSvc)
	settings.Mount(v1, authSvc.AuthRequired(), setSvc)
	syssettings.Mount(v1, adminMw, syssettings.NewService(db))
	latex.MountFiles(v1, authSvc.AuthRequired(), latexFiles)
	latex.MountCompile(v1, authSvc.AuthRequired(), compileSvc, compileQueue)
	agent.Mount(v1, authSvc.AuthRequired(), agentSvc)

	return &App{API: engine, DB: db, Close: agentSvc.Close}, nil
}

// defaultMailer 启动时的邮件 Provider（EMAIL_PROVIDER=resend/smtp/noop）。
func defaultMailer(cfg *config.Settings) email.Sender {
	switch cfg.EmailProvider {
	case "resend":
		// From 需要域名；缺域名的 key 发不出信，按未配置处理。
		if cfg.ResendAPIKey != "" && cfg.ResendDomain != "" {
			from := "PaperSage <noreply@" + cfg.ResendDomain + ">"
			return email.NewResend(cfg.ResendAPIKey, from, "https://api.resend.com")
		}
	case "smtp":
		// From/MAIL FROM 用 SMTP_USER；为空时多数中继会拒收。
		if cfg.SMTPHost != "" && cfg.SMTPUser != "" {
			a := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, hostOnly(cfg.SMTPHost))
			return &email.SMTP{Host: cfg.SMTPHost, From: cfg.SMTPUser, Auth: a}
		}
	}
	return email.Noop{}
}

// defaultEmbedder 管理后台 embed 组优先，env 其次，哈希兜底。
func defaultEmbedder(admin syssettings.EmbedSettings, cfg *config.Settings) rag.Embedder {
	if admin.APIKey != "" && admin.BaseURL != "" {
		return &rag.HTTPEmbedder{BaseURL: admin.BaseURL, APIKey: admin.APIKey, Model: admin.Model}
	}
	if cfg.EmbedAPIKey != "" && cfg.EmbedBaseURL != "" {
		return &rag.HTTPEmbedder{BaseURL: cfg.EmbedBaseURL, APIKey: cfg.EmbedAPIKey, Model: cfg.EmbedModel}
	}
	return rag.HashEmbedder{}
}

// degradedMailer 按运行时解析顺序判断有效邮件器是否退化为 Noop：
// 管理后台 email 组优先（resend 需要 key+domain；noop 表示显式关闭），
// 否则看启动配置。
func degradedMailer(admin syssettings.EmailSettings, cfg *config.Settings) bool {
	switch admin.Provider {
	case "resend":
		if admin.ResendAPIKey != "" && admin.ResendDomain != "" {
			return false
		}
	case "noop":
		return true
	}
	_, isNoop := defaultMailer(cfg).(email.Noop)
	return isNoop
}

// embedderDegraded 报告嵌入器是否退回了哈希（词袋）实现。
func embedderDegraded(e rag.Embedder) bool {
	switch e.(type) {
	case rag.HashEmbedder, *rag.HashEmbedder:
		return true
	}
	return false
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

// purgeProjectHook 清理已永久删除项目的对象与派生索引。行已删除，失败只记日志。
func purgeProjectHook(db *gorm.DB, s3 storage.Storage, ragStore *rag.Store) func(context.Context, store.UUID) {
	return func(ctx context.Context, projectID store.UUID) {
		var docs []store.Document
		if err := db.WithContext(ctx).Unscoped().Where("project_id = ?", projectID).Find(&docs).Error; err != nil {
			slog.Warn("purge: list documents failed", "project", projectID.String(), "err", err)
		}
		for _, d := range docs {
			if err := s3.Delete(ctx, d.S3Key); err != nil {
				slog.Warn("purge: delete document object failed", "key", d.S3Key, "err", err)
			}
		}
		var files []store.LatexFile
		if err := db.WithContext(ctx).Unscoped().Where("project_id = ?", projectID).Find(&files).Error; err != nil {
			slog.Warn("purge: list latex files failed", "project", projectID.String(), "err", err)
		}
		for _, f := range files {
			if f.S3Key == nil || *f.S3Key == "" {
				continue
			}
			if err := s3.Delete(ctx, *f.S3Key); err != nil {
				slog.Warn("purge: delete latex object failed", "key", *f.S3Key, "err", err)
			}
		}
		if err := ragStore.DeleteProject(ctx, projectID); err != nil {
			slog.Warn("purge: delete rag chunks failed", "project", projectID.String(), "err", err)
		}
	}
}

func hostOnly(hostport string) string {
	if i := strings.LastIndex(hostport, ":"); i > 0 {
		return hostport[:i]
	}
	return hostport
}
