// Package ratelimit 提供进程内滑动窗口限流。
//
// 单实例部署足够；多实例时统一换 Redis——与 cache 域的取舍一致。
// 键 = 类型 + 标识（IP / 用户 / 账号），窗口与阈值由调用方给，
// 属工程参数而非业务配置。
package ratelimit

import (
	"sync"
	"time"
)

// DefaultWindow 是 Allow 使用的默认窗口。
const DefaultWindow = time.Minute

type entry struct {
	hits   []time.Time
	window time.Duration
}

// Limiter 是按 key 计数的滑动窗口限流器，可并发使用。
type Limiter struct {
	mu        sync.Mutex
	hits      map[string]*entry
	lastSweep time.Time
}

func New() *Limiter { return &Limiter{hits: map[string]*entry{}} }

// Allow 记录一次尝试；DefaultWindow 内超过 max 次返回 false。
func (l *Limiter) Allow(key string, max int) bool {
	return l.AllowWindow(key, max, DefaultWindow)
}

// AllowWindow 是 Allow 的通用形式：window 内最多 max 次。
// 惰性清扫：键会按任意标识累积，过期条目在调用时删除，避免无界内存。
func (l *Limiter) AllowWindow(key string, max int, window time.Duration) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) >= DefaultWindow {
		for k, e := range l.hits {
			if len(e.hits) == 0 || now.Sub(e.hits[len(e.hits)-1]) >= e.window {
				delete(l.hits, k)
			}
		}
		l.lastSweep = now
	}
	e := l.hits[key]
	if e == nil {
		e = &entry{}
		l.hits[key] = e
	}
	e.window = window
	keep := e.hits[:0]
	for _, t := range e.hits {
		if now.Sub(t) < window {
			keep = append(keep, t)
		}
	}
	e.hits = keep
	if len(keep) >= max {
		return false
	}
	e.hits = append(e.hits, now)
	return true
}
