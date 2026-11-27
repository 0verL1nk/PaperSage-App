package testutil_test

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 高频主操作的 API 基准：/health、24 KB LaTeX 写入、写作提案 apply。
// 计时只覆盖 HTTP 请求本身，数据准备放在 StopTimer 区间；`queries/op`
// 是这次请求发出的 SQL 条数——远程库上每条都是一次网络往返。

const (
	benchFilePath = "ch2_tech.tex"
	benchFileSize = 24 * 1024
	benchAnchor   = "ANCHOR_TEXT_UNDER_TEST"
)

func benchDo(tb testing.TB, ts *testutil.TestServer, method, path string, body []byte) (int, []byte) {
	tb.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		tb.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		tb.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, out
}

func benchMustOK(tb testing.TB, status int, body []byte, what string) {
	tb.Helper()
	if status != http.StatusOK {
		tb.Fatalf("%s: status %d body %s", what, status, body)
	}
}

func benchCreateProject(tb testing.TB, ts *testutil.TestServer) uuid.UUID {
	tb.Helper()
	body, _ := json.Marshal(map[string]string{
		"name": "bench-" + uuid.NewString()[:8], "scenario": "undergraduate_thesis",
	})
	status, out := benchDo(tb, ts, http.MethodPost, "/api/v1/projects/", body)
	benchMustOK(tb, status, out, "create project")
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		tb.Fatalf("decode project: %v (%s)", err, out)
	}
	id, err := uuid.Parse(env.Data.ID)
	if err != nil {
		tb.Fatalf("project id: %v", err)
	}
	return id
}

// benchText 生成固定大小的 ASCII 正文，可注入一个锚点。
func benchText(anchor string) string {
	line := "The system grounds every suggestion in retrieved evidence and adopted norms.\n"
	buf := make([]byte, 0, benchFileSize)
	if anchor != "" {
		buf = append(buf, anchor...)
		buf = append(buf, '\n')
	}
	for len(buf) < benchFileSize {
		buf = append(buf, line...)
	}
	return string(buf[:benchFileSize])
}

func BenchmarkHotPathHealth(b *testing.B) {
	ts := testutil.New(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		status, out := benchDo(b, ts, http.MethodGet, "/health", nil)
		benchMustOK(b, status, out, "health")
	}
}

func BenchmarkHotPathLatexWrite24KB(b *testing.B) {
	ts := testutil.New(b)
	pid := benchCreateProject(b, ts)
	base := benchText("")
	fileURL := "/api/v1/projects/" + pid.String() + "/latex-files"
	body, _ := json.Marshal(map[string]string{"path": benchFilePath, "content": base})
	status, out := benchDo(b, ts, http.MethodPost, fileURL, body)
	benchMustOK(b, status, out, "create file")

	counter := testutil.CountQueries(ts.DB)
	putURL := fileURL + "/" + benchFilePath
	var queries int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每轮内容都不同：走真实写路径（S3 Put + 元数据 UPDATE），不被 md5 短路。
		body, _ := json.Marshal(map[string]string{"content": fmt.Sprintf("%s\n%% rev %d", base, i)})
		counter.Reset()
		status, out := benchDo(b, ts, http.MethodPut, putURL, body)
		benchMustOK(b, status, out, "write file")
		queries += counter.Value()
	}
	b.StopTimer()
	b.ReportMetric(float64(queries)/float64(b.N), "queries/op")
}

func BenchmarkHotPathProposalApply(b *testing.B) {
	ts := testutil.New(b)
	pid := benchCreateProject(b, ts)
	content := benchText(benchAnchor)
	mainURL := "/api/v1/projects/" + pid.String() + "/latex-files/main.tex"
	body, _ := json.Marshal(map[string]string{"content": content})
	status, out := benchDo(b, ts, http.MethodPut, mainURL, body)
	benchMustOK(b, status, out, "seed main.tex")

	sum := md5.Sum([]byte(content))
	baseHash := hex.EncodeToString(sum[:])
	counter := testutil.CountQueries(ts.DB)
	var queries int64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// 复位稿件与建议：apply 是终态操作，每轮都要一条新的 pending 建议。
		body, _ := json.Marshal(map[string]string{"content": content})
		status, out := benchDo(b, ts, http.MethodPut, mainURL, body)
		benchMustOK(b, status, out, "reset main.tex")
		p := store.AgentChangeProposal{
			ID: store.MustNewUUID(), ProjectID: store.UUID{UUID: pid},
			FilePath: "main.tex", BaseContentHash: baseHash,
			OldText: benchAnchor, NewText: fmt.Sprintf("REPLACED_%d", i),
			Status: "pending",
		}
		if err := ts.DB.Create(&p).Error; err != nil {
			b.Fatalf("seed proposal: %v", err)
		}
		counter.Reset()
		b.StartTimer()

		applyURL := "/api/v1/agent/change-proposals/" + p.ID.String() + "/apply"
		status, out = benchDo(b, ts, http.MethodPost, applyURL, nil)
		benchMustOK(b, status, out, "apply proposal")
		queries += counter.Value()
	}
	b.StopTimer()
	b.ReportMetric(float64(queries)/float64(b.N), "queries/op")
}
