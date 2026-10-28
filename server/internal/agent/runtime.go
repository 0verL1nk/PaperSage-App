package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Part 类型。词汇对齐 AI SDK UI Message Stream Protocol，
// 使前端 reducer 与工具/审批卡片有现成参照。
const (
	PartText     = "text"
	PartTool     = "tool"
	PartActivity = "activity"
	PartApproval = "approval"
	PartCitation = "citation"
	PartArtifact = "artifact"
	PartError    = "error"
)

// Run 状态。
const (
	RunRunning   = "running"
	RunCompleted = "completed"
	RunFailed    = "failed"
	RunCancelled = "cancelled"
)

// AgentKeyMain 主对话 agent 的标识；子 agent 用各自的 key 挂子 Run。
const AgentKeyMain = "main"

// ── Turn / Run ──────────────────────────────────────────────────────

// ensureTurn 按 clientRequestId 幂等创建 Turn：重复提交（双击、网络重试）
// 返回既有 Turn，不产生第二次模型调用。clientRequestID 为空时每次都新建。
func (s *Service) ensureTurn(ctx context.Context, convID store.UUID, clientRequestID string) (*store.AgentTurn, error) {
	if clientRequestID != "" {
		var t store.AgentTurn
		err := s.DB.WithContext(ctx).
			Where("conversation_id = ? AND client_request_id = ?", convID, clientRequestID).
			First(&t).Error
		if err == nil {
			return &t, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	t := store.AgentTurn{ID: store.MustNewUUID(), ConversationID: convID}
	if clientRequestID != "" {
		t.ClientRequestID = &clientRequestID
	}
	if err := s.DB.WithContext(ctx).Create(&t).Error; err != nil {
		// 并发下另一请求已建好：读回即可
		if clientRequestID != "" {
			var again store.AgentTurn
			if e := s.DB.WithContext(ctx).
				Where("conversation_id = ? AND client_request_id = ?", convID, clientRequestID).
				First(&again).Error; e == nil {
				return &again, nil
			}
		}
		return nil, err
	}
	return &t, nil
}

// createRun 建一个 Run 并快照模型与提示版本。
// Run 独立于 HTTP 连接存在：可查询、可取消、可恢复。
func (s *Service) createRun(ctx context.Context, convID, turnID store.UUID, agentKey, provider, model, promptVersion string) (*store.AgentRun, error) {
	now := time.Now().UTC()
	run := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: convID,
		AgentKey: agentKey, Status: RunRunning,
		ModelProvider: provider, ModelName: model, PromptVersion: promptVersion,
		StartedAt: &now,
	}
	run.TurnID = &turnID
	if err := s.DB.WithContext(ctx).Create(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// appendEvent 追加一条执行事实记录，sequence 在 Run 内单调递增。
// sequence 留在服务端（对外用 AI SDK 协议），使将来可平滑切到增量补拉。
func (s *Service) appendEvent(ctx context.Context, convID, runID store.UUID, eventType string, payload any) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.appendEventTx(ctx, tx, convID, runID, eventType, payload)
	})
}

// appendEventTx requires a transaction. Updating first locks this Run across
// SQLite/PG/MySQL; the sequence and event either commit together or both roll back.
func (s *Service) appendEventTx(ctx context.Context, tx *gorm.DB, convID, runID store.UUID, eventType string, payload any) error {
	tx = tx.WithContext(ctx)
	result := tx.Model(&store.AgentRun{}).Where("id = ?", runID).
		Update("last_sequence", gorm.Expr("last_sequence + ?", 1))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	var run store.AgentRun
	if err := tx.Select("last_sequence").Where("id = ?", runID).First(&run).Error; err != nil {
		return err
	}
	ev := store.AgentEvent{
		ID: store.MustNewUUID(), ConversationID: convID, RunID: runID,
		Sequence: run.LastSequence, EventType: eventType,
	}
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			ev.PayloadJSON = b
		}
	}
	return tx.Create(&ev).Error
}

// ── 消息与 Part ─────────────────────────────────────────────────────

// appendPart 追加一条 part；ordinal 决定消息内顺序，前端据此幂等归并。
func (s *Service) appendPart(ctx context.Context, tx *gorm.DB, msgID store.UUID, ordinal int,
	typ, status string, text *string, content datatypes.JSON) error {
	p := store.AgentMessagePart{
		ID: store.MustNewUUID(), MessageID: msgID, Ordinal: ordinal,
		Type: typ, Status: status, TextContent: text, ContentJSON: content,
	}
	if status == "completed" {
		now := time.Now().UTC()
		p.CompletedAt = &now
	}
	return tx.WithContext(ctx).Create(&p).Error
}

// createUserMessage 落库用户消息（文本作为第一个 part）。
func (s *Service) createUserMessage(ctx context.Context, tx *gorm.DB,
	convID, turnID store.UUID, content string) (*store.AgentMessage, error) {
	now := time.Now().UTC()
	msg := store.AgentMessage{
		ID: store.MustNewUUID(), ConversationID: convID,
		TurnID: &turnID, Role: "user", Status: "completed", CompletedAt: &now,
	}
	if err := tx.WithContext(ctx).Create(&msg).Error; err != nil {
		return nil, err
	}
	if err := s.appendPart(ctx, tx, msg.ID, 0, PartText, "completed", &content, nil); err != nil {
		return nil, err
	}
	return &msg, nil
}

// createAssistantMessage 落库助手终答：文本 part + 工具活动 part。
func (s *Service) createAssistantMessage(ctx context.Context, tx *gorm.DB,
	convID store.UUID, runID *store.UUID, content string, toolEvents any) (*store.AgentMessage, error) {
	now := time.Now().UTC()
	msg := store.AgentMessage{
		ID: store.MustNewUUID(), ConversationID: convID,
		RunID: runID, Role: "assistant", Status: "completed", CompletedAt: &now,
	}
	if err := tx.WithContext(ctx).Create(&msg).Error; err != nil {
		return nil, err
	}
	ord := 0
	if content != "" {
		if err := s.appendPart(ctx, tx, msg.ID, ord, PartText, "completed", &content, nil); err != nil {
			return nil, err
		}
		ord++
	}
	// 工具活动整段作为一个 tool part（含 args/result）。
	// 待工具状态机落地后再逐次调用拆成独立 part。
	if b, err := json.Marshal(toolEvents); err == nil && len(b) > 4 {
		if err := s.appendPart(ctx, tx, msg.ID, ord, PartTool, "completed", nil, datatypes.JSON(b)); err != nil {
			return nil, err
		}
	}
	return &msg, nil
}
