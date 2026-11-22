package sources

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func setupService(t *testing.T) (*Service, *gorm.DB, store.UUID, store.UUID, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sources-svc.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "svc-a", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "svc-b", PasswordHash: "x", BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}).Error)

	src := store.ProjectSource{ProjectID: pid, Kind: "document", CurrentRevision: 1}
	require.NoError(t, db.Create(&src).Error)
	s3Key := "projects/" + pid.String() + "/documents/x.txt"
	require.NoError(t, db.Create(&store.Document{Name: "x.txt", Type: "text/plain", Status: "uploaded",
		S3Key: s3Key, ProjectID: pid, OwnerID: uid, SourceID: &src.ID,
		BaseModel: store.BaseModel{ID: store.MustNewUUID()}}).Error)
	ver := store.SourceVersion{SourceID: src.ID, Revision: 1, S3Key: s3Key,
		ContentSHA256: strings.Repeat("a", 64), SizeBytes: 10}
	require.NoError(t, db.Create(&ver).Error)
	return NewService(db, newMemStorage()), db, uid, other, pid, ver.ID
}

func TestExtractAndReadText(t *testing.T) {
	svc, db, uid, _, pid, vid := setupService(t)
	ctx := context.Background()
	var ver store.SourceVersion
	require.NoError(t, db.First(&ver, "id = ?", vid).Error)
	content := "第一段内容。Second paragraph."
	require.NoError(t, svc.Storage.Put(ctx, ver.S3Key, []byte(content)))

	require.NoError(t, svc.ExtractVersion(ctx, pid, vid))
	info, err := svc.ownedVersion(ctx, uid, pid, vid)
	require.NoError(t, err)
	meta := svc.versionInfo(ctx, info)
	require.NotNil(t, meta.Extraction)
	assert.Equal(t, "ready", meta.Extraction.State)

	text, next, done, err := svc.ReadText(ctx, uid, pid, vid, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, content, text)
	assert.EqualValues(t, len(content), next)
	assert.True(t, done)

	partial, next2, done2, err := svc.ReadText(ctx, uid, pid, vid, 0, 6)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(content, partial))
	assert.EqualValues(t, utf8Len(partial), next2)
	assert.False(t, done2)
}

func TestOwnershipAndMissingExtraction(t *testing.T) {
	svc, _, uid, other, pid, vid := setupService(t)
	ctx := context.Background()
	var ver store.SourceVersion
	require.NoError(t, svc.DB.First(&ver, "id = ?", vid).Error)
	require.NoError(t, svc.Storage.Put(ctx, ver.S3Key, []byte("text")))

	_, _, _, err := svc.ReadText(ctx, other, pid, vid, 0, 0)
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.Code, "别人的版本不能读")

	_, _, _, err = svc.ReadText(ctx, uid, pid, vid, 0, 0)
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code, "未抽取时明确失败而不是返回空文本")
}

func utf8Len(s string) int { return len([]byte(s)) }

type memStorage struct{ data map[string][]byte }

func newMemStorage() *memStorage { return &memStorage{data: map[string][]byte{}} }

func (m *memStorage) Put(_ context.Context, key string, data []byte) error {
	m.data[key] = append([]byte(nil), data...)
	return nil
}

func (m *memStorage) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := m.data[key]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return data, nil
}

func (m *memStorage) Delete(_ context.Context, key string) error { delete(m.data, key); return nil }

func (m *memStorage) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func (m *memStorage) List(context.Context, string) ([]string, error) { return nil, nil }

func (m *memStorage) Exists(_ context.Context, key string) bool { _, ok := m.data[key]; return ok }
