package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 模型契约：backend/models/*.py（SQLAlchemy 定义为唯一 schema 真相）。
// gorm 标签同时服务两条建表路径：SQLite（AutoMigrate，对应 Python 的
// create_all 兜底）与 PostgreSQL（migrations/ 下 golang-migrate SQL）。

// BaseModel 对应 UUIDMixin + TimestampMixin：应用侧生成 uuid4 主键，
// 时间戳自动填充（Python default=now UTC / onupdate=now UTC）。
type BaseModel struct {
	ID        UUID      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

// BeforeCreate 填充 uuid4 主键（Python 侧 default=uuid.uuid4）。
func (b *BaseModel) BeforeCreate(*gorm.DB) error {
	if b.ID.UUID == uuid.Nil {
		b.ID = UUID{UUID: uuid.New()}
	}
	return nil
}

type User struct {
	BaseModel
	Username          string  `gorm:"size:50;not null;uniqueIndex"`
	PasswordHash      string  `gorm:"size:255;not null"`
	Email             *string `gorm:"size:255;uniqueIndex"`
	EmailVerified     bool    `gorm:"not null;default:false"`
	IsEmailRegistered bool    `gorm:"not null;default:false"` // 邮箱注册制账号（验证门槛只约束这类）
	IsActive          bool    `gorm:"not null;default:true"`
	IsSuperuser       bool    `gorm:"not null;default:false"`

	Projects      []Project      `gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`
	ProjectGroups []ProjectGroup `gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`
}

type UserSettings struct {
	UserID    UUID           `gorm:"primaryKey;constraint:OnDelete:CASCADE"` // → users.id
	Category  string         `gorm:"primaryKey;size:100"`
	Settings  datatypes.JSON `gorm:"not null;default:'{}'"`
	UpdatedAt time.Time      `gorm:"not null;autoUpdateTime"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

type ProjectGroup struct {
	BaseModel
	Name      string `gorm:"size:50;not null"`
	OwnerID   UUID   `gorm:"not null;index"`
	SortOrder int    `gorm:"not null;default:0"`

	Owner    User      `gorm:"foreignKey:OwnerID"`
	Projects []Project `gorm:"foreignKey:GroupID;constraint:OnDelete:SET NULL"`
}

type Project struct {
	BaseModel
	Name        string     `gorm:"size:100;not null"`
	Description *string    // Text
	OwnerID     UUID       `gorm:"not null;index"`
	GroupID     *UUID      `gorm:"index"`
	Status      string     `gorm:"size:20;not null;default:active"`
	IsDeleted   bool       `gorm:"not null;default:false"`
	DeletedAt   *time.Time // 普通可空列（DateTime，非 gorm 软删标记）

	Owner      User          `gorm:"foreignKey:OwnerID"`
	Group      *ProjectGroup `gorm:"foreignKey:GroupID"`
	LatexFiles []LatexFile   `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
}

type Folder struct {
	BaseModel
	Name      string `gorm:"size:100;not null"`
	ParentID  *UUID
	ProjectID UUID `gorm:"not null"`
	OwnerID   UUID `gorm:"not null"`

	Project  Project  `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Owner    User     `gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`
	Children []Folder `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE"`
}

type Document struct {
	BaseModel
	Name      string  `gorm:"size:255;not null"`
	Type      string  `gorm:"size:100;not null"`
	Size      int64   `gorm:"not null;default:0"` // BigInteger
	Status    string  `gorm:"size:20;not null;default:uploaded"`
	S3Key     string  `gorm:"size:500;not null"`
	Remark    *string // Text
	ProjectID UUID    `gorm:"not null"`
	FolderID  *UUID
	OwnerID   UUID `gorm:"not null"`

	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	Folder  *Folder `gorm:"foreignKey:FolderID;constraint:OnDelete:SET NULL"`
	Owner   User    `gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`
}

type LatexFile struct {
	BaseModel
	ProjectID   UUID    `gorm:"not null;uniqueIndex:uq_project_file_path;index"`
	FilePath    string  `gorm:"size:500;not null;uniqueIndex:uq_project_file_path"`
	ContentType *string `gorm:"size:100"`
	SizeBytes   int64   `gorm:"not null;default:0"`
	ContentHash *string `gorm:"size:64;index"`
	S3Key       *string `gorm:"size:500"`
	LastSavedAt *time.Time

	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
}

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSuccess   JobStatus = "success"
	JobFailed    JobStatus = "failed"
	JobTimeout   JobStatus = "timeout"
	JobCancelled JobStatus = "cancelled"
)

// IsTerminal 对应 CompileJobStatus.is_terminal：SUCCESS/FAILED/TIMEOUT/CANCELLED。
func (s JobStatus) IsTerminal() bool {
	switch s {
	case JobSuccess, JobFailed, JobTimeout, JobCancelled:
		return true
	}
	return false
}

func (s JobStatus) String() string { return string(s) }

type CompileJob struct {
	BaseModel
	ProjectID UUID    `gorm:"not null;index"`
	UserID    UUID    `gorm:"not null;index"`
	Status    string  `gorm:"size:20;not null;default:pending;index"`
	RootFile  string  `gorm:"size:500;not null;default:main.tex"`
	Compiler  string  `gorm:"size:20;not null;default:pdflatex"`
	BuildID   *string `gorm:"size:100"`
	Error     *string // Text
	Logs      *string // Text
	SyncState datatypes.JSON

	Project Project `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE"`
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

// AllModels returns every model for AutoMigrate (the Go counterpart of
// Python's Base.metadata.create_all fallback).
// VerificationToken：邮箱验证 token 注册表（一次性，hash 存储）。
type VerificationToken struct {
	TokenHash string     `gorm:"column:token_hash;primaryKey;size:64"`
	UserID    UUID       `gorm:"column:user_id;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	UsedAt    *time.Time `gorm:"column:used_at"`
}

func (VerificationToken) TableName() string { return "verification_tokens" }

// AgentConversation：AI 助手会话（云端化，DB 持久）。
type AgentConversation struct {
	ID        UUID      `gorm:"column:id;primaryKey"`
	ProjectID UUID      `gorm:"column:project_id;not null;index"`
	UserID    UUID      `gorm:"column:user_id;not null"`
	Title     string    `gorm:"column:title;size:200;default:新对话"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AgentConversation) TableName() string { return "agent_conversations" }

type AgentMessage struct {
	ID             UUID           `gorm:"column:id;primaryKey"`
	ConversationID UUID           `gorm:"column:conversation_id;not null;index"`
	Role           string         `gorm:"column:role;size:20;not null"`
	Content        string         `gorm:"column:content;not null"`
	ToolEvents     datatypes.JSON `gorm:"column:tool_events"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (AgentMessage) TableName() string { return "agent_messages" }

// SystemSetting：admin 可改的运行时配置（key-value JSON）。
// 已知键：allowed_email_domains（[]string，空/缺省 = 不限制）。
type SystemSetting struct {
	Key       string         `gorm:"column:key;primaryKey;size:100"`
	Value     datatypes.JSON `gorm:"column:value;not null"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (SystemSetting) TableName() string { return "system_settings" }

func AllModels() []any {
	return []any{
		&User{}, &UserSettings{}, &ProjectGroup{}, &Project{},
		&Folder{}, &Document{}, &LatexFile{}, &CompileJob{}, &RefreshToken{},
		&SystemSetting{}, &VerificationToken{},
		&AgentConversation{}, &AgentMessage{},
	}
}

// RefreshToken：refresh token 注册表行（DB 持久化，重启不丢）。
// 只存 SHA-256 hash；本体不入库。
type RefreshToken struct {
	TokenHash string     `gorm:"column:token_hash;primaryKey;size:64"`
	UserID    UUID       `gorm:"column:user_id;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
