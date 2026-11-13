package agent

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// 上下文阈值对齐 Claude Code 的实现：
//
//	compact = window - 13,000（摘要缓冲区）
//	warn    = compact - 20,000
//	block   = window - 3,000
//
// 参考其 CLI 中 jX/Uke 两个函数的常量；窗口未知时不做任何裁剪。
const (
	summaryTokenReserve = 13000
	warnTokenBuffer     = 20000
	blockTokenBuffer    = 3000
)

type contextLimits struct {
	window  int
	compact int
	warn    int
	block   int
}

func limitsFor(window int) contextLimits {
	if window <= 0 {
		return contextLimits{}
	}
	return contextLimits{
		window:  window,
		compact: window - summaryTokenReserve,
		warn:    window - summaryTokenReserve - warnTokenBuffer,
		block:   window - blockTokenBuffer,
	}
}

// estimateTokens 是保守估算：ASCII 约 4 字符/token，非 ASCII 约 1 字符/token。
// 只用于接近窗口时触发保护，不追求与具体分词器一致。
func estimateTokens(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < 128 {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}

func messagesTokens(msgs []*schema.Message) int {
	n := 0
	for _, m := range msgs {
		n += estimateTokens(m.Content)
	}
	return n
}

// trimHistory 保留 system 提示与尽量新的消息；必要时插入一条省略说明。
// 最新一条消息即使超出预算也会保留，否则模型看不到用户当前问题。
func trimHistory(system *schema.Message, msgs []*schema.Message, budget int) []*schema.Message {
	out := make([]*schema.Message, 0, len(msgs)+2)
	out = append(out, system)
	if budget <= 0 {
		return append(out, msgs...)
	}
	used := estimateTokens(system.Content)
	kept := make([]*schema.Message, 0, len(msgs))
	dropped := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		cost := estimateTokens(msgs[i].Content)
		if used+cost > budget && len(kept) > 0 {
			dropped = i + 1
			break
		}
		used += cost
		kept = append(kept, msgs[i])
	}
	if dropped > 0 {
		out = append(out, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("较早的 %d 条消息因上下文长度限制已省略；如需其中细节，请让用户重新提供。", dropped),
		})
	}
	for i := len(kept) - 1; i >= 0; i-- {
		out = append(out, kept[i])
	}
	return out
}
