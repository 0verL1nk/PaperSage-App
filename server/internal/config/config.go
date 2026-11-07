package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Settings holds application settings loaded from environment variables.
// Env keys and defaults mirror backend/app/config.py 1:1 (PORT default is the
// single-binary serving port instead of uvicorn's 8000).
type Settings struct {
	DatabaseURL              string `env:"DATABASE_URL" envDefault:"postgresql+asyncpg://papersage:papersage_dev_password@localhost:5432/papersage"`
	DatabaseMigrationURL     string `env:"DATABASE_MIGRATION_URL"`
	RunDBMigrations          bool   `env:"RUN_DB_MIGRATIONS" envDefault:"true"`
	DBMaxOpenConns           int    `env:"DB_MAX_OPEN_CONNS" envDefault:"10"`
	DBMaxIdleConns           int    `env:"DB_MAX_IDLE_CONNS" envDefault:"2"`
	DBConnMaxLifetimeSeconds int    `env:"DB_CONN_MAX_LIFETIME_SECONDS" envDefault:"1800"`
	RedisURL                 string `env:"REDIS_URL"`

	SecretKey                string `env:"SECRET_KEY" envDefault:"dev-secret-key-change-in-production"`
	AccessTokenExpireMinutes int    `env:"ACCESS_TOKEN_EXPIRE_MINUTES" envDefault:"30"`
	RefreshTokenExpireDays   int    `env:"REFRESH_TOKEN_EXPIRE_DAYS" envDefault:"7"`

	Debug        bool     `env:"DEBUG"`
	AllowedHosts []string `env:"ALLOWED_HOSTS" envSeparator:"," envDefault:"localhost,127.0.0.1"`

	Host string `env:"HOST" envDefault:"0.0.0.0"`
	Port int    `env:"PORT" envDefault:"5478"`

	// CLSI stays in source for possible reactivation, but is disabled by default
	// while PaperSage moves LaTeX compilation into browser workers.
	LatexCLSIEnabled        bool   `env:"LATEX_CLSI_ENABLED" envDefault:"false"`
	LatexCLSIBaseURL        string `env:"LATEX_CLSI_BASE_URL" envDefault:"http://localhost:3013"`
	LatexCLSITimeoutSeconds int    `env:"LATEX_CLSI_TIMEOUT_SECONDS" envDefault:"120"`
	// LatexResourceProfile identifies one immutable, catalog-backed TeX Live tree.
	LatexResourceProfile string `env:"LATEX_RESOURCE_PROFILE" envDefault:"tl2026-swift-ef78b612-r80534-v7"`
	// LatexResourceRepository is the HTTPS TeX Live tlnet repository used for on-demand package archives.
	LatexResourceRepository string `env:"LATEX_RESOURCE_REPOSITORY" envDefault:"https://mirror.ctan.org/systems/texlive/tlnet"`

	MaxConcurrentCompiles int `env:"MAX_CONCURRENT_COMPILES" envDefault:"3"`
	LatexCompileTimeout   int `env:"LATEX_COMPILE_TIMEOUT" envDefault:"120"`
	CompileJobMaxAgeHours int `env:"COMPILE_JOB_MAX_AGE_HOURS" envDefault:"24"`
	CompileHistoryLimit   int `env:"COMPILE_HISTORY_LIMIT" envDefault:"20"`

	S3Endpoint                 string `env:"S3_ENDPOINT"`
	S3AccessKey                string `env:"S3_ACCESS_KEY"`
	S3SecretKey                string `env:"S3_SECRET_KEY"`
	S3Bucket                   string `env:"S3_BUCKET" envDefault:"papersage"`
	S3Region                   string `env:"S3_REGION" envDefault:"auto"`
	S3PublicURL                string `env:"S3_PUBLIC_URL"`
	S3ParallelFetchConcurrency int    `env:"S3_PARALLEL_FETCH_CONCURRENCY" envDefault:"8"`
	S3PresignedURLTTLSeconds   int    `env:"S3_PRESIGNED_URL_TTL_SECONDS" envDefault:"300"`

	CompileIncrementalSyncEnabled bool `env:"COMPILE_INCREMENTAL_SYNC_ENABLED" envDefault:"true"`

	AppURL                      string        `env:"APP_URL" envDefault:"http://localhost:5173"` // 验证邮件链接前缀
	NeonDatabaseURL             string        `env:"NEON_DATABASE_URL"`
	AgentAPIKey                 string        `env:"AGENT_API_KEY"`
	AgentBaseURL                string        `env:"AGENT_BASE_URL"`
	AgentModel                  string        `env:"AGENT_MODEL" envDefault:"deepseek-chat"`
	AgentDocumentReadLimitBytes int           `env:"AGENT_DOCUMENT_READ_LIMIT_BYTES" envDefault:"0"` // 0 disables the per-read limit.
	AgentMaxIterations          int           `env:"AGENT_MAX_ITERATIONS" envDefault:"20"`           // Eino DeepAgent 显式上限，必须为正数
	AgentModelTimeout           time.Duration `env:"AGENT_MODEL_TIMEOUT" envDefault:"2m"`            // 模型响应头超时，防止连接悬挂
	DataDir                     string        `env:"DATA_DIR" envDefault:"./data"`
	EmbedAPIKey                 string        `env:"EMBED_API_KEY"`
	EmbedBaseURL                string        `env:"EMBED_BASE_URL"`
	EmbedModel                  string        `env:"EMBED_MODEL" envDefault:"BAAI/bge-m3"`
	SuperuserEmails             string        `env:"SUPERUSER_EMAILS" envDefault:""` // bootstrap admin（逗号分隔邮箱）
	EmailProvider               string        `env:"EMAIL_PROVIDER" envDefault:"resend"`
	FromEmail                   string        `env:"FROM_EMAIL" envDefault:"noreply@papersage.dev"`
	FromName                    string        `env:"FROM_NAME" envDefault:"PaperSage"`

	ResendAPIKey string `env:"RESEND_API_KEY"`
	ResendDomain string `env:"RESEND_DOMAIN"`

	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     int    `env:"SMTP_PORT" envDefault:"587"`
	SMTPUser     string `env:"SMTP_USER"`
	SMTPPassword string `env:"SMTP_PASSWORD"`
	SMTPTLS      bool   `env:"SMTP_TLS" envDefault:"true"`
}

// Load loads settings from the environment. A .env file next to the working
// directory is merged in first (missing file is not an error), mirroring the
// Pydantic Settings env_file behavior.
func Load() (*Settings, error) {
	_ = godotenv.Load()

	s, err := env.ParseAs[Settings]()
	if err != nil {
		return nil, err
	}
	if s.AgentDocumentReadLimitBytes < 0 {
		return nil, fmt.Errorf("AGENT_DOCUMENT_READ_LIMIT_BYTES must be non-negative")
	}
	if s.AgentMaxIterations <= 0 {
		return nil, fmt.Errorf("AGENT_MAX_ITERATIONS must be positive: Eino treats non-positive values as its built-in default")
	}
	if s.AgentModelTimeout <= 0 {
		return nil, fmt.Errorf("AGENT_MODEL_TIMEOUT must be positive")
	}
	// Python's parse_allowed_hosts strips whitespace and drops empty items.
	s.AllowedHosts = normalizeHosts(s.AllowedHosts)
	return &s, nil
}

func normalizeHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// DSN returns the database URL normalized for Go drivers.
func (s *Settings) DSN() string {
	return NormalizeDatabaseURL(s.DatabaseURL)
}

// MigrationDSN prefers a direct database connection for one-off schema changes.
// Runtime requests may use a provider pooler via DATABASE_URL.
func (s *Settings) MigrationDSN() string {
	if strings.TrimSpace(s.DatabaseMigrationURL) != "" {
		return NormalizeDatabaseURL(strings.TrimSpace(s.DatabaseMigrationURL))
	}
	return s.DSN()
}

// NormalizeDatabaseURL converts SQLAlchemy-style URLs to Go driver style:
// postgresql+asyncpg:// / postgresql+psycopg:// / postgresql:// → postgres://,
// sqlite+aiosqlite:///path / sqlite:///path → bare file path.
func NormalizeDatabaseURL(raw string) string {
	for _, prefix := range []string{"postgresql+asyncpg://", "postgresql+psycopg://", "postgresql://"} {
		if strings.HasPrefix(raw, prefix) {
			return "postgres://" + strings.TrimPrefix(raw, prefix)
		}
	}
	for _, prefix := range []string{"sqlite+aiosqlite:///", "sqlite:///"} {
		if strings.HasPrefix(raw, prefix) {
			return strings.TrimPrefix(raw, prefix)
		}
	}
	return raw
}
