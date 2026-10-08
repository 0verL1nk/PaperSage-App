package writingstate_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

func createProject(t *testing.T, ts *testutil.TestServer, name, scenario string) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "scenario": scenario})
	require.NoError(t, err)
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	return envelope.Data
}

func decodeEnvelope(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	return envelope
}

func stateBody(revision int, topic string) []byte {
	body, _ := json.Marshal(map[string]any{
		"revision": revision,
		"brief": map[string]any{
			"topic": topic, "background": "背景", "questions": []string{"问题"}, "goals": []string{"目标"},
			"scope_out": []string{}, "objects_and_data": "对象", "methods": "访谈",
			"key_chapters": []string{}, "milestones": []string{}, "constraints": []string{},
			"known_facts": []string{}, "assumptions": []string{}, "unverified": []string{},
		},
		"requirements":  []any{},
		"phases":        map[string]string{"prepare": "in_progress", "write": "not_started", "review": "not_started"},
		"current_phase": "prepare",
	})
	return body
}

func TestDefaultSaveConflictAndValidation(t *testing.T) {
	ts := testutil.New(t)
	project := createProject(t, ts, "写作状态契约", "intro_paper")
	id := project["id"].(string)
	endpoint := "/api/v1/projects/" + id + "/writing-state"

	resp := ts.Do(t, http.MethodGet, endpoint, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defaultState := decodeEnvelope(t, resp)["data"].(map[string]any)
	assert.Equal(t, float64(0), defaultState["revision"])
	assert.Equal(t, "intro_paper", defaultState["scenario"])
	assert.Equal(t, "in_progress", defaultState["phases"].(map[string]any)["prepare"])

	resp = ts.Do(t, http.MethodPut, endpoint, stateBody(0, "已保存主题"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	saved := decodeEnvelope(t, resp)["data"].(map[string]any)
	assert.Equal(t, float64(1), saved["revision"])

	resp = ts.Do(t, http.MethodPut, endpoint, stateBody(0, "过期覆盖"))
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	conflict := decodeEnvelope(t, resp)
	assert.Equal(t, float64(http.StatusConflict), conflict["code"])
	assert.Equal(t, "已保存主题", conflict["data"].(map[string]any)["brief"].(map[string]any)["topic"])

	var tooLong map[string]any
	require.NoError(t, json.Unmarshal(stateBody(1, ""), &tooLong))
	tooLong["brief"].(map[string]any)["background"] = strings.Repeat("x", 20_001)
	body, err := json.Marshal(tooLong)
	require.NoError(t, err)
	resp = ts.Do(t, http.MethodPut, endpoint, body)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "单个文本字段超过 20000 字符应被拒绝")

	oversized := append(stateBody(1, ""), []byte(strings.Repeat(" ", 256*1024))...)
	resp = ts.Do(t, http.MethodPut, endpoint, oversized)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "请求体超过 256KB 应被拒绝")

	bad := map[string]any{"revision": 1, "brief": map[string]any{}, "requirements": []any{},
		"phases": map[string]string{"prepare": "in_progress", "write": "invented", "review": "not_started"}, "current_phase": "prepare"}
	body, _ = json.Marshal(bad)
	resp = ts.Do(t, http.MethodPut, endpoint, body)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	resp = ts.Do(t, http.MethodGet, endpoint, nil)
	latest := decodeEnvelope(t, resp)["data"].(map[string]any)
	assert.Equal(t, "已保存主题", latest["brief"].(map[string]any)["topic"])
}

func TestOwnerIsolationAndLegacyDefaultMerge(t *testing.T) {
	ts := testutil.New(t)
	other := store.User{Username: "writing-state-owner", PasswordHash: "x"}
	require.NoError(t, ts.DB.Create(&other).Error)
	privateProject := store.Project{Name: "私有项目", OwnerID: other.ID, Scenario: "undergraduate_thesis"}
	require.NoError(t, ts.DB.Create(&privateProject).Error)
	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/"+privateProject.ID.String()+"/writing-state", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	project := createProject(t, ts, "旧状态", "undergraduate_thesis")
	projectID := project["id"].(string)
	row := store.ProjectWritingState{
		Brief:        datatypes.JSON(`{"topic":"legacy","questions":null}`),
		Requirements: datatypes.JSON(`[]`), Phases: datatypes.JSON(`{"prepare":"done"}`),
		CurrentPhase: "write", Revision: 3,
	}
	row.ProjectID.UUID = mustParseUUID(projectID)
	require.NoError(t, ts.DB.Create(&row).Error)
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+projectID+"/writing-state", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	merged := decodeEnvelope(t, resp)["data"].(map[string]any)
	brief := merged["brief"].(map[string]any)
	assert.Equal(t, "legacy", brief["topic"])
	assert.NotNil(t, brief["questions"], "missing fields merge with v1 defaults")
	assert.IsType(t, []any{}, brief["questions"], "legacy null arrays keep safe defaults")
	phases := merged["phases"].(map[string]any)
	assert.Equal(t, "done", phases["prepare"])
	assert.Equal(t, "not_started", phases["review"])
	assert.Equal(t, float64(3), merged["revision"])
}

func TestAgentBriefSaveRequiresCurrentRevisionAndPreservesOtherState(t *testing.T) {
	ts := testutil.New(t)
	project := createProject(t, ts, "Agent 研究蓝图", "undergraduate_thesis")
	projectID := mustParseUUID(project["id"].(string))
	service := writingstate.NewService(ts.DB)
	initial, err := service.GetForAgent(context.Background(), projectID, ts.UserID().UUID)
	require.NoError(t, err)

	brief := initial.Brief
	brief.Topic = "校园阅读习惯研究"
	brief.Questions = []string{"哪些因素影响学生的阅读习惯？"}
	requirement := writingstate.Requirement{ID: "req-1", Text: "需要分析真实案例", Source: "导师意见", Note: "至少三个案例"}
	saved, err := service.SavePreparationFromAgent(context.Background(), projectID, ts.UserID().UUID, initial.Revision, brief, []writingstate.Requirement{requirement})
	require.NoError(t, err)
	assert.Equal(t, 1, saved.Revision)
	assert.Equal(t, "校园阅读习惯研究", saved.Brief.Topic)
	assert.Equal(t, []writingstate.Requirement{requirement}, saved.Requirements)
	assert.Equal(t, "in_progress", saved.Phases["prepare"])

	_, err = service.SavePreparationFromAgent(context.Background(), projectID, ts.UserID().UUID, initial.Revision, brief, []writingstate.Requirement{requirement})
	require.ErrorContains(t, err, "changed")

	latest, err := service.GetForAgent(context.Background(), projectID, ts.UserID().UUID)
	require.NoError(t, err)
	assert.Equal(t, saved.Revision, latest.Revision)
	assert.Equal(t, "校园阅读习惯研究", latest.Brief.Topic)

	_, err = service.GetForAgent(context.Background(), projectID, uuid.New())
	require.Error(t, err, "agent state tools must enforce project ownership")
}

func mustParseUUID(value string) (id uuid.UUID) {
	id = uuid.MustParse(value)
	return id
}
