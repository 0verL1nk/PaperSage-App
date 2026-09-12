package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约来源：Python datetime.isoformat()（Pydantic v2 默认序列化）
// - 微秒非零 → 固定 6 位；为零 → 省略小数部分
// - 偏移用 +00:00 数字形式（非 Z）
func TestJSONTimeFormat(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"微秒", time.Date(2026, 4, 25, 12, 34, 56, 123456000, time.UTC), `"2026-04-25T12:34:56.123456+00:00"`},
		{"微秒尾零保留", time.Date(2026, 4, 25, 12, 34, 56, 123400000, time.UTC), `"2026-04-25T12:34:56.123400+00:00"`},
		{"整秒", time.Date(2026, 4, 25, 12, 34, 56, 0, time.UTC), `"2026-04-25T12:34:56+00:00"`},
		{"非 UTC 偏移", time.Date(2026, 4, 25, 20, 34, 56, 0, time.FixedZone("CST", 8*3600)), `"2026-04-25T20:34:56+08:00"`},
		{"零值 → null（Python None）", time.Time{}, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(JSONTime{Time: tc.in})
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(b))
		})
	}
}

// 结构体内嵌时同样生效（nullable 时间字段输出 null 而非 "0001-..."）
func TestJSONTimeInStruct(t *testing.T) {
	type resp struct {
		DeletedAt JSONTime `json:"deleted_at"`
	}

	b, err := json.Marshal(resp{})
	require.NoError(t, err)
	assert.Equal(t, `{"deleted_at":null}`, string(b))

	deleted := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	b, err = json.Marshal(resp{DeletedAt: JSONTime{Time: deleted}})
	require.NoError(t, err)
	assert.Equal(t, `{"deleted_at":"2026-06-01T09:00:00+00:00"}`, string(b))
}

// UUID 序列化：Pydantic 输出带横线的标准形式（google/uuid 原生一致，防回归）
func TestUUIDJSON(t *testing.T) {
	b, err := json.Marshal(uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a7b"))
	require.NoError(t, err)
	assert.Equal(t, `"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a7b"`, string(b))
}
