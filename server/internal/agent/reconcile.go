package agent

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// reconcileInterrupted 是进程重启导致 Run 中断时给用户看的原因。
const reconcileInterrupted = "服务重启，执行已中断"

// ReconcileOrphanRuns 收敛本进程启动前遗留的 running Run。
// 单实例部署假设：另一个进程不可能仍在执行这些 Run；未来多实例需改用租约/心跳。
func (s *Service) ReconcileOrphanRuns(ctx context.Context) error {
	if s.DB == nil || s.startedAt.IsZero() {
		return nil
	}
	var runs []store.AgentRun
	if err := s.DB.WithContext(ctx).
		Where("status = ? AND started_at < ?", RunRunning, s.startedAt).
		Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&store.AgentRun{}).
				Where("id = ? AND status = ?", run.ID, RunRunning).
				Updates(map[string]any{
					"status":       RunFailed,
					"completed_at": time.Now().UTC(),
					"error_json":   datatypes.JSON(mustJSON(map[string]string{"message": reconcileInterrupted})),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			// 崩溃可能让 last_sequence 落后于已落库事件；先对齐到真实最大值，
			// 否则补终态事件会撞 agent_events 的唯一键。
			var maxSeq int64
			if err := tx.Model(&store.AgentEvent{}).Where("run_id = ?", run.ID).
				Select("COALESCE(MAX(sequence), 0)").Scan(&maxSeq).Error; err != nil {
				return err
			}
			if err := tx.Model(&store.AgentRun{}).Where("id = ?", run.ID).
				Update("last_sequence", maxSeq).Error; err != nil {
				return err
			}
			if err := tx.Model(&store.AgentToolCall{}).Where("run_id = ? AND state = ?", run.ID, ToolExecuting).
				Updates(map[string]any{"state": ToolFailed, "completed_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
			if run.ParentRunID == nil {
				reason := reconcileInterrupted
				if run.OutputMessageID == nil {
					parts, err := (&Service{DB: tx}).buildTranscript(ctx, run.ID, "", reason)
					if err != nil {
						return err
					}
					var children []store.AgentRun
					if err := tx.Select("id", "status").Where("parent_run_id = ?", run.ID).Find(&children).Error; err != nil {
						return err
					}
					parts = settleReconciledActivities(parts, children)
					msg, err := s.createAssistantMessage(ctx, tx, run.ConversationID, &run.ID, splitReconciledText(parts))
					if err != nil {
						return err
					}
					if err := tx.Model(msg).Update("status", RunFailed).Error; err != nil {
						return err
					}
					if err := tx.Model(&store.AgentRun{}).Where("id = ?", run.ID).
						Update("output_message_id", msg.ID).Error; err != nil {
						return err
					}
				} else {
					var existing int64
					if err := tx.Model(&store.AgentMessagePart{}).
						Where("message_id = ? AND type = ?", *run.OutputMessageID, PartError).
						Count(&existing).Error; err != nil {
						return err
					}
					if existing == 0 {
						var maxOrdinal int
						if err := tx.Model(&store.AgentMessagePart{}).Where("message_id = ?", *run.OutputMessageID).
							Select("COALESCE(MAX(ordinal), -1)").Scan(&maxOrdinal).Error; err != nil {
							return err
						}
						if err := s.appendPart(ctx, tx, *run.OutputMessageID, maxOrdinal+1, PartError, "failed", &reason, nil); err != nil {
							return err
						}
					}
					if err := tx.Model(&store.AgentMessage{}).Where("id = ?", *run.OutputMessageID).
						Update("status", RunFailed).Error; err != nil {
						return err
					}
				}
			}
			if err := s.appendEventTx(ctx, tx, run.ConversationID, run.ID, evtRunError,
				map[string]string{"message": reconcileInterrupted}); err != nil {
				return err
			}
			return s.appendEventTx(ctx, tx, run.ConversationID, run.ID, evtRunFinished,
				map[string]string{"status": RunFailed})
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func settleReconciledActivities(parts []transcriptPart, children []store.AgentRun) []transcriptPart {
	statuses := make(map[string]string, len(children))
	for _, child := range children {
		status := child.Status
		if status == RunRunning {
			status = RunFailed
		}
		statuses[child.ID.String()] = status
	}
	indices := make(map[string]int)
	out := make([]transcriptPart, 0, len(parts))
	for _, part := range parts {
		if part.Type != PartActivity {
			out = append(out, part)
			continue
		}
		var content map[string]any
		if err := json.Unmarshal(part.Content, &content); err != nil {
			out = append(out, part)
			continue
		}
		id, _ := content["runId"].(string)
		if part.Status == RunRunning {
			part.Status = statuses[id]
			if part.Status == "" {
				part.Status = RunFailed
			}
			content["status"] = part.Status
			part.Content, _ = json.Marshal(content)
		}
		if index, ok := indices[id]; id != "" && ok {
			out[index] = part
			continue
		}
		if id != "" {
			indices[id] = len(out)
		}
		out = append(out, part)
	}
	return out
}

// MySQL TEXT stores at most 65,535 bytes; split without changing the recovered text.
func splitReconciledText(parts []transcriptPart) []transcriptPart {
	const maxTextBytes = 65535
	out := make([]transcriptPart, 0, len(parts))
	for _, part := range parts {
		if part.Type != PartText || part.Text == nil || len(*part.Text) <= maxTextBytes {
			out = append(out, part)
			continue
		}
		remaining := *part.Text
		continuation := false
		for len(remaining) > 0 {
			end := len(remaining)
			if end > maxTextBytes {
				end = maxTextBytes
				for !utf8.RuneStart(remaining[end]) {
					end--
				}
			}
			piece := remaining[:end]
			chunk := part
			chunk.Text = &piece
			if continuation {
				chunk.Content = datatypes.JSON(`{"textContinuation":true}`)
			}
			out = append(out, chunk)
			remaining = remaining[end:]
			continuation = true
		}
	}
	return out
}
