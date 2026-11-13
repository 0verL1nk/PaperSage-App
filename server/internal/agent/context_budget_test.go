package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
)

func TestContextLimitsMatchClaudeCodeBuffers(t *testing.T) {
	l := limitsFor(128000)
	if l.compact != 115000 || l.warn != 95000 || l.block != 125000 {
		t.Fatalf("unexpected limits: %+v", l)
	}
	if got := limitsFor(0); got != (contextLimits{}) {
		t.Fatalf("unknown window must disable limits: %+v", got)
	}
}

func TestTrimHistoryKeepsNewestAndExplainsDropped(t *testing.T) {
	system := &schema.Message{Role: schema.System, Content: "system"}
	msgs := []*schema.Message{
		{Role: schema.User, Content: "old-1"},
		{Role: schema.Assistant, Content: "old-2"},
		{Role: schema.User, Content: "newest"},
	}
	out := trimHistory(system, msgs, estimateTokens("system")+estimateTokens("newest"))
	if len(out) != 3 {
		t.Fatalf("expected system + notice + newest, got %d", len(out))
	}
	if out[0].Content != "system" || out[2].Content != "newest" {
		t.Fatalf("wrong messages: %+v", out)
	}
	if out[1].Role != schema.System || out[1].Content == "" {
		t.Fatalf("expected a system notice, got %+v", out[1])
	}
}

func TestTrimHistoryUnknownBudgetKeepsAll(t *testing.T) {
	system := &schema.Message{Role: schema.System, Content: "system"}
	msgs := []*schema.Message{{Role: schema.User, Content: "a"}, {Role: schema.Assistant, Content: "b"}}
	out := trimHistory(system, msgs, 0)
	if len(out) != 3 {
		t.Fatalf("unknown budget must not trim, got %d", len(out))
	}
}

func TestTrimMessagesKeepsSystemAndNewest(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.System, Content: "rules"},
		{Role: schema.User, Content: "old"},
		{Role: schema.Assistant, Content: "old answer"},
		{Role: schema.User, Content: "newest"},
	}
	out := trimMessages(msgs, estimateTokens("rules")+estimateTokens("newest"))
	if len(out) < 3 {
		t.Fatalf("expected system + notice + newest, got %d", len(out))
	}
	if out[0].Content != "rules" || out[len(out)-1].Content != "newest" {
		t.Fatalf("system/newest not preserved: %+v", out)
	}
}

type failingSummarizer struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.Message]
}

func (f *failingSummarizer) BeforeModelRewriteState(
	ctx context.Context,
	_ *adk.TypedChatModelAgentState[*schema.Message],
	_ *adk.TypedModelContext[*schema.Message],
) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	return ctx, nil, errors.New("summary model unavailable")
}

func TestContextGuardFallsBackToTrimWhenSummarizerFails(t *testing.T) {
	guard := &contextGuard{
		inner:  &failingSummarizer{},
		window: 128000,
	}
	state := &adk.TypedChatModelAgentState[*schema.Message]{Messages: []*schema.Message{
		{Role: schema.System, Content: "rules"},
		{Role: schema.User, Content: strings.Repeat("旧消息", 50000)},
		{Role: schema.User, Content: "最新"},
	}}
	_, next, err := guard.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("summarizer failure must not abort the run: %v", err)
	}
	for _, m := range next.Messages {
		if strings.Contains(m.Content, "旧消息") {
			t.Fatalf("oversized message must be dropped: %+v", next.Messages)
		}
	}
	if next.Messages[len(next.Messages)-1].Content != "最新" {
		t.Fatalf("newest message must survive: %+v", next.Messages)
	}
}

func TestSummarizationTokenCounterCountsCJKAndToolArgs(t *testing.T) {
	got, err := summarizationTokenCounter(context.Background(), &summarization.TokenCounterInput{
		Messages: []*schema.Message{{
			Role:      schema.Assistant,
			Content:   "四个汉字",
			ToolCalls: []schema.ToolCall{{Function: schema.FunctionCall{Arguments: "{\"path\":\"main.tex\"}"}}},
		}},
	})
	if err != nil || got <= 4 {
		t.Fatalf("CJK text must not be estimated at 4 chars/token: got %d err=%v", got, err)
	}
}
