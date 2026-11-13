package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	compactEventType = "context.compacted"
	// compactKeepRecent 保留最近若干条原文；更早的消息压缩成摘要。
	compactKeepRecent = 6
)

type compactionPayload struct {
	Summary string `json:"summary"`
	Dropped int    `json:"dropped"`
	Through string `json:"through"` // 最后一条被摘要消息的创建时间（RFC3339）
}

// latestCompaction 读取会话最近一次压缩事件。
func (s *Service) latestCompaction(ctx context.Context, convID store.UUID) (*compactionPayload, error) {
	var ev store.AgentEvent
	err := s.DB.WithContext(ctx).
		Where("conversation_id = ? AND event_type = ?", convID, compactEventType).
		Order("sequence DESC").First(&ev).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	var p compactionPayload
	if err := json.Unmarshal(ev.PayloadJSON, &p); err != nil || p.Summary == "" {
		return nil, nil
	}
	return &p, nil
}

// maybeCompact 在接近上下文窗口时，把较早的消息压缩成一条结构化摘要。
// 失败不阻断本轮：loadHistory 仍有确定性裁剪兜底。
func (s *Service) maybeCompact(ctx context.Context, rc runContext, chatModel model.BaseChatModel) {
	if rc.Model == nil || rc.Model.ContextWindow == nil {
		return
	}
	limits := limitsFor(*rc.Model.ContextWindow)
	if limits.compact <= 0 {
		return
	}
	msgs, err := s.readHistory(ctx, rc.ConversationID)
	if err != nil {
		return
	}
	// 已经被上次摘要覆盖的消息不再重复压缩，避免“摘要的摘要”。
	compacted, _ := s.latestCompaction(ctx, rc.ConversationID)
	fresh := make([]historyMessage, 0, len(msgs))
	for _, m := range msgs {
		if !compactionBefore(compacted, m.message.CreatedAt) {
			fresh = append(fresh, m)
		}
	}
	if len(fresh) <= compactKeepRecent {
		return
	}
	indexTokens := 0
	if rc.Model.MemoryEnabled {
		indexTokens = estimateTokens(s.memoryIndex(ctx, rc.ProjectID))
	}
	used := estimateTokens(systemPrompt) + indexTokens
	if compacted != nil {
		used += estimateTokens(compacted.Summary)
	}
	for _, m := range fresh {
		used += estimateTokens(messageText(m))
	}
	if used < limits.compact {
		return
	}

	cut := len(fresh) - compactKeepRecent
	older := fresh[:cut]
	var b strings.Builder
	for _, m := range older {
		role := "用户"
		if m.message.Role == "assistant" {
			role = "助手"
		}
		b.WriteString(role)
		b.WriteString("：")
		b.WriteString(messageText(m))
		b.WriteString("\n")
	}
	prompt := []*schema.Message{
		{Role: schema.System, Content: "你是论文写作助手的上下文压缩器。只根据给定对话生成摘要，不要编造。" +
			"必须保留：研究目标与验收标准、硬约束（引用格式/禁改内容/术语）、已定决策及理由、" +
			"当前断点（章节/文件/版本）、待办事项、被省略内容的主题。" +
			// Google ADK 的实测：摘要里把具体值改写成泛称（如用“部署区域”代替具体区域）后，
			// 下一轮再也拿不回来，召回是全有或全无，所以要求逐字保留。
			"专有名词、数字、文件名、路径、参数值必须逐字保留，不要改写或泛化。用简洁中文条目输出。"},
		{Role: schema.User, Content: b.String()},
	}
	resp, err := chatModel.Generate(ctx, prompt)
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		slog.Warn("context compaction failed; falling back to deterministic trim", "err", err)
		return
	}
	payload := compactionPayload{
		Summary: strings.TrimSpace(resp.Content),
		Dropped: len(older),
		Through: older[len(older)-1].message.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := s.appendEvent(ctx, rc.ConversationID, rc.RunID, compactEventType, payload); err != nil {
		slog.Warn("context compaction event failed", "err", err)
		return
	}
	slog.Info("context compacted", "dropped", payload.Dropped, "window", *rc.Model.ContextWindow)
}

func messageText(m historyMessage) string {
	var b strings.Builder
	for _, p := range m.parts {
		if p.Type == PartText && p.TextContent != nil {
			b.WriteString(*p.TextContent)
		}
	}
	return b.String()
}

// compactionBefore 返回创建时间不晚于 through 的消息应被跳过。
func compactionBefore(p *compactionPayload, createdAt time.Time) bool {
	if p == nil || p.Through == "" {
		return false
	}
	through, err := time.Parse(time.RFC3339Nano, p.Through)
	if err != nil {
		return false
	}
	return !createdAt.After(through)
}

// contextTrimHandler 是 Eino ChatModelAgentMiddleware 的 BeforeModelRewriteState
// 钩子：每次模型调用前按块阈值做确定性裁剪，防止单次 Run 内工具结果把上下文撑爆。
// 跨 Run 的摘要压缩由 maybeCompact 完成并持久化，避免“摘要的摘要”。
type contextTrimHandler struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.Message]
	window int
}

func (h *contextTrimHandler) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.TypedChatModelAgentState[*schema.Message],
	_ *adk.TypedModelContext[*schema.Message],
) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	if h.window <= 0 || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	state.Messages = trimMessages(state.Messages, limitsFor(h.window).block)
	return ctx, state, nil
}

// trimMessages 保留全部 system 消息与最新消息，必要时插入一条省略说明。
func trimMessages(msgs []*schema.Message, budget int) []*schema.Message {
	if budget <= 0 {
		return msgs
	}
	used := 0
	kept := make([]*schema.Message, 0, len(msgs))
	// 从新到旧保留；system 消息无论新旧都保留（它们是规则与摘要）。
	for i := len(msgs) - 1; i >= 0; i-- {
		cost := estimateTokens(msgs[i].Content)
		if msgs[i].Role != schema.System && used+cost > budget && len(kept) > 0 {
			break
		}
		used += cost
		kept = append(kept, msgs[i])
	}
	out := make([]*schema.Message, 0, len(kept)+1)
	for _, m := range msgs {
		if m.Role == schema.System {
			out = append(out, m)
		}
	}
	if len(kept) < len(msgs) {
		out = append(out, &schema.Message{Role: schema.System, Content: "较早的部分消息因上下文长度限制已省略。"})
	}
	for i := len(kept) - 1; i >= 0; i-- {
		if kept[i].Role != schema.System {
			out = append(out, kept[i])
		}
	}
	return out
}
