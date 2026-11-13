package modelregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshAndLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"p1": {"models": {"m1": {"id":"m1","tool_call":true,"modalities":{"input":["text","image"]},"limit":{"context":128000,"output":8192}}}},
			"p2": {"models": {
				"m1": {"id":"m1","tool_call":false,"limit":{"context":64000,"output":4096}},
				"custom/x": {"id":"custom/x","tool_call":true,"limit":{"context":32000,"output":2048}},
					"att-only": {"id":"att-only","attachment":true}
			}}
		}`))
	}))
	defer srv.Close()

	var reg Registry
	if err := reg.Refresh(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	m, ok := reg.Lookup("m1")
	if !ok || m.ContextWindow == nil || *m.ContextWindow != 128000 {
		t.Fatalf("duplicate ids should keep the larger window: %+v ok=%v", m, ok)
	}
	if m.SupportsVision == nil || !*m.SupportsVision {
		t.Fatalf("image modality should mark vision support: %+v", m)
	}
	if m.SupportsTools == nil || !*m.SupportsTools {
		t.Fatalf("tool_call should be preserved: %+v", m)
	}
	if x, ok := reg.Lookup("custom/x"); !ok || x.ContextWindow == nil || *x.ContextWindow != 32000 {
		t.Fatalf("exact id lookup failed: %+v ok=%v", x, ok)
	}
	if x, ok := reg.Lookup("x"); !ok || x.ContextWindow == nil || *x.ContextWindow != 32000 {
		t.Fatalf("provider-prefixed fallback failed: %+v ok=%v", x, ok)
	}
	if _, ok := reg.Lookup("unknown-model"); ok {
		t.Fatal("unknown model must stay unknown")
	}
	if a, ok := reg.Lookup("att-only"); !ok || a.SupportsVision == nil || !*a.SupportsVision {
		t.Fatalf("attachment without modalities should imply vision: %+v ok=%v", a, ok)
	}
}

func TestRefreshFailureKeepsPreviousSnapshot(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"p":{"models":{"m":{"id":"m","limit":{"context":1000}}}}}`))
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer badSrv.Close()

	var reg Registry
	if err := reg.Refresh(context.Background(), okSrv.URL); err != nil {
		t.Fatal(err)
	}
	if err := reg.Refresh(context.Background(), badSrv.URL); err == nil {
		t.Fatal("expected refresh error")
	}
	if m, ok := reg.Lookup("m"); !ok || m.ContextWindow == nil || *m.ContextWindow != 1000 {
		t.Fatalf("failed refresh must keep the previous snapshot: %+v ok=%v", m, ok)
	}
}
