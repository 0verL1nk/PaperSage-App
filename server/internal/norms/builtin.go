package norms

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// 内置权威种子：内容随二进制分发，启动时幂等写入。
//
// 只收可公开核验的国家标准与教育部规章，且不含任何学校专属要求——
// 没有校方原始文件就编造条款号和校名，比没有更糟。学校规范走用户自建包。
//
//go:embed builtin/*.json
var builtinFS embed.FS

type builtinPack struct {
	Key        string         `json:"key"`
	Name       string         `json:"name"`
	Authority  string         `json:"authority"`
	Edition    string         `json:"edition"`
	SourceURL  string         `json:"sourceUrl"`
	SourceNote string         `json:"sourceNote"`
	Scenario   string         `json:"scenario"`
	Entries    []builtinEntry `json:"entries"`
}

type builtinEntry struct {
	Code               string    `json:"code"`
	Category           string    `json:"category"`
	RuleKind           string    `json:"ruleKind"`
	Binding            string    `json:"binding"`
	Scenario           string    `json:"scenario"`
	SortOrder          int       `json:"sortOrder"`
	Tags               []string  `json:"tags"`
	Title              string    `json:"title"`
	RuleText           string    `json:"ruleText"`
	Rationale          string    `json:"rationale"`
	Examples           []Example `json:"examples"`
	ApplicableSections []string  `json:"applicableSections"`
	SourceRef          string    `json:"sourceRef"`
	SourceQuote        string    `json:"sourceQuote"`
	SourceURL          string    `json:"sourceUrl"`
}

// BuiltinPacks 解析并校验内置种子。校验不过直接报错——宁可启动失败，
// 也不允许无出处的规范进库。
func BuiltinPacks() ([]builtinPack, error) {
	names, err := fs.Glob(builtinFS, "builtin/*.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]builtinPack, 0, len(names))
	seenKeys := map[string]bool{}
	for _, name := range names {
		raw, err := builtinFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var pack builtinPack
		if err := json.Unmarshal(raw, &pack); err != nil {
			return nil, fmt.Errorf("内置规范包 %s 解析失败: %w", name, err)
		}
		if err := validateBuiltinPack(pack); err != nil {
			return nil, fmt.Errorf("内置规范包 %s: %w", name, err)
		}
		if seenKeys[pack.Key] {
			return nil, fmt.Errorf("内置规范包标识重复: %s", pack.Key)
		}
		seenKeys[pack.Key] = true
		out = append(out, pack)
	}
	return out, nil
}

func validateBuiltinPack(pack builtinPack) error {
	if !packKeyPattern.MatchString(pack.Key) {
		return fmt.Errorf("标识 %q 不合法", pack.Key)
	}
	if strings.TrimSpace(pack.Name) == "" {
		return errors.New("缺少名称")
	}
	if !authorities[pack.Authority] {
		return fmt.Errorf("来源类型 %q 不合法", pack.Authority)
	}
	if !scenarios[pack.Scenario] || pack.Scenario == store.NormScenarioAll {
		return fmt.Errorf("适用场景 %q 不合法：内置包必须明确场景", pack.Scenario)
	}
	if len(pack.Entries) == 0 {
		return errors.New("没有条目")
	}
	codes := map[string]bool{}
	for _, e := range pack.Entries {
		if !entryCodePattern.MatchString(e.Code) {
			return fmt.Errorf("条目编号 %q 不合法", e.Code)
		}
		if codes[e.Code] {
			return fmt.Errorf("条目编号重复: %s", e.Code)
		}
		codes[e.Code] = true
		if strings.TrimSpace(e.Title) == "" || strings.TrimSpace(e.RuleText) == "" || strings.TrimSpace(e.Category) == "" {
			return fmt.Errorf("条目 %s 缺少标题、规则说明或分类", e.Code)
		}
		if strings.TrimSpace(e.SourceRef) == "" {
			return fmt.Errorf("条目 %s 缺少来源出处", e.Code)
		}
		if strings.TrimSpace(e.SourceQuote) == "" && strings.TrimSpace(e.SourceURL) == "" {
			return fmt.Errorf("条目 %s 的来源既无原文摘录也无链接，无法核验", e.Code)
		}
		if !ruleKinds[e.RuleKind] {
			return fmt.Errorf("条目 %s 的规范类型不合法", e.Code)
		}
		if !bindings[e.Binding] {
			return fmt.Errorf("条目 %s 的强制程度不合法", e.Code)
		}
		if !scenarios[e.Scenario] {
			return fmt.Errorf("条目 %s 的适用场景不合法", e.Code)
		}
		for _, ex := range e.Examples {
			if !exampleKinds[ex.Kind] {
				return fmt.Errorf("条目 %s 的示例类型只能是 good 或 bad", e.Code)
			}
		}
	}
	return nil
}

// EnsureBuiltins 幂等写入内置规范包：按 pack_key 判断，已存在一律不动。
// 内置包更新 = 新 key + 新 edition，旧包归档但保留，已采用的项目不受影响。
func EnsureBuiltins(ctx context.Context, db *gorm.DB) error {
	packs, err := BuiltinPacks()
	if err != nil {
		return err
	}
	for _, pack := range packs {
		var existing store.NormPack
		err := db.WithContext(ctx).Where("pack_key = ?", pack.Key).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := seedBuiltinPack(ctx, db, pack); err != nil {
			return fmt.Errorf("写入内置规范包 %s 失败: %w", pack.Key, err)
		}
	}
	return nil
}

func seedBuiltinPack(ctx context.Context, db *gorm.DB, pack builtinPack) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := store.NormPack{
			Key: pack.Key, Name: pack.Name, Authority: pack.Authority, Edition: pack.Edition,
			Scenario: pack.Scenario, IsBuiltin: true, State: store.NormStateActive,
		}
		if pack.SourceURL != "" {
			sourceURL := pack.SourceURL
			row.SourceURL = &sourceURL
		}
		if pack.SourceNote != "" {
			sourceNote := pack.SourceNote
			row.SourceNote = &sourceNote
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, e := range pack.Entries {
			req := entryRequest{
				Code: e.Code, Category: e.Category, RuleKind: e.RuleKind, Binding: e.Binding,
				Scenario: e.Scenario, SortOrder: e.SortOrder, Tags: e.Tags,
				Title: e.Title, RuleText: e.RuleText, Rationale: e.Rationale,
				Examples: e.Examples, ApplicableSections: e.ApplicableSections,
				SourceRef: e.SourceRef, SourceQuote: e.SourceQuote, SourceURL: e.SourceURL,
			}
			version, err := buildVersion(req, 1, store.NormOriginBuiltin, nil)
			if err != nil {
				return fmt.Errorf("条目 %s: %w", e.Code, err)
			}
			entry := store.NormEntry{
				PackID: row.ID, Code: e.Code, Category: e.Category, RuleKind: e.RuleKind,
				Binding: e.Binding, Scenario: e.Scenario, SortOrder: e.SortOrder,
				CurrentVersion: 1, State: store.NormStateActive,
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			version.EntryID = entry.ID
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
			if err := createTags(tx, entry.ID, e.Tags); err != nil {
				return err
			}
		}
		return nil
	})
}
