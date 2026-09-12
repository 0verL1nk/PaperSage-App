package httpapi

import (
	"errors"
	"net/http"
)

// APIError is the Go counterpart of AppException in backend/app/exceptions.py.
type APIError struct {
	Code int
	Info string
}

func (e *APIError) Error() string { return e.Info }

// NotFound mirrors NotFoundException: 404 "<resource> not found".
func NotFound(resource string) *APIError {
	return &APIError{Code: http.StatusNotFound, Info: resource + " not found"}
}

// Unauthorized mirrors UnauthorizedException: 401.
func Unauthorized(detail string) *APIError {
	if detail == "" {
		detail = "Unauthorized"
	}
	return &APIError{Code: http.StatusUnauthorized, Info: detail}
}

// Forbidden mirrors ForbiddenException: 403.
func Forbidden(detail string) *APIError {
	if detail == "" {
		detail = "Forbidden"
	}
	return &APIError{Code: http.StatusForbidden, Info: detail}
}

// BadRequest mirrors BadRequestException: 400.
func BadRequest(detail string) *APIError {
	if detail == "" {
		detail = "Bad request"
	}
	return &APIError{Code: http.StatusBadRequest, Info: detail}
}

// ErrValidation mirrors validation_exception_handler: fixed 422 message that
// does not leak validation internals.
var ErrValidation = &APIError{Code: http.StatusUnprocessableEntity, Info: "请求参数验证失败，请检查输入"}

// WriteError maps an error to the envelope shape: *APIError keeps its code and
// info, anything else becomes a generic 500 without internal details.
func WriteError(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = &APIError{Code: http.StatusInternalServerError, Info: "Internal server error"}
	}
	_ = WriteEnvelope(w, apiErr.Code, Failure(apiErr.Code, apiErr.Info))
}
