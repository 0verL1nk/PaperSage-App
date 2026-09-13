package syssettings_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

func TestDbgNormalUser(t *testing.T) {
	ts := testutil.New(t)
	var isSuper bool
	ts.DB.Raw("SELECT is_superuser FROM users WHERE id = ?", ts.UserID().String()).Scan(&isSuper)
	t.Logf("seeded user is_superuser=%v", isSuper)
	resp := ts.Do(t, http.MethodGet, "/api/v1/admin/settings", nil)
	body, _ := io.ReadAll(resp.Body)
	n := len(body)
	if n > 150 {
		n = 150
	}
	t.Logf("GET => %d body=%s", resp.StatusCode, string(body[:n]))
}
