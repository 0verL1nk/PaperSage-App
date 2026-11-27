package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 检查单元 Run：与聊天共用 Run/SSE/取消基础设施，但工具集是只读的，
// 且只拿到服务端锁定的本单元上下文。语义结论仍由真实模型给出。

const (
	// ReviewUnitAgentKey 写入 agent_runs.agent_key，便于审计区分聊天与检查。
	ReviewUnitAgentKey = "review_unit"
	// ReviewUnitPromptVersion 是单元提示词的版本号，参与增量指纹。
	ReviewUnitPromptVersion = "review-unit-v1"
	// reviewUnitDefaultIterations 是单元 ReAct 的默认迭代上限（控制成本与迟滞）。
	reviewUnitDefaultIterations = 12
)

// ReviewSource 是单元实际读取过的来源（服务端记录，不采信模型自报）。
type ReviewSource struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ReviewUnitReads 记录单元真实读取过的文件/资料。
type ReviewUnitReads struct {
	mu          sync.Mutex
	seen        map[string]bool
	sources     []ReviewSource
	contextRead bool
}

// NewReviewUnitReads 构造读取记录器。
func NewReviewUnitReads() *ReviewUnitReads {
	return &ReviewUnitReads{seen: map[string]bool{}}
}

// Record 记录一次真实读取。
func (r *ReviewUnitReads) Record(kind, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := kind + "\x00" + name
	if r.seen[key] || len(r.sources) >= reviewContextSrcLimit {
		return
	}
	r.seen[key] = true
	r.sources = append(r.sources, ReviewSource{Kind: kind, Name: name})
}

// Has 判断某个来源是否真的读过。
func (r *ReviewUnitReads) Has(kind, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[kind+"\x00"+strings.TrimSpace(name)]
}

// MarkContext 记录模型已获取单元上下文（提交报告的前置条件）。
func (r *ReviewUnitReads) MarkContext() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contextRead = true
}

// ContextRead 判断模型是否真的先读过上下文。
func (r *ReviewUnitReads) ContextRead() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contextRead
}

// Snapshot 返回实际读取来源的副本。
func (r *ReviewUnitReads) Snapshot() []ReviewSource {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ReviewSource, len(r.sources))
	copy(out, r.sources)
	return out
}

// ReviewUnitSpec 是一个检查单元的只读执行规格。
type ReviewUnitSpec struct {
	UnitID      string
	Instruction string
	ContextJSON []byte
	Files       []string
	Norms       []norms.Entry
	Dimensions  []string
	Reads       *ReviewUnitReads
	// Submit 由 review 域注入：先做服务端校验，通过后才允许落库。
	Submit func(ctx context.Context, payload []byte, sources []ReviewSource) error
}

// AllowsFile 判断文件是否在本单元范围内（含归一化后的比较）。
func (s *ReviewUnitSpec) AllowsFile(p string) bool {
	p = strings.TrimLeft(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"), "/")
	for _, f := range s.Files {
		if f == p {
			return true
		}
	}
	return false
}

// ReviewUnitRequest 是启动一个单元 Run 所需的全部信息。
type ReviewUnitRequest struct {
	ConversationID store.UUID
	ProjectID      store.UUID
	UserID         store.UUID
	Model          *ResolvedModel
	Content        string
	Spec           *ReviewUnitSpec
	OnSettled      func(status, failure string)
}

// StartReviewUnit 建立并启动一个检查单元 Run，返回 agent_runs.id。
func (s *Service) StartReviewUnit(req ReviewUnitRequest) (store.UUID, error) {
	if req.Spec == nil {
		return store.UUID{}, errors.New("检查单元规格不能为空")
	}
	provider, modelName := "", ""
	if req.Model != nil {
		provider, modelName = req.Model.Name, req.Model.Model
	}
	now := time.Now().UTC()
	run := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: req.ConversationID,
		AgentKey: ReviewUnitAgentKey, Status: RunRunning,
		ModelProvider: provider, ModelName: modelName, PromptVersion: ReviewUnitPromptVersion,
		StartedAt: &now,
	}
	if err := s.DB.Create(&run).Error; err != nil {
		return store.UUID{}, err
	}
	rc := runContext{
		RunID: run.ID, ConversationID: req.ConversationID,
		ProjectID: req.ProjectID, UserID: req.UserID,
		Content: req.Content, Model: req.Model,
		Review:    req.Spec,
		OnSettled: req.OnSettled,
	}
	if err := s.StartRun(rc); err != nil {
		return run.ID, err
	}
	return run.ID, nil
}

// ReviewUnitTools 构建只读检查工具集：读锁定上下文、读范围内文件与资料、提交报告。
// 没有 propose_edit / 编译 / 写入工具——检查单元不能改稿。
func ReviewUnitTools(db *gorm.DB, stg storage.Storage, projectID store.UUID, spec *ReviewUnitSpec) ([]tool.BaseTool, error) {
	if spec == nil || spec.Reads == nil {
		return nil, errors.New("检查单元未初始化")
	}
	contextTool, err := utils.InferTool(
		"get_review_unit_context",
		"读取本单元锁定的检查上下文（章节正文、章节计划、已采用规范、项目蓝图、资料清单）。提交报告前必须先调用；报告只能基于这里返回的上下文与随后真实读取的资料。",
		func(ctx context.Context, _ struct{}) (string, error) {
			spec.Reads.MarkContext()
			for _, f := range spec.Files {
				spec.Reads.Record("latex", f)
			}
			return string(spec.ContextJSON), nil
		})
	if err != nil {
		return nil, err
	}

	readFile, err := utils.InferTool(
		"read_latex_file",
		"读取本单元范围内的一个 LaTeX 文件全文。只能读取单元范围内列出的文件；引用原文时必须逐字复制。",
		func(ctx context.Context, in struct {
			Path string `json:"path" jsonschema:"description=文件路径，必须属于本单元范围"`
		}) (string, error) {
			path := strings.TrimSpace(in.Path)
			if !spec.AllowsFile(path) {
				return "", fmt.Errorf("文件 %s 不在本次检查范围内", in.Path)
			}
			_, content, err := findLatex(ctx, db, stg, projectID, path)
			if err != nil {
				return "", err
			}
			spec.Reads.Record("latex", path)
			return content, nil
		})
	if err != nil {
		return nil, err
	}

	listDocs, err := utils.InferTool(
		"list_documents",
		"列出项目已入库的参考文献资料（名称、类型、大小）。需要外部证据时先列清单，再用 read_document 真正读取。",
		func(ctx context.Context, _ struct{}) (string, error) {
			var docs []store.Document
			if err := db.WithContext(ctx).Where("project_id = ? AND status = ?", projectID, "uploaded").
				Order("created_at").Find(&docs).Error; err != nil {
				return "", err
			}
			out := make([]map[string]any, 0, len(docs))
			for _, doc := range docs {
				out = append(out, map[string]any{
					"document_id": doc.ID.String(), "name": doc.Name, "type": doc.Type, "size_bytes": doc.Size,
				})
			}
			encoded, err := json.Marshal(out)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	reader := &documentTextReader{}
	readDoc, err := utils.InferTool(
		"read_document",
		"读取已入库参考文献的正文区间。先用 list_documents 取得 document_id；未读取的部分不得声称已核查。",
		func(ctx context.Context, in documentReadInput) (*documentReadResult, error) {
			query := db.WithContext(ctx).Where("project_id = ? AND status = ?", projectID, "uploaded")
			if in.DocumentID != "" {
				query = query.Where("id = ?", in.DocumentID)
			} else if in.Name != "" {
				query = query.Where("name = ?", in.Name)
			} else {
				return nil, fmt.Errorf("document_id or name is required")
			}
			var docs []store.Document
			if err := query.Limit(2).Find(&docs).Error; err != nil {
				return nil, err
			}
			if len(docs) == 0 {
				return nil, fmt.Errorf("document not found")
			}
			if len(docs) > 1 {
				return nil, fmt.Errorf("multiple documents have this name; use document_id")
			}
			result, err := reader.read(ctx, stg, docs[0], in)
			if err != nil {
				return nil, err
			}
			spec.Reads.Record("document", docs[0].Name)
			return result, nil
		})
	if err != nil {
		return nil, err
	}

	submit, err := utils.InferTool(
		"submit_review_unit_report",
		"提交本单元的维度状态与问题清单。服务端会逐条核对范围、逐字引文、行号与依据；核对失败会退回原因，请读取原文修正后重试。",
		func(ctx context.Context, in struct {
			ReportJSON string `json:"report_json" jsonschema:"description=符合单元报告契约的 JSON 对象字符串"`
		}) (string, error) {
			if !spec.Reads.ContextRead() {
				return "", fmt.Errorf("请先调用 get_review_unit_context 读取上下文")
			}
			if err := spec.Submit(ctx, []byte(in.ReportJSON), spec.Reads.Snapshot()); err != nil {
				return "", err
			}
			return `{"accepted":true}`, nil
		})
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{contextTool, readFile, listDocs, readDoc, submit}, nil
}
