package latex

import (
	"testing"
)

func TestParseStructureMultiFileThesis(t *testing.T) {
	files := []ProjectText{
		{Path: "thesis.tex", Content: `\documentclass{article}
\begin{document}
\input{body/ch1}
\input{./body/ch2.tex}
\bibliography{refs}
\end{document}`},
		{Path: "body/ch1.tex", Content: `\chapter{绪论}
\input{sections/background}`},
		{Path: "sections/background.tex", Content: `背景正文`},
		{Path: "body/ch2.tex", Content: `\chapter{研究方法}
方法正文`},
		{Path: "refs.bib", Content: `@article{a, title={A}}`},
	}
	got := ParseStructure(files)

	if len(got.Roots) != 1 || got.Roots[0].Path != "thesis.tex" || !got.Roots[0].IsDefault {
		t.Fatalf("roots = %+v, want thesis.tex as default root", got.Roots)
	}
	if len(got.Units) != 2 {
		t.Fatalf("units = %+v, want 2 top-level chapters (nested input must not create a unit)", got.Units)
	}
	if got.Units[0].Title != "绪论" || got.Units[1].Title != "研究方法" {
		t.Fatalf("unit titles = %q, %q", got.Units[0].Title, got.Units[1].Title)
	}
	if got.Units[0].FilePaths[0] != "body/ch1.tex" || got.Units[1].FilePaths[0] != "body/ch2.tex" {
		t.Fatalf("unit files = %+v", got.Units)
	}
	if got.TooManyFiles || got.TooManyUnits {
		t.Fatalf("unexpected limit flags: %+v", got)
	}
	for _, f := range got.Files {
		if f.Path == "refs.bib" && !f.Referenced {
			t.Fatalf("bib file should be marked referenced")
		}
		if f.Path == "body/ch1.tex" && f.UnitID != got.Units[0].ID {
			t.Fatalf("ch1 unit id = %q, want %q", f.UnitID, got.Units[0].ID)
		}
	}
}

func TestParseStructureSingleFileAndPlaceholders(t *testing.T) {
	got := ParseStructure([]ProjectText{
		{Path: "paper.tex", Content: "\\documentclass{article}\n\\begin{document}\n正文\n\\end{document}\n"},
	})
	if len(got.Roots) != 1 || len(got.Units) != 1 || got.Units[0].FilePaths[0] != "paper.tex" {
		t.Fatalf("single-file project should yield one unit: %+v", got)
	}
	if got.Units[0].Title != "paper" {
		t.Fatalf("title fallback = %q, want file base name", got.Units[0].Title)
	}
}

func TestParseStructureNoRootUsesTopLevelFiles(t *testing.T) {
	got := ParseStructure([]ProjectText{
		{Path: "intro.tex", Content: "\\section{引言}\n\\input{shared}"},
		{Path: "shared.tex", Content: "共享段落"},
	})
	if len(got.Roots) != 0 {
		t.Fatalf("roots = %+v, want none", got.Roots)
	}
	if len(got.Units) != 1 || got.Units[0].FilePaths[0] != "intro.tex" {
		t.Fatalf("units = %+v, want only intro.tex (shared.tex is included)", got.Units)
	}
}

func TestParseStructureWarnings(t *testing.T) {
	got := ParseStructure([]ProjectText{
		{Path: "thesis.tex", Content: "\\documentclass{article}\n\\begin{document}\n\\input{body/missing}\n\\end{document}"},
		{Path: "other.tex", Content: "\\documentclass{article}"},
		{Path: "stray.tex", Content: "没有被包含的文件"},
	})
	codes := map[string]bool{}
	for _, w := range got.Warnings {
		codes[w.Code] = true
	}
	for _, want := range []string{"missing_include", "multiple_roots", "unreferenced_file"} {
		if !codes[want] {
			t.Fatalf("warnings = %+v, missing %s", got.Warnings, want)
		}
	}
}

func TestParseStructureCycle(t *testing.T) {
	got := ParseStructure([]ProjectText{
		{Path: "thesis.tex", Content: "\\documentclass{article}\n\\input{a}\n\\input{b}"},
		{Path: "a.tex", Content: "\\input{b}"},
		{Path: "b.tex", Content: "\\input{a}"},
	})
	found := false
	for _, w := range got.Warnings {
		if w.Code == "include_cycle" {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v, want include_cycle", got.Warnings)
	}
}

func TestStripLatexComments(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"正文 % 注释\n下一行", "正文 \n下一行"},
		{`50\% 不是注释`, `50\% 不是注释`},
		{`\\% 是注释`, `\\`},
		{"没有注释", "没有注释"},
	}
	for _, c := range cases {
		if got := StripLatexComments(c.in); got != c.want {
			t.Fatalf("StripLatexComments(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseStructureLimitsFlagWithoutTruncation(t *testing.T) {
	files := make([]ProjectText, 0, StructureMaxUnits+1)
	for i := 0; i <= StructureMaxUnits; i++ {
		files = append(files, ProjectText{Path: "ch" + string(rune('a'+i)) + ".tex", Content: "\\section{章节}"})
	}
	got := ParseStructure(files)
	if !got.TooManyUnits {
		t.Fatalf("TooManyUnits = false, want true for %d units", len(got.Units))
	}
	if len(got.Units) != StructureMaxUnits+1 {
		t.Fatalf("units truncated to %d; limits must be reported, not silently truncate", len(got.Units))
	}
}
