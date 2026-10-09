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
