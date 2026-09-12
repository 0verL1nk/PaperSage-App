package latex_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 恢复与清理队列语义（compile_queue.py recover_on_startup / cleanup_old_jobs 契约）
func TestQueueRecoverAndCleanup(t *testing.T) {
	ts := testutil.New(t)

	// 落库一个 pending 任务（模拟上个进程中断）
	p := store.Project{Name: "恢复项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	pending := store.CompileJob{
		ProjectID: p.ID, UserID: ts.UserID(),
		Status: string(store.JobPending), RootFile: "main.tex", Compiler: "pdflatex",
	}
	require.NoError(t, ts.DB.Create(&pending).Error)

	// 落库一个孤儿 running（进程重启期间）
	running := store.CompileJob{
		ProjectID: p.ID, UserID: ts.UserID(),
		Status: string(store.JobRunning), RootFile: "main.tex", Compiler: "pdflatex",
	}
	require.NoError(t, ts.DB.Create(&running).Error)

	q := ts.CompileQueue
	q.RecoverOnStartup(ts.DB.Statement.Context)

	// running → failed + "Process restarted during compilation"
	var job store.CompileJob
	require.NoError(t, ts.DB.Where("id = ?", running.ID).First(&job).Error)
	assert.Equal(t, string(store.JobFailed), job.Status)
	require.NotNil(t, job.Error)
	assert.Equal(t, "Process restarted during compilation", *job.Error)

	// pending → 被恢复调度（CLSI 不可达 → 最终 failed，而非永远 pending）
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_ = ts.DB.Where("id = ?", pending.ID).First(&job).Error
		if job.Status == string(store.JobFailed) || job.Status == string(store.JobSuccess) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.Equal(t, string(store.JobFailed), job.Status, "恢复的 pending 任务应被重新执行（无 CLSI → failed）")

	// cleanup：把一个终态任务改旧 → CleanupOldJobs 标记 cancelled
	old := time.Now().UTC().Add(-48 * time.Hour)
	require.NoError(t, ts.DB.Exec(
		"UPDATE compile_jobs SET updated_at = ? WHERE id = ?", old, running.ID).Error)
	n := q.CleanupOldJobs(ts.DB.Statement.Context)
	assert.GreaterOrEqual(t, n, int64(1))
	_ = ts.DB.Where("id = ?", running.ID).First(&job).Error
	assert.Equal(t, string(store.JobCancelled), job.Status)
}

// multipart 上传二进制 latex 文件（createHandler 的 multipart 分支）
func TestUploadBinaryLatexFile(t *testing.T) {
	ts := testutil.New(t)
	p := store.Project{Name: "二进制项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)

	// multipart：file 字段 + path 字段
	body, contentType := multipartBody(t, map[string]string{"path": "logo.png"}, "logo.png", "image/png", []byte("PNGDATA"))
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects/"+p.ID.String()+"/latex-files/", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, 200, resp.StatusCode)

	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "logo.png", m["data"].(map[string]any)["file_path"])

	// multipart 缺 file 字段 → 400 "No file provided in multipart"
	body2, ct2 := multipartBody(t, map[string]string{"path": "x.png"}, "", "", nil)
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects/"+p.ID.String()+"/latex-files/", body2)
	req2.Header.Set("Authorization", "Bearer "+ts.Token)
	req2.Header.Set("Content-Type", ct2)
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })
	var m2 map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&m2))
	assert.Equal(t, float64(400), m2["code"])
	assert.Equal(t, "No file provided in multipart", m2["info"])
}

func multipartBody(t *testing.T, fields map[string]string, filename, ctype string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if filename != "" {
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		if ctype != "" {
			hdr.Set("Content-Type", ctype)
		}
		fw, _ := mw.CreatePart(hdr)
		_, _ = fw.Write(content)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}
