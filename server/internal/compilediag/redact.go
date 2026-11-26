package compilediag

import (
	"regexp"
	"strings"
)

// 脱敏是白名单而非黑名单：只保留「TeX 诊断行」，其余（源码回显、
// (./chapters/x.tex) 文件展开、\argument 展开内容）一律丢弃。
// 客户端先跑同一套规则保证正文不出浏览器，服务端再强制跑一遍（不信任客户端）。

const (
	maxLogExcerptBytes   = 32 << 10
	maxErrorMessageRunes = 500
)

var (
	// ANSI CSI（颜色/光标控制）与除 \n \t 外的控制字符。
	ansiCSI      = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")
	controlChars = regexp.MustCompile("[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")
	// l.42 \secret：只留行号，源码回显必须丢弃。
	lineNumber = regexp.MustCompile(`^l\.\d+`)
	// Package <名> Error / Package <名> Warning。
	packageDiag = regexp.MustCompile(`^Package \S+ (Error|Warning)\b`)
	// This is <引擎> ...：引擎横幅，用于确认实际引擎与版本。
	engineBanner = regexp.MustCompile(`^This is (pdfTeX|XeTeX|LuaTeX|e-TeX)`)
)

// diagnosticPrefixes 是逐行白名单前缀（行首 trim 后匹配）。
var diagnosticPrefixes = []string{
	"!",
	"LaTeX Error",
	"LaTeX Warning",
	"LaTeX Font Warning",
	"Overfull",
	"Underfull",
	"Emergency stop",
	"Fatal error",
	"Runaway argument",
	"Output written",
	"No pages of output",
}

// RedactLog 返回只含诊断行的日志片段，按行边界截断到 32 KiB。
func RedactLog(raw string) string {
	if raw == "" {
		return ""
	}
	cleaned := controlChars.ReplaceAllString(ansiCSI.ReplaceAllString(raw, ""), "")
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")

	var b strings.Builder
	for _, line := range strings.Split(cleaned, "\n") {
		if b.Len() >= maxLogExcerptBytes {
			break
		}
		out, ok := redactLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		if b.Len()+len(out)+1 > maxLogExcerptBytes {
			break
		}
		b.WriteString(out)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func redactLine(line string) (string, bool) {
	if line == "" {
		return "", false
	}
	if m := lineNumber.FindString(line); m != "" {
		return m, true
	}
	for _, prefix := range diagnosticPrefixes {
		if strings.HasPrefix(line, prefix) {
			return line, true
		}
	}
	if packageDiag.MatchString(line) || engineBanner.MatchString(line) {
		return line, true
	}
	return "", false
}

// RedactErrorMessage 生成不超过 500 字符的错误摘要：latex 类优先首条 "!" 行，
// 否则用调用方给的摘要（客户端已是中文摘要）。
func RedactErrorMessage(kind, message string) string {
	if kind == "latex" {
		for _, line := range strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "!") {
				message = line
				break
			}
		}
	}
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > maxErrorMessageRunes {
		runes = runes[:maxErrorMessageRunes]
	}
	return string(runes)
}
