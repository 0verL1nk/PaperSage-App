package rag

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 索引代次状态。
const (
	GenBuilding   = "building"   // 构建中：新片段写入该代次，尚不参与检索
	GenActive     = "active"     // 活动：检索只命中该代次
	GenSuperseded = "superseded" // 已被取代：可回查，标注对应旧版材料
	GenFailed     = "failed"     // 失败：保留可重试，不影响当前活动代次
)

// RagIndexGeneration 一个项目的检索索引代次。
//
// 来源：docs/project/12-ai-writing-design.md §6.3——索引重建走
// 「建立新代次 → 校验 → 切换活动代次」，而不是先删旧片段再嵌入：
// 新代次失败可重试，旧代次可回查。更换嵌入模型必须重建，因此代次上
// 记录模型标识与维度，避免新旧向量混用后仍宣称检索质量。
type RagIndexGeneration struct {
	ID             store.UUID `gorm:"column:id;primaryKey"`
	ProjectID      store.UUID `gorm:"column:project_id;index"`
	Generation     int64      `gorm:"column:generation"`
	Status         string     `gorm:"column:status;size:20;default:building"`
	EmbeddingModel string     `gorm:"column:embedding_model;size:128"`
	EmbeddingDim   int        `gorm:"column:embedding_dim"`
	ChunkCount     int        `gorm:"column:chunk_count"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	ActivatedAt    *time.Time `gorm:"column:activated_at"`
}

func (RagIndexGeneration) TableName() string { return "rag_index_generations" }

// ActiveGeneration 返回项目当前的活动代次；没有则建立第 1 代并置为活动。
//
// 迁移已为存量项目补齐第 1 代活动代次，这里的兜底覆盖全新项目与并发情形。
func (s *Store) ActiveGeneration(ctx context.Context, projectID store.UUID) (int64, error) {
	// 纯 Neon 模式（NewStore(nil, …) + AttachNeon）：没有主库，代次登记表不存在，
	// 返回 0 表示「不按代次过滤」——见 pgvector.go 里对 generation <= 0 的处理。
	if s.DB == nil {
		return 0, nil
	}
	var g RagIndexGeneration
	err := s.DB.WithContext(ctx).
		Where("project_id = ? AND status = ?", projectID, GenActive).
		Order("generation DESC").First(&g).Error
	if err == nil {
		return g.Generation, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	model, dim := s.Embed.Describe()
	now := time.Now().UTC()
	g = RagIndexGeneration{
		ID: store.MustNewUUID(), ProjectID: projectID, Generation: 1,
		Status: GenActive, EmbeddingModel: model, EmbeddingDim: dim, ActivatedAt: &now,
	}
	if cerr := s.DB.WithContext(ctx).Create(&g).Error; cerr != nil {
		// 并发下另一请求已建好：读回即可，不需要报错。
		var again RagIndexGeneration
		if e := s.DB.WithContext(ctx).
			Where("project_id = ? AND status = ?", projectID, GenActive).
			First(&again).Error; e == nil {
			return again.Generation, nil
		}
		return 0, cerr
	}
	return 1, nil
}

// WriteGeneration 返回写侧应使用的代次：优先「构建中」，否则「活动」。
//
// 重建期间产生的片段必须落进新代次，否则切换后这批内容会被丢掉
// （新代次是空的，"新内容进了旧代次"就等于没重建成功）。
func (s *Store) WriteGeneration(ctx context.Context, projectID store.UUID) (int64, error) {
	if s.DB == nil { // 纯 Neon 模式：同 ActiveGeneration
		return 0, nil
	}
	var g RagIndexGeneration
	err := s.DB.WithContext(ctx).
		Where("project_id = ? AND status IN ?", projectID, []string{GenBuilding, GenActive}).
		Order("generation DESC").First(&g).Error
	if err == nil {
		return g.Generation, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	return s.ActiveGeneration(ctx, projectID)
}

// BeginGeneration 建立新代次（building），号取当前最大值 +1。
// 写侧把片段写进该代次，构建完成后由 ActivateGeneration 切换。
func (s *Store) BeginGeneration(ctx context.Context, projectID store.UUID) (*RagIndexGeneration, error) {
	var maxGen int64
	if err := s.DB.WithContext(ctx).Model(&RagIndexGeneration{}).
		Where("project_id = ?", projectID).
		Select("COALESCE(MAX(generation), 0)").
		Row().Scan(&maxGen); err != nil {
		return nil, err
	}
	model, dim := s.Embed.Describe()
	g := RagIndexGeneration{
		ID: store.MustNewUUID(), ProjectID: projectID, Generation: maxGen + 1,
		Status: GenBuilding, EmbeddingModel: model, EmbeddingDim: dim,
	}
	if err := s.DB.WithContext(ctx).Create(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// ActivateGeneration 把 building 代次切为 active，并将原活动代次置为 superseded。
//
// 条件更新保证幂等与并发安全：仅当该代次仍处于 building 时才切换；
// 重复切换（或切换一个已失败/已取代的代次）会明确报错而不是静默生效。
func (s *Store) ActivateGeneration(ctx context.Context, projectID store.UUID, gen int64) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&IndexedChunk{}).
			Where("project_id = ? AND generation = ?", projectID, gen).
			Count(&n).Error; err != nil {
			return err
		}
		res := tx.Model(&RagIndexGeneration{}).
			Where("project_id = ? AND generation = ? AND status = ?", projectID, gen, GenBuilding).
			Updates(map[string]any{
				"status": GenActive, "activated_at": time.Now().UTC(), "chunk_count": n,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("generation %d is not in %s state", gen, GenBuilding)
		}
		return tx.Model(&RagIndexGeneration{}).
			Where("project_id = ? AND generation <> ? AND status = ?", projectID, gen, GenActive).
			Update("status", GenSuperseded).Error
	})
	if err != nil {
		return err
	}
	s.invalidate(projectID) // 活动代次变了，内存索引必须重建
	return nil
}

// FailGeneration 标记代次构建失败。失败代次保留可重试，且不影响当前活动代次。
func (s *Store) FailGeneration(ctx context.Context, projectID store.UUID, gen int64) error {
	return s.DB.WithContext(ctx).Model(&RagIndexGeneration{}).
		Where("project_id = ? AND generation = ? AND status = ?", projectID, gen, GenBuilding).
		Update("status", GenFailed).Error
}
