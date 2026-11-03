// Package latex LaTeX 域：文件 CRUD、CLSI 编译、异步队列。
// 本文件：文件管理（契约 backend/services/latex/file_service.py + api/routes/latex_files.py）。
package latex

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/importreceipt"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 文本/二进制扩展集合（对齐 file_service.py）
var allowedExtensions = []string{".tex", ".bib", ".cls", ".sty", ".bst", ".txt", ".md", ".lof", ".lot", ".toc"}

var blockedExtensions = map[string]bool{".exe": true, ".dll": true, ".so": true, ".dylib": true}

var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".svg": true,
	".webp": true, ".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".bz2": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eps": true, ".ps": true,
}

func hasExt(p string, exts []string) bool {
	lp := strings.ToLower(p)
	for _, e := range exts {
		if strings.HasSuffix(lp, e) {
			return true
		}
	}
	return false
}

func isBinaryExt(p string) bool {
	lp := strings.ToLower(p)
	for e := range binaryExtensions {
		if strings.HasSuffix(lp, e) {
			return true
		}
	}
	return false
}

// validatePath 校验并归一化路径（".." 禁止、blocked 扩展拒绝、去前导斜杠）。
func validatePath(p string) (string, error) {
	if strings.Contains(p, "..") {
		return "", fmt.Errorf("Path traversal is not allowed") //nolint:staticcheck // ST1005: 与 python 契约逐字节一致
	}
	normalized := strings.TrimLeft(p, "/")
	if i := strings.LastIndex(normalized, "."); i >= 0 {
		if blockedExtensions[strings.ToLower(normalized[i:])] {
			return "", fmt.Errorf("File type not allowed: %s", strings.ToLower(normalized[i:])) //nolint:staticcheck // ST1005: python 契约
		}
	}
	return normalized, nil
}

// FilesService LaTeX 文件管理（LRU 内容缓存 + S3 存储，全部注入）。
type FilesService struct {
	DB      *gorm.DB
	Storage storage.Storage
	cache   *lru.Cache[string, string] // s3_key → text content
}

// NewFilesService wires the latex files domain（LRU 512 项，对齐 _MAX_CACHE_SIZE）。
func NewFilesService(db *gorm.DB, stg storage.Storage) (*FilesService, error) {
	cache, err := lru.New[string, string](512)
	if err != nil {
		return nil, err
	}
	return &FilesService{DB: db, Storage: stg, cache: cache}, nil
}

func (s *FilesService) verifyProjectAccess(ctx context.Context, projectID, userID uuid.UUID) error {
	var p store.Project
	err := s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?",
			store.UUID{UUID: projectID}, store.UUID{UUID: userID}).First(&p).Error
	if err != nil {
		return fmt.Errorf("Project not found") //nolint:staticcheck // ST1005: python 契约
	}
	return nil
}

func (s *FilesService) findByPath(ctx context.Context, projectID uuid.UUID, filePath string) (*store.LatexFile, error) {
	var f store.LatexFile
	err := s.DB.WithContext(ctx).
		Where("project_id = ? AND file_path = ?", store.UUID{UUID: projectID}, filePath).
		First(&f).Error
	if err != nil {
		return nil, fmt.Errorf("File not found") //nolint:staticcheck // ST1005: python 契约文案
	}
	return &f, nil
}

// ListFiles 列出项目全部文件（按 file_path 排序）。
func (s *FilesService) ListFiles(ctx context.Context, projectID, userID uuid.UUID) ([]store.LatexFile, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return nil, err
	}
	var files []store.LatexFile
	_ = s.DB.WithContext(ctx).
		Where("project_id = ?", store.UUID{UUID: projectID}).
		Order("file_path").Find(&files).Error
	return files, nil
}

// GetFileRaw 返回原始字节与 content-type（文本走缓存）。
func (s *FilesService) GetFileRaw(ctx context.Context, projectID uuid.UUID, filePath string, userID uuid.UUID) ([]byte, string, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return nil, "", err
	}
	f, err := s.findByPath(ctx, projectID, filePath)
	if err != nil {
		return nil, "", err
	}
	s3Key := ""
	if f.S3Key != nil {
		s3Key = *f.S3Key
	}
	contentType := "application/octet-stream"
	if f.ContentType != nil {
		contentType = *f.ContentType
	}
	if hasExt(filePath, allowedExtensions) {
		if cached, ok := s.cache.Get(s3Key); ok {
			return []byte(cached), contentType, nil
		}
	}
	content, err := s.Storage.Get(ctx, s3Key)
	if err != nil {
		return nil, "", err
	}
	if hasExt(filePath, allowedExtensions) {
		s.cache.Add(s3Key, string(content))
	}
	return content, contentType, nil
}

// CreateFileBinary 创建文本或二进制文件。
func (s *FilesService) CreateFileBinary(ctx context.Context, projectID uuid.UUID, p string, content []byte, contentType string, userID uuid.UUID) (*store.LatexFile, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return nil, err
	}
	validated, err := validatePath(p)
	if err != nil {
		return nil, err
	}
	if _, err := s.findByPath(ctx, projectID, validated); err == nil {
		return nil, fmt.Errorf("File already exists: %s", validated) //nolint:staticcheck // ST1005: python 契约文案
	}

	sum := md5.Sum(content)
	hash := hex.EncodeToString(sum[:])
	// 每次创建独立对象键：路径键在重命名后会被新建同路径文件复用并覆盖原对象。
	s3Key := "latex-file-objects/" + projectID.String() + "/" + uuid.NewString()
	if err := s.Storage.Put(ctx, s3Key, content); err != nil {
		return nil, err
	}
	if strings.Contains(contentType, "text") {
		s.cache.Add(s3Key, string(content))
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	f := store.LatexFile{
		ProjectID:   store.UUID{UUID: projectID},
		FilePath:    validated,
		ContentType: &contentType,
		SizeBytes:   int64(len(content)),
		ContentHash: &hash,
		S3Key:       &s3Key,
		LastSavedAt: nowUTC(),
	}
	if err := s.DB.WithContext(ctx).Create(&f).Error; err != nil {
		if deleteErr := s.Storage.Delete(ctx, s3Key); deleteErr != nil {
			return nil, fmt.Errorf("save LaTeX file metadata: %w (clean up object: %v)", err, deleteErr)
		}
		s.cache.Remove(s3Key)
		return nil, err
	}
	return &f, nil
}

// CreateFileBinaryWithImport makes a browser import create replay-safe when a
// stable import key is supplied. A nil key preserves the normal create path.
func (s *FilesService) CreateFileBinaryWithImport(ctx context.Context, projectID uuid.UUID, p string, content []byte, contentType string, userID uuid.UUID, key *importreceipt.Key) (*store.LatexFile, error) {
	if key == nil {
		return s.CreateFileBinary(ctx, projectID, p, content, contentType, userID)
	}
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return nil, err
	}
	validated, err := validatePath(p)
	if err != nil {
		return nil, err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	contentDigest := sha256.Sum256(content)
	fingerprint, err := importreceipt.Fingerprint(struct {
		ProjectID   uuid.UUID
		Path        string
		ContentType string
		ContentSHA  string
	}{ProjectID: projectID, Path: validated, ContentType: contentType, ContentSHA: hex.EncodeToString(contentDigest[:])})
	if err != nil {
		return nil, err
	}
	md5Digest := md5.Sum(content)
	contentHash := hex.EncodeToString(md5Digest[:])
	s3Key := storage.BuildKey(projectID.String(), "_imports/"+key.BundleID.String()+"/"+key.LocalID.String())
	file := store.LatexFile{
		ProjectID: store.UUID{UUID: projectID}, FilePath: validated,
		ContentType: &contentType, SizeBytes: int64(len(content)), ContentHash: &contentHash,
		S3Key: &s3Key, LastSavedAt: nowUTC(),
	}
	file.ID = store.MustNewUUID()
	objectWritten := false
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resourceID, replay, err := importreceipt.Claim(tx, store.UUID{UUID: userID}, *key, "latex_file", fingerprint, file.ID)
		if err != nil {
			return err
		}
		if replay {
			file.ID = resourceID
			return tx.Where("id = ? AND project_id = ?", file.ID, store.UUID{UUID: projectID}).First(&file).Error
		}
		var existing int64
		if err := tx.Model(&store.LatexFile{}).Where("project_id = ? AND file_path = ?", store.UUID{UUID: projectID}, validated).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return fmt.Errorf("file already exists: %s", validated)
		}
		if err := s.Storage.Put(ctx, s3Key, content); err != nil {
			return err
		}
		objectWritten = true
		return tx.Create(&file).Error
	})
	if err != nil {
		if objectWritten {
			_ = s.Storage.Delete(ctx, s3Key)
		}
		return nil, err
	}
	if strings.Contains(contentType, "text") {
		s.cache.Add(s3Key, string(content))
	}
	return &file, nil
}

// CreateFile 创建文本文件（content_type=text/plain）。
func (s *FilesService) CreateFile(ctx context.Context, projectID uuid.UUID, p, content string, userID uuid.UUID) (*store.LatexFile, error) {
	return s.CreateFileBinary(ctx, projectID, p, []byte(content), "text/plain", userID)
}

func (s *FilesService) CreateFileWithImport(ctx context.Context, projectID uuid.UUID, p, content string, userID uuid.UUID, key *importreceipt.Key) (*store.LatexFile, error) {
	return s.CreateFileBinaryWithImport(ctx, projectID, p, []byte(content), "text/plain", userID, key)
}

// UpdateFile 更新内容（md5 相同则跳过上传）。
func (s *FilesService) UpdateFile(ctx context.Context, projectID uuid.UUID, filePath, content string, userID uuid.UUID) (*store.LatexFile, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return nil, err
	}
	f, err := s.findByPath(ctx, projectID, filePath)
	if err != nil {
		return nil, err
	}
	contentBytes := []byte(content)
	sum := md5.Sum(contentBytes)
	newHash := hex.EncodeToString(sum[:])
	if f.ContentHash != nil && *f.ContentHash == newHash {
		return f, nil // 内容未变，跳过
	}
	s3Key := ""
	if f.S3Key != nil {
		s3Key = *f.S3Key
	}
	if err := s.Storage.Put(ctx, s3Key, contentBytes); err != nil {
		return nil, err
	}
	s.cache.Add(s3Key, content)
	now := nowUTC()
	_ = s.DB.WithContext(ctx).Model(&store.LatexFile{}).Where("id = ?", f.ID).
		Updates(map[string]any{
			"content_hash": newHash, "size_bytes": len(contentBytes), "last_saved_at": now,
		}).Error
	f.ContentHash = &newHash
	f.SizeBytes = int64(len(contentBytes))
	f.LastSavedAt = now
	return f, nil
}

// DeleteFile 删除文件（S3 对象 + 缓存 + 记录）。
func (s *FilesService) DeleteFile(ctx context.Context, projectID uuid.UUID, filePath string, userID uuid.UUID) error {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return err
	}
	f, err := s.findByPath(ctx, projectID, filePath)
	if err != nil {
		return err
	}
	if f.S3Key != nil {
		_ = s.Storage.Delete(ctx, *f.S3Key)
		s.cache.Remove(*f.S3Key)
	}
	return s.DB.WithContext(ctx).Delete(f).Error
}

// RenameFile 改路径（S3 对象不动）。
func (s *FilesService) RenameFile(ctx context.Context, projectID uuid.UUID, oldPath, newPath string, userID uuid.UUID) (*store.LatexFile, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return nil, err
	}
	f, err := s.findByPath(ctx, projectID, oldPath)
	if err != nil {
		return nil, err
	}
	if _, err := s.findByPath(ctx, projectID, newPath); err == nil {
		return nil, fmt.Errorf("A file with that path already exists") //nolint:staticcheck // ST1005: python 契约文案
	}
	f.FilePath = newPath
	if err := s.DB.WithContext(ctx).Model(&store.LatexFile{}).Where("id = ?", f.ID).
		Update("file_path", newPath).Error; err != nil {
		return nil, err
	}
	return f, nil
}

// MoveFile 移动到目录（保留文件名）。
func (s *FilesService) MoveFile(ctx context.Context, projectID uuid.UUID, oldPath, newDir string, userID uuid.UUID) (*store.LatexFile, error) {
	filename := oldPath
	if i := strings.LastIndex(oldPath, "/"); i >= 0 {
		filename = oldPath[i+1:]
	}
	newPath := filename
	if newDir != "" {
		newPath = newDir + "/" + filename
	}
	return s.RenameFile(ctx, projectID, oldPath, newPath, userID)
}

func filesWithPrefix(ctx context.Context, db *gorm.DB, projectID uuid.UUID, prefix string) []store.LatexFile {
	var files []store.LatexFile
	_ = db.WithContext(ctx).
		Where("project_id = ? AND file_path LIKE ?", store.UUID{UUID: projectID}, prefix+"%").
		Find(&files).Error
	return files
}

// RenameFolder 前缀替换式改目录名。
func (s *FilesService) RenameFolder(ctx context.Context, projectID uuid.UUID, oldFolder, newFolder string, userID uuid.UUID) (int, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return 0, err
	}
	oldPrefix := strings.TrimRight(oldFolder, "/") + "/"
	newPrefix := strings.TrimRight(newFolder, "/") + "/"

	if collision := filesWithPrefix(ctx, s.DB, projectID, newPrefix); len(collision) > 0 {
		return 0, &ContractError{Message: "A folder with that name already exists"}
	}
	files := filesWithPrefix(ctx, s.DB, projectID, oldPrefix)
	for i := range files {
		f := &files[i]
		if f.S3Key != nil {
			s.cache.Remove(*f.S3Key)
		}
		newPath := newPrefix + f.FilePath[len(oldPrefix):]
		_ = s.DB.WithContext(ctx).Model(&store.LatexFile{}).Where("id = ?", f.ID).
			Update("file_path", newPath).Error
		f.FilePath = newPath
	}
	return len(files), nil
}

// DeleteFolder 递归删除目录下全部文件。
func (s *FilesService) DeleteFolder(ctx context.Context, projectID uuid.UUID, folderPath string, userID uuid.UUID) (int, error) {
	if err := s.verifyProjectAccess(ctx, projectID, userID); err != nil {
		return 0, err
	}
	prefix := strings.TrimRight(folderPath, "/") + "/"
	files := filesWithPrefix(ctx, s.DB, projectID, prefix)
	for i := range files {
		f := &files[i]
		if f.S3Key != nil {
			_ = s.Storage.Delete(ctx, *f.S3Key)
			s.cache.Remove(*f.S3Key)
		}
		_ = s.DB.WithContext(ctx).Delete(f).Error
	}
	return len(files), nil
}

// LoadTextContents 载入全部文本文件（并行 + LRU；编译用）。
// 任一文件读取失败即返回错误：不完整输入不能当作成功编译。
func (s *FilesService) LoadTextContents(ctx context.Context, projectID uuid.UUID, concurrency int) ([][2]string, error) {
	var files []store.LatexFile
	_ = s.DB.WithContext(ctx).Where("project_id = ?", store.UUID{UUID: projectID}).Find(&files).Error
	text := make([]store.LatexFile, 0, len(files))
	for _, f := range files {
		if !isBinaryExt(f.FilePath) {
			text = append(text, f)
		}
	}
	return s.parallelLoad(ctx, text, concurrency)
}

// GetBinaryFileKeys 返回二进制文件 (path, s3_key) 列表（编译用）。
func (s *FilesService) GetBinaryFileKeys(ctx context.Context, projectID uuid.UUID) [][2]string {
	var files []store.LatexFile
	_ = s.DB.WithContext(ctx).Where("project_id = ?", store.UUID{UUID: projectID}).Find(&files).Error
	out := [][2]string{}
	for _, f := range files {
		if isBinaryExt(f.FilePath) {
			key := ""
			if f.S3Key != nil {
				key = *f.S3Key
			}
			out = append(out, [2]string{f.FilePath, key})
		}
	}
	return out
}

func (s *FilesService) parallelLoad(ctx context.Context, files []store.LatexFile, concurrency int) ([][2]string, error) {
	if concurrency <= 0 {
		concurrency = 8
	}
	var (
		mu       sync.Mutex
		pairs    [][2]string
		firstErr error
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, concurrency)
	for i := range files {
		f := files[i]
		key := ""
		if f.S3Key != nil {
			key = *f.S3Key
		}
		if cached, ok := s.cache.Get(key); ok {
			pairs = append(pairs, [2]string{f.FilePath, cached})
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			content, err := s.Storage.Get(ctx, key)
			if err != nil {
				slog.Error("failed to load latex file", "path", f.FilePath, "key", key, "error", err)
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("load %s: %w", f.FilePath, err)
				}
				mu.Unlock()
				return
			}
			decoded := string(content)
			s.cache.Add(key, decoded)
			mu.Lock()
			pairs = append(pairs, [2]string{f.FilePath, decoded})
			mu.Unlock()
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	return pairs, nil
}

// ── 响应 DTO ──

type fileResponse struct {
	ID          uuid.UUID     `json:"id"`
	FilePath    string        `json:"file_path"`
	ContentType *string       `json:"content_type"`
	SizeBytes   int64         `json:"size_bytes"`
	LastSavedAt *dto.JSONTime `json:"last_saved_at"`
	CreatedAt   dto.JSONTime  `json:"created_at"`
	UpdatedAt   dto.JSONTime  `json:"updated_at"`
}

func toFileResponse(f *store.LatexFile) fileResponse {
	r := fileResponse{
		ID: f.ID.UUID, FilePath: f.FilePath, ContentType: f.ContentType,
		SizeBytes: f.SizeBytes,
		CreatedAt: dto.JSONTime{Time: f.CreatedAt},
		UpdatedAt: dto.JSONTime{Time: f.UpdatedAt},
	}
	if f.LastSavedAt != nil {
		r.LastSavedAt = &dto.JSONTime{Time: *f.LastSavedAt}
	}
	return r
}

// ContractError 承载与 Python 响应逐字节一致的错误文案。
// 已知错误类型化（而非裸 fmt.Errorf）是 Go 惯用法；文案属于边界层
// 响应数据，不走 error string 命名约定。
type ContractError struct{ Message string }

func (e *ContractError) Error() string { return e.Message }

func errText(code int, msg string) *httpapi.APIError { return &httpapi.APIError{Code: code, Info: msg} }

func writeLatexCreateError(c *gin.Context, err error) {
	if errors.Is(err, importreceipt.ErrConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
		httpapi.WriteError(c.Writer, errText(http.StatusConflict, "Import key conflicts with existing content or resource"))
		return
	}
	httpapi.WriteError(c.Writer, errText(http.StatusBadRequest, err.Error()))
}

// MountFiles 注册 latex-files 路由（catch-all 内部分发，精确复刻 FastAPI 顺序匹配）。
func MountFiles(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *FilesService) {
	g := rg.Group("/projects/:project_id/latex-files", authMw)

	projectID := func(c *gin.Context) uuid.UUID {
		id, _ := uuid.Parse(c.Param("project_id"))
		return id
	}
	uid := func(c *gin.Context) uuid.UUID { return auth.CurrentUser(c).ID.UUID }

	g.GET("", s.listHandler(projectID, uid))
	g.POST("", s.createHandler(projectID, uid))

	// FastAPI GET /{file_path:path}；尾斜杠请求转发到列表
	g.GET("/*fp", func(c *gin.Context) {
		fp := strings.TrimPrefix(c.Param("fp"), "/")
		if fp == "" {
			s.listHandler(projectID, uid)(c)
			return
		}
		content, ctype, err := s.GetFileRaw(c.Request.Context(), projectID(c), fp, uid(c))
		if err != nil {
			httpapi.WriteError(c.Writer, errText(404, err.Error()))
			return
		}
		raw := c.Query("raw") == "true"
		if raw || !hasExt(fp, allowedExtensions) {
			filename := fp
			if i := strings.LastIndex(filename, "/"); i >= 0 {
				filename = filename[i+1:]
			}
			c.Header("Content-Type", ctype)
			c.Header("Content-Disposition",
				fmt.Sprintf("inline; filename=\"%s\"; filename*=UTF-8''%s", queryEscape(filename), queryEscape(filename)))
			c.Status(200)
			_, _ = c.Writer.Write(content)
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"path": fp, "content": string(content)}, ""))
	})

	g.PUT("/*fp", func(c *gin.Context) {
		fp := strings.TrimPrefix(c.Param("fp"), "/")
		var req struct {
			Content string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		f, err := s.UpdateFile(c.Request.Context(), projectID(c), fp, req.Content, uid(c))
		if err != nil {
			httpapi.WriteError(c.Writer, errText(404, err.Error()))
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toFileResponse(f), ""))
	})

	// POST 内部分发：/folder/rename、{path}/rename、{path}/move（对齐 FastAPI 注册顺序）
	g.POST("/*fp", func(c *gin.Context) {
		fp := strings.TrimPrefix(c.Param("fp"), "/")
		pid := projectID(c)

		if fp == "" { // 尾斜杠请求转发到创建
			s.createHandler(projectID, uid)(c)
			return
		}
		switch {
		case fp == "folder/rename":
			var req struct {
				FolderPath string `json:"folder_path"`
				NewFolder  string `json:"new_folder"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			n, err := s.RenameFolder(c.Request.Context(), pid, req.FolderPath, req.NewFolder, uid(c))
			if err != nil {
				httpapi.WriteError(c.Writer, errText(400, err.Error()))
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{
				"renamed_count": n, "old_folder": req.FolderPath, "new_folder": req.NewFolder,
			}, ""))
		case strings.HasSuffix(fp, "/rename"):
			oldPath := strings.TrimSuffix(fp, "/rename")
			var req struct {
				NewPath string `json:"new_path"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || req.NewPath == "" {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			f, err := s.RenameFile(c.Request.Context(), pid, oldPath, req.NewPath, uid(c))
			if err != nil {
				httpapi.WriteError(c.Writer, errText(400, err.Error()))
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toFileResponse(f), ""))
		case strings.HasSuffix(fp, "/move"):
			oldPath := strings.TrimSuffix(fp, "/move")
			var req struct {
				NewDir string `json:"new_dir"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			f, err := s.MoveFile(c.Request.Context(), pid, oldPath, req.NewDir, uid(c))
			if err != nil {
				httpapi.WriteError(c.Writer, errText(400, err.Error()))
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toFileResponse(f), ""))
		default:
			httpapi.WriteError(c.Writer, errText(404, "Not Found"))
		}
	})

	// DELETE 内部分发：/folder（JSON body）或文件
	g.DELETE("/*fp", func(c *gin.Context) {
		fp := strings.TrimPrefix(c.Param("fp"), "/")
		pid := projectID(c)

		if fp == "folder" {
			var req struct {
				FolderPath string `json:"folder_path"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				httpapi.WriteError(c.Writer, httpapi.ErrValidation)
				return
			}
			folderPath := strings.TrimRight(req.FolderPath, "/")
			n, err := s.DeleteFolder(c.Request.Context(), pid, folderPath, uid(c))
			if err != nil {
				httpapi.WriteError(c.Writer, errText(404, err.Error()))
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{
				"deleted_count": n, "folder_path": req.FolderPath,
			}, ""))
			return
		}
		if err := s.DeleteFile(c.Request.Context(), pid, fp, uid(c)); err != nil {
			httpapi.WriteError(c.Writer, errText(404, err.Error()))
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"deleted": true}, ""))
	})
}

// listHandler 列表（GET ""，尾斜杠时由 catch-all 转发）。
func (s *FilesService) listHandler(projectID, uid func(*gin.Context) uuid.UUID) gin.HandlerFunc {
	return func(c *gin.Context) {
		files, err := s.ListFiles(c.Request.Context(), projectID(c), uid(c))
		if err != nil {
			httpapi.WriteError(c.Writer, errText(404, err.Error()))
			return
		}
		out := make([]fileResponse, 0, len(files))
		for i := range files {
			out = append(out, toFileResponse(&files[i]))
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(gin.H{"files": out}, ""))
	}
}

// createHandler 创建（JSON 文本或 multipart 二进制）。
func (s *FilesService) createHandler(projectID, uid func(*gin.Context) uuid.UUID) gin.HandlerFunc {
	return func(c *gin.Context) {
		pid := projectID(c)
		importKey, hasImportKey, keyErr := importreceipt.Parse(c.Request.Header)
		if keyErr != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		var key *importreceipt.Key
		if hasImportKey {
			key = &importKey
		}
		if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
			fh, err := c.FormFile("file")
			if err != nil {
				httpapi.WriteError(c.Writer, errText(400, "No file provided in multipart"))
				return
			}
			p := c.PostForm("path")
			if p == "" {
				p = fh.Filename
			}
			if p == "" {
				p = "unnamed"
			}
			content, err := readAll(c, fh)
			if err != nil {
				httpapi.WriteError(c.Writer, errText(500, "Internal server error"))
				return
			}
			ct := fh.Header.Get("Content-Type")
			f, err := s.CreateFileBinaryWithImport(c.Request.Context(), pid, p, content, ct, uid(c), key)
			if err != nil {
				writeLatexCreateError(c, err)
				return
			}
			_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toFileResponse(f), ""))
			return
		}
		var req struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		f, err := s.CreateFileWithImport(c.Request.Context(), pid, req.Path, req.Content, uid(c), key)
		if err != nil {
			writeLatexCreateError(c, err)
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, 200, httpapi.Success(toFileResponse(f), ""))
	}
}

// DefaultTemplate 是项目创建时的 main.tex 模板（对齐 projects.py 内嵌字符串）。
const DefaultTemplate = `\documentclass{article}
\usepackage[utf8]{inputenc}

\title{New Project}
\author{}
\date{\today}

\begin{document}

\maketitle

\section{Introduction}

Write your content here.

\end{document}`

// CreateTemplate 实现 projects 域的模板钩子（4.3 接线）。
func CreateTemplate(files *FilesService) func(ctx context.Context, projectID, userID uuid.UUID) error {
	return func(ctx context.Context, projectID, userID uuid.UUID) error {
		_, err := files.CreateFile(ctx, projectID, "main.tex", DefaultTemplate, userID)
		return err
	}
}
