package agent

import (
	"strings"
	"testing"
)

// #80：任何改稿请求都必须走 propose_edit 出建议卡，不能只在回复里贴代码。
func TestSystemPromptMandatesProposeEditForManuscriptChanges(t *testing.T) {
	for _, want := range []string{"必须调用 propose_edit", "给个建议", "润色", "代码块"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("systemPrompt 缺少强制建议卡规则片段 %q", want)
		}
	}
}

// #79：编译工具只发起浏览器编译请求，提示不得暗示模型能拿到编译结果。
func TestSystemPromptDoesNotClaimCompileResult(t *testing.T) {
	if strings.Contains(systemPrompt, "修改后建议 compile_project 验证") {
		t.Fatal("systemPrompt 不应把 compile_project 描述成可直接验证编译")
	}
	if !strings.Contains(systemPrompt, "不返回编译结果") {
		t.Fatal("systemPrompt 应说明 compile_project 不返回编译结果")
	}
}
