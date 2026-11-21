package research

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func setup(t *testing.T) (*Service, *gorm.DB, store.UUID, store.UUID, store.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/research.db"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{Username: "research-a", PasswordHash: "x", BaseModel: store.BaseModel{ID: uid}}).Error)
	require.NoError(t, db.Create(&store.User{Username: "research-b", PasswordHash: "x", BaseModel: store.BaseModel{ID: other}}).Error)
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "P", OwnerID: uid, BaseModel: store.BaseModel{ID: pid}}).Error)
	return NewService(db), db, uid, other, pid
}

func seedSource(t *testing.T, db *gorm.DB, projectID store.UUID) store.UUID {
	t.Helper()
	src := store.ProjectSource{ProjectID: projectID, Kind: "document", CurrentRevision: 1}
	require.NoError(t, db.Create(&src).Error)
	ver := store.SourceVersion{SourceID: src.ID, Revision: 1, S3Key: "k", ContentSHA256: strings.Repeat("a", 64), SizeBytes: 1}
	require.NoError(t, db.Create(&ver).Error)
	return ver.ID
}

func notePayload(source store.UUID) json.RawMessage {
	field := func(text string, evidence bool) map[string]any {
		f := map[string]any{"text": text, "status": "reported"}
		if evidence {
			f["evidence"] = []map[string]any{{"sourceVersionId": source.String()}}
		}
		return f
	}
	raw, _ := json.Marshal(map[string]any{
		"source":   map[string]any{"sourceVersionId": source.String()},
		"question": field("研究问题", true), "method": field("方法", true),
		"objects": field("对象", true), "conditions": field("条件", true),
		"results": field("结果", true), "limitations": field("限制", true),
	})
	return raw
}

func TestCreateListAndIdempotency(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	source := seedSource(t, db, pid)
	ctx := context.Background()
	clientID := "req-1"
	created, err := svc.Create(ctx, uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source),
		Origin: "agent", ClientRequestID: &clientID,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, created.CurrentRevision)
	require.NotNil(t, created.Current)
	assert.Equal(t, store.ResearchArtifactRevisionCandidate, created.Current.Status)
	assert.Nil(t, created.AdoptedRevision)

	again, err := svc.Create(ctx, uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source), ClientRequestID: &clientID,
	})
	require.NoError(t, err)
	assert.Equal(t, created.ID, again.ID, "同一 clientRequestId 不得重复建产物")

	items, err := svc.List(ctx, uid, pid, store.ResearchArtifactLiteratureNote)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestReportedFieldRequiresEvidence(t *testing.T) {
	svc, db, uid, _, pid := setup(t)
	source := seedSource(t, db, pid)
	_, err := svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source),
	})
	require.NoError(t, err)

	bad := notePayload(source)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(bad, &payload))
	delete(payload["results"].(map[string]any), "evidence")
	raw, _ := json.Marshal(payload)
	_, err = svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: raw,
	})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
}

func TestAdoptAndSupersede(t *testing.T) {
	svc, _, uid, _, pid := setup(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactChapterPlan, Payload: planPayload()})
	require.NoError(t, err)
	artifactID := store.UUID{UUID: created.ID}

	adopted, err := svc.Adopt(ctx, uid, pid, artifactID, decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err)
	require.NotNil(t, adopted.AdoptedRevision)
	assert.Equal(t, 1, *adopted.AdoptedRevision)

	withCandidate, err := svc.AddRevision(ctx, uid, pid, artifactID, revisionRequest{
		ExpectedRevision: 1, Payload: planPayload(), Origin: "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, withCandidate.CurrentRevision)
	assert.Equal(t, 1, *withCandidate.AdoptedRevision, "新候选不得自动替换已采用计划")

	after, err := svc.Adopt(ctx, uid, pid, artifactID, decisionRequest{Revision: 2, ExpectedRevision: 2})
	require.NoError(t, err)
	assert.Equal(t, 2, *after.AdoptedRevision)
	statuses := map[int]string{}
	for _, rev := range after.Revisions {
		statuses[rev.Revision] = rev.Status
	}
	assert.Equal(t, store.ResearchArtifactRevisionSuperseded, statuses[1])
	assert.Equal(t, store.ResearchArtifactRevisionAdopted, statuses[2])
}

func TestRevisionConflictAndReject(t *testing.T) {
	svc, _, uid, _, pid := setup(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactChapterPlan, Payload: planPayload()})
	require.NoError(t, err)
	artifactID := store.UUID{UUID: created.ID}

	_, err = svc.AddRevision(ctx, uid, pid, artifactID, revisionRequest{ExpectedRevision: 9, Payload: planPayload()})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 409, apiErr.Code)

	rejected, err := svc.Reject(ctx, uid, pid, artifactID, decisionRequest{Revision: 1, ExpectedRevision: 1})
	require.NoError(t, err)
	assert.Equal(t, store.ResearchArtifactRevisionRejected, rejected.Revisions[0].Status)
}

func TestCrossProjectIsolation(t *testing.T) {
	svc, db, uid, other, pid := setup(t)
	source := seedSource(t, db, pid)
	ctx := context.Background()
	_, err := svc.Create(ctx, other, pid, createRequest{Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source)})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.Code)

	created, err := svc.Create(ctx, uid, pid, createRequest{Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(source)})
	require.NoError(t, err)
	_, err = svc.Get(ctx, other, pid, store.UUID{UUID: created.ID})
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.Code)
}

func TestForeignEvidenceRejected(t *testing.T) {
	svc, db, uid, other, pid := setup(t)
	_ = other
	foreignPID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.Project{Name: "Other", OwnerID: uid, BaseModel: store.BaseModel{ID: foreignPID}}).Error)
	foreignSource := seedSource(t, db, foreignPID)
	_, err := svc.Create(context.Background(), uid, pid, createRequest{
		Kind: store.ResearchArtifactLiteratureNote, Payload: notePayload(foreignSource),
	})
	var apiErr *httpapi.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
}

func planPayload() json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"sections": []map[string]any{
			{"id": "s1", "title": "引言", "responsibility": "提出问题", "order": 1},
			{"id": "s2", "title": "方法", "responsibility": "说明方法", "order": 2, "dependencies": []string{"s1"}},
		},
	})
	return raw
}
