package agent

import (
	"context"
	"encoding/json"

	"gorm.io/datatypes"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// transcriptPart 是 assistant 消息的一条持久化 part。
// 实时事件与历史转录共用同一套 part 语义：text 按 id 聚合，
// tool 按 toolCallId 区分，proposal 只带建议 id 与展示快照。
type transcriptPart struct {
	Type    string
	Status  string
	Text    *string
	Content datatypes.JSON
}

type transcriptBuilder struct {
	parts     []transcriptPart
	textIndex map[string]int
	toolIndex map[string]int
}

func (b *transcriptBuilder) add(p transcriptPart) int {
	b.parts = append(b.parts, p)
	return len(b.parts) - 1
}

func (b *transcriptBuilder) addText(id, delta string) {
	if i, ok := b.textIndex[id]; ok {
		if b.parts[i].Text == nil {
			b.parts[i].Text = new(string)
		}
		*b.parts[i].Text += delta
		return
	}
	text := delta
	b.textIndex[id] = b.add(transcriptPart{Type: PartText, Status: "completed", Text: &text})
}

func (b *transcriptBuilder) addTool(toolCallID, toolName, input string) {
	if toolCallID == "" {
		toolCallID = toolName
	}
	if _, ok := b.toolIndex[toolCallID]; ok {
		return
	}
	content, _ := json.Marshal(map[string]any{"toolName": toolName, "input": input})
	b.toolIndex[toolCallID] = b.add(transcriptPart{
		Type: PartTool, Status: "executing", Content: content,
	})
}

func (b *transcriptBuilder) finishTool(toolCallID, output, status string) {
	i, ok := b.toolIndex[toolCallID]
	if !ok {
		return
	}
	var meta map[string]any
	_ = json.Unmarshal(b.parts[i].Content, &meta)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["output"] = output
	b.parts[i].Content, _ = json.Marshal(meta)
	if status == "failed" {
		b.parts[i].Status = ToolFailed
		return
	}
	b.parts[i].Status = "succeeded"
}

// buildTranscript 从事件事实重建 assistant 转录。
// 子 agent 的 silent 事件写在子 Run 名下，不会出现在这里。
func (s *Service) buildTranscript(ctx context.Context, runID store.UUID, fallback, failure string) ([]transcriptPart, error) {
	var events []store.AgentEvent
	if err := s.DB.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("sequence ASC").Find(&events).Error; err != nil {
		return nil, err
	}
	var calls []store.AgentToolCall
	if err := s.DB.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("seq ASC").Find(&calls).Error; err != nil {
		return nil, err
	}

	b := &transcriptBuilder{textIndex: map[string]int{}, toolIndex: map[string]int{}}
	used := make([]bool, len(calls))
	claim := func(name, input string) *store.AgentToolCall {
		for i := range calls {
			if used[i] || calls[i].ToolName != name {
				continue
			}
			if input != "" && string(calls[i].InputJSON) != input {
				continue
			}
			used[i] = true
			return &calls[i]
		}
		return nil
	}
	str := func(p map[string]any, k string) string {
		v, _ := p[k].(string)
		return v
	}
	for _, ev := range events {
		var p map[string]any
		if len(ev.PayloadJSON) > 0 {
			_ = json.Unmarshal(ev.PayloadJSON, &p)
		}
		switch ev.EventType {
		case evtTextDelta:
			b.addText(str(p, "id"), str(p, "delta"))
		case evtToolInput:
			id, name := str(p, "toolCallId"), str(p, "toolName")
			b.addTool(id, name, str(p, "input"))
			if row := claim(name, str(p, "input")); row != nil {
				if i, ok := b.toolIndex[id]; ok {
					b.parts[i].Status = row.State
				}
			}
		case evtToolOutput:
			b.finishTool(str(p, "toolCallId"), str(p, "output"), str(p, "status"))
		case evtActivity:
			content, _ := json.Marshal(p)
			status := str(p, "status")
			if status == "" {
				status = "completed"
			}
			b.add(transcriptPart{Type: PartActivity, Status: status, Content: content})
		case evtProposal:
			content, _ := json.Marshal(p)
			status := str(p, "status")
			if status == "" {
				status = "pending"
			}
			b.add(transcriptPart{Type: PartProposal, Status: status, Content: content})
		case evtRunError:
			msg := str(p, "message")
			b.add(transcriptPart{Type: PartError, Status: "failed", Text: &msg})
		}
	}
	if len(b.parts) == 0 && fallback != "" {
		text := fallback
		b.parts = append(b.parts, transcriptPart{Type: PartText, Status: "completed", Text: &text})
	}
	if failure != "" {
		hasError := false
		for _, p := range b.parts {
			if p.Type == PartError {
				hasError = true
				break
			}
		}
		if !hasError {
			msg := failure
			b.parts = append(b.parts, transcriptPart{Type: PartError, Status: "failed", Text: &msg})
		}
	}
	// 没有 output 的工具在 finalize 时会被收敛为 failed；转录不能留下 executing。
	for i := range b.parts {
		if b.parts[i].Type == PartTool && b.parts[i].Status == ToolExecuting {
			b.parts[i].Status = ToolFailed
		}
	}
	return b.parts, nil
}
