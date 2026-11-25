package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

// systemPromptFor 在基础提示上注入场景、阶段与研究蓝图摘要。
// 只注入用户已确认的蓝图字段；完整状态仍由 get_writing_state 按需读取。
func (s *Service) systemPromptFor(ctx context.Context, projectID, ownerID store.UUID) string {
	var project store.Project
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ?", projectID, ownerID).First(&project).Error; err != nil {
		return systemPrompt
	}
	state, err := writingstate.NewService(s.DB).GetForAgent(ctx, projectID.UUID, ownerID.UUID)
	if err != nil {
		return systemPrompt
	}
	var b strings.Builder
	b.WriteString(systemPrompt)
	fmt.Fprintf(&b, "\n\n当前项目上下文：\n- 场景：%s\n- 阶段：%s\n", scenarioLabel(project.Scenario), phaseLabel(state.CurrentPhase))
	brief := state.Brief
	if brief.Topic != "" {
		fmt.Fprintf(&b, "- 研究主题：%s\n", brief.Topic)
	}
	if brief.Background != "" {
		fmt.Fprintf(&b, "- 背景：%s\n", brief.Background)
	}
	appendList := func(label string, items []string) {
		if len(items) > 0 {
			fmt.Fprintf(&b, "- %s：%s\n", label, strings.Join(items, "；"))
		}
	}
	appendList("研究问题", brief.Questions)
	appendList("研究目标", brief.Goals)
	appendList("范围外", brief.ScopeOut)
	appendList("关键章节", brief.KeyChapters)
	appendList("约束", brief.Constraints)
	appendList("已知事实", brief.KnownFacts)
	appendList("假设", brief.Assumptions)
	appendList("待验证", brief.Unverified)
	if brief.ObjectsAndData != "" {
		fmt.Fprintf(&b, "- 对象与数据：%s\n", brief.ObjectsAndData)
	}
	if brief.Methods != "" {
		fmt.Fprintf(&b, "- 方法：%s\n", brief.Methods)
	}
	s.appendAdoptedNorms(ctx, &b, projectID)
	return b.String()
}

// appendAdoptedNorms 注入项目已采用的规范（采用时锁定的版本）。
//
// 只提供条款原文、出处与稳定引用号；是否符合规范由模型结合草稿做语义判断，
// 这里不做任何关键词/正则匹配。规范库不可用或未采用时如实说明，
// 让模型没有条件假装「符合某条规范」。
func (s *Service) appendAdoptedNorms(ctx context.Context, b *strings.Builder, projectID store.UUID) {
	const unavailable = "\n学术规范：本次没有可用的项目规范，不要声称遵循或符合任何具体规范条款。\n"
	if s.Norms == nil {
		b.WriteString(unavailable)
		return
	}
	entries, err := s.Norms.AdoptedForProject(ctx, projectID)
	switch {
	case errors.Is(err, norms.ErrUnavailable):
		b.WriteString(unavailable)
	case err != nil:
		slog.Warn("agent: 读取项目已采用规范失败", "project_id", projectID.String(), "err", err)
		b.WriteString(unavailable)
	case len(entries) == 0:
		b.WriteString("\n学术规范：本项目尚未采用任何规范条目；不要声称引用或符合某条规范。\n")
	default:
		b.WriteString("\n本项目已采用的学术规范（给出建议或改动时必须逐条对照；引用时使用方括号里的编号，不得编造未列出的条款）：\n")
		for _, entry := range entries {
			fmt.Fprintf(b, "- [%s] %s\n  规则：%s\n  出处：%s\n", entry.Cite(), entry.Title, entry.Rule, entry.Source)
		}
	}
}

func scenarioLabel(s string) string {
	switch s {
	case "undergraduate_thesis":
		return "本科毕业设计"
	case "intro_paper":
		return "入门级学术小论文"
	default:
		return s
	}
}

func phaseLabel(p string) string {
	switch p {
	case "prepare":
		return "写作准备"
	case "write":
		return "内容撰写"
	case "review":
		return "检查完善"
	default:
		return p
	}
}
