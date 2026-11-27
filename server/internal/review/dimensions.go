package review

import "github.com/0verL1nk/GraduateProject/server/internal/store"

// 检查维度注册表：维度只决定「问模型什么」，不预设任何结论。
// 适用场景留空表示所有场景可用；场景专属维度由服务端按项目场景过滤，错场景直接拒绝。

// Dimension 是一个语义检查维度。
type Dimension struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Scope       string   `json:"scope"` // chapter | collection
	Description string   `json:"description"`
	Scenarios   []string `json:"scenarios,omitempty"`
}

var dimensionRegistry = []Dimension{
	{
		Key: "goal_conclusion", Label: "章节计划对齐", Scope: "chapter",
		Description: "章节计划中的目标、职责与研究问题，是否与正文实际结论一致。",
	},
	{
		Key: "evidence_support", Label: "论断证据支撑", Scope: "chapter",
		Description: "关键论断是否有已读资料或原文证据支撑；无据或过度断言逐条指出。",
	},
	{
		Key: "norm_compliance", Label: "学术规范合规", Scope: "chapter",
		Description: "是否符合作者已采用且锁定版本的学术规范条目。",
	},
	{
		Key: "cross_section", Label: "章节间重复/矛盾", Scope: "collection",
		Description: "不同章节之间的重复表述或相互矛盾（样本量、对象、方法、结论等）。",
	},
	{
		Key: "terminology_consistency", Label: "术语与符号统一", Scope: "collection",
		Description: "同一概念、符号、单位在不同章节的叫法、定义与口径是否一致。",
	},
	{
		Key: "thesis_engineering_closure", Label: "设计—实现—测试—结论闭环", Scope: "collection",
		Description: "本科毕设的设计、实现、测试与结论是否形成闭环。",
		Scenarios:   []string{store.ScenarioUndergraduateThesis},
	},
	{
		Key: "paper_contribution_closure", Label: "贡献—方法—结果—结论一致性", Scope: "collection",
		Description: "学术小论文声明的贡献、采用的方法、实验结果与结论是否一致。",
		Scenarios:   []string{store.ScenarioIntroPaper},
	},
}

// DimensionsFor 返回该场景可用的维度（顺序稳定，前端直接渲染）。
func DimensionsFor(scenario string) []Dimension {
	out := make([]Dimension, 0, len(dimensionRegistry))
	for _, d := range dimensionRegistry {
		if dimensionAllowed(d, scenario) {
			out = append(out, d)
		}
	}
	return out
}

// LookupDimension 按 key 取维度定义；不存在时 ok=false。
func LookupDimension(key string) (Dimension, bool) {
	for _, d := range dimensionRegistry {
		if d.Key == key {
			return d, true
		}
	}
	return Dimension{}, false
}

// DimensionAllowed 判断维度是否适用于该场景（用于请求校验，错场景返回 400）。
func DimensionAllowed(scenario, key string) bool {
	d, ok := LookupDimension(key)
	return ok && dimensionAllowed(d, scenario)
}

func dimensionAllowed(d Dimension, scenario string) bool {
	if len(d.Scenarios) == 0 {
		return true
	}
	for _, s := range d.Scenarios {
		if s == scenario {
			return true
		}
	}
	return false
}
