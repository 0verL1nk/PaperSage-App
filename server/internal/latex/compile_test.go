package latex_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// fakeCLSI 脚本化 CLSI：按序返回 compile 结果并记录请求。
type fakeCLSI struct {
	mu          sync.Mutex
	compiles    []map[string]any // 收到的 compile 请求体
	script      []string         // 每次 compile 的 status（"success"/"failure"/"conflict"/"hang"）
	buildN      int
	logBody     string
	pdf         []byte
	calls       map[string]int
	hangStarted chan struct{}
	hangRelease chan struct{}
}

func TestBrowserCompileResultIsPersistedAndIdempotent(t *testing.T) {
	ts := testutil.New(t)
	body, err := json.Marshal(map[string]any{"name": "浏览器编译项目"})
	require.NoError(t, err)
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var envelope map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	projectID := envelope["data"].(map[string]any)["id"].(string)
	base := "/api/v1/projects/" + projectID + "/latex/compile"

	requestBody, err := json.Marshal(map[string]string{"root_file": "main.tex", "compiler": "xelatex"})
	require.NoError(t, err)
	resp = ts.Do(t, http.MethodPost, base+"/browser", requestBody)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	job := envelope["data"].(map[string]any)
	jobID := job["job_id"].(string)
	assert.Equal(t, string(store.JobRunning), job["status"])

	pdf := minimalValidPDF()
	postResult := func() *http.Response {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("logs", "XeTeX browser compile log"))
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="pdf"; filename="output.pdf"`)
		header.Set("Content-Type", "application/pdf")
		file, createErr := writer.CreatePart(header)
		require.NoError(t, createErr)
		_, writeErr := file.Write(pdf)
		require.NoError(t, writeErr)
		require.NoError(t, writer.Close())
		req, requestErr := http.NewRequest(http.MethodPost, ts.URL+base+"/"+jobID+"/browser-result", &body)
		require.NoError(t, requestErr)
		req.Header.Set("Authorization", "Bearer "+ts.Token)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	resp = postResult()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	firstBuildID := envelope["data"].(map[string]any)["build_id"].(string)
	assert.Equal(t, string(store.JobSuccess), envelope["data"].(map[string]any)["status"])

	// A retried upload with identical bytes must keep the same published PDF.
	resp = postResult()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	assert.Equal(t, firstBuildID, envelope["data"].(map[string]any)["build_id"])
	stored, err := ts.CompileQueue.Files.Storage.Get(t.Context(), "projects/"+strings.ReplaceAll(projectID, "-", "")+"/latex/builds/"+firstBuildID+"/output.pdf")
	require.NoError(t, err)
	assert.Equal(t, pdf, stored)

	status := ts.CompileQueue.GetStatus(t.Context(), mustUUID(t, jobID), ts.UID)
	require.NotNil(t, status)
	assert.Equal(t, string(store.JobSuccess), status.Status)
	assert.Equal(t, firstBuildID, *status.BuildID)
}

func TestBrowserCompileResultRejectsMalformedPDF(t *testing.T) {
	ts := testutil.New(t)
	body, err := json.Marshal(map[string]any{"name": "坏 PDF 上传项目"})
	require.NoError(t, err)
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var envelope map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	projectID := envelope["data"].(map[string]any)["id"].(string)
	base := "/api/v1/projects/" + projectID + "/latex/compile"

	resp = ts.Do(t, http.MethodPost, base+"/browser", []byte(`{"root_file":"main.tex","compiler":"xelatex"}`))
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	jobID := envelope["data"].(map[string]any)["job_id"].(string)

	// A header and EOF marker alone do not make a structurally valid PDF.
	fakePDF := []byte("%PDF-1.7\nthis is not a PDF object table\n%%EOF\n")
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	part, err := writer.CreateFormFile("pdf", "output.pdf")
	require.NoError(t, err)
	_, err = part.Write(fakePDF)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req, err := http.NewRequest(http.MethodPost, ts.URL+base+"/"+jobID+"/browser-result", &multipartBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	status := ts.CompileQueue.GetStatus(t.Context(), mustUUID(t, jobID), ts.UID)
	require.NotNil(t, status)
	assert.Equal(t, string(store.JobRunning), status.Status)
	assert.Nil(t, status.BuildID)
}

func TestBrowserCompileCancelAndCompletionRacePreservesWinningTerminalState(t *testing.T) {
	ts := testutil.New(t)
	project := store.Project{Name: "编译取消竞态", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&project).Error)

	for index := 0; index < 32; index++ {
		job, err := ts.CompileQueue.CreateBrowserJob(t.Context(), project.ID.UUID, ts.UID, "main.tex", "xelatex")
		require.NoError(t, err)
		buildID := fmt.Sprintf("browser-race-build-%d", index)
		start := make(chan struct{})
		cancelDone := make(chan *store.CompileJob, 1)
		cancelErr := make(chan error, 1)
		completeErr := make(chan error, 1)
		go func() {
			<-start
			cancelled, err := ts.CompileQueue.Cancel(t.Context(), job.ID.UUID, ts.UID)
			cancelDone <- cancelled
			cancelErr <- err
		}()
		go func() {
			<-start
			completeErr <- ts.CompileQueue.CompleteBrowserJob(t.Context(), job.ID.UUID, project.ID.UUID, ts.UID, buildID, "")
		}()
		close(start)

		cancelled := <-cancelDone
		require.NoError(t, <-cancelErr)
		completionError := <-completeErr
		status := ts.CompileQueue.GetStatus(t.Context(), job.ID.UUID, ts.UID)
		require.NotNil(t, status)
		switch status.Status {
		case string(store.JobSuccess):
			require.NoError(t, completionError)
			require.NotNil(t, status.BuildID)
			require.Equal(t, buildID, *status.BuildID)
			require.NotNil(t, cancelled)
			require.Equal(t, string(store.JobSuccess), cancelled.Status)
		case string(store.JobCancelled):
			require.Error(t, completionError)
			require.Nil(t, status.BuildID)
			require.NotNil(t, cancelled)
			require.Equal(t, string(store.JobCancelled), cancelled.Status)
		default:
			t.Fatalf("race ended in nonterminal status %q", status.Status)
		}
	}
}

func minimalValidPDF() []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 5)
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] /Resources << >> /Contents 4 0 R >>\nendobj\n",
		"4 0 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n",
	}
	for i, object := range objects {
		offsets[i+1] = pdf.Len()
		pdf.WriteString(object)
	}
	xrefOffset := pdf.Len()
	pdf.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		_, _ = fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	_, _ = fmt.Fprintf(&pdf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)
	return pdf.Bytes()
}

func mustUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(value)
	require.NoError(t, err)
	return id
}

func newFake(script ...string) *fakeCLSI {
	return &fakeCLSI{script: script, pdf: []byte("PDFBYTES-0123456789AB"), calls: map[string]int{}}
}

func (f *fakeCLSI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls[r.Method+" "+r.URL.Path]++
		switch {
		case strings.HasSuffix(r.URL.Path, "/compile") && strings.Contains(r.URL.Path, "/project/") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.compiles = append(f.compiles, body)
			idx := len(f.compiles) - 1
			status := "success"
			if idx < len(f.script) {
				status = f.script[idx]
			}
			if status == "hang" {
				if f.hangStarted != nil && f.hangRelease != nil {
					close(f.hangStarted)
					<-f.hangRelease
				} else {
					time.Sleep(3 * time.Second)
				}
			}
			f.buildN++
			buildID := fmt.Sprintf("b%d", f.buildN)
			if status == "conflict" {
				_, _ = w.Write([]byte(`{"compile":{"status":"conflict"}}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"compile":{"status":%q,"buildId":%q}}`, status, buildID)
		case strings.HasSuffix(r.URL.Path, "/output/output.log"):
			_, _ = w.Write([]byte(f.logBody))
		case strings.HasSuffix(r.URL.Path, "/output/output.pdf"):
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(f.pdf)
		case strings.HasSuffix(r.URL.Path, "/output/output.zip"):
			_, _ = w.Write([]byte("ZIPDATA"))
		case strings.HasSuffix(r.URL.Path, "/wordcount"):
			_, _ = w.Write([]byte(`{"texcount":{"words":42}}`))
		case strings.HasSuffix(r.URL.Path, "/sync/code"):
			_, _ = w.Write([]byte(`{"pdf":[{"file":"main.tex","page":1,"h":100.5}]}`))
		case strings.HasSuffix(r.URL.Path, "/sync/pdf"):
			_, _ = w.Write([]byte(`{"code":[{"file":"main.tex","line":3}]}`))
		default:
			w.WriteHeader(http.StatusNoContent) // stop/clear cache → 204
		}
	})
}

func (f *fakeCLSI) compileCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.compiles)
}

func compileBody(t *testing.T, i int, f *fakeCLSI) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Less(t, i, len(f.compiles))
	return f.compiles[i]
}

// Queue's retained CLSI worker can still be exercised directly for regression coverage;
// the public POST /compile route is separately verified as disabled.
func TestCompileSuccessFlow(t *testing.T) {
	f := newFake("success")
	clsi := httptest.NewServer(f.handler())
	defer clsi.Close()
	ts := testutil.NewWithClsi(t, clsi.URL)

	// 经 projects 域创建项目（带 main.tex 模板）
	body, _ := json.Marshal(map[string]any{"name": "编译项目"})
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	pid := m["data"].(map[string]any)["id"].(string)
	base := "/api/v1/projects/" + pid + "/latex"

	job, err := ts.CompileQueue.Submit(context.Background(), uuid.MustParse(pid), ts.UserID().UUID, "main.tex", "pdflatex")
	require.NoError(t, err)
	jobID := job.ID.String()

	// 轮询至 success
	var status string
	for i := 0; i < 50; i++ {
		resp = ts.Do(t, http.MethodGet, base+"/compile/"+jobID+"/status", nil)
		require.Equal(t, 200, resp.StatusCode)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
		d := m["data"].(map[string]any)
		status = d["status"].(string)
		if status == "success" || status == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Equal(t, "success", status)
	require.Equal(t, "b1", m["data"].(map[string]any)["build_id"])

	// CLSI 收到的请求（project id 无横线）
	c := compileBody(t, 0, f)
	comp := c["compile"].(map[string]any)
	assert.Equal(t, "main.tex", comp["rootResourcePath"])
	opts := comp["options"].(map[string]any)
	assert.Equal(t, "pdflatex", opts["compiler"])
	assert.Equal(t, "full", opts["syncType"])
	res := comp["resources"].([]any)
	require.Len(t, res, 1)
	r0 := res[0].(map[string]any)
	assert.Equal(t, "main.tex", r0["path"])
	assert.Contains(t, r0["content"], `\documentclass{article}`)

	// history
	resp = ts.Do(t, http.MethodGet, base+"/compile/history", nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	hist := m["data"].(map[string]any)
	assert.Equal(t, float64(1), hist["total"])
}

// 编译失败 → 从 output.log 提取 "! ..." 错误行（_enrich_error_from_log 契约）
func TestCompileFailureEnrichesErrorFromLog(t *testing.T) {
	f := newFake("failure")
	f.logBody = "This is pdfTeX\n! Undefined control sequence.\nl.5 \\badcmd\n"
	clsi := httptest.NewServer(f.handler())
	defer clsi.Close()
	ts := testutil.NewWithClsi(t, clsi.URL)

	p := store.Project{Name: "失败项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	base := "/api/v1/projects/" + p.ID.String() + "/latex"
	seedOneTex(t, ts, base)

	job, err := ts.CompileQueue.Submit(context.Background(), p.ID.UUID, ts.UserID().UUID, "main.tex", "pdflatex")
	require.NoError(t, err)
	jobID := job.ID.String()
	var m map[string]any
	var resp *http.Response

	for i := 0; i < 50; i++ {
		resp = ts.Do(t, http.MethodGet, base+"/compile/"+jobID+"/status", nil)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
		if m["data"].(map[string]any)["status"] == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	d := m["data"].(map[string]any)
	assert.Equal(t, "failed", d["status"])
	require.NotNil(t, d["error"])
	assert.Contains(t, d["error"], "! Undefined control sequence.", "错误摘要取自 log 的 ! 行")
	require.NotNil(t, d["logs"])
	assert.Contains(t, d["logs"], "pdfTeX")
}

// conflict → 自动重试 full（第二次带全量文件）
func TestCompileConflictRetriesFull(t *testing.T) {
	f := newFake("conflict", "success")
	clsi := httptest.NewServer(f.handler())
	defer clsi.Close()
	ts := testutil.NewWithClsi(t, clsi.URL)

	p := store.Project{Name: "冲突项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	base := "/api/v1/projects/" + p.ID.String() + "/latex"
	seedOneTex(t, ts, base)

	job, err := ts.CompileQueue.Submit(context.Background(), p.ID.UUID, ts.UserID().UUID, "main.tex", "pdflatex")
	require.NoError(t, err)
	jobID := job.ID.String()
	var m map[string]any
	var resp *http.Response

	for i := 0; i < 50; i++ {
		resp = ts.Do(t, http.MethodGet, base+"/compile/"+jobID+"/status", nil)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
		if s := m["data"].(map[string]any)["status"]; s == "success" || s == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, "success", m["data"].(map[string]any)["status"])
	assert.Equal(t, 2, f.compileCount(), "conflict 应触发重试")

	second := compileBody(t, 1, f)
	opts := second["compile"].(map[string]any)["options"].(map[string]any)
	assert.Equal(t, "full", opts["syncType"], "重试必须 full")
}

// 404：查不存在的任务
func TestCompileStatusNotFound(t *testing.T) {
	ts := testutil.New(t)
	p := store.Project{Name: "P", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/latex/compile/00000000-0000-0000-0000-000000000000/status", nil)
	assert.Equal(t, 404, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "Compile job not found", m["info"])
}

// PDF 代理：全量 200 / HEAD 无 body / Range 206 / 越界 416（get_output_pdf 契约）
func TestOutputPDFRangeSupport(t *testing.T) {
	f := newFake()
	clsi := httptest.NewServer(f.handler())
	defer clsi.Close()
	ts := testutil.NewWithClsi(t, clsi.URL)
	p := store.Project{Name: "PDF项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	base := "/api/v1/projects/" + p.ID.String() + "/latex/output/b1/pdf"

	// 全量
	resp := ts.Do(t, http.MethodGet, base, nil)
	require.Equal(t, 200, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "PDFBYTES-0123456789AB", string(b))
	assert.Equal(t, "bytes", resp.Header.Get("Accept-Ranges"))
	assert.Equal(t, "21", resp.Header.Get("Content-Length"))

	// HEAD：无 body 但有 Content-Length
	req, _ := http.NewRequest(http.MethodHead, ts.URL+base, nil)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "21", resp.Header.Get("Content-Length"))
	hb, _ := io.ReadAll(resp.Body)
	assert.Empty(t, hb, "HEAD 不应有 body")

	// Range 0-9 → 206
	req, _ = http.NewRequest(http.MethodGet, ts.URL+base, nil)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Range", "bytes=0-9")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, 206, resp.StatusCode)
	assert.Equal(t, "bytes 0-9/21", resp.Header.Get("Content-Range"))
	chunk, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "PDFBYTES-0", string(chunk))

	// 越界 → 416
	req, _ = http.NewRequest(http.MethodGet, ts.URL+base, nil)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Range", "bytes=99-")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, 416, resp.StatusCode)
}

// wordcount / sync / zip 代理透传
func TestProxyEndpoints(t *testing.T) {
	f := newFake()
	clsi := httptest.NewServer(f.handler())
	defer clsi.Close()
	ts := testutil.NewWithClsi(t, clsi.URL)
	p := store.Project{Name: "代理项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	base := "/api/v1/projects/" + p.ID.String() + "/latex"

	// Legacy submission is a deliberate migration error and must not enqueue CLSI.
	resp := ts.Do(t, http.MethodPost, base+"/compile", []byte(`{"root_file":"main.tex","compiler":"pdflatex"}`))
	require.Equal(t, http.StatusGone, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Contains(t, m["info"], "browser engine")

	resp = ts.Do(t, http.MethodGet, base+"/wordcount?file=main.tex", nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, map[string]any{"words": float64(42)}, m["data"].(map[string]any)["texcount"])

	resp = ts.Do(t, http.MethodPost, base+"/sync/code", []byte(`{"file":"main.tex","line":5,"column":1}`))
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	pdf := m["data"].(map[string]any)["pdf"].([]any)
	require.Len(t, pdf, 1)
	pos := pdf[0].(map[string]any)
	assert.Equal(t, "main.tex", pos["file"])
	assert.Equal(t, float64(100.5), pos["h"])

	resp = ts.Do(t, http.MethodPost, base+"/sync/pdf", []byte(`{"page":1,"h":100,"v":200}`))
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Len(t, m["data"].(map[string]any)["code"], 1)

	resp = ts.Do(t, http.MethodGet, base+"/output/b1/output.zip", nil)
	require.Equal(t, 200, resp.StatusCode)
	zb, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ZIPDATA", string(zb))
	assert.Equal(t, "application/zip", resp.Header.Get("Content-Type"))

	// DELETE /compile（无 job_id）→ stopped；DELETE /cache → cleared
	resp = ts.Do(t, http.MethodDelete, base+"/compile", nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, true, m["data"].(map[string]any)["stopped"])

	resp = ts.Do(t, http.MethodDelete, base+"/cache", nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, true, m["data"].(map[string]any)["cleared"])
}

// ── 纯函数单元测试（契约：_compute_sync_params / _filter_files_for_incremental / build_resources）──

func pf(path, content string) latex.ProjectFile {
	return latex.ProjectFile{Path: path, Content: &content}
}

// seedOneTex 为直落库项目补一个 main.tex（空项目编译按契约报 No files）
func seedOneTex(t *testing.T, ts *testutil.TestServer, base string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"path": "main.tex", "content": `tex content placeholder`})
	resp := ts.Do(t, http.MethodPost, base+"-files/", body)
	require.Equal(t, 200, resp.StatusCode)
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func strp(s string) *string { return &s }

func jsonMap(t *testing.T, m map[string]string) datatypes.JSON {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return datatypes.JSON(b)
}

func TestComputeSyncParamsMatrix(t *testing.T) {
	files := []latex.ProjectFile{pf("main.tex", "A"), pf("s.tex", "B")}

	// 无 sync_state / 禁用 → full
	syncType, syncState, _ := latex.ComputeSyncParams(files, nil, true)
	assert.Equal(t, "full", syncType)
	assert.Nil(t, syncState)

	syncType, _, _ = latex.ComputeSyncParams(files, jsonMap(t, map[string]string{"main.tex": "x"}), false)
	assert.Equal(t, "full", syncType, "禁用增量时恒 full")

	// 键集变化（新增文件）→ full
	syncType, _, _ = latex.ComputeSyncParams(files, jsonMap(t, map[string]string{"main.tex": "x"}), true)
	assert.Equal(t, "full", syncType)

	// 内容完全一致 → full（Python 明确行为）
	same := map[string]string{}
	for _, f := range files {
		same[f.Path] = md5Hex(*f.Content)
	}
	syncType, _, _ = latex.ComputeSyncParams(files, jsonMap(t, same), true)
	assert.Equal(t, "full", syncType, "内容未变仍走 full（避免边界情况）")

	// 内容变化 → incremental + syncState=md5(prev "k:v:k:v")
	prev := map[string]string{"main.tex": md5Hex("OLD"), "s.tex": md5Hex("B")}
	syncType, syncState, current := latex.ComputeSyncParams(files, jsonMap(t, prev), true)
	assert.Equal(t, "incremental", syncType)
	require.NotNil(t, syncState)
	assert.Equal(t, md5Hex("main.tex:"+md5Hex("OLD")+":s.tex:"+md5Hex("B")), *syncState)
	assert.Equal(t, md5Hex("A"), current["main.tex"])
}

func TestFilterFilesForIncremental(t *testing.T) {
	bin := latex.ProjectFile{Path: "a.png", URL: strp("http://x"), IsBinary: true}
	changed := pf("main.tex", "NEW")
	unchanged := pf("s.tex", "B")

	prev := map[string]string{"main.tex": md5Hex("OLD"), "s.tex": md5Hex("B")}
	out := latex.FilterFilesForIncremental([]latex.ProjectFile{bin, changed, unchanged}, jsonMap(t, prev))
	// 二进制恒发 + 变更文本；未变更文本剔除
	paths := []string{}
	for _, f := range out {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"a.png", "main.tex"}, paths)
}

func TestBuildResourcesContract(t *testing.T) {
	// root 缺失 → 推断第一个 .tex
	_, root := latex.BuildResources([]latex.ProjectFile{pf("a.bib", "x"), pf("ch1.tex", "y")}, "main.tex")
	assert.Equal(t, "ch1.tex", root)

	// root 存在 → 保留
	_, root = latex.BuildResources([]latex.ProjectFile{pf("main.tex", "y")}, "main.tex")
	assert.Equal(t, "main.tex", root)

	// 无 .tex → main.tex 兜底
	_, root = latex.BuildResources([]latex.ProjectFile{pf("a.bib", "y")}, "main.tex")
	assert.Equal(t, "main.tex", root)

	// 二进制 → url 引用（无 content 字段）；文本 → content（无 url）
	bin := latex.ProjectFile{Path: "f.png", URL: strp("http://presign"), IsBinary: true}
	rs, _ := latex.BuildResources([]latex.ProjectFile{bin, pf("main.tex", "c")}, "main.tex")
	require.Len(t, rs, 2)
	assert.Equal(t, "http://presign", *rs[0].URL)
	assert.Nil(t, rs[0].Content, "二进制资源不应有 content（exclude_none）")
	require.NotNil(t, rs[1].Content)
	assert.Nil(t, rs[1].URL)
}

// 取消正在运行的任务（CLSI hang → cancel → cancelled）
func TestCancelRunningJob(t *testing.T) {
	f := newFake("hang")
	f.hangStarted = make(chan struct{})
	f.hangRelease = make(chan struct{})
	clsi := httptest.NewServer(f.handler())
	defer clsi.Close()
	defer close(f.hangRelease)
	ts := testutil.NewWithClsi(t, clsi.URL)
	p := store.Project{Name: "取消项目", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	base := "/api/v1/projects/" + p.ID.String() + "/latex"
	seedOneTex(t, ts, base)

	job, err := ts.CompileQueue.Submit(context.Background(), p.ID.UUID, ts.UserID().UUID, "main.tex", "pdflatex")
	require.NoError(t, err)
	jobID := job.ID.String()
	select {
	case <-f.hangStarted:
	case <-time.After(5 * time.Second):
		status := ts.CompileQueue.GetStatus(context.Background(), job.ID.UUID, ts.UserID().UUID)
		t.Fatalf("CLSI compile did not start before cancellation: status=%+v calls=%d error=%v", status, f.compileCount(), status.Error)
	}
	var m map[string]any
	var resp *http.Response

	resp = ts.Do(t, http.MethodDelete, base+"/compile?job_id="+jobID, nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, true, m["data"].(map[string]any)["cancelled"])

	// 取消不存在的 → 404
	resp = ts.Do(t, http.MethodDelete, base+"/compile?job_id=00000000-0000-0000-0000-000000000000", nil)
	assert.Equal(t, 404, resp.StatusCode)
}
