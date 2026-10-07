package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// 传输层：AI SDK UI Message Stream Protocol。
//
// 为什么对齐它而不是继续自造事件：该协议把「文本/工具/审批/引用」都定义为
// 有 ID、有生命周期的 part，前端可以有现成的状态机与组件切分参照，也不必
// 让模型供应方的原生事件格式渗进业务层。响应头必须带 streamVersionHeader。
//
// 该协议没有 sequence（顺序号留在服务端的 agent_events 里），因此断线恢复走
// 「拉消息快照 → 重放该 Run 已产生的 parts」，由前端 reducer 按 part id 幂等归并。
const (
	// streamVersionHeader 是该协议的版本协商头，缺失则客户端不会按此协议解析。
	streamVersionHeader = "x-vercel-ai-ui-message-stream"
	streamVersion       = "v1"
)

// Part 类型（协议词汇）。
const (
	evStart       = "start"
	evTextStart   = "text-start"
	evTextDelta   = "text-delta"
	evTextEnd     = "text-end"
	evStartStep   = "start-step"
	evFinishStep  = "finish-step"
	evToolInStart = "tool-input-start"
	evToolInAvail = "tool-input-available"
	evToolOut     = "tool-output-available"
	evToolDenied  = "tool-output-denied"
	evDataPrefix  = "data-" // 业务自定义：data-conversation / data-proposal / …
	evError       = "error"
	evFinish      = "finish"
	evAbort       = "abort"
)

// streamWriter 以 SSE 推送 UI Message Stream part。
type streamWriter struct {
	w        gin.ResponseWriter
	flusher  http.Flusher
	stepOpen bool
}

func newStreamWriter(c *gin.Context) (*streamWriter, bool) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, false
	}
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	h.Set(streamVersionHeader, streamVersion)
	return &streamWriter{w: c.Writer, flusher: flusher}, true
}

// part 推一个 part。字段用 omitempty，未用到的字段不出现在报文里。
func (s *streamWriter) part(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", b)
	s.flusher.Flush()
}

func (s *streamWriter) Start(messageID string) {
	s.part(map[string]any{"type": evStart, "messageId": messageID})
}

func (s *streamWriter) StartStep() {
	if !s.stepOpen {
		s.part(map[string]any{"type": evStartStep})
		s.stepOpen = true
	}
}

func (s *streamWriter) FinishStep() {
	if s.stepOpen {
		s.part(map[string]any{"type": evFinishStep})
		s.stepOpen = false
	}
}

func (s *streamWriter) TextStart(id string) {
	s.StartStep()
	s.part(map[string]any{"type": evTextStart, "id": id})
}

func (s *streamWriter) TextDelta(id, delta string) {
	s.part(map[string]any{"type": evTextDelta, "id": id, "delta": delta})
}

func (s *streamWriter) TextEnd(id string) {
	s.part(map[string]any{"type": evTextEnd, "id": id})
}

func (s *streamWriter) ToolInput(toolCallID, toolName string) {
	s.StartStep()
	s.part(map[string]any{
		"type": evToolInStart, "toolCallId": toolCallID, "toolName": toolName,
	})
}

func (s *streamWriter) ToolInputAvailable(toolCallID, toolName string, input string) {
	// input 在协议里是对象；模型给的是 JSON 文本，能解析就内联，否则原样字符串。
	var parsed any
	if input != "" && json.Unmarshal([]byte(input), &parsed) == nil {
		s.part(map[string]any{
			"type": evToolInAvail, "toolCallId": toolCallID, "toolName": toolName, "input": parsed,
		})
		return
	}
	s.part(map[string]any{
		"type": evToolInAvail, "toolCallId": toolCallID, "toolName": toolName, "input": input,
	})
}

func (s *streamWriter) ToolOutput(toolCallID, result string) {
	s.part(map[string]any{"type": evToolOut, "toolCallId": toolCallID, "output": result})
}

// Data 推一条业务自定义 part（data-<name>）。
func (s *streamWriter) Data(name string, payload any) {
	s.part(map[string]any{"type": evDataPrefix + name, "data": payload})
}

func (s *streamWriter) Error(text string) {
	s.part(map[string]any{"type": evError, "errorText": text})
}

// Finish 收尾：关掉未结束的 step，推 finish 与终止标记。
func (s *streamWriter) Finish() {
	s.FinishStep()
	s.part(map[string]any{"type": evFinish})
	_, _ = fmt.Fprint(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}

// StreamKeepAlive 心跳间隔。SSE 注释行不计入事件，但能让代理不把空闲连接回收
// （参考架构 §3.3：每 15–30 秒发一次心跳）。
const streamKeepAlive = 20 * time.Second

// ping 发 SSE 注释行作为心跳。注释不是事件，客户端解析器会忽略它。
func (s *streamWriter) ping() {
	_, _ = fmt.Fprint(s.w, ": ping\n\n")
	s.flusher.Flush()
}

// Abort 客户端/服务端取消。
func (s *streamWriter) Abort(reason string) {
	s.FinishStep()
	s.part(map[string]any{"type": evAbort, "reason": reason})
	_, _ = fmt.Fprint(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}
