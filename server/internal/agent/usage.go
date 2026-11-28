package agent

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/0verL1nk/GraduateProject/server/internal/aiusage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// usageMeta 把一次 Run 的归属与模型快照带进装饰器；子 agent / 压缩 / 摘要
// 共用同一实例，因此全部计入同一 Run。
type usageMeta struct {
	recorder       aiusage.Recorder
	userID         store.UUID
	projectID      store.UUID
	runID          store.UUID
	featureKind    string
	provider       string
	providerSource string
	model          string
}

// wrapUsage 在唯一模型调用链路上做计量：Generate 记总耗时，Stream 额外记
// 首字延迟并保留末帧 usage。recorder 未注入时原样返回（测试/未装配场景）。
func wrapUsage(inner model.BaseChatModel, meta usageMeta) model.BaseChatModel {
	if inner == nil || meta.recorder == nil {
		return inner
	}
	um := &usageModel{inner: inner, meta: meta}
	if tool, ok := inner.(model.ToolCallingChatModel); ok {
		return &usageToolModel{usageModel: um, tool: tool}
	}
	return um
}

type usageModel struct {
	inner model.BaseChatModel
	meta  usageMeta
}

func (m *usageModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	start := time.Now()
	msg, err := m.inner.Generate(ctx, input, opts...)
	m.record(ctx, false, start, msg, err, false, nil)
	return msg, err
}

func (m *usageModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	start := time.Now()
	inner, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		m.record(ctx, true, start, nil, err, false, nil)
		return nil, err
	}
	out, writer := schema.Pipe[*schema.Message](0)
	go m.forward(ctx, start, inner, writer)
	return out, nil
}

// forward 逐块转发内层流，记录首字时间与末帧 usage。
// 先落库再 Close writer：消费者看到 EOF 时记录已写入。
func (m *usageModel) forward(ctx context.Context, start time.Time, inner *schema.StreamReader[*schema.Message], writer *schema.StreamWriter[*schema.Message]) {
	defer inner.Close()
	var (
		usage          *schema.TokenUsage
		firstToken     *int
		streamErr      error
		consumerClosed bool
	)
	for {
		chunk, recvErr := inner.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			streamErr = recvErr
			break
		}
		if firstToken == nil && isMeaningfulChunk(chunk) {
			ms := int(time.Since(start).Milliseconds())
			firstToken = &ms
		}
		if chunk != nil && chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
			usage = chunk.ResponseMeta.Usage
		}
		if writer.Send(chunk, nil) {
			consumerClosed = true
			break
		}
	}
	if streamErr != nil {
		_ = writer.Send(nil, streamErr)
	}
	var msg *schema.Message
	if usage != nil {
		msg = &schema.Message{ResponseMeta: &schema.ResponseMeta{Usage: usage}}
	}
	m.record(ctx, true, start, msg, streamErr, consumerClosed, firstToken)
	writer.Close()
}

func isMeaningfulChunk(chunk *schema.Message) bool {
	if chunk == nil {
		return false
	}
	return chunk.Content != "" || chunk.ReasoningContent != "" || len(chunk.ToolCalls) > 0
}

func (m *usageModel) record(ctx context.Context, isStream bool, start time.Time,
	msg *schema.Message, err error, consumerClosed bool, firstToken *int) {
	if m == nil || m.meta.recorder == nil {
		return
	}
	call := aiusage.Call{
		UserID:         m.meta.userID,
		ProjectID:      m.meta.projectID,
		RunID:          m.meta.runID,
		FeatureKind:    m.meta.featureKind,
		Provider:       m.meta.provider,
		ProviderSource: m.meta.providerSource,
		Model:          m.meta.model,
		IsStream:       isStream,
		LatencyMs:      int(time.Since(start).Milliseconds()),
		FirstTokenMs:   firstToken,
	}
	call.Status, call.ErrorCode, call.HTTPStatus = aiusage.Classify(err, consumerClosed)
	if err == nil && msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		u := msg.ResponseMeta.Usage
		prompt, completion, reasoning := u.PromptTokens, u.CompletionTokens, u.CompletionTokensDetails.ReasoningTokens
		call.PromptTokens = &prompt
		call.CompletionTokens = &completion
		call.ReasoningTokens = &reasoning
	}
	m.meta.recorder.Record(ctx, call)
}

// usageToolModel 同时满足 ToolCallingChatModel，WithTools 后继续包一层。
type usageToolModel struct {
	*usageModel
	tool model.ToolCallingChatModel
}

func (m *usageToolModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	inner, err := m.tool.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &usageToolModel{usageModel: &usageModel{inner: inner, meta: m.meta}, tool: inner}, nil
}
