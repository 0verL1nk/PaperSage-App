package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 限流器自身的窗口语义与惰性清扫（原 auth 的 TestRateLimiterWindowAndSweep，
// 随实现提取迁到本包）。
func TestWindowAndSweep(t *testing.T) {
	r := New()
	assert.True(t, r.AllowWindow("k", 2, time.Hour))
	assert.True(t, r.AllowWindow("k", 2, time.Hour))
	assert.False(t, r.AllowWindow("k", 2, time.Hour), "窗口内超过上限必须拒绝")

	// 惰性清扫：过期条目应在后续调用中被移除，避免内存无界增长。
	r.mu.Lock()
	r.hits["stale"] = &entry{hits: []time.Time{time.Now().Add(-2 * time.Hour)}, window: time.Hour}
	r.lastSweep = time.Now().Add(-2 * time.Minute)
	r.mu.Unlock()
	assert.True(t, r.AllowWindow("other", 1, time.Minute))
	r.mu.Lock()
	_, staleKept := r.hits["stale"]
	r.mu.Unlock()
	assert.False(t, staleKept, "过期键必须被清扫")
}
