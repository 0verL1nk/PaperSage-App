// Package norms 是「已采用学术规范」的输入边界。
//
// HELE-25 落地规范库之前，装配层注入 Unavailable：调用方拿到的是明确的
// 「规范库尚未接入」，而不是空集合——空集合会被误读成「已接入但没有采用任何规范」。
// HELE-25 落地后只需替换 Provider 实现，检查链路与 payload 契约不变。
package norms

import (
	"context"
	"errors"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Entry 是一条已采用的规范条目；字段与未来规范库的对外契约保持一致。
type Entry struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Rule     string `json:"rule"`
	Category string `json:"category,omitempty"`
	Source   string `json:"source,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

// ErrUnavailable 表示规范库尚未接入；调用方必须把它呈现为「不可用」。
var ErrUnavailable = errors.New("学术规范库尚未接入")

// Provider 返回项目已采用的规范条目。返回空切片且无错误表示「规范库可用，
// 但该项目未采用任何条目」。
type Provider interface {
	AdoptedForProject(ctx context.Context, projectID store.UUID) ([]Entry, error)
}

// Unavailable 是 HELE-25 落地前的占位实现，永远返回 ErrUnavailable。
type Unavailable struct{}

func (Unavailable) AdoptedForProject(context.Context, store.UUID) ([]Entry, error) {
	return nil, ErrUnavailable
}
