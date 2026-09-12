package documents_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 补测 upload 分支与 folder 文档列表（拉覆盖）

func uploadPart(t *testing.T, ts *testutil.TestServer, fields map[string]string, filename, ctype string, content []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if filename != "" {
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		if ctype != "" {
			hdr.Set("Content-Type", ctype)
		}
		fw, _ := mw.CreatePart(hdr)
		_, _ = fw.Write(content)
	}
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/documents/upload", &buf)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestUploadValidationBranches(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	// 缺 projectId → 422
	resp := uploadPart(t, ts, nil, "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, 422, resp.StatusCode)

	// projectId 非法 → 422
	resp = uploadPart(t, ts, map[string]string{"projectId": "zzz"}, "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, 422, resp.StatusCode)

	// 缺 file → 422
	resp = uploadPart(t, ts, map[string]string{"projectId": p.ID.String()}, "", "", nil)
	assert.Equal(t, 422, resp.StatusCode)

	// folderId 非法 → 422
	resp = uploadPart(t, ts, map[string]string{"projectId": p.ID.String(), "folderId": "zzz"}, "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, 422, resp.StatusCode)

	// 非本人项目 → 400 Project not found
	resp = uploadPart(t, ts, map[string]string{"projectId": "00000000-0000-0000-0000-000000000000"}, "a.txt", "text/plain", []byte("x"))
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, float64(400), m["code"])
	assert.Equal(t, "Project not found", m["info"])

	// folder 不存在 → 400
	resp = uploadPart(t, ts, map[string]string{"projectId": p.ID.String(), "folderId": "00000000-0000-0000-0000-000000000000"}, "a.txt", "text/plain", []byte("x"))
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, float64(400), m["code"])
	assert.Equal(t, "Folder not found", m["info"])

	// 带 remark 与 folder 正常上传
	f := store.Folder{Name: "F", ProjectID: p.ID, OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&f).Error)
	resp = uploadPart(t, ts, map[string]string{
		"projectId": p.ID.String(), "folderId": f.ID.String(), "remark": "备注",
	}, "带备注.txt", "text/plain", []byte("hello"))
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	doc := m["data"].(map[string]any)
	assert.Equal(t, "备注", doc["remark"])
	assert.Equal(t, f.ID.String(), doc["folderId"])
}

func TestListFolderDocumentsEndpoint(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)
	f := store.Folder{Name: "F", ProjectID: p.ID, OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&f).Error)

	resp := ts.Do(t, http.MethodGet, "/api/v1/folders/"+f.ID.String()+"/documents", nil)
	require.Equal(t, 200, resp.StatusCode)
	var m struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Empty(t, m.Data)

	// 非法 UUID → 422
	resp = ts.Do(t, http.MethodGet, "/api/v1/folders/zzz/documents", nil)
	assert.Equal(t, 422, resp.StatusCode)
}

func TestPreviewNotFound(t *testing.T) {
	ts := testutil.New(t)
	resp := ts.Do(t, http.MethodGet, "/api/v1/documents/00000000-0000-0000-0000-000000000000/preview", nil)
	assert.Equal(t, 404, resp.StatusCode)

	resp = ts.Do(t, http.MethodGet, "/api/v1/documents/zzz/preview", nil)
	assert.Equal(t, 422, resp.StatusCode)

	// 非 JSON body move → 422
	resp = ts.Do(t, http.MethodPut, "/api/v1/documents/00000000-0000-0000-0000-000000000000/move", []byte(`bad`))
	assert.Equal(t, 422, resp.StatusCode)
	resp = ts.Do(t, http.MethodPut, "/api/v1/documents/00000000-0000-0000-0000-000000000000/remark", []byte(`bad`))
	assert.Equal(t, 422, resp.StatusCode)
}
