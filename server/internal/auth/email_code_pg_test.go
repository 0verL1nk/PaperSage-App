//go:build integration

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 并发一次性核销（真实 PostgreSQL）：同一验证码被并发提交时必须只有一个成功。
// SQLite 会忽略 SELECT ... FOR UPDATE，串行结果不能代表并发安全，故用 PG。
func TestEmailCodeConcurrentVerifyConsumesOnce(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	schema := fmt.Sprintf("auth_it_%d", time.Now().UnixNano())

	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	schemaDSN := withSearchPath(dsn, schema)
	require.NoError(t, store.MigrateUp(context.Background(), schemaDSN))

	db, err := gorm.Open(postgres.Open(schemaDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	mailer := &captureSender{}
	svc := NewService(db, &config.Settings{
		SecretKey:                testSecret,
		AccessTokenExpireMinutes: 30,
		RefreshTokenExpireDays:   7,
	}, mailer)
	api := httpapi.New(nil)
	Mount(api.Group(httpapi.APIVersion), svc)
	srv := httptest.NewServer(api)
	defer srv.Close()

	email := fmt.Sprintf("race-%d@hit.edu.cn", time.Now().UnixNano())
	payload, err := json.Marshal(map[string]string{"email": email})
	require.NoError(t, err)
	sendResp, err := http.Post(srv.URL+"/api/v1/auth/email-code/send", "application/json", bytes.NewReader(payload))
	require.NoError(t, err)
	_ = sendResp.Body.Close()
	require.Equal(t, http.StatusOK, sendResp.StatusCode)
	code := latestLoginCode(t, mailer)

	const concurrency = 4
	var wg sync.WaitGroup
	start := make(chan struct{})
	statuses := make([]int, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]string{"email": email, "code": code})
			<-start
			resp, err := http.Post(srv.URL+"/api/v1/auth/email-code/verify", "application/json", bytes.NewReader(body))
			if err != nil {
				statuses[i] = -1
				return
			}
			_ = resp.Body.Close()
			statuses[i] = resp.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()

	success := 0
	for _, status := range statuses {
		if status == http.StatusOK {
			success++
		}
	}
	assert.Equal(t, 1, success, "并发核销同一验证码只能有一个成功，实际状态=%v", statuses)
}

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}
