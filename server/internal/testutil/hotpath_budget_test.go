package testutil_test

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 查询预算回归：远程库上每条 SQL 都是一次网络往返，热路径多一条查询就是几十到几百毫秒。
// 预算按「auth + 业务」计，放宽到实现细节变化也不误报，但平行实现回潮会被挡住。

func TestLatexHotPathQueryBudget(t *testing.T) {
	ts := testutil.New(t)
	pid := benchCreateProject(t, ts)
	counter := testutil.CountQueries(ts.DB)
	fileURL := "/api/v1/projects/" + pid.String() + "/latex-files"
	content := benchText("")

	body, err := json.Marshal(map[string]string{"path": benchFilePath, "content": content})
	require.NoError(t, err)
	counter.Reset()
	status, out := benchDo(t, ts, http.MethodPost, fileURL, body)
	benchMustOK(t, status, out, "create file")
	assert.LessOrEqual(t, counter.Value(), int64(4), "create: auth + 归属校验 + 存在检查 + INSERT")

	changed, err := json.Marshal(map[string]string{"content": content + "\n% rev"})
	require.NoError(t, err)
	counter.Reset()
	status, out = benchDo(t, ts, http.MethodPut, fileURL+"/"+benchFilePath, changed)
	benchMustOK(t, status, out, "update file")
	assert.LessOrEqual(t, counter.Value(), int64(3), "update: auth + 归属/文件合并 + UPDATE")

	counter.Reset()
	status, out = benchDo(t, ts, http.MethodGet, fileURL+"/"+benchFilePath, nil)
	benchMustOK(t, status, out, "read file")
	assert.LessOrEqual(t, counter.Value(), int64(2), "read: auth + 归属/文件合并")
}

func TestProposalApplyQueryBudget(t *testing.T) {
	ts := testutil.New(t)
	pid := benchCreateProject(t, ts)
	content := benchText(benchAnchor)
	mainURL := "/api/v1/projects/" + pid.String() + "/latex-files/main.tex"
	body, err := json.Marshal(map[string]string{"content": content})
	require.NoError(t, err)
	status, out := benchDo(t, ts, http.MethodPut, mainURL, body)
	benchMustOK(t, status, out, "seed main.tex")

	sum := md5.Sum([]byte(content))
	p := store.AgentChangeProposal{
		ID: store.MustNewUUID(), ProjectID: store.UUID{UUID: pid},
		FilePath: "main.tex", BaseContentHash: hex.EncodeToString(sum[:]),
		OldText: benchAnchor, NewText: "REPLACED_BY_TEST", Status: "pending",
	}
	require.NoError(t, ts.DB.Create(&p).Error)

	counter := testutil.CountQueries(ts.DB)
	counter.Reset()
	status, out = benchDo(t, ts, http.MethodPost, "/api/v1/agent/change-proposals/"+p.ID.String()+"/apply", nil)
	benchMustOK(t, status, out, "apply proposal")
	assert.LessOrEqual(t, counter.Value(), int64(5),
		"apply: auth + 提案/归属合并 + 稿件 + 文件 UPDATE + 提案 UPDATE")

	status, out = benchDo(t, ts, http.MethodGet, mainURL, nil)
	benchMustOK(t, status, out, "read applied file")
	var env struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out, &env))
	assert.Contains(t, env.Data.Content, "REPLACED_BY_TEST", "提案必须真正写入稿件")
}

// 归属合并查询不得改变原有 404 文案（前端与契约测试依赖这两条区分）。
func TestLatexLookupErrorContract(t *testing.T) {
	ts := testutil.New(t)
	pid := benchCreateProject(t, ts)

	status, out := benchDo(t, ts, http.MethodGet,
		"/api/v1/projects/"+pid.String()+"/latex-files/absent.tex", nil)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, string(out), "File not found")

	status, out = benchDo(t, ts, http.MethodGet,
		"/api/v1/projects/"+uuid.NewString()+"/latex-files/main.tex", nil)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, string(out), "Project not found")
}
