package agent

import (
	"context"
	"time"

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
			// 被杀掉的进程往往没来得及物化助手终答；没有消息就没有 part，
			// 转录接口会把这次失败显示成“只有用户消息”。这里补一条带 error part
			// 的助手消息，前端刷新后才能看到中断原因。
			reason := reconcileInterrupted
			if run.OutputMessageID == nil {
				msg, err := s.createAssistantMessage(ctx, tx, run.ConversationID, &run.ID,
					[]transcriptPart{{Type: PartError, Status: "failed", Text: &reason}})
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
