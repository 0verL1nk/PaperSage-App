package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 建议状态。
const (
	ProposalPending  = "pending"
	ProposalApplied  = "applied"
	ProposalRejected = "rejected"
	ProposalStale    = "stale"
)

// linkProposal 把刚产出的建议挂到发起它的工具调用上（args 里带 path）。
// 用 run + path 定位：工具是顺序执行的，同一 Run 内同路径的最新建议即本次产出。
func (s *Service) linkProposal(ctx context.Context, toolCallID, runID store.UUID, args string) {
	var in struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(args), &in) != nil || in.Path == "" {
		return
	}
	_ = s.DB.WithContext(ctx).Model(&store.AgentChangeProposal{}).
		Where("run_id = ? AND file_path = ? AND tool_call_id IS NULL", runID, in.Path).
		Update("tool_call_id", toolCallID).Error
}

// decideProposal 是唯一的决策路径：apply / reject 两个入口共用它。
//
// 应用前必须**重新**校验（参考架构 §7：服务端应用时重新校验权限、基准版本与幂等键）：
//   - 建议仍处于 pending（幂等：重复应用被拒）
//   - 基准内容哈希与稿件当前内容一致（稿件变了则旧建议失效，不得用旧基准覆盖新内容）
func (s *Service) decideProposal(ctx context.Context, userID, projectID, proposalID store.UUID, approve bool) (*store.AgentChangeProposal, int, error) {
	var p store.AgentChangeProposal
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND project_id = ?", proposalID, projectID).First(&p).Error; err != nil {
		return nil, http.StatusNotFound, err
	}
	if p.Status != ProposalPending {
		return &p, http.StatusConflict, errors.New("proposal is not pending")
	}

	now := time.Now().UTC()
	if !approve {
		if err := s.DB.WithContext(ctx).Model(&store.AgentChangeProposal{}).
			Where("id = ? AND status = ?", p.ID, ProposalPending).
			Updates(map[string]any{"status": ProposalRejected, "rejected_at": now}).Error; err != nil {
			return nil, http.StatusInternalServerError, err
		}
		p.Status = ProposalRejected
		s.closeToolCallOf(ctx, p)
		return &p, http.StatusOK, nil
	}

	// 读当前稿件并比对基准版本
	f, content, err := s.readLatexForProposal(ctx, projectID, p.FilePath)
	if err != nil {
		return nil, http.StatusNotFound, err
	}
	_ = f
	sum := md5.Sum([]byte(content))
	if hex.EncodeToString(sum[:]) != p.BaseContentHash {
		// 稿件已变：不得用旧基准写回，标记过期，让用户重新生成
		_ = s.DB.WithContext(ctx).Model(&store.AgentChangeProposal{}).
			Where("id = ? AND status = ?", p.ID, ProposalPending).
			Update("status", ProposalStale).Error
		p.Status = ProposalStale
		return &p, http.StatusConflict, errors.New("base content changed")
	}
	if !strings.Contains(content, p.OldText) {
		return &p, http.StatusConflict, errors.New("anchor no longer present")
	}
	newContent := strings.Replace(content, p.OldText, p.NewText, 1)
	if s.files == nil {
		// 不能静默跳过写入后仍标记为已应用——那会让"应用成功"成为谎报。
		return nil, http.StatusInternalServerError, errors.New("files service unavailable")
	}
	if _, err := s.files.UpdateFile(ctx, projectID.UUID, p.FilePath, newContent, userID.UUID); err != nil {
		return nil, http.StatusInternalServerError, err
	}

	if err := s.DB.WithContext(ctx).Model(&store.AgentChangeProposal{}).
		Where("id = ? AND status = ?", p.ID, ProposalPending).
		Updates(map[string]any{"status": ProposalApplied, "applied_at": now}).Error; err != nil {
		return nil, http.StatusInternalServerError, err
	}
	p.Status = ProposalApplied
	s.closeToolCallOf(ctx, p)
	return &p, http.StatusOK, nil
}

// closeToolCallOf 建议决策后收敛其工具调用的状态机。
// 建议类工具的调用会停留在 awaiting_approval，直到用户做出决定。
func (s *Service) closeToolCallOf(ctx context.Context, p store.AgentChangeProposal) {
	if p.ToolCallID == nil {
		return
	}
	state := ToolApproved
	if p.Status != ProposalApplied {
		state = ToolDenied
	}
	_ = s.DB.WithContext(ctx).Model(&store.AgentToolCall{}).
		Where("id = ?", *p.ToolCallID).
		Updates(map[string]any{"state": state, "completed_at": time.Now().UTC()}).Error
}

func (s *Service) readLatexForProposal(ctx context.Context, projectID store.UUID, filePath string) (*store.LatexFile, string, error) {
	var f store.LatexFile
	if err := s.DB.WithContext(ctx).
		Where("project_id = ? AND file_path = ?", projectID, filePath).First(&f).Error; err != nil {
		return nil, "", err
	}
	if f.S3Key == nil {
		return &f, "", nil
	}
	b, err := s.Storage.Get(ctx, *f.S3Key)
	if err != nil {
		return nil, "", err
	}
	return &f, string(b), nil
}

// ── HTTP ────────────────────────────────────────────────────────────

// latestProposalOfCall 取某次工具调用产出的建议。
func (s *Service) latestProposalOfCall(ctx context.Context, toolCallID store.UUID) (*store.AgentChangeProposal, error) {
	var p store.AgentChangeProposal
	if err := s.DB.WithContext(ctx).Where("tool_call_id = ?", toolCallID).
		Order("created_at DESC").First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// proposalPayload 推给前端的建议形状（含 diff 锚点与状态）。
func proposalPayload(p *store.AgentChangeProposal) gin.H {
	h := gin.H{
		"id": p.ID.String(), "filePath": p.FilePath,
		"oldText": p.OldText, "newText": p.NewText, "status": p.Status,
	}
	if p.Reason != nil {
		h["reason"] = *p.Reason
	}
	return h
}

// listProposals GET /projects/:project_id/agent/proposals?status=pending
// 刷新后仍要看得到未决建议——流里推过的 part 不提供持久列表。
func (s *Service) listProposals(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	pid, err := parseUUIDParam(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	projectID := store.UUID{UUID: pid}
	var n int64
	_ = s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", projectID, uid).Count(&n).Error
	if n == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Project not found"})
		return
	}
	q := s.DB.WithContext(ctx).Where("project_id = ?", projectID)
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	var rows []store.AgentChangeProposal
	if err := q.Order("created_at DESC").Find(&rows).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 500, Info: "Internal server error"})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for i := range rows {
		out = append(out, proposalPayload(&rows[i]))
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(out, ""))
}

// ownedProposalProject 校验建议归属（经项目 owner）。
func (s *Service) ownedProposalProject(ctx context.Context, proposalID, uid store.UUID) (store.UUID, error) {
	var p store.AgentChangeProposal
	if err := s.DB.WithContext(ctx).Where("id = ?", proposalID).First(&p).Error; err != nil {
		return store.UUID{}, err
	}
	var n int64
	if err := s.DB.WithContext(ctx).Model(&store.Project{}).
		Where("id = ? AND owner_id = ?", p.ProjectID, uid).Count(&n).Error; err != nil {
		return store.UUID{}, err
	}
	if n == 0 {
		return store.UUID{}, errors.New("not owned")
	}
	return p.ProjectID, nil
}

// decideProposalHandler 供 apply / reject 两个路由复用。
func (s *Service) decideProposalHandler(approve bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		uid := auth.CurrentUser(c).ID
		pid, err := parseUUIDParam(c.Param("proposal_id"))
		if err != nil {
			httpapi.WriteError(c.Writer, httpapi.ErrValidation)
			return
		}
		proposalID := store.UUID{UUID: pid}
		projectID, err := s.ownedProposalProject(ctx, proposalID, uid)
		if err != nil {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Proposal not found"})
			return
		}
		p, code, derr := s.decideProposal(ctx, uid, projectID, proposalID, approve)
		if derr != nil && code != http.StatusConflict {
			httpapi.WriteError(c.Writer, &httpapi.APIError{Code: code, Info: "Internal server error"})
			return
		}
		info := ""
		if code == http.StatusConflict {
			info = "建议已失效（稿件已变化或已处理），请重新生成"
		}
		_ = httpapi.WriteEnvelope(c.Writer, code, httpapi.Envelope{
			Code: code, Data: gin.H{"status": p.Status}, Info: info,
		})
	}
}

// approveToolCall POST /agent/tool-calls/:id/approval
//
// 状态机入口（参考架构 §8.3）：审批绑定参数哈希。本项目的审批型工具
// 都是「产出建议」，故批准即应用其建议、拒绝即驳回；未关联建议的工具调用
// 只做状态流转。
func (s *Service) approveToolCall(c *gin.Context) {
	ctx := c.Request.Context()
	uid := auth.CurrentUser(c).ID
	tid, err := parseUUIDParam(c.Param("tool_call_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var req struct {
		Decision      string `json:"decision"` // approved | denied
		ArgumentsHash string `json:"argumentsHash"`
		Comment       string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	approve := req.Decision == ToolApproved
	callID := store.UUID{UUID: tid}

	var tc store.AgentToolCall
	if err := s.DB.WithContext(ctx).Where("id = ?", callID).First(&tc).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Tool call not found"})
		return
	}
	// 归属：经 Run → 会话 → 用户
	var run store.AgentRun
	if err := s.DB.WithContext(ctx).Where("id = ?", tc.RunID).First(&run).Error; err != nil {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Tool call not found"})
		return
	}
	var n int64
	_ = s.DB.WithContext(ctx).Model(&store.AgentConversation{}).
		Where("id = ? AND user_id = ?", run.ConversationID, uid).Count(&n).Error
	if n == 0 {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: 404, Info: "Tool call not found"})
		return
	}

	// 审批绑定参数哈希：参数变了旧审批自动失效
	if req.ArgumentsHash != "" && tc.ArgumentsHash != nil && req.ArgumentsHash != *tc.ArgumentsHash {
		httpapi.WriteError(c.Writer, &httpapi.APIError{Code: http.StatusConflict,
			Info: "调用参数已变化，请重新请求审批"})
		return
	}

	// 幂等：已决策过的调用不再处理
	if tc.State != ToolAwaiting {
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK,
			httpapi.Success(gin.H{"state": tc.State, "alreadyDecided": true}, ""))
		return
	}

	var p store.AgentChangeProposal
	if err := s.DB.WithContext(ctx).Where("tool_call_id = ?", tc.ID).First(&p).Error; err == nil {
		_, code, derr := s.decideProposal(ctx, uid, p.ProjectID, p.ID, approve)
		if derr != nil && code == http.StatusConflict {
			_ = httpapi.WriteEnvelope(c.Writer, http.StatusConflict, httpapi.Envelope{
				Code: http.StatusConflict, Data: gin.H{"status": ProposalStale},
				Info: "建议已失效（稿件已变化）",
			})
			return
		}
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"state": ToolApproved}, ""))
		return
	}

	state := ToolDenied
	if approve {
		state = ToolApproved
	}
	_ = s.DB.WithContext(ctx).Model(&store.AgentToolCall{}).Where("id = ?", tc.ID).
		Update("state", state).Error
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(gin.H{"state": state}, ""))
}
