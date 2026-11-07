package agent

import (
	"context"
	"fmt"
	"strings"

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
	return b.String()
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
