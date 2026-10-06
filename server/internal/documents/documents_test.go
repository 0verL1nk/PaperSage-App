package documents_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// 契约来源：backend/api/routes/folders.py + folder_service.py + document_service.py

func seedProject(t *testing.T, ts *testutil.TestServer) store.Project {
	t.Helper()
	p := store.Project{Name: "P", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	return p
}

// camelCase 请求（projectId/parentId 别名）与 camelCase 响应
func TestFolderCreateCamelCase(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	body, _ := json.Marshal(map[string]any{"name": "参考资料", "projectId": p.ID.String()})
	resp := ts.Do(t, http.MethodPost, "/api/v1/folders/", body)
	require.Equal(t, 200, resp.StatusCode)

	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	data := m["data"].(map[string]any)
	assert.Equal(t, p.ID.String(), data["projectId"], "响应字段应为 camelCase")
	assert.Contains(t, data, "createdAt")
	assert.Equal(t, nil, data["parentId"])
}

func TestFolderCreateSnakeCase(t *testing.T) {
	ts := testutil.New(t)
	p := store.Project{Name: "S", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)

	// 存量前端（folders-api.ts toSnakeFolder）发送 snake_case
	body, _ := json.Marshal(map[string]any{
		"name": "snake夹", "project_id": p.ID.String(), "parent_id": nil,
	})
	resp := ts.Do(t, http.MethodPost, "/api/v1/folders/", body)
	require.Equal(t, 200, resp.StatusCode, "snake_case 键也应被接受（populate_by_name 对等行为）")
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	assert.NotEmpty(t, m["data"].(map[string]any)["id"])
}

func TestDocumentMoveSnakeCase(t *testing.T) {
	ts := testutil.New(t)
	p := store.Project{Name: "M", OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&p).Error)
	f := store.Folder{Name: "目标夹", ProjectID: p.ID, OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&f).Error)
	d := store.Document{
		Name: "d.pdf", Type: "application/pdf", Size: 3, Status: "uploaded",
		S3Key: "k", ProjectID: p.ID, OwnerID: ts.UserID(),
	}
	require.NoError(t, ts.DB.Create(&d).Error)

	// 存量前端（files-api.ts move）发送 {folder_id}
	body, _ := json.Marshal(map[string]any{"folder_id": f.ID.String()})
	resp := ts.Do(t, http.MethodPut, "/api/v1/documents/"+d.ID.String()+"/move", body)
	require.Equal(t, 200, resp.StatusCode)
	var doc store.Document
	require.NoError(t, ts.DB.Where("id = ?", d.ID).First(&doc).Error)
	require.NotNil(t, doc.FolderID)
	assert.Equal(t, f.ID.UUID, doc.FolderID.UUID, "文档应移动到目标文件夹")
}

func TestFolderCreateParentNotFound(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	body, _ := json.Marshal(map[string]any{
		"name": "f", "projectId": p.ID.String(),
		"parentId": "00000000-0000-0000-0000-000000000000",
	})
	resp := ts.Do(t, http.MethodPost, "/api/v1/folders/", body)
	assert.Equal(t, 400, resp.StatusCode)
}

// 删除文件夹：挂靠文档显式置空 folder_id 归项目根。
//
// 注意：软删除是 UPDATE，DB 的 ON DELETE SET NULL 外键只在物理 DELETE 时触发，
// 因此 deleteFolder 必须自己把子行解绑——不能依赖外键。
func TestFolderDeleteMovesDocsToRoot(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	body, _ := json.Marshal(map[string]any{"name": "F1", "projectId": p.ID.String()})
	resp := ts.Do(t, http.MethodPost, "/api/v1/folders/", body)
	require.Equal(t, 200, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	folderID := m["data"].(map[string]any)["id"].(string)
	// 列以 store.UUID（32 位十六进制）落库，绑定参数须同形，否则 WHERE 不匹配
	fid, err := uuid.Parse(folderID)
	require.NoError(t, err)

	// 文档直接落库（upload 端点属后续任务）
	require.NoError(t, ts.DB.Exec(
		"INSERT INTO documents (id, name, type, size, status, s3_key, project_id, folder_id, owner_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1", "doc.pdf", "application/pdf", 10, "uploaded", "k",
		p.ID, store.UUID{UUID: fid}, ts.UserID(), "2026-01-01 00:00:00", "2026-01-01 00:00:00",
	).Error)

	resp = ts.Do(t, http.MethodDelete, "/api/v1/folders/"+folderID, nil)
	require.Equal(t, 200, resp.StatusCode)

	var folderIDNull sql.NullString
	require.NoError(t, ts.DB.Raw("SELECT folder_id FROM documents LIMIT 1").Scan(&folderIDNull).Error)
	assert.False(t, folderIDNull.Valid, "删除文件夹后文档应落到项目根（folder_id = NULL）")

	// 再删 → 404
	resp = ts.Do(t, http.MethodDelete, "/api/v1/folders/"+folderID, nil)
	assert.Equal(t, 404, resp.StatusCode)
}

// 回归：删除文件夹后，其文档未被删除，且能从项目文档列表取到（folderId 为 null）。
// 仅断言“文件夹已删除”抓不到本 bug——文档会悬空指向已软删的文件夹、从 UI 消失。
func TestFolderDeleteDetachesDocuments(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)

	f := store.Folder{Name: "F", ProjectID: p.ID, OwnerID: ts.UserID()}
	require.NoError(t, ts.DB.Create(&f).Error)
	d := store.Document{
		Name: "d.pdf", Type: "application/pdf", Size: 1, Status: "uploaded",
		S3Key: "k", ProjectID: p.ID, OwnerID: ts.UserID(), FolderID: &f.ID,
	}
	require.NoError(t, ts.DB.Create(&d).Error)

	// 子文件夹：删除必须覆盖整棵子树，否则子文件夹会留着指向已删父级
	child := store.Folder{Name: "F/child", ProjectID: p.ID, OwnerID: ts.UserID(), ParentID: &f.ID}
	require.NoError(t, ts.DB.Create(&child).Error)
	d2 := store.Document{
		Name: "d2.pdf", Type: "application/pdf", Size: 1, Status: "uploaded",
		S3Key: "k2", ProjectID: p.ID, OwnerID: ts.UserID(), FolderID: &child.ID,
	}
	require.NoError(t, ts.DB.Create(&d2).Error)

	resp := ts.Do(t, http.MethodDelete, "/api/v1/folders/"+f.ID.String(), nil)
	require.Equal(t, 200, resp.StatusCode)

	// 两层的文档本身都仍在，只是从各自的文件夹解绑
	var got, got2 store.Document
	require.NoError(t, ts.DB.Where("id = ?", d.ID).First(&got).Error)
	require.Nil(t, got.FolderID, "文档应从文件夹解绑（folder_id = NULL）")
	require.NoError(t, ts.DB.Where("id = ?", d2.ID).First(&got2).Error)
	require.Nil(t, got2.FolderID, "子文件夹中的文档同样应被解绑")

	// 子文件夹本身也要一并删除，否则会留下悬挂的 parent_id
	var folders int64
	require.NoError(t, ts.DB.Model(&store.Folder{}).
		Where("project_id = ?", p.ID).Count(&folders).Error)
	assert.Zero(t, folders, "整棵子树的文件夹都应被删除")

	// 可从项目文档列表取到，folderId 为 null（项目根）
	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/documents", nil)
	require.Equal(t, 200, resp.StatusCode)
	var m struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	require.Len(t, m.Data, 2)
	for _, doc := range m.Data {
		assert.Equal(t, nil, doc["folderId"])
	}
}

func TestListProjectFoldersSortedByName(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)
	for _, n := range []string{"b", "a", "c"} {
		f := store.Folder{Name: n, ProjectID: p.ID, OwnerID: ts.UserID()}
		require.NoError(t, ts.DB.Create(&f).Error)
	}

	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/folders", nil)
	require.Equal(t, 200, resp.StatusCode)
	var m struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	require.Len(t, m.Data, 3)
	assert.Equal(t, "a", m.Data[0]["name"])
	assert.Equal(t, "b", m.Data[1]["name"])
}

func TestListProjectDocuments(t *testing.T) {
	ts := testutil.New(t)
	p := seedProject(t, ts)
	for _, n := range []string{"d1", "d2"} {
		require.NoError(t, ts.DB.Exec(
			"INSERT INTO documents (id, name, type, size, status, s3_key, project_id, owner_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
			n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n+n, n, "application/pdf", 1, "uploaded", "k-"+n,
			p.ID, ts.UserID(), "2026-01-01 00:00:0"+n[1:], "2026-01-01 00:00:00",
		).Error)
	}

	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/documents", nil)
	require.Equal(t, 200, resp.StatusCode)
	var m struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	require.Len(t, m.Data, 2)
	// camelCase 输出
	assert.Contains(t, m.Data[0], "projectId")
	assert.Contains(t, m.Data[0], "s3Key")
	assert.Contains(t, m.Data[0], "createdAt")
}

// 非本人项目：documents 列表 → 404（坏设计修正：Python 未捕获 ValueError 返回 500）
func TestListDocumentsForeignProject(t *testing.T) {
	ts := testutil.New(t)
	other := store.User{Username: "other2", PasswordHash: "x"}
	require.NoError(t, ts.DB.Create(&other).Error)
	p := store.Project{Name: "foreign", OwnerID: other.ID}
	require.NoError(t, ts.DB.Create(&p).Error)

	resp := ts.Do(t, http.MethodGet, "/api/v1/projects/"+p.ID.String()+"/documents", nil)
	assert.Equal(t, 404, resp.StatusCode)
}
