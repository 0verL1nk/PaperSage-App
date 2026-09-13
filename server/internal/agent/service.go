package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
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
}

func NewService(db *gorm.DB, stg storage.Storage, q *latex.Queue, cfg *config.Settings, rg *rag.Store) *Service {
	return &Service{DB: db, Storage: stg, Queue: q, Cfg: cfg, RAG: rg}
}

// sseEvent 流给前端的事件形状。
type sseEvent struct {
	Type           string `json:"type"` // delta|tool_call|tool_result|done|error
	Delta          string `json:"delta,omitempty"`
	Tool           string `json:"tool,omitempty"`
	Args           string `json:"args,omitempty"`
	Result         string `json:"result,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	Message        string `json:"message,omitempty"`
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
	chatModel, err := NewChatModel(ctx, resolved)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}

	// 会话（新建或续接，校验归属）
	conv, err := s.ensureConversation(ctx, store.UUID{UUID: pid}, uid, req.ConversationID, req.Content)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
		return
	}
	// 用户消息先落库
	msg := store.AgentMessage{
		ID: store.MustNewUUID(), ConversationID: conv.ID, Role: "user", Content: req.Content,
	}
	if err := s.DB.WithContext(ctx).Create(&msg).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}

	// 历史消息（含本轮）→ schema.Messages
	history, err := s.loadHistory(ctx, conv.ID)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}

	// 工具 + Agent（ReAct）。search_references 是 agentic search：内部
	// 子 agent 多跳检索（rag_search/wiki 读写）
	tools, err := ProjectTools(s.DB, s.Storage, s.Queue, store.UUID{UUID: pid}, uid)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	if s.RAG != nil {
		if tcm, ok := chatModel.(model.ToolCallingChatModel); ok {
			if st, err := SearchTool(ctx, s.DB, s.RAG, store.UUID{UUID: pid}, uid, tcm); err == nil {
				tools = append(tools, st)
			}
		}
	}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "papersage-writing-assistant",
		Description: "论文写作助手（可读写项目文件与文献、触发编译）",
		Instruction: systemPrompt,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
		},
	})
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})

	// SSE 头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "streaming unsupported"})
		return
	}
	writeEvent := func(e sseEvent) {
		b, _ := json.Marshal(e)
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		flusher.Flush()
	}
	writeEvent(sseEvent{Type: "meta", ConversationID: conv.ID.String()})

	// 流式执行（eino ADK 事件模型：工具调用挂在 assistant 消息的
	// ToolCalls 上，结果以 Role=Tool 的消息出现；Next() 返回 (ev, ok)）
	var final strings.Builder
	type toolEvt struct {
		Tool   string `json:"tool"`
		Args   string `json:"args"`
		Result string `json:"result"`
	}
	var toolEvents []toolEvt
	toolNames := map[string]string{} // callID → tool 名（跨消息关联）

	handleMsg := func(m *schema.Message) {
		if m == nil {
			return
		}
		switch m.Role {
		case schema.Assistant:
			for _, tc := range m.ToolCalls {
				toolNames[tc.ID] = tc.Function.Name
				writeEvent(sseEvent{Type: "tool_call", Tool: tc.Function.Name, Args: compactStr(tc.Function.Arguments)})
			}
			if m.Content != "" {
				final.WriteString(m.Content)
				writeEvent(sseEvent{Type: "delta", Delta: m.Content})
			}
		case schema.Tool:
			name := toolNames[m.ToolCallID]
			if name == "" {
				name = "tool"
			}
			res := m.Content
			if len(res) > 2000 {
				res = res[:2000] + "…"
			}
			writeEvent(sseEvent{Type: "tool_result", Tool: name, Result: res})
			toolEvents = append(toolEvents, toolEvt{Tool: name, Result: res})
		}
	}

	iter := runner.Run(ctx, history) // 流式开关在 RunnerConfig.EnableStreaming
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			writeEvent(sseEvent{Type: "error", Message: friendlyErr(ev.Err)})
			s.finishAssistant(ctx, conv.ID, final.String(), toolEvents)
			return
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mo := ev.Output.MessageOutput
		if mo.IsStreaming && mo.MessageStream != nil {
			for {
				chunk, err := mo.MessageStream.Recv()
				if err != nil || chunk == nil {
					break
				}
				// 工具调用也可能出现在流式 chunk 上
				for _, tc := range chunk.ToolCalls {
					toolNames[tc.ID] = tc.Function.Name
					writeEvent(sseEvent{Type: "tool_call", Tool: tc.Function.Name, Args: compactStr(tc.Function.Arguments)})
				}
				if chunk.Content == "" {
					continue
				}
				final.WriteString(chunk.Content)
				writeEvent(sseEvent{Type: "delta", Delta: chunk.Content})
			}
		} else {
			handleMsg(mo.Message)
		}
	}

	if final.Len() == 0 && len(toolEvents) == 0 {
		writeEvent(sseEvent{Type: "error", Message: "模型未返回内容，请重试"})
		return
	}
	s.finishAssistant(ctx, conv.ID, final.String(), toolEvents)
	writeEvent(sseEvent{Type: "done"})
}

// finishAssistant 落库助手消息（云端会话状态）。
func (s *Service) finishAssistant(ctx context.Context, convID UUID0, content string, toolEvents any) {
	var te datatypes.JSON
	if b, err := json.Marshal(toolEvents); err == nil && len(b) > 4 {
		te = b
	}
	msg := store.AgentMessage{
		ID: store.MustNewUUID(), ConversationID: convID,
		Role: "assistant", Content: content, ToolEvents: te,
	}
	_ = s.DB.WithContext(ctx).Create(&msg).Error
	_ = s.DB.WithContext(ctx).Exec(
		"UPDATE agent_conversations SET updated_at = now() WHERE id = ?", convID.String()).Error
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
	if err := s.DB.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("created_at ASC").Limit(50).Find(&msgs).Error; err != nil {
		return nil, err
	}
	out := []*schema.Message{{
		Role:    schema.System,
		Content: systemPrompt,
	}}
	for _, m := range msgs {
		role := schema.User
		if m.Role == "assistant" {
			role = schema.Assistant
		}
		out = append(out, &schema.Message{Role: role, Content: m.Content})
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

func compactStr(s string) string {
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

type UUID0 = store.UUID

func parseUUIDParam(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// NewUUID 测试与构建共用的 UUID 便捷构造。
func NewUUID() store.UUID { return store.MustNewUUID() }
