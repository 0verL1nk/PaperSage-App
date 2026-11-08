package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/stretchr/testify/require"
)

func TestToolFailureBecomesModelReadableResult(t *testing.T) {
	tracker := &toolFailureTracker{}
	next := tracker.middleware(func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, errors.New("NoSuchKey: main.tex")
	})
	out, err := next(context.Background(), &compose.ToolInput{Name: "read_latex_file", CallID: "c1"})
	require.NoError(t, err)
	require.Contains(t, out.Result, "NoSuchKey: main.tex")
	require.True(t, tracker.failedCall("c1"))
}

func TestToolFailureRedactsCredentials(t *testing.T) {
	tracker := &toolFailureTracker{}
	next := tracker.middleware(func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, errors.New(`dial postgres://user:secret@db:5432/app failed`)
	})
	out, err := next(context.Background(), &compose.ToolInput{Name: "rag_search", CallID: "c2"})
	require.NoError(t, err)
	require.NotContains(t, out.Result, "secret")
	require.Contains(t, out.Result, "***:***@")
}

func TestToolCancellationStillAbortsRun(t *testing.T) {
	tracker := &toolFailureTracker{}
	next := tracker.middleware(func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, context.Canceled
	})
	_, err := next(context.Background(), &compose.ToolInput{Name: "read_latex_file", CallID: "c3"})
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, tracker.failedCall("c3"))
}

func TestToolFailureDoesNotMarkSuccessfulCall(t *testing.T) {
	tracker := &toolFailureTracker{}
	next := tracker.middleware(func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		return &compose.ToolOutput{Result: "工具执行失败：this is file content"}, nil
	})
	out, err := next(context.Background(), &compose.ToolInput{Name: "read_latex_file", CallID: "c4"})
	require.NoError(t, err)
	require.Equal(t, "工具执行失败：this is file content", out.Result)
	require.False(t, tracker.failedCall("c4"), "content must not be used as a failure signal")
}
