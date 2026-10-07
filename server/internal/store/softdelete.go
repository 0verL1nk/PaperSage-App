package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// DeletedSeqColumn 是随 deleted_at 一并写入的列名。
//
// 唯一键可以包含它：活跃行为 0，删除行取单调非零值（unix 微秒）。
// 这样「删除后重建同名资源」既能保住活跃行的唯一性，又不会被已删行永久占位。
//
// 为什么需要它：MySQL/TiDB 不支持部分唯一索引（WHERE deleted_at IS NULL）；
// 把 deleted_at 直接放进唯一键又因唯一索引中 NULL 不去重，
// 反而让活跃行失去约束。
const DeletedSeqColumn = "deleted_seq"

// SoftDeleteTime 是逻辑删除标记，替代 gorm.DeletedAt。
//
// 与 gorm.DeletedAt 的唯一区别：删除时在同一条 UPDATE 里额外写入 deleted_seq，
// 保持原子性（不用 BeforeDelete 钩子再发一条 UPDATE，那会递归且非原子）。
type SoftDeleteTime struct {
	sql.NullTime
}

// Scan/Value 使该类型可直接读写 nullable 时间列。
func (n *SoftDeleteTime) Scan(value any) error { return n.NullTime.Scan(value) }

func (n SoftDeleteTime) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Time, nil
}

func (n SoftDeleteTime) MarshalJSON() ([]byte, error) {
	if n.Valid {
		return json.Marshal(n.Time)
	}
	return json.Marshal(nil)
}

func (n *SoftDeleteTime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		n.Valid = false
		return nil
	}
	if err := json.Unmarshal(b, &n.Time); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

// GORM 按字段类型断言这三个接口来自定义软删子句（schema/schema.go）。
func (SoftDeleteTime) QueryClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{softDeleteQueryClause{field: f}}
}

func (SoftDeleteTime) UpdateClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{softDeleteUpdateClause{field: f}}
}

func (SoftDeleteTime) DeleteClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{softDeleteDeleteClause{field: f}}
}

// softDeleteQueryClause：查询自动过滤已删行（deleted_at IS NULL）。
type softDeleteQueryClause struct{ field *schema.Field }

func (softDeleteQueryClause) Name() string               { return "" }
func (softDeleteQueryClause) Build(clause.Builder)       {}
func (softDeleteQueryClause) MergeClause(*clause.Clause) {}

func (sd softDeleteQueryClause) ModifyStatement(stmt *gorm.Statement) {
	if _, ok := stmt.Clauses["soft_delete_enabled"]; ok || stmt.Statement.Unscoped {
		return
	}
	// 已有 OR 条件需要整体加括号，否则 AND 会改变语义。
	if c, ok := stmt.Clauses["WHERE"]; ok {
		if where, ok := c.Expression.(clause.Where); ok && len(where.Exprs) >= 1 {
			for _, expr := range where.Exprs {
				if orCond, ok := expr.(clause.OrConditions); ok && len(orCond.Exprs) == 1 {
					where.Exprs = []clause.Expression{clause.And(where.Exprs...)}
					c.Expression = where
					stmt.Clauses["WHERE"] = c
					break
				}
			}
		}
	}
	stmt.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: sd.field.DBName}, Value: nil},
	}})
	stmt.Clauses["soft_delete_enabled"] = clause.Clause{}
}

// softDeleteUpdateClause：更新语句同样限定在未删除行上。
type softDeleteUpdateClause struct{ field *schema.Field }

func (softDeleteUpdateClause) Name() string               { return "" }
func (softDeleteUpdateClause) Build(clause.Builder)       {}
func (softDeleteUpdateClause) MergeClause(*clause.Clause) {}

func (sd softDeleteUpdateClause) ModifyStatement(stmt *gorm.Statement) {
	if stmt.SQL.Len() == 0 && !stmt.Statement.Unscoped {
		softDeleteQueryClause(sd).ModifyStatement(stmt)
	}
}

// softDeleteDeleteClause：删除转成 UPDATE，同时写入 deleted_at 与 deleted_seq。
type softDeleteDeleteClause struct{ field *schema.Field }

func (softDeleteDeleteClause) Name() string               { return "" }
func (softDeleteDeleteClause) Build(clause.Builder)       {}
func (softDeleteDeleteClause) MergeClause(*clause.Clause) {}

func (sd softDeleteDeleteClause) ModifyStatement(stmt *gorm.Statement) {
	if stmt.SQL.Len() != 0 || stmt.Statement.Unscoped {
		return
	}
	now := stmt.NowFunc()
	seq := now.UnixMicro()
	stmt.AddClause(clause.Set{
		{Column: clause.Column{Name: sd.field.DBName}, Value: now},
		{Column: clause.Column{Name: DeletedSeqColumn}, Value: seq},
	})
	stmt.SetColumn(sd.field.DBName, now, true)
	stmt.SetColumn(DeletedSeqColumn, seq, true)

	if stmt.Schema != nil {
		_, queryValues := schema.GetIdentityFieldValuesMap(stmt.Context, stmt.ReflectValue, stmt.Schema.PrimaryFields)
		column, values := schema.ToQueryValues(stmt.Table, stmt.Schema.PrimaryFieldDBNames, queryValues)
		if len(values) > 0 {
			stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: column, Values: values}}})
		}
		if stmt.ReflectValue.CanAddr() && stmt.Dest != stmt.Model && stmt.Model != nil {
			_, queryValues = schema.GetIdentityFieldValuesMap(stmt.Context, reflect.ValueOf(stmt.Model), stmt.Schema.PrimaryFields)
			column, values = schema.ToQueryValues(stmt.Table, stmt.Schema.PrimaryFieldDBNames, queryValues)
			if len(values) > 0 {
				stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: column, Values: values}}})
			}
		}
	}

	softDeleteQueryClause(sd).ModifyStatement(stmt)
	stmt.AddClauseIfNotExists(clause.Update{})
	stmt.Build(stmt.DB.Callback().Update().Clauses...)
}
