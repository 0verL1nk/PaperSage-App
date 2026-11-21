package agent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdleTimeoutTransportAbortsStalledStream(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	rt := idleTimeoutTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: pr, Header: make(http.Header)}, nil
	}), 50*time.Millisecond)

	resp, err := rt.RoundTrip(httptest.NewRequest(http.MethodPost, "https://example.invalid", nil))
	require.NoError(t, err)
	start := time.Now()
	_, err = resp.Body.Read(make([]byte, 1))
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "idle streams must classify as a timeout")
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestIdleTimeoutTransportResetsWhileReceivingData(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	rt := idleTimeoutTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: pr, Header: make(http.Header)}, nil
	}), 200*time.Millisecond)

	resp, err := rt.RoundTrip(httptest.NewRequest(http.MethodPost, "https://example.invalid", nil))
	require.NoError(t, err)
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(20 * time.Millisecond)
			_, _ = pw.Write([]byte("x"))
		}
		_ = pw.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "xxxxx", string(body))
}
