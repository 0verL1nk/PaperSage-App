// Package recheck 资料变更后的「待复查」标记：content_recheck_marks 的唯一所有者。
//
// 只做确定性依赖标记（产物依赖表 / 证据 / wiki 来源回溯），不做语义判断，
// 也不自动重写任何产物或正文——原内容是否仍成立由用户逐条确认。
package recheck

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// SourceChange 描述一次已提交的资料变更。
type SourceChange struct {
	Kind        string      // store.RecheckTriggerSourceDeleted | RecheckTriggerSourceNewVersion
	TriggerID   store.UUID  // 删除=文档 ID；新版本=新 source_version ID
	DocumentID  store.UUID  // 内容发生变化的文档
	SourceID    *store.UUID // 文档对应的来源（可能为空）
	NewRevision int         // source_new_version：新版本号；其余场景为 0
	Reason      string      // 中文原因，直接展示给用户
}

// MarkSourceChanged 在调用方事务内写入全部受影响目标的 open mark。
// 幂等：唯一键 + ON CONFLICT DO NOTHING，重复调用不会产生重复标记。
func MarkSourceChanged(ctx context.Context, tx *gorm.DB, projectID store.UUID, change SourceChange) error {
	if tx == nil {
		return fmt.Errorf("recheck: transaction is required")
	}
	if change.Kind != store.RecheckTriggerSourceDeleted && change.Kind != store.RecheckTriggerSourceNewVersion {
		return fmt.Errorf("recheck: unsupported trigger kind %q", change.Kind)
	}
	reason := truncateReason(change.Reason)

	hits, err := affectedRevisions(ctx, tx, projectID, change)
	if err != nil {
		return err
	}
	marks := make([]store.ContentRecheckMark, 0, len(hits))
	planRevisions := map[store.UUID]int{} // 计划产物 ID → 被标记的修订号
	for _, hit := range hits {
		artifactID, revision := hit.ArtifactID, hit.Revision
		marks = append(marks, store.ContentRecheckMark{
			ID: store.MustNewUUID(), ProjectID: projectID,
			TargetType: store.RecheckTargetArtifactRevision, TargetID: hit.RevisionID,
			ArtifactID: &artifactID, Revision: &revision,
			TriggerKind: change.Kind, TriggerID: change.TriggerID, Reason: reason,
		})
		if hit.ArtifactKind == store.ResearchArtifactChapterPlan {
			planRevisions[hit.ArtifactID] = hit.Revision
		}
	}

	// 证据：agent_evidence 的 source_version 是 chunk 文本哈希，无法精确归因来源版本；
	// 新版本触发时按文档保守标记（宁可多复查，不假装精确）。
	var evidenceIDs []store.UUID
	if err := tx.WithContext(ctx).Model(&store.AgentEvidence{}).
		Where("source_type = ? AND source_id = ?", "document", change.DocumentID).
		Pluck("id", &evidenceIDs).Error; err != nil {
		return err
	}
	for _, id := range evidenceIDs {
		marks = append(marks, store.ContentRecheckMark{
			ID: store.MustNewUUID(), ProjectID: projectID,
			TargetType: store.RecheckTargetEvidence, TargetID: id,
			TriggerKind: change.Kind, TriggerID: change.TriggerID, Reason: reason,
		})
	}

	// wiki：只标记有来源回溯记录的页；没有记录的页不假装可追溯。
	var wikiRows []struct {
		WikiPageID store.UUID `gorm:"column:wiki_page_id"`
	}
	if err := tx.WithContext(ctx).Table("agent_wiki_sources AS ws").
		Select("DISTINCT ws.wiki_page_id AS wiki_page_id").
		Joins("JOIN agent_wiki_pages AS p ON p.id = ws.wiki_page_id").
		Where("p.project_id = ? AND ws.source_type = ? AND ws.source_id = ?", projectID, "document", change.DocumentID).
		Scan(&wikiRows).Error; err != nil {
		return err
	}
	pageIDs := make([]store.UUID, 0, len(wikiRows))
	for _, row := range wikiRows {
		pageIDs = append(pageIDs, row.WikiPageID)
		marks = append(marks, store.ContentRecheckMark{
			ID: store.MustNewUUID(), ProjectID: projectID,
			TargetType: store.RecheckTargetWikiPage, TargetID: row.WikiPageID,
			TriggerKind: change.Kind, TriggerID: change.TriggerID, Reason: reason,
		})
	}
	if len(pageIDs) > 0 {
		// 已有 stale_at 不覆盖（保留最早过期时间）；agent_wiki_pages 无软删列，可用 Table。
		if err := tx.WithContext(ctx).Table("agent_wiki_pages").
			Where("id IN ? AND stale_at IS NULL", pageIDs).
			Update("stale_at", time.Now().UTC()).Error; err != nil {
			return err
		}
	}

	// 跨章节检查报告：只做 review ← plan 一跳级联，不递归。
	for planArtifactID, planRevision := range planRevisions {
		cascaded, err := cascadedReviewMarks(ctx, tx, projectID, planArtifactID, planRevision, change)
		if err != nil {
			return err
		}
		marks = append(marks, cascaded...)
	}

	if len(marks) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "target_type"}, {Name: "target_id"},
			{Name: "trigger_kind"}, {Name: "trigger_id"}, {Name: "resolved_seq"},
		},
		DoNothing: true,
	}).CreateInBatches(&marks, 100).Error
}

type revisionHit struct {
	RevisionID   store.UUID `gorm:"column:revision_id"`
	ArtifactID   store.UUID `gorm:"column:artifact_id"`
	Revision     int        `gorm:"column:revision"`
	ArtifactKind string     `gorm:"column:artifact_kind"`
}

// affectedRevisions 找出依赖该文档（document kind）或该来源版本（source_version kind）
// 的产物修订；已删产物不算。
func affectedRevisions(ctx context.Context, tx *gorm.DB, projectID store.UUID, change SourceChange) ([]revisionHit, error) {
	conds := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if change.SourceID != nil {
		sub := tx.WithContext(ctx).Model(&store.SourceVersion{}).Select("id").Where("source_id = ?", *change.SourceID)
		if change.Kind == store.RecheckTriggerSourceNewVersion {
			// 只有旧版本依赖失效；已经指向新版本的修订不需要复查。
			sub = sub.Where("revision < ?", change.NewRevision)
		}
		conds = append(conds, "(d.kind = ? AND d.ref_id IN (?))")
		args = append(args, store.ResearchDependencySourceVersion, sub)
	}
	conds = append(conds, "(d.kind = ? AND d.ref_id = ?)")
	args = append(args, store.ResearchDependencyDocument, change.DocumentID)

	var hits []revisionHit
	err := tx.WithContext(ctx).Table("research_artifact_dependencies AS d").
		Select("r.id AS revision_id, d.artifact_id, d.revision, a.kind AS artifact_kind").
		Joins("JOIN research_artifacts AS a ON a.id = d.artifact_id AND a.deleted_at IS NULL").
		Joins("JOIN research_artifact_revisions AS r ON r.artifact_id = d.artifact_id AND r.revision = d.revision").
		Where("d.project_id = ?", projectID).
		Where("("+strings.Join(conds, " OR ")+")", args...).
		Find(&hits).Error
	return hits, err
}

// cascadedReviewMarks 把「依据了该计划修订」的跨章节检查报告一并标为待复查。
// 依赖行只记计划产物 ID；具体检查的是哪一版由 payload.input.planRevision 决定，
// 因此基于另一版计划的报告不会被误标。
func cascadedReviewMarks(ctx context.Context, tx *gorm.DB, projectID store.UUID,
	planArtifactID store.UUID, planRevision int, change SourceChange) ([]store.ContentRecheckMark, error) {
	var rows []struct {
		RevisionID  store.UUID     `gorm:"column:revision_id"`
		ArtifactID  store.UUID     `gorm:"column:artifact_id"`
		Revision    int            `gorm:"column:revision"`
		PayloadJSON datatypes.JSON `gorm:"column:payload_json"`
	}
	if err := tx.WithContext(ctx).Table("research_artifact_dependencies AS d").
		Select("r.id AS revision_id, d.artifact_id, d.revision, r.payload_json").
		Joins("JOIN research_artifacts AS a ON a.id = d.artifact_id AND a.deleted_at IS NULL").
		Joins("JOIN research_artifact_revisions AS r ON r.artifact_id = d.artifact_id AND r.revision = d.revision").
		Where("d.project_id = ? AND d.kind = ? AND d.ref_id = ? AND a.kind = ?",
			projectID, store.ResearchDependencyArtifact, planArtifactID, store.ResearchArtifactChapterReview).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	reason := truncateReason(fmt.Sprintf("所依据的章节计划 v%d 因资料变更待复查", planRevision))
	out := make([]store.ContentRecheckMark, 0, len(rows))
	for _, row := range rows {
		var payload struct {
			Input struct {
				PlanArtifactID *store.UUID `json:"planArtifactId"`
				PlanRevision   *int        `json:"planRevision"`
			} `json:"input"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			continue
		}
		if payload.Input.PlanArtifactID == nil || payload.Input.PlanArtifactID.UUID != planArtifactID.UUID {
			continue
		}
		if payload.Input.PlanRevision != nil && *payload.Input.PlanRevision != planRevision {
			continue
		}
		artifactID, revision := row.ArtifactID, row.Revision
		out = append(out, store.ContentRecheckMark{
			ID: store.MustNewUUID(), ProjectID: projectID,
			TargetType: store.RecheckTargetArtifactRevision, TargetID: row.RevisionID,
			ArtifactID: &artifactID, Revision: &revision,
			TriggerKind: change.Kind, TriggerID: change.TriggerID, Reason: reason,
		})
	}
	return out, nil
}

// OpenReasons 返回单个目标当前未处理的待复查原因（按创建时间）。
func OpenReasons(ctx context.Context, db *gorm.DB, targetType string, targetID store.UUID) ([]string, error) {
	var reasons []string
	err := db.WithContext(ctx).Model(&store.ContentRecheckMark{}).
		Where("target_type = ? AND target_id = ? AND resolved_seq = 0", targetType, targetID).
		Order("created_at").Pluck("reason", &reasons).Error
	return reasons, err
}

// OpenRevisionMarks 批量返回产物修订的 open 原因（列表聚合用，避免 N+1）。
func OpenRevisionMarks(ctx context.Context, db *gorm.DB, revisionIDs []store.UUID) (map[store.UUID][]string, error) {
	out := map[store.UUID][]string{}
	if len(revisionIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		TargetID store.UUID `gorm:"column:target_id"`
		Reason   string     `gorm:"column:reason"`
	}
	if err := db.WithContext(ctx).Model(&store.ContentRecheckMark{}).
		Select("target_id, reason").
		Where("target_type = ? AND target_id IN ? AND resolved_seq = 0",
			store.RecheckTargetArtifactRevision, revisionIDs).
		Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		reasons := out[row.TargetID]
		duplicate := false
		for _, existing := range reasons {
			if existing == row.Reason {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out[row.TargetID] = append(reasons, row.Reason)
		}
	}
	return out, nil
}

func truncateReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "依赖的资料已发生变化，请确认原有内容是否仍然成立"
	}
	if runes := []rune(reason); len(runes) > 500 {
		return string(runes[:500])
	}
	return reason
}
