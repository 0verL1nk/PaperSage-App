package agent

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// createRun POST /agent/conversations/:conversation_id/runs
//
// 参考架构模式 B：先建 Run 再订阅事件。Run 立刻返回、独立于连接执行，
// 因此客户端可以在响应到达前就断开，也可以稍后另开连接订阅。
func (s *Service) createRunHandler(c *gin.Context) {
	ctx, release, accepted := s.acceptRun(c.Request.Context())
	if !accepted {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusServiceUnavailable, Info: "服务正在关闭，请稍后重试"})
		return
	}
	defer release()
	uid := auth.CurrentUser(c).ID
	cidU, err := parseUUIDParam(c.Param("conversation_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	cid := store.UUID{UUID: cidU}

	var req struct {
		Content               string `json:"content"`
		ClientRequestID       string `json:"clientRequestId"`
		ExpectedThreadVersion *int64 `json:"expectedThreadVersion"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	// 会话归属（走 Model 才会带上软删过滤）
	var conv store.AgentConversation
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ?", cid, uid).First(&conv).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Conversation not found"})
		return
	}

	// 乐观锁：两个标签页/设备并发提交时，后到的必须重新决策而不是静默覆盖。
	// 冲突响应带上服务端最新版本，前端可直接提示并允许用户重试。
	if req.ExpectedThreadVersion != nil && *req.ExpectedThreadVersion != conv.Version {
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusConflict, httpapi.Envelope{
			Code: http.StatusConflict,
			Data: gin.H{"threadVersion": conv.Version},
			Info: "会话已在其他位置被修改",
		})
		return
	}

	// 未配置模型在创建时就 400（而不是等流里才报错），前端据此展示设置入口
	resolved, err := ResolveModelWithEnv(ctx, s.DB, uid, s.Cfg)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{
			Code: http.StatusBadRequest,
			Info: "尚未配置 AI 模型：请在设置 → AI 助手中添加 Provider，或由管理员配置服务端默认模型",
		})
		return
	}

	turn, err := s.ensureTurn(ctx, cid, req.ClientRequestID)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	// 幂等：该 Turn 已有 Run 时直接返回既有结果，不再建 Run、更不再跑一次模型。
	// （只复用 Turn 而不复用 Run 会让"幂等键"名存实亡。）
	if turn.RootRunID != nil {
		var existing store.AgentRun
		if e := s.DB.WithContext(ctx).Where("id = ?", *turn.RootRunID).First(&existing).Error; e == nil {
			resp := gin.H{
				"turnId":        turn.ID.String(),
				"runId":         existing.ID.String(),
				"streamUrl":     "/api/v1/agent/runs/" + existing.ID.String() + "/events",
				"threadVersion": conv.Version,
			}
			if turn.UserMessageID != nil {
				resp["userMessageId"] = turn.UserMessageID.String()
			}
			_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(resp, ""))
			return
		}
	}
	userMsg, err := s.createUserMessage(ctx, s.DB, cid, turn.ID, req.Content)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	run, err := s.createRun(ctx, cid, turn.ID, AgentKeyMain,
		resolved.Source, resolved.Model, systemPromptVersion)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	rc := runContext{RunID: run.ID, ConversationID: cid,
		ProjectID: conv.ProjectID, UserID: uid, Content: req.Content}
	started := false
	defer func() {
		if !started {
			s.finishUnstartedRun(rc)
		}
	}()

	// 回填 Turn 的指针并推进会话版本（同一事务，避免半步状态）
	newVersion := conv.Version + 1
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&store.AgentTurn{}).Where("id = ?", turn.ID).
			Updates(map[string]any{"user_message_id": userMsg.ID, "root_run_id": run.ID}).Error; e != nil {
			return e
		}
		return tx.Model(&store.AgentConversation{}).Where("id = ?", cid).
			Update("version", newVersion).Error
	})
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = s.appendEvent(ctx, cid, run.ID, evtRunStarted, nil)

	started = true // StartRun owns terminal persistence even when shutdown rejects it.
	if err := s.StartRun(rc); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusServiceUnavailable, Info: "服务正在关闭，请稍后重试"})
		return
	}

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"turnId":        turn.ID.String(),
		"runId":         run.ID.String(),
		"userMessageId": userMsg.ID.String(),
		"streamUrl":     "/api/v1/agent/runs/" + run.ID.String() + "/events",
		"threadVersion": newVersion,
	}, ""))
}

// subscribeRun GET /agent/runs/:run_id/events?afterSeq=N
// 先补历史（断线重放），再跟随实时事件。
func (s *Service) subscribeRun(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	rid, err := parseUUIDParam(c.Param("run_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	runID := store.UUID{UUID: rid}

	run, _, err := s.ownedRun(ctx, runID, uid)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Run not found"})
		return
	}
	afterSeq := int64(0)
	if v := c.Query("afterSeq"); v != "" {
		if n, e := strconv.ParseInt(v, 10, 64); e == nil && n > 0 {
			afterSeq = n
		}
	}

	sw, ok := newStreamWriter(c)
	if !ok {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "streaming unsupported"})
		return
	}
	_ = s.streamRun(ctx, sw, runID, run.ConversationID, afterSeq)
}

// cancelRun POST /agent/runs/:run_id/cancel
func (s *Service) cancelRun(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	rid, err := parseUUIDParam(c.Param("run_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	runID := store.UUID{UUID: rid}
	if _, _, err := s.ownedRun(ctx, runID, uid); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Run not found"})
		return
	}
	cancelled := s.CancelRun(runID)
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK,
		httpapi.Success(gin.H{"cancelled": cancelled}, ""))
}

// retryRun POST /agent/runs/:run_id/retry
// 复用同一个 Turn 新建一个 Run（重试不新增用户消息）。
func (s *Service) retryRun(c *gin.Context) {
	ctx, release, accepted := s.acceptRun(c.Request.Context())
	if !accepted {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusServiceUnavailable, Info: "服务正在关闭，请稍后重试"})
		return
	}
	defer release()
	uid := auth.CurrentUser(c).ID
	rid, err := parseUUIDParam(c.Param("run_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	runID := store.UUID{UUID: rid}
	old, projID, err := s.ownedRun(ctx, runID, uid)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Run not found"})
		return
	}
	if old.TurnID == nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 400, Info: "Run has no turn"})
		return
	}

	// 取回该 Turn 的用户输入（内容在 parts 里）
	var content string
	var turn store.AgentTurn
	if err := s.DB.WithContext(ctx).Where("id = ?", *old.TurnID).First(&turn).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Turn not found"})
		return
	}
	if turn.UserMessageID != nil {
		var parts []store.AgentMessagePart
		_ = s.DB.WithContext(ctx).
			Where("message_id = ? AND type = ?", *turn.UserMessageID, PartText).
			Order("ordinal ASC").Find(&parts).Error
		for _, p := range parts {
			if p.TextContent != nil {
				content += *p.TextContent
			}
		}
	}

	resolved, rerr := ResolveModelWithEnv(ctx, s.DB, uid, s.Cfg)
	if rerr != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusBadRequest,
			Info: "尚未配置 AI 模型：请在设置 → AI 助手中添加 Provider，或由管理员配置服务端默认模型"})
		return
	}
	run, err := s.createRun(ctx, old.ConversationID, *old.TurnID, AgentKeyMain,
		resolved.Source, resolved.Model, systemPromptVersion)
	if err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	_ = s.appendEvent(ctx, old.ConversationID, run.ID, evtRunStarted, nil)
	if err := s.StartRun(runContext{
		RunID: run.ID, ConversationID: old.ConversationID,
		ProjectID: projID, UserID: uid, Content: content,
	}); err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusServiceUnavailable, Info: "服务正在关闭，请稍后重试"})
		return
	}

	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{
		"runId":     run.ID.String(),
		"streamUrl": "/api/v1/agent/runs/" + run.ID.String() + "/events",
	}, ""))
}

// getArtifact GET /agent/artifacts/:artifact_id
// 取回大结果的完整内容（工具结果超阈值时已存为 artifact）。
func (s *Service) getArtifact(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	aid, err := parseUUIDParam(c.Param("artifact_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var art store.AgentArtifact
	if err := s.DB.WithContext(ctx).Where("id = ?", store.UUID{UUID: aid}).First(&art).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Artifact not found"})
		return
	}
	var n int64
	_ = s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", art.ProjectID, uid).Count(&n).Error
	if n == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Artifact not found"})
		return
	}
	b, gerr := s.Storage.Get(ctx, art.S3Key)
	if gerr != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	c.Data(http.StatusOK, art.Mime, b)
}

// ownedRun 取出属于当前用户的 Run（经会话归属校验），并带回其所属项目 id。
func (s *Service) ownedRun(ctx context.Context, runID, uid store.UUID) (*store.AgentRun, store.UUID, error) {
	var run store.AgentRun
	if err := s.DB.WithContext(ctx).Where("id = ?", runID).First(&run).Error; err != nil {
		return nil, store.UUID{}, err
	}
	var conv store.AgentConversation
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ?", run.ConversationID, uid).First(&conv).Error; err != nil {
		return nil, store.UUID{}, errors.New("run not owned")
	}
	return &run, conv.ProjectID, nil
}
