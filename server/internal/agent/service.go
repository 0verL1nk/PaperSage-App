package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 系统提示：分场景（任务书要求 prompt 优化）——本科生毕设为核心场景。
const systemPrompt = `你是 PaperSage 的论文写作助手，服务对象是本科毕业设计学生。

行为准则：
- 回答用简体中文；学术术语首次出现时附英文原文。
- 涉及项目内容（文件、文献、论文结构）时，先用工具查证再回答，不要臆测。
- 用户在写作准备阶段讨论选题、研究问题或计划时，先调用 get_writing_state 了解已有信息；像研究导师一样循序追问，一次聚焦一个关键缺口，不要求用户逐项填写空白字段。
- 对话中区分已知事实、假设和待验证事项。先向用户总结拟更新的研究蓝图与课题要求；只有用户明确确认保存后，才调用 save_writing_preparation，并使用最新 revision。未讨论课题要求时原样保留已有要求。不得捏造研究材料或把建议写成事实。
- 任何修改稿件的请求（包括“给个建议”“帮我改”“润色这段”）都必须调用 propose_edit 产出建议卡，由用户在界面上确认；不得只在回复里贴代码或声称已修改。不确定替换锚点时先 read_latex_file 取回原文。
- compile_project 只请求浏览器在写作页编译，不返回编译结果；不要声称编译已通过。
- LaTeX 建议保持与现有模板一致的结构（documentclass、宏包、章节层级）。
- 引用文献时遵循 GB/T 7714 规范格式。
- 回复保持精炼：先结论，再展开；代码/公式放代码块。`

// Service — agent 域。依赖均接口化由装配层注入。
type Service struct {
	DB      *gorm.DB
	Storage storage.Storage
	Cfg     *config.Settings
	RAG     *rag.Store
	// hub 只做进程内唤醒，事件的真相在 agent_events（多实例/重启不丢事件）。
	hub *runHub
	// cancels 登记进行中 Run 的取消函数（POST /runs/:id/cancel）。
	cancelMu sync.Mutex
	cancels  map[store.UUID]context.CancelFunc
	// retries 防止并发双击重试创建多个 Run；单实例内有效。
	// shortcut: in-process and unbounded; add a DB unique key if retries become frequent.
	retryMu sync.Mutex
	retries map[string]store.UUID
	// startedAt 用于启动时收敛上一进程遗留的 running Run。
	startedAt time.Time
	closing   bool
	runs      sync.WaitGroup
	drained   chan struct{}
	closeErr  error
	runCtx    context.Context
	stopRuns  context.CancelCauseFunc
	// files 复用 latex 域的文件服务（路径校验 + 存储 + 缓存），应用建议时用。
	// 必须复用同一个实例：FilesService 带自己的 LRU 内容缓存，各建一份会让
	// 同一 S3 key 出现两份缓存、读出陈旧内容。
	files *latex.FilesService
}

func NewService(db *gorm.DB, stg storage.Storage, files *latex.FilesService,
	cfg *config.Settings, rg *rag.Store) *Service {
	runCtx, stopRuns := context.WithCancelCause(context.Background())
	s := &Service{
		DB: db, Storage: stg, Cfg: cfg, RAG: rg, files: files,
		hub: newRunHub(), cancels: map[store.UUID]context.CancelFunc{},
		retries: map[string]store.UUID{}, startedAt: time.Now().UTC(),
		runCtx: runCtx, stopRuns: stopRuns,
	}
	_ = s.ReconcileOrphanRuns(context.Background())
	return s
}

func writeModelResolutionError(c *gin.Context, err error) {
	if errors.Is(err, ErrNoModel) || errors.Is(err, ErrInvalidModel) {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusBadRequest, Info: "尚未配置 AI 模型或选择无效：请检查设置中的服务商和模型"})
		return
	}
	httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusInternalServerError, Info: "读取 AI 模型设置失败，请稍后重试"})
}

// chat POST /projects/:id/agent/chat {conversation_id?, content} → SSE
func (s *Service) chat(c *gin.Context) {
	ctx, release, accepted := s.acceptRun(c.Request.Context())
	if !accepted {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusServiceUnavailable, Info: "服务正在关闭，请稍后重试"})
		return
	}
	defer release()
	uid := auth.CurrentUser(c).ID
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var req struct {
		ConversationID string `json:"conversation_id"`
		Content        string `json:"content"`
		// ClientRequestID 幂等键：双击发送/网络重试不产生第二个 Turn。
		ClientRequestID       string `json:"clientRequestId"`
		ExpectedThreadVersion *int64 `json:"expectedThreadVersion"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" || len([]rune(req.ClientRequestID)) > 64 {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	// 会话（新建或续接，校验归属）
	conv, err := s.ensureConversation(ctx, store.UUID{UUID: pid}, uid, req.ConversationID, req.Content)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
		return
	}
	result, err := s.createRootRun(ctx, *conv, uid, req.ClientRequestID, req.Content, req.ExpectedThreadVersion)
	if err != nil {
		s.writeRootRunError(c, conv.ID, uid, err)
		return
	}
	if result.created {
		rc := runContext{RunID: result.run.ID, ConversationID: conv.ID,
			ProjectID: store.UUID{UUID: pid}, UserID: uid, Content: req.Content, Model: result.model, finalizeDeadline: result.finalizeDeadline}
		if err := s.StartRun(rc); err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusServiceUnavailable, Info: "服务正在关闭，请稍后重试"})
			return
		}
	}

	sw, ok := newStreamWriter(c)
	if !ok {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "streaming unsupported"})
		return
	}
	_ = s.streamRun(ctx, sw, result.run.ID, conv.ID, 0)
}

// systemPromptVersion 提示版本。改动 systemPrompt 语义时应递增；
// 值会快照进 Run，避免换了提示却仍声称同一实验条件。
const systemPromptVersion = "v4"

func (s *Service) ensureConversation(ctx context.Context, pid, uid store.UUID, idStr, firstMsg string) (*store.AgentConversation, error) {
	if idStr != "" {
		var conv store.AgentConversation
		err := s.DB.WithContext(ctx).
			Where("id = ? AND project_id = ? AND user_id = ?", store.UUID{UUID: uuid.MustParse(idStr)}, pid, uid).
			First(&conv).Error
		if err != nil {
			return nil, err
		}
		return &conv, nil
	}
	title := strings.TrimSpace(firstMsg)
	if len([]rune(title)) > 40 {
		title = string([]rune(title)[:40]) + "…"
	}
	conv := store.AgentConversation{
		ID: store.MustNewUUID(), ProjectID: pid, UserID: uid, Title: title,
	}
	if err := s.DB.WithContext(ctx).Create(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

type historyMessage struct {
	message store.AgentMessage
	parts   []store.AgentMessagePart
}

type historyRow struct {
	store.AgentMessage `gorm:"embedded"`
	PartPresent        *store.UUID            `gorm:"column:part_present"`
	Part               store.AgentMessagePart `gorm:"embedded;embeddedPrefix:part_"`
}

func (s *Service) readHistory(ctx context.Context, convID store.UUID) ([]historyMessage, error) {
	var rows []historyRow
	// One statement keeps message metadata and parts from the same database snapshot.
	if err := s.DB.WithContext(ctx).Model(&store.AgentMessage{}).
		Select(`agent_messages.*, agent_message_parts.id AS part_present,
			agent_message_parts.id AS part_id, agent_message_parts.message_id AS part_message_id,
			agent_message_parts.ordinal AS part_ordinal, agent_message_parts.type AS part_type,
			agent_message_parts.status AS part_status, agent_message_parts.text_content AS part_text_content,
			agent_message_parts.content_json AS part_content_json, agent_message_parts.created_at AS part_created_at,
			agent_message_parts.completed_at AS part_completed_at`).
		Joins("LEFT JOIN agent_message_parts ON agent_message_parts.message_id = agent_messages.id").
		Where("agent_messages.conversation_id = ? AND agent_messages.deleted_at IS NULL", convID).
		Order("agent_messages.created_at ASC, agent_messages.id ASC, agent_message_parts.ordinal ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]historyMessage, 0)
	for _, row := range rows {
		if len(out) == 0 || out[len(out)-1].message.ID != row.ID {
			out = append(out, historyMessage{message: row.AgentMessage})
		}
		if row.PartPresent != nil {
			last := &out[len(out)-1]
			last.parts = append(last.parts, row.Part)
		}
	}
	return out, nil
}

func (s *Service) loadHistory(ctx context.Context, convID UUID0) ([]*schema.Message, error) {
	msgs, err := s.readHistory(ctx, convID)
	if err != nil {
		return nil, err
	}
	out := []*schema.Message{{
		Role:    schema.System,
		Content: systemPrompt,
	}}
	for _, m := range msgs {
		role := schema.User
		if m.message.Role == "assistant" {
			role = schema.Assistant
		}
		var content strings.Builder
		for _, p := range m.parts {
			if p.Type == PartText && p.TextContent != nil {
				content.WriteString(*p.TextContent)
			}
		}
		out = append(out, &schema.Message{Role: role, Content: content.String()})
	}
	return out, nil
}

func friendlyErr(err error) string {
	msg := err.Error()
	if errors.Is(err, adk.ErrExceedMaxIterations) {
		return "本次执行达到迭代上限；可发送“继续”让助手接着处理未完成的部分。"
	}
	if strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") {
		return "模型 API Key 无效或已过期"
	}
	if strings.Contains(msg, "429") {
		return "模型调用达到限额，请稍后重试"
	}
	// Provider error bodies may contain credentials; do not persist them.
	return "模型调用失败，请检查模型配置或稍后重试"
}

type UUID0 = store.UUID

func parseUUIDParam(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// NewUUID 测试与构建共用的 UUID 便捷构造。
func NewUUID() store.UUID { return store.MustNewUUID() }
