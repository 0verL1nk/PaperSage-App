package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Mount 注册检查域路由（挂在 /api/v1，均需登录）。
func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, s *Service) {
	g := rg.Group("", authMw)
	g.GET("/projects/:project_id/review/options", s.optionsHandler)
	g.POST("/projects/:project_id/review-runs", s.createRunHandler)
	g.GET("/projects/:project_id/review-runs", s.listRunsHandler)
	g.GET("/projects/:project_id/review-runs/:run_id", s.getRunHandler)
	g.GET("/projects/:project_id/review-runs/:run_id/events", s.streamRunHandler)
	g.POST("/projects/:project_id/review-runs/:run_id/cancel", s.cancelRunHandler)
}

func (s *Service) optionsHandler(c *gin.Context) {
	pid, ok := projectIDParam(c)
	if !ok {
		return
	}
	out, err := s.Options(c.Request.Context(), auth.CurrentUser(c).ID, pid)
	if err != nil {
		writeReviewError(c, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

func (s *Service) createRunHandler(c *gin.Context) {
	pid, ok := projectIDParam(c)
	if !ok {
		return
	}
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	out, err := s.Create(c.Request.Context(), auth.CurrentUser(c).ID, pid, req)
	if err != nil {
		writeReviewError(c, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

func (s *Service) listRunsHandler(c *gin.Context) {
	pid, ok := projectIDParam(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	out, err := s.List(c.Request.Context(), auth.CurrentUser(c).ID, pid, limit)
	if err != nil {
		writeReviewError(c, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

func (s *Service) getRunHandler(c *gin.Context) {
	pid, rid, ok := runParams(c)
	if !ok {
		return
	}
	out, err := s.Get(c.Request.Context(), auth.CurrentUser(c).ID, pid, rid)
	if err != nil {
		writeReviewError(c, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

func (s *Service) cancelRunHandler(c *gin.Context) {
	pid, rid, ok := runParams(c)
	if !ok {
		return
	}
	out, err := s.Cancel(c.Request.Context(), auth.CurrentUser(c).ID, pid, rid)
	if err != nil {
		writeReviewError(c, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

// streamRunHandler 用 SSE 推真实进度快照；状态真相在 DB，断线重连即可恢复。
func (s *Service) streamRunHandler(c *gin.Context) {
	pid, rid, ok := runParams(c)
	if !ok {
		return
	}
	uid := auth.CurrentUser(c).ID
	if _, err := s.Get(c.Request.Context(), uid, pid, rid); err != nil {
		writeReviewError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, canFlush := c.Writer.(http.Flusher)
	if !canFlush {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "当前环境不支持实时进度，请刷新页面重试"})
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		dto, err := s.Get(c.Request.Context(), uid, pid, rid)
		if err != nil {
			// SSE 写入是尽力而为：客户端断开就结束，错误没有可恢复动作。
			_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: {\"info\":\"读取检查进度失败\"}\n\n")
			flusher.Flush()
			return
		}
		payload, _ := json.Marshal(dto)
		_, _ = fmt.Fprintf(c.Writer, "event: snapshot\ndata: %s\n\n", payload)
		flusher.Flush()
		if isTerminalRun(dto.Status) {
			_, _ = fmt.Fprint(c.Writer, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(c.Writer, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func isTerminalRun(status string) bool {
	switch status {
	case store.ReviewRunSucceeded, store.ReviewRunPartial, store.ReviewRunFailed, store.ReviewRunCancelled:
		return true
	}
	return false
}

func projectIDParam(c *gin.Context) (store.UUID, bool) {
	pid, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return store.UUID{}, false
	}
	return store.UUID{UUID: pid}, true
}

func runParams(c *gin.Context) (store.UUID, store.UUID, bool) {
	pid, ok := projectIDParam(c)
	if !ok {
		return store.UUID{}, store.UUID{}, false
	}
	rid, err := uuid.Parse(c.Param("run_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return store.UUID{}, store.UUID{}, false
	}
	return pid, store.UUID{UUID: rid}, true
}

func writeReviewError(c *gin.Context, err error) {
	var apiErr *httpapi.APIError
	var active *ActiveRunError
	switch {
	case errors.As(err, &apiErr):
		httpapi.WriteError(c.Writer, apiErr)
	case errors.As(err, &active):
		httpapi.WriteError(c.Writer, &httpapi.APIError{
			Code: 409, Info: "已有正在进行的检查（" + active.RunID + "），请等待完成或先取消",
		})
	case errors.Is(err, ErrProjectNotFound):
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "项目不存在或无权访问"})
	case errors.Is(err, ErrRunNotFound):
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "检查任务不存在"})
	case errors.Is(err, context.Canceled):
		// 客户端已断开，不再写响应。
		return
	default:
		slog.Error("review request failed", "err", err)
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "检查服务暂时不可用，请稍后重试"})
	}
}
