// Package syssettings — 系统级运行时配置（admin 可改，DB 持久化）。
//
// 与 user_settings（个人偏好）相对：这里存站点级行为开关，如
// allowed_email_domains（注册邮箱白名单）。已知键集中在 Keys 常量，
// 不写死业务值——白名单内容本身是数据不是代码。
package syssettings

import (
	"encoding/json"
	"time"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 已知设置键（值形态在注释中约定）。
const (
	KeyAllowedEmailDomains = "allowed_email_domains" // []string，空/缺省 = 不限制
)

// All 返回全量设置（合并已知键缺省值），admin 读取用。
func All(db *gorm.DB) (map[string]any, error) {
	var rows []struct {
		Key   string
		Value datatypes.JSON
	}
	if err := db.Table("system_settings").Select("key", "value").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]any{
		KeyAllowedEmailDomains: []string{},
	}
	for _, r := range rows {
		switch r.Key {
		case KeyAllowedEmailDomains:
			var v []string
			if err := json.Unmarshal(r.Value, &v); err == nil {
				out[r.Key] = v
			}
		}
	}
	return out, nil
}

// Put 覆盖写若干键（值须与键约定形态一致；未知键忽略）。
func Put(db *gorm.DB, kv map[string]any) error {
	type write struct {
		key   string
		value datatypes.JSON
	}
	var writes []write
	for key, val := range kv {
		switch key {
		case KeyAllowedEmailDomains:
			list, ok := toStringSlice(val)
			if !ok {
				continue
			}
			raw, err := json.Marshal(list)
			if err != nil {
				return err
			}
			writes = append(writes, write{key, raw})
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, w := range writes {
			// 惯用法 upsert：ON CONFLICT(key) DO UPDATE。
			// 此前 Table().Updates() 后读 tx.RowsAffected 判断是否
			// Create——链式会话的 RowsAffected 不可靠，已存在行时
			// 误走 INSERT 撞唯一键（实测 500）。
			row := store.SystemSetting{
				Key:       w.key,
				Value:     w.value,
				UpdatedAt: time.Now(),
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// toStringSlice 兼容 []string 与 []any（JSON 数字/字符串混合数组）。
func toStringSlice(v any) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}
