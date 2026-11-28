package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
)

func TestDegradedMailerMatchesRuntimeResolution(t *testing.T) {
	cases := []struct {
		name     string
		admin    syssettings.EmailSettings
		cfg      *config.Settings
		degraded bool
	}{
		{"env resend configured", syssettings.EmailSettings{}, &config.Settings{EmailProvider: "resend", ResendAPIKey: "key", ResendDomain: "example.com"}, false},
		{"env resend missing domain", syssettings.EmailSettings{}, &config.Settings{EmailProvider: "resend", ResendAPIKey: "key"}, true},
		{"env smtp configured", syssettings.EmailSettings{}, &config.Settings{EmailProvider: "smtp", SMTPHost: "smtp.example.com:587", SMTPUser: "bot@example.com"}, false},
		{"env smtp missing user", syssettings.EmailSettings{}, &config.Settings{EmailProvider: "smtp", SMTPHost: "smtp.example.com:587"}, true},
		{"admin resend configured", syssettings.EmailSettings{Provider: "resend", ResendAPIKey: "key", ResendDomain: "example.com"}, &config.Settings{}, false},
		{"admin resend incomplete falls back to env", syssettings.EmailSettings{Provider: "resend", ResendAPIKey: "key"}, &config.Settings{EmailProvider: "smtp", SMTPHost: "smtp.example.com:587", SMTPUser: "bot@example.com"}, false},
		{"admin noop disables env", syssettings.EmailSettings{Provider: "noop"}, &config.Settings{EmailProvider: "smtp", SMTPHost: "smtp.example.com:587", SMTPUser: "bot@example.com"}, true},
		{"nothing configured", syssettings.EmailSettings{}, &config.Settings{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.degraded, degradedMailer(tc.admin, tc.cfg))
		})
	}
}

func TestDefaultEmbedderDegradation(t *testing.T) {
	cases := []struct {
		name     string
		admin    syssettings.EmbedSettings
		cfg      *config.Settings
		degraded bool
		mode     string
	}{
		{"admin config", syssettings.EmbedSettings{BaseURL: "https://embed.example.com", APIKey: "key"}, &config.Settings{}, false, "semantic"},
		{"env config", syssettings.EmbedSettings{}, &config.Settings{EmbedBaseURL: "https://embed.example.com", EmbedAPIKey: "key"}, false, "semantic"},
		{"nothing configured", syssettings.EmbedSettings{}, &config.Settings{}, true, "keyword"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			embedder := defaultEmbedder(tc.admin, tc.cfg)
			assert.Equal(t, tc.degraded, embedderDegraded(embedder))
			assert.Equal(t, tc.mode, retrievalModeFor(embedder))
		})
	}
}

func TestEmbedderDegradedDetectsHashFallback(t *testing.T) {
	assert.True(t, embedderDegraded(rag.HashEmbedder{}))
	assert.False(t, embedderDegraded(&rag.HTTPEmbedder{BaseURL: "https://embed.example.com", APIKey: "key"}))
}
