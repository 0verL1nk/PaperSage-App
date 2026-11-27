package latex

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// 项目结构解析：只回答「文件之间是怎么组织的」，不判断章节写得好不好。
// 语义结论一律由真实模型给出；这里不做任何关键词/正则式的写作评价。

const (
	// StructureMaxTexFiles 是参与结构分析的 .tex 文件上限。
	// 超过时返回 TooManyFiles，由调用方要求用户缩小范围，不静默截断。
	StructureMaxTexFiles = 40
	// StructureMaxUnits 是一次检查允许的章节单元上限。
	StructureMaxUnits = 12
)

var (
	documentClassRe = regexp.MustCompile(`\\documentclass(?:\[[^\]]*\])?\s*\{[^}]+\}`)
	includeRe       = regexp.MustCompile(`\\(?:input|include|subfile)\s*\{([^}]+)\}`)
	bibResourceRe   = regexp.MustCompile(`\\(?:bibliography|addbibresource)\s*\{([^}]+)\}`)
	chapterRe       = regexp.MustCompile(`\\chapter\*?\s*\{([^}]*)\}`)
	sectionRe       = regexp.MustCompile(`\\section\*?\s*\{([^}]*)\}`)
	beginDocRe      = regexp.MustCompile(`\\begin\s*\{document\}`)
)

// ProjectText 是参与结构分析的一个文本文件（.tex / .bib）。
type ProjectText struct {
	Path    string
	Content string
}

// StructureRoot 是含 \documentclass 的主文件候选。
type StructureRoot struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	IsDefault bool   `json:"isDefault"`
}

// StructureUnit 是一个可检查的章节单元（按主文件包含顺序）。
type StructureUnit struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	FilePaths []string `json:"filePaths"`
	Order     int      `json:"order"`
	SizeBytes int      `json:"sizeBytes"`
	Empty     bool     `json:"empty"`
}

// StructureFile 是项目文本文件的结构视角。
type StructureFile struct {
	Path       string `json:"path"`
	SizeBytes  int    `json:"sizeBytes"`
	Kind       string `json:"kind"` // tex | bib
	Referenced bool   `json:"referenced"`
	UnitID     string `json:"unitId,omitempty"`
}

// StructureWarning 是结构层面的确定性提示（缺文件、环、多主文件等）。
type StructureWarning struct {
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
}

// Structure 是一次结构解析的完整结果。
type Structure struct {
	Roots        []StructureRoot    `json:"roots"`
	Units        []StructureUnit    `json:"units"`
	Files        []StructureFile    `json:"files"`
	Warnings     []StructureWarning `json:"warnings"`
	TooManyFiles bool               `json:"tooManyFiles"`
	TooManyUnits bool               `json:"tooManyUnits"`
}

// StripLatexComments 去掉行内注释（% 未被奇数个反斜杠转义）。
// 前端 preflight 使用相同规则，避免「注释里的命令被当成真的」。
func StripLatexComments(source string) string {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		for j := 0; j < len(line); j++ {
			if line[j] != '%' {
				continue
			}
			backslashes := 0
			for k := j - 1; k >= 0 && line[k] == '\\'; k-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				lines[i] = line[:j]
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// normalizeTexPath 归一化 LaTeX 包含路径：反斜杠转正斜杠、去 ./、补 .tex。
func normalizeTexPath(raw string) string {
	p := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimLeft(p, "/")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	if path.Ext(p) == "" {
		p += ".tex"
	}
	return p
}

// NormalizeTexPath 暴露同一套归一化规则，供计划 filePath 与项目文件路径对齐。
func NormalizeTexPath(raw string) string { return normalizeTexPath(raw) }

// ParseStructure 解析项目文件的主入口、章节单元与包含关系。
// 单元顺序始终由主文件的包含顺序决定；没有主文件时按路径排序。
func ParseStructure(files []ProjectText) Structure {
	out := Structure{Roots: []StructureRoot{}, Units: []StructureUnit{}, Files: []StructureFile{}, Warnings: []StructureWarning{}}
	byPath := make(map[string]string, len(files))
	texOrder := make([]string, 0, len(files))
	bibs := make([]string, 0)
	for _, f := range files {
		p := normalizeTexPath(f.Path)
		if p == "" {
			continue
		}
		byPath[p] = f.Content
		if strings.EqualFold(path.Ext(p), ".bib") {
			bibs = append(bibs, p)
		} else {
			texOrder = append(texOrder, p)
		}
	}
	sort.Strings(texOrder)
	sort.Strings(bibs)

	// 主文件候选：任何含 \documentclass 的 .tex，不要求文件名叫 main.tex。
	for _, p := range texOrder {
		if documentClassRe.MatchString(StripLatexComments(byPath[p])) {
			out.Roots = append(out.Roots, StructureRoot{Path: p, Title: unitTitle(p, byPath[p])})
		}
	}
	defaultRoot := ""
	for _, r := range out.Roots {
		if beginDocRe.MatchString(StripLatexComments(byPath[r.Path])) {
			defaultRoot = r.Path
			break
		}
	}
	if defaultRoot == "" && len(out.Roots) > 0 {
		defaultRoot = out.Roots[0].Path
	}
	for i := range out.Roots {
		out.Roots[i].IsDefault = out.Roots[i].Path == defaultRoot
	}
	switch {
	case len(out.Roots) == 0:
		out.Warnings = append(out.Warnings, StructureWarning{
			Code: "no_root", Detail: "没有找到包含 \\documentclass 的主文件；将按顶层章节文件逐个检查。",
		})
	case len(out.Roots) > 1:
		out.Warnings = append(out.Warnings, StructureWarning{
			Code: "multiple_roots", Detail: fmt.Sprintf("发现 %d 个主文件候选，默认使用 %s。", len(out.Roots), defaultRoot),
		})
	}

	included := map[string]bool{}
	unitOf := map[string]string{}
	visited := map[string]bool{}
	missing := map[string]bool{}

	// 先记录所有文件声明的包含关系：既用于判断「文件是否被引用」，
	// 也让没有主文件的项目能识别出顶层章节。
	for _, p := range texOrder {
		for _, m := range includeRe.FindAllStringSubmatch(StripLatexComments(byPath[p]), -1) {
			target := normalizeTexPath(m[1])
			if target == "" {
				continue
			}
			included[target] = true
			if _, exists := byPath[target]; !exists && !missing[target] {
				missing[target] = true
				out.Warnings = append(out.Warnings, StructureWarning{
					Code: "missing_include", Path: p, Detail: fmt.Sprintf("%s 引用的文件 %s 不存在。", p, target),
				})
			}
		}
	}

	var walk func(p string, top bool)
	walk = func(p string, top bool) {
		if visited[p] {
			return
		}
		visited[p] = true
		content, ok := byPath[p]
		if !ok {
			return
		}
		for _, m := range includeRe.FindAllStringSubmatch(StripLatexComments(content), -1) {
			target := normalizeTexPath(m[1])
			if target == "" {
				continue
			}
			included[target] = true
			if _, exists := byPath[target]; !exists {
				continue
			}
			if top {
				id := fmt.Sprintf("u%d", len(out.Units)+1)
				unitOf[target] = id
				out.Units = append(out.Units, StructureUnit{
					ID: id, Title: unitTitle(target, byPath[target]), FilePaths: []string{target},
					Order: len(out.Units) + 1, SizeBytes: len(byPath[target]), Empty: strings.TrimSpace(byPath[target]) == "",
				})
			}
			if visited[target] {
				out.Warnings = append(out.Warnings, StructureWarning{
					Code: "include_cycle", Path: target, Detail: fmt.Sprintf("%s 被重复包含（可能存在循环引用）。", target),
				})
				continue
			}
			walk(target, false)
		}
	}

	switch {
	case defaultRoot != "":
		walk(defaultRoot, true)
		if len(out.Units) == 0 && strings.TrimSpace(byPath[defaultRoot]) != "" {
			// 单文件论文：主文件本身就是唯一检查单元。
			out.Units = append(out.Units, StructureUnit{
				ID: "u1", Title: unitTitle(defaultRoot, byPath[defaultRoot]), FilePaths: []string{defaultRoot},
				Order: 1, SizeBytes: len(byPath[defaultRoot]),
			})
			unitOf[defaultRoot] = "u1"
		}
	default:
		// 没有主文件：取没有被其他文件包含的顶层 .tex 作为单元。
		for _, p := range texOrder {
			if included[p] {
				continue
			}
			id := fmt.Sprintf("u%d", len(out.Units)+1)
			unitOf[p] = id
			out.Units = append(out.Units, StructureUnit{
				ID: id, Title: unitTitle(p, byPath[p]), FilePaths: []string{p},
				Order: len(out.Units) + 1, SizeBytes: len(byPath[p]), Empty: strings.TrimSpace(byPath[p]) == "",
			})
		}
	}

	// .bib 是否被声明引用（\bibliography / \addbibresource）。
	bibReferenced := map[string]bool{}
	for _, p := range texOrder {
		for _, m := range bibResourceRe.FindAllStringSubmatch(StripLatexComments(byPath[p]), -1) {
			for _, raw := range strings.Split(m[1], ",") {
				name := strings.TrimSpace(raw)
				if name == "" {
					continue
				}
				if path.Ext(name) == "" {
					name += ".bib"
				}
				bibReferenced[normalizeTexPath(name)] = true
			}
		}
	}

	for _, p := range texOrder {
		referenced := p == defaultRoot || included[p]
		if !referenced && defaultRoot != "" {
			out.Warnings = append(out.Warnings, StructureWarning{
				Code: "unreferenced_file", Path: p, Detail: fmt.Sprintf("%s 没有被主文件包含，检查时不会被自动纳入。", p),
			})
		}
		out.Files = append(out.Files, StructureFile{
			Path: p, SizeBytes: len(byPath[p]), Kind: "tex", Referenced: referenced, UnitID: unitOf[p],
		})
	}
	for _, p := range bibs {
		out.Files = append(out.Files, StructureFile{
			Path: p, SizeBytes: len(byPath[p]), Kind: "bib", Referenced: bibReferenced[p],
		})
	}

	out.TooManyFiles = len(texOrder) > StructureMaxTexFiles
	out.TooManyUnits = len(out.Units) > StructureMaxUnits
	if out.TooManyFiles {
		out.Warnings = append(out.Warnings, StructureWarning{
			Code: "too_many_files", Detail: fmt.Sprintf("项目包含 %d 个 .tex 文件，超过单次检查上限 %d 个；请拆分项目或缩小检查范围。", len(texOrder), StructureMaxTexFiles),
		})
	}
	if out.TooManyUnits {
		out.Warnings = append(out.Warnings, StructureWarning{
			Code: "too_many_units", Detail: fmt.Sprintf("识别到 %d 个章节单元，超过单次检查上限 %d 个；请缩小检查范围。", len(out.Units), StructureMaxUnits),
		})
	}
	return out
}

// unitTitle 从文件内容提取章节标题；没有章节命令时退回文件名。
func unitTitle(p, content string) string {
	clean := StripLatexComments(content)
	for _, re := range []*regexp.Regexp{chapterRe, sectionRe} {
		if m := re.FindStringSubmatch(clean); len(m) == 2 {
			if title := strings.TrimSpace(m[1]); title != "" {
				return truncateRunes(title, 80)
			}
		}
	}
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
