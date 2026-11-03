package latex_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

func collisionDB(t *testing.T) (*gorm.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&store.User{}, &store.Project{}, &store.LatexFile{}))
	user := store.User{Username: "test", PasswordHash: "test"}
	require.NoError(t, db.Create(&user).Error)
	project := store.Project{Name: "test", OwnerID: user.ID}
	require.NoError(t, db.Create(&project).Error)
	return db, project.ID.UUID, user.ID.UUID
}

func TestRecreateRenamedFileKeepsBothObjects(t *testing.T) {
	db, project, user := collisionDB(t)
	svc, err := latex.NewFilesService(db, testutil.NewMemStorage())
	require.NoError(t, err)
	ctx := context.Background()
	_, err = svc.CreateFileBinary(ctx, project, "a.png", []byte("original"), "application/octet-stream", user)
	require.NoError(t, err)
	_, err = svc.RenameFile(ctx, project, "a.png", "b.png", user)
	require.NoError(t, err)
	_, err = svc.CreateFileBinary(ctx, project, "a.png", []byte("replacement"), "application/octet-stream", user)
	require.NoError(t, err)

	for _, tc := range []struct{ path, want string }{{"a.png", "replacement"}, {"b.png", "original"}} {
		t.Run(tc.path, func(t *testing.T) {
			content, _, err := svc.GetFileRaw(ctx, project, tc.path, user)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(content))
		})
	}
}

type gatedStorage struct {
	*testutil.MemStorage
	firstWritten, secondWritten chan struct{}
	releaseFirst, releaseSecond chan struct{}
}

func (s *gatedStorage) Put(ctx context.Context, key string, data []byte) error {
	if err := s.MemStorage.Put(ctx, key, data); err != nil {
		return err
	}
	var written, release chan struct{}
	if string(data) == "winner" {
		written, release = s.firstWritten, s.releaseFirst
	} else {
		written, release = s.secondWritten, s.releaseSecond
	}
	close(written)
	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestLoadTextContentsFailsWhenAnInputIsMissing(t *testing.T) {
	db, project, _ := collisionDB(t)
	svc, err := latex.NewFilesService(db, testutil.NewMemStorage())
	require.NoError(t, err)
	key := "missing-object"
	file := store.LatexFile{
		ProjectID: store.UUID{UUID: project}, FilePath: "main.tex", S3Key: &key,
	}
	require.NoError(t, db.Create(&file).Error)

	_, err = svc.LoadTextContents(context.Background(), project, 2)
	require.ErrorContains(t, err, "main.tex")
}

func TestConcurrentCreateFailureKeepsWinnerObject(t *testing.T) {
	db, project, user := collisionDB(t)
	storage := &gatedStorage{
		MemStorage:   testutil.NewMemStorage(),
		firstWritten: make(chan struct{}), secondWritten: make(chan struct{}),
		releaseFirst: make(chan struct{}), releaseSecond: make(chan struct{}),
	}
	svc, err := latex.NewFilesService(db, storage)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	firstResult, secondResult := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := svc.CreateFileBinary(ctx, project, "same.png", []byte("winner"), "application/octet-stream", user)
		firstResult <- err
	}()
	select {
	case <-storage.firstWritten:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		_, err := svc.CreateFileBinary(ctx, project, "same.png", []byte("loser"), "application/octet-stream", user)
		secondResult <- err
	}()
	select {
	case <-storage.secondWritten:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// 让第一个请求完成数据库写入成为赢家，再放行失败者及其对象补偿。
	close(storage.releaseFirst)
	require.NoError(t, <-firstResult)
	close(storage.releaseSecond)
	require.Error(t, <-secondResult)
	content, _, err := svc.GetFileRaw(ctx, project, "same.png", user)
	require.NoError(t, err)
	assert.Equal(t, "winner", string(content))
}
