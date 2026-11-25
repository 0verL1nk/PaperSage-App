package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/sources"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

// toolErrorPrefix 标记「工具失败但 Run 继续」的结果；模型据此解释失败。
const toolErrorPrefix = "工具执行失败："

// credentialURL 把错误里的连接串凭据脱敏后再交给外部模型。
var credentialURL = regexp.MustCompile(`(?i)(\w+://)[^/\s:@]+:[^/\s@]+@`)

// toolFailureTracker 记录哪些 toolCallId 失败过。错误文本本身只是给模型看的
// 结果，不能反过来当作状态判据（正常输出也可能以同样的字开头）。
type toolFailureTracker struct{ failed sync.Map }

func (t *toolFailureTracker) failedCall(callID string) bool {
	_, ok := t.failed.Load(callID)
	return ok
}

// middleware 把工具失败变成模型可读的结果。
// 默认行为是工具返回 error 即中断整个 Run，用户只会看到「模型调用失败」——
// 但文件不存在、锚点对不上这类问题模型本可以自行解释或改用别的做法。
// 取消/超时仍然向上抛：那是用户或系统要求停止，不是可恢复的工具错误。
func (t *toolFailureTracker) middleware(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
	return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		out, err := next(ctx, input)
		if err == nil {
			return out, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		t.failed.Store(input.CallID, struct{}{})
		slog.Warn("tool call failed", "tool", input.Name, "call_id", input.CallID, "err", err)
		return &compose.ToolOutput{Result: toolErrorPrefix + credentialURL.ReplaceAllString(err.Error(), "$1***:***@")}, nil
	}
}

// ProjectTools 构建 project 作用域的工具集（agentic 核心：读文献、
// 读写 LaTeX、触发编译）。每次请求构建，闭包携带归属校验。

// normRefMeta 是落到建议上的条款元数据：服务端按本次注入集合解析，模型无法伪造。
type normRefMeta struct {
	Cite    string `json:"cite"`
	Version int    `json:"version,omitempty"`
	Title   string `json:"title"`
	Rule    string `json:"rule"`
	Source  string `json:"source"`
}

const maxNormRefsPerProposal = 10

// resolveNormRefs 把模型给出的引用号解析成本次注入集合里的真实条款。
// 未命中的引用号整条拒绝——宁可让模型重试，也不落一条无法回查的依据。
func resolveNormRefs(ctx context.Context, provider norms.Provider, projectID store.UUID, cites []string) (datatypes.JSON, error) {
	empty := datatypes.JSON([]byte("[]"))
	if len(cites) == 0 {
		return empty, nil
	}
	if len(cites) > maxNormRefsPerProposal {
		return nil, fmt.Errorf("一条建议最多引用 %d 条规范", maxNormRefsPerProposal)
	}
	if provider == nil {
		return nil, fmt.Errorf("项目未接入学术规范库，不能引用规范条款")
	}
	entries, err := provider.AdoptedForProject(ctx, projectID)
	if errors.Is(err, norms.ErrUnavailable) {
		return nil, fmt.Errorf("学术规范库不可用，不能引用规范条款")
	}
	if err != nil {
		return nil, fmt.Errorf("读取项目规范失败：%w", err)
	}
	byCite := make(map[string]norms.Entry, len(entries))
	for _, entry := range entries {
		byCite[entry.Cite()] = entry
	}
	out := make([]normRefMeta, 0, len(cites))
	seen := map[string]bool{}
	for _, raw := range cites {
		cite := strings.TrimSpace(raw)
		if cite == "" || seen[cite] {
			continue
		}
		seen[cite] = true
		entry, ok := byCite[cite]
		if !ok {
			return nil, fmt.Errorf("引用号 %s 不在本项目生效的规范里。只能用上下文中列出的编号，或去掉规范引用", cite)
		}
		out = append(out, normRefMeta{
			Cite: cite, Version: entry.Version, Title: entry.Title, Rule: entry.Rule, Source: entry.Source,
		})
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(encoded), nil
}

func ProjectTools(db *gorm.DB, stg storage.Storage, normsProvider norms.Provider,
	projectID, ownerID, runID store.UUID, documentReadLimitBytes int) ([]tool.BaseTool, error) {

	verify := func(ctx context.Context) error {
		var n int64
		if err := db.WithContext(ctx).Model(&store.Project{}).
			Where("id = ? AND owner_id = ?", projectID, ownerID).
			Count(&n).Error; err != nil || n == 0 {
			return fmt.Errorf("project not accessible")
		}
		return nil
	}
	// 跨章节检查用：记录本轮实际读过的材料，报告落库时以此覆盖模型自报的来源。
	reviewReads := newReviewReads()

	// ── 读侧 ──────────────────────────────────────────────

	listDocuments, err := utils.InferTool(
		"list_documents",
		"列出项目已入库的参考文献（名称/类型/大小/备注）。用户问「我有哪些文献/参考资料」时调用。",
		func(ctx context.Context, _ struct{}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			var docs []store.Document
			if err := db.WithContext(ctx).
				Where("project_id = ? AND status = ?", projectID, "uploaded").
				Order("created_at DESC").Find(&docs).Error; err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 篇：\n", len(docs))
			for _, d := range docs {
				remark := ""
				if d.Remark != nil {
					remark = *d.Remark
				}
				fmt.Fprintf(&b, "- %s (document_id: %s, %s, %d bytes) %s\n", d.Name, d.ID.String(), d.Type, d.Size, remark)
			}
			return b.String(), nil
		})
	if err != nil {
		return nil, err
	}

	documentReader := documentTextReader{maxSegmentBytes: documentReadLimitBytes}
	readDocument, err := utils.InferTool(
		"read_document",
		"读取已入库参考文献的正文信息或指定范围（PDF/DOCX/TXT）。同名文件使用list_documents提供的document_id；省略max_bytes只返回总字节数和摘要；续读传next_offset和expected_digest，不要声称未读取部分已被核查。",
		func(ctx context.Context, in documentReadInput) (*documentReadResult, error) {
			if err := verify(ctx); err != nil {
				return nil, err
			}
			query := db.WithContext(ctx).Where("project_id = ? AND status = ?", projectID, "uploaded")
			if in.DocumentID != "" {
				id, err := uuid.Parse(in.DocumentID)
				if err != nil {
					return nil, fmt.Errorf("invalid document_id")
				}
				query = query.Where("id = ?", store.UUID{UUID: id})
			} else if in.Name != "" {
				query = query.Where("name = ?", in.Name)
			} else {
				return nil, fmt.Errorf("document_id or name is required")
			}
			var docs []store.Document
			if err := query.Limit(2).Find(&docs).Error; err != nil {
				return nil, err
			}
			if len(docs) == 0 {
				return nil, fmt.Errorf("document not found")
			}
			if len(docs) > 1 {
				return nil, fmt.Errorf("multiple documents have this name; use document_id from list_documents")
			}
			doc := docs[0]
			result, err := documentReader.read(ctx, stg, doc, in)
			if err != nil {
				return nil, err
			}
			if err := verify(ctx); err != nil {
				return nil, err
			}
			var current store.Document
			if err := db.WithContext(ctx).
				Where("id = ? AND project_id = ? AND status = ?", doc.ID, projectID, "uploaded").
				First(&current).Error; err != nil || current.S3Key != doc.S3Key || !current.UpdatedAt.Equal(doc.UpdatedAt) {
				return nil, fmt.Errorf("document changed or is no longer available; read again")
			}
			reviewReads.record("document", doc.Name)
			return result, nil
		})
	if err != nil {
		return nil, err
	}

	sourcesSvc := sources.NewService(db, stg)
	readSource, err := utils.InferTool(
		"read_source_version",
		"读取来源版本的抽取文本区间（UTF-8 字节偏移）。先用 list_documents 获取 source_version_id；引用依据时记录该版本与引文，未读取的部分不要声称已核查。",
		func(ctx context.Context, in struct {
			SourceVersionID string `json:"source_version_id"`
			Offset          int64  `json:"offset,omitempty" jsonschema:"description=起始字节偏移，默认 0"`
			Limit           int    `json:"limit,omitempty" jsonschema:"description=本次最多读取字节数，默认/上限由服务端控制"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			id, err := uuid.Parse(in.SourceVersionID)
			if err != nil {
				return "", fmt.Errorf("invalid source_version_id")
			}
			text, next, done, err := sourcesSvc.ReadText(ctx, ownerID, projectID, store.UUID{UUID: id}, in.Offset, in.Limit)
			if err != nil {
				return "", err
			}
			reviewReads.record("source", in.SourceVersionID)
			encoded, err := json.Marshal(map[string]any{"text": text, "next_offset": next, "done": done})
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	listLatexFiles, err := utils.InferTool(
		"list_latex_files",
		"列出项目的 LaTeX 源文件树（路径/大小）。了解论文结构时调用。",
		func(ctx context.Context, _ struct{}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			var files []store.LatexFile
			if err := db.WithContext(ctx).
				Where("project_id = ?", projectID).
				Order("file_path").Find(&files).Error; err != nil {
				return "", err
			}
			var b strings.Builder
			for _, f := range files {
				fmt.Fprintf(&b, "- %s (%d bytes)\n", f.FilePath, f.SizeBytes)
			}
			return b.String(), nil
		})
	if err != nil {
		return nil, err
	}

	readLatexFile, err := utils.InferTool(
		"read_latex_file",
		"读取一个 LaTeX 源文件的内容。",
		func(ctx context.Context, in struct {
			Path string `json:"path" jsonschema:"description=文件路径，如 main.tex"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			f, content, err := findLatex(ctx, db, stg, projectID, in.Path)
			if err != nil {
				return "", err
			}
			_ = f
			reviewReads.record("latex", in.Path)
			return content, nil
		})
	if err != nil {
		return nil, err
	}

	// ── 写侧（agentic 分水岭）────────────────────────────

	// ── 建议侧：只产出建议，不写盘 ────────────────────────
	//
	// 助手不再直接改稿。原先的 write_latex_file（整文件覆盖、无确认、无版本校验）
	// 已摘除——docs/project/12-ai-writing-design.md §10 P0 要求「切断未经确认的
	// 稿件覆盖」。改为产出带锚点的建议，由用户确认后经服务端校验基准版本再应用。
	proposeEdit, err := utils.InferTool(
		"propose_edit",
		"提出对某个 LaTeX 源文件的修改建议（不直接改稿，用户确认后才应用）。用户提出任何改稿请求（包括“给个建议”“帮我改”“润色这段”）都必须调用本工具，不要用回复里的代码块代替建议卡。需给出目标文件路径、要替换的原文片段、替换后的新文与理由。old_text 必须与文件当前内容逐字一致，否则会被拒绝——不确定时先 read_latex_file。",
		func(ctx context.Context, in struct {
			Path    string `json:"path" jsonschema:"description=文件路径，如 main.tex"`
			OldText string `json:"old_text" jsonschema:"description=要被替换的原文片段（必须与文件当前内容逐字一致）"`
			NewText string `json:"new_text" jsonschema:"description=替换后的新文"`
			Reason  string `json:"reason" jsonschema:"description=修改理由"`
			// 只能引用系统提示里列出的本项目规范编号；服务端会按本次注入集合校验。
			NormRefs []string `json:"norm_refs,omitempty" jsonschema:"description=本建议依据的项目规范引用号（形如 N-GBT7714@v1），只能取自上下文列出的条目；没有依据就留空"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			if err := validateAgentPath(in.Path); err != nil {
				return "", err
			}
			if in.OldText == "" {
				return "", fmt.Errorf("old_text 不能为空")
			}
			_, content, err := findLatex(ctx, db, stg, projectID, in.Path)
			if err != nil {
				return "", err
			}
			if !strings.Contains(content, in.OldText) {
				// 当场报错让模型自我纠正，而不是留下一条注定无法应用的无效建议。
				return "", fmt.Errorf(
					"old_text 在 %s 的当前内容中找不到。请先 read_latex_file 取回原文，再给出逐字一致的片段", in.Path)
			}
			sum := md5.Sum([]byte(content))
			base := hex.EncodeToString(sum[:])
			normRefs, err := resolveNormRefs(ctx, normsProvider, projectID, in.NormRefs)
			if err != nil {
				return "", err
			}
			p := store.AgentChangeProposal{
				ID: store.MustNewUUID(), ProjectID: projectID, RunID: &runID,
				FilePath: in.Path, BaseContentHash: base,
				OldText: in.OldText, NewText: in.NewText, NormRefs: normRefs, Status: "pending",
			}
			if in.Reason != "" {
				p.Reason = &in.Reason
			}
			if err := db.WithContext(ctx).Create(&p).Error; err != nil {
				return "", err
			}
			return fmt.Sprintf(
				"已生成修改建议（%s，建议 id %s），等待用户确认。不要声称已修改文件——用户确认后才会应用。",
				in.Path, p.ID.String()), nil
		})
	if err != nil {
		return nil, err
	}

	compileProject, err := utils.InferTool(
		"compile_project",
		"请求当前写作页在用户浏览器中运行 LaTeX 编译。只发起请求，不返回编译结果；结果由用户在写作页查看。",
		func(ctx context.Context, in struct {
			RootFile string `json:"root_file,omitempty" jsonschema:"description=编译入口文件，默认 main.tex 可省略"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			root := in.RootFile
			if root == "" {
				root = "main.tex"
			}
			var file store.LatexFile
			if err := db.WithContext(ctx).
				Where("project_id = ? AND file_path = ?", projectID, root).
				First(&file).Error; err != nil {
				return "", fmt.Errorf("LaTeX root file %q was not found in this project", root)
			}
			request, err := json.Marshal(struct {
				RootFile string `json:"root_file"`
			}{RootFile: root})
			if err != nil {
				return "", err
			}
			return "browser_compile_request:" + string(request) +
				"\n\n已请求浏览器在写作页编译；编译结果只在写作页显示，你无法读取，不要声称编译已成功。", nil
		})
	if err != nil {
		return nil, err
	}

	writingStateService := writingstate.NewService(db)
	getWritingState, err := utils.InferTool(
		"get_writing_state",
		"读取项目的研究蓝图、要求与阶段状态。写作准备对话开始或准备修改蓝图前先调用。",
		func(ctx context.Context, _ struct{}) (string, error) {
			state, err := writingStateService.GetForAgent(ctx, projectID.UUID, ownerID.UUID)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(state)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	saveWritingBrief, err := utils.InferTool(
		"save_writing_preparation",
		"保存用户明确确认后的研究蓝图与课题要求。保存前必须先向用户展示拟保存内容并得到明确同意；不得把假设或未确认信息写成事实。未修改要求时应原样保留 get_writing_state 返回的 requirements。必须先调用 get_writing_state 并提交当前 revision。",
		func(ctx context.Context, in struct {
			ExpectedRevision int                        `json:"expected_revision" jsonschema:"description=get_writing_state 返回的 revision，防止覆盖并发修改"`
			Topic            string                     `json:"topic"`
			Background       string                     `json:"background"`
			Questions        []string                   `json:"questions"`
			Goals            []string                   `json:"goals"`
			ScopeOut         []string                   `json:"scope_out"`
			ObjectsAndData   string                     `json:"objects_and_data"`
			Methods          string                     `json:"methods"`
			KeyChapters      []string                   `json:"key_chapters"`
			Milestones       []string                   `json:"milestones"`
			Constraints      []string                   `json:"constraints"`
			KnownFacts       []string                   `json:"known_facts"`
			Assumptions      []string                   `json:"assumptions"`
			Unverified       []string                   `json:"unverified"`
			Requirements     []writingstate.Requirement `json:"requirements,omitempty" jsonschema:"description=课题或导师要求；未修改时传回当前值，明确清空时传空数组"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			brief := writingstate.Brief{
				Topic: in.Topic, Background: in.Background,
				Questions: in.Questions, Goals: in.Goals, ScopeOut: in.ScopeOut,
				ObjectsAndData: in.ObjectsAndData, Methods: in.Methods,
				KeyChapters: in.KeyChapters, Milestones: in.Milestones,
				Constraints: in.Constraints, KnownFacts: in.KnownFacts,
				Assumptions: in.Assumptions, Unverified: in.Unverified,
			}
			for _, list := range []*[]string{&brief.Questions, &brief.Goals, &brief.ScopeOut, &brief.KeyChapters, &brief.Milestones, &brief.Constraints, &brief.KnownFacts, &brief.Assumptions, &brief.Unverified} {
				if *list == nil {
					*list = []string{}
				}
			}
			state, err := writingStateService.SavePreparationFromAgent(ctx, projectID.UUID, ownerID.UUID, in.ExpectedRevision, brief, in.Requirements)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(state)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	// ── 科研组织产物 ──────────────────────────────────────
	researchSvc := research.NewService(db)

	listResearch, err := utils.InferTool(
		"list_research_artifacts",
		"列出项目已保存的科研组织产物（阅读记录/文献比较/章节计划/章节摘要/跨章节检查报告）。回答前先读取已采用版本；不要把较新的候选当成用户已采用的计划。",
		func(ctx context.Context, in struct {
			Kind string `json:"kind,omitempty" jsonschema:"description=可选：literature_note/literature_comparison/chapter_plan/chapter_digest/chapter_review"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			items, err := researchSvc.List(ctx, ownerID, projectID, in.Kind)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(items)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	readResearch, err := utils.InferTool(
		"read_research_artifact",
		"读取单个组织产物的当前或指定修订（含 payload、状态与生成来源）。",
		func(ctx context.Context, in struct {
			ArtifactID string `json:"artifact_id"`
			Revision   int    `json:"revision,omitempty" jsonschema:"description=省略或 0 表示当前修订"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			id, err := uuid.Parse(in.ArtifactID)
			if err != nil {
				return "", fmt.Errorf("invalid artifact_id")
			}
			var out any
			if in.Revision > 0 {
				out, err = researchSvc.GetRevision(ctx, ownerID, projectID, store.UUID{UUID: id}, in.Revision)
			} else {
				out, err = researchSvc.Get(ctx, ownerID, projectID, store.UUID{UUID: id})
			}
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(out)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	createResearch, err := utils.InferTool(
		"create_research_artifact",
		"保存一份组织产物候选（不会自动采用）。payload_json 必须符合对应 kind 的结构；每个已报告字段必须带原文依据（sourceVersionId/documentId），未读或未报告要如实标注，不得编造来源。chapter_review 报告落库前会逐条核对引文是否真的来自原文，核对不过会被拒绝。",
		func(ctx context.Context, in struct {
			Kind        string `json:"kind" jsonschema:"description=literature_note/literature_comparison/chapter_plan/chapter_digest/chapter_review"`
			PayloadJSON string `json:"payload_json" jsonschema:"description=与 kind 对应的 JSON 对象字符串"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			payload := []byte(in.PayloadJSON)
			if len(payload) == 0 {
				return "", fmt.Errorf("payload_json is required")
			}
			if in.Kind == store.ResearchArtifactChapterReview {
				verified, verifyErr := verifyChapterReviewPayload(ctx, db, stg, projectID, payload, reviewReads)
				if verifyErr != nil {
					return "", verifyErr
				}
				payload = verified
			}
			sum := md5.Sum(payload)
			clientID := runID.String() + ":" + hex.EncodeToString(sum[:])
			out, err := researchSvc.Create(ctx, ownerID, projectID, research.CreateInput{
				Kind: in.Kind, Payload: payload, Origin: "agent", RunID: &runID, ClientRequestID: &clientID,
			})
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(out)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	addResearchRevision, err := utils.InferTool(
		"add_research_artifact_revision",
		"为已有组织产物生成新的候选修订；替换已采用计划时必须先展示变化并由用户确认，expected_revision 传当前修订号。",
		func(ctx context.Context, in struct {
			ArtifactID       string `json:"artifact_id"`
			ExpectedRevision int    `json:"expected_revision"`
			PayloadJSON      string `json:"payload_json"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			id, err := uuid.Parse(in.ArtifactID)
			if err != nil {
				return "", fmt.Errorf("invalid artifact_id")
			}
			payload := []byte(in.PayloadJSON)
			var existing store.ResearchArtifact
			if err := db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).First(&existing).Error; err != nil {
				return "", fmt.Errorf("artifact not found")
			}
			if existing.Kind == store.ResearchArtifactChapterReview {
				verified, verifyErr := verifyChapterReviewPayload(ctx, db, stg, projectID, payload, reviewReads)
				if verifyErr != nil {
					return "", verifyErr
				}
				payload = verified
			}
			out, err := researchSvc.AddRevision(ctx, ownerID, projectID, store.UUID{UUID: id}, research.RevisionInput{
				ExpectedRevision: in.ExpectedRevision, Payload: payload, Origin: "agent", RunID: &runID,
			})
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(out)
			return string(encoded), err
		})
	if err != nil {
		return nil, err
	}

	reviewContext, err := chapterReviewContextTool(db, stg, normsProvider, projectID, ownerID, verify, reviewReads)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{
		listDocuments, readDocument, readSource, listLatexFiles, readLatexFile,
		proposeEdit, compileProject, getWritingState, saveWritingBrief,
		listResearch, readResearch, createResearch, addResearchRevision, reviewContext,
	}, nil
}

// SearchSubAgent 构建检索子 agent（检索专家）：持 rag_search（hybrid 检索）与
// wiki_read/wiki_write，多跳检索（改写查询、读块、沉淀 wiki）后给出带出处的结论。
//
// 它作为 `deep.Config.SubAgents` 的一项交给主 agent：DeepAgent 会自动生成一个
// `task` 工具（参数 subagent_type / description）来做编排，因此这里返回 agent 本身，
// 不再手工包 AgentTool。用户视角仍只有一个 Assistant（参考架构 §9.1）。
func SearchSubAgent(ctx context.Context, db *gorm.DB, ragStore *rag.Store,
	projectID, ownerID, rootRunID store.UUID, chatModel model.ToolCallingChatModel,
	tracker *toolFailureTracker, memoryEnabled bool) (adk.Agent, error) {

	verify := func(ctx context.Context) error {
		var n int64
		if err := db.WithContext(ctx).Model(&store.Project{}).
			Where("id = ? AND owner_id = ?", projectID, ownerID).
			Count(&n).Error; err != nil || n == 0 {
			return fmt.Errorf("project not accessible")
		}
		return nil
	}

	ragSearch, err := utils.InferTool(
		"rag_search",
		ragSearchMode(ragStore.Embed)+"项目资料库与 wiki。返回最相关的内容块及出处。可用不同措辞多次调用以扩展召回。",
		func(ctx context.Context, in struct {
			Query string `json:"query" jsonschema:"description=检索词（可中英混合）"`
			K     int    `json:"k,omitempty" jsonschema:"description=返回条数，默认 5"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			hits, err := ragStore.Search(ctx, projectID, in.Query, in.K)
			if err != nil {
				return "", err
			}
			if len(hits) == 0 {
				return "（资料库中无相关内容）", nil
			}
			evidence, err := persistRunEvidence(ctx, db, rootRunID, hits)
			if err != nil {
				return "", err
			}
			evidenceByChunk := make(map[string]store.AgentEvidence, len(evidence))
			for _, row := range evidence {
				if row.ChunkID != nil {
					evidenceByChunk[*row.ChunkID] = row
				}
			}
			var b strings.Builder
			for i, h := range hits {
				row, ok := evidenceByChunk[h.ChunkID]
				if !ok {
					return "", fmt.Errorf("persisted evidence missing for chunk %s", h.ChunkID)
				}
				version := ""
				if row.SourceVersion != nil {
					version = *row.SourceVersion
				}
				fmt.Fprintf(&b, "[%d] evidence_id=%s source=%s source_id=%s source_version=%s chunk_id=%s 名称=%s\n%s\n\n",
					i+1, row.ID.String(), h.SourceType, h.SourceID, version, h.ChunkID, h.SourceName, h.Content)
			}
			return b.String(), nil
		})
	if err != nil {
		return nil, err
	}

	wikiRead, err := utils.InferTool(
		"wiki_read",
		"读取项目知识库（LLM wiki）的一页。先查这里——过往研究结论已沉淀。",
		func(ctx context.Context, in struct {
			Title string `json:"title" jsonschema:"description=页标题"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			page, err := ragStore.GetWiki(ctx, projectID, in.Title)
			if err != nil {
				return fmt.Sprintf("wiki 无 %q 页", in.Title), nil
			}
			return page.Content, nil
		})
	if err != nil {
		return nil, err
	}

	wikiWrite, err := utils.InferTool(
		"wiki_write",
		"把研究结论沉淀为项目知识库一页（供后续检索直达）。检索完成后应把综合发现写入。",
		func(ctx context.Context, in struct {
			Title   string `json:"title" jsonschema:"description=页标题（如：注意力机制综述）"`
			Content string `json:"content" jsonschema:"description=markdown 内容，含出处文件名"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			if _, err := ragStore.UpsertWiki(ctx, projectID, in.Title, in.Content, ""); err != nil {
				return "", err
			}
			return "已沉淀 wiki 页：" + in.Title, nil
		})
	if err != nil {
		return nil, err
	}

	steps := []string{
		"你是文献检索专家。任务：针对给定的研究问题，在项目资料库中检索并综合出带出处的答案。",
		"流程：",
		"1. 先 rag_search 用原始问题检索；",
		"2. 结果不足时改写关键词（同义词/英文术语）再检索；",
	}
	subTools := []tool.BaseTool{ragSearch}
	if memoryEnabled {
		steps = append(steps,
			"3. 相关主题若 wiki_read 已有沉淀则优先引用；",
			"4. 综合多路结果给出结论，每条结论标注 [来源: 文件名]；",
			"5. 结论有沉淀价值时 wiki_write 存为知识页（标题概括主题）。",
		)
		subTools = append(subTools, wikiRead, wikiWrite)
	} else {
		steps = append(steps, "3. 综合多路结果给出结论，每条结论标注 [来源: 文件名]。")
	}
	steps = append(steps, "回答要精炼，先给结论再给出处；找不到就明说，不编造。")

	// 注意：AgentTool 对模型暴露的名字 = 子 agent 的 Name（ADK 约定），
	// 因此这里就叫 search_references
	subAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "search_references",
		Description: ragSearchMode(ragStore.Embed) + "：深度检索项目资料库（文献+wiki），多轮检索并综合出带出处的调研结论。任何涉及文献内容/相关工作的调研问题都应调用。",
		Instruction: strings.Join(steps, "\n"),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               subTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{{Invokable: tracker.middleware}},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return subAgent, nil
}

// validateAgentPath 与 latex 域同规则：禁穿越、禁危险扩展。
func validateAgentPath(p string) error {
	if strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
		return fmt.Errorf("path traversal is not allowed")
	}
	lower := strings.ToLower(p)
	for _, ext := range []string{".exe", ".dll", ".so", ".sh", ".bat"} {
		if strings.HasSuffix(lower, ext) {
			return fmt.Errorf("file type not allowed: %s", ext)
		}
	}
	if len(p) == 0 || len(p) > 500 {
		return fmt.Errorf("invalid path length")
	}
	return nil
}

func findLatex(ctx context.Context, db *gorm.DB, stg storage.Storage,
	projectID store.UUID, path string) (*store.LatexFile, string, error) {
	var f store.LatexFile
	if err := db.WithContext(ctx).
		Where("project_id = ? AND file_path = ?", projectID, path).
		First(&f).Error; err != nil {
		return nil, "", fmt.Errorf("file %q not found", path)
	}
	if f.S3Key == nil {
		return &f, "", nil
	}
	content, err := stg.Get(ctx, *f.S3Key)
	if err != nil {
		return nil, "", err
	}
	return &f, string(content), nil
}

// ragSearchMode 如实描述当前检索能力：未配置嵌入模型时只有词袋降级，不是语义检索。
func ragSearchMode(embed rag.Embedder) string {
	if model, _ := embed.Describe(); model == "hash" {
		return "词袋降级检索（未配置嵌入模型，不具语义理解）"
	}
	return "混合检索（向量+关键词）"
}
