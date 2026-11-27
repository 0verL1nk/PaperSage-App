package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 跨章节检查的执行记录。报告本身仍落在 research_artifacts(kind=chapter_review)，
// 这里只保存任务/单元状态、锁定的范围与经服务端校验后的单元结果。

// ReviewRun 一次检查任务。
type ReviewRun struct {
	ID                UUID           `gorm:"column:id;primaryKey"`
	ProjectID         UUID           `gorm:"column:project_id;not null;index:ix_review_runs_project_created,priority:1"`
	UserID            UUID           `gorm:"column:user_id;not null"`
	ConversationID    UUID           `gorm:"column:conversation_id;not null"`
	ClientRequestID   string         `gorm:"column:client_request_id;size:64;not null;uniqueIndex:uq_review_runs_client_request,priority:2"`
	Status            string         `gorm:"column:status;size:20;not null;index:ix_review_runs_status"`
	ScopeJSON         datatypes.JSON `gorm:"column:scope_json;not null"`
	Incremental       bool           `gorm:"column:incremental;not null;default:false"`
	ModelProvider     string         `gorm:"column:model_provider;size:64;not null;default:''"`
	ModelName         string         `gorm:"column:model_name;size:128;not null;default:''"`
	PromptVersion     string         `gorm:"column:prompt_version;size:32;not null;default:''"`
	ArtifactID        *UUID          `gorm:"column:artifact_id"`
	ArtifactRevision  *int           `gorm:"column:artifact_revision"`
	ErrorText         string         `gorm:"column:error_text;size:512;not null;default:''"`
	CancelRequestedAt *time.Time     `gorm:"column:cancel_requested_at"`
	StartedAt         *time.Time     `gorm:"column:started_at"`
	CompletedAt       *time.Time     `gorm:"column:completed_at"`
	CreatedAt         time.Time      `gorm:"column:created_at;autoCreateTime;index:ix_review_runs_project_created,priority:2"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (ReviewRun) TableName() string { return "review_runs" }

func (r *ReviewRun) BeforeCreate(*gorm.DB) error {
	if r.ID.UUID == uuid.Nil {
		r.ID = UUID{UUID: uuid.Must(uuid.NewV7())}
	}
	return nil
}

// ReviewUnit 一个检查单元（章节×维度，或一个跨章维度）。
type ReviewUnit struct {
	ID                   UUID           `gorm:"column:id;primaryKey"`
	ReviewRunID          UUID           `gorm:"column:review_run_id;not null;uniqueIndex:uq_review_units_seq,priority:1"`
	Seq                  int            `gorm:"column:seq;not null;uniqueIndex:uq_review_units_seq,priority:2"`
	Kind                 string         `gorm:"column:kind;size:16;not null"`
	Label                string         `gorm:"column:label;size:200;not null"`
	FilePathsJSON        datatypes.JSON `gorm:"column:file_paths_json;not null"`
	SectionIDsJSON       datatypes.JSON `gorm:"column:section_ids_json;not null"`
	DimensionsJSON       datatypes.JSON `gorm:"column:dimensions_json;not null"`
	ContextJSON          datatypes.JSON `gorm:"column:context_json;not null"`
	InputFingerprint     string         `gorm:"column:input_fingerprint;size:64;not null;index:ix_review_units_fingerprint,priority:1"`
	Status               string         `gorm:"column:status;size:16;not null;index:ix_review_units_fingerprint,priority:2"`
	AgentRunID           *UUID          `gorm:"column:agent_run_id"`
	ReusedFromUnitID     *UUID          `gorm:"column:reused_from_unit_id"`
	FindingsJSON         datatypes.JSON `gorm:"column:findings_json;not null"`
	DimensionResultsJSON datatypes.JSON `gorm:"column:dimension_results_json;not null"`
	SourcesReadJSON      datatypes.JSON `gorm:"column:sources_read_json;not null"`
	ErrorText            string         `gorm:"column:error_text;size:512;not null;default:''"`
	StartedAt            *time.Time     `gorm:"column:started_at"`
	CompletedAt          *time.Time     `gorm:"column:completed_at"`
	CreatedAt            time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (ReviewUnit) TableName() string { return "review_units" }

func (u *ReviewUnit) BeforeCreate(*gorm.DB) error {
	if u.ID.UUID == uuid.Nil {
		u.ID = UUID{UUID: uuid.Must(uuid.NewV7())}
	}
	return nil
}

// 检查任务与单元状态（与报告维度状态区分）。
const (
	ReviewRunQueued    = "queued"
	ReviewRunRunning   = "running"
	ReviewRunSucceeded = "succeeded"
	ReviewRunPartial   = "partial"
	ReviewRunFailed    = "failed"
	ReviewRunCancelled = "cancelled"

	ReviewUnitPending   = "pending"
	ReviewUnitRunning   = "running"
	ReviewUnitSucceeded = "succeeded"
	ReviewUnitFailed    = "failed"
	ReviewUnitReused    = "reused"
	ReviewUnitCancelled = "cancelled"
)
