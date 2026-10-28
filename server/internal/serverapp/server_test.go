package serverapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/bootstrap"
)

func TestShutdownKeepsDatabaseOpenUntilAgentDrain(t *testing.T) {
	for _, drainErr := range []error{nil, errors.New("terminal persistence failed")} {
		t.Run("drain", func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(t.TempDir()+"/shutdown.db"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			entered, release := make(chan struct{}), make(chan struct{})
			app := &bootstrap.App{DB: db, Close: func(ctx context.Context) error {
				close(entered)
				<-release
				return errors.Join(drainErr, sqlDB.PingContext(ctx))
			}}
			done := make(chan error, 1)
			go func() { done <- shutdown(context.Background(), &http.Server{}, app) }()
			<-entered
			require.NoError(t, sqlDB.Ping(), "DB must be usable for the final Run transaction")
			close(release)
			err = <-done
			if drainErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, drainErr)
			}
			require.Error(t, sqlDB.Ping(), "DB closes after both drains, even on failure")
		})
	}
}

func TestShutdownForcesHTTPClosedOnDeadline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/shutdown.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	entered, disconnected := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(disconnected)
	}))
	defer srv.Close()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, err := http.Get(srv.URL)
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Errorf("close response body: %v", closeErr)
			}
		}
	}()
	<-entered
	app := &bootstrap.App{DB: db, Close: func(ctx context.Context) error {
		<-ctx.Done()
		return sqlDB.Ping() // final persistence has an independent context
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, shutdown(ctx, srv.Config, app), context.DeadlineExceeded)
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("timed-out HTTP connection was not closed")
	}
	<-clientDone
	require.Error(t, sqlDB.Ping())
}
