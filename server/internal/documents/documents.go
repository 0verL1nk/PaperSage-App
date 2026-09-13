// Package documents 文档与文件夹域。
// 契约来源：backend/api/routes/folders.py + services/documents/*.py + schemas/document.py, folder.py
package documents

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/store/query"
)

// Service 文档/文件夹域逻辑。
type Service struct {
	DB      *gorm.DB
	Storage storage.Storage
	Indexer *DocIndexer // 可选：入库后的 RAG 索引钩子（装配层注入）
}

// DocIndexer documents 不直接依赖 rag（保持域单向）；由装配层闭包实现。
// IndexDocFn 索引钩子签名（bootstrap 闭包实现）。
type IndexDocFn = func(projectID, docID store.UUID, name, s3Key, contentType string)

type DocIndexer struct {
	IndexDocument IndexDocFn
}

// NewService wires the documents domain（storage 为对象存储接口）。
func NewService(db *gorm.DB, stg storage.Storage) *Service {
	return &Service{DB: db, Storage: stg}
}

// ── 响应 DTO（ser_json_alias → camelCase 输出）──

type documentResponse struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	Type      string       `json:"type"`
	Size      int64        `json:"size"`
	Status    string       `json:"status"`
	Duplicate bool         `json:"duplicate"`
	ProjectID uuid.UUID    `json:"projectId"`
	FolderID  *uuid.UUID   `json:"folderId"`
	OwnerID   uuid.UUID    `json:"ownerId"`
	S3Key     string       `json:"s3Key"`
	Remark    *string      `json:"remark"`
	CreatedAt dto.JSONTime `json:"createdAt"`
	UpdatedAt dto.JSONTime `json:"updatedAt"`
}

type folderResponse struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	ParentID  *uuid.UUID   `json:"parentId"`
	ProjectID uuid.UUID    `json:"projectId"`
	OwnerID   uuid.UUID    `json:"ownerId"`
	CreatedAt dto.JSONTime `json:"createdAt"`
	UpdatedAt dto.JSONTime `json:"updatedAt"`
}

func toDocumentResponse(d *store.Document) documentResponse {
	resp := documentResponse{
		ID: d.ID.UUID, Name: d.Name, Type: d.Type, Size: d.Size, Status: d.Status,
		ProjectID: d.ProjectID.UUID, OwnerID: d.OwnerID.UUID, S3Key: d.S3Key,
		Remark:    d.Remark,
		CreatedAt: dto.JSONTime{Time: d.CreatedAt},
		UpdatedAt: dto.JSONTime{Time: d.UpdatedAt},
	}
	if d.FolderID != nil {
		id := d.FolderID.UUID
		resp.FolderID = &id
	}
	return resp
}

func toFolderResponse(f *store.Folder) folderResponse {
	resp := folderResponse{
		ID: f.ID.UUID, Name: f.Name, ProjectID: f.ProjectID.UUID, OwnerID: f.OwnerID.UUID,
		CreatedAt: dto.JSONTime{Time: f.CreatedAt},
		UpdatedAt: dto.JSONTime{Time: f.UpdatedAt},
	}
	if f.ParentID != nil {
		id := f.ParentID.UUID
		resp.ParentID = &id
	}
	return resp
}

// verifyProjectAccess 校验项目归属（未删除），失败 → ValueError 语义错误。
func (s *Service) verifyProjectAccess(ctx context.Context, projectID, userID uuid.UUID) (*store.Project, error) {
	var p store.Project
	err := s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?",
			store.UUID{UUID: projectID}, store.UUID{UUID: userID}).First(&p).Error
	if err != nil {
		return nil, &httpapi.APIError{Code: 400, Info: "Project not found"}
	}
	return &p, nil
}

// ── Folders ──

func (s *Service) createFolder(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID

	// FastAPI populate_by_name 同时接受 camelCase/snake_case 键名；
	// 存量前端（folders-api.ts toSnakeFolder）发送 snake_case，
	// Go 的 encoding/json 不视 project_id 与 projectId 等价，双字段兜底
	var req struct {
		Name       *string    `json:"name"`
		ProjectID  *uuid.UUID `json:"projectId"`
		ParentID   *uuid.UUID `json:"parentId"`
		ProjectIDS *uuid.UUID `json:"project_id"`
		ParentIDS  *uuid.UUID `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	projectID := req.ProjectID
	if projectID == nil {
		projectID = req.ProjectIDS
	}
	if projectID == nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	parentUUID := req.ParentID
	if parentUUID == nil {
		parentUUID = req.ParentIDS
	}

	if _, err := s.verifyProjectAccess(ctx, *projectID, uid); err != nil {
		httpapi.WriteError(c.Writer, err) // 400 "Project not found"
		return
	}
	var parentID *store.UUID
	if parentUUID != nil {
		var parent store.Folder
		err := s.DB.WithContext(ctx).
			Where("id = ? AND project_id = ?", store.UUID{UUID: *parentUUID}, store.UUID{UUID: *projectID}).
			First(&parent).Error
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Parent folder not found"})
			return
		}
		parentID = &store.UUID{UUID: *parentUUID}
	}

	f := store.Folder{
		Name: *req.Name, ParentID: parentID,
		ProjectID: store.UUID{UUID: *projectID}, OwnerID: store.UUID{UUID: uid},
	}
	if err := s.DB.WithContext(ctx).Create(&f).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toFolderResponse(&f), ""))
}

func (s *Service) deleteFolder(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	fid, err := uuid.Parse(c.Param("folder_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var f store.Folder
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: fid}, store.UUID{UUID: uid}).First(&f).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Folder not found"})
		return
	}
	_ = s.DB.WithContext(ctx).Delete(&f).Error // 文档经 FK SET NULL 归到项目根
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"id": fid.String(), "deleted": true}, ""))
}

func (s *Service) listFolderDocuments(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	fid, err := uuid.Parse(c.Param("folder_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	fdocs, _ := query.Document.WithContext(ctx).
		Where(query.Document.FolderID.Eq(store.UUID{UUID: fid}), query.Document.OwnerID.Eq(store.UUID{UUID: uid})).
		Order(query.Document.CreatedAt.Desc()).Find()
	docs := make([]store.Document, 0, len(fdocs))
	for i := range fdocs {
		docs = append(docs, *fdocs[i])
	}
	if len(docs) > 0 {
		// verify_project_access(docs[0])：非本人项目 → 400
		if _, err := s.verifyProjectAccess(ctx, docs[0].ProjectID.UUID, uid); err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Folder not found"})
			return
		}
	}
	out := make([]documentResponse, 0, len(docs))
	for i := range docs {
		out = append(out, toDocumentResponse(&docs[i]))
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}

// ── Project-scoped 列表（Python 挂在 projects 路由下，此处同路径注册）──

func (s *Service) listProjectDocuments(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if _, err := s.verifyProjectAccess(ctx, pid, uid); err != nil {
		// 改进：Python 未捕获 ValueError 泄漏为 500（坏设计不迁移）→ 404
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found"})
		return
	}
	dq := query.Document.WithContext(ctx).
		Where(query.Document.ProjectID.Eq(store.UUID{UUID: pid}))
	if c.Query("includeStaged") != "true" {
		dq = dq.Where(query.Document.Status.Neq("staged"))
	}
	docs, _ := dq.Order(query.Document.CreatedAt.Desc()).Find()
	// []*store.Document → 值切片（响应装配用值语义）
	docVals := make([]store.Document, 0, len(docs))
	for i := range docs {
		docVals = append(docVals, *docs[i])
	}
	out := make([]documentResponse, 0, len(docVals))
	for i := range docVals {
		out = append(out, toDocumentResponse(&docVals[i]))
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}

func (s *Service) listProjectFolders(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	if _, err := s.verifyProjectAccess(ctx, pid, uid); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found"})
		return
	}
	var fs []store.Folder
	_ = s.DB.WithContext(ctx).
		Where("project_id = ?", store.UUID{UUID: pid}).Order("name").Find(&fs).Error
	out := make([]folderResponse, 0, len(fs))
	for i := range fs {
		out = append(out, toFolderResponse(&fs[i]))
	}
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(out, ""))
}

// Mount 注册文档域路由（folders 前缀组 + projects 子路径）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("/folders", authMw)
	g.POST("/", s.createFolder)
	g.DELETE("/:folder_id", s.deleteFolder)
	g.GET("/:folder_id/documents", s.listFolderDocuments)

	p := rg.Group("/projects", authMw)
	p.GET("/:project_id/documents", s.listProjectDocuments)
	p.GET("/:project_id/folders", s.listProjectFolders)

	d := rg.Group("/documents", authMw)
	d.POST("/upload", s.upload)
	d.POST("/:document_id/confirm", s.confirm)
	d.GET("/:document_id/preview", s.preview)
	d.PUT("/:document_id/move", s.move)
	d.PUT("/:document_id/remark", s.updateRemark)
	d.DELETE("/:document_id", s.delete)
}
