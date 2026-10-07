package rag

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 代次契约（docs/project/12-ai-writing-design.md §6.3）：
// 检索只命中活动代次；构建中的代次不参与检索；切换是条件更新；
// 旧代次保留可回查；失败代次可重试且不影响当前活动代次。
func TestIndexGenerationLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := store.MustNewUUID()

	// 第 1 代：首次写入时自动建立并置为活动
	_, err := s.IndexSource(ctx, pid, "document", store.MustNewUUID(),
		"旧材料", "旧材料：区块链技术保证数据不可篡改。")
	require.NoError(t, err)
	gen1, err := s.ActiveGeneration(ctx, pid)
	require.NoError(t, err)
	require.EqualValues(t, 1, gen1)

	// 开新代次后写入的片段必须落进新代次（写侧优先"构建中"）
	g2, err := s.BeginGeneration(ctx, pid)
	require.NoError(t, err)
	require.EqualValues(t, 2, g2.Generation)
	_, err = s.IndexSource(ctx, pid, "document", store.MustNewUUID(),
		"新材料", "新材料：深度学习在图像识别中的应用广泛。")
	require.NoError(t, err)

	var inG2 int64
	require.NoError(t, s.DB.Model(&IndexedChunk{}).
		Where("project_id = ? AND generation = ?", pid, g2.Generation).Count(&inG2).Error)
	assert.NotZero(t, inG2, "重建期间的新内容应写入构建中的代次")

	// 未切换前：新代次不参与检索
	hits, err := s.Search(ctx, pid, "深度学习 图像识别", 5)
	require.NoError(t, err)
	for _, h := range hits {
		assert.NotEqual(t, "新材料", h.SourceName, "构建中的代次不得参与检索")
	}

	// 切换后：只命中新代次，旧代次不再参与
	require.NoError(t, s.ActivateGeneration(ctx, pid, g2.Generation))
	hits, err = s.Search(ctx, pid, "深度学习 图像识别", 5)
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	for _, h := range hits {
		assert.Equal(t, "新材料", h.SourceName, "切换后只应命中活动代次")
	}

	// 旧代次保留可回查（切换不是删除）
	var inG1 int64
	require.NoError(t, s.DB.Model(&IndexedChunk{}).
		Where("project_id = ? AND generation = ?", pid, gen1).Count(&inG1).Error)
	assert.NotZero(t, inG1, "旧代次片段应保留以便回查")

	// 切换是条件更新：已非 building 的代次不可再次切换
	require.Error(t, s.ActivateGeneration(ctx, pid, g2.Generation), "重复切换应报错而非静默生效")

	// 失败代次：保留可重试，且不改变当前活动代次
	g3, err := s.BeginGeneration(ctx, pid)
	require.NoError(t, err)
	require.NoError(t, s.FailGeneration(ctx, pid, g3.Generation))
	active, err := s.ActiveGeneration(ctx, pid)
	require.NoError(t, err)
	assert.EqualValues(t, g2.Generation, active, "失败代次不应改变活动代次")

	var failed RagIndexGeneration
	require.NoError(t, s.DB.Where("project_id = ? AND generation = ?", pid, g3.Generation).
		First(&failed).Error)
	assert.Equal(t, GenFailed, failed.Status, "失败代次应保留并标记为 failed，便于重试")
}
