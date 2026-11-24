package norms

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/email"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Info string          `json:"info"`
}

func setupRouter(t *testing.T) (*httptest.Server, *Service, store.UUID, store.UUID, string) {
	t.Helper()
	svc, db, uid, _, pid := setup(t)
	cfg := &config.Settings{SecretKey: "norms-test", AccessTokenExpireMinutes: 30, RefreshTokenExpireDays: 7}
	authSvc := auth.NewService(db, cfg, email.Noop{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Mount(router.Group("/api/v1"), authSvc.AuthRequired(), svc)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	return srv, svc, uid, pid, token
}

func doJSON(t *testing.T, srv *httptest.Server, method, path, token string, body any) (*http.Response, envelope) {
	t.Helper()
	var raw []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		raw = encoded
	}
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	var env envelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	return resp, env
}

// 路由树里 /norms/adopt（静态段）与 /norms/:entry_id（参数段）同级，
// gin 若不允许这种组合会在 Mount 时 panic——本测试即守这条线。
func TestRoutesMountAndServe(t *testing.T) {
	srv, _, _, pid, token := setupRouter(t)

	resp, _ := doJSON(t, srv, "GET", "/api/v1/norms/packs", "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp, env := doJSON(t, srv, "GET", "/api/v1/norms/packs", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, http.StatusOK, env.Code)
	var packs []packResponse
	require.NoError(t, json.Unmarshal(env.Data, &packs))
	require.Len(t, packs, 2)

	resp, env = doJSON(t, srv, "POST", "/api/v1/norms/packs", token, packRequest{
		Key: "route-pack", Name: "路由测试包", Authority: store.NormAuthorityUser,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var created packResponse
	require.NoError(t, json.Unmarshal(env.Data, &created))
	assert.False(t, created.IsBuiltin)

	resp, env = doJSON(t, srv, "POST", "/api/v1/projects/"+pid.String()+"/norms/adopt", token,
		adoptRequest{PackID: packs[0].ID})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var adoptions []adoptionResponse
	require.NoError(t, json.Unmarshal(env.Data, &adoptions))
	require.NotEmpty(t, adoptions)

	resp, _ = doJSON(t, srv, "PATCH", "/api/v1/projects/"+pid.String()+"/norms/"+adoptions[0].EntryID, token,
		map[string]any{"state": store.NormAdoptionDisabled})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, env = doJSON(t, srv, "GET", "/api/v1/norms/entries?pageSize=2", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page entryPage
	require.NoError(t, json.Unmarshal(env.Data, &page))
	assert.Len(t, page.Items, 2)
	assert.Equal(t, 2, page.PageSize)

	resp, env = doJSON(t, srv, "GET", "/api/v1/norms/entries/not-a-uuid", token, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, http.StatusNotFound, env.Code)
	assert.NotEmpty(t, env.Info)

	resp, _ = doJSON(t, srv, "DELETE", "/api/v1/projects/"+pid.String()+"/norms/"+adoptions[0].EntryID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, env = doJSON(t, srv, "GET", "/api/v1/projects/"+pid.String()+"/norms", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var remaining []adoptionResponse
	require.NoError(t, json.Unmarshal(env.Data, &remaining))
	assert.Len(t, remaining, len(adoptions)-1)
}

func TestEntryAndPackRoutes(t *testing.T) {
	srv, _, _, _, token := setupRouter(t)

	_, env := doJSON(t, srv, "POST", "/api/v1/norms/packs", token, packRequest{
		Key: "route-crud", Name: "增删改测试包",
	})
	var pack packResponse
	require.NoError(t, json.Unmarshal(env.Data, &pack))

	resp, env := doJSON(t, srv, "POST", "/api/v1/norms/entries", token, entryReq(pack.ID, "ROUTE-1"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var entry entryDetailResponse
	require.NoError(t, json.Unmarshal(env.Data, &entry))
	assert.Equal(t, 1, entry.Version)

	update := entryUpdateRequest{entryRequest: entryReq(pack.ID, "ROUTE-1"), ExpectedVersion: 1}
	update.Title = "改后的路由标题"
	resp, env = doJSON(t, srv, "PUT", "/api/v1/norms/entries/"+entry.ID, token, update)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated entryDetailResponse
	require.NoError(t, json.Unmarshal(env.Data, &updated))
	assert.Equal(t, 2, updated.Version)

	resp, _ = doJSON(t, srv, "GET", "/api/v1/norms/entries/"+entry.ID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = doJSON(t, srv, "DELETE", "/api/v1/norms/entries/"+entry.ID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	name := "改名后的包"
	resp, env = doJSON(t, srv, "PATCH", "/api/v1/norms/packs/"+pack.ID, token, packPatch{Name: &name})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var renamed packResponse
	require.NoError(t, json.Unmarshal(env.Data, &renamed))
	assert.Equal(t, name, renamed.Name)

	resp, _ = doJSON(t, srv, "DELETE", "/api/v1/norms/packs/"+pack.ID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
