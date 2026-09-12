package projects_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 补测组列表/更新与项目更新分支（拉覆盖）
func TestGroupListAndUpdate(t *testing.T) {
	ts := testutil.New(t)

	body, _ := json.Marshal(map[string]any{"name": "G1"})
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/groups", body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	gid := m["data"].(map[string]any)["id"].(string)

	body, _ = json.Marshal(map[string]any{"name": "G2"})
	_ = ts.Do(t, http.MethodPost, "/api/v1/projects/groups", body)

	// 列表（sort_order 排序）
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/groups", nil)
	require.Equal(t, 200, resp.StatusCode)
	var lst struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&lst))
	require.Len(t, lst.Data, 2)
	assert.Equal(t, "G1", lst.Data[0]["name"])

	// 更新：改名 + sort_order
	body, _ = json.Marshal(map[string]any{"name": "G1改", "sort_order": 9})
	resp = ts.Do(t, http.MethodPut, "/api/v1/projects/groups/"+gid, body)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "G1改", m["data"].(map[string]any)["name"])
	assert.Equal(t, float64(9), m["data"].(map[string]any)["sort_order"])

	// 更新不存在的组 → 404
	body, _ = json.Marshal(map[string]any{"name": "x"})
	resp = ts.Do(t, http.MethodPut, "/api/v1/projects/groups/00000000-0000-0000-0000-000000000000", body)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestProjectUpdateBranches(t *testing.T) {
	ts := testutil.New(t)
	p := createProject(t, ts, "原名")

	// 空名单独校验（422）
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", []byte(`{"name": ""}`))
	assert.Equal(t, 422, resp.StatusCode)

	// 非 JSON body → 422
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/", []byte(`not-json`))
	assert.Equal(t, 422, resp.StatusCode)

	// 改名成功
	body, _ := json.Marshal(map[string]any{"name": "新名", "status": "archived"})
	pid := p["id"].(string)
	resp = ts.Do(t, http.MethodPut, "/api/v1/projects/"+pid, body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "archived", m["data"].(map[string]any)["status"])

	// 改名撞已有名 → 400
	createProject(t, ts, "已存在")
	body, _ = json.Marshal(map[string]any{"name": "已存在"})
	resp = ts.Do(t, http.MethodPut, "/api/v1/projects/"+pid, body)
	assert.Equal(t, 400, resp.StatusCode)

	// 挂非法组 → 400
	body, _ = json.Marshal(map[string]any{"group_id": "00000000-0000-0000-0000-000000000000"})
	resp = ts.Do(t, http.MethodPut, "/api/v1/projects/"+pid, body)
	assert.Equal(t, 400, resp.StatusCode)

	// 非法 UUID 参数 → 422
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/not-a-uuid", nil)
	assert.Equal(t, 422, resp.StatusCode)

	// group_id 查询参数非法 → 422
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/?group_id=zzz", nil)
	assert.Equal(t, 422, resp.StatusCode)

	// 软删后再软删 → 404（require active）
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/"+pid, nil)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/"+pid, nil)
	assert.Equal(t, 404, resp.StatusCode)

	// restore 不存在的 → 404
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/trash/project/00000000-0000-0000-0000-000000000000/restore", nil)
	assert.Equal(t, 404, resp.StatusCode)
	resp = ts.Do(t, http.MethodDelete, "/api/v1/projects/trash/project/00000000-0000-0000-0000-000000000000", nil)
	assert.Equal(t, 404, resp.StatusCode)
}
