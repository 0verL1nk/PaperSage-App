package updater

import (
	"testing"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
)

func TestStateWithoutSetup(t *testing.T) {
	if got := State(); got != wailsupdater.StateUnconfigured {
		t.Fatalf("State() = %q, want %q", got, wailsupdater.StateUnconfigured)
	}
}

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{name: "value payload", data: wailsupdater.ErrorInfo{Message: "checksum mismatch"}, want: "checksum mismatch"},
		{name: "pointer payload", data: &wailsupdater.ErrorInfo{Message: "download failed"}, want: "download failed"},
		{name: "nil pointer", data: (*wailsupdater.ErrorInfo)(nil), want: ""},
		{name: "nil", data: nil, want: ""},
		{name: "unknown payload", data: "boom", want: "boom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorMessage(tt.data); got != tt.want {
				t.Fatalf("errorMessage(%v) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}
