package review

import (
	"strings"
	"testing"

	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestDimensionsForScenario(t *testing.T) {
	thesis := DimensionsFor(store.ScenarioUndergraduateThesis)
	paper := DimensionsFor(store.ScenarioIntroPaper)
	if len(thesis) != 6 || len(paper) != 6 {
		t.Fatalf("dimension counts = %d/%d, want 6/6", len(thesis), len(paper))
	}
	if !DimensionAllowed(store.ScenarioUndergraduateThesis, "thesis_engineering_closure") {
		t.Fatal("undergraduate thesis must allow thesis_engineering_closure")
	}
	if DimensionAllowed(store.ScenarioIntroPaper, "thesis_engineering_closure") {
		t.Fatal("intro paper must reject thesis_engineering_closure")
	}
	if !DimensionAllowed(store.ScenarioIntroPaper, "paper_contribution_closure") {
		t.Fatal("intro paper must allow paper_contribution_closure")
	}
	if DimensionAllowed(store.ScenarioUndergraduateThesis, "unknown_dimension") {
		t.Fatal("unknown dimension must be rejected")
	}
}

func TestBuildUnitsMergesPlanByFilePath(t *testing.T) {
	structure := latex.Structure{
		Units: []latex.StructureUnit{
			{ID: "u1", Title: "ch1", FilePaths: []string{"body/ch1.tex"}, Order: 1, SizeBytes: 10},
			{ID: "u2", Title: "ch2", FilePaths: []string{"body/ch2.tex"}, Order: 2, SizeBytes: 20},
		},
		Files: []latex.StructureFile{
			{Path: "body/ch1.tex", Kind: "tex"},
			{Path: "body/ch2.tex", Kind: "tex"},
			{Path: "body/ch3.tex", Kind: "tex", SizeBytes: 30},
		},
	}
	plan := &research.LoadedChapterPlan{
		ArtifactID: store.MustNewUUID(), Revision: 2, Status: "adopted",
		Plan: &research.ChapterPlan{Sections: []research.ChapterSection{
			{ID: "s1", Title: "绪论", FilePath: ptr("body/ch1.tex")},
			{ID: "s2", Title: "研究方法", FilePath: ptr("body/ch2.tex")},
			{ID: "s3", Title: "实验", FilePath: ptr("body/ch3.tex")},
			{ID: "s4", Title: "没有文件的章节"},
		}},
	}
	units, warnings := BuildUnits(structure, plan)
	if len(units) != 3 {
		t.Fatalf("units = %+v, want 3 (two structural + one plan-only)", units)
	}
	if units[0].SectionIDs[0] != "s1" || units[1].SectionIDs[0] != "s2" {
		t.Fatalf("section ids = %+v", units)
	}
	if units[0].Title != "绪论" || units[1].Title != "研究方法" {
		t.Fatalf("plan titles should replace filename fallback: %+v", units)
	}
	if units[2].FilePaths[0] != "body/ch3.tex" || units[2].SectionIDs[0] != "s3" {
		t.Fatalf("plan-only file should become its own unit: %+v", units[2])
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "没有文件的章节") {
		t.Fatalf("warnings = %v, want unbound section warning", warnings)
	}
}

func TestBuildUnitsWarnsMissingPlanFile(t *testing.T) {
	structure := latex.Structure{
		Units: []latex.StructureUnit{{ID: "u1", Title: "绪论", FilePaths: []string{"ch1.tex"}, Order: 1}},
		Files: []latex.StructureFile{{Path: "ch1.tex", Kind: "tex"}},
	}
	plan := &research.LoadedChapterPlan{
		Plan: &research.ChapterPlan{Sections: []research.ChapterSection{
			{ID: "s9", Title: "缺失章节", FilePath: ptr("ch9.tex")},
		}},
	}
	_, warnings := BuildUnits(structure, plan)
	if !strings.Contains(strings.Join(warnings, "\n"), "ch9.tex") {
		t.Fatalf("warnings = %v, want missing file warning", warnings)
	}
}

func TestBuildSnapshotCapsAndFlags(t *testing.T) {
	files := []latex.ProjectText{
		{Path: "a.tex", Content: "12345"},
		{Path: "b.tex", Content: "67890"},
		{Path: "notes.md", Content: "不是结构文件"},
	}
	out, truncated := BuildSnapshot(files)
	if truncated {
		t.Fatalf("unexpected truncation for small snapshot")
	}
	if len(out) != 2 || out[1].Path != "b.tex" {
		t.Fatalf("snapshot = %+v", out)
	}
	big := []latex.ProjectText{{Path: "big.tex", Content: strings.Repeat("x", maxSnapshotBytes+1)}}
	out, truncated = BuildSnapshot(big)
	if !truncated || len(out) != 0 {
		t.Fatalf("oversized snapshot must be flagged and empty, got %d files truncated=%v", len(out), truncated)
	}
}

func ptr(s string) *string { return &s }
