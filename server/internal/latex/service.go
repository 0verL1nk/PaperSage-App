package latex

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 契约来源：backend/services/latex/resource_builder.py + service.py

// ProjectFile 项目文件（文本 content 或二进制 URL 引用）。
type ProjectFile struct {
	Path     string
	Content  *string
	URL      *string
	IsBinary bool
}

func strPtr(s string) *string { return &s }

// BuildResources 转换为 CLSI 资源列表，返回 (resources, 校验后的 root)。
// root 缺失时推断第一个 .tex；二进制走 URL 引用。
func BuildResources(files []ProjectFile, rootFile string) ([]Resource, string) {
	resources := make([]Resource, 0, len(files))
	foundRoot := false
	for _, f := range files {
		if f.IsBinary || (f.Content == nil && isBinaryExt(f.Path)) {
			u := ""
			if f.URL != nil {
				u = *f.URL
			}
			resources = append(resources, Resource{Path: f.Path, URL: &u})
		} else if f.Content != nil {
			c := *f.Content
			resources = append(resources, Resource{Path: f.Path, Content: &c})
		} else if f.URL != nil {
			u := *f.URL
			resources = append(resources, Resource{Path: f.Path, URL: &u})
		} else {
			empty := ""
			resources = append(resources, Resource{Path: f.Path, Content: &empty})
		}
		if f.Path == rootFile {
			foundRoot = true
		}
	}
	if foundRoot {
		return resources, rootFile
	}
	for _, r := range resources {
		if strings.HasSuffix(strings.ToLower(r.Path), ".tex") {
			return resources, r.Path
		}
	}
	return resources, "main.tex"
}

// ServiceError 对应 LaTeXServiceError（携带状态码）。
type ServiceError struct {
	Code int
	Msg  string
}

func (e *ServiceError) Error() string { return e.Msg }

// Service 编排 CLSI 操作（PDF 字节 LRU 缓存 64）。
type Service struct {
	Client   ClsiClient
	Cfg      *config.Settings
	pdfCache *lru.Cache[string, []byte]
}

// NewService wires the latex compile service.
func NewService(client ClsiClient, cfg *config.Settings) (*Service, error) {
	cache, err := lru.New[string, []byte](64)
	if err != nil {
		return nil, err
	}
	return &Service{Client: client, Cfg: cfg, pdfCache: cache}, nil
}

// VerifyProjectAccess 校验项目归属（失败 → 404 ServiceError）。
func (s *Service) VerifyProjectAccess(ctx context.Context, db *gorm.DB, projectID, userID uuid.UUID) error {
	var n int64
	err := db.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ? AND is_deleted = ?",
			store.UUID{UUID: projectID}, store.UUID{UUID: userID}, false).
		Count(&n).Error
	if err != nil || n == 0 {
		return &ServiceError{Code: 404, Msg: "Project not found"}
	}
	return nil
}

var failedStatuses = map[string]bool{StatusFailure: true, StatusError: true, StatusTimedout: true}

// CompileProject 组装资源并提交 CLSI；失败时从 output.log 提取错误摘要。
func (s *Service) CompileProject(ctx context.Context, projectID string, files []ProjectFile, rootFile, compiler, syncType string, syncState *string) (*CompileResult, error) {
	if !s.Cfg.LatexCLSIEnabled {
		return nil, &ServiceError{Code: 503, Msg: "LaTeX compilation is disabled"}
	}
	resources, validatedRoot := BuildResources(files, rootFile)
	if len(resources) == 0 {
		return nil, &ServiceError{Code: 400, Msg: "No files provided for compilation"}
	}
	result, err := s.Client.Compile(ctx, projectID, resources, validatedRoot, compiler, syncType, syncState)
	if err != nil {
		return nil, err
	}
	if failedStatuses[result.Status] && result.BuildID != nil {
		s.enrichErrorFromLog(ctx, projectID, *result.BuildID, result)
	}
	return result, nil
}

const (
	maxLogSize           = 8192
	maxErrorSummaryLines = 30
)

var latexErrorRe = regexp.MustCompile(`(?m)^!\s+.+`)

// enrichErrorFromLog 拉取 output.log 提取错误行（对齐 _enrich_error_from_log）。
func (s *Service) enrichErrorFromLog(ctx context.Context, projectID, buildID string, result *CompileResult) {
	code, body, _, err := s.Client.GetOutputFile(ctx, projectID, buildID, "output.log")
	if err != nil || code != 200 {
		return
	}
	raw := string(body)
	if len(raw) > maxLogSize {
		raw = raw[:maxLogSize]
	}
	result.Logs = &raw

	var summary string
	if lines := latexErrorRe.FindAllString(raw, -1); len(lines) > 0 {
		if len(lines) > maxErrorSummaryLines {
			lines = lines[:maxErrorSummaryLines]
		}
		summary = strings.Join(lines, "\n")
	} else if strings.Contains(strings.ToLower(raw), "fatal error") {
		var fatal []string
		for _, line := range strings.Split(raw, "\n") {
			if strings.Contains(strings.ToLower(line), "fatal error") {
				fatal = append(fatal, line)
			}
		}
		summary = strings.Join(fatal, "\n")
		if len(summary) > maxLogSize {
			summary = summary[:maxLogSize]
		}
	} else {
		var tail []string
		for _, line := range strings.Split(raw, "\n") {
			if strings.TrimSpace(line) != "" {
				tail = append(tail, line)
			}
		}
		if len(tail) > 10 {
			tail = tail[len(tail)-10:]
		}
		summary = strings.Join(tail, "\n")
	}
	if summary == "" && result.Error != nil {
		summary = *result.Error
	}
	if summary == "" {
		summary = "Compilation failed"
	}
	result.Error = &summary
}

// GetOutputPDF 拉取编译 PDF（带 LRU 缓存）。
func (s *Service) GetOutputPDF(ctx context.Context, projectID, buildID string) ([]byte, error) {
	if cached, ok := s.pdfCache.Get(buildID); ok {
		return cached, nil
	}
	code, body, _, err := s.Client.GetOutputFile(ctx, projectID, buildID, "output.pdf")
	if err != nil {
		return nil, &ServiceError{Code: 502, Msg: fmt.Sprintf("无法获取编译输出: %v", err)}
	}
	if code != 200 {
		return nil, &ServiceError{Code: 502, Msg: fmt.Sprintf("CLSI returned %d", code)}
	}
	s.pdfCache.Add(buildID, body)
	return body, nil
}

// CompileStatus 常量别名（store.JobStatus 字符串一致）。
func jobStatus(s string) store.JobStatus { return store.JobStatus(s) }
