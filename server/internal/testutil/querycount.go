package testutil

import (
	"sync/atomic"

	"gorm.io/gorm"
)

// QueryCounter 统计经 GORM 发出的 SQL 语句数（读 + 写 + Row + Raw）。
// 把「一次主操作要几个 DB 往返」变成可回归的断言——远程库上每次往返都是实打实的延迟。
type QueryCounter struct{ n atomic.Int64 }

// CountQueries 在 db 上注册计数回调并返回计数器。
// 同一 db 上重复注册同名回调会失败，每个测试装配只调用一次。
func CountQueries(db *gorm.DB) *QueryCounter {
	c := &QueryCounter{}
	inc := func(*gorm.DB) { c.n.Add(1) }
	_ = db.Callback().Query().Before("gorm:query").Register("testutil:count_query", inc)
	_ = db.Callback().Create().Before("gorm:create").Register("testutil:count_create", inc)
	_ = db.Callback().Update().Before("gorm:update").Register("testutil:count_update", inc)
	_ = db.Callback().Delete().Before("gorm:delete").Register("testutil:count_delete", inc)
	_ = db.Callback().Row().Before("gorm:row").Register("testutil:count_row", inc)
	_ = db.Callback().Raw().Before("gorm:raw").Register("testutil:count_raw", inc)
	return c
}

// Reset 归零。
func (c *QueryCounter) Reset() { c.n.Store(0) }

// Value 返回自上次 Reset 以来的语句数。
func (c *QueryCounter) Value() int64 { return c.n.Load() }
