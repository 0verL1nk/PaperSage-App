package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// sseParts 解析 UI Message Stream：跳过终止标记 [DONE]（它不是 JSON part）。
func sseParts(t *testing.T, body io.Reader) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[6:]
		if payload == "[DONE]" {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &e))
		out = append(out, e)
	}
	return out
}

func partTypes(parts []map[string]any) []string {
	types := make([]string, 0, len(parts))
	for _, p := range parts {
		types = append(types, p["type"].(string))
	}
	return types
}

// Run 生命周期契约：创建 → 订阅 → 幂等 → 乐观锁 → 重试 → 取消。
func TestRunLifecycleContract(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/runs.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})

	uid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "run-user", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "RP", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	convID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentConversation{
		ID: convID, ProjectID: pid, UserID: uid, Title: "t"}).Error)

	fake := fakeOpenAI(t)
	cfg := &config.Settings{SecretKey: "t", AgentAPIKey: "k",
		AgentBaseURL: fake.URL, AgentModel: "test-model"}
	rgStore, rerr := rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
	require.NoError(t, rerr)
	svc := agent.NewService(db, testutil.NewMemStorage(), nil, cfg, rgStore)

	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != "" {
			rdr = bytes.NewReader([]byte(body))
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	type createResp struct {
		Data struct {
			TurnID        string `json:"turnId"`
			RunID         string `json:"runId"`
			UserMessageID string `json:"userMessageId"`
			StreamURL     string `json:"streamUrl"`
			ThreadVersion int64  `json:"threadVersion"`
		} `json:"data"`
	}
	runsPath := "/api/v1/agent/conversations/" + convID.String() + "/runs"

	// 1. 创建 Run
	rec := do(http.MethodPost, runsPath, `{"content":"看看我的文件","clientRequestId":"req-1","expectedThreadVersion":0}`)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var created createResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.RunID)
	assert.EqualValues(t, 1, created.Data.ThreadVersion)

	// 2. 订阅：协议版本头存在，part 以 finish 收尾
	rec = do(http.MethodGet, "/api/v1/agent/runs/"+created.Data.RunID+"/events", "")
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "v1", rec.Header().Get("x-vercel-ai-ui-message-stream"))
	types := partTypes(sseParts(t, rec.Body))
	require.NotEmpty(t, types)
	assert.Equal(t, "finish", types[len(types)-1], "流应以 finish 收尾")
	assert.Contains(t, types, "data-conversation")

	// 2b. 工具调用已落入状态机：有序号、有幂等键、已收敛到终态
	runUUID, err := uuid.Parse(created.Data.RunID)
	require.NoError(t, err)
	var calls []store.AgentToolCall
	require.NoError(t, db.Where("run_id = ?", store.UUID{UUID: runUUID}).
		Order("seq ASC").Find(&calls).Error)
	require.NotEmpty(t, calls, "工具调用应落库")
	assert.EqualValues(t, 1, calls[0].Seq, "序号从 1 开始且单调")
	assert.Equal(t, "list_latex_files", calls[0].ToolName)
	assert.Equal(t, "read", calls[0].RiskLevel, "只读工具自动执行")
	assert.NotNil(t, calls[0].IdempotencyKey, "应记录幂等键")
	assert.Equal(t, "succeeded", calls[0].State)

	// 3. 幂等：同一 clientRequestId 复用同一 Turn 与同一 Run，不重复执行
	rec = do(http.MethodPost, runsPath, `{"content":"看看我的文件","clientRequestId":"req-1","expectedThreadVersion":0}`)
	require.Equal(t, 200, rec.Code)
	var again createResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &again))
	assert.Equal(t, created.Data.TurnID, again.Data.TurnID, "应复用同一 Turn")
	assert.Equal(t, created.Data.RunID, again.Data.RunID, "应复用同一 Run（否则幂等键名存实亡）")

	// 4. 乐观锁：版本不匹配 → 409 且带服务端最新版本
	rec = do(http.MethodPost, runsPath, `{"content":"x","clientRequestId":"req-2","expectedThreadVersion":99}`)
	assert.Equal(t, 409, rec.Code)
	assert.Contains(t, rec.Body.String(), "threadVersion")

	// 5. 重试：复用同一 Turn 新建 Run
	rec = do(http.MethodPost, "/api/v1/agent/runs/"+created.Data.RunID+"/retry", "")
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "runId")

	// 6. 取消：终态后再取消不报错（cancelled=false）
	rec = do(http.MethodPost, "/api/v1/agent/runs/"+created.Data.RunID+"/cancel", "")
	require.Equal(t, 200, rec.Code)

	// 7. 越权：不存在/不属于自己的 Run → 404
	rec = do(http.MethodGet, "/api/v1/agent/runs/00000000-0000-0000-0000-000000000000/events", "")
	assert.Equal(t, 404, rec.Code)

	// 8. Run evidence is owner-scoped and includes the exact stored excerpt version.
	sourceID := store.MustNewUUID()
	chunkID := "document:" + sourceID.String() + ":2"
	sourceVersion := strings.Repeat("a", 64)
	locator := "chunk:2"
	quote := "a persisted source excerpt"
	sourceName := "research-plan.pdf"
	evidence := store.AgentEvidence{ID: store.MustNewUUID(), RunID: store.UUID{UUID: runUUID},
		SourceType: "document", SourceID: sourceID, SourceName: sourceName, SourceVersion: &sourceVersion,
		ChunkID: &chunkID, Locator: &locator, Quote: &quote}
	require.NoError(t, db.Create(&evidence).Error)
	rec = do(http.MethodGet, "/api/v1/agent/runs/"+created.Data.RunID+"/evidence", "")
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"sourceVersion":"`+sourceVersion+`"`)
	assert.Contains(t, rec.Body.String(), `"quote":"`+quote+`"`)
	assert.Contains(t, rec.Body.String(), `"sourceName":"`+sourceName+`"`)

	strangerID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "run-stranger", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: strangerID}}).Error)
	strangerToken, err := auth.CreateAccessToken(cfg.SecretKey, strangerID.String(), 30)
	require.NoError(t, err)
	strangerReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/agent/runs/"+created.Data.RunID+"/evidence", nil)
	strangerReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerRec := httptest.NewRecorder()
	api.ServeHTTP(strangerRec, strangerReq)
	assert.Equal(t, 404, strangerRec.Code)
	assert.NotContains(t, strangerRec.Body.String(), quote)
}

// lifecycleAPI uses an isolated SQLite file and a local scripted model only.
func lifecycleAPI(t *testing.T, modelURL string) (*agent.Service, *gorm.DB, string, func(string, string, string) *httptest.ResponseRecorder) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/lifecycle.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	uid, pid, cid := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "lifecycle", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	require.NoError(t, db.Create(&store.AgentConversation{ID: cid, ProjectID: pid, UserID: uid, Title: "t"}).Error)
	cfg := &config.Settings{SecretKey: "t", AgentAPIKey: "fake", AgentBaseURL: modelURL, AgentModel: "fake"}
	svc := agent.NewService(db, testutil.NewMemStorage(), nil, cfg, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	api := httpapi.New(nil)
	agent.Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	return svc, db, "/api/v1/agent/conversations/" + cid.String() + "/runs", do
}

func TestCreateRunIdempotentReplayAndConflict(t *testing.T) {
	var modelCalls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		modelCalls.Add(1)
		http.Error(w, "scripted failure", http.StatusBadRequest)
	}))
	defer fake.Close()
	svc, db, path, do := lifecycleAPI(t, fake.URL)
	first := do(http.MethodPost, path, `{"content":"question","clientRequestId":"same","expectedThreadVersion":0}`)
	id := createdRunID(t, first)
	assertRunTerminal(t, db, id, agent.RunFailed)
	assert.EqualValues(t, 1, modelCalls.Load())
	// The committed request is replayable even after its expected version and model settings change.
	svc.Cfg.AgentAPIKey = ""
	replay := do(http.MethodPost, path, `{"content":"question","clientRequestId":"same","expectedThreadVersion":0}`)
	assert.Equal(t, id, createdRunID(t, replay))
	changed := do(http.MethodPost, path, `{"content":"different","clientRequestId":"same","expectedThreadVersion":0}`)
	assert.Equal(t, http.StatusConflict, changed.Code, changed.Body.String())
	var turns, runs, messages int64
	require.NoError(t, db.Model(&store.AgentTurn{}).Count(&turns).Error)
	require.NoError(t, db.Model(&store.AgentRun{}).Count(&runs).Error)
	require.NoError(t, db.Model(&store.AgentMessage{}).Where("role = ?", "user").Count(&messages).Error)
	assert.EqualValues(t, 1, turns)
	assert.EqualValues(t, 1, runs)
	assert.EqualValues(t, 1, messages)
	var started int64
	require.NoError(t, db.Model(&store.AgentEvent{}).Where("run_id = ? AND event_type = ?", id, "run.started").Count(&started).Error)
	assert.EqualValues(t, 1, started)
	assert.EqualValues(t, 1, modelCalls.Load(), "replaying a completed Run must not execute the model again")
}

func TestCreateRunRollsBackAllRowsWhenInsertFails(t *testing.T) {
	for _, table := range []string{"agent_turns", "agent_messages", "agent_message_parts", "agent_runs", "agent_events"} {
		t.Run(table, func(t *testing.T) {
			_, db, path, do := lifecycleAPI(t, "http://127.0.0.1:1")
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-insert", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					_ = tx.AddError(assert.AnError)
				}
			}))
			rec := do(http.MethodPost, path, `{"content":"question","clientRequestId":"fail","expectedThreadVersion":0}`)
			assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
			var turns, runs, messages, parts, events int64
			require.NoError(t, db.Model(&store.AgentTurn{}).Count(&turns).Error)
			require.NoError(t, db.Model(&store.AgentRun{}).Count(&runs).Error)
			require.NoError(t, db.Model(&store.AgentMessage{}).Count(&messages).Error)
			require.NoError(t, db.Model(&store.AgentMessagePart{}).Count(&parts).Error)
			require.NoError(t, db.Model(&store.AgentEvent{}).Count(&events).Error)
			var conv store.AgentConversation
			require.NoError(t, db.First(&conv).Error)
			assert.Zero(t, turns)
			assert.Zero(t, runs)
			assert.Zero(t, messages)
			assert.Zero(t, parts)
			assert.Zero(t, events)
			assert.Zero(t, conv.Version)
		})
	}
}

func TestConcurrentCreateRunRequests(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "same key", false: "different keys"}[sameKey], func(t *testing.T) {
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "scripted failure", http.StatusBadRequest)
			}))
			defer fake.Close()
			_, db, path, do := lifecycleAPI(t, fake.URL)
			bodies := []string{
				`{"content":"question","clientRequestId":"key-1","expectedThreadVersion":0}`,
				`{"content":"question","clientRequestId":"key-1","expectedThreadVersion":0}`,
			}
			if !sameKey {
				bodies[1] = `{"content":"question","clientRequestId":"key-2","expectedThreadVersion":0}`
			}
			start := make(chan struct{})
			results := make([]*httptest.ResponseRecorder, 2)
			var wg sync.WaitGroup
			for i := range results {
				wg.Add(1)
				go func(i int) { defer wg.Done(); <-start; results[i] = do(http.MethodPost, path, bodies[i]) }(i)
			}
			close(start)
			wg.Wait()
			if sameKey {
				assert.Equal(t, createdRunID(t, results[0]), createdRunID(t, results[1]))
			} else {
				statuses := []int{results[0].Code, results[1].Code}
				assert.ElementsMatch(t, []int{http.StatusOK, http.StatusConflict}, statuses)
			}
			var turns, runs, messages int64
			require.NoError(t, db.Model(&store.AgentTurn{}).Count(&turns).Error)
			require.NoError(t, db.Model(&store.AgentRun{}).Count(&runs).Error)
			require.NoError(t, db.Model(&store.AgentMessage{}).Where("role = ?", "user").Count(&messages).Error)
			assert.EqualValues(t, 1, turns)
			assert.EqualValues(t, 1, runs)
			assert.EqualValues(t, 1, messages)
		})
	}
}

func TestLegacyChatReusesCommittedRun(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "scripted failure", http.StatusBadRequest)
	}))
	defer fake.Close()
	_, db, path, do := lifecycleAPI(t, fake.URL)
	id := createdRunID(t, do(http.MethodPost, path, `{"content":"question","clientRequestId":"same","expectedThreadVersion":0}`))
	var conv store.AgentConversation
	require.NoError(t, db.First(&conv).Error)
	legacyPath := "/api/v1/projects/" + conv.ProjectID.String() + "/agent/chat"
	body := `{"conversation_id":"` + conv.ID.String() + `","content":"question","clientRequestId":"same"}`
	rec := do(http.MethodPost, legacyPath, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), id.String())
	changed := `{"conversation_id":"` + conv.ID.String() + `","content":"different","clientRequestId":"same"}`
	assert.Equal(t, http.StatusConflict, do(http.MethodPost, legacyPath, changed).Code)
	var runs int64
	require.NoError(t, db.Model(&store.AgentRun{}).Count(&runs).Error)
	assert.EqualValues(t, 1, runs)
}

func createdRunID(t *testing.T, rec *httptest.ResponseRecorder) store.UUID {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	id, err := uuid.Parse(resp.Data.RunID)
	require.NoError(t, err)
	return store.UUID{UUID: id}
}

func assertRunTerminal(t *testing.T, db *gorm.DB, id store.UUID, status string) {
	t.Helper()
	var run store.AgentRun
	require.Eventually(t, func() bool {
		return db.Where("id = ?", id).First(&run).Error == nil && run.Status != agent.RunRunning
	}, 3*time.Second, 10*time.Millisecond, "cancelled Run must not remain running")
	assert.Equal(t, status, run.Status)
	assert.NotNil(t, run.CompletedAt)
	var events []store.AgentEvent
	require.NoError(t, db.Where("run_id = ? AND event_type = ?", id, "run.finished").Find(&events).Error)
	require.Len(t, events, 1, "exactly one durable terminal event")
	assert.JSONEq(t, `{"status":"`+status+`"}`, string(events[0].PayloadJSON))
}

func TestChildRunClosesBeforeParentOnCancelAndError(t *testing.T) {
	for _, outcome := range []string{"cancel", "model-error", "stream-error"} {
		t.Run(outcome, func(t *testing.T) {
			var calls atomic.Int32
			release := make(chan struct{})
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				n := calls.Add(1)
				if n == 1 {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"task1\",\"type\":\"function\",\"function\":{\"name\":\"task\",\"arguments\":\"{\\\"subagent_type\\\":\\\"search_references\\\",\\\"description\\\":\\\"search\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				if outcome == "model-error" && n == 2 {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"c2\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"rag1\",\"type\":\"function\",\"function\":{\"name\":\"rag_search\",\"arguments\":\"{\\\"query\\\":\\\"test\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				if outcome == "model-error" {
					http.Error(w, `{"error":{"message":"broken model","type":"server_error"}}`, http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"id\":\"c2\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"private child text\"}}]}\n\n")
				w.(http.Flusher).Flush()
				if outcome == "stream-error" {
					_, _ = io.WriteString(w, "data: {\"error\":{\"message\":\"broken stream\",\"type\":\"server_error\"}}\n\n")
					return
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(fake.Close)
			t.Cleanup(func() { close(release) })
			svc, db, path, do := lifecycleAPI(t, fake.URL)
			svc.RAG, _ = rag.NewStore(db, rag.HashEmbedder{Dim: 64}, "")
			id := createdRunID(t, do(http.MethodPost, path, `{"content":"search","expectedThreadVersion":0}`))
			t.Cleanup(func() { svc.CancelRun(id) })
			var children []store.AgentRun
			require.Eventually(t, func() bool {
				return db.Where("parent_run_id = ?", id).Find(&children).Error == nil && len(children) > 0
			}, 3*time.Second, 10*time.Millisecond)
			status := agent.RunFailed
			if outcome == "cancel" {
				status = agent.RunCancelled
				require.True(t, svc.CancelRun(id))
			}
			assertRunTerminal(t, db, id, status)
			require.NoError(t, db.Where("parent_run_id = ?", id).Find(&children).Error)
			for _, child := range children {
				assert.Equal(t, status, child.Status)
				assert.NotNil(t, child.CompletedAt)
			}
			var events []store.AgentEvent
			require.NoError(t, db.Where("run_id = ?", id).Order("sequence").Find(&events).Error)
			terminalActivity := false
			for _, event := range events {
				if event.EventType == "activity" && bytes.Contains(event.PayloadJSON, []byte(`"status":"`+status+`"`)) {
					terminalActivity = true
				}
				if event.EventType == "run.finished" {
					assert.True(t, terminalActivity, "child terminal activity must arrive before the parent stream ends")
				}
			}
			require.NoError(t, svc.Close(context.Background()))
		})
	}
}

func TestRunExecutesModelCapturedAtCreation(t *testing.T) {
	requests := make(chan map[string]any, 1)
	authHeader := make(chan string, 1)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests <- body
		authHeader <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(fake.Close)
	_, db, path, do := lifecycleAPI(t, fake.URL)
	var conv store.AgentConversation
	require.NoError(t, db.First(&conv).Error)
	settings := func(model string) []byte {
		b, err := json.Marshal(map[string]any{"active_provider_id": "p", "providers": []any{map[string]any{
			"id": "p", "base_url": fake.URL, "api_key": "fake-snapshot-key", "models": []string{"A", "B"}, "active_model": model,
		}}})
		require.NoError(t, err)
		return b
	}
	require.NoError(t, db.Create(&store.UserSettings{UserID: conv.UserID, Category: "agent", Settings: datatypes.JSON(settings("A"))}).Error)
	changed := settings("B")
	require.NoError(t, db.Callback().Create().After("gorm:create").Before("gorm:commit_or_rollback_transaction").Register("test:change-model", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_runs" {
			if e := tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE user_settings SET settings = ? WHERE user_id = ? AND category = ?", string(changed), conv.UserID, "agent").Error; e != nil {
				_ = tx.AddError(e)
			}
		}
	}))
	defer func() { require.NoError(t, db.Callback().Create().Remove("test:change-model")) }()
	id := createdRunID(t, do(http.MethodPost, path, `{"content":"hello","expectedThreadVersion":0}`))
	var request map[string]any
	select {
	case request = <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("fake model was not called")
	}
	assert.Equal(t, "A", request["model"])
	assert.Equal(t, "Bearer fake-snapshot-key", <-authHeader)
	assertRunTerminal(t, db, id, agent.RunCompleted)
	var run store.AgentRun
	require.NoError(t, db.First(&run, "id = ?", id).Error)
	assert.Equal(t, "A", run.ModelName)
	raw, err := json.Marshal(run)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "fake-snapshot-key")
	var events []store.AgentEvent
	require.NoError(t, db.Where("run_id = ?", id).Find(&events).Error)
	for _, event := range events {
		assert.NotContains(t, string(event.PayloadJSON), "fake-snapshot-key")
	}
}

func TestRunSettingsDatabaseFailureIsNotReportedAsMissingModel(t *testing.T) {
	_, db, path, do := lifecycleAPI(t, "http://unused.invalid")
	require.NoError(t, db.Migrator().DropTable(&store.UserSettings{}))
	rec := do(http.MethodPost, path, `{"content":"hello","expectedThreadVersion":0}`)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "尚未配置")
	assert.NotContains(t, rec.Body.String(), "user_settings")
}

func TestCloseDuringRunCreationDoesNotLeaveRunningRow(t *testing.T) {
	svc, db, path, do := lifecycleAPI(t, "http://unused.invalid")
	entered := make(chan struct{})
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:pause-turn", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_turns" {
			close(entered)
			<-tx.Statement.Context.Done()
		}
	}))
	defer func() { require.NoError(t, db.Callback().Update().Remove("test:pause-turn")) }()
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() { response <- do(http.MethodPost, path, `{"content":"work","expectedThreadVersion":0}`) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Run creation did not reach the transaction window")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, svc.Close(ctx), context.DeadlineExceeded)
	<-response
	var runs []store.AgentRun
	require.NoError(t, db.Find(&runs).Error)
	assert.Empty(t, runs, "shutdown before commit rolls back the whole submission")
	var turns, messages, events int64
	require.NoError(t, db.Model(&store.AgentTurn{}).Count(&turns).Error)
	require.NoError(t, db.Model(&store.AgentMessage{}).Count(&messages).Error)
	require.NoError(t, db.Model(&store.AgentEvent{}).Count(&events).Error)
	assert.Zero(t, turns)
	assert.Zero(t, messages)
	assert.Zero(t, events)
}

func TestServiceCloseCancelsAndRejectsRuns(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(fake.Close)
	t.Cleanup(func() { close(release) })
	svc, db, path, do := lifecycleAPI(t, fake.URL)
	id := createdRunID(t, do(http.MethodPost, path, `{"content":"work","expectedThreadVersion":0}`))
	t.Cleanup(func() { svc.CancelRun(id) })
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("model was not called")
	}
	// A foreign running row must not be cancelled by this Service's shutdown.
	foreign := store.AgentRun{ID: store.MustNewUUID(), ConversationID: store.MustNewUUID(),
		AgentKey: "main", Status: agent.RunRunning}
	require.NoError(t, db.Create(&foreign).Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, svc.Close(ctx), context.Canceled)
	assertRunTerminal(t, db, id, agent.RunCancelled)
	var cancelled store.AgentRun
	require.NoError(t, db.First(&cancelled, "id = ?", id).Error)
	assert.Contains(t, string(cancelled.ErrorJSON), "agent service is shutting down")
	require.NoError(t, db.Where("id = ?", foreign.ID).First(&foreign).Error)
	assert.Equal(t, agent.RunRunning, foreign.Status)
	var before int64
	require.NoError(t, db.Model(&store.AgentRun{}).Count(&before).Error)
	for _, target := range []string{path, "/api/v1/agent/runs/" + id.String() + "/retry"} {
		rec := do(http.MethodPost, target, `{"content":"new work"}`)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
	var conv store.AgentConversation
	require.NoError(t, db.First(&conv).Error)
	rec := do(http.MethodPost, "/api/v1/projects/"+conv.ProjectID.String()+"/agent/chat", `{"content":"new work"}`)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	var after int64
	require.NoError(t, db.Model(&store.AgentRun{}).Count(&after).Error)
	assert.Equal(t, before, after, "rejected requests must not create running rows")
	require.NoError(t, svc.Close(context.Background()), "Close is repeatable after draining")
	assertRunTerminal(t, db, id, agent.RunCancelled)
}

func TestServiceCloseWaitsForCompletion(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"finished\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(fake.Close)
	svc, db, path, do := lifecycleAPI(t, fake.URL)
	id := createdRunID(t, do(http.MethodPost, path, `{"content":"work","expectedThreadVersion":0}`))
	t.Cleanup(func() { svc.CancelRun(id) })
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("model was not called")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Close(ctx) }()
	select {
	case err := <-done:
		close(release)
		t.Fatalf("Close returned before in-flight work finished: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	rec := do(http.MethodPost, path, `{"content":"rejected during drain","expectedThreadVersion":0}`)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	close(release)
	require.NoError(t, <-done)
	assertRunTerminal(t, db, id, agent.RunCompleted)
}

func TestCancelWhileModelBlockedPersistsTerminal(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "waiting-for-model"
		if partial {
			name = "receiving-stream"
		}
		t.Run(name, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if partial {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"partial\"}}]}\n\n")
					w.(http.Flusher).Flush()
				}
				close(entered)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			t.Cleanup(fake.Close)
			t.Cleanup(func() { close(release) })
			svc, db, path, do := lifecycleAPI(t, fake.URL)
			id := createdRunID(t, do(http.MethodPost, path, `{"content":"keep working","expectedThreadVersion":0}`))
			t.Cleanup(func() { svc.CancelRun(id) })
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("model was not called")
			}
			if partial {
				require.Eventually(t, func() bool {
					var n int64
					return db.Model(&store.AgentEvent{}).Where("run_id = ? AND event_type = ?", id, "text.delta").Count(&n).Error == nil && n > 0
				}, 3*time.Second, 10*time.Millisecond)
			}
			rec := do(http.MethodPost, "/api/v1/agent/runs/"+id.String()+"/cancel", "")
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), `"cancelled":true`)
			assertRunTerminal(t, db, id, agent.RunCancelled)
		})
	}
}

func TestCreateRunRequiresExpectedVersion(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "scripted failure", http.StatusBadRequest)
	}))
	defer fake.Close()
	_, _, path, do := lifecycleAPI(t, fake.URL)
	rec := do(http.MethodPost, path, `{"content":"question","clientRequestId":"no-version"}`)
	require.Equal(t, http.StatusPreconditionRequired, rec.Code, rec.Body.String())
}

func TestRetryIsIdempotentForSameClientRequest(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "scripted failure", http.StatusBadRequest)
	}))
	defer fake.Close()
	_, db, path, do := lifecycleAPI(t, fake.URL)
	id := createdRunID(t, do(http.MethodPost, path, `{"content":"question","clientRequestId":"first","expectedThreadVersion":0}`))
	assertRunTerminal(t, db, id, agent.RunFailed)

	retryPath := "/api/v1/agent/runs/" + id.String() + "/retry"
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = do(http.MethodPost, retryPath, `{"clientRequestId":"retry-1"}`)
		}(i)
	}
	wg.Wait()
	require.Equal(t, http.StatusOK, results[0].Code, results[0].Body.String())
	require.Equal(t, http.StatusOK, results[1].Code, results[1].Body.String())
	first := createdRunID(t, results[0])
	second := createdRunID(t, results[1])
	assert.Equal(t, first, second, "同一 clientRequestId 的并发重试必须返回同一个 Run")
	var runs int64
	require.NoError(t, db.Model(&store.AgentRun{}).Count(&runs).Error)
	assert.EqualValues(t, 2, runs, "原始 Run + 一个重试 Run")
}
