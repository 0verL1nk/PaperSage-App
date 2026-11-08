> 依赖：`data-integrity-refactor` 先落地。任务按阶段分批，每批自带迁移与测试、可独立验收。

## 1. 数据模型与迁移

- [x] 1.1 `server/schema/tables/`：新增 9 张表（turns / message_parts / runs / tool_calls / events / artifacts / change_proposals / evidence / wiki_sources）
- [x] 1.2 改造三张表：`agent_conversations` +`version`；`agent_messages` 去 `content`/`tool_events`、加 `turn_id`/`run_id`/`status`/`completed_at`；`agent_wiki_pages` +`source_run_id`/`stale_at`
- [x] 1.3 双方言迁移 `0013_agent_run_architecture.{up,down}.sql`（含 down 尽力还原 content / tool_events）
- [x] 1.4 数据迁移：`content` → text part、`tool_events` → tool part（整段保留，见迁移内注释说明为何不逐项展开）
- [x] 1.5 `store` 模型（`internal/store/agentrun.go`）+ `AllModels()` 注册
- [x] 1.6 迁移测试：隔离 PG 与 MySQL 8.4 服务上的 fresh install + handoff 集成测试已通过，结果记录于 `docs/agents/handoff-2026-10-07.md`；当前业务表数为 27（0016 只加字段，不增加表）。测试使用隔离服务 DSN，不使用生产库做迁移测试。

## 2. 事件协议层

- [x] 2.1 `internal/agent/protocol.go`：part 类型定义（start / text-* / start-step / finish-step / tool-input-* / tool-output-* / data-* / error / finish / abort / [DONE]）
- [x] 2.2 事件写入 `agent_events`：`appendEvent` 按 Run 分配单调 sequence 并推进 `runs.last_sequence`（当前用于 run.started；随工具状态机接入更多事件）
- [x] 2.3a SSE 输出改造：响应头 `x-vercel-ai-ui-message-stream: v1`；`sw.Start/TextStart/TextDelta/ToolInput/ToolInputAvailable/ToolOutput/Data/Error/Finish` 收口全部 part 输出，handler 内不再手拼
- [x] 2.4 测试同步到新词汇：`agent_test.go` / `agentic_search_test.go` 断言 `tool-input-start` / `tool-output-available` / `text-delta` / `finish`，并跳过终止标记 `[DONE]`（它不是 JSON part）
- [x] 2.3b keep-alive：每 20s 发 SSE 注释行心跳（`streamKeepAlive`），避免代理回收空闲连接。
  **订阅端断开只结束推送、不取消 Run**——Run 独立于连接存在，用户可稍后重订阅（模式 B 的意义）；
  要停止执行须显式 `POST /runs/:id/cancel`

## 3. Run / Turn 运行时

- [x] 3.1 `POST /agent/conversations/:id/runs`：创建 Turn + Run + 用户消息，返回 `{turnId, runId, userMessageId, streamUrl, threadVersion}`；未配置模型在此同步 400
- [x] 3.1a **执行与订阅分离**：`StartRun` 在独立 goroutine 中跑 `ExecuteRun`，Run 不碰 `http.ResponseWriter`——客户端断开后仍会跑完（参考架构 §3.2 模式 B）
- [x] 3.2 Thread 乐观锁：`expectedThreadVersion` 不匹配 → 409 且响应体带服务端最新 `threadVersion`
- [x] 3.3 `GET /agent/runs/:run_id/events?afterSeq=N`：先补历史再跟随实时；事件真相在 `agent_events`，进程内 hub 只做唤醒（多实例/重启不丢事件）
- [x] 3.4 `POST /agent/runs/:run_id/cancel`：登记取消函数；取消终止后续步骤但**不回滚已生效的写入**（「取消不等于已撤销」）；终态写入用独立 ctx
- [x] 3.5 `POST /agent/runs/:run_id/retry`：复用同一 Turn 新建 Run（重试不新增用户消息）
- [x] 3.6 配置快照：`model_provider`（ResolvedModel.Source）、`model_name`、`prompt_version`——**不含 API key**
- [x] 3.7 契约测试 `TestRunLifecycleContract`：创建 → 订阅（协议头 + finish 收尾）→ 幂等（同 clientRequestId 复用同一 Turn **与同一 Run**）→ 乐观锁 409 → 重试 → 取消 → 越权 404
- [x] 3.8 消息改为 Part 承载：`createUserMessage` / `createAssistantMessage` 写 text/tool part；`loadHistory` 读 text part 拼接；`GET .../messages` 返回 parts（`content` 内联 JSON 对象）

## 4. 工具状态机、只读工具集与多 Agent 架构

- [x] 4.1 工具定义扩展：`ToolSpec` 登记表（`internal/agent/toolcalls.go`）承载 `riskLevel` / `requiresApproval` / `timeout`。Eino 的 `InferTool` 只带 name/description/schema，治理属性塞进描述文本既不可查询也无法强制，故按工具名登记
- [x] 4.2 状态机落库：`agent_tool_calls` 记录 `seq`（Run 内单调）+ 全生命周期 `state` + `idempotency_key = sha256(runID, seq, name, args)`
- [x] 4.3 主 agent **摘除 `write_latex_file`**，新增 `propose_edit`：校验路径、校验 `old_text` 逐字存在于文件当前内容（否则当场报错让模型自纠）、写入 `agent_change_proposals` 并记下基准哈希
- [x] 4.4 `POST /agent/tool-calls/:id/approval`：归属校验、参数哈希比对、幂等（已决策不再处理）；批准即应用其关联建议
- [x] 4.5 大结果走 `agent_artifacts`：超 8KB 的工具结果转存 artifact，消息与事件流只留摘要 + 引用；`GET /agent/artifacts/:id` 取回
- [x] 4.6 测试：`TestProjectToolsHasNoWriteTool`（P0 断言：写盘工具必须已摘除）+ 生命周期测试断言工具调用落库（seq / risk / 幂等键 / 终态）
- [x] 4.7 子 Run：`search_references` 调用时创建 `parent_run_id` 指向父 Run 的子 Run，形成 Run 树
- [x] 4.8 结果压缩：**靠结构而非格式约定**——父 agent 只收到子 agent 的最终回答，多轮检索过程留在子 Run 供审计，父上下文不会被撑满。
  **不给子 agent 规定固定输出模板**：那等于把父 agent 的消费方式写死进子 agent 的提示词，换个父 agent 或换个问题就得重写（见 `tools.go` 中 SearchTool 的 Instruction 注释）
- [x] 4.9 上下文隔离：子 agent 经 Eino `AgentAsTool` 调用，**结构上**不继承父 Run 的会话历史（父只收它的压缩结论），故父上下文不会被多子 agent 撑满
- [x] 4.10 `data-activity` part：子 agent 活动嵌入当前 Assistant 消息，不另起顶层发言者
- [x] 4.11 测试：`TestAgenticSearchSubagent` 断言子 Run 存在、`agent_key = search_references`、指向父 Run、同会话、且父 Run 无上级（树只有一层）
- [x] 4.12 **基座改用 Eino prebuilt `DeepAgent`**（`adk/prebuilt/deep`），取代裸 `adk.NewChatModelAgent`：
  - 子 agent **声明式**交给 `deep.Config.SubAgents`，由 DeepAgent 生成 `task` 工具（`subagent_type` / `description`）编排，
    不再手工包 `adk.NewAgentTool`（那样等于自己实现框架已有的东西）
  - `ToolsConfig.EmitInternalEvents: true`：子 agent 的事件带 `AgentEvent.AgentName` 流出。
    **不问"哪个子 agent"由我们自己解析工具参数去猜——框架已经在事件上告诉我们了**
  - 不设 `Backend` / `Shell`：不注册任何文件系统与命令执行工具（写作应用不需要，多一套 shell 也是不打算暴露的能力）；
    `WithoutGeneralSubAgent: true`（通用子 agent 会继承全部工具且职责不明）
  - 不设 `MaxIteration`：用框架默认，不凭空定轮数上限
  - 事件分流：子 agent 的事件写进**它自己的子 Run**（独立 sequence / events / tool_calls）并留档，
    但**不计入主回答**——否则子 agent 的原文会混进用户看到的回答，等于让它成为第二个发言者。
    测试 `TestAgenticSearchSubagent` 钉住该语义（含"子 agent 中间结论不得出现在主回答里"）
  - 代价：`deep` 包连带引入 `adk/filesystem`（→ `bmatcuk/doublestar/v4`）依赖，已 go mod tidy

## 5. 建议的确认与应用

- [x] 5.1 `POST /agent/change-proposals/:id/apply`：服务端**重新**校验归属 + 仍为 pending + 基准哈希与稿件当前内容一致，才执行替换并写盘
- [x] 5.2 基准不匹配 → 409 且建议转 `stale`，稿件不被修改
- [x] 5.3 `POST /agent/change-proposals/:id/reject` → `rejected`，稿件不变
- [x] 5.4 幂等：非 pending 一律拒绝重复应用
- [x] 5.5 契约测试 `TestProposalApplyReject`：应用成功 / 越权 404 / 重复应用 409 / 基准漂移 409+stale 且稿件不变 / 驳回不改稿 / 按状态列表
- [x] 5.6 `GET /projects/:id/agent/proposals?status=`：刷新后仍能看到未决建议（流里推过的 part 不提供持久列表）

## 6. 前端服务层

- [x] 6.1 `api-routes.ts` 增 `AGENT` 路由常量（会话/消息/Run/事件/取消/重试/建议/artifact）
- [x] 6.2 `services/agent-api.ts`：会话与消息、创建 Run、审批、建议 apply/reject、取消/重试；**`streamRun` 用 fetch + ReadableStream**（EventSource 不支持 POST 与 Authorization 头），跨 chunk 缓冲不完整 JSON 并跳过终止标记 `[DONE]`
- [x] 6.3 单测 `agent-api.test.ts`：5 例——含**跨 chunk 边界不丢事件**、单条坏 JSON 不中断整条流、**HTTP 失败抛错而非静默返回空流**

## 7. 前端状态与界面

- [x] 7.1-7.2 chat store + 纯函数 `reduceChatEvent`（`store/chat-store.ts`）；6 例单测覆盖：同 id 文本累加、**重放不重复（`start` 重置该消息 parts）**、**工具按 `toolCallId` 归并且同名工具两次调用不串味**、建议登记、会话信息、错误
- [x] 7.3 AIPanel 实装（`views/projects/ai-panel.tsx`）：会话下拉、消息流、PromptInput（Enter 发送 / Shift+Enter 换行）、流式渲染、停止
- [x] 7.4 工具卡片：可折叠，展开看入参出参，状态显示在标题行
- [x] 7.5 建议卡片：diff 双色预览 + 理由 + 应用/拒绝 + 过期态
- [x] 7.6 未配置模型引导：直接给「去设置」入口，**且不渲染输入框**（不给一个发不出去的空框）
- [x] 7.7 三层信息密度：对话主线 / 消息内活动（工具卡片、子 agent 活动行）/ 按需展开的执行细节
- [x] 7.8 模型选择器回写 `settings.agent.active_provider_id`（此前选中值只留在本地 state，**根本到不了后端**）
- [x] 7.9 组件测试 `ai-panel.test.tsx`：未配置态给引导且无输入框；已配置态有输入框

## 8. 联动

- [x] 8.1 应用建议后失效 `latexKeys.files` 与 `latexKeys.content`，刷新文件树与编辑器——
  否则用户会在旧内容上继续编辑、再被自动保存把助手的改动盖回去。
  为此把查询键抽到 `@/lib/query-keys`：从 `use-latex` 导入会把 pdfjs/jszip 拖进面板与测试
- [x] 8.2 编译联动：`chat-store` 从 `compile_project` 的工具结果里取出 job id（`pendingCompileJobId`），
  `WritingPage` 取走后用它启动编译轮询——否则助手触发的编译要等用户再点一次编译才看得到。
  测试含反向断言：其他工具的文本里出现 `job id:` 不得被误取

## 9. e2e 与验收

- [x] 9.1-9.3 e2e 已在本机**实际执行**（后端 + seed + `cmd/fakes3` 本地 S3 替身 + Vite，无 Docker）。
  全套 **56 通过 / 1 跳过 / 0 失败**（`compile-real` 跳过：需真实 CLSI）。
  - **登录限速**：`POST /auth/login` 有 8 次/分钟/账号的内存滑动窗口（`internal/auth/email_flow.go`，
    刻意做成常量，注释说明「如同 bcrypt cost，不是业务配置」）。全套有 ~33 次真实登录，
    密集跑必然触发 `Too many attempts, try later`，表现是 `waitForURL` 超时、快照停在登录页。
    这是既有测试基建与限速器的相互作用，与本次改动无关（`git diff` 确认限速器代码未被触碰）；
    限速器计数在**后端进程**里，所以连续两次 Playwright 调用也会叠加。
    CI 用 `retries: 1` 吸收这类抖动；本机按「每批 ≤7 次登录 + 批间隔 65s（窗口 1 分钟）」分批跑，全绿。
- [x] 9.5 **e2e 抓出两个真 bug**（都不是测试问题，已修并补测）：
  - **面板永远显示「尚未配置 AI 模型」**：`AiPanel` 自己不加载 settings，而是依赖 `ModelSelector`
    去加载——但 `ModelSelector` 只在 `hasProvider` 为真时渲染，构成死循环。改为面板自己
    `fetchSettings()`；补组件测试断言「settings 为空时会发起加载」。
  - **新项目的第一句话无处可去**：Run 路径要求会话已存在，而**没有任何接口能建会话**
    （建会话只藏在旧 `chat` handler 的 `ensureConversation` 里）。新项目会话列表为空 →
    前端没有 conversationId → `send()` 直接 return，用户看到输入被清空却什么都没发生。
    补 `POST /projects/:id/agent/conversations`（含归属校验）+ 前端首条消息先建会话；
    补后端契约测试 `TestCreateConversationThenRun`（越权 404 / 建会话 / 立刻可发 Run /
    列表可见）、前端 store 测试（无会话时先建、有会话时不重复建）、e2e 用例。
  - 顺带修一处不一致：`listConversations` 把 32 位无横线 hex 直接吐给前端，
    而本域其余响应（run/turn/artifact）一律 `.String()` 带横线 → 同一 id 在列表与创建
    两个接口里长得不一样。统一回规范形式。
- [x] 9.4 四个目标分别跑过且全绿：`check-backend`（gofmt+vet+编译）、`check-frontend`（typecheck+lint）、
  `test-backend`、`test-frontend`（175 通过 / 1 跳过）
  （Makefile 里没有聚合的 `check`/`test` 目标，顺带修正了 `CLAUDE.md` 里并不存在的 `make check` / `make test` 命令）

## 10. 文档

- [x] 10.1 `CLAUDE.md` agent 域小节：Run/Turn、事件协议、工具状态机、审批与建议
- [x] 10.2 `docs/project/12-ai-writing-design.md` §7/§8 状态同步
- [x] 10.3 `.env` 模板补 `AGENT_*`（承接 `2026-09-13-ai-agent-module` task 9）
- [x] 10.4 `2026-09-13-ai-agent-module/tasks.md` 的 7–9 标注为「被本变更取代」

## 11. 对抗性审查（Workflow：5 维度发现 → 对抗性复核 → 完整性批评）

21 个 agent、15 条复核、6 条确认、9 条被否。确认项处置：

- [x] 11.1 `loadHistory` 的 `Limit(50)`：超过 50 条的会话会**静默丢掉最近消息**（本轮刚写入的用户提问也取不到），还会把一次问答从中间劈开 → 去掉条数上限（上下文应由 token 预算约束，不是条数魔法数）
- [x] 11.2 子 agent 提示词里的「至多 4 轮」→ 删除（轮次由框架的 ReAct 循环管）
- [x] 11.3 RRF 的 `ranks += 10_000` → 改为该路真实名次。**顺带发现更根本的错**：上一版只换了基数，没改「名次相加后只取一次倒数」——现按 §6.2 公式实现（逐路取倒数相加、名次从 1 起），补 `internal/rag/rrf_test.go`
- [x] 11.4 `ToolSpec.Timeout` 写了从不读（运维会误以为工具真被超时限住）→ 删字段
- [x] 11.5 子 Run 的终态事件排在 `run.finished` 之后（订阅端永远收不到），且所有退出路径一律标 `completed` → 终态前先收子 Run、结局随主 Run、加幂等保护
- [x] 11.6 工具状态机缺 `ready`/`validating`/`input_streaming` → **不补假状态**：这三档在本实现里没有写入点（参数由框架校验、工具同步执行）。改为只记录真实迁移，偏离写进 design D4
- [x] 11.7 **顺带修掉一个真竞态**（修 11.5 后全量跑才暴露）：订阅端用「Run 是否终态」判断收流，而 `finishRunTx` 改状态与 `run.finished` 落库之间存在窗口 → 会把**已落库但尚未读到的终答整段丢掉**。改为「终态 **且** 已读到该 Run 的最大 sequence」才收流；连续 3 次全量并行验证通过

被否掉的 9 条中一条值得记：有复核者认为 `listMessages` 的 `.Table(...)` 会让子 agent 文本泄漏 —— 复核判定**不可达**（该方法只服务主 Run），核对后同意，未改。
