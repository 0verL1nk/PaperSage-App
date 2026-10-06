package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/extract"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/rag"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// ProjectTools 构建 project 作用域的工具集（agentic 核心：读文献、
// 读写 LaTeX、触发编译）。每次请求构建，闭包携带归属校验。
func ProjectTools(db *gorm.DB, stg storage.Storage, queue *latex.Queue,
	projectID, ownerID, runID store.UUID) ([]tool.BaseTool, error) {

	verify := func(ctx context.Context) error {
		var n int64
		if err := db.WithContext(ctx).Model(&store.Project{}).
			Where("id = ? AND owner_id = ?", projectID, ownerID).
			Count(&n).Error; err != nil || n == 0 {
			return fmt.Errorf("project not accessible")
		}
		return nil
	}

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
				Where("project_id = ?", projectID).
				Order("created_at DESC").Limit(100).Find(&docs).Error; err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 篇：\n", len(docs))
			for _, d := range docs {
				remark := ""
				if d.Remark != nil {
					remark = *d.Remark
				}
				fmt.Fprintf(&b, "- %s (%s, %d bytes) %s\n", d.Name, d.Type, d.Size, remark)
			}
			return b.String(), nil
		})
	if err != nil {
		return nil, err
	}

	readDocument, err := utils.InferTool(
		"read_document",
		"读取一篇参考文献的正文文本（支持 PDF/DOCX/TXT 全量抽取）。",
		func(ctx context.Context, in struct {
			Name string `json:"name" jsonschema:"description=文献文件名"`
		}) (string, error) {
			if err := verify(ctx); err != nil {
				return "", err
			}
			var doc store.Document
			if err := db.WithContext(ctx).
				Where("project_id = ? AND name = ?", projectID, in.Name).
				First(&doc).Error; err != nil {
				return "", fmt.Errorf("document %q not found", in.Name)
			}
			content, err := stg.Get(ctx, doc.S3Key)
			if err != nil {
				return "", err
			}
			return extract.Text(ctx, content, doc.Name, doc.Type, 48*1024)
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
		"提出对某个 LaTeX 源文件的修改建议（不直接改稿，用户确认后才应用）。需给出目标文件路径、要替换的原文片段、替换后的新文与理由。old_text 必须与文件当前内容逐字一致，否则会被拒绝——不确定时先 read_latex_file。",
		func(ctx context.Context, in struct {
			Path    string `json:"path" jsonschema:"description=文件路径，如 main.tex"`
			OldText string `json:"old_text" jsonschema:"description=要被替换的原文片段（必须与文件当前内容逐字一致）"`
			NewText string `json:"new_text" jsonschema:"description=替换后的新文"`
			Reason  string `json:"reason" jsonschema:"description=修改理由"`
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
			p := store.AgentChangeProposal{
				ID: store.MustNewUUID(), ProjectID: projectID, RunID: &runID,
				FilePath: in.Path, BaseContentHash: base,
				OldText: in.OldText, NewText: in.NewText, Status: "pending",
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
		"触发项目 LaTeX 编译（异步任务）。修改论文后用来验证能否编译通过。",
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
			job, err := queue.Submit(ctx, projectID.UUID, ownerID.UUID, root, "pdflatex")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("编译任务已提交（job id: %s），用户可在编译面板查看进度。", job.ID.String()), nil
		})
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{
		listDocuments, readDocument, listLatexFiles, readLatexFile,
		proposeEdit, compileProject,
	}, nil
}

// SearchSubAgent 构建检索子 agent（检索专家）：持 rag_search（hybrid 检索）与
// wiki_read/wiki_write，多跳检索（改写查询、读块、沉淀 wiki）后给出带出处的结论。
//
// 它作为 `deep.Config.SubAgents` 的一项交给主 agent：DeepAgent 会自动生成一个
// `task` 工具（参数 subagent_type / description）来做编排，因此这里返回 agent 本身，
// 不再手工包 AgentTool。用户视角仍只有一个 Assistant（参考架构 §9.1）。
func SearchSubAgent(ctx context.Context, db *gorm.DB, ragStore *rag.Store,
	projectID, ownerID store.UUID, chatModel model.ToolCallingChatModel) (adk.Agent, error) {

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
		"混合检索（向量+关键词）项目资料库与 wiki。返回最相关的内容块及出处。可用不同措辞多次调用以扩展召回。",
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
			var b strings.Builder
			for i, h := range hits {
				fmt.Fprintf(&b, "[%d] 来源=%s 名称=%s\n%s\n\n", i+1, h.SourceType, h.SourceName, h.Content)
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

	// 注意：AgentTool 对模型暴露的名字 = 子 agent 的 Name（ADK 约定），
	// 因此这里就叫 search_references
	subAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "search_references",
		Description: "深度检索项目资料库（文献+wiki）：多轮混合检索并综合出带出处的调研结论。任何涉及文献内容/相关工作的调研问题都应调用。",
		Instruction: strings.Join([]string{
			"你是文献检索专家。任务：针对给定的研究问题，在项目资料库中检索并综合出带出处的答案。",
			"流程：",
			"1. 先 rag_search 用原始问题检索；",
			"2. 结果不足时改写关键词（同义词/英文术语）再检索；",
			"3. 相关主题若 wiki_read 已有沉淀则优先引用；",
			"4. 综合多路结果给出结论，每条结论标注 [来源: 文件名]；",
			"5. 结论有沉淀价值时 wiki_write 存为知识页（标题概括主题）。",
			// 压缩靠结构而非格式约定：父 agent 只收到本子 agent 的最终回答，
			// 多轮检索过程留在子 Run 的 parts/events 里供审计，父上下文不会被撑满。
			// 因此这里不规定固定输出模板——那等于把父 agent 的消费方式写死进
			// 子 agent 的提示词，换个父 agent 或换个问题就得重写。
			"回答要精炼，先给结论再给出处；找不到就明说，不编造。",
		}, "\n"),
		Model: chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{ragSearch, wikiRead, wikiWrite},
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

func ptr[T any](v T) *T { return &v }
