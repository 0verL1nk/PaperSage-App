package store

import (
	"time"

	"gorm.io/datatypes"
)

// ProjectSource is the aggregate root; publishing must create the matching
// SourceVersion and advance CurrentRevision in the same transaction.
type ProjectSource struct {
	ID              UUID      `gorm:"primaryKey;uniqueIndex:uq_project_sources_id_project,priority:1"`
	ProjectID       UUID      `gorm:"not null;index:ix_project_sources_project;uniqueIndex:uq_project_sources_id_project,priority:2"`
	Kind            string    `gorm:"size:32;not null"`
	CurrentRevision int64     `gorm:"not null;default:0;check:current_revision >= 0"`
	State           string    `gorm:"size:20;not null;default:active"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"not null;autoUpdateTime"`

	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
}

// SourceVersion refers to immutable object content; restores must be checked
// by the service to ensure RestoredFromID belongs to the same source.
type SourceVersion struct {
	ID             UUID   `gorm:"primaryKey"`
	SourceID       UUID   `gorm:"not null;uniqueIndex:uq_source_versions_revision,priority:1"`
	Revision       int64  `gorm:"not null;uniqueIndex:uq_source_versions_revision,priority:2;check:revision > 0"`
	S3Key          string `gorm:"size:500;not null"`
	ContentSHA256  string `gorm:"size:64;not null"`
	SizeBytes      int64  `gorm:"not null;check:size_bytes >= 0"`
	RestoredFromID *UUID
	CreatedBy      *UUID
	CreatedAt      time.Time `gorm:"not null;autoCreateTime"`

	Source       ProjectSource  `gorm:"foreignKey:SourceID;constraint:OnDelete:RESTRICT"`
	RestoredFrom *SourceVersion `gorm:"foreignKey:RestoredFromID;constraint:OnDelete:RESTRICT"`
	Creator      *User          `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL"`
}

// SourceExtraction tracks deterministic processing; completed content is immutable.
type SourceExtraction struct {
	ID               UUID    `gorm:"primaryKey"`
	SourceVersionID  UUID    `gorm:"not null;uniqueIndex:uq_source_extractions_version,priority:1"`
	ExtractorVersion string  `gorm:"size:64;not null;uniqueIndex:uq_source_extractions_version,priority:2"`
	State            string  `gorm:"size:20;not null;default:pending"`
	TextS3Key        *string `gorm:"size:500"`
	TextSHA256       *string `gorm:"size:64"`
	TextBytes        int64   `gorm:"not null;default:0"`
	LeaseToken       *string `gorm:"size:64"`
	LeaseUntil       *time.Time
	AttemptCount     int64 `gorm:"not null;default:0"`
	ErrorJSON        datatypes.JSON
	CreatedAt        time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt        time.Time `gorm:"not null;autoUpdateTime"`

	SourceVersion SourceVersion `gorm:"foreignKey:SourceVersionID;constraint:OnDelete:RESTRICT"`
}

// SourceSegment byte offsets refer to UTF-8 extraction content, not chunk indices.
type SourceSegment struct {
	ID           UUID    `gorm:"primaryKey"`
	ExtractionID UUID    `gorm:"not null;uniqueIndex:uq_source_segments_ordinal,priority:1"`
	Ordinal      int     `gorm:"not null;uniqueIndex:uq_source_segments_ordinal,priority:2;check:ordinal >= 0"`
	StartByte    int64   `gorm:"not null;check:start_byte >= 0"`
	EndByte      int64   `gorm:"not null;check:end_byte >= start_byte"`
	LocatorKind  string  `gorm:"size:32;not null"`
	PhysicalPage *int    `gorm:"check:physical_page IS NULL OR physical_page > 0"`
	ParagraphID  *string `gorm:"size:128"`

	Extraction SourceExtraction `gorm:"foreignKey:ExtractionID;constraint:OnDelete:RESTRICT"`
}
