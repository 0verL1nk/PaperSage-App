package review_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 这一组测试用假的 OpenAI 兼容服务驱动真实 Eino ReAct 循环：
// 单元照样走「读上下文 → 读原文/资料 → 提交报告」的多步工具调用，
// 覆盖此前只有真实模型才会暴露的迭代上限与单元失败隔离问题。

type fakeReply struct {
	Tool string
	Args string
	Hang bool
}

type fakeModelServer struct {
	*httptest.Server
	seq atomic.Int64
}

func newFakeModelServer(t *testing.T) *fakeModelServer {
	t.Helper()
	f := &fakeModelServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeModelServer) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.NotFound(w, r)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var req struct {
		Stream   *bool `json:"stream"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &req)
	var text strings.Builder
	systemText := ""
	assistantRounds := 0
	for _, m := range req.Messages {
		content := flattenContent(m.Content)
		text.WriteString(content)
		text.WriteByte('\n')
		if m.Role == "system" {
			systemText = content
		}
		if m.Role == "assistant" {
			assistantRounds++
		}
	}
	reply := fakeScript(detectDimension(systemText), assistantRounds, text.String())
	if reply.Hang {
		<-r.Context().Done()
		return
	}
	if req.Stream != nil && *req.Stream {
		f.writeStream(w, reply)
		return
	}
	f.writeJSON(w, reply)
}

// flattenContent 兼容 OpenAI 的两种 content 形态：纯字符串或多段 [{type,text}]。
func flattenContent(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return string(raw)
}

// detectDimension 从单元指令的报告契约（"key":"<维度>"）里认出当前单元的维度。
// 不能用「文本包含维度名」判断：报告契约里会顺带提到 evidence_support 等其他维度。
func detectDimension(systemText string) string {
	if m := dimKeyRe.FindStringSubmatch(systemText); len(m) == 2 {
		return m[1]
	}
	return ""
}

var dimKeyRe = regexp.MustCompile(`"key":"([a-z_]+)"`)

var texPathRe = regexp.MustCompile(`[A-Za-z0-9_./-]+\.tex`)

// fakeScript 定义每个维度的行为：
//   - goal_conclusion：正常收敛（上下文 → 读原文 → 提交 insufficient）
//   - cross_section：一直读文件，耗尽迭代上限
//   - terminology_consistency：挂起不返回，触发单元超时
func fakeScript(dim string, round int, text string) fakeReply {
	file := "body/ch1.tex"
	if paths := texPathRe.FindAllString(text, -1); len(paths) > 0 {
		file = paths[0]
	}
	switch dim {
	case "cross_section":
		return fakeReply{Tool: "read_latex_file", Args: fmt.Sprintf(`{"path":%q}`, file)}
	case "terminology_consistency":
		return fakeReply{Hang: true}
	}
	switch round {
	case 0:
		return fakeReply{Tool: "get_review_unit_context", Args: `{}`}
	case 1:
		return fakeReply{Tool: "read_latex_file", Args: fmt.Sprintf(`{"path":%q}`, file)}
	}
	payload := fmt.Sprintf(`{"dimensions":[{"key":%q,"status":"insufficient","note":"测试：未提供章节计划或已采用规范"}],"findings":[]}`, dim)
	args, _ := json.Marshal(map[string]string{"report_json": payload})
	return fakeReply{Tool: "submit_review_unit_report", Args: string(args)}
}

func (f *fakeModelServer) writeStream(w http.ResponseWriter, reply fakeReply) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	id := fmt.Sprintf("chatcmpl-%d", f.seq.Add(1))
	callID := fmt.Sprintf("call_%d", f.seq.Add(1))
	writeSSE(w, flusher, map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": 1, "model": "fake-model",
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"index": 0, "id": callID, "type": "function",
					"function": map[string]any{"name": reply.Tool, "arguments": reply.Args},
				}},
			},
			"finish_reason": nil,
		}},
	})
	writeSSE(w, flusher, map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": 1, "model": "fake-model",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
	})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func (f *fakeModelServer) writeJSON(w http.ResponseWriter, reply fakeReply) {
	callID := fmt.Sprintf("call_%d", f.seq.Add(1))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-json", "object": "chat.completion", "created": 1, "model": "fake-model",
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"id": callID, "type": "function",
					"function": map[string]any{"name": reply.Tool, "arguments": reply.Args},
				}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, payload any) {
	encoded, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
	if flusher != nil {
		flusher.Flush()
	}
}

// reviewTestEnv 是跑一次真实编排所需的项目与文件。
type reviewTestEnv struct {
	ts  *testutil.TestServer
	pid string
}

func newReviewTestEnv(t *testing.T, fake *fakeModelServer) *reviewTestEnv {
	t.Helper()
	ts := testutil.New(t)
	ts.ReviewService.Cfg.AgentBaseURL = fake.URL
	ts.ReviewService.Cfg.AgentAPIKey = "fake-key"
	ts.ReviewService.Cfg.AgentModel = "fake-model"

	resp := ts.Do(t, http.MethodPost, "/api/v1/projects", []byte(`{"name":"review-orchestrator-test"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var project struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&project))
	require.NotEmpty(t, project.Data.ID)

	main := "\\documentclass{article}\n\\begin{document}\n\\input{body/ch1}\n\\input{body/ch2}\n\\end{document}\n"
	resp = ts.Do(t, http.MethodPut,
		"/api/v1/projects/"+project.Data.ID+"/latex-files/main.tex",
		[]byte(`{"content":`+quoteJSON(main)+`}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	for path, content := range map[string]string{
		"body/ch1.tex": "\\chapter{绪论}\n本文的样本为 120 名学生。\n",
		"body/ch2.tex": "\\chapter{方法}\n本文的样本为 80 名学生。\n",
	} {
		resp := ts.Do(t, http.MethodPost, "/api/v1/projects/"+project.Data.ID+"/latex-files",
			[]byte(`{"path":"`+path+`","content":`+quoteJSON(content)+`}`))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	return &reviewTestEnv{ts: ts, pid: project.Data.ID}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type runSnapshot struct {
	Status     string  `json:"status"`
	Error      string  `json:"error"`
	ArtifactID *string `json:"artifactId"`
	Units      []struct {
		Label      string   `json:"label"`
		Dimensions []string `json:"dimensions"`
		Status     string   `json:"status"`
		Error      string   `json:"error"`
	} `json:"units"`
}

func (e *reviewTestEnv) startReview(t *testing.T, clientRequestID string, dims ...string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"clientRequestId": clientRequestID,
		"scope": map[string]any{
			"filePaths":  []string{"body/ch1.tex", "body/ch2.tex"},
			"dimensions": dims,
		},
	})
	resp := e.ts.Do(t, http.MethodPost, "/api/v1/projects/"+e.pid+"/review-runs", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var run struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&run))
	require.NotEmpty(t, run.Data.ID)
	return run.Data.ID
}

func (e *reviewTestEnv) waitRun(t *testing.T, runID string, timeout time.Duration) runSnapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := e.ts.Do(t, http.MethodGet, "/api/v1/projects/"+e.pid+"/review-runs/"+runID, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out struct {
			Data runSnapshot `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		switch out.Data.Status {
		case store.ReviewRunSucceeded, store.ReviewRunPartial, store.ReviewRunFailed, store.ReviewRunCancelled:
			return out.Data
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("review run %s did not finish in %s", runID, timeout)
	return runSnapshot{}
}

func containsChatWording(s string) bool {
	return strings.Contains(s, "发送") || strings.Contains(s, "让助手接着处理")
}

// 一个单元耗尽迭代上限时，其余单元必须照常完成并落库；失败原因要指向可执行动作，
// 不能出现聊天态的「发送继续」。
func TestReviewRunPersistsPartialReportWhenOneUnitExhaustsBudget(t *testing.T) {
	fake := newFakeModelServer(t)
	env := newReviewTestEnv(t, fake)
	env.ts.ReviewService.Cfg.ReviewUnitConcurrency = 2

	runID := env.startReview(t, "exhaust-one", "goal_conclusion", "cross_section")
	run := env.waitRun(t, runID, 120*time.Second)

	require.Equal(t, store.ReviewRunPartial, run.Status, "one failed unit must not fail the whole run: %s", run.Error)
	require.NotNil(t, run.ArtifactID, "partial run must persist a chapter_review artifact")

	succeeded, failed := 0, 0
	for _, unit := range run.Units {
		switch unit.Status {
		case store.ReviewUnitSucceeded:
			succeeded++
			require.Equal(t, "goal_conclusion", unit.Dimensions[0])
		case store.ReviewUnitFailed:
			failed++
			require.Equal(t, "cross_section", unit.Dimensions[0])
			require.Contains(t, unit.Error, "步数上限")
			require.False(t, containsChatWording(unit.Error), "unit error must not use chat wording: %s", unit.Error)
		default:
			t.Fatalf("unit %s ended in %q: %s", unit.Label, unit.Status, unit.Error)
		}
	}
	require.Equal(t, 2, succeeded, "both goal_conclusion chapter units must complete")
	require.Equal(t, 1, failed, "the cross_section unit must fail alone")
	require.Contains(t, run.Error, "1 个检查单元失败")
}

// 全部单元都失败时任务必须失败并给出可读原因，而不是留下 0 产物的空壳。
func TestReviewRunAllUnitsExhaustedReportsActionableReason(t *testing.T) {
	fake := newFakeModelServer(t)
	env := newReviewTestEnv(t, fake)

	runID := env.startReview(t, "exhaust-all", "cross_section")
	run := env.waitRun(t, runID, 120*time.Second)

	require.Equal(t, store.ReviewRunFailed, run.Status)
	require.Nil(t, run.ArtifactID)
	require.Contains(t, run.Error, "所有检查单元都失败了")
	require.Contains(t, run.Error, "步数上限")
	require.False(t, containsChatWording(run.Error), "run error must not use chat wording: %s", run.Error)
	for _, unit := range run.Units {
		require.Equal(t, store.ReviewUnitFailed, unit.Status)
	}
}

// 一个单元卡住不返回时，只终止它自己：其余单元仍要完成并落库（此前会拖到 Run 总超时一起失败）。
func TestReviewRunUnitTimeoutDoesNotCascade(t *testing.T) {
	fake := newFakeModelServer(t)
	env := newReviewTestEnv(t, fake)
	env.ts.ReviewService.Cfg.ReviewUnitTimeout = 1500 * time.Millisecond

	runID := env.startReview(t, "hang-one", "goal_conclusion", "terminology_consistency")
	run := env.waitRun(t, runID, 90*time.Second)

	require.Equal(t, store.ReviewRunPartial, run.Status, "hung unit must fail alone: %s", run.Error)
	require.NotNil(t, run.ArtifactID)

	var timedOut, succeeded int
	for _, unit := range run.Units {
		switch unit.Dimensions[0] {
		case "terminology_consistency":
			require.Equal(t, store.ReviewUnitFailed, unit.Status)
			require.Contains(t, unit.Error, "未完成")
			require.NotContains(t, unit.Error, "已取消", "unit timeout must not look like a user cancel")
			timedOut++
		case "goal_conclusion":
			require.Equal(t, store.ReviewUnitSucceeded, unit.Status, unit.Error)
			succeeded++
		}
	}
	require.Equal(t, 1, timedOut)
	require.Equal(t, 2, succeeded)
}
