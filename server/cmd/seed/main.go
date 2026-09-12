// Command seed 向 E2E 数据库写入确定性种子数据（Go 版 seed_e2e_user.py）。
// 幂等：已存在的行跳过。S3_ENDPOINT 未设置时跳过对象上传。
package main

import (
	"context"
	"crypto/md5"
	cryptoSha256 "crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

var (
	e2eUserID    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	e2eProjectID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	e2eGroupID   = uuid.MustParse("33333333-3333-3333-3333-333333333333")

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
	var files []store.LatexFile
	_ = db.Where("project_id = ?", store.UUID{UUID: e2eProjectID}).Find(&files).Error
	existing := map[string]bool{}
	for _, f := range files {
		existing[f.FilePath] = true
	}
	for _, sf := range fileSeed() {
		if existing[sf.path] {
			fmt.Printf("= File %s already exists, skipping\n", sf.path)
			continue
		}
		sum := md5.Sum([]byte(sf.content))
		hash := hex.EncodeToString(sum[:])
		key := storage.BuildKey(e2eProjectID.String(), sf.path)
		ctype, sz, k := sf.ctype, int64(len(sf.content)), key
		f := store.LatexFile{
			ProjectID:   store.UUID{UUID: e2eProjectID},
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
				"E2E_USER=%s\nE2E_PASS=%s\nE2E_PROJECT_ID=%s\n", e2eUsername, e2ePassword, e2eProjectID)
			_ = f.Close()
		}
	}
	fmt.Printf("✓ seed complete (user=%s project=%s)\n", e2eUsername, e2eProjectID)
	return nil
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
		key := storage.BuildKey(e2eProjectID.String(), sf.path)
		if s3.Exists(ctx, key) {
			fmt.Printf("= S3 object %s already exists, skipping\n", key)
			continue
		}
		if err := s3.Put(ctx, key, []byte(sf.content)); err != nil {
			fmt.Printf("! Failed to upload %s: %v\n", key, err)
			continue
		}
		fmt.Printf("✓ Uploaded S3 object %s\n", key)
	}
	return nil
}

func sha256sum(s string) string {
	h := cryptoSha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func utcNow() *time.Time {
	t := time.Now().UTC()
	return &t
}
