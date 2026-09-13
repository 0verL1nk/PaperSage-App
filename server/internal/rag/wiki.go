package rag

import (
	"context"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// WikiPage — LLM wiki：子 agent 检索过程的沉淀页（也被 hybrid 索引，
// 下次检索可直达知识而非重读原文）。
type WikiPage struct {
	ID        store.UUID     `gorm:"column:id;primaryKey"`
	ProjectID store.UUID     `gorm:"column:project_id;index"`
	Title     string         `gorm:"column:title;size:200"`
	Content   string         `gorm:"column:content"`
	Meta      datatypes.JSON `gorm:"column:meta"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (WikiPage) TableName() string { return "agent_wiki_pages" }

// UpsertWiki 写/更新一页 wiki 并同步重建其索引。
func (s *Store) UpsertWiki(ctx context.Context, projectID store.UUID,
	title, content, metaJSON string) (*WikiPage, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, gorm.ErrInvalidData // 空标题无意义
	}
	var page WikiPage
	err := s.DB.WithContext(ctx).
		Where("project_id = ? AND title = ?", projectID, title).
		First(&page).Error
	if err != nil {
		page = WikiPage{
			ID: store.MustNewUUID(), ProjectID: projectID, Title: title,
		}
	}
	page.Content = content
	page.Meta = datatypes.JSON(metaJSON)
	if err := s.DB.WithContext(ctx).Save(&page).Error; err != nil {
		return nil, err
	}
	if _, err := s.IndexSource(ctx, projectID, "wiki", page.ID, title, content); err != nil {
		return &page, err
	}
	return &page, nil
}

// ListWiki 列出项目的 wiki 页（标题+摘要）。
func (s *Store) ListWiki(ctx context.Context, projectID store.UUID) ([]WikiPage, error) {
	var pages []WikiPage
	err := s.DB.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("updated_at DESC").Limit(100).Find(&pages).Error
	return pages, err
}

// GetWiki 按标题取整页。
func (s *Store) GetWiki(ctx context.Context, projectID store.UUID, title string) (*WikiPage, error) {
	var page WikiPage
	err := s.DB.WithContext(ctx).
		Where("project_id = ? AND title = ?", projectID, strings.TrimSpace(title)).
		First(&page).Error
	if err != nil {
		return nil, err
	}
	return &page, nil
}
