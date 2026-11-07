package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 工具风险分级（参考架构 §12.1 工具权限分层）。
const (
	RiskRead        = "read"
	RiskWrite       = "write"
	RiskDestructive = "destructive"
)

// 工具调用状态机。**只使用实际会被写入的状态**：
//
//	executing → succeeded | failed            普通工具（框架已校验好参数才派发）
//	executing → awaiting_approval → approved | denied   需用户确认的工具
//
// 设计文档（design.md D4）按参考架构列了完整链路
// `input_streaming → ready → validating → awaiting_approval → approved|denied → executing → succeeded|failed`，
// 但那几档在我们的实现里没有对应的写入点：参数由 Eino 的工具节点做 schema 校验、
// 工具是同步执行的，不存在「流式收参」「排队待命」「执行前再校验」三个可观测阶段。
// 为凑齐状态名而写假状态，比少写几个状态更糟——审计日志一旦有假值就整份不可信。
const (
	ToolExecuting = "executing"
	ToolAwaiting  = "awaiting_approval"
	ToolApproved  = "approved"
	ToolDenied    = "denied"
	ToolSucceeded = "succeeded"
	ToolFailed    = "failed"
)

// ToolSpec 工具的审计与治理元数据。
//
// 为什么单独建表而不是塞进 Eino 的工具定义：Eino 的 InferTool 只承载
// name/description/schema，没有风险分级、审批要求这些治理属性；把它们
// 硬塞进描述文本既不可查询也无法强制。这里按工具名登记，执行时据以落库。
//
// 只登记真正被读取的字段。曾有过一个 Timeout 字段，但没有任何超时执行点
// 会用它（工具本身没有 per-tool 超时执行）——写了不读的配置比没有更糟：
// 运维会以为 search_references 真被 180s 限住。
type ToolSpec struct {
	Risk             string
	RequiresApproval bool
}

var toolSpecs = map[string]ToolSpec{
	"list_documents":    {Risk: RiskRead},
	"read_document":     {Risk: RiskRead},
	"list_latex_files":  {Risk: RiskRead},
	"read_latex_file":   {Risk: RiskRead},
	"search_references": {Risk: RiskRead},
	"wiki_read":         {Risk: RiskRead},
	"compile_project":   {Risk: RiskWrite},
	// 建议类工具：产出建议但不写盘，仍需用户确认才由服务端应用。
	"propose_edit": {Risk: RiskWrite, RequiresApproval: true},
	// 写研究蓝图与 wiki：工具自身要求用户确认/修订号，但审计必须显示为 write。
	"save_writing_preparation": {Risk: RiskWrite},
	"wiki_write":               {Risk: RiskWrite},
}

// specFor 未知工具按最保守的写处理（不会因未登记而放开权限）。
func specFor(name string) ToolSpec {
	if s, ok := toolSpecs[name]; ok {
		return s
	}
	return ToolSpec{Risk: RiskWrite}
}

// recordToolCall 落一条工具调用记录并返回它。
// idempotency_key = sha256(runID, seq, toolName, args)：同一 Run 内同序号同参数
// 视为同一次调用，断线重试不会重复产生副作用（参考架构 §8.4、§11.1）。
func (s *Service) recordToolCall(ctx context.Context, tx *gorm.DB,
	runID store.UUID, seq int, name, args string) (*store.AgentToolCall, error) {
	spec := specFor(name)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%s", runID.String(), seq, name, args)))
	key := hex.EncodeToString(sum[:])

	tc := store.AgentToolCall{
		ID: store.MustNewUUID(), RunID: runID, Seq: seq, ToolName: name,
		State: ToolExecuting, RiskLevel: spec.Risk, RequiresApproval: spec.RequiresApproval,
		IdempotencyKey: &key,
	}
	if args != "" {
		tc.InputJSON = datatypes.JSON([]byte(args))
	}
	now := time.Now().UTC()
	tc.StartedAt = &now
	if err := tx.WithContext(ctx).Create(&tc).Error; err != nil {
		return nil, err
	}
	return &tc, nil
}

// finishToolCall 收敛一次工具调用的终态与结果。
func (s *Service) finishToolCall(ctx context.Context, id store.UUID, state, result, errMsg string) {
	now := time.Now().UTC()
	updates := map[string]any{"state": state, "completed_at": now}
	if result != "" {
		updates["output_json"] = datatypes.JSON(mustJSON(map[string]string{"text": result}))
	}
	if errMsg != "" {
		updates["error_json"] = datatypes.JSON(mustJSON(map[string]string{"message": errMsg}))
	}
	_ = s.DB.WithContext(ctx).Model(&store.AgentToolCall{}).Where("id = ?", id).Updates(updates).Error
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
