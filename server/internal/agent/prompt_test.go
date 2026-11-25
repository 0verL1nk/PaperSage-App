package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type fakeNorms struct {
	entries []norms.Entry
	err     error
}

func (f fakeNorms) AdoptedForProject(context.Context, store.UUID) ([]norms.Entry, error) {
	return f.entries, f.err
}

func promptFixture(t *testing.T, provider norms.Provider) (*Service, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/prompt.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid, pid := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "prompt-user", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}).Error)
	return &Service{DB: db, Norms: provider}, uid, pid
}

func TestSystemPromptInjectsAdoptedNorms(t *testing.T) {
	provider := fakeNorms{entries: []norms.Entry{{
		ID: "entry-1", Code: "GBT7714-TYPE-MARKS", Version: 2,
		Title: "按文献类型标注类型标识", Rule: "期刊文章用 [J] 标注。", Source: "GB/T 7714—2015",
	}}}
	svc, uid, pid := promptFixture(t, provider)
	prompt := svc.systemPromptFor(context.Background(), pid, uid)

	assert.Contains(t, prompt, "[N-GBT7714-TYPE-MARKS@v2]")
	assert.Contains(t, prompt, "期刊文章用 [J] 标注。")
	assert.Contains(t, prompt, "GB/T 7714—2015")
	assert.Contains(t, prompt, "不得编造未列出的条款")
}

func TestSystemPromptStatesNoAdoptedNorms(t *testing.T) {
	svc, uid, pid := promptFixture(t, fakeNorms{})
	prompt := svc.systemPromptFor(context.Background(), pid, uid)
	assert.Contains(t, prompt, "尚未采用任何规范条目")
	assert.NotContains(t, prompt, "必须逐条对照")
}

func TestSystemPromptStatesNormsUnavailable(t *testing.T) {
	for name, provider := range map[string]norms.Provider{
		"unavailable": norms.Unavailable{},
		"error":       fakeNorms{err: errors.New("db down")},
		"nil":         nil,
	} {
		t.Run(name, func(t *testing.T) {
			svc, uid, pid := promptFixture(t, provider)
			prompt := svc.systemPromptFor(context.Background(), pid, uid)
			assert.Contains(t, prompt, "不要声称遵循或符合任何具体规范条款")
			assert.NotContains(t, prompt, "规则：")
		})
	}
}

func TestEntryCite(t *testing.T) {
	assert.Equal(t, "N-GBT7714@v3", norms.Entry{Code: "GBT7714", Version: 3}.Cite())
	assert.Equal(t, "N-GBT7714", norms.Entry{Code: "GBT7714"}.Cite())
	assert.Equal(t, "N-entry-1", norms.Entry{ID: "entry-1"}.Cite())
}
