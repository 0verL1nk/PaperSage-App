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
	ID        UUID           `gorm:"primaryKey"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"not null;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"` // 软删除：Delete() 落 UPDATE，查询自动过滤
}

// BeforeCreate 填充 uuid4 主键（Python 侧 default=uuid.uuid4）。
func (b *BaseModel) BeforeCreate(*gorm.DB) error {
	if b.ID.UUID == uuid.Nil {
		b.ID = UUID{UUID: uuid.Must(uuid.NewV7())} // v7：时间有序（RFC 9562），索引局部性更好
	}
	return nil
}

// SoftDeleteModel 用于唯一键需要包含 deleted_seq 的表。
//
// 与 BaseModel 的唯一区别是 DeletedAt 改用 SoftDeleteTime——逻辑删除时
// 在同一条 UPDATE 里额外写入 deleted_seq，使唯一键可以带上它。
// 使用本基类的模型 MUST 自行声明：
//
//	DeletedSeq int64 `gorm:"not null;default:0;uniqueIndex:<索引名>,priority:<末位>"`
//
// 否则删除行仍会占住唯一键（见 SoftDeleteTime 的说明）。
type SoftDeleteModel struct {
	ID        UUID           `gorm:"primaryKey"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"not null;autoUpdateTime"`
	DeletedAt SoftDeleteTime `gorm:"index"`
}

// BeforeCreate 填充 uuid4 主键。
func (b *SoftDeleteModel) BeforeCreate(*gorm.DB) error {
	if b.ID.UUID == uuid.Nil {
		b.ID = UUID{UUID: uuid.Must(uuid.NewV7())}
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
	Name        string  `gorm:"size:100;not null"`
	Description *string // Text
	OwnerID     UUID    `gorm:"not null;index"`
	GroupID     *UUID   `gorm:"index"`
	Status      string  `gorm:"size:20;not null;default:active"`

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
	SoftDeleteModel
	ProjectID   UUID    `gorm:"not null;uniqueIndex:uq_project_file_path,priority:1;index"`
	FilePath    string  `gorm:"size:500;not null;uniqueIndex:uq_project_file_path,priority:2"`
	ContentType *string `gorm:"size:100"`
	SizeBytes   int64   `gorm:"not null;default:0"`
	ContentHash *string `gorm:"size:64;index"`
	S3Key       *string `gorm:"size:500"`
	LastSavedAt *time.Time
	// 逻辑删除时由 SoftDeleteTime 写入非零值，使已删行不再占用 uq_project_file_path。
	DeletedSeq int64 `gorm:"not null;default:0;uniqueIndex:uq_project_file_path,priority:3"`

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
// 用 gorm.DeletedAt 而非 SoftDeleteTime：本表无唯一键，不需要 deleted_seq。
type AgentConversation struct {
	ID        UUID           `gorm:"column:id;primaryKey"`
	ProjectID UUID           `gorm:"column:project_id;not null;index"`
	UserID    UUID           `gorm:"column:user_id;not null"`
	Title     string         `gorm:"column:title;size:200;default:新对话"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
	// 乐观锁：提交消息携带 expected_thread_version，不匹配则 409。
	Version int64 `gorm:"column:version;not null;default:0"`
}

func (AgentConversation) TableName() string { return "agent_conversations" }

// AgentMessage 不可变（表无 updated_at），故不复用 BaseModel。
// 内容由 agent_message_parts 承载，不再有 content / tool_events 两列。
type AgentMessage struct {
	ID             UUID           `gorm:"column:id;primaryKey"`
	ConversationID UUID           `gorm:"column:conversation_id;not null;index:ix_agent_messages_history,priority:1"`
	TurnID         *UUID          `gorm:"column:turn_id"`
	RunID          *UUID          `gorm:"column:run_id;index"`
	Role           string         `gorm:"column:role;size:20;not null"`
	Status         string         `gorm:"column:status;size:20;not null"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime;index:ix_agent_messages_history,priority:2"`
	CompletedAt    *time.Time     `gorm:"column:completed_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AgentMessage) TableName() string { return "agent_messages" }

// RAG 域模型（表定义在 rag 包，这里注册 AutoMigrate 用的裸结构）。

// SystemSetting：admin 可改的运行时配置（key-value JSON）。
// 已知键：allowed_email_domains（[]string，空/缺省 = 不限制）。
type SystemSetting struct {
	Key       string         `gorm:"column:key;primaryKey;size:100"`
	Value     datatypes.JSON `gorm:"column:value;not null"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// extraModels 由各域在 init() 中注册；rag 依赖 store，反向 import 会成环，
// 故用注册口而非在 store 内重复声明域外结构（那会造成第二份真相源）。
var extraModels []any

// RegisterModels 注册域外模型参与 AutoMigrate（SQLite 建表路径）。
func RegisterModels(models ...any) { extraModels = append(extraModels, models...) }

func coreModels() []any {
	return []any{
		&User{}, &UserSettings{}, &ProjectGroup{}, &Project{},
		&Folder{}, &Document{}, &LatexFile{}, &CompileJob{}, &RefreshToken{},
		&SystemSetting{}, &VerificationToken{},
		&AgentConversation{}, &AgentMessage{}, &AgentMessagePart{},
		&AgentTurn{}, &AgentRun{}, &AgentToolCall{}, &AgentEvent{},
		&AgentArtifact{}, &AgentChangeProposal{}, &AgentEvidence{}, &AgentWikiSource{},
	}
}

// AllModels 返回 AutoMigrate 的完整模型集合。
func AllModels() []any { return append(coreModels(), extraModels...) }

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
