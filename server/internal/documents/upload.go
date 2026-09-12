package documents

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/api/routes/documents.py + document_service.py

// allowedMimetypes 对应 ALLOWED_MIME_TYPES。
var allowedMimetypes = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"text/plain": true,
}

const maxFileSizeBytes = 50 * 1024 * 1024 // 50 MB

func allowedMimeList() string {
	ms := make([]string, 0, len(allowedMimetypes))
	for m := range allowedMimetypes {
		ms = append(ms, m)
	}
	sort.Strings(ms)
	return strings.Join(ms, ", ")
}

// POST /documents/upload（multipart：projectId、file、folderId?、remark）
func (s *Service) upload(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID

	projectID := c.PostForm("projectId")
	if projectID == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	pid, err := uuid.Parse(projectID)
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	fileName := fh.Filename
	if fileName == "" {
		fileName = "unnamed"
	}
	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	remark := c.PostForm("remark")
	var remarkPtr *string
	if remark != "" {
		remarkPtr = &remark
	}
	var folderID *store.UUID
	if fid := c.PostForm("folderId"); fid != "" {
		f, err := uuid.Parse(fid)
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		folderID = &store.UUID{UUID: f}
	}

	if _, err := s.verifyProjectAccess(ctx, pid, uid); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Project not found"})
		return
	}
	if !allowedMimetypes[contentType] {
		httpapi.WriteError(c.Writer, &httpapi.APIError{
			Code: 400, Info: fmt.Sprintf("Unsupported file type: %s. Allowed: %s", contentType, allowedMimeList()),
		})
		return
	}
	if fh.Size > maxFileSizeBytes {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "文件大小不能超过 50MB"})
		return
	}
	if folderID != nil {
		var folder store.Folder
		err := s.DB.WithContext(ctx).
			Where("id = ? AND project_id = ?", folderID, store.UUID{UUID: pid}).First(&folder).Error
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Folder not found"})
			return
		}
	}

	content, err := readFileHeader(c, fh)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	s3Key := path.Join("projects", pid.String(), "documents", uuid.New().String(), fileName)
	if err := s.Storage.Put(ctx, s3Key, content); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}

	// staged=true → 暂存行（不进默认列表），由显式 confirm 入库
	status := "uploaded"
	if v := c.PostForm("staged"); v == "true" || v == "1" {
		status = "staged"
	}
	// 查重：同项目内已入库的同名同大小文件（提示级，不阻断）
	var dupCount int64
	_ = s.DB.WithContext(ctx).Model(&store.Document{}).
		Where("project_id = ? AND name = ? AND size = ? AND status = ?",
			store.UUID{UUID: pid}, fileName, int64(len(content)), "uploaded").
		Count(&dupCount).Error

	doc := store.Document{
		Name: fileName, Type: contentType, Size: int64(len(content)), Status: status,
		S3Key: s3Key, Remark: remarkPtr,
		ProjectID: store.UUID{UUID: pid}, FolderID: folderID, OwnerID: store.UUID{UUID: uid},
	}
	if err := s.DB.WithContext(ctx).Create(&doc).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	resp := toDocumentResponse(&doc)
	resp.Duplicate = dupCount > 0
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(resp, ""))
}

// GET /documents/{id}/preview — 裸响应（非信封），inline Content-Disposition
func (s *Service) preview(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	did, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var doc store.Document
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: did}, store.UUID{UUID: uid}).First(&doc).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Document not found"})
		return
	}
	content, err := s.Storage.Get(ctx, doc.S3Key)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	encoded := url.QueryEscape(doc.Name)
	c.Header("Content-Type", doc.Type)
	c.Header("Content-Disposition",
		fmt.Sprintf(`inline; filename="%s"; filename*=UTF-8''%s`, encoded, encoded))
	c.Status(200)
	_, _ = c.Writer.Write(content)
}

// POST /documents/{id}/confirm — staged → uploaded（入库）
func (s *Service) confirm(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	did, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var doc store.Document
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: did}, store.UUID{UUID: uid}).First(&doc).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Document not found"})
		return
	}
	if doc.Status != "staged" {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 409, Info: "Document is not staged"})
		return
	}
	_ = s.DB.WithContext(ctx).Model(&doc).Update("status", "uploaded").Error
	doc.Status = "uploaded"
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toDocumentResponse(&doc), ""))
}

func (s *Service) move(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	did, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var req struct {
		FolderID  *uuid.UUID `json:"folderId"`
		FolderIDS *uuid.UUID `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var doc store.Document
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: did}, store.UUID{UUID: uid}).First(&doc).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Document not found"})
		return
	}
	// snake_case 兼容（存量前端 files-api.ts move 发送 folder_id）
	targetFolder := req.FolderID
	if targetFolder == nil {
		targetFolder = req.FolderIDS
	}
	var folderID *store.UUID
	if targetFolder != nil {
		var folder store.Folder
		err := s.DB.WithContext(ctx).
			Where("id = ? AND project_id = ?", store.UUID{UUID: *targetFolder}, doc.ProjectID).First(&folder).Error
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Target folder not found"})
			return
		}
		folderID = &store.UUID{UUID: *targetFolder}
	}
	doc.FolderID = folderID
	_ = s.DB.WithContext(ctx).Model(&store.Document{}).Where("id = ?", doc.ID).
		Update("folder_id", folderID).Error
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toDocumentResponse(&doc), ""))
}

func (s *Service) updateRemark(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	did, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var req struct {
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var doc store.Document
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: did}, store.UUID{UUID: uid}).First(&doc).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Document not found"})
		return
	}
	var remark *string
	if req.Remark != "" { // Python: remark or None
		remark = &req.Remark
	}
	doc.Remark = remark
	_ = s.DB.WithContext(ctx).Model(&store.Document{}).Where("id = ?", doc.ID).
		Update("remark", remark).Error
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toDocumentResponse(&doc), ""))
}

func (s *Service) delete(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID.UUID
	did, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var doc store.Document
	err = s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", store.UUID{UUID: did}, store.UUID{UUID: uid}).First(&doc).Error
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Document not found"})
		return
	}
	// S3 对象删除失败不阻断（Python 侧行为：仅告警）
	_ = s.Storage.Delete(ctx, doc.S3Key)
	_ = s.DB.WithContext(ctx).Delete(&doc).Error
	_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"id": did.String(), "deleted": true}, ""))
}

func readFileHeader(c *gin.Context, fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}
