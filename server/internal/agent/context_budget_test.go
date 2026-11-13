package agent

import (
	"testing"

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
