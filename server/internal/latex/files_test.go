package latex_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 契约来源：backend/api/routes/latex_files.py + file_service.py

func seed(t *testing.T) (*testutil.TestServer, store.Project) {
	t.Helper()
	ts := testutil.New(t)
	p := store.Project{Name: "LP", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	return ts, p
}

func latexPath(p store.Project, suffix string) string {
	return "/api/v1/projects/" + p.ID.String() + "/latex-files" + suffix
}

func TestProjectCreateSeedsMainTex(t *testing.T) {
	ts := testutil.New(t)
	// 经 projects 域创建项目 → 模板钩子应生成 main.tex
	body, _ := json.Marshal(map[string]any{"name": "模板项目"})
	resp := ts.Do(t, http.MethodPost, "/api/v1/projects/", body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	pid := m["data"].(map[string]any)["id"].(string)

	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+pid+"/latex-files/", nil)
	require.Equal(t, 200, resp.StatusCode)
	var lm struct {
		Data struct {
			Files []map[string]any `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&lm))
	require.Len(t, lm.Data.Files, 1)
	assert.Equal(t, "main.tex", lm.Data.Files[0]["file_path"])
	assert.NotZero(t, lm.Data.Files[0]["size_bytes"])
}

func TestFileCRUDFlow(t *testing.T) {
	ts, p := seed(t)

	// 创建（JSON）
	body, _ := json.Marshal(map[string]any{"path": "chapters/intro.tex", "content": "\\section{Intro}"})
	resp := ts.Do(t, http.MethodPost, latexPath(p, "/"), body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	f := m["data"].(map[string]any)
	assert.Equal(t, "chapters/intro.tex", f["file_path"])
	assert.Equal(t, "text/plain", f["content_type"])
	assert.Contains(t, f, "last_saved_at")

	// 重复创建 → 400
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/"), body)
	assert.Equal(t, 400, resp.StatusCode)

	// 读取文本 → JSON 信封 {path, content}
	resp = ts.Do(t, http.MethodGet, latexPath(p, "/chapters/intro.tex"), nil)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	d := m["data"].(map[string]any)
	assert.Equal(t, "chapters/intro.tex", d["path"])
	assert.Equal(t, "\\section{Intro}", d["content"])

	// 更新（内容不变 → 跳过；变化 → 新 hash）
	body, _ = json.Marshal(map[string]any{"content": "\\section{Intro v2}"})
	resp = ts.Do(t, http.MethodPut, latexPath(p, "/chapters/intro.tex"), body)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodGet, latexPath(p, "/chapters/intro.tex"), nil)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "\\section{Intro v2}", m["data"].(map[string]any)["content"])

	// 路径校验：..、blocked 扩展
	body, _ = json.Marshal(map[string]any{"path": "../evil.tex", "content": "x"})
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/"), body)
	assert.Equal(t, 400, resp.StatusCode)

	body, _ = json.Marshal(map[string]any{"path": "virus.exe", "content": "x"})
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/"), body)
	assert.Equal(t, 400, resp.StatusCode)

	// rename
	body, _ = json.Marshal(map[string]any{"new_path": "chapters/abstract.tex"})
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/chapters/intro.tex/rename"), body)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "chapters/abstract.tex", m["data"].(map[string]any)["file_path"])

	// move
	body, _ = json.Marshal(map[string]any{"new_dir": "appendix"})
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/chapters/abstract.tex/move"), body)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "appendix/abstract.tex", m["data"].(map[string]any)["file_path"])

	// delete
	resp = ts.Do(t, http.MethodDelete, latexPath(p, "/appendix/abstract.tex"), nil)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodGet, latexPath(p, "/appendix/abstract.tex"), nil)
	assert.Equal(t, 404, resp.StatusCode)
}

// 二进制文件：GET 返回裸字节 + inline 头（非 JSON 信封）
func TestBinaryFileRawResponse(t *testing.T) {
	ts, p := seed(t)

	body, _ := json.Marshal(map[string]any{"path": "figs/a.png", "content": "PNGDATA"})
	resp := ts.Do(t, http.MethodPost, latexPath(p, "/"), body)
	require.Equal(t, 200, resp.StatusCode)

	resp = ts.Do(t, http.MethodGet, latexPath(p, "/figs/a.png"), nil)
	require.Equal(t, 200, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "PNGDATA", string(b), "二进制文件应返回裸字节")
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "inline")

	// raw=true 强制裸响应（文本也是）
	resp = ts.Do(t, http.MethodGet, latexPath(p, "/figs/a.png?raw=true"), nil)
	require.Equal(t, 200, resp.StatusCode)
}

func TestFolderRenameAndDelete(t *testing.T) {
	ts, p := seed(t)
	var resp *http.Response
	for _, fp := range []string{"sec/a.tex", "sec/b.tex", "other/c.tex"} {
		body, _ := json.Marshal(map[string]any{"path": fp, "content": "x"})
		resp = ts.Do(t, http.MethodPost, latexPath(p, "/"), body)
		require.Equal(t, 200, resp.StatusCode, fp)
	}

	// rename folder
	body, _ := json.Marshal(map[string]any{"folder_path": "sec", "new_folder": "sections"})
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/folder/rename"), body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	d := m["data"].(map[string]any)
	assert.Equal(t, float64(2), d["renamed_count"])
	assert.Equal(t, "sec", d["old_folder"])

	// rename 到已有目录 → 400
	body, _ = json.Marshal(map[string]any{"folder_path": "sections", "new_folder": "other"})
	resp = ts.Do(t, http.MethodPost, latexPath(p, "/folder/rename"), body)
	assert.Equal(t, 400, resp.StatusCode)

	// delete folder（JSON body）
	body, _ = json.Marshal(map[string]any{"folder_path": "sections/"})
	resp = ts.Do(t, http.MethodDelete, latexPath(p, "/folder"), body)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, float64(2), m["data"].(map[string]any)["deleted_count"])

	resp = ts.Do(t, http.MethodGet, latexPath(p, "/"), nil)
	var lm struct {
		Data struct {
			Files []map[string]any `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&lm))
	assert.Len(t, lm.Data.Files, 1, "仅剩 other/c.tex")
}

func TestForeignProjectDenied(t *testing.T) {
	ts := testutil.New(t)
	other := store.User{Username: "latexother", PasswordHash: "x"}
	require.NoError(t, ts.DB.Create(&other).Error)
	p := store.Project{Name: "F", OwnerID: other.ID}
	require.NoError(t, ts.DB.Create(&p).Error)

	resp := ts.Do(t, http.MethodGet, latexPath(p, "/"), nil)
	assert.Equal(t, 404, resp.StatusCode)
}
