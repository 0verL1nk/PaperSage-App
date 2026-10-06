package agent

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

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
- 修改论文内容前先 read_latex_file 了解现状；修改后建议 compile_project 验证。
- LaTeX 建议保持与现有模板一致的结构（documentclass、宏包、章节层级）。
- 引用文献时遵循 GB/T 7714 规范格式。
- 回复保持精炼：先结论，再展开；代码/公式放代码块。`

// Service — agent 域。依赖均接口化由装配层注入。
type Service struct {
	DB      *gorm.DB
	Storage storage.Storage
	Queue   *latex.Queue
	Cfg     *config.Settings
	RAG     *rag.Store
	// hub 只做进程内唤醒，事件的真相在 agent_events（多实例/重启不丢事件）。
	hub *runHub
	// cancels 登记进行中 Run 的取消函数（POST /runs/:id/cancel）。
	cancelMu sync.Mutex
	cancels  map[store.UUID]context.CancelFunc
	// files 复用 latex 域的文件服务（路径校验 + 存储 + 缓存），应用建议时用。
	// 必须复用同一个实例：FilesService 带自己的 LRU 内容缓存，各建一份会让
	// 同一 S3 key 出现两份缓存、读出陈旧内容。
	files *latex.FilesService
}

func NewService(db *gorm.DB, stg storage.Storage, files *latex.FilesService,
	q *latex.Queue, cfg *config.Settings, rg *rag.Store) *Service {
	return &Service{
		DB: db, Storage: stg, Queue: q, Cfg: cfg, RAG: rg, files: files,
		hub: newRunHub(), cancels: map[store.UUID]context.CancelFunc{},
	}
}

// chat POST /projects/:id/agent/chat {conversation_id?, content} → SSE
func (s *Service) chat(c *gin.Context) {
	ctx := c.Request.Context()
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
		ClientRequestID string `json:"clientRequestId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	// 模型解析（user settings → env）
	resolved, err := ResolveModelWithEnv(ctx, s.DB, uid, s.Cfg)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{
			Code: http.StatusBadRequest,
			Info: "尚未配置 AI 模型：请在设置 → AI 助手中添加 Provider，或由管理员配置服务端默认模型",
		})
		return
	}
	// 模型实例由执行器按 Run 构建（无全局态）；这里只做「是否已配置」的同步校验，
	// 使未配置模型能在创建 Run 的请求上直接 400，而不是等到流里才报错。

	// 会话（新建或续接，校验归属）
	conv, err := s.ensureConversation(ctx, store.UUID{UUID: pid}, uid, req.ConversationID, req.Content)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
		return
	}
	// Turn 幂等 + Run（独立于 HTTP 连接存在）
	turn, err := s.ensureTurn(ctx, conv.ID, req.ClientRequestID)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	run, err := s.createRun(ctx, conv.ID, turn.ID, AgentKeyMain,
		resolved.Source, resolved.Model, systemPromptVersion)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	// 用户消息先落库（文本作为第一个 part）
	if _, err := s.createUserMessage(ctx, s.DB, conv.ID, turn.ID, req.Content); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = s.appendEvent(ctx, conv.ID, run.ID, "run.started", nil)

	// 执行与订阅分离：Run 在独立 goroutine 中跑，HTTP 连接只订阅事件。
	// 客户端断开后 Run 仍会跑完（参考架构 §3.2 模式 B）。
	s.StartRun(runContext{
		RunID:          run.ID,
		ConversationID: conv.ID,
		ProjectID:      store.UUID{UUID: pid},
		UserID:         uid,
		Content:        req.Content,
	})

	sw, ok := newStreamWriter(c)
	if !ok {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "streaming unsupported"})
		return
	}
	_ = s.streamRun(ctx, sw, run.ID, conv.ID, 0)
}

// systemPromptVersion 提示版本。改动 systemPrompt 语义时应递增；
// 值会快照进 Run，避免换了提示却仍声称同一实验条件。
const systemPromptVersion = "v1"

// finishAssistant 落库助手终答并收敛 Run（status 为 RunCompleted / RunFailed）。
func (s *Service) finishAssistant(ctx context.Context, convID, runID store.UUID, content string, toolEvents any, status string) {
	msg, err := s.createAssistantMessage(ctx, s.DB, convID, &runID, content, toolEvents)
	if err != nil {
		s.finishRun(ctx, runID, RunFailed, nil, "persist assistant message failed")
		return
	}
	s.finishRun(ctx, runID, status, &msg.ID, "")
	// 用应用层时间而非 SQL now()：SQLite 没有 now()，且错误会被丢弃（静默失效）。
	_ = s.DB.WithContext(ctx).Model(&store.AgentConversation{}).
		Where("id = ?", convID).Update("updated_at", time.Now().UTC()).Error
}

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

func (s *Service) loadHistory(ctx context.Context, convID UUID0) ([]*schema.Message, error) {
	var msgs []store.AgentMessage
	// 不按条数截断：定长上限会让超过该长度的会话**静默丢掉最近的消息**
	//（本轮刚写入的用户提问也可能取不到），而且会把一次问答从中间劈开。
	// 上下文长度应由 token 预算约束，不是这里的条数魔法数字。
	if err := s.DB.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("created_at ASC").Find(&msgs).Error; err != nil {
		return nil, err
	}
	out := []*schema.Message{{
		Role:    schema.System,
		Content: systemPrompt,
	}}
	if len(msgs) == 0 {
		return out, nil
	}
	// 内容在 parts 里：取每条消息的 text part 按 ordinal 拼接。
	ids := make([]store.UUID, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	var parts []store.AgentMessagePart
	if err := s.DB.WithContext(ctx).
		Where("message_id IN ? AND type = ?", ids, PartText).
		Order("ordinal ASC").Find(&parts).Error; err != nil {
		return nil, err
	}
	byMsg := map[store.UUID]string{}
	for _, p := range parts {
		if p.TextContent != nil {
			byMsg[p.MessageID] += *p.TextContent
		}
	}
	for _, m := range msgs {
		role := schema.User
		if m.Role == "assistant" {
			role = schema.Assistant
		}
		out = append(out, &schema.Message{Role: role, Content: byMsg[m.ID]})
	}
	return out, nil
}

func friendlyErr(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") {
		return "模型 API Key 无效或已过期"
	}
	if strings.Contains(msg, "429") {
		return "模型调用达到限额，请稍后重试"
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return "模型调用失败：" + msg
}

type UUID0 = store.UUID

func parseUUIDParam(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// NewUUID 测试与构建共用的 UUID 便捷构造。
func NewUUID() store.UUID { return store.MustNewUUID() }
