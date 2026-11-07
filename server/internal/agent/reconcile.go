package agent

import (
	"context"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

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
					"error_json":   datatypes.JSON(mustJSON(map[string]string{"message": "服务重启，执行已中断"})),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			if err := s.appendEventTx(ctx, tx, run.ConversationID, run.ID, evtRunError,
				map[string]string{"message": "服务重启，执行已中断"}); err != nil {
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
