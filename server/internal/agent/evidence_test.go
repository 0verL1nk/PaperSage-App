package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	testing "testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestPersistRunEvidenceIsVersionedAndIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/evidence.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	userID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "evidence-owner", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: userID}}).Error)
	projectID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "evidence-project", OwnerID: userID,
		BaseModel: store.BaseModel{ID: projectID}}).Error)
	conversationID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentConversation{ID: conversationID, ProjectID: projectID,
		UserID: userID, Title: "evidence"}).Error)
	runID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentRun{ID: runID, ConversationID: conversationID,
		AgentKey: "main", Status: "completed", ModelProvider: "test", ModelName: "test",
		PromptVersion: "v1"}).Error)

	hit := rag.Hit{
		ChunkID:    "document:8a8aef17-b11c-4d31-bce1-5b9ea4226348:3",
		SourceType: "document", SourceID: "8a8aef17-b11c-4d31-bce1-5b9ea4226348",
		SourceName: "research-plan.pdf", Content: "exact excerpt from the indexed source",
	}
	first, err := persistRunEvidence(context.Background(), db, runID, []rag.Hit{hit})
	require.NoError(t, err)
	require.Len(t, first, 1)
	second, err := persistRunEvidence(context.Background(), db, runID, []rag.Hit{hit, hit})
	require.NoError(t, err)
	require.Len(t, second, 1, "duplicate hits in a Run collapse to the same evidence row")
	assert.Equal(t, first[0].ID, second[0].ID, "repeated retrieval reuses the persistent evidence identity")
	assert.Equal(t, hit.Content, *first[0].Quote)
	assert.Equal(t, hit.SourceName, first[0].SourceName)
	assert.Equal(t, "chunk:3", *first[0].Locator)
	digest := sha256.Sum256([]byte(hit.Content))
	assert.Equal(t, hex.EncodeToString(digest[:]), *first[0].SourceVersion)

	changed := hit
	changed.Content = "updated excerpt at the same chunk position"
	third, err := persistRunEvidence(context.Background(), db, runID, []rag.Hit{changed})
	require.NoError(t, err)
	require.Len(t, third, 1)
	assert.NotEqual(t, first[0].ID, third[0].ID, "changed excerpt gets its own versioned identity")
	assert.NotEqual(t, *first[0].SourceVersion, *third[0].SourceVersion)

	var count int64
	require.NoError(t, db.Model(&store.AgentEvidence{}).Where("run_id = ?", runID).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

// wiki_write 的来源回溯：把本 Run 的证据写成 wiki 来源行，重复写不产生重复。
func TestPersistWikiSourcesIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wiki-sources.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	uid, pid := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "wiki-owner", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.Project{Name: "wiki-project", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	conversationID, runID := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentConversation{ID: conversationID, ProjectID: pid,
		UserID: uid, Title: "wiki"}).Error)
	require.NoError(t, db.Create(&store.AgentRun{ID: runID, ConversationID: conversationID,
		AgentKey: "search", Status: "completed", ModelProvider: "test", ModelName: "test",
		PromptVersion: "v1"}).Error)

	hit := rag.Hit{
		ChunkID:    "document:8a8aef17-b11c-4d31-bce1-5b9ea4226348:3",
		SourceType: "document", SourceID: "8a8aef17-b11c-4d31-bce1-5b9ea4226348",
		SourceName: "research-plan.pdf", Content: "exact excerpt",
	}
	_, err = persistRunEvidence(context.Background(), db, runID, []rag.Hit{hit})
	require.NoError(t, err)
	ragStore, err := rag.NewStore(db, rag.HashEmbedder{Dim: 16}, "")
	require.NoError(t, err)
	page, err := ragStore.UpsertWiki(context.Background(), pid, "综述页", "内容", "")
	require.NoError(t, err)

	require.NoError(t, persistWikiSources(context.Background(), db, runID, page.ID))
	require.NoError(t, persistWikiSources(context.Background(), db, runID, page.ID))

	var sources []store.AgentWikiSource
	require.NoError(t, db.Where("wiki_page_id = ?", page.ID).Find(&sources).Error)
	require.Len(t, sources, 1, "同一页同一证据只记一次")
	assert.Equal(t, "document", sources[0].SourceType)
	assert.Equal(t, hit.ChunkID, *sources[0].ChunkID)
}
