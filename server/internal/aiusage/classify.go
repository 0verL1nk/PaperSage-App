package aiusage

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"

	openai "github.com/meguminnnnnnnnn/go-openai"
)

// 调用状态与错误分类枚举。只落枚举与 HTTP 状态码：provider 的原始错误消息
// 可能回显提示词或论文片段，绝不能入库。
const (
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusTimeout   = "timeout"
	StatusCancelled = "cancelled"
)

const (
	CodeAuth              = "auth"
	CodeRateLimit         = "rate_limit"
	CodeBadRequest        = "bad_request"
	CodeProvider5xx       = "provider_5xx"
	CodeNetwork           = "network"
	CodeStreamInterrupted = "stream_interrupted"
	CodeUnknown           = "unknown"
)

// Classify 把一次模型调用的结果映射为 (status, error_code, http_status)。
// consumerClosed 表示流的使用方提前关闭（例如 Run 被取消），按 cancelled 记录。
func Classify(err error, consumerClosed bool) (string, string, *int16) {
	if consumerClosed {
		return StatusCancelled, "", nil
	}
	if err == nil {
		return StatusSuccess, "", nil
	}
	if errors.Is(err, context.Canceled) {
		return StatusCancelled, "", nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return StatusTimeout, "", nil
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return statusForHTTP(apiErr.HTTPStatusCode)
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return statusForHTTP(reqErr.HTTPStatusCode)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StatusTimeout, "", nil
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return StatusError, CodeNetwork, nil
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return StatusTimeout, "", nil
		}
		return StatusError, CodeNetwork, nil
	}

	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return StatusError, CodeStreamInterrupted, nil
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return StatusError, CodeNetwork, nil
	}
	return StatusError, CodeUnknown, nil
}

func statusForHTTP(code int) (string, string, *int16) {
	var httpStatus *int16
	if code > 0 && code <= 32767 {
		v := int16(code)
		httpStatus = &v
	}
	switch {
	case code == 401 || code == 403:
		return StatusError, CodeAuth, httpStatus
	case code == 429:
		return StatusError, CodeRateLimit, httpStatus
	case code == 400 || code == 404 || code == 422:
		return StatusError, CodeBadRequest, httpStatus
	case code >= 500 && code <= 599:
		return StatusError, CodeProvider5xx, httpStatus
	}
	return StatusError, CodeUnknown, httpStatus
}
