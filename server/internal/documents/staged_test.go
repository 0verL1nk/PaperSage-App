package documents_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 暂存上传契约（用户需求）：自动上传 → staged 行（不进默认列表）→
// 显式 confirm 入库；同名同大小文件标记 duplicate。

func TestStagedUploadAndConfirm(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	// staged=true 上传 → status=staged、duplicate=false
	resp := uploadPart(t, ts, map[string]string{
		"projectId": p.ID.String(), "staged": "true",
	}, "draft.pdf", "application/pdf", []byte("draft v1"))
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	doc := m["data"].(map[string]any)
	require.Equal(t, "staged", doc["status"], "staged 上传不直接入库")
	assert.Equal(t, false, doc["duplicate"])
	docID := doc["id"].(string)

	// 默认列表不含 staged（未入库）
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/documents", nil)
	require.Equal(t, 200, resp.StatusCode)
	var lm map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&lm))
	items := lm["data"].([]any)
	assert.Empty(t, items, "默认列表应排除 staged 文档")

	// includeStaged=true 可见（预览面板等场景）
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/documents?includeStaged=true", nil)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&lm))
	items = lm["data"].([]any)
	assert.Len(t, items, 1)

	// confirm 入库 → status=uploaded，出现在默认列表
	resp = ts.Do(t, http.MethodPost, "/api/v1/documents/"+docID+"/confirm", nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "uploaded", m["data"].(map[string]any)["status"])

	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/documents", nil)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&lm))
	items = lm["data"].([]any)
	assert.Len(t, items, 1)

	// 重复 confirm → 409（幂等保护）
	resp = ts.Do(t, http.MethodPost, "/api/v1/documents/"+docID+"/confirm", nil)
	assert.Equal(t, 409, resp.StatusCode)
}

func TestUploadDuplicateFlag(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)
	body := []byte("same bytes")

	// 已入库的同名同大小文件
	resp := uploadPart(t, ts, map[string]string{"projectId": p.ID.String()},
		"paper.pdf", "application/pdf", body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, false, m["data"].(map[string]any)["duplicate"])

	// 再次上传相同文件 → duplicate=true（同名 + 同大小）
	resp = uploadPart(t, ts, map[string]string{"projectId": p.ID.String()},
		"paper.pdf", "application/pdf", body)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, true, m["data"].(map[string]any)["duplicate"], "同名同大小应标记重复")

	// 同名不同大小 → 不算重复
	resp = uploadPart(t, ts, map[string]string{"projectId": p.ID.String()},
		"paper.pdf", "application/pdf", append(append([]byte{}, body...), 'x'))
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, false, m["data"].(map[string]any)["duplicate"])
}

func TestDiscardStagedDeletesRow(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	resp := uploadPart(t, ts, map[string]string{
		"projectId": p.ID.String(), "staged": "true",
	}, "tmp.pdf", "application/pdf", []byte("x"))
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	docID := m["data"].(map[string]any)["id"].(string)

	// 丢弃 = 既有 DELETE 端点（staged 行同样可删）
	resp = ts.Do(t, http.MethodDelete, "/api/v1/documents/"+docID, nil)
	require.Equal(t, 200, resp.StatusCode)

	var n int64
	require.NoError(t, ts.DB.Model(&store.Document{}).Where("id = ?", docID).Count(&n).Error)
	assert.Zero(t, n, "丢弃后行应删除")
}
