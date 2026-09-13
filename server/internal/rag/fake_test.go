package rag

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newFakeEmbedServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]any{"object": "list"}
		data := make([]any, len(req.Input))
		for i, s := range req.Input {
			// 稳定向量：由首字符决定，长度 3
			v := []float64{float64(len(s)) * 0.01, 0.2, 0.3}
			data[i] = map[string]any{"index": i, "embedding": v}
		}
		resp["data"] = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
}
