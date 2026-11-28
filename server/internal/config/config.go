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
	// The TeX Live profile is embedded in the binary; only its on-disk cache is
	// configurable. The directory must survive releases, so it lives under the
	// writable state directory rather than next to the binary.
	LatexResourceCacheDir      string `env:"LATEX_RESOURCE_CACHE_DIR" envDefault:"./data/latex-resources"`
	LatexResourceCacheMaxBytes int64  `env:"LATEX_RESOURCE_CACHE_MAX_BYTES" envDefault:"2147483648"`

	MaxConcurrentCompiles int `env:"MAX_CONCURRENT_COMPILES" envDefault:"3"`
	LatexCompileTimeout   int `env:"LATEX_COMPILE_TIMEOUT" envDefault:"120"`
	// Browser compiles refresh their job with heartbeats; a running job that
	// stays silent longer than this is treated as abandoned and timed out.
	LatexBrowserJobStaleSeconds int `env:"LATEX_BROWSER_JOB_STALE_SECONDS" envDefault:"300"`
	CompileJobMaxAgeHours       int `env:"COMPILE_JOB_MAX_AGE_HOURS" envDefault:"24"`
	CompileHistoryLimit         int `env:"COMPILE_HISTORY_LIMIT" envDefault:"20"`
	// 浏览器编译错误诊断的保留天数（clamp 1–365）：到期硬删，防止表膨胀。
	CompileDiagnosticRetentionDays int `env:"COMPILE_DIAGNOSTIC_RETENTION_DAYS" envDefault:"30"`

	S3Endpoint                 string `env:"S3_ENDPOINT"`
	S3AccessKey                string `env:"S3_ACCESS_KEY"`
	S3SecretKey                string `env:"S3_SECRET_KEY"`
	S3Bucket                   string `env:"S3_BUCKET" envDefault:"papersage"`
	S3Region                   string `env:"S3_REGION" envDefault:"auto"`
	S3PublicURL                string `env:"S3_PUBLIC_URL"`
	S3ParallelFetchConcurrency int    `env:"S3_PARALLEL_FETCH_CONCURRENCY" envDefault:"8"`
	S3PresignedURLTTLSeconds   int    `env:"S3_PRESIGNED_URL_TTL_SECONDS" envDefault:"300"`

	CompileIncrementalSyncEnabled bool `env:"COMPILE_INCREMENTAL_SYNC_ENABLED" envDefault:"true"`

	AppURL                      string `env:"APP_URL" envDefault:"http://localhost:5173"` // 验证邮件链接前缀
	NeonDatabaseURL             string `env:"NEON_DATABASE_URL"`
	AgentAPIKey                 string `env:"AGENT_API_KEY"`
	AgentBaseURL                string `env:"AGENT_BASE_URL"`
	AgentModel                  string `env:"AGENT_MODEL" envDefault:"deepseek-chat"`
	AgentDocumentReadLimitBytes int    `env:"AGENT_DOCUMENT_READ_LIMIT_BYTES" envDefault:"0"` // 0 disables the per-read limit.
	AgentMaxIterations          int    `env:"AGENT_MAX_ITERATIONS" envDefault:"20"`           // Eino DeepAgent 显式上限，必须为正数
	// 动态写作建议：总开关与单段检查的迭代上限（控制成本与迟滞）。
	AgentSuggestionEnabled       bool `env:"AGENT_SUGGESTION_ENABLED" envDefault:"true"`
	AgentSuggestionMaxIterations int  `env:"AGENT_SUGGESTION_MAX_ITERATIONS" envDefault:"8"`
	// 跨章节检查：单元并发、单次检查总超时、单元迭代上限。
	ReviewUnitConcurrency   int           `env:"REVIEW_UNIT_CONCURRENCY" envDefault:"3"`
	ReviewRunTimeout        time.Duration `env:"REVIEW_RUN_TIMEOUT" envDefault:"20m"`
	ReviewUnitMaxIterations int           `env:"REVIEW_UNIT_MAX_ITERATIONS" envDefault:"12"`
	AgentModelTimeout       time.Duration `env:"AGENT_MODEL_TIMEOUT" envDefault:"2m"`      // 模型响应头超时，防止连接悬挂
	AgentModelIdleTimeout   time.Duration `env:"AGENT_MODEL_IDLE_TIMEOUT" envDefault:"5m"` // 流式空闲超时；0 关闭
	// 允许用户配置的 provider base_url 指向回环/私网（ollama、LM Studio 等本地模型）。
	// 默认关闭：生产环境里 base_url 由普通用户填写，开放后等于允许访问服务端所在网络。
	AgentAllowPrivateProviderURLs bool `env:"AGENT_ALLOW_PRIVATE_PROVIDER_URLS" envDefault:"false"`
	// 模型能力目录：后台刷新 models.dev 快照；置空可禁用（此时能力未知，由用户手填）。
	ModelRegistryURL          string `env:"MODEL_REGISTRY_URL" envDefault:"https://models.dev/api.json"`
	ModelRegistryRefreshHours int    `env:"MODEL_REGISTRY_REFRESH_HOURS" envDefault:"24"`
	DataDir                   string `env:"DATA_DIR" envDefault:"./data"`
	EmbedAPIKey               string `env:"EMBED_API_KEY"`
	EmbedBaseURL              string `env:"EMBED_BASE_URL"`
	EmbedModel                string `env:"EMBED_MODEL" envDefault:"BAAI/bge-m3"`
	SuperuserEmails           string `env:"SUPERUSER_EMAILS" envDefault:""` // bootstrap admin（逗号分隔邮箱）
	EmailProvider             string `env:"EMAIL_PROVIDER" envDefault:"resend"`
	FromEmail                 string `env:"FROM_EMAIL" envDefault:"noreply@papersage.dev"`
	FromName                  string `env:"FROM_NAME" envDefault:"PaperSage"`

	ResendAPIKey string `env:"RESEND_API_KEY"`
	ResendDomain string `env:"RESEND_DOMAIN"`

	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     int    `env:"SMTP_PORT" envDefault:"587"`
	SMTPUser     string `env:"SMTP_USER"`
	SMTPPassword string `env:"SMTP_PASSWORD"`
	SMTPTLS      bool   `env:"SMTP_TLS" envDefault:"true"`

	// 开发联调开关：开启后邮箱登录验证码会写入服务端日志，便于无邮箱访问时建号。
	// 生产必须保持 false——能读日志的人即可登录任意邮箱账号。
	DevLogEmailCode bool `env:"DEV_LOG_EMAIL_CODE" envDefault:"false"`
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
	if s.AgentSuggestionMaxIterations <= 0 {
		return nil, fmt.Errorf("AGENT_SUGGESTION_MAX_ITERATIONS must be positive")
	}
	if s.ReviewUnitConcurrency < 1 || s.ReviewUnitConcurrency > 4 {
		return nil, fmt.Errorf("REVIEW_UNIT_CONCURRENCY must be between 1 and 4")
	}
	if s.ReviewRunTimeout <= 0 {
		return nil, fmt.Errorf("REVIEW_RUN_TIMEOUT must be positive")
	}
	if s.ReviewUnitMaxIterations <= 0 {
		return nil, fmt.Errorf("REVIEW_UNIT_MAX_ITERATIONS must be positive")
	}
	if s.AgentModelTimeout <= 0 {
		return nil, fmt.Errorf("AGENT_MODEL_TIMEOUT must be positive")
	}
	if s.AgentModelIdleTimeout < 0 {
		return nil, fmt.Errorf("AGENT_MODEL_IDLE_TIMEOUT must not be negative")
	}
	if s.LatexResourceCacheMaxBytes <= 0 {
		return nil, fmt.Errorf("LATEX_RESOURCE_CACHE_MAX_BYTES must be positive")
	}
	if s.CompileDiagnosticRetentionDays < 1 {
		s.CompileDiagnosticRetentionDays = 1
	}
	if s.CompileDiagnosticRetentionDays > 365 {
		s.CompileDiagnosticRetentionDays = 365
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
