package rag

import "strings"

// tokenize：检索分词。英文按词、中文按 bigram（简易但有效的
// CJK 混合分词，BM25 与 HashEmbedder 共用，保证两路一致性）。
func tokenize(text string) []string {
	var toks []string
	var runes []rune
	for _, r := range text {
		switch {
		case isLatin(r):
			runes = append(runes, toLower(r))
		default:
			if len(runes) > 0 {
				toks = append(toks, string(runes))
				runes = runes[:0]
			}
			if isCJK(r) {
				toks = append(toks, string(r))
			}
		}
	}
	if len(runes) > 0 {
		toks = append(toks, string(runes))
	}
	// bigram 合并相邻 CJK 单字
	var out []string
	for i := 0; i < len(toks); i++ {
		out = append(out, toks[i])
		if i+1 < len(toks) && len([]rune(toks[i])) == 1 && len([]rune(toks[i+1])) == 1 {
			out = append(out, toks[i]+toks[i+1])
		}
	}
	return out
}

func isLatin(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isCJK(r rune) bool {
	return r >= 0x4e00 && r <= 0x9fff
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}

// Chunk：递归分块——段落优先聚合，目标 ~target 字符、overlap 滑窗。
// 中文论文友好：按空行分段，段内超长再按句号切。
type Chunk struct {
	ID      string
	Idx     int
	Content string
}

func SplitChunks(text string, target, overlap int) []Chunk {
	if target <= 0 {
		target = 1200
	}
	if overlap < 0 || overlap >= target {
		overlap = 150
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	paras := splitParagraphs(text)
	var chunks []Chunk
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if curLen > 0 {
			chunks = append(chunks, Chunk{Content: strings.TrimSpace(cur.String())})
			cur.Reset()
			curLen = 0
		}
	}
	for _, p := range paras {
		for _, piece := range splitLongParagraph(p, target) {
			if curLen+len(piece) > target && curLen > 0 {
				flush()
			}
			// 单段即超目标：直接作为独立块
			if len(piece) > target {
				flush()
				chunks = append(chunks, Chunk{Content: piece})
				continue
			}
			cur.WriteString(piece)
			cur.WriteString("\n\n")
			curLen += len(piece) + 2
		}
	}
	flush()
	for i := range chunks {
		chunks[i].Idx = i
		_ = overlap // v1 不做跨块 overlap：段落边界天然语义完整
	}
	return chunks
}

func splitParagraphs(text string) []string {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(norm, "\n\n")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitLongParagraph(p string, target int) []string {
	if len(p) <= target {
		return []string{p}
	}
	var out []string
	for len(p) > target {
		cut := findSentenceBreak(p[:target])
		if cut < target/2 {
			cut = target
		}
		out = append(out, strings.TrimSpace(p[:cut]))
		p = p[cut:]
	}
	if strings.TrimSpace(p) != "" {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func findSentenceBreak(s string) int {
	best := -1
	for _, b := range []string{"。", "！", "？", ". ", "! ", "? ", "；", "; "} {
		if i := strings.LastIndex(s, b); i > best {
			best = i + len(b)
		}
	}
	return best
}
