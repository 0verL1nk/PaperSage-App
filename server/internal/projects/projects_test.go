package projects_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 契约来源：backend/api/routes/projects.py + tests/test_projects_routes.py 语义

func decodeData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	require.Equal(t, float64(200), m["code"], "body: %v", m)
	return m["data"].(map[string]any)
}

func createProject(t *testing.T, ts *testutil.TestServer, name string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name})
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, 200, resp.StatusCode, "创建项目应成功")
	return decodeData(t, resp)
}

func TestProjectCRUDAndTrashFlow(t *testing.T) {
	ts := testutil.New(t)

	// 创建
	p := createProject(t, ts, "毕设论文")
	require.NotEmpty(t, p["id"])
	assert.Equal(t, "active", p["status"])
	assert.Equal(t, false, p["is_deleted"])
	assert.Equal(t, nil, p["group"], "无分组时 group 为 null")
	assert.Contains(t, p, "created_at")

	// 列表
	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/", nil)
	require.Equal(t, 200, resp.StatusCode)
	var list []any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&struct {
		Data *[]any `json:"data"`
	}{Data: &list}))
	assert.Len(t, list, 1)

	// 重名 → 400
	body, _ := json.Marshal(map[string]any{"name": "毕设论文"})
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	assert.Equal(t, 400, resp.StatusCode)

	// 更新
	body, _ = json.Marshal(map[string]any{"description": "第一版"})
	pid := p["id"].(string)
	resp = ts.Do(t, http.MethodPut, "/api/v1/projects/"+pid, body)
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "第一版", decodeData(t, resp)["description"])

	// 软删 → 列表隐藏
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/"+pid, nil)
	require.Equal(t, 200, resp.StatusCode)

	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/", nil)
	var empty []any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&struct {
		Data *[]any `json:"data"`
	}{Data: &empty}))
	assert.Len(t, empty, 0, "软删后默认列表不显示")

	// include_deleted=true 可见
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/?include_deleted=true", nil)
	var all []any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&struct {
		Data *[]any `json:"data"`
	}{Data: &all}))
	assert.Len(t, all, 1)

	// trash 列表
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/trash", nil)
	require.Equal(t, 200, resp.StatusCode)
	var trash []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&struct {
		Data *[]map[string]any `json:"data"`
	}{Data: &trash}))
	require.Len(t, trash, 1)
	assert.Equal(t, "project", trash[0]["type"])
	assert.Equal(t, "毕设论文", trash[0]["name"])
	od := trash[0]["original_data"].(map[string]any)
	assert.Equal(t, "active", od["status"])

	// 恢复
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/trash/project/"+pid+"/restore", nil)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/", nil)
	var restored []any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&struct {
		Data *[]any `json:"data"`
	}{Data: &restored}))
	assert.Len(t, restored, 1)

	// 彻底删除
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/"+pid, nil)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/trash/project/"+pid, nil)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/trash", nil)
	var trash2 []any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&struct {
		Data *[]any `json:"data"`
	}{Data: &trash2}))
	assert.Len(t, trash2, 0)
}

func TestGroupsFlow(t *testing.T) {
	ts := testutil.New(t)

	// 两个组：sort_order 0,1
	body, _ := json.Marshal(map[string]any{"name": "课程作业"})
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/groups", body)
	require.Equal(t, 200, resp.StatusCode)
	g1 := decodeData(t, resp)
	assert.Equal(t, float64(0), g1["sort_order"])

	body, _ = json.Marshal(map[string]any{"name": "科研"})
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/groups", body)
	require.Equal(t, 200, resp.StatusCode)
	g2 := decodeData(t, resp)
	assert.Equal(t, float64(1), g2["sort_order"])

	// 项目挂组
	g2ID := g2["id"].(string)
	body, _ = json.Marshal(map[string]any{"name": "带组项目", "group_id": g2ID})
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, 200, resp.StatusCode)
	pd := decodeData(t, resp)
	grp := pd["group"].(map[string]any)
	assert.Equal(t, "科研", grp["name"])

	// 未知组 → 400
	body, _ = json.Marshal(map[string]any{"name": "x", "group_id": "00000000-0000-0000-0000-000000000000"})
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	assert.Equal(t, 400, resp.StatusCode)

	// 删组 → 项目 group_id 置空
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/groups/"+g2ID, nil)
	require.Equal(t, 200, resp.StatusCode)

	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+pd["id"].(string), nil)
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, nil, decodeData(t, resp)["group_id"], "删组后项目应回到未分组")

	// 删除不存在的组 → 404
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/groups/"+g2ID, nil)
	assert.Equal(t, 404, resp.StatusCode)
}

// 他人项目不可见（owner 隔离 → 404）
func TestOwnerIsolation(t *testing.T) {
	ts := testutil.New(t)
	other := store.User{Username: "other", PasswordHash: "x"}
	require.NoError(t, ts.DB.Create(&other).Error)
	p := store.Project{Name: "别人的", OwnerID: other.ID}
	require.NoError(t, ts.DB.Create(&p).Error)

	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String(), nil)
	assert.Equal(t, 404, resp.StatusCode)
}
