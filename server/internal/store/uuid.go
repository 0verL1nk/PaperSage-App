package store

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// UUID mirrors backend/core/base.py's UUID TypeDecorator: native uuid column
// on PostgreSQL, 32-char hex (no dashes) elsewhere.
type UUID struct {
	uuid.UUID
}

// Value stores as 32-char hex without dashes (SQLite-compatible shape).
func (u UUID) Value() (driver.Value, error) {
	return strings.ReplaceAll(u.String(), "-", ""), nil
}

// Scan accepts 32-hex (Python SQLite shape), dashed strings, []byte (hex or
// raw 16 bytes), and nil.
func (u *UUID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		u.UUID = uuid.Nil
		return nil
	case []byte:
		if len(v) == 16 {
			id, err := uuid.FromBytes(v)
			u.UUID = id
			return err
		}
		parsed, err := uuid.Parse(string(v))
		u.UUID = parsed
		return err
	case string:
		parsed, err := uuid.Parse(v)
		u.UUID = parsed
		return err
	default:
		return fmt.Errorf("store.UUID: unsupported scan source %T", src)
	}
}

func uuidDataType(dialect string) string {
	if dialect == "postgres" {
		return "uuid"
	}
	return "varchar(32)"
}

// GormDBDataType picks the column type per dialect.
func (UUID) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	return uuidDataType(db.Dialector.Name()) //nolint:staticcheck // QF1008 误报：Name 是方法
}

// ErrBadUUID is returned when a stored value cannot be parsed.
var ErrBadUUID = errors.New("invalid uuid value")

var (
	_ driver.Valuer = UUID{}
	_ sql.Scanner   = (*UUID)(nil)
)
