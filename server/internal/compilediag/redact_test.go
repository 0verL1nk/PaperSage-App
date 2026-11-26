package compilediag

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 隐私红线：源码回显行（l.NN 后的内容）、文件展开、正文一律不得出现在 excerpt 中。
func TestRedactLogKeepsOnlyDiagnosticLines(t *testing.T) {
	raw := strings.Join([]string{
		"This is XeTeX, Version 3.141592653-2.6-0.999996 (TeX Live 2026)",
		"(./chapters/intro.tex",
		"! Undefined control sequence.",
		"l.42 \\secretCommandWithPaperText",
		"                    ^^",
		"LaTeX Warning: Reference `fig:1' on page 1 undefined on input line 12.",
		"Package hyperref Warning: Token not allowed in a PDF string.",
		"Overfull \\hbox (12.3pt too wide) in paragraph at lines 5--6",
		"我的论文正文这一行绝对不能上传",
		"Output written on main.pdf (3 pages, 12345 bytes).",
	}, "\n")

	got := RedactLog(raw)

	assert.Contains(t, got, "! Undefined control sequence.")
	assert.Contains(t, got, "l.42")
	assert.Contains(t, got, "LaTeX Warning")
	assert.Contains(t, got, "Package hyperref Warning")
	assert.Contains(t, got, "Overfull")
	assert.Contains(t, got, "This is XeTeX")
	assert.Contains(t, got, "Output written")
	assert.NotContains(t, got, "secretCommandWithPaperText")
	assert.NotContains(t, got, "我的论文正文")
	assert.NotContains(t, got, "chapters/intro.tex")
	assert.NotContains(t, got, "^^")
}

func TestRedactLogStripsAnsiAndCapsAt32KiB(t *testing.T) {
	raw := "\x1b[31m! Emergency stop.\x1b[0m\n" + strings.Repeat("! another error line\n", 5000)
	got := RedactLog(raw)
	assert.NotContains(t, got, "\x1b[")
	assert.LessOrEqual(t, len(got), maxLogExcerptBytes)
	assert.True(t, strings.HasPrefix(got, "! Emergency stop."))
}

func TestRedactErrorMessagePrefersFirstBangLine(t *testing.T) {
	got := RedactErrorMessage("latex", "编译失败\n! Undefined control sequence.\nl.42 \\x")
	assert.Equal(t, "! Undefined control sequence.", got)
	assert.Equal(t, "资源加载失败", RedactErrorMessage("resource", "资源加载失败"))
}
