package latex_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

func compileBase(projectID string) string {
	return "/api/v1/projects/" + projectID + "/latex/compile"
}

func uploadBrowserResult(t *testing.T, ts *testutil.TestServer, base, jobID string, pdf []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("pdf", "output.pdf")
	require.NoError(t, err)
	_, err = part.Write(pdf)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req, err := http.NewRequest(http.MethodPost, ts.URL+base+"/"+jobID+"/browser-result", &body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestBrowserCompileHeartbeatRefreshesRunningJob(t *testing.T) {
	ts := testutil.New(t)
	project := store.Project{Name: "心跳项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&project).Error)
	job, err := ts.CompileQueue.CreateBrowserJob(t.Context(), project.ID.UUID, ts.UID, "main.tex", "pdflatex")
	require.NoError(t, err)
	stale := time.Now().UTC().Add(-10 * time.Minute)
	require.NoError(t, ts.DB.Exec("UPDATE compile_jobs SET updated_at = ? WHERE id = ?", stale, job.ID).Error)

	base := compileBase(project.ID.UUID.String())
	heartbeat := func() *http.Response {
		return ts.Do(t, http.MethodPost, base+"/"+job.ID.UUID.String()+"/browser-heartbeat", nil)
	}

	resp := heartbeat()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var refreshed store.CompileJob
	require.NoError(t, ts.DB.Where("id = ?", job.ID).First(&refreshed).Error)
	assert.Equal(t, string(store.JobRunning), refreshed.Status)
	assert.True(t, refreshed.UpdatedAt.After(stale.Add(time.Minute)), "heartbeat must refresh updated_at")
	assert.Zero(t, ts.CompileQueue.ExpireStaleRunningJobs(t.Context()), "a heartbeating job must not expire")

	// A finished job can no longer be kept alive.
	require.NoError(t, ts.CompileQueue.CompleteBrowserJob(
		t.Context(), job.ID.UUID, project.ID.UUID, ts.UID, "browser-heartbeat-build", ""))
	resp = heartbeat()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestStaleBrowserCompileJobConvergesToTimeout(t *testing.T) {
	ts := testutil.New(t)
	project := store.Project{Name: "超时收敛项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&project).Error)
	stale, err := ts.CompileQueue.CreateBrowserJob(t.Context(), project.ID.UUID, ts.UID, "main.tex", "xelatex")
	require.NoError(t, err)
	fresh, err := ts.CompileQueue.CreateBrowserJob(t.Context(), project.ID.UUID, ts.UID, "main.tex", "xelatex")
	require.NoError(t, err)
	require.NoError(t, ts.DB.Exec("UPDATE compile_jobs SET updated_at = ? WHERE id = ?",
		time.Now().UTC().Add(-10*time.Minute), stale.ID).Error)

	base := compileBase(project.ID.UUID.String())
	statusOf := func(jobID string) map[string]any {
		resp := ts.Do(t, http.MethodGet, base+"/"+jobID+"/status/", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var envelope map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
		return envelope["data"].(map[string]any)
	}

	data := statusOf(stale.ID.UUID.String())
	assert.Equal(t, string(store.JobTimeout), data["status"], "查询不能永远返回 running")
	require.NotNil(t, data["error"])
	assert.Contains(t, data["error"], "没有回传结果")

	assert.Equal(t, string(store.JobRunning), statusOf(fresh.ID.UUID.String())["status"])

	var expired store.CompileJob
	require.NoError(t, ts.DB.Where("id = ?", stale.ID).First(&expired).Error)
	assert.Equal(t, string(store.JobTimeout), expired.Status)
}

func TestBrowserCompileResultAfterTimeoutStillCompletes(t *testing.T) {
	ts := testutil.New(t)
	project := store.Project{Name: "迟到结果项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&project).Error)
	job, err := ts.CompileQueue.CreateBrowserJob(t.Context(), project.ID.UUID, ts.UID, "main.tex", "xelatex")
	require.NoError(t, err)
	require.NoError(t, ts.DB.Exec("UPDATE compile_jobs SET updated_at = ? WHERE id = ?",
		time.Now().UTC().Add(-10*time.Minute), job.ID).Error)

	base := compileBase(project.ID.UUID.String())
	resp := ts.Do(t, http.MethodGet, base+"/"+job.ID.UUID.String()+"/status/", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The tab was merely suspended: a late but valid artifact still wins over
	// losing the PDF the browser produced.
	resp = uploadBrowserResult(t, ts, base, job.ID.UUID.String(), minimalValidPDF())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var envelope map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	assert.Equal(t, string(store.JobSuccess), envelope["data"].(map[string]any)["status"])

	var stored store.CompileJob
	require.NoError(t, ts.DB.Where("id = ?", job.ID).First(&stored).Error)
	assert.Equal(t, string(store.JobSuccess), stored.Status)
	require.NotNil(t, stored.BuildID)
}
