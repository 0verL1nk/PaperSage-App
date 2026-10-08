// Package email — 交易邮件发送域。
//
// Provider 由 EMAIL_PROVIDER 选择：resend（HTTP API）/ smtp（自建
// 中继）/ noop（本地开发，仅日志）。统一 Sender 接口，auth 等域注入。
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
)

// Sender 发送一封交易邮件（HTML 正文）。
type Sender interface {
	Send(ctx context.Context, to, subject, html string) error
}

// Noop：未配置邮件服务时的占位 sender。它必须返回错误，避免上层把
// “没有发送”误报为“已发送”。需要测试替身时请使用明确的 fake sender。
type Noop struct{}

func (Noop) Send(_ context.Context, _, _, _ string) error {
	return errors.New("email delivery is not configured")
}

// Resend：https://resend.com HTTP API。
type Resend struct {
	apiKey string
	from   string
	base   string // 测试可注入
}

func NewResend(apiKey, from, baseURL string) *Resend {
	return &Resend{apiKey: apiKey, from: from, base: baseURL}
}

func (r *Resend) Send(ctx context.Context, to, subject, html string) error {
	payload := map[string]any{
		"from":    r.from,
		"to":      to,
		"subject": subject,
		"html":    html,
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/emails", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// SMTP：自建中继发送（HTML 邮件）。
type SMTP struct {
	Host string // host:port
	From string
	Auth smtp.Auth
}

func (s *SMTP) Send(_ context.Context, to, subject, html string) error {
	msg := "From: " + s.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		html
	var auth smtp.Auth
	if s.Auth != nil {
		auth = s.Auth
	}
	return smtp.SendMail(s.Host, auth, s.From, []string{to}, []byte(msg))
}
