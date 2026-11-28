package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/norms"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestLatexIntegrityGuard(t *testing.T) {
	cases := []struct {
		name    string
		oldText string
		newText string
		wantErr bool
	}{
		{name: "保留引用", oldText: `见\cite{a}的结论`, newText: `见\cite{a}的结论，并补充背景`, wantErr: false},
		{name: "新增引用允许", oldText: `见\cite{a}`, newText: `见\cite{a}\cite{b}`, wantErr: false},
		{name: "删除引用拒绝", oldText: `见\cite{a}的结论`, newText: `见的结论`, wantErr: true},
		{name: "引用参数被改拒绝", oldText: `见\cite{a}`, newText: `见\cite{b}`, wantErr: true},
		{name: "重复引用少一个拒绝", oldText: `\cite{a}与\cite{a}`, newText: `\cite{a}`, wantErr: true},
		{name: "删除 label 拒绝", oldText: `\label{sec:1}正文`, newText: `正文`, wantErr: true},
		{name: "保留公式", oldText: `设 $x=1$，则`, newText: `设 $x=1$，因此`, wantErr: false},
		{name: "公式定界符变化拒绝", oldText: `设 $x=1$，则`, newText: `设 x=1，则`, wantErr: true},
		{name: "equation 环境开闭变化拒绝", oldText: "\\begin{equation}\nx=1\n\\end{equation}", newText: "\\begin{equation}\nx=1\n", wantErr: true},
		{name: "转义美元符不算公式", oldText: `价格 \$5`, newText: `价格 \$6`, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := latexIntegrityError(tc.oldText, tc.newText)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSuggestionQuality(t *testing.T) {
	support := []proposalBasis{{Type: BasisNorm, Stance: StanceSupports, Cite: "N-1"}}
	uncertain := []proposalBasis{{Type: BasisNorm, Stance: StanceUncertain, Cite: "N-1"}}
	valid := proposeEditArgs{SuggestionKind: "language", Confidence: ConfidenceHigh}

	assert.NoError(t, validateSuggestionQuality(valid, support))
	assert.Error(t, validateSuggestionQuality(valid, nil), "无依据必须丢弃")
	assert.Error(t, validateSuggestionQuality(valid, uncertain), "全部 uncertain 必须丢弃")
	assert.Error(t, validateSuggestionQuality(proposeEditArgs{SuggestionKind: "language", Confidence: ConfidenceMedium}, support), "medium 必须丢弃")
	assert.Error(t, validateSuggestionQuality(proposeEditArgs{SuggestionKind: "made_up", Confidence: ConfidenceHigh}, support), "未知 kind 必须拒绝")
}

func TestResolveProposalBasisEvidenceOwnership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/basis.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	uid, pid := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "basis", PasswordHash: "x",
		BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.Project{Name: "B", OwnerID: uid,
		BaseModel: store.BaseModel{ID: pid}}).Error)
	convID, runID, otherRun := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentConversation{ID: convID, ProjectID: pid, UserID: uid, Title: "t"}).Error)
	for _, id := range []store.UUID{runID, otherRun} {
		require.NoError(t, db.Create(&store.AgentRun{ID: id, ConversationID: convID, AgentKey: AgentKeyMain,
			Status: RunCompleted, ModelProvider: "p", ModelName: "m", PromptVersion: "v"}).Error)
	}
	quote := "原始材料中的一句话"
	evidence := store.AgentEvidence{ID: store.MustNewUUID(), RunID: runID, SourceType: "document",
		SourceID: store.MustNewUUID(), SourceName: "文献A", Quote: &quote}
	require.NoError(t, db.Create(&evidence).Error)

	ctx := context.Background()
	inputs := []proposalBasisInput{{Type: BasisEvidence, EvidenceID: evidence.ID.String(), Stance: StanceSupports}}
	basis, err := resolveProposalBasis(ctx, db, norms.Unavailable{}, pid, runID, inputs, nil)
	require.NoError(t, err)
	require.Len(t, basis, 1)
	assert.Equal(t, "文献A", basis[0].SourceName)
	assert.Equal(t, quote, basis[0].Quote)

	// 跨 Run 的 evidence_id 不可冒用
	_, err = resolveProposalBasis(ctx, db, norms.Unavailable{}, pid, otherRun, inputs, nil)
	assert.Error(t, err)

	// 没有已采用计划时不能引用 plan 依据
	_, err = resolveProposalBasis(ctx, db, norms.Unavailable{}, pid, runID, []proposalBasisInput{
		{Type: BasisPlan, SectionID: "ch1", Gap: "缺口", Stance: StanceSupports},
	}, nil)
	assert.Error(t, err)
}
