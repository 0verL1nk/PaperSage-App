package store

// 项目场景是检查维度、提示词与界面文案的共同依据，常量与中文标签只在这里定义一次。
const (
	ScenarioUndergraduateThesis = "undergraduate_thesis"
	ScenarioIntroPaper          = "intro_paper"
)

// ScenarioLabel 返回面向用户的中文场景名；未知场景原样返回，便于排查。
func ScenarioLabel(s string) string {
	switch s {
	case ScenarioUndergraduateThesis:
		return "本科毕业设计"
	case ScenarioIntroPaper:
		return "入门级学术小论文"
	default:
		return s
	}
}
