package documents_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 契约来源：backend/api/routes/documents.py + test_document_routes.py 语义

func TestUploadAndPreviewFlow(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	// text/plain 上传（gin multipart part 的 CT 由浏览器给；此处经 part 头）
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("projectId", p.ID.String())
	// CreateFormFile 固定 octet-stream，需 CreatePart 指定 text/plain（浏览器行为）
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="notes.txt"`)
	hdr.Set("Content-Type", "text/plain")
	fw, _ := mw.CreatePart(hdr)
	_, _ = fw.Write([]byte("hello papersage"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/documents/upload", &buf)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, 200, resp.StatusCode, "text/plain 应可上传")

	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	doc := m["data"].(map[string]any)
	docID := doc["id"].(string)
	assert.Equal(t, "notes.txt", doc["name"])
	assert.Equal(t, float64(15), doc["size"])
	assert.Equal(t, "uploaded", doc["status"])
	assert.Regexp(t, `^projects/[^/]+/documents/[^/]+/notes\.txt$`, doc["s3Key"])

	// preview：裸响应 + inline Content-Disposition
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/documents/"+docID+"/preview", nil)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello papersage", string(body))
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "inline")
}

func TestUploadRejectsBadType(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("projectId", p.ID.String())
	fw, _ := mw.CreateFormFile("file", "evil.exe")
	_, _ = fw.Write([]byte("MZ"))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/documents/upload", &buf)
	req.Header.Set("Authorization", "Bearer "+ts.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	// gin multipart 的 part CT 缺省 application/octet-stream → 白名单拒绝
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, float64(400), m["code"])
	assert.Contains(t, m["info"], "Unsupported file type")
}

func TestDocumentImportUploadIsIdempotentAndRejectsContentConflict(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)
	key := map[string]string{
		"X-PaperSage-Import-Bundle-Id": uuid.NewString(),
		"X-PaperSage-Import-Local-Id":  uuid.NewString(),
	}
	upload := func(content string, headers map[string]string) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("projectId", p.ID.String())
		_ = mw.WriteField("staged", "true")
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="import.txt"`)
		hdr.Set("Content-Type", "text/plain")
		part, err := mw.CreatePart(hdr)
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
		require.NoError(t, mw.Close())
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/documents/upload", &buf)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+ts.Token)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		resp, err := http.DefaultClient.Do(req) //nolint:gosec,noctx // test
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	first := upload("local draft", key)
	require.Equal(t, http.StatusOK, first.StatusCode)
	var firstBody map[string]any
	require.NoError(t, json.NewDecoder(first.Body).Decode(&firstBody))
	firstID := firstBody["data"].(map[string]any)["id"]

	retry := upload("local draft", key)
	require.Equal(t, http.StatusOK, retry.StatusCode)
	var retryBody map[string]any
	require.NoError(t, json.NewDecoder(retry.Body).Decode(&retryBody))
	assert.Equal(t, firstID, retryBody["data"].(map[string]any)["id"])

	conflict := upload("different bytes", key)
	assert.Equal(t, http.StatusConflict, conflict.StatusCode)
	partial := upload("partial", map[string]string{"X-PaperSage-Import-Bundle-Id": uuid.NewString()})
	assert.Equal(t, http.StatusUnprocessableEntity, partial.StatusCode)
	var count int64
	require.NoError(t, ts.DB.Model(&store.Document{}).Where("owner_id = ?", ts.UserID()).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestMoveRemarkDeleteFlow(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	// 直接落库一条文档（upload 已另行覆盖）
	doc := store.Document{Name: "d.pdf", Type: "application/pdf", Size: 3, Status: "uploaded",
		S3Key: "k", ProjectID: p.ID, OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&doc).Error)
	id := doc.ID.String()

	// remark（空串 → null）
	body, _ := json.Marshal(map[string]any{"remark": "重要文献"})
	resp := ts.Do(t, http.MethodPut, "/api/v1/documents/"+id+"/remark", body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.Equal(t, "重要文献", m["data"].(map[string]any)["remark"])

	// move 到 folder
	f := store.Folder{Name: "F", ProjectID: p.ID, OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&f).Error)
	body, _ = json.Marshal(map[string]any{"folderId": f.ID.String()})
	resp = ts.Do(t, http.MethodPut, "/api/v1/documents/"+id+"/move", body)
	require.Equal(t, 200, resp.StatusCode)

	// move 到不存在 folder → 404 Target folder not found
	body, _ = json.Marshal(map[string]any{"folderId": "00000000-0000-0000-0000-000000000000"})
	resp = ts.Do(t, http.MethodPut, "/api/v1/documents/"+id+"/move", body)
	assert.Equal(t, 404, resp.StatusCode)

	// delete
	resp = ts.Do(t, http.MethodDelete, "/api/v1/documents/"+id, nil)
	require.Equal(t, 200, resp.StatusCode)
	resp = ts.Do(t, http.MethodDelete, "/api/v1/documents/"+id, nil)
	assert.Equal(t, 404, resp.StatusCode)
}
