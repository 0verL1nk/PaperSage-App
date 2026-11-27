package review

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/research"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func testContext() *projectContext {
	ch1 := "\\chapter{绪论}\n本文研究样本为 120 名学生。\n"
	ch2 := "\\chapter{方法}\n样本为 80 名学生。\n"
	filePath := "body/ch1.tex"
	plan := &research.LoadedChapterPlan{
		ArtifactID: store.MustNewUUID(), Revision: 2, Status: "adopted",
		Plan: &research.ChapterPlan{Sections: []research.ChapterSection{
			{ID: "s1", Title: "绪论", FilePath: &filePath, Responsibility: "说明研究背景"},
		}},
	}
	return &projectContext{
		project: store.Project{Name: "测试论文", Scenario: store.ScenarioUndergraduateThesis},
		files: []latex.ProjectText{
			{Path: "body/ch1.tex", Content: ch1},
			{Path: "body/ch2.tex", Content: ch2},
		},
		fileByPath: map[string]string{"body/ch1.tex": ch1, "body/ch2.tex": ch2},
		units: []UnitOption{
			{ID: "u1", Title: "绪论", FilePaths: []string{"body/ch1.tex"}, SectionIDs: []string{"s1"}, Order: 1},
			{ID: "u2", Title: "方法", FilePaths: []string{"body/ch2.tex"}, Order: 2},
		},
		plan:    plan,
		normsOK: true,
		norms: []norms.Entry{{
			ID: "n1", Version: 3, Title: "引用规范", Rule: "引用必须标注来源", Source: "测试规范",
		}},
	}
}

func TestPlanSplitsChapterAndCollectionUnits(t *testing.T) {
	pc := testContext()
	svc := &Service{}
	planned, scope, err := svc.Plan(context.Background(), pc, &agent.ResolvedModel{Name: "test", Model: "m"}, CreateRequest{
		ClientRequestID: "req-1",
		Scope: ScopeRequest{
			FilePaths: []string{"body/ch1.tex"}, Dimensions: []string{"evidence_support", "cross_section"},
		},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(planned) != 2 {
		t.Fatalf("units = %d, want 2 (one chapter dimension + one collection dimension)", len(planned))
	}
	if planned[0].Kind != "chapter" || planned[0].Dimensions[0] != "evidence_support" {
		t.Fatalf("chapter unit = %+v", planned[0])
	}
	if planned[1].Kind != "collection" || planned[1].Dimensions[0] != "cross_section" {
		t.Fatalf("collection unit = %+v", planned[1])
	}
	if len(scope.Files) != 1 || scope.Files[0] != "body/ch1.tex" {
		t.Fatalf("scope files = %v", scope.Files)
	}
	if planned[0].Instruction == "" || !strings.Contains(planned[0].Instruction, "evidence_support") {
		t.Fatalf("instruction missing dimension contract")
	}
}

func TestPlanRejectsOutOfScopeAndWrongScenario(t *testing.T) {
	pc := testContext()
	svc := &Service{}
	cases := []struct {
		name string
		req  CreateRequest
		want string
	}{
		{"missing client id", CreateRequest{Scope: ScopeRequest{FilePaths: []string{"body/ch1.tex"}, Dimensions: []string{"evidence_support"}}}, "请求标识"},
		{"unknown file", CreateRequest{ClientRequestID: "r", Scope: ScopeRequest{FilePaths: []string{"nope.tex"}, Dimensions: []string{"evidence_support"}}}, "不存在"},
		{"wrong scenario", CreateRequest{ClientRequestID: "r", Scope: ScopeRequest{FilePaths: []string{"body/ch1.tex"}, Dimensions: []string{"paper_contribution_closure"}}}, "不适用"},
		{"no dimensions", CreateRequest{ClientRequestID: "r", Scope: ScopeRequest{FilePaths: []string{"body/ch1.tex"}}}, "维度"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := svc.Plan(context.Background(), pc, &agent.ResolvedModel{Name: "test", Model: "m"}, c.req)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want contains %q", err, c.want)
			}
		})
	}
}

func TestFingerprintStableAndContentSensitive(t *testing.T) {
	pc := testContext()
	model := &agent.ResolvedModel{Name: "test", Model: "m"}
	a := fingerprintUnit(pc, model, []string{"evidence_support"}, []string{"body/ch1.tex"}, []string{"s1"})
	b := fingerprintUnit(pc, model, []string{"evidence_support"}, []string{"body/ch1.tex"}, []string{"s1"})
	if a != b {
		t.Fatalf("fingerprint not stable: %s vs %s", a, b)
	}
	pc.fileByPath["body/ch1.tex"] = "内容已修改"
	c := fingerprintUnit(pc, model, []string{"evidence_support"}, []string{"body/ch1.tex"}, []string{"s1"})
	if a == c {
		t.Fatalf("fingerprint must change when content changes")
	}
}

func validUnit() *plannedUnit {
	return &plannedUnit{
		Kind: "chapter", Label: "绪论 · 论断证据支撑",
		FilePaths: []string{"body/ch1.tex"}, SectionIDs: []string{"s1"}, Dimensions: []string{"evidence_support"},
	}
}

func payloadFor(findings ...research.ReviewFinding) []byte {
	raw, _ := json.Marshal(unitReportPayload{
		Dimensions: []research.ReviewDimension{{Key: "evidence_support", Status: "findings"}},
		Findings:   findings,
	})
	return raw
}

func validFinding() research.ReviewFinding {
	return research.ReviewFinding{
		ID: "f1", Dimension: "evidence_support", Severity: "high",
		FilePath: "body/ch1.tex", Quote: "本文研究样本为 120 名学生。",
		Problem: "样本量结论缺少来源", NextStep: "补充样本来源说明",
		ClaimStatus: "overclaimed",
		Basis: []research.ReviewBasis{{
			Kind: "text", Ref: "body/ch1.tex", Quote: "样本为 80 名学生。",
		}},
	}
}

func TestValidateUnitReportAcceptsEvidenceWithVerbatimBasis(t *testing.T) {
	pc := testContext()
	// 同一文件里放两段文本以便 basis 命中；真实场景中依据来自资料或原文。
	pc.fileByPath["body/ch1.tex"] = "\\chapter{绪论}\n本文研究样本为 120 名学生。\n样本为 80 名学生。\n"
	svc := &Service{}
	result, err := svc.validateUnitReport(context.Background(), pc, validUnit(), payloadFor(validFinding()), nil)
	if err != nil {
		t.Fatalf("validateUnitReport() error = %v", err)
	}
	if len(result.Findings) != 1 || !result.Findings[0].AnchorVerified {
		t.Fatalf("finding not verified: %+v", result.Findings)
	}
	if result.Findings[0].LineStart == 0 || result.Findings[0].Fingerprint == "" {
		t.Fatalf("server must compute line numbers and fingerprint: %+v", result.Findings[0])
	}
}

func TestValidateUnitReportRejectsScopeAndEvidenceViolations(t *testing.T) {
	svc := &Service{}
	cases := []struct {
		name   string
		mutate func(*projectContext, *plannedUnit, *research.ReviewFinding)
		want   string
	}{
		{"file out of scope", func(_ *projectContext, _ *plannedUnit, f *research.ReviewFinding) {
			f.FilePath = "body/ch2.tex"
		}, "不在本单元范围内"},
		{"quote not verbatim", func(_ *projectContext, _ *plannedUnit, f *research.ReviewFinding) {
			f.Quote = "原文被改写了"
		}, "找不到"},
		{"claim status required", func(_ *projectContext, _ *plannedUnit, f *research.ReviewFinding) {
			f.ClaimStatus = ""
		}, "必须标注"},
		{"overclaimed needs basis", func(_ *projectContext, _ *plannedUnit, f *research.ReviewFinding) {
			f.Basis = nil
		}, "对照依据"},
		{"basis outside scope", func(_ *projectContext, _ *plannedUnit, f *research.ReviewFinding) {
			f.Basis[0].Ref = "body/ch2.tex"
		}, "不在本单元范围内"},
		{"norms missing blocks findings", func(pc *projectContext, u *plannedUnit, f *research.ReviewFinding) {
			pc.normsOK = false
			pc.norms = nil
			u.Dimensions = []string{"norm_compliance"}
			f.Dimension = "norm_compliance"
			f.ClaimStatus = ""
			f.Quote = ""
			f.Basis = nil
		}, "只能标记为证据不足"},
		{"plan missing blocks goal findings", func(pc *projectContext, u *plannedUnit, f *research.ReviewFinding) {
			pc.plan = nil
			u.Dimensions = []string{"goal_conclusion"}
			f.Dimension = "goal_conclusion"
			f.ClaimStatus = ""
		}, "只能标记为证据不足"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pc := testContext()
			pc.fileByPath["body/ch1.tex"] = "\\chapter{绪论}\n本文研究样本为 120 名学生。\n样本为 80 名学生。\n"
			unit := validUnit()
			finding := validFinding()
			c.mutate(pc, unit, &finding)
			payload, _ := json.Marshal(unitReportPayload{
				Dimensions: []research.ReviewDimension{{Key: unit.Dimensions[0], Status: "findings"}},
				Findings:   []research.ReviewFinding{finding},
			})
			_, err := svc.validateUnitReport(context.Background(), pc, unit, payload, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want contains %q", err, c.want)
			}
		})
	}
}

func TestValidateUnitReportRejectsUnknownFields(t *testing.T) {
	pc := testContext()
	svc := &Service{}
	payload := []byte(`{"dimensions":[{"key":"evidence_support","status":"checked"}],"findings":[],"verdict":"pass"}`)
	_, err := svc.validateUnitReport(context.Background(), pc, validUnit(), payload, nil)
	if err == nil || !strings.Contains(err.Error(), "结构不合法") {
		t.Fatalf("err = %v, want unknown-field rejection", err)
	}
}

func TestValidateUnitReportOrphanedRequiresReadingMaterials(t *testing.T) {
	pc := testContext()
	pc.documents = []store.Document{{Name: "参考文献.pdf", Type: "pdf"}}
	finding := validFinding()
	finding.ClaimStatus = "orphaned"
	finding.Basis = nil
	payload := payloadFor(finding)
	svc := &Service{}
	if _, err := svc.validateUnitReport(context.Background(), pc, validUnit(), payload, nil); err == nil || !strings.Contains(err.Error(), "未读取") {
		t.Fatalf("err = %v, want unread-materials rejection", err)
	}
	read := []agent.ReviewSource{{Kind: "document", Name: "参考文献.pdf"}}
	if _, err := svc.validateUnitReport(context.Background(), pc, validUnit(), payload, read); err != nil {
		t.Fatalf("after reading materials, orphaned should pass: %v", err)
	}
}

func TestValidateUnitReportCrossSectionNeedsBothSides(t *testing.T) {
	pc := testContext()
	pc.fileByPath["body/ch2.tex"] += "样本为 80 名学生。\n"
	unit := &plannedUnit{
		Kind: "collection", Label: "章节间重复/矛盾",
		FilePaths: []string{"body/ch1.tex", "body/ch2.tex"}, Dimensions: []string{"cross_section"},
	}
	finding := research.ReviewFinding{
		ID: "f1", Dimension: "cross_section", Severity: "high",
		FilePath: "body/ch1.tex", Quote: "本文研究样本为 120 名学生。",
		Problem: "样本量前后不一致", NextStep: "统一样本量",
		Basis: []research.ReviewBasis{
			{Kind: "text", Ref: "body/ch1.tex", Quote: "本文研究样本为 120 名学生。"},
			{Kind: "text", Ref: "body/ch2.tex", Quote: "样本为 80 名学生。"},
		},
	}
	payload, _ := json.Marshal(unitReportPayload{
		Dimensions: []research.ReviewDimension{{Key: "cross_section", Status: "findings"}},
		Findings:   []research.ReviewFinding{finding},
	})
	svc := &Service{}
	if _, err := svc.validateUnitReport(context.Background(), pc, unit, payload, nil); err != nil {
		t.Fatalf("cross-section with both sides should pass: %v", err)
	}
	finding.Basis = finding.Basis[:1]
	payload, _ = json.Marshal(unitReportPayload{
		Dimensions: []research.ReviewDimension{{Key: "cross_section", Status: "findings"}},
		Findings:   []research.ReviewFinding{finding},
	})
	if _, err := svc.validateUnitReport(context.Background(), pc, unit, payload, nil); err == nil {
		t.Fatal("cross-section with one side must be rejected")
	}
}
