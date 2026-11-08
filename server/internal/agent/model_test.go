package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/agent/opencode"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/settings"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestResolveModelDoesNotFallbackOnSystemSettingsReadError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/missing-system.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("CREATE TABLE user_settings (user_id TEXT, category TEXT, settings TEXT)").Error)
	cfg := &config.Settings{AgentAPIKey: "fallback", AgentBaseURL: "https://example.invalid", AgentModel: "fallback"}
	resolved, err := ResolveModelWithEnv(context.Background(), db, uuid.New(), cfg)
	require.Error(t, err)
	require.Nil(t, resolved)
}

func TestResolveModelDoesNotFallbackOnDatabaseError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/missing.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := &config.Settings{AgentAPIKey: "fallback", AgentBaseURL: "https://example.invalid", AgentModel: "fallback"}
	resolved, err := ResolveModelWithEnv(context.Background(), db, uuid.New(), cfg)
	require.Error(t, err)
	require.Nil(t, resolved)
}

func TestModelSelectionThroughSettingsService(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/persist.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&store.UserSettings{}))
	uid := uuid.New()
	service := settings.NewService(db, nil)
	for _, selected := range []string{"A", "B"} {
		_, validationErr := service.UpdateCategory(context.Background(), uid, "agent", map[string]any{
			"active_provider_id": "p",
			"providers":          []any{map[string]any{"id": "p", "api_key": "fake-key", "base_url": "https://example.invalid", "models": []any{"A", "B"}, "active_model": selected}},
		})
		require.Nil(t, validationErr)
		resolved, err := ResolveModel(context.Background(), db, uid)
		require.NoError(t, err)
		require.Equal(t, selected, resolved.Model)
	}
}

func TestResolveSelectedUserModel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/models.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("CREATE TABLE user_settings (user_id TEXT, category TEXT, settings TEXT)").Error)
	uid := uuid.New()
	set := func(raw string) {
		t.Helper()
		require.NoError(t, db.Exec("DELETE FROM user_settings").Error)
		require.NoError(t, db.Exec("INSERT INTO user_settings VALUES (?, 'agent', ?)", store.UUID{UUID: uid}, raw).Error)
	}
	ctx := context.Background()
	cfg := &config.Settings{AgentAPIKey: "env-key", AgentBaseURL: "https://example.invalid", AgentModel: "env-model"}
	for _, tc := range []struct {
		name, raw, want string
		invalid         bool
	}{
		{"selected A", `{"active_provider_id":"p","providers":[{"id":"p","name":"provider","api_key":"secret","base_url":"https://example.invalid","models":["A","B"],"active_model":"A"}]}`, "A", false},
		{"selected B", `{"active_provider_id":"p","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A","B"],"active_model":"B"}]}`, "B", false},
		{"legacy missing active model", `{"active_provider_id":"p","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A","B"]}]}`, "B", false},
		{"different provider", `{"active_provider_id":"q","providers":[{"id":"p","api_key":"other","base_url":"https://example.invalid","models":["A"],"active_model":"A"},{"id":"q","api_key":"secret","base_url":"https://example.invalid","models":["B","C"],"active_model":"B"}]}`, "B", false},
		{"invalid selection", `{"active_provider_id":"p","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A","B"],"active_model":"missing"}]}`, "", true},
		{"empty selection", `{"active_provider_id":"p","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A","B"],"active_model":""}]}`, "", true},
		{"null selection", `{"active_provider_id":"p","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A"],"active_model":null}]}`, "", true},
		{"non-string selection", `{"active_provider_id":"p","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A"],"active_model":42}]}`, "", true},
		{"missing provider", `{"active_provider_id":"missing","providers":[{"id":"p","api_key":"secret","base_url":"https://example.invalid","models":["A"],"active_model":"A"}]}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set(tc.raw)
			got, err := ResolveModelWithEnv(ctx, db, uid, cfg)
			if tc.invalid {
				require.Error(t, err)
				require.Nil(t, got)
				require.NotContains(t, err.Error(), "secret")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Model)
			require.Equal(t, "user-settings", got.Source)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCodeGatewayAddsSessionHeader(t *testing.T) {
	var got string
	transport := opencode.SessionTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Header.Get("x-opencode-session")
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	}), "conv-1")
	req, err := http.NewRequest(http.MethodPost, "https://opencode.ai/inference/go/openai/v1/chat/completions", nil)
	require.NoError(t, err)
	_, err = transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, "conv-1", got)
}

func TestOpenCodeGatewayDetection(t *testing.T) {
	require.True(t, opencode.IsGateway("https://opencode.ai/inference/go/openai/v1"))
	require.False(t, opencode.IsGateway("https://api.minimax.cn/v1"))
	require.False(t, opencode.IsGateway("https://notopencode.ai.example/v1"))
}
