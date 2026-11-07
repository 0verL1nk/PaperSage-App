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
	PartProposal = "proposal"
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

var (
	errThreadConflict  = errors.New("conversation version conflict")
	errRequestConflict = errors.New("client request id already used with different content")
	errIncompleteTurn  = errors.New("existing turn has no complete root run")
	errCommitUnknown   = errors.New("run submission commit outcome is unknown")
)

type rootRunResult struct {
	turn             store.AgentTurn
	run              store.AgentRun
	version          int64
	model            *ResolvedModel
	created          bool
	finalizeDeadline time.Time
}

// replayRootRun only returns fully committed turns. Legacy incomplete turns are
// not repaired here: starting another Run could repeat already-paid work.
func (s *Service) replayRootRun(ctx context.Context, convID, userID store.UUID, key, content string) (*rootRunResult, error) {
	return s.lookupRootRun(ctx, convID, userID, key, content, nil)
}

// lookupRootRun can identify this transaction's Turn even if no idempotency key was supplied.
func (s *Service) lookupRootRun(ctx context.Context, convID, userID store.UUID, key, content string, turnID *store.UUID) (*rootRunResult, error) {
	if key == "" && turnID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var conv store.AgentConversation
	if err := s.DB.WithContext(ctx).Where("id = ? AND user_id = ?", convID, userID).First(&conv).Error; err != nil {
		return nil, err
	}
	var turn store.AgentTurn
	query := s.DB.WithContext(ctx).Where("conversation_id = ?", convID)
	if key != "" {
		query = query.Where("client_request_id = ?", key)
	}
	if turnID != nil {
		query = query.Where("id = ?", *turnID)
	}
	if err := query.First(&turn).Error; err != nil {
		return nil, err
	}
	if turn.UserMessageID == nil || turn.RootRunID == nil {
		return nil, errIncompleteTurn
	}
	var msg store.AgentMessage
	if err := s.DB.WithContext(ctx).Where("id = ? AND conversation_id = ? AND turn_id = ? AND role = ?", *turn.UserMessageID, convID, turn.ID, "user").First(&msg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errIncompleteTurn
		}
		return nil, err
	}
	var part store.AgentMessagePart
	if err := s.DB.WithContext(ctx).Where("message_id = ? AND ordinal = ? AND type = ?", msg.ID, 0, PartText).First(&part).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errIncompleteTurn
		}
		return nil, err
	}
	if part.TextContent == nil || *part.TextContent != content {
		return nil, errRequestConflict
	}
	var run store.AgentRun
	if err := s.DB.WithContext(ctx).Where("id = ? AND conversation_id = ? AND turn_id = ?", *turn.RootRunID, convID, turn.ID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errIncompleteTurn
		}
		return nil, err
	}
	return &rootRunResult{turn: turn, run: run, version: conv.Version}, nil
}

// createRootRun commits all durable submission records before starting any work.
func (s *Service) createRootRun(ctx context.Context, conv store.AgentConversation, userID store.UUID, key, content string, expected *int64) (*rootRunResult, error) {
	if existing, err := s.replayRootRun(ctx, conv.ID, userID, key, content); !errors.Is(err, gorm.ErrRecordNotFound) {
		return existing, err
	}
	resolved, err := ResolveModelWithEnv(ctx, s.DB, userID, s.Cfg)
	if err != nil {
		// A concurrent request may have committed while model settings were read.
		if existing, replayErr := s.replayRootRun(ctx, conv.ID, userID, key, content); !errors.Is(replayErr, gorm.ErrRecordNotFound) {
			return existing, replayErr
		}
		return nil, err
	}
	result := &rootRunResult{model: resolved, created: true}
	bodyComplete := false
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&store.AgentConversation{}).Where("id = ? AND user_id = ?", conv.ID, userID)
		if expected != nil {
			query = query.Where("version = ?", *expected)
		}
		updated := query.Update("version", gorm.Expr("version + ?", 1))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errThreadConflict
		}
		var current store.AgentConversation
		if err := tx.Select("version").Where("id = ?", conv.ID).First(&current).Error; err != nil {
			return err
		}
		result.version = current.Version
		result.turn = store.AgentTurn{ID: store.MustNewUUID(), ConversationID: conv.ID}
		if key != "" {
			result.turn.ClientRequestID = &key
		}
		if err := tx.Create(&result.turn).Error; err != nil {
			return err
		}
		msg, err := s.createUserMessage(ctx, tx, conv.ID, result.turn.ID, content)
		if err != nil {
			return err
		}
		run, err := s.createRun(ctx, tx, conv.ID, result.turn.ID, AgentKeyMain, resolved.Source, resolved.Model, systemPromptVersion)
		if err != nil {
			return err
		}
		result.run = *run
		result.turn.UserMessageID, result.turn.RootRunID = &msg.ID, &run.ID
		if err := tx.Model(&store.AgentTurn{}).Where("id = ?", result.turn.ID).Updates(map[string]any{"user_message_id": msg.ID, "root_run_id": run.ID}).Error; err != nil {
			return err
		}
		if err := s.appendEventTx(ctx, tx, conv.ID, run.ID, evtRunStarted, nil); err != nil {
			return err
		}
		bodyComplete = true
		return nil
	})
	if err != nil {
		// An admitted handler keeps the DB open through shutdown. Share this
		// detached budget with terminal persistence if shutdown has started.
		checkCtx, cancel := context.WithTimeout(context.Background(), runFinalizeTimeout)
		defer cancel()
		if bodyComplete {
			// Only the request that generated both IDs owns the right to start it.
			existing, checkErr := s.lookupRootRun(checkCtx, conv.ID, userID, key, content, &result.turn.ID)
			if checkErr == nil && existing.run.ID == result.run.ID {
				result.version = existing.version
				result.finalizeDeadline, _ = checkCtx.Deadline()
				return result, nil
			}
			return nil, errCommitUnknown
		}
		// A same-key winner may have committed before this transaction lost CAS
		// or the unique constraint. Never read through the failed transaction.
		if existing, replayErr := s.replayRootRun(checkCtx, conv.ID, userID, key, content); !errors.Is(replayErr, gorm.ErrRecordNotFound) {
			return existing, replayErr
		}
		return nil, err
	}
	return result, nil
}

// createRun 建一个 Run 并快照模型与提示版本。
// Run 独立于 HTTP 连接存在：可查询、可取消、可恢复。
func (s *Service) createRun(ctx context.Context, tx *gorm.DB, convID, turnID store.UUID, agentKey, provider, model, promptVersion string) (*store.AgentRun, error) {
	now := time.Now().UTC()
	run := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: convID,
		AgentKey: agentKey, Status: RunRunning,
		ModelProvider: provider, ModelName: model, PromptVersion: promptVersion,
		StartedAt: &now,
	}
	run.TurnID = &turnID
	if err := tx.WithContext(ctx).Create(&run).Error; err != nil {
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

// createAssistantMessage 落库助手终答：parts 由事件事实重建，保留文本、
// 每个 toolCallId 的工具、建议与错误，而不是一条聚合工具记录。
func (s *Service) createAssistantMessage(ctx context.Context, tx *gorm.DB,
	convID store.UUID, runID *store.UUID, parts []transcriptPart) (*store.AgentMessage, error) {
	now := time.Now().UTC()
	msg := store.AgentMessage{
		ID: store.MustNewUUID(), ConversationID: convID,
		RunID: runID, Role: "assistant", Status: "completed", CompletedAt: &now,
	}
	if err := tx.WithContext(ctx).Create(&msg).Error; err != nil {
		return nil, err
	}
	for i, p := range parts {
		if err := s.appendPart(ctx, tx, msg.ID, i, p.Type, p.Status, p.Text, p.Content); err != nil {
			return nil, err
		}
	}
	return &msg, nil
}
