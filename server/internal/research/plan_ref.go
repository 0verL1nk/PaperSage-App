package research

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// LoadedChapterPlan 是项目当前可用的章节计划：优先已采用修订，否则退回候选修订。
// 检查（agent 单元与 review/options）共用这一份读取逻辑，避免计划状态两处漂移。
type LoadedChapterPlan struct {
	ArtifactID store.UUID
	Revision   int
	Status     string // adopted | candidate
	Plan       *ChapterPlan
}

// LoadAdoptedChapterPlan 只读，不做语义判断；没有可用计划时返回 nil。
func LoadAdoptedChapterPlan(ctx context.Context, db *gorm.DB, projectID store.UUID) (*LoadedChapterPlan, []ReviewSource, []string) {
	var artifacts []store.ResearchArtifact
	if err := db.WithContext(ctx).Where("project_id = ? AND kind = ?", projectID, store.ResearchArtifactChapterPlan).
		Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, nil, []string{"读取章节计划失败：" + err.Error()}
	}
	var chosen *store.ResearchArtifact
	status := ""
	for i := range artifacts {
		if artifacts[i].AdoptedRevision != nil {
			chosen, status = &artifacts[i], "adopted"
			break
		}
	}
	if chosen == nil && len(artifacts) > 0 {
		chosen, status = &artifacts[0], "candidate"
	}
	if chosen == nil {
		return nil, nil, nil
	}
	revision := chosen.CurrentRevision
	if chosen.AdoptedRevision != nil {
		revision = *chosen.AdoptedRevision
	}
	if revision <= 0 {
		return nil, nil, []string{"章节计划没有可用修订"}
	}
	var rev store.ResearchArtifactRevision
	if err := db.WithContext(ctx).Where("artifact_id = ? AND revision = ?", chosen.ID, revision).First(&rev).Error; err != nil {
		return nil, nil, []string{"读取章节计划修订失败：" + err.Error()}
	}
	var plan ChapterPlan
	if err := json.Unmarshal(rev.PayloadJSON, &plan); err != nil || len(plan.Sections) == 0 {
		return nil, nil, []string{"章节计划内容不可用"}
	}
	name := fmt.Sprintf("章节计划 v%d（%s）", revision, map[string]string{"adopted": "已采用", "candidate": "候选未采用"}[status])
	return &LoadedChapterPlan{ArtifactID: chosen.ID, Revision: revision, Status: status, Plan: &plan},
		[]ReviewSource{{Kind: "plan", Name: name}}, nil
}
