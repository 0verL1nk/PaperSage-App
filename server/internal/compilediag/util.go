package compilediag

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func parsePositiveInt(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, errInvalidParam
	}
	return n, nil
}

func parseBool(v string) (bool, bool) {
	switch v {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	}
	return false, false
}

func parseUUID(v string) (*store.UUID, bool) {
	id, err := uuid.Parse(v)
	if err != nil {
		return nil, false
	}
	return &store.UUID{UUID: id}, true
}

// parseTimeParam 接受 RFC3339 或 YYYY-MM-DD；dateOnly 用于把日期上界
// 扩到次日零点（闭区间语义）。
func parseTimeParam(v string) (time.Time, bool, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, false, true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true, true
	}
	return time.Time{}, false, false
}

func applyFilters(q *gorm.DB, f *listFilters) *gorm.DB {
	if f.FailureKind != "" {
		q = q.Where("r.failure_kind = ?", f.FailureKind)
	}
	if f.Engine != "" {
		q = q.Where("r.engine = ?", f.Engine)
	}
	if f.IsGuest != nil {
		q = q.Where("r.is_guest = ?", *f.IsGuest)
	}
	if f.UserID != nil {
		q = q.Where("r.user_id = ?", *f.UserID)
	}
	if f.ProjectID != nil {
		q = q.Where("r.project_id = ?", *f.ProjectID)
	}
	if f.From != nil {
		q = q.Where("r.created_at >= ?", *f.From)
	}
	if f.To != nil {
		if f.ToDateOnly {
			q = q.Where("r.created_at < ?", f.To.AddDate(0, 0, 1))
		} else {
			q = q.Where("r.created_at <= ?", *f.To)
		}
	}
	return q
}
