package compilediag_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

const diagnosticPath = "/api/v1/compile-diagnostics"

type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Info string          `json:"info"`
}

func do(t *testing.T, ts *testutil.TestServer, method, path, body, token string) (*http.Response, envelope) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var env envelope
	require.NoError(t, json.Unmarshal(raw, &env), "响应不是信封 JSON: %s", raw)
	return resp, env
}

func sampleBody(extra string) string {
	return fmt.Sprintf(`{
		"compile_job_id": "",
		"project_id": "",
		"client_kind": "web",
		"engine": "xelatex",
		"engine_version": "papersage-swiftlatex-2026-10-2",
		"resource_profile": "texlive-2026-r80534",
		"failure_kind": "latex",
		"failure_stage": "compile",
		"duration_ms": 8421,
		"error_message": "编译失败",
		"log_excerpt": "! Undefined control sequence.\nl.42 \\secretSourceText\n(./chapters/intro.tex",
		"failed_resources": [],
		"client_env": {"user_agent":"UA","platform":"Linux","language":"zh-CN","hardware_concurrency":8,"device_memory_gb":8,"online":true,"unknown_key":"dropped"}
	}%s`, extra)
}

func TestIngestGuestStoresSanitizedReport(t *testing.T) {
	ts := testutil.New(t)

	resp, env := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 200, env.Code)

	var data struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &data))
	require.NotEmpty(t, data.ID)

	var report store.CompileErrorReport
	require.NoError(t, ts.DB.First(&report, "id = ?", store.UUID{UUID: uuid.MustParse(data.ID)}).Error)
	assert.True(t, report.IsGuest)
	assert.Nil(t, report.UserID)
	assert.Nil(t, report.ProjectID)
	assert.Equal(t, "xelatex", report.Engine)
	assert.Equal(t, "latex", report.FailureKind)
	// 白名单脱敏：保留诊断行与行号，源码与文件路径不得落库。
	require.NotNil(t, report.LogExcerpt)
	assert.Contains(t, *report.LogExcerpt, "! Undefined control sequence.")
	assert.Contains(t, *report.LogExcerpt, "l.42")
	assert.NotContains(t, *report.LogExcerpt, "secretSourceText")
	assert.NotContains(t, *report.LogExcerpt, "chapters/intro.tex")
	// client_env 只保留固定键。
	assert.NotContains(t, string(report.ClientEnv), "unknown_key")
	assert.Contains(t, string(report.ClientEnv), "user_agent")
}

func TestIngestAuthenticatedBindsUserAndOwnedProject(t *testing.T) {
	ts := testutil.New(t)
	project := &store.Project{Name: "论文", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(project).Error)

	body := strings.Replace(sampleBody(""), `"project_id": ""`, fmt.Sprintf(`"project_id": "%s"`, project.ID.String()), 1)
	resp, env := do(t, ts, http.MethodPost, diagnosticPath, body, ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 200, env.Code)

	var report store.CompileErrorReport
	require.NoError(t, ts.DB.Order("created_at DESC").First(&report).Error)
	assert.False(t, report.IsGuest)
	require.NotNil(t, report.UserID)
	assert.Equal(t, ts.UID, report.UserID.UUID)
	require.NotNil(t, report.ProjectID)
	assert.Equal(t, project.ID.UUID, report.ProjectID.UUID)
}

func TestIngestDropsForeignProjectAndJobLinks(t *testing.T) {
	ts := testutil.New(t)
	other := &store.User{Username: "other", PasswordHash: "x"}
	require.NoError(t, ts.DB.Create(other).Error)
	foreign := &store.Project{Name: "别人的论文", OwnerID: other.ID}
	require.NoError(t, ts.DB.Create(foreign).Error)

	body := strings.Replace(sampleBody(""), `"project_id": ""`, fmt.Sprintf(`"project_id": "%s"`, foreign.ID.String()), 1)
	resp, _ := do(t, ts, http.MethodPost, diagnosticPath, body, ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var report store.CompileErrorReport
	require.NoError(t, ts.DB.Order("created_at DESC").First(&report).Error)
	assert.Nil(t, report.ProjectID, "不属于本人的项目不得挂链")
}

func TestIngestRejectsInvalidTokenInsteadOfDowngrading(t *testing.T) {
	ts := testutil.New(t)
	resp, env := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), "not-a-token")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, 401, env.Code)
}

func TestIngestRejectsOversizedPayload(t *testing.T) {
	ts := testutil.New(t)
	huge := sampleBody("")
	huge = strings.Replace(huge, `"error_message": "编译失败"`,
		fmt.Sprintf(`"error_message": "%s"`, strings.Repeat("a", 200<<10)), 1)
	resp, env := do(t, ts, http.MethodPost, diagnosticPath, huge, "")
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	assert.Equal(t, 413, env.Code)
}

func TestIngestRateLimitsGuests(t *testing.T) {
	ts := testutil.New(t)
	for i := 0; i < 3; i++ {
		resp, _ := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), "")
		require.Equal(t, http.StatusOK, resp.StatusCode, "第 %d 次应放行", i+1)
	}
	resp, env := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), "")
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, 429, env.Code)
}

func TestLogExcerptOnlyStoredForLatexFailures(t *testing.T) {
	ts := testutil.New(t)
	body := strings.Replace(sampleBody(""), `"failure_kind": "latex"`, `"failure_kind": "resource"`, 1)
	body = strings.Replace(body, `"error_message": "编译失败"`, `"error_message": "资源加载失败：ctex.sty（HTTP 404）"`, 1)
	resp, _ := do(t, ts, http.MethodPost, diagnosticPath, body, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var report store.CompileErrorReport
	require.NoError(t, ts.DB.Order("created_at DESC").First(&report).Error)
	assert.Nil(t, report.LogExcerpt)
	assert.Equal(t, "resource", report.FailureKind)
	assert.Contains(t, report.ErrorMessage, "ctex.sty")
}

func TestAdminListRequiresSuperuser(t *testing.T) {
	ts := testutil.New(t)
	resp, env := do(t, ts, http.MethodGet, "/api/v1/admin/compile-diagnostics", "", ts.Token)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, 403, env.Code)
}

func TestAdminListFiltersAndPaginates(t *testing.T) {
	ts := testutil.New(t)
	ts.MakeSuperuser(t)

	// 3 条 latex + 1 条 timeout
	for i := 0; i < 3; i++ {
		resp, _ := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), ts.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	timeoutBody := strings.Replace(sampleBody(""), `"failure_kind": "latex"`, `"failure_kind": "timeout"`, 1)
	resp, _ := do(t, ts, http.MethodPost, diagnosticPath, timeoutBody, ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, env := do(t, ts, http.MethodGet, "/api/v1/admin/compile-diagnostics?failure_kind=latex&page=1&page_size=2", "", ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var page struct {
		Items []struct {
			ID          string `json:"id"`
			Username    string `json:"username"`
			FailureKind string `json:"failure_kind"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &page))
	assert.Equal(t, int64(3), page.Total)
	assert.Len(t, page.Items, 2)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 2, page.PageSize)
	for _, item := range page.Items {
		assert.Equal(t, "latex", item.FailureKind)
		assert.Equal(t, "tester", item.Username)
	}

	resp, env = do(t, ts, http.MethodGet, "/api/v1/admin/compile-diagnostics?engine=lualatex", "", ts.Token)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Equal(t, 422, env.Code)
}

func TestAdminDetailAndNotFound(t *testing.T) {
	ts := testutil.New(t)
	ts.MakeSuperuser(t)

	_, env := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), ts.Token)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &created))

	resp, detailEnv := do(t, ts, http.MethodGet, "/api/v1/admin/compile-diagnostics/"+created.ID, "", ts.Token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail struct {
		ID         string  `json:"id"`
		LogExcerpt *string `json:"log_excerpt"`
		ClientEnv  any     `json:"client_env"`
	}
	require.NoError(t, json.Unmarshal(detailEnv.Data, &detail))
	assert.Equal(t, created.ID, detail.ID)
	require.NotNil(t, detail.LogExcerpt)
	assert.NotContains(t, *detail.LogExcerpt, "secretSourceText")

	resp, env = do(t, ts, http.MethodGet,
		"/api/v1/admin/compile-diagnostics/01999999-9999-7999-8999-999999999999", "", ts.Token)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, 404, env.Code)
}

func TestRetentionCleanupRemovesExpiredRows(t *testing.T) {
	ts := testutil.New(t)
	old := &store.CompileErrorReport{
		Engine: "xelatex", ClientKind: "web", FailureKind: "latex",
		FailureStage: "compile", ErrorMessage: "旧记录",
		CreatedAt: timeAgo(31 * 24 * 60 * 60),
	}
	require.NoError(t, ts.DB.Create(old).Error)

	resp, _ := do(t, ts, http.MethodPost, diagnosticPath, sampleBody(""), "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var count int64
	require.NoError(t, ts.DB.Model(&store.CompileErrorReport{}).Where("id = ?", old.ID).Count(&count).Error)
	assert.Zero(t, count, "超过保留期的诊断应被清理")
}

func timeAgo(seconds int) time.Time { return time.Now().Add(-time.Duration(seconds) * time.Second) }
