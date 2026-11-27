package review

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
)

// 单元报告的服务端校验：只做确定性核对（范围、逐字引文、行号、依据是否读过、
// 规范版本、Claim–Evidence 硬约束）。语义判断已由模型完成，这里不生成结论。

type unitReportPayload struct {
	Dimensions []research.ReviewDimension `json:"dimensions"`
	Findings   []research.ReviewFinding   `json:"findings"`
}

type unitResult struct {
	Dimensions  []research.ReviewDimension `json:"dimensions"`
	Findings    []research.ReviewFinding   `json:"findings"`
	SourcesRead []research.ReviewSource    `json:"sourcesRead"`
}

var reviewStatuses = map[string]bool{"checked": true, "findings": true, "insufficient": true, "not_checked": true, "failed": true}

// validateUnitReport 校验并收敛一个单元的结果；任何一条不合规都退回给模型修正。
func (s *Service) validateUnitReport(ctx context.Context, pc *projectContext, unit *plannedUnit, payload []byte, sources []agent.ReviewSource) (*unitResult, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields() // 拒绝 verdict/score 等未定义字段，防止契约外的「通过判定」。
	var report unitReportPayload
	if err := dec.Decode(&report); err != nil {
		return nil, fmt.Errorf("报告结构不合法：%v", err)
	}
	if len(unit.Dimensions) != 1 {
		return nil, fmt.Errorf("检查单元配置错误")
	}
	dimKey := unit.Dimensions[0]
	if len(report.Dimensions) != 1 || report.Dimensions[0].Key != dimKey {
		return nil, fmt.Errorf("报告必须且只能包含本单元维度 %s", dimKey)
	}
	dim := report.Dimensions[0]
	if !reviewStatuses[dim.Status] {
		return nil, fmt.Errorf("维度状态不合法")
	}
	if (dim.Status == "insufficient" || dim.Status == "not_checked" || dim.Status == "failed") && strings.TrimSpace(dim.Note) == "" {
		return nil, fmt.Errorf("未覆盖或证据不足的维度必须写明原因")
	}
	if unit.Truncated && dimKey == "evidence_support" && (dim.Status == "checked" || dim.Status == "findings") {
		return nil, fmt.Errorf("本章节内容被截断，证据支撑维度不能标记为已完成；请标为证据不足并说明")
	}
	if dimKey == "goal_conclusion" && pc.plan == nil && (dim.Status == "checked" || dim.Status == "findings") {
		return nil, fmt.Errorf("项目没有可用章节计划，目标—结论维度只能标记为证据不足")
	}
	if dimKey == "norm_compliance" && (!pc.normsOK || len(pc.norms) == 0) && (dim.Status == "checked" || dim.Status == "findings") {
		return nil, fmt.Errorf("没有可用的已采用规范，规范符合度只能标记为证据不足或未检查")
	}
	if len(report.Findings) > maxFindingsPerUnit {
		return nil, fmt.Errorf("单个检查单元的问题数量超过上限 %d 条", maxFindingsPerUnit)
	}
	switch dim.Status {
	case "findings":
		if len(report.Findings) == 0 {
			return nil, fmt.Errorf("维度标记为发现问题时必须至少给出一条问题")
		}
	case "checked":
		if len(report.Findings) > 0 {
			return nil, fmt.Errorf("维度标记为已检查时不能包含问题")
		}
	}

	sectionByID := map[string]research.ChapterSection{}
	if pc.plan != nil && pc.plan.Plan != nil {
		for _, sec := range pc.plan.Plan.Sections {
			sectionByID[sec.ID] = sec
		}
	}
	unitFiles := map[string]bool{}
	for _, f := range unit.FilePaths {
		unitFiles[f] = true
	}
	unitSections := map[string]bool{}
	for _, id := range unit.SectionIDs {
		unitSections[id] = true
	}

	findingIDs := map[string]bool{}
	out := &unitResult{Dimensions: []research.ReviewDimension{dim}, Findings: make([]research.ReviewFinding, 0, len(report.Findings))}
	documentRead := false
	for _, src := range sources {
		if src.Kind == "document" {
			documentRead = true
		}
	}
	for _, f := range report.Findings {
		if strings.TrimSpace(f.ID) == "" || findingIDs[f.ID] {
			return nil, fmt.Errorf("问题必须包含唯一 ID")
		}
		findingIDs[f.ID] = true
		if f.Dimension != dimKey {
			return nil, fmt.Errorf("问题 %s 的维度超出本单元范围", f.ID)
		}
		if f.Severity != "high" && f.Severity != "medium" && f.Severity != "low" {
			return nil, fmt.Errorf("问题 %s 的严重程度不合法", f.ID)
		}
		if strings.TrimSpace(f.Problem) == "" || strings.TrimSpace(f.NextStep) == "" {
			return nil, fmt.Errorf("问题 %s 必须写清问题与下一步", f.ID)
		}
		if f.Confidence != "" && f.Confidence != "high" && f.Confidence != "low" {
			return nil, fmt.Errorf("问题 %s 的置信度不合法", f.ID)
		}
		f.FilePath = latex.NormalizeTexPath(f.FilePath)
		if !unitFiles[f.FilePath] {
			return nil, fmt.Errorf("问题 %s 定位的文件 %s 不在本单元范围内", f.ID, f.FilePath)
		}
		if f.SectionID != "" {
			if !unitSections[f.SectionID] {
				return nil, fmt.Errorf("问题 %s 指向的章节不在本单元范围内", f.ID)
			}
			if sec, ok := sectionByID[f.SectionID]; ok && strings.TrimSpace(f.SectionTitle) == "" {
				f.SectionTitle = sec.Title
			}
		}
		content := pc.fileByPath[f.FilePath]
		if strings.TrimSpace(f.Quote) != "" {
			start, end, ok := agent.LocateQuote(content, f.Quote)
			if !ok {
				return nil, fmt.Errorf("问题 %s 的引文在 %s 中找不到；请逐字引用原文，不要改写或拼接", f.ID, f.FilePath)
			}
			f.LineStart, f.LineEnd = start, end
		} else if dimKey != "norm_compliance" {
			return nil, fmt.Errorf("问题 %s 必须给出 %s 中的逐字原文", f.ID, f.FilePath)
		}
		if err := validateUnitBasis(f, pc, unitFiles, unitSections, sectionByID, sources); err != nil {
			return nil, err
		}
		if err := validateUnitClaim(f, documentRead, len(pc.documents)); err != nil {
			return nil, err
		}
		f.AnchorVerified = true
		f.Fingerprint = findingFingerprint(f)
		out.Findings = append(out.Findings, f)
	}
	out.SourcesRead = make([]research.ReviewSource, 0, len(sources))
	for _, src := range sources {
		out.SourcesRead = append(out.SourcesRead, research.ReviewSource{Kind: src.Kind, Name: src.Name})
	}
	return out, nil
}

func validateUnitBasis(f research.ReviewFinding, pc *projectContext, unitFiles, unitSections map[string]bool,
	sectionByID map[string]research.ChapterSection, sources []agent.ReviewSource) error {

	for i := range f.Basis {
		b := &f.Basis[i]
		switch b.Kind {
		case "text":
			path := latex.NormalizeTexPath(firstNonEmpty(b.FilePath, b.Ref))
			if !unitFiles[path] {
				return fmt.Errorf("问题 %s 的文字依据 %s 不在本单元范围内", f.ID, path)
			}
			content, ok := pc.fileByPath[path]
			if !ok {
				return fmt.Errorf("问题 %s 的文字依据文件 %s 不存在", f.ID, path)
			}
			if strings.TrimSpace(b.Quote) == "" {
				return fmt.Errorf("问题 %s 的文字依据必须给出原文摘录", f.ID)
			}
			start, end, ok := agent.LocateQuote(content, b.Quote)
			if !ok {
				return fmt.Errorf("问题 %s 的文字依据在 %s 中找不到；依据必须逐字来自原文", f.ID, path)
			}
			b.FilePath, b.Ref = path, path
			b.LineStart, b.LineEnd = start, end
		case "plan":
			if !unitSections[b.Ref] {
				return fmt.Errorf("问题 %s 的计划依据不在本单元范围内", f.ID)
			}
			sec, ok := sectionByID[b.Ref]
			if !ok {
				return fmt.Errorf("问题 %s 的计划依据指向不存在的章节", f.ID)
			}
			if _, _, found := agent.LocateQuote(research.PlanSectionText(sec), b.Quote); !found {
				return fmt.Errorf("问题 %s 的计划依据在章节计划中找不到；依据必须逐字来自计划", f.ID)
			}
		case "norm":
			matched := false
			for _, entry := range pc.norms {
				if b.Ref == entry.ID || b.Ref == entry.Cite() || b.Ref == entry.Title {
					b.Ref = entry.ID
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("问题 %s 的规范依据不在本次已采用规范中", f.ID)
			}
		case "source":
			if strings.TrimSpace(b.Quote) == "" {
				return fmt.Errorf("问题 %s 的资料依据必须给出原文摘录", f.ID)
			}
			name := b.Ref
			for _, doc := range pc.documents {
				if doc.ID.String() == b.Ref {
					name = doc.Name
				}
			}
			found := false
			for _, src := range sources {
				if src.Kind == "document" && src.Name == name {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("问题 %s 引用的资料 %s 尚未在本轮检查中读取", f.ID, name)
			}
			b.Ref = name
		default:
			return fmt.Errorf("问题 %s 的依据类型不合法", f.ID)
		}
	}
	return nil
}

func validateUnitClaim(f research.ReviewFinding, documentRead bool, documentCount int) error {
	switch f.Dimension {
	case "evidence_support":
		if f.ClaimStatus != "overclaimed" && f.ClaimStatus != "orphaned" {
			return fmt.Errorf("问题 %s 必须标注过度断言（overclaimed）或无据孤立（orphaned）", f.ID)
		}
		if f.ClaimStatus == "overclaimed" && len(f.Basis) == 0 {
			return fmt.Errorf("问题 %s 是过度断言，必须给出对照依据", f.ID)
		}
		if f.ClaimStatus == "orphaned" && documentCount > 0 && !documentRead {
			return fmt.Errorf("问题 %s 声称无据，但项目有资料且本轮未读取；请先用 read_document 核对资料", f.ID)
		}
	case "cross_section":
		files := map[string]bool{}
		for _, b := range f.Basis {
			if b.Kind == "text" {
				files[firstNonEmpty(b.FilePath, b.Ref)] = true
			}
		}
		if len(files) < 2 {
			return fmt.Errorf("问题 %s 是跨章矛盾，必须给出冲突双方的原文依据", f.ID)
		}
	}
	return nil
}

// findingFingerprint 由服务端计算：维度 + 文件 + 归一化引文 + 归一化问题描述。
func findingFingerprint(f research.ReviewFinding) string {
	h := sha256.Sum256([]byte(strings.Join([]string{
		f.Dimension, f.FilePath, normalizeSpace(f.Quote), normalizeSpace(f.Problem),
	}, "\x00")))
	return "sha256:" + hex.EncodeToString(h[:])
}

func normalizeSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
