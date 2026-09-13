// Command papersage serves the PaperSage API (/api/v1) and, when built with
// the frontend assets present, the SPA from a single binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/cache"
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

func ragDir(cfg *config.Settings) string {
	if cfg.DataDir != "" {
		return cfg.DataDir + "/rag"
	}
	return "./data/rag"
}

func hostOnly(hostport string) string {
	if i := strings.LastIndex(hostport, ":"); i > 0 {
		return hostport[:i]
	}
	return hostport
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "papersage: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	_ = cfg

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           buildHandler(cfg, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr, "debug", cfg.Debug)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		logger.Info("server stopped")
		return nil
	}
}

// buildHandler 装配全部域路由（与 lifespan 对应的启动逻辑在 store.Open 内）。
func buildHandler(cfg *config.Settings, logger *slog.Logger) http.Handler {
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		logger.Error("database init failed", "error", err)
		os.Exit(1)
	}
	mem := cache.New(cfg.RedisURL)
	_ = mem.Connect(context.Background())

	// 邮件 Provider 装配（EMAIL_PROVIDER=resend/smtp/noop）
	var mailer email.Sender = email.Noop{}
	switch cfg.EmailProvider {
	case "resend":
		if cfg.ResendAPIKey != "" {
			from := "PaperSage <noreply@" + cfg.ResendDomain + ">"
			mailer = email.NewResend(cfg.ResendAPIKey, from, "https://api.resend.com")
		}
	case "smtp":
		if cfg.SMTPHost != "" {
			var auth smtp.Auth
			if cfg.SMTPUser != "" {
				auth = smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, hostOnly(cfg.SMTPHost))
			}
			mailer = &email.SMTP{Host: cfg.SMTPHost, From: cfg.SMTPUser, Auth: auth}
		}
	}
	authSvc := auth.NewService(db, cfg, mailer)
	s3, err := storage.NewS3(context.Background(), cfg)
	if err != nil {
		logger.Error("s3 init failed", "error", err)
		os.Exit(1)
	}
	latexFiles, err := latex.NewFilesService(db, s3)
	if err != nil {
		logger.Error("latex files init failed", "error", err)
		os.Exit(1)
	}
	compileSvc, err := latex.NewService(latex.NewClsiClient(cfg.LatexCLSIBaseURL, cfg.LatexCLSITimeoutSeconds), cfg)
	if err != nil {
		logger.Error("latex compile init failed", "error", err)
		os.Exit(1)
	}
	compileQueue := latex.NewQueue(db, latexFiles, compileSvc, cfg)
	compileQueue.RecoverOnStartup(context.Background()) // 对应 lifespan 的孤儿任务恢复

	api := httpapi.New(logger)
	v1 := api.Group(httpapi.APIVersion)
	auth.Mount(v1, authSvc)
	projects.Mount(v1, authSvc.AuthRequired(), projects.NewService(db, latex.CreateTemplate(latexFiles)))
	docSvc := documents.NewService(db, s3)
	settings.Mount(v1, authSvc.AuthRequired(), settings.NewService(db))
	syssettings.Mount(v1, authSvc.AuthRequired(), syssettings.NewService(db))
	// RAG：嵌入端点（env EMBED_*），未配置则用 HashEmbedder 兜底
	var embedder rag.Embedder = rag.HashEmbedder{Dim: 64}
	if cfg.EmbedAPIKey != "" && cfg.EmbedBaseURL != "" {
		embedder = &rag.HTTPEmbedder{BaseURL: cfg.EmbedBaseURL, APIKey: cfg.EmbedAPIKey, Model: cfg.EmbedModel}
	}
	ragStore, err := rag.NewStore(db, embedder, ragDir(cfg))
	if err != nil {
		logger.Error("rag store init failed", "error", err)
		os.Exit(1)
	}
	docSvc.Indexer = &documents.DocIndexer{
		IndexDocument: func(projectID, docID store.UUID, name, s3Key, contentType string) {
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
		},
	}
	agentSvc := agent.NewService(db, s3, compileQueue, cfg, ragStore)
	documents.Mount(v1, authSvc.AuthRequired(), docSvc)
	agent.Mount(v1, authSvc.AuthRequired(), agentSvc)
	latex.MountFiles(v1, authSvc.AuthRequired(), latexFiles)
	latex.MountCompile(v1, authSvc.AuthRequired(), compileSvc, compileQueue)
	return api
}
