package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/dto"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Mount 注册 agent 域路由（挂在 /api/v1，均需登录）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("", authMw)

	g.POST("/projects/:project_id/agent/chat", s.chat)
	g.GET("/projects/:project_id/agent/conversations", s.listConversations)
	g.POST("/projects/:project_id/agent/conversations", s.createConversation)
	g.GET("/projects/:project_id/agent/proposals", s.listProposals)
	g.GET("/agent/conversations/:conversation_id/messages", s.listMessages)
	g.DELETE("/agent/conversations/:conversation_id", s.deleteConversation)

	// 模式 B：先建 Run 再订阅（Run 独立于连接，可取消、可重试、可断线恢复）
	g.POST("/agent/conversations/:conversation_id/runs", s.createRunHandler)
	g.GET("/agent/runs/:run_id/events", s.subscribeRun)
	g.POST("/agent/runs/:run_id/cancel", s.cancelRun)
	g.POST("/agent/runs/:run_id/retry", s.retryRun)

	// 审批与建议：审批是工具调用状态机的入口，应用/驳回是用户可见的动作，
	// 两者共用同一条决策路径（internal/agent/proposals.go）。
	g.POST("/agent/tool-calls/:tool_call_id/approval", s.approveToolCall)
	g.POST("/agent/change-proposals/:proposal_id/apply", s.decideProposalHandler(true))
	g.POST("/agent/change-proposals/:proposal_id/reject", s.decideProposalHandler(false))
	g.GET("/agent/artifacts/:artifact_id", s.getArtifact)
}

func (s *Service) listConversations(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	pidU, err := parseUUIDParam(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	pid := store.UUID{UUID: pidU} // 走自定义 Valuer（hex 文本列）
	var convs []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	// 走 Model 而非 Table：后者没有挂模型，GORM 的软删过滤不会生效，
	// 已删会话会被一并列出。
	if err := s.DB.WithContext(ctx).
		Model(&store.AgentConversation{}).
		Select("id", "title").
		Where("project_id = ? AND user_id = ?", pid, uid).
		Order("updated_at DESC").Limit(50).Find(&convs).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	// 列里存的是 32 位无横线 hex（SQLite 兼容形状），但本域其余响应一律用
	// 带横线的规范形式（run/turn/artifact 都是 .String()）。直接扫成 string 会
	// 让同一个 id 在列表与创建两个接口里长得不一样，这里统一回规范形式。
	for i := range convs {
		if u, perr := uuid.Parse(convs[i].ID); perr == nil {
			convs[i].ID = u.String()
		}
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(convs, ""))
}

// createConversation POST /projects/:project_id/agent/conversations
//
// 「会话在用户发出第一条消息时诞生」是 ensureConversation 的既有语义；但模式 B 的
// Run 路径要求会话已存在，面板因此需要一个显式的建会话入口——否则新项目的第一句话
// 没有地方可去（前端本地回显后请求无处发送，用户看到输入被清空却什么都没发生）。
func (s *Service) createConversation(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	pidU, err := parseUUIDParam(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	pid := store.UUID{UUID: pidU}

	// 归属校验：只能在自己的项目下建会话
	var n int64
	if err := s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", pid, uid).Count(&n).Error; err != nil || n == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found"})
		return
	}

	// body 可空：面板不传标题时用默认值，与 ensureConversation 的截断规则一致
	var req struct {
		Title string `json:"title"`
	}
	_ = c.ShouldBindJSON(&req)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "新对话"
	}
	if r := []rune(title); len(r) > 40 {
		title = string(r[:40]) + "…"
	}

	conv := store.AgentConversation{ID: store.MustNewUUID(), ProjectID: pid, UserID: uid, Title: title}
	if err := s.DB.WithContext(ctx).Create(&conv).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"id": conv.ID.String(), "title": conv.Title, "version": conv.Version,
	}, ""))
}

func (s *Service) listMessages(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	cidU, err := parseUUIDParam(c.Param("conversation_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	cid := store.UUID{UUID: cidU}
	// 归属校验：会话属于当前用户且未被软删（走 Model 才会带上软删过滤）
	var n int64
	_ = s.DB.WithContext(ctx).Model(&store.AgentConversation{}).
		Where("id = ? AND user_id = ?", cid, uid).Count(&n).Error
	if n == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
		return
	}
	var msgs []store.AgentMessage
	// 走 Model 而非 Table：后者没有挂模型，软删过滤不会生效。
	if err := s.DB.WithContext(ctx).
		Model(&store.AgentMessage{}).
		Where("conversation_id = ?", cid).
		Order("created_at ASC").Limit(200).Find(&msgs).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	out, err := s.messagesWithParts(ctx, msgs)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

// partDTO / msgDTO：消息由 Part 组成，内容不再是一个 content 字段。
// content 直接内联 JSON 对象（而非旧实现的 JSON 字符串），前端无需二次 parse。
type partDTO struct {
	ID          string          `json:"id"`
	Ordinal     int             `json:"ordinal"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Text        *string         `json:"text,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	CreatedAt   dto.JSONTime    `json:"createdAt"`
	CompletedAt *dto.JSONTime   `json:"completedAt,omitempty"`
}

type msgDTO struct {
	ID        string       `json:"id"`
	Role      string       `json:"role"`
	Status    string       `json:"status"`
	RunID     *string      `json:"runId,omitempty"`
	CreatedAt dto.JSONTime `json:"createdAt"`
	Parts     []partDTO    `json:"parts"`
}

// messagesWithParts 读回消息及其 parts（按 ordinal 排序）。
func (s *Service) messagesWithParts(ctx context.Context, msgs []store.AgentMessage) ([]msgDTO, error) {
	out := make([]msgDTO, 0, len(msgs))
	if len(msgs) == 0 {
		return out, nil
	}
	ids := make([]store.UUID, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	var parts []store.AgentMessagePart
	if err := s.DB.WithContext(ctx).
		Where("message_id IN ?", ids).
		Order("ordinal ASC").Find(&parts).Error; err != nil {
		return nil, err
	}
	byMsg := map[store.UUID][]store.AgentMessagePart{}
	for _, p := range parts {
		byMsg[p.MessageID] = append(byMsg[p.MessageID], p)
	}
	for _, m := range msgs {
		d := msgDTO{
			ID: m.ID.String(), Role: m.Role, Status: m.Status,
			CreatedAt: dto.JSONTime{Time: m.CreatedAt}, Parts: []partDTO{},
		}
		if m.RunID != nil {
			rid := m.RunID.String()
			d.RunID = &rid
		}
		for _, p := range byMsg[m.ID] {
			pd := partDTO{
				ID: p.ID.String(), Ordinal: p.Ordinal, Type: p.Type, Status: p.Status,
				Text: p.TextContent, CreatedAt: dto.JSONTime{Time: p.CreatedAt},
			}
			if len(p.ContentJSON) > 0 {
				pd.Content = json.RawMessage(p.ContentJSON)
			}
			if p.CompletedAt != nil {
				pd.CompletedAt = &dto.JSONTime{Time: *p.CompletedAt}
			}
			d.Parts = append(d.Parts, pd)
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Service) deleteConversation(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	cidU, err := parseUUIDParam(c.Param("conversation_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	cid := store.UUID{UUID: cidU}

	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv store.AgentConversation
		if err := tx.Where("id = ? AND user_id = ?", cid, uid).First(&conv).Error; err != nil {
			return err
		}
		// 软删是 UPDATE，不触发 ON DELETE CASCADE，因此必须在同一事务内
		// 显式软删消息——否则会留下指向已删会话的孤儿消息。
		if err := tx.Where("conversation_id = ?", cid).Delete(&store.AgentMessage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&conv).Error
	})
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
	case err != nil:
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
	default:
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"deleted": true}, ""))
	}
}
