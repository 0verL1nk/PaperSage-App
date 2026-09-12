// Package dto 提供跨域共享的 DTO 序列化类型，确保 JSON 形态与
// Python（Pydantic v2 / datetime.isoformat）逐字节一致。
package dto

import (
	"encoding/json"
	"fmt"
	"time"
)

// JSONTime 按 Python datetime.isoformat() 形态序列化：
// 微秒非零时固定 6 位小数，否则省略；偏移为 ±hh:mm 数字形式。
// 零值序列化为 null（对应 Python None 的可空时间字段）。
type JSONTime struct {
	time.Time
}

func (t JSONTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	// 注意：微秒数字不能直接拼进 Format layout（数字会被当作格式动词），
	// 分段拼接：日期时间 + "." + 6 位微秒 + 偏移
	s := t.Format("2006-01-02T15:04:05")
	if micro := t.Nanosecond() / 1000; micro > 0 {
		s += "." + fmt.Sprintf("%06d", micro)
	}
	s += t.Format("-07:00")
	return json.Marshal(s)
}
