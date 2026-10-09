package store

import (
	"time"

	"gorm.io/datatypes"
)

// ── Agent 运行时模型 ────────────────────────────────────────────────
// 表结构以 server/schema/tables/*.sql 为真相源；internal/store/schema_drift_test.go
// 会逐表比对模型与真相源的列集合，两处必须同时改。
// 设计依据：openspec/changes/agent-run-architecture/design.md D8。

// AgentTurn 一次用户提交。client_request_id 是幂等键：双击发送/网络重试
// 不得产生第二个 Turn，也不得触发第二次模型调用。
type AgentTurn struct {
	ID              UUID      `gorm:"column:id;primaryKey"`
	ConversationID  UUID      `gorm:"column:conversation_id;not null;uniqueIndex:uq_turn_client_request,priority:1"`
	UserMessageID   *UUID     `gorm:"column:user_message_id"`
	RootRunID       *UUID     `gorm:"column:root_run_id"`
	ClientRequestID *string   `gorm:"column:client_request_id;size:64;uniqueIndex:uq_turn_client_request,priority:2"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (AgentTurn) TableName() string { return "agent_turns" }

// AgentMessagePart 消息的组成部分：文本 / 工具 / 活动 / 审批 / 引用 / 大结果 / 错误。
// ordinal 决定消息内顺序，前端 Event Reducer 据此幂等归并。
type AgentMessagePart struct {
	ID          UUID           `gorm:"column:id;primaryKey"`
	MessageID   UUID           `gorm:"column:message_id;not null;uniqueIndex:uq_part_ordinal,priority:1"`
	Ordinal     int            `gorm:"column:ordinal;not null;default:0;uniqueIndex:uq_part_ordinal,priority:2"`
	Type        string         `gorm:"column:type;size:20;not null"`
	Status      string         `gorm:"column:status;size:20;not null"`
	TextContent *string        `gorm:"column:text_content"`
	ContentJSON datatypes.JSON `gorm:"column:content_json"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	CompletedAt *time.Time     `gorm:"column:completed_at"`
}

func (AgentMessagePart) TableName() string { return "agent_message_parts" }

// AgentRun 一次执行。独立于 HTTP 连接存在；parent_run_id 形成 Run 树
// （子 agent 挂子 Run，但用户始终只面对一个 Assistant）。
type AgentRun struct {
	ID              UUID           `gorm:"column:id;primaryKey"`
	ConversationID  UUID           `gorm:"column:conversation_id;not null;index:ix_runs_conversation_started,priority:1"`
	TurnID          *UUID          `gorm:"column:turn_id;index"`
	ParentRunID     *UUID          `gorm:"column:parent_run_id;index"`
	AgentKey        string         `gorm:"column:agent_key;size:64;not null"`
	Status          string         `gorm:"column:status;size:20;not null"`
	OutputMessageID *UUID          `gorm:"column:output_message_id"`
	ModelProvider   string         `gorm:"column:model_provider;size:64;not null"`
	ModelName       string         `gorm:"column:model_name;size:128;not null"`
	PromptVersion   string         `gorm:"column:prompt_version;size:32;not null"`
	Temperature     *float64       `gorm:"column:temperature;type:decimal(4,2)"`
	InputTokens     int            `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens    int            `gorm:"column:output_tokens;not null;default:0"`
	LatencyMs       int64          `gorm:"column:latency_ms;not null;default:0"`
	LastSequence    int64          `gorm:"column:last_sequence;not null;default:0"`
	ErrorJSON       datatypes.JSON `gorm:"column:error_json"`
	StartedAt       *time.Time     `gorm:"column:started_at;index:ix_runs_conversation_started,priority:2"`
	CompletedAt     *time.Time     `gorm:"column:completed_at"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (AgentRun) TableName() string { return "agent_runs" }

// AgentToolCall 工具调用的状态机记录。
// seq 取代原先「按同名工具队列配对」的脆弱约定；
// idempotency_key 防断线重试导致的副作用重复。
type AgentToolCall struct {
	ID               UUID           `gorm:"column:id;primaryKey"`
	RunID            UUID           `gorm:"column:run_id;not null;uniqueIndex:uq_tool_run_seq,priority:1"`
	Seq              int            `gorm:"column:seq;not null;default:0;uniqueIndex:uq_tool_run_seq,priority:2"`
	ToolName         string         `gorm:"column:tool_name;size:64;not null"`
	State            string         `gorm:"column:state;size:24;not null;index"`
	RiskLevel        string         `gorm:"column:risk_level;size:12;not null"`
	RequiresApproval bool           `gorm:"column:requires_approval;not null;default:false"`
	InputJSON        datatypes.JSON `gorm:"column:input_json"`
	OutputJSON       datatypes.JSON `gorm:"column:output_json"`
	ArtifactID       *UUID          `gorm:"column:artifact_id"`
	ApprovalID       *string        `gorm:"column:approval_id;size:64"`
	ArgumentsHash    *string        `gorm:"column:arguments_hash;size:64"`
	IdempotencyKey   *string        `gorm:"column:idempotency_key;size:128;uniqueIndex"`
	StartedAt        *time.Time     `gorm:"column:started_at"`
	CompletedAt      *time.Time     `gorm:"column:completed_at"`
	ErrorJSON        datatypes.JSON `gorm:"column:error_json"`
	CreatedAt        time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (AgentToolCall) TableName() string { return "agent_tool_calls" }

// AgentEvent 执行事实记录：断线恢复、审计与用量统计的来源。
// sequence 留在服务端，传输层对外用的是 AI SDK UI Message Stream Protocol。
type AgentEvent struct {
	ID             UUID           `gorm:"column:id;primaryKey"`
	ConversationID UUID           `gorm:"column:conversation_id;not null;index"`
	RunID          UUID           `gorm:"column:run_id;not null;uniqueIndex:uq_event_run_seq,priority:1"`
	Sequence       int64          `gorm:"column:sequence;not null;uniqueIndex:uq_event_run_seq,priority:2"`
	EventType      string         `gorm:"column:event_type;size:48;not null"`
	EntityType     *string        `gorm:"column:entity_type;size:32"`
	EntityID       *UUID          `gorm:"column:entity_id"`
	PayloadJSON    datatypes.JSON `gorm:"column:payload_json"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (AgentEvent) TableName() string { return "agent_events" }

// AgentArtifact 工具的大结果（不进消息与模型上下文，只留摘要 + 引用）。
type AgentArtifact struct {
	ID          UUID           `gorm:"column:id;primaryKey"`
	RunID       *UUID          `gorm:"column:run_id;index"`
	ProjectID   UUID           `gorm:"column:project_id;not null;index"`
	Kind        string         `gorm:"column:kind;size:32;not null"`
	S3Key       string         `gorm:"column:s3_key;size:500;not null"`
	Mime        string         `gorm:"column:mime;size:128;not null"`
	SizeBytes   int64          `gorm:"column:size_bytes;not null;default:0"`
	SummaryJSON datatypes.JSON `gorm:"column:summary_json"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (AgentArtifact) TableName() string { return "agent_artifacts" }

// AgentChangeProposal 助手对稿件的修改建议。
// 助手不写盘：base_content_hash 记的是产出建议时的稿件版本，
// 稿件一变旧建议即失效，避免用旧基准覆盖新内容。
type AgentChangeProposal struct {
	ID              UUID       `gorm:"column:id;primaryKey"`
	ProjectID       UUID       `gorm:"column:project_id;not null;index:ix_proposal_project_status,priority:1"`
	RunID           *UUID      `gorm:"column:run_id;index"`
	ToolCallID      *UUID      `gorm:"column:tool_call_id"`
	FilePath        string     `gorm:"column:file_path;size:500;not null"`
	BaseContentHash string     `gorm:"column:base_content_hash;size:64;not null"`
	OldText         string     `gorm:"column:old_text;not null"`
	NewText         string     `gorm:"column:new_text;not null"`
	Reason          *string    `gorm:"column:reason"`
	Status          string     `gorm:"column:status;size:20;not null;index:ix_proposal_project_status,priority:2"`
	AppliedAt       *time.Time `gorm:"column:applied_at"`
	RejectedAt      *time.Time `gorm:"column:rejected_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
}

func (AgentChangeProposal) TableName() string { return "agent_change_proposals" }

// AgentEvidence 证据身份与版本。当前 source_version 记录检索摘录的 SHA-256，
// 不代表整个来源文档的不可变版本。source_id 是多态引用（document / latex_file /
// wiki / chunk），DB 层不设外键，由应用层保证。
type AgentEvidence struct {
	ID            UUID      `gorm:"column:id;primaryKey"`
	RunID         UUID      `gorm:"column:run_id;not null;index;uniqueIndex:uq_evidence_run_chunk_version,priority:1"`
	SourceType    string    `gorm:"column:source_type;size:20;not null;index:ix_evidence_source,priority:1"`
	SourceID      UUID      `gorm:"column:source_id;not null;index:ix_evidence_source,priority:2"`
	SourceName    string    `gorm:"column:source_name;size:255;not null;default:''"`
	SourceVersion *string   `gorm:"column:source_version;size:64;uniqueIndex:uq_evidence_run_chunk_version,priority:3"`
	ChunkID       *string   `gorm:"column:chunk_id;size:80;uniqueIndex:uq_evidence_run_chunk_version,priority:2"`
	Locator       *string   `gorm:"column:locator;size:255"`
	Quote         *string   `gorm:"column:quote"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (AgentEvidence) TableName() string { return "agent_evidence" }

// AgentWikiSource wiki 页的来源回溯：来源 ID / 版本 / 片段。
type AgentWikiSource struct {
	ID            UUID      `gorm:"column:id;primaryKey"`
	WikiPageID    UUID      `gorm:"column:wiki_page_id;not null;index"`
	SourceType    string    `gorm:"column:source_type;size:20;not null;index:ix_wiki_source_source,priority:1"`
	SourceID      UUID      `gorm:"column:source_id;not null;index:ix_wiki_source_source,priority:2"`
	SourceVersion *string   `gorm:"column:source_version;size:64"`
	ChunkID       *string   `gorm:"column:chunk_id;size:80"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (AgentWikiSource) TableName() string { return "agent_wiki_sources" }
