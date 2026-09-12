package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约来源：backend/api/response.py — 信封 {code, data, info}，字段顺序一致
func TestSuccessEnvelopeJSON(t *testing.T) {
	b, err := json.Marshal(Success(map[string]string{"id": "a1"}, ""))
	require.NoError(t, err)
	assert.JSONEq(t, `{"code":200,"data":{"id":"a1"},"info":""}`, string(b))
	// 字段顺序也保持 code,data,info（与 Pydantic 声明顺序一致）
	assert.Equal(t, `{"code":200,"data":{"id":"a1"},"info":""}`, string(b))
}

func TestSuccessEnvelopeWithInfo(t *testing.T) {
	b, err := json.Marshal(Success("ok", "created"))
	require.NoError(t, err)
	assert.Equal(t, `{"code":200,"data":"ok","info":"created"}`, string(b))
}

func TestFailureEnvelopeJSON(t *testing.T) {
	b, err := json.Marshal(Failure(404, "Project not found"))
	require.NoError(t, err)
	// data 为 null（对应 Python data: None）
	assert.Equal(t, `{"code":404,"data":null,"info":"Project not found"}`, string(b))
}

func TestWriteEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, WriteEnvelope(rec, http.StatusCreated, Success("x", "")))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"code":200,"data":"x","info":""}`, rec.Body.String())
}

func TestWriteErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, WriteEnvelope(rec, 401, Failure(401, "Unauthorized")))

	assert.Equal(t, 401, rec.Code)
	assert.JSONEq(t, `{"code":401,"data":null,"info":"Unauthorized"}`, rec.Body.String())
}
