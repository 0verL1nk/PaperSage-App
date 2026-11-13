package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateExternalURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "http", url: "http://example.com"},
		{name: "https with path and query", url: "https://example.com/a?b=c#d"},
		{name: "uppercase scheme", url: "HTTPS://example.com"},
		{name: "surrounding whitespace", url: "  https://example.com  "},
		{name: "mailto address", url: "mailto:user@example.com"},
		{name: "mailto with subject", url: "mailto:user@example.com?subject=Hi"},
		{name: "mailto multiple recipients", url: "mailto:a@example.com,b@example.com"},
		{name: "empty", url: "", wantErr: true},
		{name: "relative", url: "example.com", wantErr: true},
		{name: "ftp", url: "ftp://example.com", wantErr: true},
		{name: "file", url: "file:///etc/passwd", wantErr: true},
		{name: "javascript", url: "javascript:alert(1)", wantErr: true},
		{name: "data", url: "data:text/html,<h1>x</h1>", wantErr: true},
		{name: "http without host", url: "http://", wantErr: true},
		{name: "https without host", url: "https:/foo", wantErr: true},
		{name: "mailto without address", url: "mailto:", wantErr: true},
		{name: "mailto only query", url: "mailto:?subject=Hi", wantErr: true},
		{name: "raw newline", url: "https://example.com/\r\nHeader:x", wantErr: true},
		{name: "scheme only", url: "https:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExternalURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateExternalURL(%q) error = %v, wantErr = %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "existing directory", path: dir, want: dir},
		{name: "existing file", path: file, want: file},
		{name: "surrounding whitespace", path: "  " + file + "  ", want: file},
		{name: "empty", path: "", wantErr: true},
		{name: "relative", path: "sample.txt", wantErr: true},
		{name: "dot relative", path: "./sample.txt", wantErr: true},
		{name: "nonexistent absolute", path: filepath.Join(dir, "missing.txt"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAbsolutePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAbsolutePath(%q) error = %v, wantErr = %v", tt.path, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("validateAbsolutePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
