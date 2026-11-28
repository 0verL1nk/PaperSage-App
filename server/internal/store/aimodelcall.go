package store

import "time"

// AIModelCall 一次模型调用的计量事实：只追加、按保留期硬删。
//
// 与 agent_runs 分离的原因：一次 Run 会发多次模型调用（agent 循环、子 agent、
// 压缩摘要），Run 级汇总给不出每次调用的状态、错误码与首字延迟。
//
// 隐私边界：不存 prompt/回复正文、API Key、base_url；token 列可空，
// provider 不返回 usage 时保持 NULL，绝不补 0 冒充有数据。
type AIModelCall struct {
	ID               UUID      `gorm:"column:id;primaryKey"`
	UserID           *UUID     `gorm:"column:user_id;index:ix_ai_calls_user_created,priority:1"`
	ProjectID        *UUID     `gorm:"column:project_id;index:ix_ai_calls_project_created,priority:1"`
	RunID            *UUID     `gorm:"column:run_id;index:ix_ai_calls_run"`
	FeatureKind      string    `gorm:"column:feature_kind;size:32;not null;index:ix_ai_calls_feature_day,priority:1"`
	Provider         string    `gorm:"column:provider;size:64;not null"`
	ProviderSource   string    `gorm:"column:provider_source;size:20;not null"`
	Model            string    `gorm:"column:model;size:128;not null"`
	IsStream         bool      `gorm:"column:is_stream;not null;default:false"`
	Status           string    `gorm:"column:status;size:16;not null"`
	ErrorCode        *string   `gorm:"column:error_code;size:32"`
	HTTPStatus       *int16    `gorm:"column:http_status;type:smallint"`
	PromptTokens     *int      `gorm:"column:prompt_tokens"`
	CompletionTokens *int      `gorm:"column:completion_tokens"`
	ReasoningTokens  *int      `gorm:"column:reasoning_tokens"`
	LatencyMs        int       `gorm:"column:latency_ms;not null"`
	FirstTokenMs     *int      `gorm:"column:first_token_ms"`
	CreatedDay       string    `gorm:"column:created_day;size:10;not null;index:ix_ai_calls_day_model,priority:1;index:ix_ai_calls_feature_day,priority:2"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime;index:ix_ai_calls_created"`
}

func (AIModelCall) TableName() string { return "ai_model_calls" }
