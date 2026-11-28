package aiusage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"testing"

	openai "github.com/meguminnnnnnnnn/go-openai"
	"github.com/stretchr/testify/assert"
)

func int16Ptr(v int16) *int16 { return &v }

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		closed     bool
		wantStatus string
		wantCode   string
		wantHTTP   *int16
	}{
		{name: "success", err: nil, wantStatus: StatusSuccess},
		{name: "consumer closed", err: nil, closed: true, wantStatus: StatusCancelled},
		{name: "cancelled", err: context.Canceled, wantStatus: StatusCancelled},
		{name: "wrapped cancelled", err: fmt.Errorf("run stopped: %w", context.Canceled), wantStatus: StatusCancelled},
		{name: "deadline", err: context.DeadlineExceeded, wantStatus: StatusTimeout},
		{name: "wrapped idle timeout", err: fmt.Errorf("model stream idle: %w", context.DeadlineExceeded), wantStatus: StatusTimeout},
		{name: "auth 401", err: &openai.APIError{HTTPStatusCode: 401}, wantStatus: StatusError, wantCode: CodeAuth, wantHTTP: int16Ptr(401)},
		{name: "auth 403", err: &openai.APIError{HTTPStatusCode: 403}, wantStatus: StatusError, wantCode: CodeAuth, wantHTTP: int16Ptr(403)},
		{name: "rate limit", err: &openai.APIError{HTTPStatusCode: 429}, wantStatus: StatusError, wantCode: CodeRateLimit, wantHTTP: int16Ptr(429)},
		{name: "bad request", err: &openai.RequestError{HTTPStatusCode: 400}, wantStatus: StatusError, wantCode: CodeBadRequest, wantHTTP: int16Ptr(400)},
		{name: "provider 5xx", err: &openai.RequestError{HTTPStatusCode: 503}, wantStatus: StatusError, wantCode: CodeProvider5xx, wantHTTP: int16Ptr(503)},
		{name: "network url error", err: &url.Error{Op: "Post", URL: "https://example.invalid", Err: errors.New("connection refused")}, wantStatus: StatusError, wantCode: CodeNetwork},
		{name: "dns", err: &net.DNSError{Err: "no such host", Name: "example.invalid"}, wantStatus: StatusError, wantCode: CodeNetwork},
		{name: "stream interrupted", err: io.ErrUnexpectedEOF, wantStatus: StatusError, wantCode: CodeStreamInterrupted},
		{name: "unknown", err: errors.New("boom"), wantStatus: StatusError, wantCode: CodeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, httpStatus := Classify(tt.err, tt.closed)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantCode, code)
			if tt.wantHTTP == nil {
				assert.Nil(t, httpStatus)
			} else {
				assert.NotNil(t, httpStatus)
				assert.Equal(t, *tt.wantHTTP, *httpStatus)
			}
		})
	}
}
