package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/extract"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// ProjectTools 构建 project 作用域的工具集（agentic 核心：读文献、
// 读写 LaTeX、触发编译）。每次请求构建，闭包携带归属校验。
func ProjectTools(db *gorm.DB, stg storage.Storage, queue *latex.Queue,
	projectID, ownerID store.UUID) ([]tool.BaseTool, error) {

	verify := func(ctx context.Context) error {
		var n int64
		if err := db.WithContext(ctx).Model(&store.Project{}).
			Where("id = ? AND owner_id = ?", projectID, ownerID).
			Count(&n).Error; err != nil || n == 0 {
			return fmt.Errorf("project not accessible")
		}
		return nil
	}

	// ── 读侧 ──────────────────────────────────────────────

	listDocuments, err := utils.InferTool(
		"list_documents",
		"列出项目已入库的参考文献（名称/类型/大小/备注）。用户问「我有哪些文献/参考资料」时调用。",
		func(ctx context.Context, _ struct{}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			var docs []store.Document
			if err := db.WithContext(ctx).
				Where("project_id = ?", projectID).
				Order("created_at DESC").Limit(100).Find(&docs).Error; err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 篇：\n", len(docs))
			for _, d := range docs {
				remark := ""
				if d.Remark != nil {
					remark = *d.Remark
				}
				fmt.Fprintf(&b, "- %s (%s, %d bytes) %s\n", d.Name, d.Type, d.Size, remark)
			}
			return b.String(), nil
		})
	if err != nil {
		return nil, err
	}

	readDocument, err := utils.InferTool(
		"read_document",
		"读取一篇参考文献的正文文本（支持 PDF/DOCX/TXT 全量抽取）。",
		func(ctx context.Context, in struct {
			Name string `json:"name" jsonschema:"description=文献文件名"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			var doc store.Document
			if err := db.WithContext(ctx).
				Where("project_id = ? AND name = ?", projectID, in.Name).
				First(&doc).Error; err != nil {
				return "", fmt.Errorf("document %q not found", in.Name)
			}
			content, err := stg.Get(ctx, doc.S3Key)
			if err != nil {
				return "", err
			}
			return extract.Text(ctx, content, doc.Name, doc.Type, 48*1024)
		})
	if err != nil {
		return nil, err
	}

	listLatexFiles, err := utils.InferTool(
		"list_latex_files",
		"列出项目的 LaTeX 源文件树（路径/大小）。了解论文结构时调用。",
		func(ctx context.Context, _ struct{}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			var files []store.LatexFile
			if err := db.WithContext(ctx).
				Where("project_id = ?", projectID).
				Order("file_path").Find(&files).Error; err != nil {
				return "", err
			}
			var b strings.Builder
			for _, f := range files {
				fmt.Fprintf(&b, "- %s (%d bytes)\n", f.FilePath, f.SizeBytes)
			}
			return b.String(), nil
		})
	if err != nil {
		return nil, err
	}

	readLatexFile, err := utils.InferTool(
		"read_latex_file",
		"读取一个 LaTeX 源文件的内容。",
		func(ctx context.Context, in struct {
			Path string `json:"path" jsonschema:"description=文件路径，如 main.tex"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			f, content, err := findLatex(ctx, db, stg, projectID, in.Path)
			if err != nil {
				return "", err
			}
			_ = f
			return content, nil
		})
	if err != nil {
		return nil, err
	}

	// ── 写侧（agentic 分水岭）────────────────────────────

	writeLatexFile, err := utils.InferTool(
		"write_latex_file",
		"写入/更新一个 LaTeX 源文件的全部内容（整文件覆盖）。修改论文后应建议用户编译验证。",
		func(ctx context.Context, in struct {
			Path    string `json:"path" jsonschema:"description=文件路径"`
			Content string `json:"content" jsonschema:"description=完整文件内容"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			if err := validateAgentPath(in.Path); err != nil {
				return "", err
			}
			sum := md5.Sum([]byte(in.Content))
			hash := hex.EncodeToString(sum[:])
			s3Key := storage.BuildKey(projectID.String(), in.Path)
			if err := stg.Put(ctx, s3Key, []byte(in.Content)); err != nil {
				return "", err
			}
			now := time.Now().UTC()
			var f store.LatexFile
			err := db.WithContext(ctx).
				Where("project_id = ? AND file_path = ?", projectID, in.Path).
				First(&f).Error
			if err != nil {
				f = store.LatexFile{
					ProjectID: projectID, FilePath: in.Path,
					ContentType: ptr("text/x-tex"), S3Key: &s3Key,
					SizeBytes: int64(len(in.Content)), ContentHash: &hash,
					LastSavedAt: &now,
				}
				if err := db.WithContext(ctx).Create(&f).Error; err != nil {
					return "", err
				}
			} else {
				updates := map[string]any{
					"s3_key": s3Key, "size_bytes": int64(len(in.Content)),
					"content_hash": hash, "last_saved_at": now,
				}
				if err := db.WithContext(ctx).Model(&f).Updates(updates).Error; err != nil {
					return "", err
				}
			}
			return fmt.Sprintf("已写入 %s（%d 字节）。", in.Path, len(in.Content)), nil
		})
	if err != nil {
		return nil, err
	}

	compileProject, err := utils.InferTool(
		"compile_project",
		"触发项目 LaTeX 编译（异步任务）。修改论文后用来验证能否编译通过。",
		func(ctx context.Context, in struct {
			RootFile string `json:"root_file,omitempty" jsonschema:"description=编译入口文件，默认 main.tex 可省略"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			root := in.RootFile
			if root == "" {
				root = "main.tex"
			}
			job, err := queue.Submit(ctx, projectID.UUID, ownerID.UUID, root, "pdflatex")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("编译任务已提交（job id: %s），用户可在编译面板查看进度。", job.ID.String()), nil
		})
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{
		listDocuments, readDocument, listLatexFiles, readLatexFile,
		writeLatexFile, compileProject,
	}, nil
}

// validateAgentPath 与 latex 域同规则：禁穿越、禁危险扩展。
func validateAgentPath(p string) error {
	if strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
		return fmt.Errorf("path traversal is not allowed")
	}
	lower := strings.ToLower(p)
	for _, ext := range []string{".exe", ".dll", ".so", ".sh", ".bat"} {
		if strings.HasSuffix(lower, ext) {
			return fmt.Errorf("file type not allowed: %s", ext)
		}
	}
	if len(p) == 0 || len(p) > 500 {
		return fmt.Errorf("invalid path length")
	}
	return nil
}

func findLatex(ctx context.Context, db *gorm.DB, stg storage.Storage,
	projectID store.UUID, path string) (*store.LatexFile, string, error) {
	var f store.LatexFile
	if err := db.WithContext(ctx).
		Where("project_id = ? AND file_path = ?", projectID, path).
		First(&f).Error; err != nil {
		return nil, "", fmt.Errorf("file %q not found", path)
	}
	if f.S3Key == nil {
		return &f, "", nil
	}
	content, err := stg.Get(ctx, *f.S3Key)
	if err != nil {
		return nil, "", err
	}
	return &f, string(content), nil
}

func ptr[T any](v T) *T { return &v }
