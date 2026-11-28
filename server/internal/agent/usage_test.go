package agent

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	openai "github.com/meguminnnnnnnnn/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/aiusage"
)

type recordingSink struct {
	mu    sync.Mutex
	calls []aiusage.Call
}

func (s *recordingSink) Record(_ context.Context, call aiusage.Call) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *recordingSink) snapshot() []aiusage.Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]aiusage.Call(nil), s.calls...)
}

type fakeModel struct {
	gen       *schema.Message
	genErr    error
	stream    *schema.StreamReader[*schema.Message]
	streamErr error
}

func (f *fakeModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return f.gen, f.genErr
}

func (f *fakeModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return f.stream, f.streamErr
}

type fakeToolModel struct {
	fakeModel
}

func (f *fakeToolModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return &fakeToolModel{fakeModel: f.fakeModel}, nil
}

func testMeta(sink aiusage.Recorder) usageMeta {
	return usageMeta{
		recorder: sink, featureKind: "chat",
		provider: "DeepSeek", providerSource: "user-settings", model: "deepseek-chat",
	}
}

func TestWrapUsageWithoutRecorderReturnsInner(t *testing.T) {
	inner := &fakeModel{}
	assert.Same(t, inner, wrapUsage(inner, usageMeta{}))
}

func TestGenerateRecordsUsage(t *testing.T) {
	sink := &recordingSink{}
	msg := &schema.Message{
		Content: "hi",
		ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{
			PromptTokens: 10, CompletionTokens: 5,
			CompletionTokensDetails: schema.CompletionTokensDetails{ReasoningTokens: 2},
		}},
	}
	m := wrapUsage(&fakeModel{gen: msg}, testMeta(sink))

	got, err := m.Generate(context.Background(), nil)
	require.NoError(t, err)
	assert.Same(t, msg, got)

	calls := sink.snapshot()
	require.Len(t, calls, 1)
	call := calls[0]
	assert.Equal(t, aiusage.StatusSuccess, call.Status)
	assert.False(t, call.IsStream)
	require.NotNil(t, call.PromptTokens)
	assert.Equal(t, 10, *call.PromptTokens)
	require.NotNil(t, call.CompletionTokens)
	assert.Equal(t, 5, *call.CompletionTokens)
	require.NotNil(t, call.ReasoningTokens)
	assert.Equal(t, 2, *call.ReasoningTokens)
	assert.Nil(t, call.FirstTokenMs)
	assert.GreaterOrEqual(t, call.LatencyMs, 0)
}

func TestGenerateClassifiesProviderError(t *testing.T) {
	sink := &recordingSink{}
	m := wrapUsage(&fakeModel{genErr: &openai.APIError{HTTPStatusCode: 429, Message: "rate limited: secret prompt"}}, testMeta(sink))

	_, err := m.Generate(context.Background(), nil)
	require.Error(t, err)

	calls := sink.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, aiusage.StatusError, calls[0].Status)
	assert.Equal(t, aiusage.CodeRateLimit, calls[0].ErrorCode)
	require.NotNil(t, calls[0].HTTPStatus)
	assert.Equal(t, int16(429), *calls[0].HTTPStatus)
	assert.Nil(t, calls[0].PromptTokens, "错误调用没有 usage 时必须保持 NULL")
}

func TestStreamForwardsChunksAndRecordsUsage(t *testing.T) {
	sink := &recordingSink{}
	chunks := []*schema.Message{
		{Content: ""},
		{Content: "hello"},
		{Content: " world", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 3, CompletionTokens: 2}}},
	}
	m := wrapUsage(&fakeModel{stream: schema.StreamReaderFromArray(chunks)}, testMeta(sink))

	reader, err := m.Stream(context.Background(), nil)
	require.NoError(t, err)
	var got []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		require.NoError(t, recvErr)
		got = append(got, chunk)
	}
	reader.Close()
	assert.Len(t, got, 3)

	calls := sink.snapshot()
	require.Len(t, calls, 1)
	call := calls[0]
	assert.Equal(t, aiusage.StatusSuccess, call.Status)
	assert.True(t, call.IsStream)
	require.NotNil(t, call.FirstTokenMs, "首个有内容的 chunk 应记首字延迟")
	assert.GreaterOrEqual(t, *call.FirstTokenMs, 0)
	require.NotNil(t, call.PromptTokens)
	assert.Equal(t, 3, *call.PromptTokens)
	require.NotNil(t, call.CompletionTokens)
	assert.Equal(t, 2, *call.CompletionTokens)
}

func TestStreamConsumerCloseRecordsCancelled(t *testing.T) {
	sink := &recordingSink{}
	inner, producer := schema.Pipe[*schema.Message](1)
	go func() {
		for i := 0; i < 5; i++ {
			if producer.Send(&schema.Message{Content: "x"}, nil) {
				return
			}
		}
		producer.Close()
	}()
	m := wrapUsage(&fakeModel{stream: inner}, testMeta(sink))

	reader, err := m.Stream(context.Background(), nil)
	require.NoError(t, err)
	_, err = reader.Recv()
	require.NoError(t, err)
	reader.Close()

	require.Eventually(t, func() bool { return len(sink.snapshot()) == 1 }, time.Second, 5*time.Millisecond)
	call := sink.snapshot()[0]
	assert.Equal(t, aiusage.StatusCancelled, call.Status)
	assert.Equal(t, "", call.ErrorCode)
}

func TestStreamErrorRecordsError(t *testing.T) {
	sink := &recordingSink{}
	inner, producer := schema.Pipe[*schema.Message](1)
	go func() {
		producer.Send(&schema.Message{Content: "x"}, nil)
		producer.Send(nil, errors.New("boom"))
		producer.Close()
	}()
	m := wrapUsage(&fakeModel{stream: inner}, testMeta(sink))

	reader, err := m.Stream(context.Background(), nil)
	require.NoError(t, err)
	var recvErr error
	for {
		_, recvErr = reader.Recv()
		if recvErr != nil {
			break
		}
	}
	reader.Close()
	require.Error(t, recvErr)

	require.Eventually(t, func() bool { return len(sink.snapshot()) == 1 }, time.Second, 5*time.Millisecond)
	call := sink.snapshot()[0]
	assert.Equal(t, aiusage.StatusError, call.Status)
	assert.Equal(t, aiusage.CodeUnknown, call.ErrorCode)
}

func TestWrapUsageKeepsToolCallingModel(t *testing.T) {
	sink := &recordingSink{}
	m := wrapUsage(&fakeToolModel{fakeModel: fakeModel{gen: &schema.Message{Content: "ok"}}}, testMeta(sink))

	toolModel, ok := m.(model.ToolCallingChatModel)
	require.True(t, ok, "内层支持工具时包装后仍须满足 ToolCallingChatModel")
	withTools, err := toolModel.WithTools(nil)
	require.NoError(t, err)
	_, err = withTools.Generate(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, sink.snapshot(), 1)
}
