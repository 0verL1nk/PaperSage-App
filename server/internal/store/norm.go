package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 学术规范资源库（HELE-25）：规范包 → 条目聚合根 → 不可变版本；
// 项目采用锁定具体版本，管理员改条目只追加新版本，已有项目的约束不静默变化。

const (
	NormAuthoritySchool   = "school"
	NormAuthorityJournal  = "journal"
	NormAuthorityStandard = "standard"
	NormAuthorityGeneral  = "general"
	NormAuthorityUser     = "user"

	NormScenarioAll                 = "all"
	NormScenarioUndergraduateThesis = "undergraduate_thesis"
	NormScenarioIntroPaper          = "intro_paper"

	NormRuleKindOfficial = "official"
	NormRuleKindGeneral  = "general"
	NormRuleKindUser     = "user"
	NormRuleKindModel    = "model"

	// 强制程度：注入上下文超预算时按 must > should > recommended 截断。
	NormBindingMust        = "must"
	NormBindingShould      = "should"
	NormBindingRecommended = "recommended"

	NormStateActive   = "active"
	NormStateArchived = "archived"

	NormOriginBuiltin = "builtin"
	NormOriginUser    = "user"
	NormOriginImport  = "import"
	NormOriginAgent   = "agent"

	NormAdoptionEnabled  = "enabled"
	NormAdoptionDisabled = "disabled"
)

// NormPack 是规范包：来源与版本的单位。内置包全局只读，用户包按 owner 隔离。
type NormPack struct {
	SoftDeleteModel
	// 列名用 pack_key：key 是 MySQL 保留字，裸用会在手写 SQL 里炸。
	Key        string  `gorm:"column:pack_key;size:80;not null;uniqueIndex:uq_norm_pack_key,priority:1"`
	Name       string  `gorm:"size:120;not null"`
	Authority  string  `gorm:"size:20;not null;default:general"`
	Edition    string  `gorm:"size:40;not null;default:''"`
	SourceURL  *string `gorm:"size:500"`
	SourceNote *string
	Scenario   string `gorm:"size:32;not null;default:all"`
	IsBuiltin  bool   `gorm:"not null;default:false"`
	OwnerID    *UUID  `gorm:"index"`
	State      string `gorm:"size:20;not null;default:active"`
	DeletedSeq int64  `gorm:"not null;default:0;uniqueIndex:uq_norm_pack_key,priority:2"`
}

// NormEntry 是条目聚合根：CurrentVersion 指向 NormVersion，内容变更只追加版本。
type NormEntry struct {
	SoftDeleteModel
	PackID         UUID   `gorm:"not null;uniqueIndex:uq_norm_entry_code,priority:1;index:ix_norm_entries_pack"`
	Code           string `gorm:"size:60;not null;uniqueIndex:uq_norm_entry_code,priority:2"`
	Category       string `gorm:"size:32;not null;index:ix_norm_entries_category"`
	RuleKind       string `gorm:"size:16;not null;default:general"`
	Binding        string `gorm:"size:16;not null;default:should"`
	Scenario       string `gorm:"size:32;not null;default:all"`
	CurrentVersion int    `gorm:"not null;default:1"`
	SortOrder      int    `gorm:"not null;default:0"`
	State          string `gorm:"size:20;not null;default:active"`
	DeletedSeq     int64  `gorm:"not null;default:0;uniqueIndex:uq_norm_entry_code,priority:3"`
}

// NormVersion 不可变。AI 证据、检查报告引用的 (entry_id, version) 永久可回查。
type NormVersion struct {
	ID                 UUID           `gorm:"primaryKey"`
	EntryID            UUID           `gorm:"not null;uniqueIndex:uq_norm_version,priority:1;index:ix_norm_versions_entry"`
	Version            int            `gorm:"not null;uniqueIndex:uq_norm_version,priority:2"`
	Title              string         `gorm:"size:200;not null"`
	RuleText           string         `gorm:"not null"`
	Rationale          string         `gorm:"not null;default:''"`
	Examples           datatypes.JSON `gorm:"not null"`
	ApplicableSections datatypes.JSON `gorm:"not null"`
	SourceRef          string         `gorm:"size:300;not null"`
	SourceQuote        string         `gorm:"not null;default:''"`
	SourceURL          string         `gorm:"size:500;not null;default:''"`
	ContentHash        string         `gorm:"size:64;not null"`
	Origin             string         `gorm:"size:16;not null;default:user"`
	// 内置条目的版本没有创建者，故可空且不加外键——历史不应随用户删除消失。
	CreatedBy *UUID
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (v *NormVersion) BeforeCreate(*gorm.DB) error {
	if v.ID.UUID == uuid.Nil {
		v.ID = MustNewUUID()
	}
	return nil
}

// NormEntryTag 是分类之外的横切标签（摘要 / 结论 / 本校特殊要求…）。
// 硬删：标签随条目内容重建，没有独立生命周期。
type NormEntryTag struct {
	EntryID UUID   `gorm:"primaryKey"`
	Tag     string `gorm:"size:32;primaryKey;index:ix_norm_entry_tags_tag"`
}

// ProjectNormAdoption 是项目对条目的采用，Version 锁定采用时的版本。
type ProjectNormAdoption struct {
	SoftDeleteModel
	ProjectID         UUID    `gorm:"not null;uniqueIndex:uq_project_norm_adoption,priority:1;index:ix_project_norm_adoptions_project"`
	EntryID           UUID    `gorm:"not null;uniqueIndex:uq_project_norm_adoption,priority:2;index:ix_project_norm_adoptions_entry"`
	Version           int     `gorm:"not null"`
	State             string  `gorm:"size:16;not null;default:enabled"`
	OverrideText      *string `gorm:"type:text"`
	OverrideUpdatedAt *time.Time
	AdoptedBy         UUID  `gorm:"not null"`
	DeletedSeq        int64 `gorm:"not null;default:0;uniqueIndex:uq_project_norm_adoption,priority:3"`
}
