// Command seed 向 E2E 数据库写入确定性种子数据（Go 版 seed_e2e_user.py）。
// 幂等：已存在的行跳过。S3_ENDPOINT 未设置时跳过对象上传。
package main

import (
	"context"
	"crypto/md5"
	cryptoSha256 "crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

var (
	e2eUserID    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	e2eProjectID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	e2eGroupID   = uuid.MustParse("33333333-3333-3333-3333-333333333333")

	// 跨章节检查固定案例：4 章正文 + 已采用章节计划 + 可读证据资料。
	e2eReviewProjectID  = uuid.MustParse("44444444-4444-4444-4444-444444444444")
	e2eReviewDocumentID = uuid.MustParse("55555555-5555-5555-5555-555555555555")

	e2eUsername = "e2euser"
	e2ePassword = "e2epass1" // 前端 zod 口令规则：≥8 位含字母数字
)

type seedFile struct{ path, ctype, content string }

func fileSeed() []seedFile {
	return []seedFile{
		{"main.tex", "text/x-tex", "\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}\n"},
		{"refs.bib", "text/x-bibtex", "@misc{test, title={Test}, year={2025}}\n"},
		{"sections/intro.tex", "text/x-tex", "\\section{Introduction}\nIntro text.\n"},
	}
}

// reviewFileSeed 是跨章节检查的固定案例：方法写 120 份、结论写 200 份，
// 引言与结果出现同一句话，结果章的论断没有证据。用于演示三类语义问题。
func reviewFileSeed() []seedFile {
	return []seedFile{
		{"main.tex", "text/x-tex", `\documentclass{ctexart}
\begin{document}
\input{chapters/intro}
\input{chapters/method}
\input{chapters/results}
\input{chapters/conclusion}
\end{document}
`},
		{"chapters/intro.tex", "text/x-tex", `\section{引言}
本文提出了一种面向本科毕业论文的写作辅助方法。
研究问题是：如何在不编造文献的前提下给出有据可查的写作建议。
`},
		{"chapters/method.tex", "text/x-tex", `\section{研究方法}
本研究共收集 120 份有效样本。
采用问卷与访谈相结合的方式，覆盖 3 所高校的本科生。
`},
		{"chapters/results.tex", "text/x-tex", `\section{实验结果}
本文提出了一种面向本科毕业论文的写作辅助方法。
在固定测试集上的准确率达到 92\%。
`},
		{"chapters/conclusion.tex", "text/x-tex", `\section{结论}
本研究共收集 200 份有效样本。
结果表明该方法能够提升写作效率。
`},
	}
}

const reviewDocumentText = "样本说明：本研究共收集 120 份有效样本，覆盖 3 所高校的本科生。本文提出了一种面向本科毕业论文的写作辅助方法。\n"

func reviewDocumentKey() string {
	return "projects/" + e2eReviewProjectID.String() + "/documents/sample-notes.txt"
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// ── user（bcrypt(SHA-256(password))，与 Python hash_password 相同）──
	var user store.User
	if err := db.Where("username = ?", e2eUsername).First(&user).Error; err != nil {
		// 前端双哈希协议：SHA-256(plain) 再 bcrypt（保留，这是契约不是怪癖）
		sha := sha256sum(e2ePassword)
		hashed, err := auth.HashPassword(sha)
		if err != nil {
			return err
		}
		email := "e2e@papersage.test"
		user = store.User{
			BaseModel:    store.BaseModel{ID: store.UUID{UUID: e2eUserID}},
			Username:     e2eUsername,
			PasswordHash: hashed,
			Email:        &email,
			IsActive:     true,
		}
		if err := db.Create(&user).Error; err != nil {
			return err
		}
		fmt.Printf("✓ Created user id=%s username=%s\n", e2eUserID, e2eUsername)
	} else {
		fmt.Printf("= User %s already exists, skipping\n", e2eUsername)
	}

	// ── group ──
	var group store.ProjectGroup
	if err := db.Where("id = ?", store.UUID{UUID: e2eGroupID}).First(&group).Error; err != nil {
		group = store.ProjectGroup{
			BaseModel: store.BaseModel{ID: store.UUID{UUID: e2eGroupID}},
			Name:      "e2e-default-group", OwnerID: store.UUID{UUID: e2eUserID}, SortOrder: 0,
		}
		if err := db.Create(&group).Error; err != nil {
			return err
		}
		fmt.Printf("✓ Created group id=%s\n", e2eGroupID)
	} else {
		fmt.Printf("= Group %s already exists, skipping\n", e2eGroupID)
	}

	// ── project ──
	var project store.Project
	if err := db.Where("id = ?", store.UUID{UUID: e2eProjectID}).First(&project).Error; err != nil {
		desc := "E2E seed project"
		project = store.Project{
			BaseModel: store.BaseModel{ID: store.UUID{UUID: e2eProjectID}},
			Name:      "default-project", Description: &desc,
			OwnerID: store.UUID{UUID: e2eUserID}, GroupID: &store.UUID{UUID: e2eGroupID},
			Status: "active",
		}
		if err := db.Create(&project).Error; err != nil {
			return err
		}
		fmt.Printf("✓ Created project id=%s\n", e2eProjectID)
	} else {
		fmt.Printf("= Project %s already exists, skipping\n", e2eProjectID)
	}

	// ── latex files（content_hash 统一 md5，与服务层去重逻辑一致；
	//     修正 Python seed 用 sha256 而服务用 md5 的不一致坏设计）──
	if err := seedLatexFiles(db, store.UUID{UUID: e2eProjectID}, fileSeed()); err != nil {
		return err
	}

	if err := seedReviewCase(ctx, db, user); err != nil {
		return err
	}

	// ── S3 对象（可选）──
	if cfg.S3Endpoint != "" {
		if err := seedS3(ctx, cfg); err != nil {
			fmt.Printf("! S3 seed skipped: %v\n", err)
		}
	} else {
		fmt.Println("! S3_ENDPOINT not set, skipping S3 seed")
	}

	// ── 导出 GITHUB_ENV（CI）──
	if ge := os.Getenv("GITHUB_ENV"); ge != "" {
		f, err := os.OpenFile(ge, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f,
				"E2E_USER=%s\nE2E_PASS=%s\nE2E_PROJECT_ID=%s\nE2E_REVIEW_PROJECT_ID=%s\n",
				e2eUsername, e2ePassword, e2eProjectID, e2eReviewProjectID)
			_ = f.Close()
		}
	}
	fmt.Printf("✓ seed complete (user=%s project=%s)\n", e2eUsername, e2eProjectID)
	return nil
}

func seedLatexFiles(db *gorm.DB, projectID store.UUID, seed []seedFile) error {
	var files []store.LatexFile
	_ = db.Where("project_id = ?", projectID).Find(&files).Error
	existing := map[string]bool{}
	for _, f := range files {
		existing[f.FilePath] = true
	}
	for _, sf := range seed {
		if existing[sf.path] {
			fmt.Printf("= File %s already exists, skipping\n", sf.path)
			continue
		}
		sum := md5.Sum([]byte(sf.content))
		hash := hex.EncodeToString(sum[:])
		key := storage.BuildKey(projectID.String(), sf.path)
		ctype, sz, k := sf.ctype, int64(len(sf.content)), key
		f := store.LatexFile{
			ProjectID:   projectID,
			FilePath:    sf.path,
			ContentType: &ctype,
			SizeBytes:   sz,
			ContentHash: &hash,
			S3Key:       &k,
			LastSavedAt: utcNow(),
		}
		if err := db.Create(&f).Error; err != nil {
			return err
		}
		fmt.Printf("✓ Created latex file %s\n", sf.path)
	}
	return nil
}

// seedReviewCase 幂等写入跨章节检查固定案例：项目、4 章正文、可读资料、
// 研究蓝图与一份已采用章节计划。演示脚本依赖这些固定内容产生三类问题。
func seedReviewCase(ctx context.Context, db *gorm.DB, user store.User) error {
	_ = ctx
	projectID := store.UUID{UUID: e2eReviewProjectID}
	var project store.Project
	if err := db.Where("id = ?", projectID).First(&project).Error; err != nil {
		desc := "固定演示案例：方法写 120 份样本、结论写 200 份，引言与结果重复同一句话，结果章论断缺证据。"
		project = store.Project{
			BaseModel:   store.BaseModel{ID: projectID},
			Name:        "跨章节检查固定案例（本科毕设）",
			Description: &desc,
			OwnerID:     user.ID, GroupID: &store.UUID{UUID: e2eGroupID},
			Status: "active", Scenario: "undergraduate_thesis",
		}
		if err := db.Create(&project).Error; err != nil {
			return err
		}
		fmt.Printf("✓ Created review project id=%s\n", e2eReviewProjectID)
	} else {
		fmt.Printf("= Review project %s already exists, skipping\n", e2eReviewProjectID)
	}
	if err := seedLatexFiles(db, projectID, reviewFileSeed()); err != nil {
		return err
	}

	var doc store.Document
	if err := db.Where("project_id = ? AND name = ?", projectID, "sample-notes.txt").First(&doc).Error; err != nil {
		doc = store.Document{
			BaseModel: store.BaseModel{ID: store.UUID{UUID: e2eReviewDocumentID}},
			Name:      "sample-notes.txt", Type: "text/plain",
			Size: int64(len(reviewDocumentText)), Status: "uploaded",
			S3Key: reviewDocumentKey(), ProjectID: projectID, OwnerID: user.ID,
		}
		if err := db.Create(&doc).Error; err != nil {
			return err
		}
		fmt.Println("✓ Created review sample document")
	}

	var state store.ProjectWritingState
	if err := db.Where("project_id = ?", projectID).First(&state).Error; err != nil {
		brief, _ := json.Marshal(map[string]any{
			"topic":            "基于大模型的本科毕业论文写作辅助方法",
			"background":       "本科毕业论文写作中常见章节目标与结论不一致、论断缺少证据、章节重复等问题。",
			"questions":        []string{"如何在不编造文献的前提下给出有据可查的写作建议？"},
			"goals":            []string{"实现跨章节检查，定位目标—结论不一致与证据缺口"},
			"scope_out":        []string{"不做查重与 AI 检测"},
			"objects_and_data": "4 章正文与 1 份样本说明资料",
			"methods":          "真实模型结合项目背景、章节计划与资料证据进行语义审阅",
			"key_chapters":     []string{"引言", "研究方法", "实验结果", "结论", "讨论"},
			"milestones":       []string{},
			"constraints":      []string{"引用遵循 GB/T 7714"},
			"known_facts":      []string{"样本说明资料记录了 120 份有效样本"},
			"assumptions":      []string{},
			"unverified":       []string{"准确率 92% 尚无原始实验记录"},
		})
		requirements, _ := json.Marshal([]map[string]string{
			{"id": "r1", "text": "参考文献按 GB/T 7714 格式著录", "source": "学校写作指南", "note": ""},
		})
		phases, _ := json.Marshal(map[string]string{"prepare": "done", "write": "in_progress", "review": "in_progress"})
		state = store.ProjectWritingState{
			ProjectID: projectID, Brief: datatypes.JSON(brief), Requirements: datatypes.JSON(requirements),
			Phases: datatypes.JSON(phases), CurrentPhase: "review", Revision: 1,
		}
		if err := db.Create(&state).Error; err != nil {
			return err
		}
		fmt.Println("✓ Created review writing state")
	}

	var artifact store.ResearchArtifact
	if err := db.Where("project_id = ? AND kind = ?", projectID, store.ResearchArtifactChapterPlan).First(&artifact).Error; err != nil {
		artifact = store.ResearchArtifact{
			ProjectID: projectID, Kind: store.ResearchArtifactChapterPlan, State: "active",
			CurrentRevision: 1, CreatedBy: user.ID,
		}
		if err := db.Create(&artifact).Error; err != nil {
			return err
		}
		revision := store.ResearchArtifactRevision{
			ArtifactID: artifact.ID, Revision: 1, PayloadJSON: reviewPlanPayload(doc.ID.UUID),
			Origin: "user", Status: store.ResearchArtifactRevisionAdopted, CreatedBy: user.ID,
		}
		if err := db.Create(&revision).Error; err != nil {
			return err
		}
		if err := db.Model(&store.ResearchArtifact{}).Where("id = ?", artifact.ID).
			Update("adopted_revision", 1).Error; err != nil {
			return err
		}
		fmt.Println("✓ Created and adopted review chapter plan")
	}
	return nil
}

func reviewPlanPayload(documentID uuid.UUID) []byte {
	payload, _ := json.Marshal(map[string]any{
		"sections": []map[string]any{
			{
				"id": "s1", "title": "引言", "responsibility": "提出研究问题与本文贡献", "order": 1,
				"questions": []string{"研究问题是什么？"}, "filePath": "chapters/intro.tex",
				"claims": []map[string]any{{
					"text": "本文提出一种面向本科毕业论文的写作辅助方法", "status": "reported",
					"evidence": []map[string]any{{"documentId": documentID.String(), "quote": "本文提出了一种面向本科毕业论文的写作辅助方法。"}},
				}},
			},
			{
				"id": "s2", "title": "研究方法", "responsibility": "说明样本、数据与方法", "order": 2,
				"questions": []string{"样本量与数据来源是什么？"}, "filePath": "chapters/method.tex",
				"dependencies": []string{"s1"},
				"claims": []map[string]any{{
					"text": "共收集 120 份有效样本", "status": "reported",
					"evidence": []map[string]any{{"documentId": documentID.String(), "quote": "本研究共收集 120 份有效样本。"}},
				}},
			},
			{
				"id": "s3", "title": "实验结果", "responsibility": "报告实验结果", "order": 3,
				"questions": []string{"结果是否支持结论？"}, "filePath": "chapters/results.tex",
				"dependencies": []string{"s2"},
				"claims":       []map[string]any{{"text": "准确率达到 92%", "status": "not_read"}},
			},
			{
				"id": "s4", "title": "结论", "responsibility": "总结发现并回答研究问题", "order": 4,
				"questions": []string{"结论是否与方法一致？"}, "filePath": "chapters/conclusion.tex",
				"dependencies": []string{"s3"},
				"claims":       []map[string]any{{"text": "研究结论应与方法部分的样本量一致", "status": "not_read"}},
			},
			{
				"id": "s5", "title": "讨论", "responsibility": "讨论局限与未来工作", "order": 5,
				"dependencies": []string{"s4"},
			},
		},
	})
	return payload
}

func seedS3(ctx context.Context, cfg *config.Settings) error {
	s3, err := storage.NewS3(ctx, cfg)
	if err != nil {
		return err
	}
	// 等待 bucket 可达（最多 60s，对齐 _wait_for_s3；探种子对象 HEAD）
	probeKey := storage.BuildKey(e2eProjectID.String(), "main.tex")
	for i := 0; i < 30; i++ {
		if s3.Exists(ctx, probeKey) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	for _, sf := range fileSeed() {
		uploadSeedObject(ctx, s3, e2eProjectID, sf)
	}
	for _, sf := range reviewFileSeed() {
		uploadSeedObject(ctx, s3, e2eReviewProjectID, sf)
	}
	docKey := reviewDocumentKey()
	if s3.Exists(ctx, docKey) {
		fmt.Printf("= S3 object %s already exists, skipping\n", docKey)
	} else if err := s3.Put(ctx, docKey, []byte(reviewDocumentText)); err != nil {
		fmt.Printf("! Failed to upload %s: %v\n", docKey, err)
	} else {
		fmt.Printf("✓ Uploaded S3 object %s\n", docKey)
	}
	return nil
}

func uploadSeedObject(ctx context.Context, s3 storage.Storage, projectID uuid.UUID, sf seedFile) {
	key := storage.BuildKey(projectID.String(), sf.path)
	if s3.Exists(ctx, key) {
		fmt.Printf("= S3 object %s already exists, skipping\n", key)
		return
	}
	if err := s3.Put(ctx, key, []byte(sf.content)); err != nil {
		fmt.Printf("! Failed to upload %s: %v\n", key, err)
		return
	}
	fmt.Printf("✓ Uploaded S3 object %s\n", key)
}

func sha256sum(s string) string {
	h := cryptoSha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func utcNow() *time.Time {
	t := time.Now().UTC()
	return &t
}
