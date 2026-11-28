package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/writingstate"
)

func experimentTableFixture() experimentTableInput {
	return experimentTableInput{
		Headers: []string{"方法", "准确率 (%)", "F1"},
		Rows:    [][]string{{"Baseline", "88.1", "0.842"}, {"Ours", "92.5", "0.881"}},
		Caption: "不同方法在测试集上的性能对比",
		Label:   "tab:results",
		Bold:    []string{"none", "max", "max"},
	}
}

func TestBuildExperimentPromptIncludesThreeSources(t *testing.T) {
	in := experimentPromptInput{
		ScenarioName: "本科毕业论文",
		Brief:        writingstate.Brief{Topic: "基于大模型的论文写作辅助", Goals: []string{"验证写作效率"}},
		NormsText:    "\n本项目已采用的学术规范：\n- [N-001@v1] 图表规范\n  规则：表格需自明\n  出处：GB/T 7714\n",
		NormsCount:   1,
		PlanStatus:   "adopted",
		PlanSections: []research.ChapterSection{{
			ID: "s4", Title: "实验与结果", Responsibility: "报告实验结果",
			Claims: []research.SectionClaim{{Text: "本文方法优于基线", Status: "supported"}},
		}},
		Table:    experimentTableFixture(),
		Language: "zh",
		Focus:    "重点说明提升幅度",
	}

	system, user, warnings := buildExperimentPrompt(in)
	assert.Contains(t, system, "不得编造任何实验数值")
	assert.Contains(t, system, "citation.value 必须与单元格逐字一致")
	for _, want := range []string{
		"[N-001@v1]", "基于大模型的论文写作辅助", "[s4] 实验与结果", "本文方法优于基线",
		"0=方法，1=准确率 (%)", "row 0: Baseline | 88.1 | 0.842", "<experiment_table>", "<user_focus>",
	} {
		assert.Contains(t, user, want)
	}
	assert.Empty(t, warnings)
}

func TestBuildExperimentPromptStatesMissingSources(t *testing.T) {
	in := experimentPromptInput{
		ScenarioName: "学术小论文",
		PlanStatus:   "missing",
		Table:        experimentTableFixture(),
		Language:     "zh",
	}
	_, user, _ := buildExperimentPrompt(in)
	assert.Contains(t, user, "本次没有可用的项目规范")
	assert.Contains(t, user, "没有可用的章节计划")
}

func TestBuildExperimentPromptTruncatesOversizeContext(t *testing.T) {
	section := research.ChapterSection{ID: "s4", Title: "实验与结果"}
	in := experimentPromptInput{
		ScenarioName: "本科毕业论文",
		PlanStatus:   "adopted",
		Section:      &section,
		ChapterText:  strings.Repeat("实验正文", 30000), // 360 KB，远超 prompt 上限
		Table:        experimentTableFixture(),
		Language:     "zh",
	}
	_, user, warnings := buildExperimentPrompt(in)
	assert.LessOrEqual(t, len(user), experimentMaxPromptBytes)
	assert.Contains(t, strings.Join(warnings, "\n"), "截断")
}

func TestVerifyExperimentOutputAcceptsExactCitation(t *testing.T) {
	raw := "```json\n" + `{
		"paragraphs": [{
			"text": "本文方法准确率达到 92.5%，较基线 88.1% 提升 4.4 个百分点。",
			"citations": [
				{"row": 1, "column": 1, "value": "92.5"},
				{"row": 0, "column": 1, "value": "88.1"}
			]
		}],
		"norm_refs": ["N-001@v1"]
	}` + "\n```"

	paragraphs, cites, apiErr := verifyExperimentOutput(raw, experimentTableFixture())
	require.Nil(t, apiErr)
	require.Len(t, paragraphs, 1)
	require.Len(t, paragraphs[0].Citations, 2)
	assert.Equal(t, "Ours", paragraphs[0].Citations[0].RowLabel)
	assert.Equal(t, "准确率 (%)", paragraphs[0].Citations[0].ColumnLabel)
	assert.Equal(t, "88.1", paragraphs[0].Citations[1].Value)
	assert.Equal(t, []string{"N-001@v1"}, cites)
}

func TestVerifyExperimentOutputRejectsFabrication(t *testing.T) {
	table := experimentTableFixture()
	cases := []struct{ name, raw, want string }{
		{"引用值与单元格不一致", `{"paragraphs":[{"text":"准确率 93.1%","citations":[{"row":1,"column":1,"value":"93.1"}]}]}`, "引用值"},
		{"引用单元格越界", `{"paragraphs":[{"text":"准确率 92.5%","citations":[{"row":9,"column":1,"value":"92.5"}]}]}`, "不存在的单元格"},
		{"数字段没有引用", `{"paragraphs":[{"text":"准确率提升 4.4 个百分点"}]}`, "没有标注来源"},
		{"全篇没有任何引用", `{"paragraphs":[{"text":"总体而言本文方法更好。"}]}`, "没有任何实验数值引用"},
		{"输出不是 JSON", `抱歉，我无法完成`, "无法解析"},
		{"段落为空", `{"paragraphs":[]}`, "段落数量"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, apiErr := verifyExperimentOutput(tc.raw, table)
			require.NotNil(t, apiErr)
			assert.Equal(t, 502, apiErr.Code)
			assert.Contains(t, apiErr.Info, tc.want)
		})
	}
}

func TestValidateExperimentRequest(t *testing.T) {
	valid := experimentRequest{Table: experimentTableFixture()}
	require.Nil(t, validateExperimentRequest(&valid))
	assert.Equal(t, "zh", valid.Language, "语言缺省为中文")

	cases := []struct {
		name   string
		mutate func(*experimentRequest)
		want   string
	}{
		{"列数超限", func(r *experimentRequest) {
			r.Table.Headers = make([]string, experimentMaxColumns+1)
			r.Table.Rows = [][]string{make([]string, experimentMaxColumns+1)}
		}, "最多 20 列"},
		{"语言非法", func(r *experimentRequest) { r.Language = "fr" }, "语言"},
		{"补充要求超长", func(r *experimentRequest) { r.Focus = strings.Repeat("要", experimentMaxFocusRunes+1) }, "补充要求"},
		{"行列不一致", func(r *experimentRequest) { r.Table.Rows = [][]string{{"只有一列"}} }, "表头是"},
		{"没有数据行", func(r *experimentRequest) { r.Table.Rows = nil }, "没有数据行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := experimentRequest{Table: experimentTableFixture()}
			tc.mutate(&req)
			apiErr := validateExperimentRequest(&req)
			require.NotNil(t, apiErr)
			assert.Equal(t, 422, apiErr.Code)
			assert.Contains(t, apiErr.Info, tc.want)
		})
	}
}
