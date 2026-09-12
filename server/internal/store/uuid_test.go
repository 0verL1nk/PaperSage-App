package store

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约来源：backend/core/base.py UUID TypeDecorator
// - PostgreSQL：原生 uuid；SQLite：String(32)、hex 无横线
// - Scan 需兼容 32 位无横线 / 36 位带横线 / []byte
func TestUUIDValue(t *testing.T) {
	u := uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a7b")

	v, err := UUID{UUID: u}.Value()
	require.NoError(t, err)
	// 存储形态：32 位 hex，无横线（与 Python 完全一致）
	assert.Equal(t, "a0eebc999c0b4ef8bb6d6bb9bd380a7b", v)
}

func TestUUIDScan(t *testing.T) {
	want := uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a7b")

	cases := []struct {
		name string
		src  any
	}{
		{"32-hex string", "a0eebc999c0b4ef8bb6d6bb9bd380a7b"}, // Python SQLite 写入形态
		{"36-dash string", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a7b"},
		{"bytes", []byte("a0eebc999c0b4ef8bb6d6bb9bd380a7b")},
		{"pg uuid value", want[:]}, // postgres 驱动可能回传 16 字节
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got UUID
			require.NoError(t, got.Scan(tc.src))
			assert.Equal(t, want, got.UUID)
		})
	}

	var got UUID
	require.NoError(t, got.Scan(nil))
	assert.Equal(t, uuid.Nil, got.UUID)
}

// 兼容 driver.Valuer / sql.Scanner 接口断言
var _ driver.Valuer = UUID{}
var _ sql.Scanner = (*UUID)(nil)
