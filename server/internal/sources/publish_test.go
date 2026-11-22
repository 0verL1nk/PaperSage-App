package sources

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func setup(t *testing.T) (*gorm.DB, store.UUID, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sources.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "sources-a", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	did := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Document{Name: "a.pdf", Type: "application/pdf",
		Status: "uploaded", S3Key: "k", ProjectID: pid, OwnerID: uid,
		BaseModel: store.BaseModel{ID: did}}).Error)
	return db, uid, pid, did
}

func TestPublishDocumentCreatesVersionAndLink(t *testing.T) {
	db, uid, pid, did := setup(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return PublishDocument(tx, pid, did, uid, "k", strings.Repeat("a", 64), 42)
	})
	require.NoError(t, err)

	var doc store.Document
	require.NoError(t, db.First(&doc, "id = ?", did).Error)
	require.NotNil(t, doc.SourceID)

	var src store.ProjectSource
	require.NoError(t, db.First(&src, "id = ?", *doc.SourceID).Error)
	assert.Equal(t, pid, src.ProjectID)
	assert.EqualValues(t, 1, src.CurrentRevision)

	var ver store.SourceVersion
	require.NoError(t, db.First(&ver, "source_id = ?", src.ID).Error)
	assert.EqualValues(t, 1, ver.Revision)
	assert.EqualValues(t, 42, ver.SizeBytes)

	// 幂等：重复发布不新建来源。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return PublishDocument(tx, pid, did, uid, "k", strings.Repeat("a", 64), 42)
	}))
	var count int64
	require.NoError(t, db.Model(&store.ProjectSource{}).Where("project_id = ?", pid).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
