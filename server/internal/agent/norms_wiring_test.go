package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 回归：Agent 工具写检查报告时必须带同一权威规范 Provider。
// 生产事故是 tools.go 直接 research.NewService(db)，Provider 为 nil，
// create_research_artifact 在「学术规范库未接入」上自我拒绝，报告永远存不下来。
func TestNewResearchServiceCarriesNormsProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wiring.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	provider := fakeNorms{entries: []norms.Entry{{ID: "e1", Code: "GBT7714", Version: 1}}}
	svc := newResearchService(db, provider)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Norms, "产物服务必须带上规范 Provider，否则报告会被失败关闭逻辑拒绝")
}

// 回归：幂等键必须塞得进 varchar(64)。旧实现 "runID:md5" 是 69 字符，
// 生产 PG 直接 22001 报错，报告写不进去。
func TestResearchClientRequestIDFitsColumn(t *testing.T) {
	runID := store.MustNewUUID()
	payload := []byte(`{"kind":"chapter_review"}`)
	id := researchClientRequestID(runID, payload)
	assert.LessOrEqual(t, len(id), 64)
	assert.NotEmpty(t, id)
	// 同一 run + 同一 payload 稳定（幂等），换 run 必须不同（不跨 run 去重）。
	assert.Equal(t, id, researchClientRequestID(runID, payload))
	assert.NotEqual(t, id, researchClientRequestID(store.MustNewUUID(), payload))
}
