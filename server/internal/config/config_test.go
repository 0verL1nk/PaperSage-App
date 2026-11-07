package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约来源：backend/app/config.py（Python 版默认值 1:1 移植）
func TestLoadDefaults(t *testing.T) {
	s, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "postgresql+asyncpg://papersage:papersage_dev_password@localhost:5432/papersage", s.DatabaseURL)
	assert.Equal(t, "", s.DatabaseMigrationURL)
	assert.True(t, s.RunDBMigrations)
	assert.Equal(t, 10, s.DBMaxOpenConns)
	assert.Equal(t, 2, s.DBMaxIdleConns)
	assert.Equal(t, 1800, s.DBConnMaxLifetimeSeconds)
	assert.Equal(t, "", s.RedisURL)
	assert.Equal(t, "dev-secret-key-change-in-production", s.SecretKey)
	assert.Equal(t, 30, s.AccessTokenExpireMinutes)
	assert.Equal(t, 7, s.RefreshTokenExpireDays)
	assert.False(t, s.Debug)
	assert.Equal(t, 0, s.AgentDocumentReadLimitBytes)
	assert.Equal(t, 20, s.AgentMaxIterations)
	assert.Equal(t, 2*time.Minute, s.AgentModelTimeout)
	assert.Equal(t, []string{"localhost", "127.0.0.1"}, s.AllowedHosts)
	assert.Equal(t, "0.0.0.0", s.Host)
	assert.Equal(t, 5478, s.Port)

	assert.False(t, s.LatexCLSIEnabled)
	assert.Equal(t, "http://localhost:3013", s.LatexCLSIBaseURL)
	assert.Equal(t, 120, s.LatexCLSITimeoutSeconds)
	assert.Equal(t, "tl2026-swift-ef78b612-r80534-v7", s.LatexResourceProfile)

	assert.Equal(t, 3, s.MaxConcurrentCompiles)
	assert.Equal(t, 120, s.LatexCompileTimeout)
	assert.Equal(t, 24, s.CompileJobMaxAgeHours)
	assert.Equal(t, 20, s.CompileHistoryLimit)

	assert.Equal(t, "", s.S3Endpoint)
	assert.Equal(t, "", s.S3AccessKey)
	assert.Equal(t, "", s.S3SecretKey)
	assert.Equal(t, "papersage", s.S3Bucket)
	assert.Equal(t, "auto", s.S3Region)
	assert.Equal(t, "", s.S3PublicURL)
	assert.Equal(t, 8, s.S3ParallelFetchConcurrency)
	assert.Equal(t, 300, s.S3PresignedURLTTLSeconds)

	assert.True(t, s.CompileIncrementalSyncEnabled)

	assert.Equal(t, "resend", s.EmailProvider)
	assert.Equal(t, "noreply@papersage.dev", s.FromEmail)
	assert.Equal(t, "PaperSage", s.FromName)
	assert.Equal(t, "", s.ResendAPIKey)
	assert.Equal(t, "", s.ResendDomain)
	assert.Equal(t, "", s.SMTPHost)
	assert.Equal(t, 587, s.SMTPPort)
	assert.Equal(t, "", s.SMTPUser)
	assert.Equal(t, "", s.SMTPPassword)
	assert.True(t, s.SMTPTLS)
}

func TestLoadDocumentReadLimit(t *testing.T) {
	t.Setenv("AGENT_DOCUMENT_READ_LIMIT_BYTES", "8192")
	s, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 8192, s.AgentDocumentReadLimitBytes)
	t.Setenv("AGENT_DOCUMENT_READ_LIMIT_BYTES", "0")
	s, err = Load()
	require.NoError(t, err)
	assert.Zero(t, s.AgentDocumentReadLimitBytes)
	t.Setenv("AGENT_DOCUMENT_READ_LIMIT_BYTES", "-1")
	_, err = Load()
	require.ErrorContains(t, err, "AGENT_DOCUMENT_READ_LIMIT_BYTES")
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql+asyncpg://u:p@db:5432/x")
	t.Setenv("DATABASE_MIGRATION_URL", "postgresql://u:p@direct:5432/x")
	t.Setenv("RUN_DB_MIGRATIONS", "false")
	t.Setenv("DB_MAX_OPEN_CONNS", "6")
	t.Setenv("DB_MAX_IDLE_CONNS", "1")
	t.Setenv("DB_CONN_MAX_LIFETIME_SECONDS", "900")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SECRET_KEY", "prod-key")
	t.Setenv("ACCESS_TOKEN_EXPIRE_MINUTES", "15")
	t.Setenv("REFRESH_TOKEN_EXPIRE_DAYS", "14")
	t.Setenv("PORT", "9000")
	t.Setenv("LATEX_CLSI_ENABLED", "false")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_BUCKET", "ci")
	t.Setenv("ALLOWED_HOSTS", "a.com, b.com ,")
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "localhost")
	t.Setenv("SMTP_TLS", "false")

	s, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "postgresql+asyncpg://u:p@db:5432/x", s.DatabaseURL)
	assert.Equal(t, "postgresql://u:p@direct:5432/x", s.DatabaseMigrationURL)
	assert.False(t, s.RunDBMigrations)
	assert.Equal(t, 6, s.DBMaxOpenConns)
	assert.Equal(t, 1, s.DBMaxIdleConns)
	assert.Equal(t, 900, s.DBConnMaxLifetimeSeconds)
	assert.Equal(t, "redis://localhost:6379/0", s.RedisURL)
	assert.Equal(t, "prod-key", s.SecretKey)
	assert.Equal(t, 15, s.AccessTokenExpireMinutes)
	assert.Equal(t, 14, s.RefreshTokenExpireDays)
	assert.Equal(t, 9000, s.Port)
	assert.False(t, s.LatexCLSIEnabled)
	assert.Equal(t, "http://localhost:9000", s.S3Endpoint)
	assert.Equal(t, "ci", s.S3Bucket)
	assert.Equal(t, []string{"a.com", "b.com"}, s.AllowedHosts)
	assert.Equal(t, "smtp", s.EmailProvider)
	assert.Equal(t, "localhost", s.SMTPHost)
	assert.False(t, s.SMTPTLS)
}

// SQLAlchemy 风格连接串 → Go 驱动风格（design.md D3/D10）
func TestNormalizeDatabaseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"asyncpg", "postgresql+asyncpg://u:p@localhost:5432/papersage", "postgres://u:p@localhost:5432/papersage"},
		{"psycopg", "postgresql+psycopg://u:p@h:5432/db", "postgres://u:p@h:5432/db"},
		{"plain postgres", "postgres://u:p@h:5432/db", "postgres://u:p@h:5432/db"},
		{"aiosqlite relative", "sqlite+aiosqlite:///./e2e.db", "./e2e.db"},
		{"sqlite relative", "sqlite:///e2e.db", "e2e.db"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeDatabaseURL(tc.in))
		})
	}
}

// Load 后应能拿到归一化的 DSN（store 层消费）
func TestSettingsDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql+asyncpg://u:p@h:5432/db")
	s, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://u:p@h:5432/db", s.DSN())
}

func TestMigrationDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql+asyncpg://u:p@pooler:5432/db")
	t.Setenv("DATABASE_MIGRATION_URL", "postgresql://u:p@direct:5432/db")
	s, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://u:p@direct:5432/db", s.MigrationDSN())
}

func TestAgentMaxIterationsMustBePositive(t *testing.T) {
	t.Setenv("AGENT_MAX_ITERATIONS", "0")
	_, err := Load()
	require.Error(t, err)
}

func TestAgentModelTimeoutMustBePositive(t *testing.T) {
	t.Setenv("AGENT_MODEL_TIMEOUT", "0s")
	_, err := Load()
	require.Error(t, err)
}
