package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 组织产物固定种类；不接受任意 kind。
const (
	ResearchArtifactLiteratureNote       = "literature_note"
	ResearchArtifactLiteratureComparison = "literature_comparison"
	ResearchArtifactChapterPlan          = "chapter_plan"
	ResearchArtifactChapterDigest        = "chapter_digest"
	ResearchArtifactChapterReview        = "chapter_review"

	ResearchArtifactRevisionCandidate  = "candidate"
	ResearchArtifactRevisionAdopted    = "adopted"
	ResearchArtifactRevisionRejected   = "rejected"
	ResearchArtifactRevisionSuperseded = "superseded"

	ResearchDependencySourceVersion = "source_version"
	ResearchDependencySourceSegment = "source_segment"
	ResearchDependencyArtifact      = "artifact_revision"
	ResearchDependencyRun           = "run"
)

// ResearchArtifact 是组织产物聚合根：当前修订与用户已采用修订分开记录。
type ResearchArtifact struct {
	SoftDeleteModel
	ProjectID       UUID   `gorm:"not null;index:ix_research_artifacts_project_kind,priority:1;uniqueIndex:uq_research_artifact_request,priority:1"`
	Kind            string `gorm:"size:32;not null;index:ix_research_artifacts_project_kind,priority:2"`
	State           string `gorm:"size:20;not null;default:active"`
	CurrentRevision int    `gorm:"not null;default:0"`
	AdoptedRevision *int
	ClientRequestID *string `gorm:"size:64;uniqueIndex:uq_research_artifact_request,priority:2"`
	DeletedSeq      int64   `gorm:"not null;default:0;uniqueIndex:uq_research_artifact_request,priority:3"`
	CreatedBy       UUID    `gorm:"not null"`

	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Creator User    `gorm:"foreignKey:CreatedBy;constraint:OnDelete:CASCADE"`
}

// ResearchArtifactRevision 不可变；payload 已由服务端按 kind 校验。
type ResearchArtifactRevision struct {
	ID          UUID           `gorm:"primaryKey"`
	ArtifactID  UUID           `gorm:"not null;uniqueIndex:uq_artifact_revision,priority:1;index:ix_artifact_revision_status,priority:1"`
	Revision    int            `gorm:"not null;uniqueIndex:uq_artifact_revision,priority:2"`
	PayloadJSON datatypes.JSON `gorm:"not null"`
	Origin      string         `gorm:"size:16;not null;default:user"`
	Status      string         `gorm:"size:20;not null;default:candidate;index:ix_artifact_revision_status,priority:2"`
	RunID       *UUID
	CreatedBy   UUID      `gorm:"not null"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
}

func (r *ResearchArtifactRevision) BeforeCreate(*gorm.DB) error {
	if r.ID.UUID == uuid.Nil {
		r.ID = MustNewUUID()
	}
	return nil
}

type ResearchArtifactDependency struct {
	ID         UUID      `gorm:"primaryKey"`
	ProjectID  UUID      `gorm:"not null;index:ix_artifact_dependency_ref,priority:1"`
	ArtifactID UUID      `gorm:"not null;uniqueIndex:uq_artifact_dependency,priority:1"`
	Revision   int       `gorm:"not null;uniqueIndex:uq_artifact_dependency,priority:2"`
	Kind       string    `gorm:"size:24;not null;uniqueIndex:uq_artifact_dependency,priority:3;index:ix_artifact_dependency_ref,priority:2"`
	RefID      UUID      `gorm:"not null;uniqueIndex:uq_artifact_dependency,priority:4;index:ix_artifact_dependency_ref,priority:3"`
	CreatedAt  time.Time `gorm:"not null;autoCreateTime"`
}

func (d *ResearchArtifactDependency) BeforeCreate(*gorm.DB) error {
	if d.ID.UUID == uuid.Nil {
		d.ID = MustNewUUID()
	}
	return nil
}
