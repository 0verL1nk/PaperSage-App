package store

import "time"

// 待复查标记的目标类别：产物修订行、证据、wiki 页。
const (
	RecheckTargetArtifactRevision = "artifact_revision"
	RecheckTargetEvidence         = "evidence"
	RecheckTargetWikiPage         = "wiki_page"
)

// 触发原因。撤权/排除当前无数据模型，只预留常量，不写空触发点。
const (
	RecheckTriggerSourceDeleted    = "source_deleted"
	RecheckTriggerSourceNewVersion = "source_new_version"
)

// 用户处理方式。「已重建」只是用户声明，服务端不自动重建。
const (
	RecheckResolutionRechecked = "rechecked"
	RecheckResolutionRebuilt   = "rebuilt"
	RecheckResolutionDismissed = "dismissed"
)

// ContentRecheckMark 是资料变更后挂在派生内容上的待复查标记。
// resolved_seq 沿用 deleted_seq 模式（未处理 0 / 处理时 unix 微秒），
// 同一目标同一触发在处理后可再次触发而不撞唯一键。
type ContentRecheckMark struct {
	ID          UUID      `gorm:"primaryKey"`
	ProjectID   UUID      `gorm:"not null;index:ix_recheck_project,priority:1"`
	TargetType  string    `gorm:"size:24;not null;uniqueIndex:uq_recheck_mark,priority:1"`
	TargetID    UUID      `gorm:"not null;uniqueIndex:uq_recheck_mark,priority:2"`
	ArtifactID  *UUID     `gorm:"index:ix_recheck_artifact,priority:1"`
	Revision    *int      `gorm:"index:ix_recheck_artifact,priority:2"`
	TriggerKind string    `gorm:"size:24;not null;uniqueIndex:uq_recheck_mark,priority:3"`
	TriggerID   UUID      `gorm:"not null;uniqueIndex:uq_recheck_mark,priority:4"`
	Reason      string    `gorm:"size:500;not null"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
	ResolvedAt  *time.Time
	ResolvedBy  *UUID   `gorm:"index"`
	Resolution  *string `gorm:"size:16"`
	ResolvedSeq int64   `gorm:"not null;default:0;uniqueIndex:uq_recheck_mark,priority:5;index:ix_recheck_project,priority:2;index:ix_recheck_artifact,priority:3"`
}

func (ContentRecheckMark) TableName() string { return "content_recheck_marks" }
