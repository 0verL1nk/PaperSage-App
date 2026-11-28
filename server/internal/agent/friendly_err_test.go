package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
)

// H1 要求不同错误给出不同可执行提示，不能统一成“调用失败”。
// 检查单元是非交互的后台任务：失败说明不能出现「发送继续让助手接着处理」这类聊天话术，
// 并且要告诉用户下一步能做什么。
func TestReviewUnitFailureTextHasNoChatWording(t *testing.T) {
	got := reviewUnitFailureText(adk.ErrExceedMaxIterations)
	for _, banned := range []string{"发送", "助手", "继续让"} {
		if strings.Contains(got, banned) {
			t.Fatalf("review unit failure text must not contain %q: %s", banned, got)
		}
	}
	if !strings.Contains(got, "步数上限") || !strings.Contains(got, "重试") {
		t.Fatalf("review unit failure text must say what happened and what to do: %s", got)
	}
	// 供应商错误的可执行提示沿用统一分类，不会因为检查场景丢失。
	if !strings.Contains(reviewUnitFailureText(errors.New("status code: 429 too many requests")), "限额") {
		t.Fatal("review unit failure text must keep the 429 guidance")
	}
}

func TestFriendlyErrClassifiesProviderFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401", errors.New("status code: 401 Unauthorized"), "模型 API Key 无效或已过期"},
		{"429", errors.New("status code: 429 too many requests"), "模型调用达到限额，请稍后重试"},
		{"deadline", fmt.Errorf("chat: %w", context.DeadlineExceeded), "模型响应超时，请稍后重试或换用响应更快的模型"},
		{"timeout text", errors.New("Post \"https://provider/chat\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"), "模型响应超时，请稍后重试或换用响应更快的模型"},
		{"context length", errors.New("Upstream request failed: context_length_exceeded"), "上下文超过模型上限，请新建对话或缩短问题后重试"},
		{"maximum context", errors.New("This model's maximum context length is 128000 tokens"), "上下文超过模型上限，请新建对话或缩短问题后重试"},
		{"iterations", adk.ErrExceedMaxIterations, "本次执行达到迭代上限；可发送“继续”让助手接着处理未完成的部分。"},
		{"generic", errors.New("scripted failure"), "模型调用失败，请检查模型配置或稍后重试"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := friendlyErr(tc.err); got != tc.want {
				t.Fatalf("friendlyErr(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
