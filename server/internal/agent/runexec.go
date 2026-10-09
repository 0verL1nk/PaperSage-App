package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// Run 事件类型（写入 agent_events；传输层由订阅端翻译成协议 part）。
const (
	evtRunStarted  = "run.started"
	evtTextDelta   = "text.delta"
	evtToolInput   = "tool.input"
	evtToolOutput  = "tool.output"
	evtRunError    = "error"
	evtRunFinished = "run.finished"
	// evtActivity 子 agent 活动：作为当前 Assistant 消息内的活动呈现，
	// 不另起一个顶层发言者（参考架构 §9.1 的 UI 约定）。
	evtActivity = "activity"
	// evtProposal 修改建议详情（含 diff 锚点）：前端据此渲染建议卡片。
	evtProposal = "proposal"
)

// mainAgentName 主对话 agent 的名字；子 agent 的名字与它不同，据此区分事件来源。
const mainAgentName = "papersage-writing-assistant"

// createChildRun 为一个子 agent 开子 Run，形成 Run 树。
// 子 Run 有自己的 events 与工具调用记录；父 Run 只收它的压缩结论。
func (s *Service) createChildRun(ctx context.Context, rc runContext, agentKey string) (*store.AgentRun, error) {
	now := time.Now().UTC()
	parent := rc.RunID
	child := store.AgentRun{
		ID: store.MustNewUUID(), ConversationID: rc.ConversationID,
		ParentRunID: &parent, AgentKey: agentKey,
		Status: RunRunning, StartedAt: &now,
	}
	if err := s.DB.WithContext(ctx).Create(&child).Error; err != nil {
		return nil, err
	}
	return &child, nil
}

// artifactThreshold 定义「大结果」的门槛：超过它的工具结果转存 artifact，
// 消息与事件流里只留摘要 + 取回链接（参考架构 §8.5）。
//
// 这个数字是一个可调的经验值，不是硬性契约：定在 8KB 是因为它是「一条消息里
// 放进去会让会话记录迅速膨胀、但模型又不需要逐字看到」的量级。
// 需要改就改这里一处，不要在下游再散落别的截断阈值。
const artifactThreshold = 8 * 1024

// storeArtifact 把大结果落成 artifact，返回 (摘要文本, artifactID)。
// 失败时原样返回结果——存不下来不该让整轮对话失败。
func (s *Service) storeArtifact(ctx context.Context, rc runContext, kind, content string) (string, *store.UUID) {
	if len(content) <= artifactThreshold {
		return content, nil
	}
	id := store.MustNewUUID()
	key := "projects/" + rc.ProjectID.String() + "/agent-artifacts/" + id.String()
	if err := s.Storage.Put(ctx, key, []byte(content)); err != nil {
		return content, nil
	}
	head := content
	if len(head) > 1024 {
		head = head[:1024]
	}
	art := store.AgentArtifact{
		ID: id, RunID: &rc.RunID, ProjectID: rc.ProjectID,
		Kind: kind, S3Key: key, Mime: "text/plain", SizeBytes: int64(len(content)),
	}
	if err := s.DB.WithContext(ctx).Create(&art).Error; err != nil {
		return content, nil
	}
	return fmt.Sprintf("%s\n…（完整结果 %d 字节，已存为 artifact %s，可用 GET /agent/artifacts/%s 取回）",
		head, len(content), id.String(), id.String()), &id
}

// runTimeout 是 Run 的**泄漏兜底**，不是产品层面的时长限制：
//   - 用 context.WithTimeout 而不是 Background()，是为了让卡住的模型请求
//     最终能释放（否则一个挂死的 goroutine 永远占着连接与内存）。
//   - 它是「上面还压着一层模型/HTTP 超时」时的最后一道，处于正常业务时长之上，
//     不应被当作业务语义（比如「一次对话只能跑 10 分钟」）。
const runTimeout = 30 * time.Minute

// runHub 进程内唤醒信号：让订阅端尽快感知新事件。
// 事件真相在 DB，所以多实例部署或进程重启都不会丢事件——
// hub 只影响延迟，不影响正确性。
type runHub struct {
	mu   sync.Mutex
	subs map[store.UUID]map[chan struct{}]struct{}
}

func newRunHub() *runHub {
	return &runHub{subs: map[store.UUID]map[chan struct{}]struct{}{}}
}

func (h *runHub) subscribe(runID store.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.subs[runID] == nil {
		h.subs[runID] = map[chan struct{}]struct{}{}
	}
	h.subs[runID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs[runID], ch)
		if len(h.subs[runID]) == 0 {
			delete(h.subs, runID)
		}
		h.mu.Unlock()
	}
}

func (h *runHub) publish(runID store.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[runID] {
		select {
		case ch <- struct{}{}:
		default: // 已有待处理的唤醒信号，无需堆积
		}
	}
}

// ── 执行 ────────────────────────────────────────────────────────────

// runContext 执行一个 Run 所需的一切，由调用方在创建 Run 时解析好，
// 使执行不依赖 HTTP 请求上下文（客户端断开后 Run 照常跑完）。
type runContext struct {
	RunID          store.UUID
	ConversationID store.UUID
	ProjectID      store.UUID
	UserID         store.UUID
	Content        string
}

// StartRun 在独立 goroutine 中执行 Run。立刻返回，执行进度经 agent_events 暴露。
// 同时登记取消函数，供 POST /runs/:id/cancel 使用。
func (s *Service) StartRun(rc runContext) {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	s.cancelMu.Lock()
	s.cancels[rc.RunID] = cancel
	s.cancelMu.Unlock()

	go func() {
		defer func() {
			s.cancelMu.Lock()
			delete(s.cancels, rc.RunID)
			s.cancelMu.Unlock()
			cancel()
			s.hub.publish(rc.RunID) // 终态也要唤醒订阅端，否则它会等到轮询才收尾
		}()
		s.ExecuteRun(ctx, rc)
	}()
}

// CancelRun 取消一个进行中的 Run；返回它此前是否在运行。
// 「断流不等于成功，取消也不等于已撤销」：取消只终止后续步骤，
// 已经生效的写入不因此回滚（见 design D1）。
func (s *Service) CancelRun(runID store.UUID) bool {
	s.cancelMu.Lock()
	cancel, ok := s.cancels[runID]
	s.cancelMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// ExecuteRun 跑完一次 ReAct 循环，边跑边把过程写入 agent_events 与 parts。
//
// 与旧的「在 handler 里同步跑」相比，这里刻意不碰 http.ResponseWriter：
// Run 的生死与连接无关（参考架构 §3.2 模式 B）。
func (s *Service) ExecuteRun(ctx context.Context, rc runContext) {
	defer s.hub.publish(rc.RunID)

	textID := "text_" + rc.RunID.String()
	var final strings.Builder
	type toolEvt struct {
		Tool   string `json:"tool"`
		Result string `json:"result"`
	}
	var toolEvents []toolEvt
	toolNames := map[string]string{}

	fail := func(msg string) {
		_ = s.appendEvent(ctx, rc.ConversationID, rc.RunID, evtRunError, map[string]string{"message": msg})
		if _, err := s.persistAssistant(ctx, rc, final.String(), toolEvents, RunFailed); err != nil {
			s.finishRun(ctx, rc.RunID, RunFailed, nil, "persist assistant message failed")
		}
		_ = s.finishRunTx(ctx, rc, RunFailed)
		_ = s.appendEvent(ctx, rc.ConversationID, rc.RunID, evtRunFinished,
			map[string]string{"status": RunFailed})
	}

	resolved, err := ResolveModelWithEnv(ctx, s.DB, rc.UserID, s.Cfg)
	if err != nil {
		fail("尚未配置 AI 模型")
		return
	}
	chatModel, err := NewChatModel(ctx, resolved)
	if err != nil {
		fail("模型初始化失败")
		return
	}

	history, err := s.loadHistory(ctx, rc.ConversationID)
	if err != nil {
		fail("读取历史失败")
		return
	}
	tools, err := ProjectTools(s.DB, s.Storage, s.Queue, rc.ProjectID, rc.UserID, rc.RunID)
	if err != nil {
		fail("构建工具失败")
		return
	}
	// 子 agent 交给 DeepAgent 编排：它会生成一个 `task` 工具
	// （参数 subagent_type / description），由主 agent 自主决定何时委派。
	var subAgents []adk.Agent
	if s.RAG != nil {
		if tcm, ok := chatModel.(model.ToolCallingChatModel); ok {
			if sa, serr := SearchSubAgent(ctx, s.DB, s.RAG, rc.ProjectID, rc.UserID, rc.RunID, tcm); serr == nil {
				subAgents = append(subAgents, sa)
			}
		}
	}

	// 主 agent 用 Eino prebuilt DeepAgent（ChatModelAgent + 子 agent 编排 + write_todos 规划）。
	// 不设 Backend / Shell：不注册文件系统与命令执行工具——写作应用不需要，
	// 多出一套 shell 也是我们不打算暴露的能力。
	agent, err := deep.New(ctx, &deep.Config{
		Name:        mainAgentName,
		Description: "论文写作助手（可读写项目文件与文献、触发编译）",
		ChatModel:   chatModel,
		Instruction: systemPrompt,
		SubAgents:   subAgents,
		ToolsConfig: adk.ToolsConfig{
			// 让子 agent 的事件（带 AgentName）一并流出来：框架已经说明事件来自哪个
			// agent，不必再去解析 task 工具的参数猜目标（那是重复造轮子）。
			EmitInternalEvents: true,
			ToolsNodeConfig:    compose.ToolsNodeConfig{Tools: tools},
		},
		// 不设 MaxIteration：用 DeepAgent 自己的默认行为，不凭空定一个轮数上限。
		// 真要限流应由运维配置（模型侧 token/超时预算）决定，不是这里的魔法数字。
		// 通用子 agent 会继承全部工具且职责不明，与「领域受限」的取向不符，不予启用
		WithoutGeneralSubAgent: true,
	})
	if err != nil {
		fail("Agent 构建失败")
		return
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})

	// record 只把事件写进该 Run 的日志（子 agent 的过程要留档供审计，但不推给主订阅流）。
	record := func(runID store.UUID, eventType string, payload any) {
		_ = s.appendEvent(ctx, rc.ConversationID, runID, eventType, payload)
	}
	// emit 除了留档还唤醒主 Run 的订阅者——客户端订阅的始终是主 Run。
	emit := func(runID store.UUID, eventType string, payload any) {
		record(runID, eventType, payload)
		s.hub.publish(rc.RunID)
	}

	// 子 Run：开 EmitInternalEvents 后事件自带 AgentName，框架已经告诉我们事件来自
	// 哪个 agent——因此不必解析 task 工具参数去猜目标子 agent（那是重复造轮子）。
	// 首次见到某个子 agent 就为它开一个子 Run；主 Run 结束时统一收敛。
	childRunsByAgent := map[string]store.UUID{}
	syncSubAgent := func(agentName string) store.UUID {
		if agentName == "" || agentName == mainAgentName {
			return store.UUID{}
		}
		if id, seen := childRunsByAgent[agentName]; seen {
			return id
		}
		child, cerr := s.createChildRun(ctx, rc, agentName)
		if cerr != nil {
			return store.UUID{}
		}
		childRunsByAgent[agentName] = child.ID
		emit(rc.RunID, evtActivity, map[string]any{
			"runId": child.ID.String(), "title": agentName,
			"status": RunRunning, "parentRunId": rc.RunID.String(),
		})
		return child.ID
	}
	// closeChildren 收敛所有子 Run 并广播终态。必须在主 Run 写 run.finished **之前**
	// 调用：run.finished 是订阅端的收流信号，之后追加的事件不会再送达订阅者，
	// 子任务的「已完成」就会永远看不到。
	// 子 Run 的实际结局随主 Run：失败/取消时不谎报 completed。
	closed := false
	closeChildren := func(final string) {
		if closed {
			return // 幂等：正常路径收过之后，兜底的 defer 不应再改写终态或重复广播
		}
		closed = true
		bg := context.Background()
		now := time.Now().UTC()
		for name, id := range childRunsByAgent {
			_ = s.DB.WithContext(bg).Model(&store.AgentRun{}).Where("id = ?", id).
				Updates(map[string]any{"status": final, "completed_at": now}).Error
			_ = s.appendEvent(bg, rc.ConversationID, rc.RunID, evtActivity, map[string]any{
				"runId": id.String(), "title": name,
				"status": final, "parentRunId": rc.RunID.String(),
			})
		}
		s.hub.publish(rc.RunID) // 唤醒订阅端把这几条推出去
	}
	// 兜底：任何提前 return 的路径（构建失败、模型错误、取消）也要收掉子 Run。
	// 正常路径已先调用过，闭包内幂等，不会覆盖终态。
	defer closeChildren(RunFailed)
	toolCallRows := map[string]store.UUID{} // toolCallId → agent_tool_calls.id
	toolArgs := map[string]string{}         // toolCallId → 原始 args（关联建议时需要）
	toolSeq := 0

	// emitCalls 记录工具调用（状态机 + 幂等键），子 agent 调用另开子 Run。
	// 工具调用事件带 toolCallId：前端不再需要按同名队列配对。
	emitCalls := func(tcs []schema.ToolCall, targetRun store.UUID, silent bool) {
		for _, tc := range tcs {
			if _, seen := toolCallRows[tc.ID]; seen {
				continue // 流式 chunk 可能重发同一调用
			}
			toolNames[tc.ID] = tc.Function.Name
			toolArgs[tc.ID] = tc.Function.Arguments
			toolSeq++
			if row, rerr := s.recordToolCall(ctx, s.DB, targetRun, toolSeq,
				tc.Function.Name, tc.Function.Arguments); rerr == nil {
				toolCallRows[tc.ID] = row.ID
			}
			if silent {
				// 子 agent 的调用：记在子 Run 名下留档，但不推进主回答
				record(targetRun, evtToolInput, map[string]string{
					"toolCallId": tc.ID, "toolName": tc.Function.Name,
					"input": tc.Function.Arguments,
				})
				continue
			}
			emit(targetRun, evtToolInput, map[string]string{
				"toolCallId": tc.ID, "toolName": tc.Function.Name,
				"input": tc.Function.Arguments,
			})
		}
	}
	handleMsg := func(m *schema.Message, targetRun store.UUID, silent bool) {
		if m == nil {
			return
		}
		switch m.Role {
		case schema.Assistant:
			emitCalls(m.ToolCalls, targetRun, silent)
			// silent 时只留档，不计入主回答：子 agent 的原文不属于用户的回答
			//（用户只该看到主 agent 的结论 + 一条子任务活动）。
			if m.Content != "" {
				if silent {
					record(targetRun, evtTextDelta, map[string]string{"id": textID, "delta": m.Content})
				} else {
					final.WriteString(m.Content)
					emit(targetRun, evtTextDelta, map[string]string{"id": textID, "delta": m.Content})
				}
			}
		case schema.Tool:
			name := toolNames[m.ToolCallID]
			if name == "" {
				name = "tool"
			}
			// 大结果改存 artifact（消息与事件流里只留摘要 + 引用）；
			// 其余原样输出，不做额外字符截断——截断只会让结构化内容变成无法解析的半截文本。
			// 模型上下文由工具自身负责（如 read_document 的抽取上限）。
			res, _ := s.storeArtifact(ctx, rc, name, m.Content)
			if rowID, ok := toolCallRows[m.ToolCallID]; ok {
				if specFor(name).RequiresApproval {
					// 建议类工具：调用停在 awaiting_approval，直到用户对建议做出决定；
					// 顺带把建议挂到这次调用上（args 里带 path），并把 diff 推给前端。
					// Run 不挂起——助手已经交付了建议，等待确认的是用户而非模型。
					if args, ok := toolArgs[m.ToolCallID]; ok {
						s.linkProposal(ctx, rowID, rc.RunID, args)
					}
					s.finishToolCall(ctx, rowID, ToolAwaiting, res, "")
					if p, perr := s.latestProposalOfCall(ctx, rowID); perr == nil {
						emit(targetRun, evtProposal, proposalPayload(p))
					}
				} else {
					s.finishToolCall(ctx, rowID, ToolSucceeded, res, "")
				}
			}
			if silent {
				record(targetRun, evtToolOutput, map[string]string{"toolCallId": m.ToolCallID, "output": res})
			} else {
				emit(targetRun, evtToolOutput, map[string]string{"toolCallId": m.ToolCallID, "output": res})
				toolEvents = append(toolEvents, toolEvt{Tool: name, Result: res})
			}
		}
	}

	iter := runner.Run(ctx, history)
	for {
		// 取消：终止后续步骤，但不回滚已经生效的写入
		//（「断流不等于成功，取消也不等于已撤销」）。终态写入用独立 ctx，
		// 因为此时 ctx 已被取消。
		select {
		case <-ctx.Done():
			bg := context.Background()
			_ = s.finishRunTx(bg, rc, RunCancelled)
			_ = s.appendEvent(bg, rc.ConversationID, rc.RunID, evtRunFinished,
				map[string]string{"status": RunCancelled})
			return
		default:
		}

		ev, ok := iter.Next()
		if !ok {
			break
		}
		// 事件来自哪个 agent 由框架给出（EmitInternalEvents）：子 agent 的事件写进它
		// 自己的子 Run，且**不计入主回答**——否则子 agent 会成为用户眼里的第二个发言者。
		targetRun := rc.RunID
		silent := false
		if ev.AgentName != "" && ev.AgentName != mainAgentName {
			silent = true
			if id := syncSubAgent(ev.AgentName); id != (store.UUID{}) {
				targetRun = id
			}
		}
		if ev.Err != nil {
			fail(friendlyErr(ev.Err))
			return
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mo := ev.Output.MessageOutput
		if mo.IsStreaming && mo.MessageStream != nil {
			for {
				chunk, rerr := mo.MessageStream.Recv()
				if rerr != nil || chunk == nil {
					break
				}
				emitCalls(chunk.ToolCalls, targetRun, silent)
				if chunk.Content == "" {
					continue
				}
				if silent {
					record(targetRun, evtTextDelta, map[string]string{"id": textID, "delta": chunk.Content})
					continue
				}
				final.WriteString(chunk.Content)
				emit(targetRun, evtTextDelta, map[string]string{"id": textID, "delta": chunk.Content})
			}
		} else {
			handleMsg(mo.Message, targetRun, silent)
		}
	}

	if final.Len() == 0 && len(toolEvents) == 0 {
		fail("模型未返回内容，请重试")
		return
	}
	if _, err := s.persistAssistant(ctx, rc, final.String(), toolEvents, RunCompleted); err != nil {
		s.finishRun(ctx, rc.RunID, RunFailed, nil, "persist assistant message failed")
	} else {
		_ = s.finishRunTx(ctx, rc, RunCompleted)
	}
	// 先收子 Run，再发主 Run 的终态（顺序见 closeChildren 的注释）
	closeChildren(RunCompleted)
	emit(rc.RunID, evtRunFinished, map[string]string{"status": RunCompleted})
}

// persistAssistant 落库助手终答（文本 + 工具活动 part）并推进会话更新时间。
func (s *Service) persistAssistant(ctx context.Context, rc runContext, content string, toolEvents any, status string) (*store.AgentMessage, error) {
	msg, err := s.createAssistantMessage(ctx, s.DB, rc.ConversationID, &rc.RunID, content, toolEvents)
	if err != nil {
		return nil, err
	}
	_ = s.DB.WithContext(ctx).Model(&store.AgentConversation{}).
		Where("id = ?", rc.ConversationID).Update("updated_at", time.Now().UTC()).Error
	return msg, nil
}

// finishRunTx 收敛 Run 终态（占用 endTime 与 output 由调用方另行设置）。
func (s *Service) finishRunTx(ctx context.Context, rc runContext, status string) error {
	return s.DB.WithContext(ctx).Model(&store.AgentRun{}).
		Where("id = ?", rc.RunID).
		Updates(map[string]any{"status": status, "completed_at": time.Now().UTC()}).Error
}

// ── 订阅 ────────────────────────────────────────────────────────────

// streamRun 把一个 Run 的事件从 fromSeq 之后推成 UI Message Stream part。
// 先补历史（断线重放），再跟随实时事件，直到 Run 进入终态。
func (s *Service) streamRun(ctx context.Context, sw *streamWriter, runID, convID store.UUID, fromSeq int64) error {
	wake, unsubscribe := s.hub.subscribe(runID)
	defer unsubscribe()

	// 先发 start；会话/Run id 走 data-* 扩展（协议本身没有 meta 事件）。
	sw.Start("msg_" + runID.String())
	sw.Data("conversation", map[string]string{
		"conversationId": convID.String(),
		"runId":          runID.String(),
	})

	last := fromSeq
	textOpen := false
	// 事件靠 hub 唤醒即时推送；这个轮询只是**跨进程兜底**（多实例部署时
	// 另一个实例写入的事件唤不醒本进程），故间隔取得比人眼能察觉的延迟更短即可。
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	ping := time.NewTicker(streamKeepAlive)
	defer ping.Stop()

	for {
		var events []store.AgentEvent
		if err := s.DB.WithContext(ctx).
			Where("run_id = ? AND sequence > ?", runID, last).
			Order("sequence ASC").Find(&events).Error; err != nil {
			return err
		}
		terminal := false
		for _, ev := range events {
			last = ev.Sequence
			terminal = s.emitProtocol(sw, ev, &textOpen) || terminal
		}
		if terminal {
			sw.Finish()
			return nil
		}

		// 兜底：Run 已终态但**该 Run 已无更多事件可读**时才收流。
		// 不能只凭 status 就收：finishRunTx 把状态改为终态与 run.finished 落库之间
		// 存在窗口，此时收流会把「已落库但尚未读到」的终答整段丢掉（真实竞态，
		// 负载高时暴露）。判据必须是「终态 + 已读到该 Run 的最大 sequence」。
		var run store.AgentRun
		if err := s.DB.WithContext(ctx).
			Select("status", "last_sequence").Where("id = ?", runID).First(&run).Error; err != nil {
			return err
		}
		if run.Status != RunRunning && last >= run.LastSequence {
			sw.Finish()
			return nil
		}

		select {
		case <-ctx.Done():
			// 订阅端断开：只结束推送，**不取消 Run**——Run 独立于连接存在，
			// 用户可以稍后重新订阅（这正是模式 B 的意义）。
			// 要停止执行得显式 POST /runs/:id/cancel。
			return ctx.Err()
		case <-wake:
		case <-ping.C:
			sw.ping()
		case <-poll.C:
		}
	}
}

// emitProtocol 把一个事件翻译成协议 part；返回该事件是否终结了整个流。
func (s *Service) emitProtocol(sw *streamWriter, ev store.AgentEvent, textOpen *bool) bool {
	var p map[string]any
	if len(ev.PayloadJSON) > 0 {
		_ = json.Unmarshal(ev.PayloadJSON, &p)
	}
	str := func(k string) string {
		if v, ok := p[k].(string); ok {
			return v
		}
		return ""
	}
	switch ev.EventType {
	case evtTextDelta:
		id := str("id")
		if !*textOpen {
			sw.TextStart(id)
			*textOpen = true
		}
		sw.TextDelta(id, str("delta"))
	case evtToolInput:
		if *textOpen {
			sw.TextEnd(str("id"))
			*textOpen = false
		}
		sw.ToolInput(str("toolCallId"), str("toolName"))
		sw.ToolInputAvailable(str("toolCallId"), str("toolName"), str("input"))
	case evtToolOutput:
		sw.ToolOutput(str("toolCallId"), str("output"))
	case evtActivity:
		sw.Data("activity", p)
	case evtProposal:
		sw.Data("proposal", p)
	case evtRunError:
		sw.Error(str("message"))
	case evtRunFinished:
		if *textOpen {
			sw.TextEnd(str("id"))
			*textOpen = false
		}
		return true
	}
	return false
}

// ErrRunNotFound 订阅或取消一个不存在的 Run。
var ErrRunNotFound = errors.New("run not found")
