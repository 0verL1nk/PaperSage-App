package settings

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/netguard"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/syssettings"
)

func init() {
	// httptest servers listen on loopback; production uses the public-only dialer.
	testClientFactory = func(string, bool) *http.Client { return &http.Client{Timeout: agentTestTimeout} }
}

// probeTestService 打开本地模型开关：httptest 服务都在回环地址上。
func probeTestService() *Service {
	return NewService(nil, &config.Settings{AgentAllowPrivateProviderURLs: true})
}

// 生产默认不放开本地模型：用户填的回环/私网地址在校验阶段就被拒绝。
func TestAgentConnectionRejectsPrivateBaseURLByDefault(t *testing.T) {
	for _, base := range []string{
		"http://127.0.0.1:11434/v1", "http://10.0.0.5/v1",
		"http://169.254.169.254/latest/meta-data", "http://[::1]:11434/v1",
	} {
		_, err := NewService(nil, &config.Settings{}).TestAgentConnection(context.Background(), AgentTestRequest{
			BaseURL: base, APIKey: "synthetic-key", Model: "m1",
		})
		require.ErrorIs(t, err, netguard.ErrInvalidBaseURL, base)
	}
}

func TestAgentConnectionModelsSuccess(t *testing.T) {
	var gotAuth, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "m1"}, {"id": "m2"}},
		})
	}))
	t.Cleanup(upstream.Close)

	res, err := probeTestService().TestAgentConnection(context.Background(), AgentTestRequest{
		BaseURL: upstream.URL + "/v1/", APIKey: "synthetic-key", Model: "m1",
	})
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, []string{"m1", "m2"}, res.Models)
	assert.Equal(t, "Bearer synthetic-key", gotAuth)
	assert.Equal(t, "/v1/models", gotPath)
}

func TestAgentConnectionFallsBackToChat(t *testing.T) {
	var chatBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			http.NotFound(w, r)
			return
		}
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&chatBody))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	res, err := probeTestService().TestAgentConnection(context.Background(), AgentTestRequest{
		BaseURL: upstream.URL + "/v1", APIKey: "synthetic-key", Model: "m1",
	})
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Empty(t, res.Models)
	assert.Equal(t, "m1", chatBody["model"])
	assert.Equal(t, float64(1), chatBody["max_tokens"])
}

func TestAgentConnectionTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(upstream.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := probeTestService().TestAgentConnection(ctx, AgentTestRequest{
		BaseURL: upstream.URL + "/v1", APIKey: "synthetic-key", Model: "m1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection timed out")
}

func TestAgentConnectionNetworkError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := upstream.URL
	upstream.Close()

	_, err := probeTestService().TestAgentConnection(context.Background(), AgentTestRequest{
		BaseURL: closedURL + "/v1", APIKey: "synthetic-key", Model: "m1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not connect to provider")
}

func TestAgentTestEndpointDoesNotLeakKey(t *testing.T) {
	const key = "synthetic-secret-key"
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)

	srv, _ := newSettingsTestServer(t)
	resp := authedJSON(t, http.MethodPost, srv.URL+"/api/v1/settings/agent/test", mintToken(t, seededUID), map[string]any{
		"base_url": upstream.URL + "/v1", "api_key": key, "model": "m1",
	})
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := string(raw)
	assert.Equal(t, "Bearer "+key, gotAuth)
	assert.Contains(t, body, "401")
	assert.NotContains(t, body, key)
}

func TestAgentTestEndpointRejectsMissingFields(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	for _, payload := range []map[string]any{
		{"api_key": "k", "model": "m1"},
		{"base_url": "https://example.invalid/v1", "model": "m1"},
	} {
		resp := authedJSON(t, http.MethodPost, srv.URL+"/api/v1/settings/agent/test", mintToken(t, seededUID), payload)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%v", payload)
	}
}

// 内网/回环地址在接口层就是 4xx（用户填错地址），而不是 502。
func TestAgentTestEndpointRejectsPrivateBaseURL(t *testing.T) {
	srv, _ := newSettingsTestServer(t, &config.Settings{SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7})
	resp := authedJSON(t, http.MethodPost, srv.URL+"/api/v1/settings/agent/test", mintToken(t, seededUID), map[string]any{
		"base_url": "http://169.254.169.254/latest/meta-data", "api_key": "k", "model": "m1",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "private or loopback")
}

func TestAgentTestEndpointRequiresAuth(t *testing.T) {
	srv, _ := newSettingsTestServer(t)
	resp, err := http.Post(srv.URL+"/api/v1/settings/agent/test", "application/json", strings.NewReader(`{}`)) //nolint:gosec,noctx // test
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSettingsExposeServerFallback(t *testing.T) {
	fallback := func(t *testing.T, srvURL, token string) map[string]any {
		t.Helper()
		resp := authedGet(t, srvURL+"/api/v1/settings", token)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		agent := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)["agent"].(map[string]any)
		return agent["server_fallback"].(map[string]any)
	}

	t.Run("not configured", func(t *testing.T) {
		srv, _ := newSettingsTestServer(t)
		got := fallback(t, srv.URL, mintToken(t, seededUID))
		assert.Equal(t, false, got["configured"])
		assert.Equal(t, "", got["model"])
	})

	t.Run("env fallback", func(t *testing.T) {
		srv, _ := newSettingsTestServer(t, &config.Settings{
			SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7,
			AgentAPIKey: "synthetic", AgentBaseURL: "https://example.invalid/v1", AgentModel: "env-model",
		})
		got := fallback(t, srv.URL, mintToken(t, seededUID))
		assert.Equal(t, true, got["configured"])
		assert.Equal(t, "env-model", got["model"])
	})

	t.Run("admin settings win and default the model from env", func(t *testing.T) {
		srv, db := newSettingsTestServer(t, &config.Settings{
			SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7,
			AgentModel: "env-default",
		})
		require.NoError(t, syssettings.Put(db, map[string]any{
			syssettings.KeyAgent: map[string]any{"api_key": "synthetic", "base_url": "https://example.invalid/v1"},
		}))
		got := fallback(t, srv.URL, mintToken(t, seededUID))
		assert.Equal(t, true, got["configured"])
		assert.Equal(t, "env-default", got["model"])
	})
}

func TestServerFallbackIsReadOnly(t *testing.T) {
	srv, db := newSettingsTestServer(t, &config.Settings{
		SecretKey: "t", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7,
		AgentAPIKey: "synthetic", AgentBaseURL: "https://example.invalid/v1", AgentModel: "env-model",
	})
	token := mintToken(t, seededUID)
	resp := authedJSON(t, http.MethodPatch, srv.URL+"/api/v1/settings/agent", token, map[string]any{
		"settings": map[string]any{"providers": []any{}},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	settings := decode(t, resp)["data"].(map[string]any)["settings"].(map[string]any)
	got := settings["server_fallback"].(map[string]any)
	assert.Equal(t, true, got["configured"])
	assert.Equal(t, "env-model", got["model"])

	var row store.UserSettings
	require.NoError(t, db.Where("user_id = ? AND category = ?", store.UUID{UUID: seededUID}, "agent").First(&row).Error)
	assert.NotContains(t, string(row.Settings), "server_fallback")
}
