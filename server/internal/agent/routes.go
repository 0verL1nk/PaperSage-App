package agent

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Mount 注册 agent 域路由（挂在 /api/v1，均需登录）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("", authMw)

	g.POST("/projects/:project_id/agent/chat", s.chat)
	g.GET("/projects/:project_id/agent/conversations", s.listConversations)
	g.GET("/agent/conversations/:conversation_id/messages", s.listMessages)
	g.DELETE("/agent/conversations/:conversation_id", s.deleteConversation)
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
	if err := s.DB.WithContext(ctx).
		Table("agent_conversations").
		Select("id", "title").
		Where("project_id = ? AND user_id = ?", pid, uid).
		Order("updated_at DESC").Limit(50).Find(&convs).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(convs, ""))
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
	// 归属校验：会话属于当前用户
	var n int64
	_ = s.DB.WithContext(ctx).Table("agent_conversations").
		Where("id = ? AND user_id = ?", cid, uid).Count(&n).Error
	if n == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
		return
	}
	var msgs []struct {
		ID         string  `json:"id"`
		Role       string  `json:"role"`
		Content    string  `json:"content"`
		ToolEvents *string `json:"toolEvents"`
		CreatedAt  string  `json:"createdAt"`
	}
	if err := s.DB.WithContext(ctx).
		Table("agent_messages").
		Select("id", "role", "content", "tool_events", "created_at").
		Where("conversation_id = ?", cid).
		Order("created_at ASC").Limit(200).Find(&msgs).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(msgs, ""))
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
	// 消息随 FK 级联删除
	if err := s.DB.WithContext(ctx).Exec(
		"DELETE FROM agent_conversations WHERE id = ? AND user_id = ?", cid, uid).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"deleted": true}, ""))
}
