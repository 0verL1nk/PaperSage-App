package deeplink

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		raw       string
		want      Target
		wantError bool
	}{
		{raw: "papersage://project/abc", want: Target{ProjectID: "abc"}},
		{raw: "papersage://project/abc/writing", want: Target{ProjectID: "abc", Stage: "writing"}},
		{raw: "papersage://project/3f2a9c1e-4b7d-4e2a-9c1e-4b7d4e2a9c1e/review", want: Target{ProjectID: "3f2a9c1e-4b7d-4e2a-9c1e-4b7d4e2a9c1e", Stage: "review"}},
		{raw: "papersage://project/abc/PREPARE", want: Target{ProjectID: "abc", Stage: "prepare"}},
		{raw: "papersage://project/../../etc", wantError: true},
		{raw: "papersage://project/abc/unknown", wantError: true},
		{raw: "papersage://project/", wantError: true},
		{raw: "papersage://other/abc", wantError: true},
		{raw: "papersage:///project/abc", wantError: true},
		{raw: "https://project/abc", wantError: true},
		{raw: "papersage://project/abc/writing/extra", wantError: true},
		{raw: "papersage://project/" + longID(65), wantError: true},
		{raw: "papersage://project/abc%2Fdef", wantError: true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.raw)
		if tt.wantError {
			if err == nil {
				t.Errorf("%q: expected error, got %+v", tt.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%q: got %+v, want %+v", tt.raw, got, tt.want)
		}
	}
}

func longID(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
